# VPC Networking Implementation Plan (TDD)

## Overview

Implement the VPC (Virtual Private Cloud) networking module for Banyan using Test-Driven Development methodology. All tests are already written (35 test functions, 197+ scenarios). This plan implements the 6 core managers to make tests pass while following the principle: **simple implementation with extensible interfaces**.

## Current State Analysis

### What Exists:
- ✅ 6 interfaces fully defined (`pkg/vpc/interfaces.go`)
- ✅ 12 types with JSON serialization (`pkg/vpc/types.go`)
- ✅ 6 stub manager implementations (all return `nil`)
- ✅ 35 test functions with 197+ test scenarios
- ✅ Comprehensive documentation (`docs/vpc/`)

### What's Missing:
- ❌ Actual implementation code (all managers are stubs)
- ❌ Storage abstraction layer
- ❌ CNI integration with Flannel
- ❌ iptables rule management
- ❌ DNS service discovery
- ❌ Network debugging utilities

### Key Discoveries:
- All managers follow consistent pattern: `pkg/vpc/interfaces.go:9-96`
- Tests use table-driven approach with custom check functions
- Security model is deny-by-default with explicit prefixes (service:, cidr:, internet)
- Hierarchical IPAM: VPC gets /16, hosts get /24, containers get individual IPs
- Interface compliance verified at compile time: `var _ vpc.XxxManager = (*Manager)(nil)`

## Desired End State

### Success Criteria:
- All 35 test functions pass (currently failing with stub implementations)
- Zero lint errors: `make lint` or `golangci-lint run`
- Full test coverage: `go test -v ./pkg/vpc/...`
- VPC package can create networks, allocate IPs, attach containers, apply security rules, and provide service discovery
- Ready for integration with Banyan engine

### How to Verify:
```bash
cd pkg/vpc
go test -v ./...                    # All tests pass
golangci-lint run                   # No lint errors
go build ./...                       # Compiles successfully
```

## What We're NOT Doing

**Out of Scope for MVP:**
- ❌ Embedded etcd integration (use in-memory storage first)
- ❌ Calico/Cilium CNI plugins (Flannel only)
- ❌ CoreDNS integration (use /etc/hosts)
- ❌ Network migration tools
- ❌ Multi-region support
- ❌ Performance benchmarking
- ❌ Observability/metrics
- ❌ banyan-vpc CLI (defer to Phase 2)

**Deferred to Future:**
- Advanced CNI plugins (after Flannel works)
- etcd state management (after in-memory works)
- CoreDNS (after /etc/hosts works)
- Network observability
- Load balancer integration

## Implementation Approach

**Strategy:** Implement bottom-up, simplest working solution first, with extensible interfaces for future enhancements.

**Order of Implementation:**
1. Storage abstraction (foundation for all managers)
2. NetworkManager (creates networks)
3. IPAMManager (allocates IPs)
4. CNIRuntime (attaches containers)
5. SecurityManager (applies firewall rules)
6. DNSManager (service discovery)
7. DebugManager (diagnostics)

**Key Principles:**
- Make tests pass, not more
- Simple implementation (in-memory, exec commands)
- Extensible interfaces (swap backends later)
- No premature optimization

## vpc-cli: Unified Development & Testing Tool

Instead of creating multiple throwaway test binaries, we'll build a **single production-ready CLI** (`vpc-cli`) that grows with each implementation phase.

### Architecture

```
cmd/vpc-cli/
├── main.go              # Cobra root command + global flags
├── cmd/
│   ├── network.go       # Phase 1: network create/list/get/delete
│   ├── ipam.go          # Phase 2: ipam allocate-subnet/allocate-ip/release-ip
│   ├── cni.go           # Phase 3: cni setup-plugin/add-container/remove-container
│   ├── security.go      # Phase 4: security add-rule/list-rules/apply-rules
│   ├── dns.go           # Phase 5: dns register/lookup/unregister
│   └── debug.go         # Phase 6: debug trace/check-connectivity/get-iptables
```

### Usage Pattern

```bash
# Phase 1 - Network commands
vpc-cli network create my-vpc --cidr 10.5.0.0/16
vpc-cli network list
vpc-cli network get <network-id>
vpc-cli network delete <network-id>

# Phase 2 - IPAM commands
vpc-cli ipam allocate-subnet host-1
vpc-cli ipam allocate-ip host-1
vpc-cli ipam release-ip 10.0.1.5

# Phase 3 - CNI commands
vpc-cli cni setup-plugin flannel
vpc-cli cni add-container container-1 network-1 10.0.1.5

# Phase 4 - Security commands
vpc-cli security add-rule --from internet --to 10.0.1.5 --port 443
vpc-cli security apply-rules network-1

# Phase 5 - DNS commands
vpc-cli dns register web.internal 10.0.1.5
vpc-cli dns lookup web.internal

# Phase 6 - Debug commands
vpc-cli debug trace 10.0.1.5 10.0.2.10 8080
vpc-cli debug check-connectivity container-1
```

### Benefits

- ✅ **Single binary** - one tool for all VPC operations
- ✅ **Production-ready** - becomes `banyan vpc` CLI later
- ✅ **Incremental** - add subcommands as you implement managers
- ✅ **Consistent UX** - familiar pattern like `kubectl`, `docker`, `gh`
- ✅ **No throwaway code** - real deliverable from day one

### Implementation Strategy

Each phase will:
1. Implement the manager (e.g., `pkg/vpc/network/manager.go`)
2. Add corresponding CLI subcommands (e.g., `cmd/vpc-cli/cmd/network.go`)
3. Test using `vpc-cli <subcommand>` instead of separate test binaries

---

## Phase 1: Storage Abstraction & NetworkManager

### Overview
Create the storage abstraction layer and implement NetworkManager to create/delete/list VPC networks. This provides the foundation for all other managers.

**IMPORTANT VNI Fix**: The original plan had a bug where VxlanID defaulted to 4789 (the VXLAN UDP port). This has been corrected to use sequential VNI allocation starting from 100, ensuring proper network isolation. Each network gets a unique VNI: 100, 101, 102, etc.

### Changes Required:

#### 1. Storage Interface
**File**: `pkg/vpc/storage/interface.go` (new)
**Changes**: Define StateStore interface for state management

```go
package storage

import "context"

// StateStore provides persistence for VPC state
type StateStore interface {
    // Save stores a value by key
    Save(ctx context.Context, key string, value interface{}) error

    // Get retrieves a value by key
    Get(ctx context.Context, key string, value interface{}) error

    // Delete removes a value by key
    Delete(ctx context.Context, key string) error

    // List returns all keys with a given prefix
    List(ctx context.Context, prefix string) ([]string, error)
}
```

#### 2. In-Memory Storage Implementation
**File**: `pkg/vpc/storage/memory.go` (new)
**Changes**: Implement in-memory storage with mutex

```go
package storage

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "sync"
)

// MemoryStore implements StateStore with in-memory storage
type MemoryStore struct {
    mu   sync.RWMutex
    data map[string][]byte
}

func NewMemoryStore() *MemoryStore {
    return &MemoryStore{
        data: make(map[string][]byte),
    }
}

func (m *MemoryStore) Save(ctx context.Context, key string, value interface{}) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("failed to marshal value: %w", err)
    }

    m.data[key] = data
    return nil
}

func (m *MemoryStore) Get(ctx context.Context, key string, value interface{}) error {
    m.mu.RLock()
    defer m.mu.RUnlock()

    data, ok := m.data[key]
    if !ok {
        return fmt.Errorf("key not found: %s", key)
    }

    if err := json.Unmarshal(data, value); err != nil {
        return fmt.Errorf("failed to unmarshal value: %w", err)
    }

    return nil
}

func (m *MemoryStore) Delete(ctx context.Context, key string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    delete(m.data, key)
    return nil
}

func (m *MemoryStore) List(ctx context.Context, prefix string) ([]string, error) {
    m.mu.RLock()
    defer m.mu.RUnlock()

    var keys []string
    for k := range m.data {
        if strings.HasPrefix(k, prefix) {
            keys = append(keys, k)
        }
    }

    return keys, nil
}
```

#### 3. NetworkManager Implementation
**File**: `pkg/vpc/network/manager.go`
**Changes**: Implement actual network management logic

