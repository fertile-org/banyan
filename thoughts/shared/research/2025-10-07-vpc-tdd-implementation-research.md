---
date: 2025-10-07T10:53:02+07:00
researcher: Hung Nguyen
git_commit: 40b050437164499c712e370722e1ae28447d8740
branch: feat/vpc-implementation
repository: banyan
topic: "VPC Package TDD Implementation Status"
tags: [research, codebase, vpc, networking, tdd, test-driven-development]
status: complete
last_updated: 2025-10-07
last_updated_by: Hung Nguyen
---

# Research: VPC Package TDD Implementation Status

**Date**: 2025-10-07T10:53:02+07:00
**Researcher**: Hung Nguyen
**Git Commit**: 40b050437164499c712e370722e1ae28447d8740
**Branch**: feat/vpc-implementation
**Repository**: banyan

## Research Question

Document the current state of the VPC (Virtual Private Cloud) networking implementation in the Banyan project following a Test-Driven Development (TDD) approach. This research provides a comprehensive overview of interfaces, types, package structure, test coverage, and how VPC fits into the overall Banyan architecture.

## Summary

The VPC package (`pkg/vpc/`) implements network management for the Banyan Docker Compose orchestrator using a comprehensive Test-Driven Development approach. The implementation consists of **6 core interfaces** defining networking capabilities, **6 stub manager implementations**, and **35 test functions** covering 197+ test scenarios across all components. All tests are written before implementations, following pure TDD methodology, with stub implementations returning `nil` values and comments indicating "TDD: Implementation will be added after test review".

### Key Findings:

- **Package Structure**: Modular design with 6 specialized packages (network, cni, ipam, security, dns, debug)
- **Interface-Driven**: All components defined as interfaces with stub implementations
- **Test Coverage**: Comprehensive test suite with table-driven tests, concurrency tests, and integration scenarios
- **Architectural Fit**: VPC designed as independent module that will be imported by Banyan engine
- **Implementation Status**: 100% stub implementations - all tests written, zero implementation code

## Detailed Findings

### VPC Package Structure

#### Package Hierarchy (`pkg/vpc/`)

```
pkg/vpc/
├── interfaces.go          # Core interface definitions (6 interfaces)
├── types.go              # Data structure definitions (12 types)
├── go.mod               # Independent module definition
├── network/             # Network management
│   ├── manager.go       # Stub NetworkManager implementation
│   └── manager_test.go  # 5 test functions, 297 lines
├── cni/                 # CNI runtime integration
│   ├── runtime.go       # Stub CNIRuntime implementation
│   └── runtime_test.go  # 4 test functions, 272 lines
├── ipam/                # IP address management
│   ├── manager.go       # Stub IPAMManager implementation
│   └── manager_test.go  # 8 test functions, 361 lines
├── security/            # Security rules & iptables
│   ├── manager.go       # Stub SecurityManager implementation
│   └── manager_test.go  # 6 test functions, 635 lines
├── dns/                 # DNS management
│   ├── manager.go       # Stub DNSManager implementation
│   └── manager_test.go  # 7 test functions, 404 lines
└── debug/               # Debugging utilities
    ├── manager.go       # Stub DebugManager implementation
    └── manager_test.go  # 5 test functions, 485 lines
```

#### Core Interfaces (`pkg/vpc/interfaces.go`)

**1. NetworkManager** - VPC network lifecycle management
- `CreateNetwork(ctx, config) (*Network, error)` - Create new VPC network
- `DeleteNetwork(ctx, networkID) error` - Remove network
- `GetNetwork(ctx, networkID) (*Network, error)` - Retrieve network info
- `ListNetworks(ctx) ([]*Network, error)` - List all networks

**2. CNIRuntime** - Container Network Interface operations
- `AddToNetwork(ctx, containerID, networkID, ip) error` - Attach container
- `RemoveFromNetwork(ctx, containerID, networkID) error` - Detach container
- `SetupPlugin(ctx, plugin, config) error` - Initialize CNI plugin (Flannel, Calico)
- `GetPluginStatus(ctx, plugin) (*PluginStatus, error)` - Get plugin status

**3. IPAMManager** - IP Address Management (hierarchical)
- `AllocateHostSubnet(ctx, hostID) (*net.IPNet, error)` - Allocate /24 subnet per host
- `AllocateIP(ctx, subnet) (net.IP, error)` - Allocate IP from subnet
- `ReleaseIP(ctx, ip) error` - Release IP
- `RenewLease(ctx, hostID) error` - Renew subnet lease
- `GetHostSubnet(ctx, hostID) (*net.IPNet, error)` - Get host subnet

