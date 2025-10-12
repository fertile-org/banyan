# VPC Implementation Summary

Quick reference for Banyan VPC networking implementation. This document summarizes the purpose and components of each phase.

---

## Overview

**Purpose**: Provide AWS-like virtual private cloud networking for distributed applications.

**Key Features**:
- Multi-host overlay networking (VXLAN via Flannel)
- Hierarchical IPAM (VPC → Subnets → IPs)
- Container network isolation via CNI
- Security groups and network policies
- Service discovery via DNS

**Architecture**: Daemon model
- **banyan-engine**: Privileged daemon (handles network operations)
- **banyan-cli**: User client (no sudo required)
- **vpc-cli**: Admin/debug tool (requires sudo for modifications)

---

## Phase 1: Storage & NetworkManager ✅

**Completed**: 2025-10-12

### Components

**1. Storage Layer** (`pkg/vpc/storage/`)
- **Purpose**: Persist VPC state to disk
- **Implementation**: Key-value store with JSON file backend
- **Location**: `/var/lib/banyan/vpc/state.json` (root) or `~/.vpc/state.json` (user)

**2. NetworkManager** (`pkg/vpc/network/`)
- **Purpose**: Manage VPC network lifecycle
- **Responsibilities**:
  - Create/delete VPC networks
  - Assign CIDR blocks (e.g., 10.0.0.0/16)
  - Track network metadata

---

## Phase 2: Hierarchical IPAM ✅

**Completed**: 2025-10-12

### Components

**IPAM Manager** (`pkg/vpc/ipam/`)
- **Purpose**: Manage IP address allocation within VPCs
- **Hierarchy**:
  ```
  VPC (10.0.0.0/16)
    ├─ Host-1 Subnet (10.0.1.0/24)
    ├─ Host-2 Subnet (10.0.2.0/24)
    └─ Host-3 Subnet (10.0.3.0/24)
  ```
- **Responsibilities**:
  - Allocate /24 subnets to hosts
  - Allocate individual IPs within host subnets
  - Manage IP leases (default: 24h)
  - Support lease renewal and release

---

## Phase 3: CNI Runtime Integration ✅

**Completed**: 2025-10-12

### Components

**1. CNI Runtime** (`pkg/vpc/cni/`)
- **Purpose**: Attach containers to VPC networks using CNI protocol
- **Responsibilities**:
  - Execute CNI binaries (Flannel/Calico)
  - Translate high-level ops → CNI ADD/DEL commands
  - Track container network attachments
  - Manage plugin configuration
  - **Auto-create network namespaces** for containers
  - **Auto-cleanup namespaces** on removal

**2. CNI Setup Tool** (`cmd/vpc-cli/cmd/setup.go`)
- **Purpose**: Automate CNI plugin installation
- **Installs**:
  - Standard CNI plugins (v1.8.0): bridge, host-local, portmap
  - Flannel CNI plugin (v1.7.1-flannel1): VXLAN overlay
  - **Flannel daemon (flanneld v0.25.4)**: Background process for VXLAN management
- **Smart Installation**: Detects and installs only missing components

**3. Automatic Daemon Management** (`cmd/vpc-cli/cmd/cni.go`)
- **Purpose**: Handle Flannel daemon lifecycle automatically
- **Responsibilities**:
  - **Auto-start flanneld** during plugin setup
  - Detect if daemon already running
  - Create subnet configuration (/run/flannel/subnet.env)
  - Track daemon PID (/var/run/flanneld.pid)
  - Log daemon output (/var/log/flanneld.log)

**4. Flannel Plugin**
- **Purpose**: Enable cross-host container communication
- **How it works**: VXLAN overlay encapsulates container traffic for routing between hosts
- **Automation**: Banyan handles everything - no manual daemon or namespace management required

---

## Phase 4: Security Rules & iptables (Planned)

**Status**: Not started

### Purpose
Translate security rules into iptables for network filtering.

### Planned Components
- Security group management
- Rule translation: `service:name`, `cidr:x.x.x.x`, `internet` → iptables

---

## Phase 5: DNS Service Discovery (Planned)

**Status**: Not started

### Purpose
Enable DNS-based service discovery within VPCs.

### Planned Components
- DNS server for VPC-internal resolution
- Service name → container IP mapping

---

## Architecture Decisions

**Storage**: Single JSON file with auto-persistence
**Privileges**: vpc-cli requires sudo for setup/modifications, no sudo for reads
**CNI**: Industry-standard protocol with plugin-based architecture (Flannel, Calico)
**Testing**: Unit tests (no privileges) + integration tests (skip when unavailable)

---

## File Structure

```
pkg/vpc/
├── storage/          # State persistence
├── network/          # VPC network lifecycle
├── ipam/             # IP address management
└── cni/              # CNI integration

cmd/vpc-cli/cmd/
├── root.go           # CLI root + storage
├── setup.go          # CNI plugin installer
├── network.go        # Network commands
├── ipam.go           # IPAM commands
└── cni.go            # CNI commands
```

---

*Last Updated: 2025-10-12 (Phase 3 completed)*