```go
package network

import (
    "context"
    "fmt"
    "time"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
    "github.com/google/uuid"
)

type Manager struct {
    store storage.StateStore
}

func NewManager(store storage.StateStore) *Manager {
    return &Manager{
        store: store,
    }
}

// allocateVxlanID finds the next available VNI starting from 100
// VNI range: 100-16777215 (avoiding lower VNIs that might be reserved)
func (m *Manager) allocateVxlanID(existingNetworks []*vpc.Network) int {
    maxVNI := 99 // Start from 100
    for _, net := range existingNetworks {
        if net.VxlanID > maxVNI {
            maxVNI = net.VxlanID
        }
    }
    return maxVNI + 1
}

func (m *Manager) CreateNetwork(ctx context.Context, config vpc.NetworkConfig) (*vpc.Network, error) {
    // Apply defaults
    if config.CIDR == "" {
        config.CIDR = "10.0.0.0/16"
    }
    if config.DNSSuffix == "" {
        config.DNSSuffix = "internal"
    }
    if config.Driver == "" {
        config.Driver = "flannel"
    }

    // Auto-allocate unique VxlanID (VNI) if not specified
    // Start from 100, increment sequentially for each new network
    // Note: 4789 is the VXLAN UDP port, NOT a VNI!
    if config.VxlanID == 0 {
        config.VxlanID = m.allocateVxlanID(existingNetworks)
    } else {
        // Validate user-specified VxlanID for collision
        for _, net := range existingNetworks {
            if net.VxlanID == config.VxlanID {
                return nil, fmt.Errorf("VxlanID %d already in use", config.VxlanID)
            }
        }
    }

    // Create network object
    network := &vpc.Network{
        ID:        uuid.New().String(),
        Name:      config.Name,
        CIDR:      config.CIDR,
        VxlanID:   config.VxlanID,
        DNSSuffix: config.DNSSuffix,
        CreatedAt: time.Now(),
        Status:    "active",
    }

    // Save to store
    key := fmt.Sprintf("networks/%s", network.ID)
    if err := m.store.Save(ctx, key, network); err != nil {
        return nil, fmt.Errorf("failed to save network: %w", err)
    }

    return network, nil
}

func (m *Manager) DeleteNetwork(ctx context.Context, networkID string) error {
    key := fmt.Sprintf("networks/%s", networkID)

    // Check if network exists
    var network vpc.Network
    if err := m.store.Get(ctx, key, &network); err != nil {
        return fmt.Errorf("network not found: %w", err)
    }

    // Delete network
    if err := m.store.Delete(ctx, key); err != nil {
        return fmt.Errorf("failed to delete network: %w", err)
    }

    return nil
}

func (m *Manager) GetNetwork(ctx context.Context, networkID string) (*vpc.Network, error) {
    key := fmt.Sprintf("networks/%s", networkID)

    var network vpc.Network
    if err := m.store.Get(ctx, key, &network); err != nil {
        return nil, fmt.Errorf("network not found: %w", err)
    }

    return &network, nil
}

func (m *Manager) ListNetworks(ctx context.Context) ([]*vpc.Network, error) {
    keys, err := m.store.List(ctx, "networks/")
    if err != nil {
        return nil, fmt.Errorf("failed to list networks: %w", err)
    }

    networks := make([]*vpc.Network, 0, len(keys))
    for _, key := range keys {
        var network vpc.Network
        if err := m.store.Get(ctx, key, &network); err != nil {
            continue // Skip invalid entries
        }
        networks = append(networks, &network)
    }

    return networks, nil
}

var _ vpc.NetworkManager = (*Manager)(nil)
```

#### 4. Update Network Tests
**File**: `pkg/vpc/network/manager_test.go`
**Changes**: Update tests to use real storage instead of nil

```go
// At the beginning of each test function, add:
store := storage.NewMemoryStore()
manager := network.NewManager(store)
```

#### 5. vpc-cli Network Subcommands
**File**: `cmd/vpc-cli/cmd/network.go` (new)
**Changes**: Add network management subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/network"
    "github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
    Use:   "network",
    Short: "Manage VPC networks",
    Long:  "Create, list, inspect, and delete VPC networks",
}

var networkCreateCmd = &cobra.Command{
    Use:   "create [name] [cidr]",
    Short: "Create a new VPC network",
    Long:  "Create a new VPC network with optional name and CIDR",
    Example: `  vpc-cli network create                    # Create with defaults
  vpc-cli network create my-vpc 10.5.0.0/16 # Create with custom CIDR`,
    RunE: func(cmd *cobra.Command, args []string) error {
        config := vpc.NetworkConfig{
            Name: "test-network",
        }
        if len(args) > 0 {
            config.Name = args[0]
        }
        if len(args) > 1 {
            config.CIDR = args[1]
        }

        manager := network.NewManager(getStore())
        net, err := manager.CreateNetwork(context.Background(), config)
        if err != nil {
            return err
        }

        fmt.Printf("✓ Created network:\n")
        fmt.Printf("  ID:         %s\n", net.ID)
        fmt.Printf("  Name:       %s\n", net.Name)
        fmt.Printf("  CIDR:       %s\n", net.CIDR)
        fmt.Printf("  DNSSuffix:  %s\n", net.DNSSuffix)
        fmt.Printf("  VxlanID:    %d\n", net.VxlanID)
        fmt.Printf("  Driver:     %s\n", net.Driver)
        fmt.Printf("  Status:     %s\n", net.Status)
        return nil
    },
}

var networkListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all VPC networks",
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := network.NewManager(getStore())
        networks, err := manager.ListNetworks(context.Background())
        if err != nil {
            return err
        }

        fmt.Printf("✓ Found %d networks:\n", len(networks))
        for i, net := range networks {
            fmt.Printf("%d. %s (%s) - %s\n", i+1, net.Name, net.CIDR, net.ID)
        }
        return nil
    },
}

var networkGetCmd = &cobra.Command{
    Use:   "get <network-id>",
    Short: "Get VPC network details",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := network.NewManager(getStore())
        net, err := manager.GetNetwork(context.Background(), args[0])
        if err != nil {
            return err
        }

        fmt.Printf("✓ Network details:\n")
        fmt.Printf("  ID:         %s\n", net.ID)
        fmt.Printf("  Name:       %s\n", net.Name)
        fmt.Printf("  CIDR:       %s\n", net.CIDR)
        fmt.Printf("  DNSSuffix:  %s\n", net.DNSSuffix)
        fmt.Printf("  Status:     %s\n", net.Status)
        return nil
    },
}

var networkDeleteCmd = &cobra.Command{
    Use:   "delete <network-id>",
    Short: "Delete a VPC network",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := network.NewManager(getStore())
        if err := manager.DeleteNetwork(context.Background(), args[0]); err != nil {
            return err
        }
        fmt.Println("✓ Network deleted")
        return nil
    },
}

func init() {
    networkCmd.AddCommand(networkCreateCmd)
    networkCmd.AddCommand(networkListCmd)
    networkCmd.AddCommand(networkGetCmd)
    networkCmd.AddCommand(networkDeleteCmd)
    rootCmd.AddCommand(networkCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [x] All network manager tests pass: `cd pkg/vpc && go test -v ./network/`
- [x] No lint errors: `golangci-lint run ./pkg/vpc/network/`
- [x] Storage tests pass: `go test -v ./pkg/vpc/storage/`
- [x] Code compiles: `go build ./pkg/vpc/network/`
- [x] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Create network with defaults: `vpc-cli network create` (verify CIDR=10.0.0.0/16, DNSSuffix=internal)
- [ ] Create network with custom values: `vpc-cli network create my-vpc 10.5.0.0/16` (verify custom CIDR)
- [ ] List networks: `vpc-cli network list` (should show all created networks)
- [ ] Get network details: `vpc-cli network get <network-id>` (use ID from create)
- [ ] Delete network: `vpc-cli network delete <network-id>` (verify deletion)

**Implementation Note**: After completing this phase and all automated verification passes, pause here for confirmation before proceeding to Phase 2.

---

## Phase 2: Hierarchical IPAM Implementation

### Overview
Implement IPAMManager to allocate /24 subnets to hosts and individual IPs within subnets, supporting the hierarchical IPAM model.

### Changes Required:

#### 1. IPAM Manager Implementation
**File**: `pkg/vpc/ipam/manager.go`
**Changes**: Implement hierarchical IP allocation

```go
package ipam

import (
    "context"
    "fmt"
    "net"
    "sync"
    "time"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
)

type Manager struct {
    store     storage.StateStore
    mu        sync.RWMutex
    vpcCIDR   *net.IPNet
    nextSubnet int // Next /24 subnet to allocate (e.g., 1 for 10.0.1.0/24)
}

func NewManager(store storage.StateStore, vpcCIDR string) (*Manager, error) {
    _, cidr, err := net.ParseCIDR(vpcCIDR)
    if err != nil {
        return nil, fmt.Errorf("invalid VPC CIDR: %w", err)
    }

    return &Manager{
        store:      store,
        vpcCIDR:    cidr,
        nextSubnet: 1, // Start from .1.0/24
    }, nil
}

func (m *Manager) AllocateHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Check if host already has a subnet
    key := fmt.Sprintf("ipam/leases/%s", hostID)
    var lease vpc.SubnetLease
    if err := m.store.Get(ctx, key, &lease); err == nil {
        return lease.Subnet, nil // Already allocated
    }

    // Allocate new /24 subnet
    ip := m.vpcCIDR.IP
    subnet := &net.IPNet{
        IP:   net.IPv4(ip[0], ip[1], byte(m.nextSubnet), 0),
        Mask: net.CIDRMask(24, 32),
    }

    // Create lease
    lease = vpc.SubnetLease{
        HostID:    hostID,
        Subnet:    subnet,
        LeaseTime: time.Now(),
        ExpiresAt: time.Now().Add(24 * time.Hour),
    }

    // Save lease
    if err := m.store.Save(ctx, key, &lease); err != nil {
        return nil, fmt.Errorf("failed to save lease: %w", err)
    }

    m.nextSubnet++
    return subnet, nil
}

func (m *Manager) AllocateIP(ctx context.Context, subnet *net.IPNet) (net.IP, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Get allocated IPs for this subnet
    prefix := fmt.Sprintf("ipam/ips/%s", subnet.String())
    allocatedKeys, err := m.store.List(ctx, prefix)
    if err != nil {
        return nil, fmt.Errorf("failed to list allocated IPs: %w", err)
    }

    // Track allocated IPs
    allocated := make(map[string]bool)
    for _, key := range allocatedKeys {
        allocated[key] = true
    }

    // Find next available IP (start from .2, .1 is gateway)
    ip := make(net.IP, len(subnet.IP))
    copy(ip, subnet.IP)
    ip[3] = 2 // Start from .2

    for subnet.Contains(ip) {
        ipKey := fmt.Sprintf("ipam/ips/%s/%s", subnet.String(), ip.String())
        if !allocated[ipKey] {
            // Allocate this IP
            if err := m.store.Save(ctx, ipKey, ip.String()); err != nil {
                return nil, fmt.Errorf("failed to save IP: %w", err)
            }
            return ip, nil
        }

        // Increment IP
        ip[3]++
        if ip[3] == 0 {
            return nil, fmt.Errorf("subnet exhausted")
        }
    }

    return nil, fmt.Errorf("no available IPs in subnet")
}

func (m *Manager) ReleaseIP(ctx context.Context, ip net.IP) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Find which subnet this IP belongs to
    keys, err := m.store.List(ctx, "ipam/ips/")
    if err != nil {
        return fmt.Errorf("failed to list IPs: %w", err)
    }

    // Delete IP allocation
    for _, key := range keys {
        if contains(key, ip.String()) {
            return m.store.Delete(ctx, key)
        }
    }

    return fmt.Errorf("IP not found: %s", ip.String())
}

func (m *Manager) RenewLease(ctx context.Context, hostID string) error {
    m.mu.Lock()
    defer m.mu.Unlock()

    key := fmt.Sprintf("ipam/leases/%s", hostID)
    var lease vpc.SubnetLease
    if err := m.store.Get(ctx, key, &lease); err != nil {
        return fmt.Errorf("lease not found: %w", err)
    }

    // Renew lease
    lease.ExpiresAt = time.Now().Add(24 * time.Hour)
    return m.store.Save(ctx, key, &lease)
}

func (m *Manager) GetHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
    key := fmt.Sprintf("ipam/leases/%s", hostID)
    var lease vpc.SubnetLease
    if err := m.store.Get(ctx, key, &lease); err != nil {
        return nil, fmt.Errorf("lease not found: %w", err)
    }

    return lease.Subnet, nil
}

func contains(s, substr string) bool {
    return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

var _ vpc.IPAMManager = (*Manager)(nil)
```

#### 2. Update IPAM Tests
**File**: `pkg/vpc/ipam/manager_test.go`
**Changes**: Initialize manager with storage and VPC CIDR

```go
// At the beginning of each test:
store := storage.NewMemoryStore()
manager, err := ipam.NewManager(store, "10.0.0.0/16")
if err != nil {
    t.Fatalf("failed to create manager: %v", err)
}
```

#### 3. vpc-cli IPAM Subcommands
**File**: `cmd/vpc-cli/cmd/ipam.go` (new)
**Changes**: Add IPAM management subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"
    "net"

    "github.com/fertile/banyan/pkg/vpc/ipam"
    "github.com/spf13/cobra"
)

var ipamCmd = &cobra.Command{
    Use:   "ipam",
    Short: "Manage IP address allocation",
    Long:  "Allocate and manage IP addresses and host subnets",
}

var ipamAllocateSubnetCmd = &cobra.Command{
    Use:   "allocate-subnet <host-id>",
    Short: "Allocate /24 subnet for host",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
        if err != nil {
            return err
        }

        subnet, err := manager.AllocateHostSubnet(context.Background(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("✓ Allocated subnet for %s: %s\n", args[0], subnet.String())
        return nil
    },
}

var ipamAllocateIPCmd = &cobra.Command{
    Use:   "allocate-ip <host-id>",
    Short: "Allocate IP from host's subnet",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
        if err != nil {
            return err
        }

        subnet, err := manager.GetHostSubnet(context.Background(), args[0])
        if err != nil {
            return err
        }

        ip, err := manager.AllocateIP(context.Background(), subnet)
        if err != nil {
            return err
        }
        fmt.Printf("✓ Allocated IP: %s (from subnet %s)\n", ip.String(), subnet.String())
        return nil
    },
}

var ipamReleaseIPCmd = &cobra.Command{
    Use:   "release-ip <ip-address>",
    Short: "Release allocated IP",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        ip := net.ParseIP(args[0])
        if ip == nil {
            return fmt.Errorf("invalid IP address: %s", args[0])
        }

        manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
        if err != nil {
            return err
        }

        if err := manager.ReleaseIP(context.Background(), ip); err != nil {
            return err
        }
        fmt.Printf("✓ Released IP: %s\n", ip.String())
        return nil
    },
}