**4. SecurityManager** - Security rules and iptables
- `AddRule(ctx, rule) error` - Add security rule
- `RemoveRule(ctx, ruleID) error` - Remove rule
- `ListRules(ctx, networkID) ([]*SecurityRule, error)` - List network rules
- `ApplyRules(ctx, networkID) error` - Apply rules to iptables

**5. DNSManager** - Service discovery and DNS
- `RegisterHost(ctx, hostname, ip) error` - Register DNS entry
- `UnregisterHost(ctx, hostname) error` - Remove DNS entry
- `LookupHost(ctx, hostname) ([]net.IP, error)` - Resolve hostname (health-aware)
- `UpdateHealth(ctx, hostname, healthy) error` - Update health status

**6. DebugManager** - Network debugging and diagnostics
- `TraceConnection(ctx, fromIP, toIP, port) (*TraceResult, error)` - Trace connectivity
- `CheckConnectivity(ctx, containerID) (*ConnectivityResult, error)` - Check container network
- `GetIPTablesRules(ctx, ip) ([]string, error)` - Inspect iptables

#### Type Definitions (`pkg/vpc/types.go`)

**Core Data Structures** (12 types, all with JSON tags):

1. **Network**: VPC network representation
   - ID, Name, CIDR, VxlanID, DNSSuffix, CreatedAt, Status

2. **NetworkConfig**: Configuration for creating networks
   - Name, CIDR (default: 10.0.0.0/16), VxlanID, DNSSuffix (default: .internal), Driver (flannel)

3. **SecurityRule**: Network security rule with explicit prefixes
   - ID, NetworkID, ServiceName, Direction (ingress/egress), Action (allow/deny)
   - From/To: `service:name`, `cidr:x.x.x.x/y`, or `internet`
   - ToPort: Port or range (e.g., "80" or "8000-8100"), Protocol

4. **Container**: Container in network
   - ID, NetworkID, IP, HostID, Status, CreatedAt

5. **PluginStatus**: CNI plugin status
   - Name, Version, Status (active/inactive/error), Error

6. **SubnetLease**: Hierarchical subnet allocation
   - HostID, Subnet (*net.IPNet), LeaseTime, ExpiresAt

7. **TraceHop**: Single hop in connection trace
   - Type (gateway, vxlan, container), Address, Latency

8. **TraceResult**: Connection trace results
   - FromIP, ToIP, Port, Status (allowed/blocked/error), Reachable
   - Hops ([]TraceHop), BlockedBy, BlockedByDetails, AllowedBy, Latency, Error

9. **ConnectivityResult**: Container connectivity check
   - ContainerID, Status (ok/degraded/error/unreachable), HasNetwork
   - IP, Gateway, DNS, InternetAccess, DefaultRoute
   - ExternalPing, InternalPing, Errors

10. **AttachOptions**: Container attachment options
    - IP (optional specific assignment), HostID

### Implementation Status

#### All Managers Follow Identical Pattern

**Example from `network/manager.go`:**
```go
package network

import (
    "context"
    "github.com/fertile/banyan/pkg/vpc"
)

// Manager implements vpc.NetworkManager interface
type Manager struct {
    // Will add fields during implementation phase
}

// NewManager creates a new network manager
func NewManager() *Manager {
    return &Manager{}
}

// CreateNetwork creates a new VPC network
func (m *Manager) CreateNetwork(ctx context.Context, config vpc.NetworkConfig) (*vpc.Network, error) {
    // TDD: Implementation will be added after test review
    return nil, nil
}

// Ensure Manager implements NetworkManager interface
var _ vpc.NetworkManager = (*Manager)(nil)
```

**Interface Compliance Status:**

| Interface | Implementation | Package | Status | Compliance Check |
|-----------|---------------|---------|--------|------------------|
| NetworkManager | network.Manager | pkg/vpc/network | Stub | ✓ Verified |
| CNIRuntime | cni.Runtime | pkg/vpc/cni | Stub | ✓ Verified |
| IPAMManager | ipam.Manager | pkg/vpc/ipam | Stub | ✓ Verified |
| SecurityManager | security.Manager | pkg/vpc/security | Stub | ✓ Verified |
| DNSManager | dns.Manager | pkg/vpc/dns | Stub | ✓ Verified |
| DebugManager | debug.Manager | pkg/vpc/debug | Stub | ✓ Verified |

