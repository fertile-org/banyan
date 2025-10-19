# VPC Architecture

## Table of Contents

1. [System Overview](#1-system-overview)
2. [VPC Component](#2-vpc-component)
3. [Engine Design](#3-engine-design)
   - [Responsibilities](#responsibilities)
   - [Why etcd?](#why-etcd)
   - [CLI Commands](#cli-commands)
4. [Agent Design](#4-agent-design)
   - [Responsibilities](#responsibilities-1)
   - [Network Flow](#network-flow)
   - [CLI Commands](#cli-commands-1)
5. [Implementation Status](#5-implementation-status)
6. [Configuration Example](#6-configuration-example)
7. [Testing](#7-testing)

## 1. System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                      Banyan Engine                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │               etcd Server (Central)                   │  │
│  │  - VPC network range: 10.0.0.0/16                    │  │
│  │  - Subnet leases: 10.0.X.0/24 per host (TTL)        │  │
│  │  - Service registry                                   │  │
│  └───────────────────┬──────────────────────────────────┘  │
└────────────────────┼─────────────────────────────────────────┘
                     │
         ┌───────────┴────────────┐
         ▼                        ▼
   ┌──────────┐            ┌──────────┐
   │ Agent-1  │            │ Agent-2  │
   │          │            │          │
   │ Flannel  │◄──VXLAN───►│ Flannel  │
   │10.0.1/24 │            │10.0.2/24 │
   │          │            │          │
   │┌────────┐│            │┌────────┐│
   ││Container││            ││Container││
   ││.1.10   ││            ││.2.10   ││
   │└────────┘│            │└────────┘│
   └──────────┘            └──────────┘
```

## 2. VPC Component

### Core Modules

**IPAM Manager** (`pkg/vpc/ipam/`)
- Hierarchical IP allocation: VPC CIDR → /24 subnets → individual IPs
- Lease-based subnet assignment with TTL (24h default)
- Automatic lease renewal every 5 minutes
- Gap detection for subnet reuse after expiration

**CNI Runtime** (`pkg/vpc/cni/`)
- Manages Flannel CNI plugin lifecycle
- Container network attachment/detachment via CNI ADD/DEL operations
- Plugin configuration and status monitoring

Flannel provides VXLAN overlay networking:
- Creates virtual network spanning multiple hosts
- Routes traffic between containers on different hosts through VXLAN tunnels
- Encapsulates L2 Ethernet frames in UDP packets for cross-host communication
- Automatically maintains routing tables based on subnet allocations in etcd

**Security Manager** (`pkg/vpc/security/`)
- Translates allow/deny rules to iptables
- Per-container network policies
- Integration with CNI for rule enforcement

**Storage Layer** (`pkg/vpc/storage/`)
- Abstraction over etcd and in-memory stores
- TTL-based key expiration for lease management
- Optimistic locking for distributed coordination

## 3. Engine Design

### Responsibilities

1. **etcd Server Management**
   - Starts single-node etcd on Engine host
   - Default endpoint: `http://0.0.0.0:2379`
   - Peer URLs use explicit IPv4: `http://127.0.0.1:2380`
   - Data dir: `/var/lib/banyan/etcd`

2. **Network Initialization**
   - Engine calls VPC initialization: `vpc.InitializeNetwork(store, vpcCIDR)`
   - VPC package writes Flannel config to etcd at `/coreos.com/network/config`
   - Config includes: network range (`10.0.0.0/16`), subnet length (/24), VXLAN backend

3. **State Coordination**
   - Central registry for subnet assignments
   - Service discovery endpoint
   - Health monitoring of agents

### Why etcd?

A distributed key-value store used by both Flannel and Banyan:

**Required by Flannel:**
- Stores network config at `/coreos.com/network/config`
- Coordinates subnet allocation via `/coreos.com/network/subnets/`
- Watch mechanism for agents to detect subnet changes and update VXLAN routes

**Used by Banyan IPAM:**
- Stores subnet leases at `/banyan/vpc/ipam/leases/{hostID}`
- TTL-based lease expiration
- Atomic operations prevent IP conflicts

Alternative: Flannel supports other backends (Kubernetes API, direct routing), but etcd mode is simplest for standalone deployment.

### CLI Commands

```bash
# etcd management
vpc-cli etcd setup    # Download and install etcd binaries
vpc-cli etcd start    # Start etcd server
vpc-cli etcd stop     # Stop etcd server
vpc-cli etcd status   # Check etcd health
```

## 4. Agent Design

### Responsibilities

1. **Subnet Acquisition**
   - Connects to Engine's etcd at startup
   - Requests /24 subnet via IPAM Manager
   - Starts automatic lease renewal background process

2. **Flannel Daemon**
   - Runs flanneld with etcd backend
   - Interface: `--iface=eth0`
   - Subnet file: `/run/flannel/subnet.env`
   - VXLAN tunnel establishment

3. **Container Networking**
   - Creates network namespaces for containers
   - Invokes Flannel CNI plugin for attachment
   - Allocates IPs from host's /24 subnet
   - Configures routes via CNI bridge

4. **Local IPAM**
   - Fast IP allocation from local subnet (no etcd query)
   - Range: host_subnet.2 to host_subnet.254
   - .1 reserved for gateway

### Network Flow

```
Container Creation:
1. Agent allocates IP from local /24 subnet
2. Creates container with --network=none
3. Gets container PID, creates netns symlink
4. Invokes CNI ADD: flannel plugin attaches eth0
5. Routes configured via cbr0 bridge
6. Flannel routes cross-host traffic via VXLAN

Cross-Host Communication:
Container-A (10.0.1.10) -> Container-B (10.0.2.10)
1. Packet hits cbr0 bridge on Host-1
2. Flannel checks route: 10.0.2.0/24 -> flannel.1 VXLAN
3. VXLAN encapsulates packet, sends to Host-2
4. Host-2 decapsulates, forwards to cbr0
5. cbr0 delivers to Container-B
```

### CLI Commands

```bash
# CNI management
vpc-cli cni setup-plugin flannel --etcd-endpoints=http://ENGINE_IP:2379
vpc-cli cni add-container <container-id> <network-id> <ip>
vpc-cli cni remove-container <container-id> <network-id>
vpc-cli cni get-status flannel

# IPAM operations
vpc-cli ipam allocate-subnet <host-id>
vpc-cli ipam allocate-ip <subnet>
vpc-cli ipam release-ip <ip>
vpc-cli ipam renew-lease <host-id>
vpc-cli ipam get-subnet <host-id>
```

## 5. Implementation Status

**Completed:**
- Hierarchical IPAM with TTL leases
- etcd integration (EtcdStore, MemoryStore)
- Flannel CNI runtime
- Automatic lease renewal
- Cross-host container communication
- Multi-host integration tests

**In Progress:**
- Security rule enforcement (iptables)
- DNS service discovery (CoreDNS)

**Planned:**
- Network policy management
- Observability (metrics, flow logs)
- Multi-CNI support (Calico, Cilium)

## 6. Configuration Example

**Flannel Network Config** (stored in etcd)
```json
{
  "Network": "10.0.0.0/16",
  "SubnetLen": 24,
  "Backend": {
    "Type": "vxlan"
  }
}
```

**Flannel CNI Config** (`/etc/cni/net.d/10-flannel.conf`)
```json
{
  "name": "cbr0",
  "cniVersion": "0.3.1",
  "type": "flannel",
  "delegate": {
    "bridge": "cbr0",
    "hairpinMode": true,
    "isDefaultGateway": true
  }
}
```

## 7. Testing

Multi-host integration test validates:
- etcd coordination
- Dynamic subnet allocation
- CNI network attachment
- Cross-host VXLAN communication
- State persistence in etcd

Run test:
```bash
sudo go run test/integration/vpc/run_multi_host_integration.go
```

Test creates 2 simulated hosts (containerd-in-containerd), allocates subnets, creates containers, and verifies bidirectional ping across VXLAN overlay.