var ipamRenewLeaseCmd = &cobra.Command{
    Use:   "renew-lease <host-id>",
    Short: "Renew subnet lease for host",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
        if err != nil {
            return err
        }

        if err := manager.RenewLease(context.Background(), args[0]); err != nil {
            return err
        }
        fmt.Printf("✓ Renewed lease for %s\n", args[0])
        return nil
    },
}

var ipamGetSubnetCmd = &cobra.Command{
    Use:   "get-subnet <host-id>",
    Short: "Get host's allocated subnet",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
        if err != nil {
            return err
        }

        subnet, err := manager.GetHostSubnet(context.Background(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("✓ Host %s subnet: %s\n", args[0], subnet.String())
        return nil
    },
}

func init() {
    ipamCmd.AddCommand(ipamAllocateSubnetCmd)
    ipamCmd.AddCommand(ipamAllocateIPCmd)
    ipamCmd.AddCommand(ipamReleaseIPCmd)
    ipamCmd.AddCommand(ipamRenewLeaseCmd)
    ipamCmd.AddCommand(ipamGetSubnetCmd)
    rootCmd.AddCommand(ipamCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [ ] All IPAM tests pass: `go test -v ./pkg/vpc/ipam/`
- [ ] No lint errors: `golangci-lint run ./pkg/vpc/ipam/`
- [ ] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Allocate subnets: `vpc-cli ipam allocate-subnet host-1` (verify 10.0.1.0/24)
- [ ] Allocate subnets: `vpc-cli ipam allocate-subnet host-2` (verify 10.0.2.0/24)
- [ ] Allocate subnets: `vpc-cli ipam allocate-subnet host-3` (verify 10.0.3.0/24)
- [ ] Allocate IP: `vpc-cli ipam allocate-ip host-1` (verify 10.0.1.2)
- [ ] Allocate IP: `vpc-cli ipam allocate-ip host-1` (verify 10.0.1.3)
- [ ] Release IP: `vpc-cli ipam release-ip 10.0.1.2` (verify success)
- [ ] Re-allocate: `vpc-cli ipam allocate-ip host-1` (should get 10.0.1.2 again)
- [ ] Renew lease: `vpc-cli ipam renew-lease host-1` (verify success)

**Implementation Note**: After completing this phase and all automated verification passes, pause here for confirmation before proceeding to Phase 3.

---

## Phase 3: CNI Runtime with Flannel

### Overview
Implement CNIRuntime to attach/detach containers using CNI plugins, starting with Flannel for VXLAN overlay networking.

### Changes Required:

#### 1. CNI Runtime Implementation
**File**: `pkg/vpc/cni/runtime.go`
**Changes**: Implement CNI plugin execution via exec

```go
package cni

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "os"
    "os/exec"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
)

type Runtime struct {
    store         storage.StateStore
    cniConfigPath string
    cniBinPath    string
}

func NewRuntime(store storage.StateStore) *Runtime {
    return &Runtime{
        store:         store,
        cniConfigPath: "/etc/cni/net.d",
        cniBinPath:    "/opt/cni/bin",
    }
}

func (r *Runtime) SetupPlugin(ctx context.Context, plugin string, config []byte) error {
    // Write CNI config to file
    configFile := fmt.Sprintf("%s/10-%s.conf", r.cniConfigPath, plugin)

    if err := os.MkdirAll(r.cniConfigPath, 0755); err != nil {
        return fmt.Errorf("failed to create CNI config dir: %w", err)
    }

    if err := os.WriteFile(configFile, config, 0644); err != nil {
        return fmt.Errorf("failed to write CNI config: %w", err)
    }

    // Save plugin status
    status := &vpc.PluginStatus{
        Name:    plugin,
        Version: "1.0.0",
        Status:  "active",
    }

    key := fmt.Sprintf("cni/plugins/%s", plugin)
    return r.store.Save(ctx, key, status)
}

func (r *Runtime) AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error {
    // Create CNI input
    cniInput := map[string]interface{}{
        "cniVersion": "0.4.0",
        "name":       networkID,
        "type":       "flannel",
        "ipam": map[string]interface{}{
            "type": "host-local",
            "ranges": [][]map[string]string{
                {{"subnet": ip.String() + "/24"}},
            },
        },
    }

    inputJSON, err := json.Marshal(cniInput)
    if err != nil {
        return fmt.Errorf("failed to marshal CNI input: %w", err)
    }

    // Execute CNI ADD command
    cmd := exec.CommandContext(ctx, fmt.Sprintf("%s/flannel", r.cniBinPath))
    cmd.Env = append(os.Environ(),
        "CNI_COMMAND=ADD",
        "CNI_CONTAINERID="+containerID,
        "CNI_NETNS=/var/run/netns/"+containerID,
        "CNI_IFNAME=eth0",
        "CNI_PATH="+r.cniBinPath,
    )
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("CNI ADD failed: %w", err)
    }

    // Save container info
    container := &vpc.Container{
        ID:        containerID,
        NetworkID: networkID,
        IP:        ip,
        Status:    "attached",
    }

    key := fmt.Sprintf("cni/containers/%s", containerID)
    return r.store.Save(ctx, key, container)
}

func (r *Runtime) RemoveFromNetwork(ctx context.Context, containerID, networkID string) error {
    // Execute CNI DEL command
    cmd := exec.CommandContext(ctx, fmt.Sprintf("%s/flannel", r.cniBinPath))
    cmd.Env = append(os.Environ(),
        "CNI_COMMAND=DEL",
        "CNI_CONTAINERID="+containerID,
        "CNI_NETNS=/var/run/netns/"+containerID,
        "CNI_IFNAME=eth0",
        "CNI_PATH="+r.cniBinPath,
    )

    if err := cmd.Run(); err != nil {
        return fmt.Errorf("CNI DEL failed: %w", err)
    }

    // Remove container info
    key := fmt.Sprintf("cni/containers/%s", containerID)
    return r.store.Delete(ctx, key)
}

func (r *Runtime) GetPluginStatus(ctx context.Context, plugin string) (*vpc.PluginStatus, error) {
    key := fmt.Sprintf("cni/plugins/%s", plugin)

    var status vpc.PluginStatus
    if err := r.store.Get(ctx, key, &status); err != nil {
        return nil, fmt.Errorf("plugin not found: %w", err)
    }

    return &status, nil
}

var _ vpc.CNIRuntime = (*Runtime)(nil)
```

#### 2. Update CNI Tests
**File**: `pkg/vpc/cni/runtime_test.go`
**Changes**: Mock CNI binary execution or skip tests requiring CNI binaries

```go
// For tests that require actual CNI binaries, add skip condition:
if _, err := os.Stat("/opt/cni/bin/flannel"); os.IsNotExist(err) {
    t.Skip("CNI binaries not installed, skipping integration test")
}

// Initialize runtime with storage
store := storage.NewMemoryStore()
runtime := cni.NewRuntime(store)
```

#### 3. vpc-cli CNI Subcommands
**File**: `cmd/vpc-cli/cmd/cni.go` (new)
**Changes**: Add CNI management subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"
    "net"

    "github.com/fertile/banyan/pkg/vpc/cni"
    "github.com/spf13/cobra"
)

var cniCmd = &cobra.Command{
    Use:   "cni",
    Short: "Manage CNI plugins and containers",
    Long:  "Setup CNI plugins and attach/detach containers from networks",
}

var cniSetupPluginCmd = &cobra.Command{
    Use:   "setup-plugin <plugin-name>",
    Short: "Setup CNI plugin (flannel)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        config := []byte(`{
            "name": "banyan-network",
            "type": "flannel",
            "delegate": {
                "bridge": "banyan0",
                "isDefaultGateway": true
            }
        }`)

        runtime := cni.NewRuntime(getStore())
        if err := runtime.SetupPlugin(context.Background(), args[0], config); err != nil {
            return err
        }
        fmt.Printf("✓ Plugin %s configured\n", args[0])
        return nil
    },
}

var cniAddContainerCmd = &cobra.Command{
    Use:   "add-container <container-id> <network-id> <ip>",
    Short: "Attach container to network",
    Args:  cobra.ExactArgs(3),
    RunE: func(cmd *cobra.Command, args []string) error {
        ip := net.ParseIP(args[2])
        if ip == nil {
            return fmt.Errorf("invalid IP address: %s", args[2])
        }

        runtime := cni.NewRuntime(getStore())
        if err := runtime.AddToNetwork(context.Background(), args[0], args[1], ip); err != nil {
            return err
        }
        fmt.Printf("✓ Container %s attached to network %s with IP %s\n", args[0], args[1], ip)
        return nil
    },
}

var cniRemoveContainerCmd = &cobra.Command{
    Use:   "remove-container <container-id> <network-id>",
    Short: "Detach container from network",
    Args:  cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        runtime := cni.NewRuntime(getStore())
        if err := runtime.RemoveFromNetwork(context.Background(), args[0], args[1]); err != nil {
            return err
        }
        fmt.Println("✓ Container detached from network")
        return nil
    },
}

var cniGetStatusCmd = &cobra.Command{
    Use:   "get-status <plugin-name>",
    Short: "Get CNI plugin status",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        runtime := cni.NewRuntime(getStore())
        status, err := runtime.GetPluginStatus(context.Background(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("✓ Plugin: %s\n", status.Name)
        fmt.Printf("  Version: %s\n", status.Version)
        fmt.Printf("  Status: %s\n", status.Status)
        return nil
    },
}

func init() {
    cniCmd.AddCommand(cniSetupPluginCmd)
    cniCmd.AddCommand(cniAddContainerCmd)
    cniCmd.AddCommand(cniRemoveContainerCmd)
    cniCmd.AddCommand(cniGetStatusCmd)
    rootCmd.AddCommand(cniCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [ ] CNI runtime tests pass (may skip some): `go test -v ./pkg/vpc/cni/`
- [ ] No lint errors: `golangci-lint run ./pkg/vpc/cni/`
- [ ] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Setup plugin: `sudo vpc-cli cni setup-plugin flannel` (check /etc/cni/net.d/10-flannel.conf exists)
- [ ] Get status: `vpc-cli cni get-status flannel` (verify active)
- [ ] Attach container: `sudo vpc-cli cni add-container test-1 net-1 10.0.1.5` (requires CNI binaries)
- [ ] Detach container: `sudo vpc-cli cni remove-container test-1 net-1`

**Implementation Note**: After completing this phase and all automated verification passes, pause here for confirmation before proceeding to Phase 4.

---

## Phase 4: Security Rules & iptables

### Overview
Implement SecurityManager to translate high-level security rules (service:name, cidr:x.x.x.x, internet) into iptables commands.

### Changes Required:

#### 1. Security Manager Implementation
**File**: `pkg/vpc/security/manager.go`
**Changes**: Implement iptables rule translation

```go
package security

import (
    "context"
    "fmt"
    "os/exec"
    "strings"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
    "github.com/google/uuid"
)

type Manager struct {
    store storage.StateStore
}

func NewManager(store storage.StateStore) *Manager {
    return &Manager{
        store: store,
    }
}

func (m *Manager) AddRule(ctx context.Context, rule *vpc.SecurityRule) error {
    // Generate rule ID if not provided
    if rule.ID == "" {
        rule.ID = uuid.New().String()
    }

    // Save rule
    key := fmt.Sprintf("security/rules/%s/%s", rule.NetworkID, rule.ID)
    if err := m.store.Save(ctx, key, rule); err != nil {
        return fmt.Errorf("failed to save rule: %w", err)
    }

    return nil
}

func (m *Manager) RemoveRule(ctx context.Context, ruleID string) error {
    // Find rule by scanning all networks
    keys, err := m.store.List(ctx, "security/rules/")
    if err != nil {
        return fmt.Errorf("failed to list rules: %w", err)
    }

    for _, key := range keys {
        if strings.Contains(key, ruleID) {
            return m.store.Delete(ctx, key)
        }
    }

    return fmt.Errorf("rule not found: %s", ruleID)
}

func (m *Manager) ListRules(ctx context.Context, networkID string) ([]*vpc.SecurityRule, error) {
    prefix := fmt.Sprintf("security/rules/%s/", networkID)
    keys, err := m.store.List(ctx, prefix)
    if err != nil {
        return nil, fmt.Errorf("failed to list rules: %w", err)
    }

    rules := make([]*vpc.SecurityRule, 0, len(keys))
    for _, key := range keys {
        var rule vpc.SecurityRule
        if err := m.store.Get(ctx, key, &rule); err != nil {
            continue
        }
        rules = append(rules, &rule)
    }

    return rules, nil
}

func (m *Manager) ApplyRules(ctx context.Context, networkID string) error {
    // Get all rules for network
    rules, err := m.ListRules(ctx, networkID)
    if err != nil {
        return fmt.Errorf("failed to list rules: %w", err)
    }

    // Create BANYAN chains if they don't exist
    chains := []string{"BANYAN-INPUT", "BANYAN-FORWARD", "BANYAN-OUTPUT"}
    for _, chain := range chains {
        cmd := exec.CommandContext(ctx, "iptables", "-N", chain)
        cmd.Run() // Ignore errors if chain exists
    }

    // Flush existing BANYAN rules
    for _, chain := range chains {
        cmd := exec.CommandContext(ctx, "iptables", "-F", chain)
        if err := cmd.Run(); err != nil {
            return fmt.Errorf("failed to flush chain %s: %w", chain, err)
        }
    }

    // Apply deny-by-default
    for _, chain := range chains {
        cmd := exec.CommandContext(ctx, "iptables", "-A", chain, "-j", "DROP")
        if err := cmd.Run(); err != nil {
            return fmt.Errorf("failed to set default policy: %w", err)
        }
    }

    // Translate and apply rules
    for _, rule := range rules {
        if err := m.applyRule(ctx, rule); err != nil {
            return fmt.Errorf("failed to apply rule %s: %w", rule.ID, err)
        }
    }

    return nil
}

func (m *Manager) applyRule(ctx context.Context, rule *vpc.SecurityRule) error {
    // Determine chain based on direction
    chain := "BANYAN-INPUT"
    if rule.Direction == "egress" {
        chain = "BANYAN-OUTPUT"
    }

    // Parse source and destination
    var src, dst string

    // Parse "from" field
    if strings.HasPrefix(rule.From, "service:") {
        src = "10.0.0.0/16" // TODO: Resolve service to actual IPs
    } else if strings.HasPrefix(rule.From, "cidr:") {
        src = strings.TrimPrefix(rule.From, "cidr:")
    } else if rule.From == "internet" {
        src = "0.0.0.0/0"
    }

    // Parse "to" field
    if strings.HasPrefix(rule.To, "service:") {
        dst = "10.0.0.0/16" // TODO: Resolve service to actual IPs
    } else if strings.HasPrefix(rule.To, "cidr:") {
        dst = strings.TrimPrefix(rule.To, "cidr:")
    } else if rule.To == "internet" {
        dst = "0.0.0.0/0"
    }

    // Build iptables command
    args := []string{"-A", chain}

    if src != "" {
        args = append(args, "-s", src)
    }
    if dst != "" {
        args = append(args, "-d", dst)
    }
    if rule.Protocol != "" {
        args = append(args, "-p", rule.Protocol)
    }
    if rule.ToPort != "" {
        args = append(args, "--dport", rule.ToPort)
    }

    // Apply action
    action := "DROP"
    if rule.Action == "allow" {
        action = "ACCEPT"
    }
    args = append(args, "-j", action)

    // Execute iptables command
    cmd := exec.CommandContext(ctx, "iptables", args...)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("iptables command failed: %w", err)
    }

    return nil
}

var _ vpc.SecurityManager = (*Manager)(nil)
```

#### 2. Update Security Tests
**File**: `pkg/vpc/security/manager_test.go`
**Changes**: Mock or skip iptables execution

```go
// For tests requiring iptables, check if running as root
if os.Geteuid() != 0 {
    t.Skip("iptables tests require root, skipping")
}

// Initialize manager
store := storage.NewMemoryStore()
manager := security.NewManager(store)
```

#### 3. vpc-cli Security Subcommands
**File**: `cmd/vpc-cli/cmd/security.go` (new)
**Changes**: Add security rule management subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/security"
    "github.com/spf13/cobra"
)

var securityCmd = &cobra.Command{
    Use:   "security",
    Short: "Manage security rules",
    Long:  "Add, list, and apply security rules for networks",
}

var securityAddRuleCmd = &cobra.Command{
    Use:   "add-rule",
    Short: "Add security rule",
    Example: `  vpc-cli security add-rule --from internet --to 10.0.1.5 --port 443 --action allow
  vpc-cli security add-rule --from 10.0.2.0/24 --to 10.0.1.5 --port 22 --action deny`,
    RunE: func(cmd *cobra.Command, args []string) error {
        from, _ := cmd.Flags().GetString("from")
        to, _ := cmd.Flags().GetString("to")
        port, _ := cmd.Flags().GetString("port")
        action, _ := cmd.Flags().GetString("action")
        protocol, _ := cmd.Flags().GetString("protocol")

        rule := &vpc.SecurityRule{
            NetworkID: "test-network",
            Direction: "ingress",
            Action:    action,
            From:      from,
            To:        to,
            ToPort:    port,
            Protocol:  protocol,
        }

        manager := security.NewManager(getStore())
        if err := manager.AddRule(context.Background(), rule); err != nil {
            return err
        }
        fmt.Printf("✓ Rule added: %s -> %s:%s (%s)\n", rule.From, rule.To, rule.ToPort, rule.Action)
        return nil
    },
}

var securityListRulesCmd = &cobra.Command{
    Use:   "list-rules <network-id>",
    Short: "List security rules for network",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := security.NewManager(getStore())
        rules, err := manager.ListRules(context.Background(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("✓ Found %d rules:\n", len(rules))
        for i, rule := range rules {
            fmt.Printf("%d. %s -> %s:%s (%s)\n", i+1, rule.From, rule.To, rule.ToPort, rule.Action)
        }
        return nil
    },
}

var securityApplyRulesCmd = &cobra.Command{
    Use:   "apply-rules <network-id>",
    Short: "Apply security rules to iptables",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := security.NewManager(getStore())
        if err := manager.ApplyRules(context.Background(), args[0]); err != nil {
            return err
        }
        fmt.Println("✓ Rules applied to iptables")
        return nil
    },
}

func init() {
    securityAddRuleCmd.Flags().String("from", "", "Source (service:name, cidr:x.x.x.x/y, internet)")
    securityAddRuleCmd.Flags().String("to", "", "Destination")
    securityAddRuleCmd.Flags().String("port", "", "Port or range")
    securityAddRuleCmd.Flags().String("action", "allow", "allow or deny")
    securityAddRuleCmd.Flags().String("protocol", "tcp", "Protocol")

    securityCmd.AddCommand(securityAddRuleCmd)
    securityCmd.AddCommand(securityListRulesCmd)
    securityCmd.AddCommand(securityApplyRulesCmd)
    rootCmd.AddCommand(securityCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [ ] Security manager tests pass (may skip iptables): `go test -v ./pkg/vpc/security/`
- [ ] No lint errors: `golangci-lint run ./pkg/vpc/security/`
- [ ] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Add allow rule: `sudo vpc-cli security add-rule --from internet --to 10.0.1.5 --port 443 --action allow`
- [ ] Add deny rule: `sudo vpc-cli security add-rule --from 10.0.2.0/24 --to 10.0.1.5 --port 22 --action deny`
- [ ] List rules: `vpc-cli security list-rules test-network`
- [ ] Apply rules: `sudo vpc-cli security apply-rules test-network`
- [ ] Verify chains: `sudo iptables -L BANYAN-INPUT -n -v` (should show rules)
- [ ] Service resolution works (when implemented)

**Implementation Note**: After completing this phase and all automated verification passes, pause here for confirmation before proceeding to Phase 5.

---

## Phase 5: DNS Service Discovery

### Overview
Implement DNSManager for service discovery using /etc/hosts file manipulation (simple approach), with health-aware DNS resolution.

### Changes Required:

#### 1. DNS Manager Implementation
**File**: `pkg/vpc/dns/manager.go`
**Changes**: Implement /etc/hosts-based DNS

```go
package dns

import (
    "context"
    "fmt"
    "net"
    "os"
    "strings"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
)

type Manager struct {
    store     storage.StateStore
    hostsFile string
}

func NewManager(store storage.StateStore) *Manager {
    return &Manager{
        store:     store,
        hostsFile: "/etc/hosts",
    }
}

func (m *Manager) RegisterHost(ctx context.Context, hostname string, ip net.IP) error {
    // Save to store with health=true by default
    key := fmt.Sprintf("dns/hosts/%s/%s", hostname, ip.String())
    entry := map[string]interface{}{
        "hostname": hostname,
        "ip":       ip.String(),
        "healthy":  true,
    }

    if err := m.store.Save(ctx, key, entry); err != nil {
        return fmt.Errorf("failed to save DNS entry: %w", err)
    }

    // Update /etc/hosts
    return m.updateHostsFile(ctx)
}

func (m *Manager) UnregisterHost(ctx context.Context, hostname string) error {
    // Delete all IPs for this hostname
    prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
    keys, err := m.store.List(ctx, prefix)
    if err != nil {
        return fmt.Errorf("failed to list DNS entries: %w", err)
    }

    for _, key := range keys {
        if err := m.store.Delete(ctx, key); err != nil {
            return fmt.Errorf("failed to delete DNS entry: %w", err)
        }
    }

    // Update /etc/hosts
    return m.updateHostsFile(ctx)
}

func (m *Manager) LookupHost(ctx context.Context, hostname string) ([]net.IP, error) {
    // Get all IPs for hostname
    prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
    keys, err := m.store.List(ctx, prefix)
    if err != nil {
        return nil, fmt.Errorf("failed to lookup host: %w", err)
    }

    if len(keys) == 0 {
        return nil, fmt.Errorf("host not found: %s", hostname)
    }

    var ips []net.IP
    for _, key := range keys {
        var entry map[string]interface{}
        if err := m.store.Get(ctx, key, &entry); err != nil {
            continue
        }

        // Only return healthy hosts
        if healthy, ok := entry["healthy"].(bool); ok && healthy {
            if ipStr, ok := entry["ip"].(string); ok {
                ips = append(ips, net.ParseIP(ipStr))
            }
        }
    }

    if len(ips) == 0 {
        return nil, fmt.Errorf("no healthy hosts for: %s", hostname)
    }

    return ips, nil
}

func (m *Manager) UpdateHealth(ctx context.Context, hostname string, healthy bool) error {
    // Update all IPs for this hostname
    prefix := fmt.Sprintf("dns/hosts/%s/", hostname)
    keys, err := m.store.List(ctx, prefix)
    if err != nil {
        return fmt.Errorf("failed to list DNS entries: %w", err)
    }

    for _, key := range keys {
        var entry map[string]interface{}
        if err := m.store.Get(ctx, key, &entry); err != nil {
            continue
        }

        entry["healthy"] = healthy
        if err := m.store.Save(ctx, key, entry); err != nil {
            return fmt.Errorf("failed to update health: %w", err)
        }
    }

    return nil
}

func (m *Manager) updateHostsFile(ctx context.Context) error {
    // Read current /etc/hosts
    content, err := os.ReadFile(m.hostsFile)
    if err != nil {
        return fmt.Errorf("failed to read hosts file: %w", err)
    }

    lines := strings.Split(string(content), "\n")

    // Remove BANYAN-managed entries
    var filtered []string
    for _, line := range lines {
        if !strings.Contains(line, "# BANYAN") {
            filtered = append(filtered, line)
        }
    }

    // Get all DNS entries from store
    keys, err := m.store.List(ctx, "dns/hosts/")
    if err != nil {
        return fmt.Errorf("failed to list DNS entries: %w", err)
    }

    // Add BANYAN entries
    for _, key := range keys {
        var entry map[string]interface{}
        if err := m.store.Get(ctx, key, &entry); err != nil {
            continue
        }

        // Only add healthy hosts
        if healthy, ok := entry["healthy"].(bool); ok && healthy {
            hostname := entry["hostname"].(string)
            ip := entry["ip"].(string)
            filtered = append(filtered, fmt.Sprintf("%s %s # BANYAN", ip, hostname))
        }
    }

    // Write back to /etc/hosts
    newContent := strings.Join(filtered, "\n")
    return os.WriteFile(m.hostsFile, []byte(newContent), 0644)
}

var _ vpc.DNSManager = (*Manager)(nil)
```

#### 2. Update DNS Tests
**File**: `pkg/vpc/dns/manager_test.go`
**Changes**: Use temporary hosts file for testing

```go
// Create temporary hosts file for testing
tmpFile, err := os.CreateTemp("", "hosts-*")
if err != nil {
    t.Fatalf("failed to create temp file: %v", err)
}
defer os.Remove(tmpFile.Name())

// Initialize manager with temp file
store := storage.NewMemoryStore()
manager := dns.NewManager(store)
manager.hostsFile = tmpFile.Name() // Override for testing
```

#### 3. vpc-cli DNS Subcommands
**File**: `cmd/vpc-cli/cmd/dns.go` (new)
**Changes**: Add DNS management subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"
    "net"
    "strconv"

    "github.com/fertile/banyan/pkg/vpc/dns"
    "github.com/spf13/cobra"
)

var dnsCmd = &cobra.Command{
    Use:   "dns",
    Short: "Manage DNS records",
    Long:  "Register, lookup, and manage DNS records for service discovery",
}

var dnsRegisterCmd = &cobra.Command{
    Use:   "register <hostname> <ip>",
    Short: "Register hostname with IP",
    Args:  cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        ip := net.ParseIP(args[1])
        if ip == nil {
            return fmt.Errorf("invalid IP address: %s", args[1])
        }

        manager := dns.NewManager(getStore())
        if err := manager.RegisterHost(context.Background(), args[0], ip); err != nil {
            return err
        }
        fmt.Printf("✓ Registered %s -> %s\n", args[0], ip)
        return nil
    },
}

var dnsLookupCmd = &cobra.Command{
    Use:   "lookup <hostname>",
    Short: "Lookup hostname",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := dns.NewManager(getStore())
        ips, err := manager.LookupHost(context.Background(), args[0])
        if err != nil {
            return err
        }
        fmt.Printf("✓ %s resolves to:\n", args[0])
        for _, ip := range ips {
            fmt.Printf("  - %s\n", ip)
        }
        return nil
    },
}

var dnsUpdateHealthCmd = &cobra.Command{
    Use:   "update-health <hostname> <healthy>",
    Short: "Update host health status (true/false)",
    Args:  cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        healthy, err := strconv.ParseBool(args[1])
        if err != nil {
            return fmt.Errorf("invalid healthy value: %s (use true/false)", args[1])
        }

        manager := dns.NewManager(getStore())
        if err := manager.UpdateHealth(context.Background(), args[0], healthy); err != nil {
            return err
        }
        fmt.Printf("✓ Updated %s health: %v\n", args[0], healthy)
        return nil
    },
}

var dnsUnregisterCmd = &cobra.Command{
    Use:   "unregister <hostname>",
    Short: "Unregister hostname",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := dns.NewManager(getStore())
        if err := manager.UnregisterHost(context.Background(), args[0]); err != nil {
            return err
        }
        fmt.Printf("✓ Unregistered %s\n", args[0])
        return nil
    },
}

func init() {
    dnsCmd.AddCommand(dnsRegisterCmd)
    dnsCmd.AddCommand(dnsLookupCmd)
    dnsCmd.AddCommand(dnsUpdateHealthCmd)
    dnsCmd.AddCommand(dnsUnregisterCmd)
    rootCmd.AddCommand(dnsCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [ ] DNS manager tests pass: `go test -v ./pkg/vpc/dns/`
- [ ] No lint errors: `golangci-lint run ./pkg/vpc/dns/`
- [ ] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Register: `sudo vpc-cli dns register web.internal 10.0.1.5`
- [ ] Register: `sudo vpc-cli dns register api.internal 10.0.2.10`
- [ ] Verify hosts file: `grep "# BANYAN" /etc/hosts` (should show entries)
- [ ] Lookup: `vpc-cli dns lookup web.internal` (should return 10.0.1.5)
- [ ] Mark unhealthy: `sudo vpc-cli dns update-health web.internal false`
- [ ] Lookup again: `vpc-cli dns lookup web.internal` (should fail - host unhealthy)
- [ ] Unregister: `sudo vpc-cli dns unregister web.internal`

**Implementation Note**: After completing this phase and all automated verification passes, pause here for confirmation before proceeding to Phase 6.

---

## Phase 6: Network Debugging Utilities

### Overview
Implement DebugManager for network diagnostics, connection tracing, and iptables inspection.

### Changes Required:

#### 1. Debug Manager Implementation
**File**: `pkg/vpc/debug/manager.go`
**Changes**: Implement network debugging utilities

```go
package debug

import (
    "context"
    "fmt"
    "net"
    "os/exec"
    "strings"

    "github.com/fertile/banyan/pkg/vpc"
    "github.com/fertile/banyan/pkg/vpc/storage"
)

type Manager struct {
    store storage.StateStore
}

func NewManager(store storage.StateStore) *Manager {
    return &Manager{
        store: store,
    }
}

func (m *Manager) TraceConnection(ctx context.Context, fromIP, toIP net.IP, port int) (*vpc.TraceResult, error) {
    result := &vpc.TraceResult{
        FromIP: fromIP,
        ToIP:   toIP,
        Port:   port,
        Status: "unknown",
    }

    // Step 1: Check if source IP is reachable
    if err := m.ping(ctx, fromIP); err != nil {
        result.Status = "error"
        result.Error = fmt.Sprintf("source unreachable: %v", err)
        return result, nil
    }

    result.Hops = append(result.Hops, vpc.TraceHop{
        Type:    "container",
        Address: fromIP.String(),
    })

    // Step 2: Check gateway
    gateway := net.IPv4(fromIP[0], fromIP[1], fromIP[2], 1)
    if err := m.ping(ctx, gateway); err != nil {
        result.Status = "error"
        result.Error = fmt.Sprintf("gateway unreachable: %v", err)
        return result, nil
    }

    result.Hops = append(result.Hops, vpc.TraceHop{
        Type:    "gateway",
        Address: gateway.String(),
    })

    // Step 3: Check if destination is reachable
    if err := m.ping(ctx, toIP); err != nil {
        result.Status = "blocked"
        result.Error = fmt.Sprintf("destination unreachable: %v", err)
        result.Reachable = false

        // Check iptables for blocking rule
        blockingRule, err := m.findBlockingRule(ctx, fromIP, toIP, port)
        if err == nil && blockingRule != "" {
            result.BlockedBy = blockingRule
        }

        return result, nil
    }

    result.Hops = append(result.Hops, vpc.TraceHop{
        Type:    "container",
        Address: toIP.String(),
    })

    // Step 4: Check port connectivity
    if err := m.checkPort(ctx, toIP, port); err != nil {
        result.Status = "blocked"
        result.Error = fmt.Sprintf("port blocked: %v", err)
        result.Reachable = false

        blockingRule, err := m.findBlockingRule(ctx, fromIP, toIP, port)
        if err == nil && blockingRule != "" {
            result.BlockedBy = blockingRule
        }

        return result, nil
    }

    result.Status = "allowed"
    result.Reachable = true
    return result, nil
}

func (m *Manager) CheckConnectivity(ctx context.Context, containerID string) (*vpc.ConnectivityResult, error) {
    result := &vpc.ConnectivityResult{
        ContainerID: containerID,
        Status:      "unknown",
    }

    // Get container info
    key := fmt.Sprintf("cni/containers/%s", containerID)
    var container vpc.Container
    if err := m.store.Get(ctx, key, &container); err != nil {
        result.Status = "error"
        result.Error = fmt.Sprintf("container not found: %v", err)
        return result, nil
    }

    result.IP = container.IP
    result.HasNetwork = true

    // Check gateway
    gateway := net.IPv4(container.IP[0], container.IP[1], container.IP[2], 1)
    result.Gateway = gateway

    if err := m.ping(ctx, gateway); err != nil {
        result.Status = "error"
        result.InternalPing = false
        result.Errors = append(result.Errors, fmt.Sprintf("gateway unreachable: %v", err))
        return result, nil
    }

    result.InternalPing = true

    // Check external connectivity
    externalIP := net.ParseIP("8.8.8.8")
    if err := m.ping(ctx, externalIP); err != nil {
        result.ExternalPing = false
        result.InternetAccess = false
        result.Status = "degraded"
    } else {
        result.ExternalPing = true
        result.InternetAccess = true
        result.Status = "ok"
    }

    return result, nil
}

func (m *Manager) GetIPTablesRules(ctx context.Context, ip net.IP) ([]string, error) {
    // Get all iptables rules
    cmd := exec.CommandContext(ctx, "iptables", "-L", "-n", "-v")
    output, err := cmd.CombinedOutput()
    if err != nil {
        return nil, fmt.Errorf("failed to list iptables rules: %w", err)
    }

    // Filter rules related to this IP
    var filtered []string
    lines := strings.Split(string(output), "\n")
    for _, line := range lines {
        if strings.Contains(line, ip.String()) {
            filtered = append(filtered, line)
        }
    }

    return filtered, nil
}

func (m *Manager) ping(ctx context.Context, ip net.IP) error {
    cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip.String())
    return cmd.Run()
}