All implementations include compile-time interface compliance: `var _ vpc.XxxManager = (*Manager)(nil)`

### Test Coverage Analysis

#### Test Suite Overview

| Component | Test Functions | Test Scenarios | Lines | Skipped Tests |
|-----------|----------------|----------------|-------|---------------|
| Network | 5 | ~25 | 297 | 0 |
| CNI | 4 | ~22 | 272 | 0 |
| IPAM | 8 | ~40 | 361 | 1 (lease expiration) |
| Security | 6 | ~45 | 635 | 0 |
| DNS | 7 | ~35 | 404 | 2 (TTL, reverse lookup) |
| Debug | 5 | ~30 | 485 | 0 |
| **TOTAL** | **35** | **~197** | **~2,454** | **3** |

#### Test Patterns Used

**1. Table-Driven Tests** (77% of tests)
```go
tests := []struct {
    name    string
    input   Type
    wantErr bool
    checks  func(t *testing.T, result *Type)
}{
    {name: "scenario description", ...},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

**2. Concurrent Operations** (2 tests)
- Network: 10 concurrent creates
- DNS: 10 concurrent registers + 10 concurrent lookups

**3. Integration Scenarios** (8 tests)
- Multi-tier security (Web → API → Database)
- Service discovery with load balancing
- Multi-hop network tracing
- Network partition diagnostics

#### Test Coverage by Scenario Type

```
Test Scenarios Distribution:
├── Happy Path Cases:        45 (23%)  - Valid inputs, expected outputs
├── Error Cases:             50 (25%)  - Invalid inputs, missing fields
├── Edge Cases:              15 (8%)   - Auto-assignment, updates, loopback
├── Integration Tests:        8 (4%)   - Complex multi-component scenarios
├── Concurrency Tests:        2 (1%)   - Race condition validation
├── Stress Tests:            4 (2%)   - Exhaustion testing
└── Validation Functions:    73 (37%)  - Custom check functions
```

#### Notable Test Features

**Security Tests** (`security/manager_test.go` - 635 lines):
- 11 scenarios for AddRule (service, CIDR, internet, deny, egress, port ranges)
- Rule translation to iptables verification
- Deny-by-default behavior testing
- Multi-tier application scenario (6 security rules)

**IPAM Tests** (`ipam/manager_test.go` - 361 lines):
- Hierarchical subnet allocation (10.0.1.0/24, 10.0.2.0/24, ...)
- IP allocation within subnet (.2, .3, .4 - .1 reserved for gateway)
- Stress tests for subnet and IP exhaustion

**DNS Tests** (`dns/manager_test.go` - 404 lines):
- Multiple IPs per hostname (load balancing)
- Health-aware DNS (unhealthy hosts excluded from lookup)
- Concurrent operations (20 goroutines)

**Debug Tests** (`debug/manager_test.go` - 485 lines):
- Multi-hop connection tracing (gateway, VXLAN hops)
- Network partition diagnostics
- Security rule conflict identification
- Performance diagnostics (MTU issues, packet drops)

#### Skipped Tests (3)

1. `TestIPAMManager_LeaseExpiration` - Requires time manipulation
2. `TestDNSManager_TTL` - Requires time manipulation
3. `TestDNSManager_ReverseLookup` - Feature may not be needed

All marked with `t.Skip()` and explanation comments.

### Banyan Architecture Context

#### Overall Project Structure

```
banyan/
├── cmd/               # Binary entry points
│   ├── cli/          # CLI binary (user machine/CI) - Stub
│   ├── engine/       # Engine binary (orchestrator server) - Stub
│   └── agent/        # Agent binary (target servers) - Stub
├── pkg/              # Public packages
│   ├── interfaces/   # Public interfaces (Engine, Agent)
│   ├── plugin-sdk/   # Plugin SDK for community
│   └── vpc/          # VPC networking module ← THIS RESEARCH
├── internal/         # Private packages
│   └── common/       # Shared utilities (Version, Logger)
├── test/             # Test suites
│   └── unit/         # Unit tests
└── docs/             # Documentation
    └── vpc/          # VPC documentation
```

#### How VPC Fits into Banyan

**Current State:**
- All cmd/ binaries are minimal stubs (just print startup messages)
- pkg/interfaces/ defines Engine and Agent interfaces
- pkg/vpc/ is the most developed package (interfaces + tests complete)

**Designed Integration Pattern:**
```
Banyan Engine (cmd/engine)
        ↓
  Uses pkg/vpc interfaces:
  - NetworkManager
  - IPAMManager
  - SecurityManager
  - DNSManager
  - CNIRuntime
  - DebugManager
        ↓
  VPC module provides implementations:
  - network.Manager
  - ipam.Manager
  - security.Manager
  - dns.Manager
  - cni.Runtime
  - debug.Manager
