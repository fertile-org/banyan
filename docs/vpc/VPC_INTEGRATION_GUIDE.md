# VPC Module Integration Guide

This is the **single source of truth** for integrating with the Banyan VPC module. All integration should be done via Go function calls, not CLI commands.

## Table of Contents

- [Quick Start](#quick-start)
- [Architecture Overview](#architecture-overview)
- [Module Components](#module-components)
  - [1. Storage Layer](#1-storage-layer)
  - [2. IPAM Manager](#2-ipam-manager)
  - [3. CNI Runtime](#3-cni-runtime)
  - [4. Security Manager](#4-security-manager)
  - [5. DNS Manager](#5-dns-manager)
  - [6. Debug Manager](#6-debug-manager)
  - [7. Network Initialization](#7-network-initialization)
- [Integration Patterns](#integration-patterns)
  - [Engine Integration](#engine-integration)
  - [Agent Integration](#agent-integration)
  - [Container Lifecycle](#container-lifecycle)
- [Data Types Reference](#data-types-reference)
  - [Core Types](#core-types)
  - [Debug Types](#debug-types)
- [Prerequisites](#prerequisites)
- [Error Handling](#error-handling)
- [Testing](#testing)
- [Summary](#summary)

---

## Quick Start

```go
import (
    "github.com/fertile-org/banyan/pkg/vpc"
    "github.com/fertile-org/banyan/pkg/vpc/storage"
    "github.com/fertile-org/banyan/pkg/vpc/ipam"
    "github.com/fertile-org/banyan/pkg/vpc/cni"
    "github.com/fertile-org/banyan/pkg/vpc/security"
    "github.com/fertile-org/banyan/pkg/vpc/dns"
    "github.com/fertile-org/banyan/pkg/vpc/debug"
)
```

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         VPC Module                                   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌────────────┐ │
│  │   Storage   │  │    IPAM     │  │     CNI     │  │  Security  │ │
│  │  (etcd or   │  │  (IP Addr   │  │  (Flannel   │  │ (iptables  │ │
│  │   memory)   │  │   Mgmt)     │  │   Plugin)   │  │   rules)   │ │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └─────┬──────┘ │
│         │                │                │                │        │
│  ┌──────┴──────┐  ┌──────┴──────┐  ┌──────┴──────┐  ┌─────┴──────┐ │
│  │     DNS     │  │    Debug    │  │   Network   │  │   Types    │ │
│  │  (Service   │  │ (Diagnostic │  │  (Network   │  │ (Shared    │ │
│  │  Discovery) │  │   Tools)    │  │   Init)     │  │  Structs)  │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └────────────┘ │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Module Components

### 1. Storage Layer

The storage layer provides state persistence. Choose based on deployment:

| Store Type | Use Case | Import |
|------------|----------|--------|
| `EtcdStore` | Production (multi-host) | `storage.NewEtcdStore(endpoints, prefix)` |
| `MemoryStore` | Testing/Single-host | `storage.NewMemoryStore()` |

```go
// Production: Use etcd for distributed state
store, err := storage.NewEtcdStore([]string{"http://engine-ip:2379"}, "/banyan")
if err != nil {
    return err
}

// Testing: Use in-memory store
store := storage.NewMemoryStore()
```

**StateStore Interface:**
```go
type StateStore interface {
    Save(ctx context.Context, key string, value interface{}) error
    Get(ctx context.Context, key string, value interface{}) error
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}
```

---

### 2. IPAM Manager

Manages IP address allocation with lease-based subnet assignment.

```go
import "github.com/fertile-org/banyan/pkg/vpc/ipam"

// Create IPAM manager
manager, err := ipam.NewManager(store, "10.0.0.0/16")  // VPC CIDR
if err != nil {
    return err
}

// Allocate /24 subnet for a host (24h TTL lease)
subnet, err := manager.AllocateHostSubnet(ctx, "host-01")
// Returns: 10.0.1.0/24

// Allocate IP from host's subnet
ip, err := manager.AllocateIP(ctx, subnet)
// Returns: 10.0.1.2

// Release IP when container is removed
err := manager.ReleaseIP(ctx, ip)

// Renew subnet lease (call every 5 minutes)
err := manager.RenewLease(ctx, "host-01")

// Get host's subnet
subnet, err := manager.GetHostSubnet(ctx, "host-01")
```

**IPAMManager Interface:**
```go
type IPAMManager interface {
    AllocateHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error)
    AllocateIP(ctx context.Context, subnet *net.IPNet) (net.IP, error)
    ReleaseIP(ctx context.Context, ip net.IP) error
    RenewLease(ctx context.Context, hostID string) error
    GetHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error)
}
```

---

### 3. CNI Runtime

Manages container network attachment via Flannel CNI plugin.

```go
import "github.com/fertile-org/banyan/pkg/vpc/cni"

// Create CNI runtime (requires security manager for rule enforcement)
runtime := cni.NewRuntime(store, securityManager)

// Setup Flannel plugin (one-time per host)
config := []byte(`{"Network": "10.0.0.0/16", "Backend": {"Type": "vxlan"}}`)
err := runtime.SetupPlugin(ctx, "flannel", config)

// Attach container to network
err := runtime.AddToNetwork(ctx, containerID, networkID, ip)

// Detach container from network
err := runtime.RemoveFromNetwork(ctx, containerID, networkID)

// Check plugin status
status, err := runtime.GetPluginStatus(ctx, "flannel")
```

**CNIRuntime Interface:**
```go
type CNIRuntime interface {
    AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error
    RemoveFromNetwork(ctx context.Context, containerID, networkID string) error
    SetupPlugin(ctx context.Context, plugin string, config []byte) error
    GetPluginStatus(ctx context.Context, plugin string) (*PluginStatus, error)
}
```

---

### 4. Security Manager

Manages network security rules via iptables.

```go
import "github.com/fertile-org/banyan/pkg/vpc/security"

// Create security manager
// - resolver: resolves service names to IPs (use security.NewRuntimeServiceResolver(store))
// - dryRun: if true, rules are validated but not applied to iptables
resolver := security.NewRuntimeServiceResolver(store)
manager := security.NewManager(resolver, false)

// Add a security rule
rule := &vpc.SecurityRule{
    ID:          "rule-001",
    NetworkID:   "vpc-default",
    Direction:   "ingress",
    Action:      "allow",
    From:        "cidr:10.0.1.0/24",   // Source CIDR (use "cidr:" prefix)
    To:          "cidr:10.0.2.10/32",  // Destination IP (use "cidr:" prefix)
    ToPort:      "5432",               // PostgreSQL
    Protocol:    "tcp",
}
err := manager.AddRule(ctx, rule)

// Apply rules to system (writes iptables)
err := manager.ApplyRules(ctx, "vpc-default")

// List rules for a network
rules, err := manager.ListRules(ctx, "vpc-default")

// Remove a rule
err := manager.RemoveRule(ctx, "rule-001")
```

**SecurityManager Interface:**
```go
type SecurityManager interface {
    AddRule(ctx context.Context, rule *SecurityRule) error
    RemoveRule(ctx context.Context, ruleID string) error
    ListRules(ctx context.Context, networkID string) ([]*SecurityRule, error)
    ApplyRules(ctx context.Context, networkID string) error  // Writes to iptables
}
```

**SecurityRule Structure:**
```go
type SecurityRule struct {
    ID          string  // Unique rule identifier
    NetworkID   string  // VPC network this applies to
    ServiceName string  // Service this rule applies to (optional)
    Direction   string  // "ingress" or "egress"
    Action      string  // "allow" or "deny"
    From        string  // Source: "service:name", "cidr:x.x.x.x/y", or "internet"
    To          string  // Destination: same formats as From
    ToPort      string  // Port or range: "80" or "8000-8100"
    Protocol    string  // "tcp", "udp", "icmp", or "" for all
}
```

---

### 5. DNS Manager

Manages DNS for service discovery with health-aware resolution.

```go
import "github.com/fertile-org/banyan/pkg/vpc/dns"

// Create DNS manager
manager := dns.NewManagerWithStore(store)

// Register hostname with IP
err := manager.RegisterHost(ctx, "web.internal", net.ParseIP("10.0.1.10"))

// Register multiple IPs for load balancing
manager.RegisterHost(ctx, "api.internal", net.ParseIP("10.0.2.10"))
manager.RegisterHost(ctx, "api.internal", net.ParseIP("10.0.2.11"))
manager.RegisterHost(ctx, "api.internal", net.ParseIP("10.0.2.12"))

// Lookup hostname (returns all healthy IPs)
ips, err := manager.LookupHost(ctx, "api.internal")
// Returns: [10.0.2.10, 10.0.2.11, 10.0.2.12]

// Update health status (unhealthy hosts excluded from lookup)
err := manager.UpdateHealth(ctx, "api.internal", false)

// Unregister hostname
err := manager.UnregisterHost(ctx, "web.internal")
```

**DNS Server (for actual DNS queries):**
```go
// Create DNS server
config := dns.ServerConfig{
    BindAddr:     "0.0.0.0:53",
    InternalZone: "internal",
    UpstreamDNS:  "8.8.8.8:53",
}
server := dns.NewServer(manager, config)

// Start server (blocks, run in goroutine)
go server.Start()

// Check status
running := server.IsRunning()
addr := server.BindAddress()

// Stop server
server.Stop()
```

**DNSManager Interface:**
```go
type DNSManager interface {
    RegisterHost(ctx context.Context, hostname string, ip net.IP) error
    UnregisterHost(ctx context.Context, hostname string) error
    LookupHost(ctx context.Context, hostname string) ([]net.IP, error)
    UpdateHealth(ctx context.Context, hostname string, healthy bool) error
}
```

---

### 6. Debug Manager

Diagnostic tools for network troubleshooting. **Read-only** - does not modify network state.

```go
import "github.com/fertile-org/banyan/pkg/vpc/debug"

// Create debug manager
manager := debug.NewManagerWithStore(store)

// Trace connection path (predicts if traffic would be allowed/blocked)
result, err := manager.TraceConnection(ctx,
    net.ParseIP("10.0.1.5"),   // from
    net.ParseIP("10.0.2.10"),  // to
    5432,                       // port
)
// result.Status: "allowed" or "blocked"
// result.Reachable: true/false
// result.Hops: network path
// result.BlockedBy: rule ID if blocked

// Check container connectivity
result, err := manager.CheckConnectivity(ctx, "container-001")
// result.HasNetwork, result.IP, result.Gateway, result.DNS
// result.InternetAccess, result.Errors

// Get iptables rules for an IP
rules, err := manager.GetIPTablesRules(ctx, net.ParseIP("10.0.1.5"))
```

**DebugManager Interface:**
```go
type DebugManager interface {
    TraceConnection(ctx context.Context, fromIP, toIP net.IP, port int) (*TraceResult, error)
    CheckConnectivity(ctx context.Context, containerID string) (*ConnectivityResult, error)
    GetIPTablesRules(ctx context.Context, ip net.IP) ([]string, error)
}
```

---

### 7. Network Initialization

One-time setup to initialize Flannel network configuration in etcd.

```go
import "github.com/fertile-org/banyan/pkg/vpc"

// Initialize network (writes Flannel config to etcd)
// Call once from Engine during startup
err := vpc.InitializeNetwork(ctx,
    []string{"http://localhost:2379"},  // etcd endpoints
    "10.0.0.0/16",                       // VPC CIDR
)
```

---

## Integration Patterns

### Engine Integration

The Engine is responsible for:
1. Starting etcd server
2. Initializing network configuration
3. Managing high-level orchestration

```go
func (e *Engine) Start(ctx context.Context) error {
    // 1. Start etcd (implementation depends on your etcd setup)
    e.startEtcd()

    // 2. Create etcd store
    store, err := storage.NewEtcdStore([]string{"http://localhost:2379"}, "/banyan")
    if err != nil {
        return err
    }

    // 3. Initialize VPC network (one-time)
    err = vpc.InitializeNetwork(ctx, []string{"http://localhost:2379"}, "10.0.0.0/16")
    if err != nil {
        return err
    }

    // 4. Create managers
    e.ipam, err = ipam.NewManager(store, "10.0.0.0/16")
    if err != nil {
        return err
    }

    resolver := security.NewRuntimeServiceResolver(store)
    e.security = security.NewManager(resolver, false)
    e.dns = dns.NewManagerWithStore(store)

    return nil
}
```

### Agent Integration

The Agent is responsible for:
1. Acquiring subnet from Engine's etcd
2. Running Flannel daemon
3. Managing container networking on its host

```go
func (a *Agent) Start(ctx context.Context) error {
    // 1. Connect to Engine's etcd
    store, err := storage.NewEtcdStore([]string{"http://engine-ip:2379"}, "/banyan")
    if err != nil {
        return err
    }

    // 2. Create managers
    a.ipam, err = ipam.NewManager(store, "10.0.0.0/16")
    if err != nil {
        return err
    }

    resolver := security.NewRuntimeServiceResolver(store)
    secMgr := security.NewManager(resolver, false)
    a.cni = cni.NewRuntime(store, secMgr)
    a.dns = dns.NewManagerWithStore(store)

    // 3. Allocate subnet for this host
    subnet, err := a.ipam.AllocateHostSubnet(ctx, a.hostID)
    if err != nil {
        return err
    }
    a.subnet = subnet

    // 4. Setup CNI plugin
    err = a.cni.SetupPlugin(ctx, "flannel", nil)
    if err != nil {
        return err
    }

    // 5. Start lease renewal goroutine
    go a.renewLeaseLoop(ctx)

    return nil
}

func (a *Agent) renewLeaseLoop(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    for {
        select {
        case <-ticker.C:
            a.ipam.RenewLease(ctx, a.hostID)
        case <-ctx.Done():
            return
        }
    }
}
```

### Container Lifecycle

```go
// Deploy container
func (a *Agent) DeployContainer(ctx context.Context, containerID, serviceName string) error {
    // 1. Allocate IP
    ip, err := a.ipam.AllocateIP(ctx, a.subnet)
    if err != nil {
        return err
    }

    // 2. Create container with --network=none (container runtime specific)
    // ... create container ...

    // 3. Attach to network
    err = a.cni.AddToNetwork(ctx, containerID, "vpc-default", ip)
    if err != nil {
        a.ipam.ReleaseIP(ctx, ip)  // Cleanup on failure
        return err
    }

    // 4. Register DNS
    hostname := fmt.Sprintf("%s.internal", serviceName)
    err = a.dns.RegisterHost(ctx, hostname, ip)
    if err != nil {
        // Log warning but don't fail - DNS is optional
        log.Warn("Failed to register DNS", "error", err)
    }

    // 5. Track container-IP mapping
    a.containers[containerID] = ip

    return nil
}

// Remove container
func (a *Agent) RemoveContainer(ctx context.Context, containerID, serviceName string) error {
    ip := a.containers[containerID]

    // 1. Unregister DNS
    hostname := fmt.Sprintf("%s.internal", serviceName)
    a.dns.UnregisterHost(ctx, hostname)

    // 2. Detach from network
    a.cni.RemoveFromNetwork(ctx, containerID, "vpc-default")

    // 3. Release IP
    a.ipam.ReleaseIP(ctx, ip)

    // 4. Remove container (container runtime specific)
    // ... remove container ...

    delete(a.containers, containerID)
    return nil
}
```

---

## Data Types Reference

### Core Types

```go
// Network represents a VPC network
type Network struct {
    ID        string
    Name      string
    CIDR      string
    VxlanID   int
    DNSSuffix string
    CreatedAt time.Time
    Status    string  // active, pending, error
}

// Container represents a container attached to the network
type Container struct {
    ID        string
    NetworkID string
    IP        net.IP
    HostID    string
    Status    string
    CreatedAt time.Time
}

// SubnetLease represents a subnet allocated to a host
type SubnetLease struct {
    HostID    string
    Subnet    *net.IPNet
    LeaseTime time.Time
    ExpiresAt time.Time
}
```

### Debug Types

```go
// TraceHop represents a single hop in a connection trace
type TraceHop struct {
    Type    string  // gateway, vxlan, container, nat, loopback, destination
    Address string  // IP or hostname
    Latency string  // e.g., "2ms"
}

// TraceResult contains the result of a connection trace
type TraceResult struct {
    FromIP           net.IP
    ToIP             net.IP
    Port             int
    Status           string      // "allowed", "blocked", or "error"
    Reachable        bool
    Hops             []TraceHop
    BlockedBy        string      // Rule ID if blocked
    BlockedByDetails string      // Detailed explanation
    AllowedBy        string      // Rule ID if allowed
    Latency          string      // Total latency
    Error            string
}

// ConnectivityResult represents connectivity check results
type ConnectivityResult struct {
    ContainerID    string
    Status         string    // ok, degraded, error, unreachable
    HasNetwork     bool
    IP             net.IP
    Gateway        net.IP
    DNS            []net.IP
    InternetAccess bool
    DefaultRoute   string
    ExternalPing   bool      // Can ping external IPs
    InternalPing   bool      // Can ping other containers
    Errors         []string  // Multiple error messages
    Error          string    // Single error message
}
```

---

## Prerequisites

### System Requirements

- Linux with kernel 4.x+ (for VXLAN support)
- containerd (not Docker - Docker 28.x has compatibility issues)
- CNI plugins in `/opt/cni/bin/` (bridge, host-local, loopback, flannel)
- etcd v3.5+ (for production multi-host)

### Required Capabilities

The process running VPC operations needs:
- `CAP_NET_ADMIN` - Network configuration
- `CAP_SYS_ADMIN` - Namespace management
- `CAP_NET_RAW` - Network operations
- `CAP_DAC_OVERRIDE` - Access CNI state files

---

## Error Handling

All VPC functions return errors that should be handled:

```go
subnet, err := manager.AllocateHostSubnet(ctx, hostID)
if err != nil {
    if strings.Contains(err.Error(), "no available subnets") {
        // VPC CIDR exhausted
    }
    if strings.Contains(err.Error(), "connection refused") {
        // etcd not reachable
    }
    return err
}
```

Common error scenarios:
- etcd connection failure
- IP/subnet exhaustion
- CNI plugin not found
- iptables permission denied
- Network namespace errors

---

## Testing

Run integration tests:
```bash
# IPAM
sudo go run test/integration/vpc/run_ipam_integration.go

# DNS
sudo go run test/integration/vpc/run_dns_integration.go

# Debug
sudo go run test/integration/vpc/run_debug_integration.go

# Multi-host (requires containerd)
sudo go run test/integration/vpc/run_multi_host_integration.go
```

---

## Summary

| Component | Purpose | Key Function |
|-----------|---------|--------------|
| Storage | State persistence | `NewEtcdStore()` / `NewMemoryStore()` |
| IPAM | IP allocation | `AllocateIP()`, `ReleaseIP()` |
| CNI | Container networking | `AddToNetwork()`, `RemoveFromNetwork()` |
| Security | Firewall rules | `AddRule()`, `ApplyRules()` |
| DNS | Service discovery | `RegisterHost()`, `LookupHost()` |
| Debug | Diagnostics | `TraceConnection()`, `CheckConnectivity()` |

**The VPC module is designed to be called programmatically. Use these Go functions directly - do not shell out to CLI commands.**