func (m *Manager) checkPort(ctx context.Context, ip net.IP, port int) error {
    // Use netcat or telnet to check port
    cmd := exec.CommandContext(ctx, "nc", "-zv", ip.String(), fmt.Sprintf("%d", port))
    return cmd.Run()
}

func (m *Manager) findBlockingRule(ctx context.Context, fromIP, toIP net.IP, port int) (string, error) {
    rules, err := m.GetIPTablesRules(ctx, toIP)
    if err != nil {
        return "", err
    }

    for _, rule := range rules {
        if strings.Contains(rule, "DROP") || strings.Contains(rule, "REJECT") {
            if strings.Contains(rule, fromIP.String()) || strings.Contains(rule, toIP.String()) {
                return rule, nil
            }
        }
    }

    return "", nil
}

var _ vpc.DebugManager = (*Manager)(nil)
```

#### 2. Update Debug Tests
**File**: `pkg/vpc/debug/manager_test.go`
**Changes**: Mock external commands or skip integration tests

```go
// Skip tests requiring network access
if os.Getenv("SKIP_NETWORK_TESTS") != "" {
    t.Skip("network tests disabled")
}

// Initialize manager
store := storage.NewMemoryStore()
manager := debug.NewManager(store)
```

#### 3. vpc-cli Debug Subcommands
**File**: `cmd/vpc-cli/cmd/debug.go` (new)
**Changes**: Add network debugging subcommands to vpc-cli

```go
package cmd