```

**VPC Module Independence:**
- Separate Go module (`pkg/vpc/go.mod`)
- Can be developed and tested independently
- Will be imported by Banyan engine when ready

#### Common Patterns in Banyan

1. **Go Workspace**: Monorepo with multiple modules
2. **Interface-First Design**: Contracts defined before implementation
3. **Manager Pattern**: Functional areas organized as managers
4. **Thin Binaries**: cmd/ minimal logic, delegates to packages
5. **TDD Approach**: Tests exist before implementations
6. **Context-First**: All methods take `context.Context` first
7. **JSON Serialization**: All types support JSON marshaling

## Code References

### Interfaces and Types
- `pkg/vpc/interfaces.go:9-21` - NetworkManager interface
- `pkg/vpc/interfaces.go:24-36` - CNIRuntime interface
- `pkg/vpc/interfaces.go:39-54` - IPAMManager interface
- `pkg/vpc/interfaces.go:57-69` - SecurityManager interface
- `pkg/vpc/interfaces.go:72-84` - DNSManager interface
- `pkg/vpc/interfaces.go:87-96` - DebugManager interface
- `pkg/vpc/types.go:9-17` - Network type
- `pkg/vpc/types.go:29-39` - SecurityRule type
- `pkg/vpc/types.go:75-87` - TraceResult type
- `pkg/vpc/types.go:90-103` - ConnectivityResult type

### Manager Implementations
- `pkg/vpc/network/manager.go:10-12` - Manager struct (stub)
- `pkg/vpc/cni/runtime.go:10-12` - Runtime struct (stub)
- `pkg/vpc/ipam/manager.go:10-12` - Manager struct (stub)
- `pkg/vpc/security/manager.go:10-12` - Manager struct (stub)
- `pkg/vpc/dns/manager.go:10-12` - Manager struct (stub)
- `pkg/vpc/debug/manager.go:10-12` - Manager struct (stub)

### Test Files
- `pkg/vpc/network/manager_test.go` - 297 lines, 5 test functions
- `pkg/vpc/cni/runtime_test.go` - 272 lines, 4 test functions
- `pkg/vpc/ipam/manager_test.go` - 361 lines, 8 test functions
- `pkg/vpc/security/manager_test.go` - 635 lines, 6 test functions
- `pkg/vpc/dns/manager_test.go` - 404 lines, 7 test functions
- `pkg/vpc/debug/manager_test.go` - 485 lines, 5 test functions

## Architecture Documentation

### VPC Design Principles

From documentation (`docs/vpc/02-vpc-architecture.md`):

1. **CNI from Day One** - Use Flannel CNI plugin for proven networking
2. **Hide Complexity** - Users only see simple `allow` rules, never CNI details
3. **Smart Defaults** - Auto-generate configs from docker-compose
4. **Progressive Enhancement** - Advanced features available when needed

### Key Technical Decisions

**Flannel CNI as First Implementation:**
- Most widely deployed simple CNI plugin
- VXLAN backend for overlay networking
- Minimal dependencies (works without Kubernetes)
- Easy migration to Calico/Cilium later

**Hierarchical IPAM Design:**
```
VPC CIDR: 10.0.0.0/16
    ↓
Host-1: 10.0.1.0/24 (254 IPs)
Host-2: 10.0.2.0/24 (254 IPs)
Host-3: 10.0.3.0/24 (254 IPs)
```

Benefits:
- No IP conflicts between hosts
- Fast local allocation
- Survives network partitions
- Simple garbage collection via lease expiry

**Security Model - Deny by Default:**
- No service can communicate unless explicitly allowed
- All rules use explicit prefixes: `service:name`, `cidr:x.x.x.x/y`, `internet`
- Translates to iptables rules with default DROP policy

### Component Relationships

```
┌─────────────────┐
│  NetworkManager │ - Creates networks with CIDR, VXLAN ID
└────────┬────────┘
         │ uses
         ▼
┌─────────────────┐
│   IPAMManager   │ - Allocates /24 subnets to hosts
└────────┬────────┘ - Allocates IPs within subnets
         │          - Manages lease renewals
         │
         ▼
