# Independent VPC Development Strategy

## Overview

This document outlines how to develop and test the VPC component independently while other Banyan components (engine, agent, CLI) are being developed in parallel.

## Core Principle

**VPC as a Standalone Module** - The VPC component should be a self-contained Go module that can be imported by the Banyan engine but also run and tested independently.

## Architecture for Independence

```
banyan/
├── cmd/
│   └── banyan-vpc/     # VPC CLI (thin wrapper)
│       └── main.go     # Just parses args and calls VPC functions
├── pkg/
│   └── vpc/            # VPC module (all logic here)
│       ├── network/    # Network operations
│       ├── cni/        # CNI runtime
│       ├── ipam/       # IP management
│       ├── security/   # Security rules
│       ├── dns/        # Service discovery
│       └── debug/      # Debug utilities
└── test/
    └── vpc/            # Integration tests
```

## Development Approach

### 1. Contract-First Design

Define interfaces that both VPC and Banyan engine will agree on:

```go
// VPC exposes these interfaces to Banyan
type NetworkManager interface {
    CreateNetwork(config NetworkConfig) (*Network, error)
    AttachContainer(networkID, containerID string, opts AttachOptions) error
    SetSecurityRules(rules []SecurityRule) error
    GetServiceEndpoint(service string) (string, error)
}

// CNI Runtime Interface
type CNIRuntime interface {
    AddToNetwork(containerID, networkID string, ip net.IP) error
    RemoveFromNetwork(containerID, networkID string) error
    SetupPlugin(plugin string, config []byte) error
}

// IPAM Interface
type IPAMManager interface {
    AllocateHostSubnet(hostID string) (*net.IPNet, error)
    AllocateIP(subnet *net.IPNet) (net.IP, error)
    ReleaseIP(ip net.IP) error
    RenewLease(hostID string) error
}
```

### 2. VPC CLI as Thin Wrapper

The `banyan-vpc` CLI is just a thin wrapper that calls VPC component functions:

```go
// cmd/banyan-vpc/main.go - Thin CLI wrapper
func main() {
    switch cmd {
    case "dns":
        if subCmd == "register" {
            // Just parse args and call VPC function
            err := vpc.DNS.Register(hostname, ip)
        }
    case "network":
        if subCmd == "create" {
            err := vpc.Network.Create(cidr, vxlanID)
        }
    }
}

// pkg/vpc/dns/dns.go - Actual implementation
func (d *DNSManager) Register(hostname string, ip net.IP) error {
    // Actual DNS registration logic here
    return d.backend.AddRecord(hostname, ip)
}
```

CLI commands for development and debugging:
```bash
# Network operations
banyan-vpc network create --cidr 10.0.0.0/16
banyan-vpc network list
banyan-vpc network inspect vpc-default

# Container operations
banyan-vpc container attach --id <docker-id> --ip 10.0.1.5
banyan-vpc container list

# DNS operations
banyan-vpc dns register --hostname web --ip 10.0.1.5
banyan-vpc dns lookup web

# Debug operations (useful in production too!)
banyan-vpc debug trace --from 10.0.1.5 --to 10.0.2.10
banyan-vpc debug connectivity --container web
banyan-vpc debug iptables --filter 10.0.1.5
```

### 3. Direct Testing with Real Containers

VPC provides low-level utilities that the engine will call. For testing without the engine, use scripts that directly call VPC functions:

```bash
# test-setup.sh - Testing VPC without Banyan engine

# Create network using CLI (which calls VPC functions)
banyan-vpc network create --cidr 10.0.0.0/16 --id vpc-test

# Start real Docker containers (no network)
docker run -d --name web --network none nginx
docker run -d --name db --network none postgres

# Use VPC to attach containers to network
WEB_ID=$(docker inspect -f '{{.Id}}' web)
DB_ID=$(docker inspect -f '{{.Id}}' db)
banyan-vpc container attach --id $WEB_ID --ip 10.0.1.5
banyan-vpc container attach --id $DB_ID --ip 10.0.2.10

# Add security rules (low-level iptables)
banyan-vpc security add --from 10.0.1.5 --to 10.0.2.10 --port 5432
banyan-vpc security add --from 0.0.0.0/0 --to 10.0.1.5 --port 443

# Debug connectivity
banyan-vpc debug trace --from 10.0.1.5 --to 10.0.2.10
banyan-vpc debug connectivity --container web
```

The VPC component only knows about:
- IP addresses and CIDRs
- Container IDs
- Port numbers
- Allow/deny rules

It doesn't know about:
- Service names (that's the engine's job to map)
- Banyan config structure
- High-level concepts like "service:web"

## Testing Strategy

### Unit Tests
- Test each component in isolation
- Mock network interfaces (netlink)
- Verify iptables rules generation
- Test IPAM allocation logic

### Integration Tests (Container-based)
```bash
# Run test environment
docker-compose -f test/vpc/docker-compose.test.yml up

# Test multi-host networking
banyan-vpc test scenario multi-host
banyan-vpc test scenario security-rules
banyan-vpc test scenario dns-discovery
```

### Test Scenarios

1. **Single Host**
   - Create network
   - Attach 2 containers
   - Test connectivity
   - Apply security rules
   - Verify isolation

2. **Multi-Host** (using containers as "hosts")
   - Simulate 2 hosts with docker containers
   - Create VXLAN tunnel between them
   - Test cross-host connectivity
   - Verify security rules work across hosts