import (
    "context"
    "fmt"
    "net"
    "strconv"

    "github.com/fertile/banyan/pkg/vpc/debug"
    "github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
    Use:   "debug",
    Short: "Network debugging utilities",
    Long:  "Trace connections, check connectivity, and inspect iptables rules",
}

var debugTraceCmd = &cobra.Command{
    Use:   "trace <from-ip> <to-ip> <port>",
    Short: "Trace connection path",
    Args:  cobra.ExactArgs(3),
    RunE: func(cmd *cobra.Command, args []string) error {
        fromIP := net.ParseIP(args[0])
        if fromIP == nil {
            return fmt.Errorf("invalid from IP: %s", args[0])
        }
        toIP := net.ParseIP(args[1])
        if toIP == nil {
            return fmt.Errorf("invalid to IP: %s", args[1])
        }
        port, err := strconv.Atoi(args[2])
        if err != nil {
            return fmt.Errorf("invalid port: %s", args[2])
        }

        manager := debug.NewManager(getStore())
        result, err := manager.TraceConnection(context.Background(), fromIP, toIP, port)
        if err != nil {
            return err
        }

        fmt.Printf("✓ Trace %s -> %s:%d\n", fromIP, toIP, port)
        fmt.Printf("  Status: %s\n", result.Status)
        fmt.Printf("  Reachable: %v\n", result.Reachable)
        fmt.Printf("  Hops:\n")
        for i, hop := range result.Hops {
            fmt.Printf("    %d. [%s] %s\n", i+1, hop.Type, hop.Address)
        }
        if result.BlockedBy != "" {
            fmt.Printf("  Blocked by: %s\n", result.BlockedBy)
        }
        return nil
    },
}