┌─────────────────┐
│   CNIRuntime    │ - Attaches containers with IPs
└────────┬────────┘ - Manages Flannel/Calico plugins
         │
         │ coordinates with
         │
         ├──────────────┐
         ▼              ▼
┌─────────────────┐  ┌─────────────────┐
│ SecurityManager │  │   DNSManager    │
└─────────────────┘  └─────────────────┘
│                    │
│ - iptables rules   │ - Service discovery
│ - Allow/deny       │ - Health-aware DNS
│                    │ - Load balancing
│                    │
└────────┬───────────┴─────────────┐
         │                         │
         ▼                         ▼
┌─────────────────────────────────────┐
│         DebugManager                │
│ - Connection tracing                │
│ - Connectivity diagnostics          │
│ - iptables inspection               │
└─────────────────────────────────────┘
```

## Historical Context (from thoughts/)

No historical research documents found for VPC implementation. This is a new feature being developed from scratch using TDD methodology.

## Related Research

This is the first research document for the VPC implementation in Banyan.

## Open Questions

### Implementation Phase Questions

1. **Storage Abstraction**: Need to add `pkg/vpc/storage/interface.go` defining StateStore interface for subnet leases, service registry, and security rules. Will start with in-memory implementation for TDD, then add etcd backend later.
2. **State Management**: How will embedded etcd be integrated for subnet leases and service registry?
3. **CNI Plugin Selection**: Should Flannel configuration be hardcoded or pluggable from day one?
4. **iptables Translation**: What is the exact translation strategy for security rules to iptables chains?
5. **DNS Backend**: CoreDNS integration approach - standalone or embedded?
6. **Error Handling**: What specific error types should be defined for each manager?
7. **Concurrency**: What locking strategies are needed for IPAM and security rule management?
8. **Persistence**: How will network state persist across restarts?
9. **Migration**: Strategy for upgrading CNI plugins or changing CIDR blocks?

### Testing Questions

1. **Integration Tests**: How to test with real Docker containers and CNI plugins?
2. **Multi-Host Testing**: Strategy for testing VXLAN overlay across multiple hosts?
3. **Performance Benchmarks**: What are the acceptable latency and throughput targets?
4. **Chaos Testing**: How to simulate network partitions, host failures, and split-brain scenarios?

### Architecture Questions

1. **banyan-vpc CLI**: Should this be a separate binary or part of main Banyan CLI?
2. **Engine Integration**: What is the exact API contract between engine and VPC package?
3. **Plugin Architecture**: Can the VPC module itself be pluggable for alternative networking backends?

## Next Steps

Based on the research findings, the recommended next steps are:

### Immediate (Implementation Phase)

1. **Review Tests with Team**: Validate test expectations before implementing
2. **Start with Network Manager**: Implement core network creation/deletion
3. **Implement IPAM**: Hierarchical subnet allocation with lease management
4. **CNI Integration**: Flannel plugin setup and container attachment
5. **Security Rules**: Translate to iptables commands
6. **DNS Integration**: CoreDNS or embedded DNS implementation

### Short-term (Integration)

1. **Create banyan-vpc CLI**: Thin wrapper for debugging and development
2. **Integration Testing**: Docker-based test environment with real containers
3. **Engine Integration**: Define exact API contracts and implement in engine
4. **Documentation**: Complete API reference and user guide

### Long-term (Production Readiness)

1. **State Management**: Embedded etcd cluster implementation
2. **Advanced Features**: Calico support, egress control, observability
3. **Performance Optimization**: Benchmarking and tuning
4. **Production Testing**: Multi-region, high-availability scenarios

## Conclusion

The VPC package demonstrates **exemplary Test-Driven Development practices** with comprehensive test coverage across all networking components. The architecture is well-designed with clear interfaces, separation of concerns, and independence from the main Banyan components.

**Key Strengths:**
- ✅ Complete interface definitions (6 interfaces, 12 types)
- ✅ Comprehensive test suite (35 test functions, 197+ scenarios)
- ✅ Consistent patterns (Manager, Context-first, Interface compliance)
- ✅ Production-ready design (deny-by-default security, hierarchical IPAM)
- ✅ Independent module (can develop/test separately)

**Current Status:**
- ✅ All tests written (100% TDD compliance)
- ⏳ Zero implementation code (stub phase)
- 🎯 Ready for implementation phase
- 📝 Well-documented with comprehensive design docs

The VPC implementation is ready to proceed to the implementation phase with high confidence in the test coverage and architectural soundness.