3. **Service Discovery**
   - Register services with DNS
   - Query DNS endpoints
   - Test health-aware updates
   - Verify load balancing

## Development Milestones

### Milestone 1: Local Networking
- [ ] Network creation with CNI
- [ ] Container attachment via CNI
- [ ] Basic connectivity test
- [ ] Unit tests for core functions

**Validation**: Two containers can ping each other

### Milestone 2: Security Rules
- [ ] IP-based security rules
- [ ] iptables rule generation
- [ ] Rule application and verification
- [ ] Isolation testing

**Validation**: Security rules correctly allow/deny traffic

### Milestone 3: Multi-Host
- [ ] Flannel overlay setup
- [ ] Cross-host routing
- [ ] Multi-host security rules
- [ ] Host failure handling

**Validation**: Containers communicate across hosts

### Milestone 4: State Management
- [ ] Embedded etcd cluster
- [ ] Hierarchical IPAM
- [ ] Lease management
- [ ] State synchronization

**Validation**: IPAM survives host failures

### Milestone 5: Integration
- [ ] Clean API documentation
- [ ] Performance benchmarks
- [ ] Error handling
- [ ] Integration test suite

**Validation**: VPC module ready for Banyan engine integration

## Environment Setup

### Development Environment

```bash
# Install dependencies
go mod init github.com/banyan/pkg/vpc
go get github.com/containernetworking/cni
go get github.com/containernetworking/plugins
go get github.com/coreos/go-iptables
go get go.etcd.io/etcd/client/v3

# Install Flannel CNI plugin
curl -L https://github.com/flannel-io/cni-plugin/releases/download/v1.2.0/flannel-amd64 -o /opt/cni/bin/flannel
chmod +x /opt/cni/bin/flannel

# Build and install the CLI
go build -o /usr/local/bin/banyan-vpc cmd/banyan-vpc/main.go

# Use the CLI (which calls VPC package functions)
banyan-vpc network create --cni flannel

# Run tests
go test ./pkg/vpc/...

# Integration test with containers
./test/vpc/run-integration.sh
```

### Docker-based Test Environment

```yaml
# test/vpc/docker-compose.test.yml
version: '3.8'

services:
  host1:
    image: vpc-test-host
    privileged: true
    cap_add:
      - NET_ADMIN
    networks:
      - testnet
    environment:
      - HOST_ID=1
      - VPC_CIDR=10.0.1.0/24

  host2:
    image: vpc-test-host
    privileged: true
    cap_add:
      - NET_ADMIN
    networks:
      - testnet
    environment:
      - HOST_ID=2
      - VPC_CIDR=10.0.2.0/24

networks:
  testnet:
    driver: bridge
```

## API Contracts (Low-Level)

### VPC Operations

```go
// Network Operations
CreateNetwork(cidr string, vxlanID int) error
DeleteNetwork(networkID string) error

// Container Operations
AttachContainer(containerID string, ip net.IP, hostID string) error
DetachContainer(containerID string) error

// Security Operations (pure IP-based)
AddSecurityRule(rule SecurityRule) error
type SecurityRule struct {
    FromIP   net.IP     // or CIDR
    ToIP     net.IP     // or CIDR
    Port     int
    Protocol string     // tcp/udp
    Action   string     // allow/deny
}

// DNS Operations (optional - VPC can work without it)
RegisterDNS(hostname string, ip net.IP) error
UnregisterDNS(hostname string) error

// IPAM Operations
AllocateIP(subnet *net.IPNet) (net.IP, error)
ReleaseIP(ip net.IP) error
```

### The Engine's Responsibility

The Banyan engine translates high-level concepts to VPC calls:

```go
// Engine translates "service:web" to actual IPs
if rule.From == "service:web" {
    webIPs := engine.GetServiceIPs("web")
    for _, ip := range webIPs {
        vpc.AddSecurityRule(SecurityRule{
            FromIP: ip,
            ToIP:   targetIP,
            Port:   rule.Port,
            Action: "allow",
        })
    }
}
```

## Benefits of This Approach

1. **Parallel Development** - VPC team works independently
2. **Early Testing** - Test networking before engine is ready
3. **Clear Contracts** - Well-defined interfaces
4. **Reusable** - VPC module can be used in other projects
5. **Debugging** - Easier to debug in isolation

## Integration Points

When ready to integrate with Banyan:

1. **Engine imports VPC module**
   ```go
   import "github.com/banyan/pkg/vpc"
   ```

2. **Engine implements ContainerRuntime interface**
   ```go
   type BanyanRuntime struct{}
   func (b *BanyanRuntime) GetContainer(id string) (*Container, error) {
       // Real implementation
   }
   ```

3. **Replace mocks with real implementations**
   - Real container IDs
   - Real host information
   - Real health checks

## Success Criteria

- [ ] VPC can run without Banyan engine
- [ ] All features testable via banyan-vpc CLI
- [ ] Clear API documentation
- [ ] Integration takes < 1 day
- [ ] No changes needed to VPC when integrating

## Summary

By developing VPC as an independent module with its own CLI and test harness, we can:
- Develop and test networking features immediately
- Not block on engine/agent development
- Maintain clear separation of concerns
- Enable easier debugging and testing

The key is defining clear interfaces early and using mocks to simulate the parts of Banyan that don't exist yet.