var debugCheckConnectivityCmd = &cobra.Command{
    Use:   "check-connectivity <container-id>",
    Short: "Check container connectivity",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        manager := debug.NewManager(getStore())
        result, err := manager.CheckConnectivity(context.Background(), args[0])
        if err != nil {
            return err
        }

        fmt.Printf("✓ Connectivity check for %s\n", result.ContainerID)
        fmt.Printf("  Status: %s\n", result.Status)
        fmt.Printf("  IP: %s\n", result.IP)
        fmt.Printf("  Gateway: %s\n", result.Gateway)
        fmt.Printf("  Internal ping: %v\n", result.InternalPing)
        fmt.Printf("  External ping: %v\n", result.ExternalPing)
        fmt.Printf("  Internet access: %v\n", result.InternetAccess)
        return nil
    },
}

var debugGetIPTablesCmd = &cobra.Command{
    Use:   "get-iptables <ip>",
    Short: "Get iptables rules for IP",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        ip := net.ParseIP(args[0])
        if ip == nil {
            return fmt.Errorf("invalid IP address: %s", args[0])
        }

        manager := debug.NewManager(getStore())
        rules, err := manager.GetIPTablesRules(context.Background(), ip)
        if err != nil {
            return err
        }

        fmt.Printf("✓ iptables rules for %s:\n", ip)
        for _, rule := range rules {
            fmt.Println(rule)
        }
        return nil
    },
}

func init() {
    debugCmd.AddCommand(debugTraceCmd)
    debugCmd.AddCommand(debugCheckConnectivityCmd)
    debugCmd.AddCommand(debugGetIPTablesCmd)
    rootCmd.AddCommand(debugCmd)
}
```

### Success Criteria:

#### Automated Verification:
- [ ] Debug manager tests pass (may skip some): `go test -v ./pkg/vpc/debug/`
- [ ] No lint errors: `golangci-lint run ./pkg/vpc/debug/`
- [ ] vpc-cli builds: `go build -o /tmp/vpc-cli ./cmd/vpc-cli/`

#### Manual Verification (with vpc-cli):
- [ ] Trace connection: `sudo vpc-cli debug trace 10.0.1.5 10.0.2.10 8080`
- [ ] Check connectivity: `sudo vpc-cli debug check-connectivity <container-id>`
- [ ] Get iptables: `sudo vpc-cli debug get-iptables 10.0.1.5`

**Implementation Note**: After completing this phase and all automated verification passes, proceed to Phase 7 for integration testing.

---

## Phase 7: Integration Test Scripts with Real Containers

### Overview
Create automated integration test scripts that use real Docker containers to verify end-to-end VPC functionality. These scripts provide concrete, runnable tests instead of vague manual verification steps.

### Changes Required:

#### 1. Test Infrastructure Setup Script
**File**: `test/vpc/setup.sh` (new)
**Changes**: Create test environment with Docker containers

```bash
#!/bin/bash
# Setup integration test environment for VPC

set -e

echo "Setting up VPC integration test environment..."

# Create test directory
mkdir -p /tmp/vpc-test

# Create Docker network for testing (we'll replace with VPC later)
docker network create vpc-test-bridge || true

# Pull test images
echo "Pulling test images..."
docker pull nginx:alpine
docker pull postgres:13-alpine
docker pull redis:alpine

# Create test containers (without network initially)
echo "Creating test containers..."
docker run -d --name vpc-web --network none nginx:alpine
docker run -d --name vpc-api --network none nginx:alpine
docker run -d --name vpc-db --network none postgres:13-alpine
docker run -d --name vpc-cache --network none redis:alpine

echo "Test containers created:"
docker ps --filter "name=vpc-"

echo ""
echo "Setup complete! Containers are ready for VPC attachment."
echo "Run ./test/vpc/test-*.sh to execute integration tests."
```

#### 2. Cleanup Script
**File**: `test/vpc/cleanup.sh` (new)
**Changes**: Clean up test environment

```bash
#!/bin/bash
# Cleanup VPC integration test environment

echo "Cleaning up VPC test environment..."

# Stop and remove test containers
docker stop vpc-web vpc-api vpc-db vpc-cache 2>/dev/null || true
docker rm vpc-web vpc-api vpc-db vpc-cache 2>/dev/null || true

# Remove test network
docker network rm vpc-test-bridge 2>/dev/null || true

# Clean iptables rules
sudo iptables -F BANYAN-INPUT 2>/dev/null || true
sudo iptables -F BANYAN-FORWARD 2>/dev/null || true
sudo iptables -F BANYAN-OUTPUT 2>/dev/null || true
sudo iptables -X BANYAN-INPUT 2>/dev/null || true
sudo iptables -X BANYAN-FORWARD 2>/dev/null || true
sudo iptables -X BANYAN-OUTPUT 2>/dev/null || true

# Clean /etc/hosts
sudo sed -i '/# BANYAN/d' /etc/hosts

echo "Cleanup complete!"
```

#### 3. Network Manager Integration Test
**File**: `test/vpc/test-network.sh` (new)
**Changes**: Test network creation and management

```bash
#!/bin/bash
# Integration test for NetworkManager

set -e

echo "=== Testing NetworkManager ==="

# Build test binary
cd pkg/vpc
go build -o /tmp/vpc-test-network ./test/integration/network_test.go

# Run tests
echo "1. Testing network creation..."
/tmp/vpc-test-network create

echo "2. Testing network listing..."
/tmp/vpc-test-network list

echo "3. Testing network retrieval..."
/tmp/vpc-test-network get

echo "4. Testing network deletion..."
/tmp/vpc-test-network delete

echo "✓ NetworkManager tests passed!"
```

**File**: `pkg/vpc/test/integration/network_test.go` (new)
**Changes**: Integration test binary for network operations

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/fertile/banyan/pkg/vpc/network"
    "github.com/fertile/banyan/pkg/vpc/storage"
    "github.com/fertile/banyan/pkg/vpc"
)

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Usage: network_test <create|list|get|delete>")
        os.Exit(1)
    }

    store := storage.NewMemoryStore()
    manager := network.NewManager(store)
    ctx := context.Background()

    switch os.Args[1] {
    case "create":
        config := vpc.NetworkConfig{
            Name: "test-network",
        }
        net, err := manager.CreateNetwork(ctx, config)
        if err != nil {
            fmt.Printf("ERROR: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("Created network: ID=%s, CIDR=%s, DNSSuffix=%s\n",
            net.ID, net.CIDR, net.DNSSuffix)

    case "list":
        networks, err := manager.ListNetworks(ctx)
        if err != nil {
            fmt.Printf("ERROR: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("Found %d networks\n", len(networks))

    case "get":
        // Assume network ID passed as arg
        net, err := manager.GetNetwork(ctx, os.Args[2])
        if err != nil {
            fmt.Printf("ERROR: %v\n", err)
            os.Exit(1)
        }
        fmt.Printf("Network: %s (%s)\n", net.Name, net.CIDR)

    case "delete":
        err := manager.DeleteNetwork(ctx, os.Args[2])
        if err != nil {
            fmt.Printf("ERROR: %v\n", err)
            os.Exit(1)
        }
        fmt.Println("Network deleted")
    }
}
```

#### 4. IPAM Integration Test
**File**: `test/vpc/test-ipam.sh` (new)
**Changes**: Test hierarchical IP allocation

```bash
#!/bin/bash
# Integration test for IPAM

set -e

echo "=== Testing IPAMManager ==="

cd pkg/vpc

echo "1. Allocating subnets for 3 hosts..."
go run ./test/integration/ipam_test.go allocate-subnet host-1
go run ./test/integration/ipam_test.go allocate-subnet host-2
go run ./test/integration/ipam_test.go allocate-subnet host-3

echo "2. Allocating IPs within subnet..."
go run ./test/integration/ipam_test.go allocate-ip host-1
go run ./test/integration/ipam_test.go allocate-ip host-1
go run ./test/integration/ipam_test.go allocate-ip host-1

echo "3. Releasing IP..."
go run ./test/integration/ipam_test.go release-ip 10.0.1.2

echo "4. Re-allocating released IP..."
go run ./test/integration/ipam_test.go allocate-ip host-1

echo "✓ IPAM tests passed!"
```

#### 5. Security Rules Integration Test
**File**: `test/vpc/test-security.sh` (new)
**Changes**: Test iptables rule application

```bash
#!/bin/bash
# Integration test for Security Manager (requires root)

set -e

if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This test requires root (sudo)"
    exit 1
fi

echo "=== Testing SecurityManager ==="

echo "1. Adding allow rule for web service..."
go run ./test/integration/security_test.go add-rule \
    --from "internet" \
    --to "10.0.1.5" \
    --port "443" \
    --action "allow"

echo "2. Adding deny rule..."
go run ./test/integration/security_test.go add-rule \
    --from "10.0.2.0/24" \
    --to "10.0.1.5" \
    --port "22" \
    --action "deny"

echo "3. Applying rules to iptables..."
go run ./test/integration/security_test.go apply-rules test-network

echo "4. Verifying iptables chains..."
iptables -L BANYAN-INPUT -n -v
iptables -L BANYAN-FORWARD -n -v
iptables -L BANYAN-OUTPUT -n -v

echo "5. Listing rules..."
go run ./test/integration/security_test.go list-rules test-network

echo "✓ Security tests passed!"
```

#### 6. DNS Integration Test
**File**: `test/vpc/test-dns.sh` (new)
**Changes**: Test DNS registration and lookup

```bash
#!/bin/bash
# Integration test for DNS Manager

set -e

echo "=== Testing DNSManager ==="

echo "1. Registering DNS entries..."
sudo go run ./test/integration/dns_test.go register web.internal 10.0.1.5
sudo go run ./test/integration/dns_test.go register api.internal 10.0.2.10
sudo go run ./test/integration/dns_test.go register db.internal 10.0.3.15

echo "2. Verifying /etc/hosts..."
grep "# BANYAN" /etc/hosts

echo "3. Looking up hostname..."
go run ./test/integration/dns_test.go lookup web.internal

echo "4. Marking host unhealthy..."
sudo go run ./test/integration/dns_test.go update-health web.internal false

echo "5. Lookup should now fail..."
go run ./test/integration/dns_test.go lookup web.internal || echo "Expected failure - host unhealthy"

echo "6. Unregistering DNS..."
sudo go run ./test/integration/dns_test.go unregister web.internal

echo "✓ DNS tests passed!"
```

#### 7. End-to-End Integration Test
**File**: `test/vpc/test-e2e.sh` (new)
**Changes**: Full end-to-end test with real containers

```bash
#!/bin/bash
# End-to-end integration test with real Docker containers

set -e

if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This test requires root (sudo)"
    exit 1
fi

echo "=== End-to-End VPC Integration Test ==="

# Setup
echo "Step 1: Setting up test environment..."
./test/vpc/setup.sh

# Create network
echo ""
echo "Step 2: Creating VPC network..."
NETWORK_ID=$(go run ./test/integration/network_test.go create | grep "ID=" | cut -d'=' -f2 | cut -d',' -f1)
echo "Network ID: $NETWORK_ID"

# Allocate subnets
echo ""
echo "Step 3: Allocating subnets..."
go run ./test/integration/ipam_test.go allocate-subnet host-1

# Allocate IPs
echo ""
echo "Step 4: Allocating IPs for containers..."
WEB_IP=$(go run ./test/integration/ipam_test.go allocate-ip host-1 | grep "Allocated" | awk '{print $2}')
API_IP=$(go run ./test/integration/ipam_test.go allocate-ip host-1 | grep "Allocated" | awk '{print $2}')
DB_IP=$(go run ./test/integration/ipam_test.go allocate-ip host-1 | grep "Allocated" | awk '{print $2}')

echo "Web IP: $WEB_IP"
echo "API IP: $API_IP"
echo "DB IP: $DB_IP"

# Register DNS
echo ""
echo "Step 5: Registering DNS..."
go run ./test/integration/dns_test.go register web.internal $WEB_IP
go run ./test/integration/dns_test.go register api.internal $API_IP
go run ./test/integration/dns_test.go register db.internal $DB_IP

# Apply security rules
echo ""
echo "Step 6: Applying security rules..."
# Web: Allow from internet
go run ./test/integration/security_test.go add-rule \
    --from "internet" --to "$WEB_IP" --port "443" --action "allow"

# API: Allow only from web
go run ./test/integration/security_test.go add-rule \
    --from "$WEB_IP" --to "$API_IP" --port "8080" --action "allow"

# DB: Allow only from API
go run ./test/integration/security_test.go add-rule \
    --from "$API_IP" --to "$DB_IP" --port "5432" --action "allow"

go run ./test/integration/security_test.go apply-rules $NETWORK_ID

# Verify connectivity
echo ""
echo "Step 7: Testing connectivity..."
go run ./test/integration/debug_test.go trace $WEB_IP $API_IP 8080
go run ./test/integration/debug_test.go trace $API_IP $DB_IP 5432

# Check container connectivity
echo ""
echo "Step 8: Checking container connectivity..."
WEB_CONTAINER=$(docker ps --filter "name=vpc-web" --format "{{.ID}}")
go run ./test/integration/debug_test.go check-connectivity $WEB_CONTAINER

echo ""
echo "✓ End-to-end test passed!"
echo ""
echo "Cleanup: Run ./test/vpc/cleanup.sh to remove test containers"
```

#### 8. Master Test Runner
**File**: `test/vpc/run-all.sh` (new)
**Changes**: Run all integration tests in sequence

```bash
#!/bin/bash
# Run all VPC integration tests

set -e

echo "======================================"
echo "VPC Integration Test Suite"
echo "======================================"

# Check prerequisites
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: Integration tests require root (use sudo)"
    exit 1
fi

if ! command -v docker &> /dev/null; then
    echo "ERROR: Docker is not installed"
    exit 1
fi

# Run tests
echo ""
./test/vpc/test-network.sh
echo ""
./test/vpc/test-ipam.sh
echo ""
./test/vpc/test-security.sh
echo ""
./test/vpc/test-dns.sh
echo ""
./test/vpc/test-e2e.sh

echo ""
echo "======================================"
echo "✓ All integration tests passed!"
echo "======================================"
echo ""
echo "Run ./test/vpc/cleanup.sh to clean up test environment"
```

#### 9. Makefile Integration
**File**: `Makefile` (update)
**Changes**: Add integration test targets

```makefile
# Add to existing Makefile

.PHONY: test-vpc-unit
test-vpc-unit:
	cd pkg/vpc && go test -v ./...

.PHONY: test-vpc-integration
test-vpc-integration:
	@echo "Running VPC integration tests (requires root)..."
	sudo ./test/vpc/run-all.sh

.PHONY: test-vpc-setup
test-vpc-setup:
	./test/vpc/setup.sh

.PHONY: test-vpc-cleanup
test-vpc-cleanup:
	./test/vpc/cleanup.sh

.PHONY: test-vpc
test-vpc: test-vpc-unit test-vpc-integration
```

### Success Criteria:

#### Automated Verification:
- [ ] All integration test scripts are executable: `chmod +x test/vpc/*.sh`
- [ ] Setup script creates test containers: `./test/vpc/setup.sh`
- [ ] Network test passes: `./test/vpc/test-network.sh`
- [ ] IPAM test passes: `./test/vpc/test-ipam.sh`
- [ ] Security test passes: `sudo ./test/vpc/test-security.sh`
- [ ] DNS test passes: `sudo ./test/vpc/test-dns.sh`
- [ ] End-to-end test passes: `sudo ./test/vpc/test-e2e.sh`
- [ ] Master test runner passes: `sudo ./test/vpc/run-all.sh`
- [ ] Cleanup script removes all artifacts: `./test/vpc/cleanup.sh`
- [ ] Makefile targets work: `make test-vpc`

#### Manual Verification (Now Concrete!):
Instead of vague checks, run actual scripts:
- [ ] `docker ps --filter "name=vpc-"` shows 4 test containers
- [ ] `sudo iptables -L BANYAN-INPUT -n` shows security chains
- [ ] `grep "# BANYAN" /etc/hosts` shows DNS entries
- [ ] Test containers can ping each other (after CNI setup)

**Implementation Note**: After completing this phase, you have a complete VPC implementation with comprehensive automated testing!

---

## Testing Strategy

### Unit Tests:
All tests already written in `*_test.go` files:
- Table-driven tests for each method
- Error case validation
- Edge case handling
- Concurrency testing (network, DNS)

### Integration Tests:
After all phases complete, run full integration:
```bash
cd pkg/vpc
go test -v ./...
```

Expected results:
- 35 test functions pass
- ~197 test scenarios pass
- Some tests may skip if CNI binaries or root access unavailable

### Manual Testing Steps:
1. Create a network: Verify VPC network object created
2. Allocate subnet: Verify host gets /24 subnet
3. Allocate IPs: Verify sequential IP allocation (.2, .3, .4, ...)
4. Attach container: Verify CNI integration (requires Flannel)
5. Apply security rules: Verify iptables chains created
6. Register DNS: Verify /etc/hosts updated
7. Trace connection: Verify network hops shown
8. Check connectivity: Verify gateway and internet checks

## Performance Considerations

**Not optimized in MVP**, but designed for optimization:

1. **Storage**: In-memory is fast, can add caching layer for etcd
2. **IPAM**: O(n) IP search acceptable for MVP, optimize with bitmap later
3. **Security Rules**: Apply all rules at once, not individually
4. **DNS**: Batch /etc/hosts updates, don't write on every change

## Migration Notes

**No migration needed for MVP** (fresh implementation).

**Future migrations:**
- In-memory → File-based: Export/import JSON
- File-based → etcd: Batch load into etcd
- /etc/hosts → CoreDNS: Switch DNS backend via interface

## References

- Research document: `thoughts/shared/research/2025-10-07-vpc-tdd-implementation-research.md`
- VPC architecture: `docs/vpc/02-vpc-architecture.md`
- VPC user guide: `docs/vpc/01-vpc-user-guide.md`
- Independent development: `docs/vpc/03-independent-development.md`
- Interface definitions: `pkg/vpc/interfaces.go:9-96`
- Type definitions: `pkg/vpc/types.go:9-103`
- All test files: `pkg/vpc/*/manager_test.go`

---

## Summary

This implementation plan provides a **simple, working MVP** for VPC networking following TDD principles. Each phase builds on the previous, making tests pass incrementally. The design uses extensible interfaces throughout, allowing easy migration from simple implementations (in-memory, /etc/hosts, iptables) to production-ready backends (etcd, CoreDNS, eBPF) without changing the core interfaces.

**Total estimated effort**: 6 phases, each 1-2 days, total ~2 weeks for complete implementation.
