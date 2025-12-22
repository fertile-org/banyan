# Milestone 4 & 5 Research: DNS, Service Discovery, and Production Features

---
**Research Metadata**
- Date: 2025-10-20
- Branch: feat/vpc-implementation
- Commit: f50e0d1
- Status: Complete
- Milestones Completed: Milestone 2 (Security Rules), Milestone 3 (Multi-Host Networking)
- Focus: Milestone 4 (DNS & Service Discovery), Milestone 5 (Production Features)
---

## Executive Summary

This research evaluates the current state of Banyan VPC implementation to plan Milestones 4 and 5. With Milestones 2 and 3 complete, we now have working security rules (iptables-based) and multi-host networking (Flannel VXLAN). The research identifies what exists, what's missing, and prioritizes implementation work.

**Key Findings:**
- **DNS Infrastructure**: Interface and comprehensive tests exist (404 lines), but implementation is stubbed
- **Service Discovery**: Critical gap - no service-to-container mapping or service registry exists
- **Observability**: DebugManager interface defined but no metrics/monitoring implementation
- **Network Policies**: Fully implemented via Security Manager (milestone 2 complete)
- **Multi-CNI**: Architecture supports multiple CNI plugins but only Flannel is fully implemented

**Recommendation**: Prioritize Milestone 4 with focus on service registry first (enables DNS), then implement DNS resolution. Milestone 5 can proceed in parallel with observability and multi-CNI support.

---

## Table of Contents

1. [Milestone Overview](#milestone-overview)
2. [Current Implementation Status](#current-implementation-status)
3. [Milestone 4: DNS & Service Discovery](#milestone-4-dns--service-discovery)
4. [Milestone 5: Production Features](#milestone-5-production-features)
5. [Implementation Recommendations](#implementation-recommendations)
6. [Technical Dependencies](#technical-dependencies)
7. [Risk Assessment](#risk-assessment)

---

## Milestone Overview

### Completed Milestones

**Milestone 2: Security Rules & Network Policies** ✅
- iptables-based security rule enforcement
- Per-container network policies
- Integration with CNI for rule application
- Service resolver interface (stub)

**Milestone 3: Multi-Host Networking** ✅
- etcd coordination for state management
- Flannel VXLAN overlay networking
- Dynamic subnet allocation with TTL leases
- Cross-host container communication
- Multi-host integration tests passing

### Target Milestones

**Milestone 4: DNS & Service Discovery**
- Health-aware DNS resolution
- Service registry (service name → container IPs)
- CoreDNS integration
- Automatic service registration/deregistration

**Milestone 5: Production Features**
- Observability (metrics, flow logs, monitoring)
- Network policy management UI/CLI
- Multi-CNI support (Calico, Cilium)
- Performance optimization

---

## Current Implementation Status

### DNS Manager (pkg/vpc/dns/)

**Implementation Status**: 🔴 **Stub Implementation**

**Files Analyzed:**
- `pkg/vpc/dns/manager.go` (46 lines) - Stub implementation
- `pkg/vpc/dns/manager_test.go` (404 lines) - Comprehensive test suite

**Interface Definition** (`pkg/vpc/interfaces.go:71-84`):
```go
type DNSManager interface {
    RegisterHost(ctx context.Context, hostname string, ip net.IP) error
    UnregisterHost(ctx context.Context, hostname string) error
    LookupHost(ctx context.Context, hostname string) ([]net.IP, error)
    UpdateHealth(ctx context.Context, hostname string, healthy bool) error
}
```

**Test Coverage:**
The 404-line test suite defines expected behavior across 7 test functions:
1. `TestDNSManager_RegisterHost` - Hostname registration and validation
2. `TestDNSManager_UnregisterHost` - Cleanup and idempotency
3. `TestDNSManager_LookupHost` - DNS resolution with load balancing
4. `TestDNSManager_UpdateHealth` - Health-aware filtering
5. `TestDNSManager_ServiceDiscovery` - Multiple instances per service
6. `TestDNSManager_Integration` - End-to-end scenarios
7. `TestDNSManager_ConcurrentOperations` - Thread safety

**Current Implementation** (`pkg/vpc/dns/manager.go:33-46`):
```go
type Manager struct {
    // Empty - will add fields during implementation phase
}

func (m *Manager) RegisterHost(ctx context.Context, hostname string, ip net.IP) error {
    // TDD: Implementation will be added after test review
    return nil
}
```

**Analysis**: Test-driven development approach - tests define behavior, implementation pending.

### Service Discovery & Registry

**Implementation Status**: 🔴 **Critical Gap**

**Files Analyzed:**
- `pkg/vpc/security/resolver.go` (36 lines) - Stub service resolver
- `pkg/vpc/types.go` - Container type definition (missing ServiceName field)

**Service Resolver Interface** (`pkg/vpc/interfaces.go:76-78`):
```go
type ServiceResolver interface {
    ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error)
}
```

**Current Implementation** (`pkg/vpc/security/resolver.go:25-36`):
```go
func (r *RuntimeServiceResolver) ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error) {
    // TODO: Implement actual service registry lookup when state store has service mappings
    return nil, fmt.Errorf("service %q not found in registry", serviceName)
}
```

**Critical Gap Identified:**
- No service-to-container mapping exists in StateStore
- Container struct (`pkg/vpc/types.go:8-13`) lacks ServiceName field:
```go
type Container struct {
    ID        string
    Name      string
    IP        net.IP
    Subnet    *net.IPNet
    // Missing: ServiceName field
}
```

**Impact**: Service discovery cannot work without service registry. DNS resolution depends on service registry for resolving service names to container IPs.

### Security Manager (Network Policies)

**Implementation Status**: ✅ **Complete**

**Files Analyzed:**
- `pkg/vpc/security/manager.go` (300+ lines) - Full implementation
- `pkg/vpc/security/manager_test.go` - Comprehensive tests

**Key Features Implemented:**
- iptables rule translation from high-level allow/deny rules
- Per-container network policy enforcement
- Integration with CNI runtime for rule application
- Support for service-based, CIDR-based, and internet-based rules

**Methods Implemented:**
- `AddRule()` - Add security rule with iptables translation
- `RemoveRule()` - Remove rule and cleanup iptables
- `ListRules()` - Query current rules
- `ApplyRules()` - Apply all rules to container

**Analysis**: Milestone 2 objective fully met. Security policies working with iptables integration.

### CNI Runtime

**Implementation Status**: ✅ **Complete (Flannel only)**

**Files Analyzed:**
- `pkg/vpc/cni/runtime.go` (300+ lines) - Full implementation
- `pkg/vpc/cni/runtime_test.go` - Unit tests

**Key Features Implemented:**
- Flannel CNI plugin lifecycle management
- Container network attachment via CNI ADD operation
- Container network detachment via CNI DEL operation
- Plugin configuration and validation
- Status monitoring

**Critical Methods:**
- `SetupPlugin()` (lines 32-72) - Configures CNI plugin
- `AddToNetwork()` (lines 74-200) - Attaches container to network
- `RemoveFromNetwork()` - Detaches container

**Multi-CNI Support**: Architecture supports multiple CNI plugins via `PluginType` enum:
```go
type PluginType string

const (
    PluginTypeFlannel PluginType = "flannel"
    PluginTypeCalico  PluginType = "calico"
    // Extension point for additional CNI plugins
)
```

**Analysis**: Flannel fully implemented. Calico support requires implementation (Milestone 5).

### Observability & Monitoring

**Implementation Status**: 🔴 **Interface Only**

**Files Analyzed:**
- `pkg/vpc/interfaces.go` (DebugManager interface)
- Documentation mentions (user guide, architecture docs)

**DebugManager Interface** (`pkg/vpc/interfaces.go:86-95`):
```go
type DebugManager interface {
    // TraceConnection traces connectivity between two IPs
    TraceConnection(ctx context.Context, fromIP, toIP net.IP, port int) (*TraceResult, error)

    // CheckConnectivity checks if a container has network connectivity
    CheckConnectivity(ctx context.Context, containerID string) (*ConnectivityResult, error)

    // GetIPTablesRules returns iptables rules for an IP
    GetIPTablesRules(ctx context.Context, ip net.IP) ([]string, error)
}
```

**Documentation References:**
- User guide mentions flow logs: `banyan network flow-logs <service>` (docs/vpc/01-vpc-user-guide.md:297)
- Architecture doc lists as planned: "Observability (metrics, flow logs)" (docs/vpc/02-vpc-architecture.md:203)

**Current State:**
- No metrics collection implementation
- No flow log implementation
- No monitoring/telemetry integration
- DebugManager interface defined but not implemented

**Analysis**: Interface designed but implementation completely missing. Milestone 5 work.

### State Storage

**Implementation Status**: ✅ **Complete**

**Files Analyzed:**
- `pkg/vpc/storage/etcd.go` - etcd backend implementation
- `pkg/vpc/storage/memory.go` - In-memory backend for testing

**Key Features:**
- StateStore interface abstraction
- etcd backend with TTL support for lease management
- In-memory backend for unit testing
- Optimistic locking via etcd transactions

**Analysis**: Storage layer complete and battle-tested in multi-host integration tests.

---

## Milestone 4: DNS & Service Discovery

### Goal

Enable services to discover each other using DNS names (e.g., `database.internal`) with health-aware load balancing across multiple instances.

### Current State Summary

| Component | Status | Lines of Code | Test Coverage | Priority |
|-----------|--------|---------------|---------------|----------|
| DNSManager Interface | ✅ Defined | 14 lines | N/A | - |
| DNSManager Tests | ✅ Complete | 404 lines | 7 test functions | - |
| DNSManager Implementation | 🔴 Stub | 46 lines | 0% | P0 |
| Service Registry | 🔴 Missing | 0 lines | None | P0 |
| Container.ServiceName | 🔴 Missing | N/A | None | P0 |
| CoreDNS Integration | 🔴 Not Started | 0 lines | None | P1 |

### Critical Dependencies

**Blocker**: Service Registry must exist before DNS can function.

**Dependency Chain**:
1. **Service Registry** (P0) - Maps service names to container IPs
   - Add `ServiceName` field to `Container` struct
   - Store service mappings in etcd: `/banyan/vpc/services/{serviceName}/instances/{containerID}`
   - Implement service registration in CNI `AddToNetwork()`
   - Implement service deregistration in CNI `RemoveFromNetwork()`

2. **DNS Manager Implementation** (P0) - Implements 4 interface methods
   - `RegisterHost()` - Add hostname → IP mapping
   - `UnregisterHost()` - Remove hostname → IP mapping
   - `LookupHost()` - Resolve hostname to IPs with load balancing
   - `UpdateHealth()` - Mark instance as healthy/unhealthy

3. **CoreDNS Integration** (P1) - External DNS server
   - Run CoreDNS as system service
   - Configure CoreDNS to query Banyan DNS Manager
   - Container DNS resolution via CoreDNS

### Implementation Approach

#### Phase 1: Service Registry (Week 1)

**Task 1.1**: Add ServiceName to Container struct
```go
// pkg/vpc/types.go
type Container struct {
    ID          string
    Name        string
    ServiceName string  // NEW: Service this container belongs to
    IP          net.IP
    Subnet      *net.IPNet
}
```

**Task 1.2**: Implement Service Registry in StateStore
```go
// New methods in StateStore interface
PutService(ctx context.Context, serviceName, containerID string, ip net.IP) error
DeleteService(ctx context.Context, serviceName, containerID string) error
GetServiceInstances(ctx context.Context, serviceName string) ([]ServiceInstance, error)
```

**Task 1.3**: Integrate with CNI Runtime
- Modify `AddToNetwork()` to register service instances
- Modify `RemoveFromNetwork()` to deregister service instances

**Success Criteria**:
- Service registry stores service → container mappings in etcd
- Services can be queried programmatically
- Unit tests validate registration/deregistration

#### Phase 2: DNS Manager Implementation (Week 1-2)

**Task 2.1**: Implement DNSManager with in-memory cache
```go
type Manager struct {
    store    StateStore
    cache    map[string][]hostRecord  // hostname → IP records
    mu       sync.RWMutex
    ttl      time.Duration
    resolver ServiceResolver
}

type hostRecord struct {
    IP      net.IP
    Healthy bool
    LastSeen time.Time
}
```

**Task 2.2**: Implement 4 interface methods
- `RegisterHost()` - Store in cache and persist to etcd
- `UnregisterHost()` - Remove from cache and etcd
- `LookupHost()` - Return healthy IPs with round-robin load balancing
- `UpdateHealth()` - Mark instance health status

**Task 2.3**: Run all existing tests
- 404 lines of tests already written
- All 7 test functions should pass
- Fix implementation until tests are green

**Success Criteria**:
- All 404 lines of existing tests pass
- DNS resolution returns healthy container IPs
- Load balancing distributes requests across instances

#### Phase 3: CoreDNS Integration (Week 2)

**Task 3.1**: CoreDNS plugin development
```go
// Custom CoreDNS plugin queries Banyan DNS Manager
func (b *BanyanPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
    hostname := extractHostname(r)
    ips, err := b.dnsManager.LookupHost(ctx, hostname)
    // Return DNS A records for all healthy IPs
}
```

**Task 3.2**: Container DNS configuration
- Configure containers to use CoreDNS as nameserver
- Update CNI plugin to inject CoreDNS resolver in `/etc/resolv.conf`

**Task 3.3**: Integration testing
- Deploy multi-container test with service discovery
- Verify DNS resolution: `nslookup database.internal`
- Verify load balancing across multiple instances

**Success Criteria**:
- Containers can resolve service names via DNS
- Health-aware resolution filters unhealthy instances
- Load balancing works across multiple instances

### Testing Strategy

**Unit Tests**: Already exist (404 lines), need implementation to pass them

**Integration Tests**: Create new test file
```go
// test/integration/vpc/test_dns_integration.go
func TestDNSServiceDiscovery(t *testing.T) {
    // 1. Start 3 instances of "web" service
    // 2. Register all 3 with DNS manager
    // 3. Query "web.internal" - should return 3 IPs
    // 4. Mark 1 instance unhealthy
    // 5. Query again - should return 2 IPs (healthy only)
}
```

**End-to-End Tests**: Real container DNS resolution
```bash
# Start containers
docker run --name web1 ...
docker run --name web2 ...

# Test DNS resolution from another container
docker exec test-container nslookup web.internal
# Should return: web1 IP, web2 IP
```

### Documentation Updates

- Update architecture doc with service registry design
- Update user guide with DNS usage examples
- Add troubleshooting guide for DNS issues
- Document CoreDNS plugin development

---

## Milestone 5: Production Features

### Goal

Add observability, monitoring, and multi-CNI support for production deployments.

### Feature Breakdown

#### 5.1 Observability & Metrics

**Current State**: 🔴 DebugManager interface only

**Components to Implement**:

1. **DebugManager Implementation**
   - `TraceConnection()` - Trace packet path between IPs
   - `CheckConnectivity()` - Validate container networking
   - `GetIPTablesRules()` - Retrieve firewall rules

2. **Metrics Collection**
   - Prometheus exporter for VPC metrics
   - Metrics to expose:
     - Container network traffic (bytes in/out)
     - DNS query rate and latency
     - Security rule hit counts
     - IPAM allocation/release rates
     - Cross-host VXLAN traffic

3. **Flow Logs**
   - Implement `banyan network flow-logs <service>` command
   - Log network flows to etcd or external system
   - Support filtering by source/destination/port

**Priority**: P1 (Important for production but not blocking)

**Effort**: 2-3 weeks

#### 5.2 Multi-CNI Support

**Current State**: 🟡 Flannel implemented, architecture supports Calico

**Components to Implement**:

1. **Calico CNI Plugin**
   - Implement `SetupPlugin()` for Calico
   - Calico network policy translation
   - Calico IPAM integration

2. **CNI Plugin Selection**
   - Allow users to choose CNI plugin in config
   - Runtime detection of available plugins
   - Fallback to Flannel if preferred plugin unavailable

**Priority**: P2 (Nice to have, not urgent)

**Effort**: 3-4 weeks

#### 5.3 Network Policy Management

**Current State**: ✅ Already implemented in Milestone 2

**Existing Features**:
- Security Manager with iptables integration
- Allow/deny rule support
- Service-based, CIDR-based, internet-based rules

**Potential Enhancements** (P2):
- Network policy visualization UI
- Policy dry-run mode (simulate without applying)
- Policy conflict detection
- Egress rule support (currently ingress only)

**Priority**: P2 (Enhancement, not new feature)

**Effort**: 2 weeks for enhancements

### Implementation Priority

| Feature | Priority | Effort | Dependencies | Start After |
|---------|----------|--------|--------------|-------------|
| Observability | P1 | 2-3 weeks | None | Milestone 4 Phase 1 |
| Multi-CNI (Calico) | P2 | 3-4 weeks | None | Milestone 4 complete |
| Policy Enhancements | P2 | 2 weeks | None | Milestone 4 complete |

### Testing Strategy

**Observability Testing**:
- Unit tests for DebugManager methods
- Integration test for metrics collection
- Validate flow log capture

**Multi-CNI Testing**:
- Test Calico plugin setup
- Cross-host communication with Calico
- Compare performance vs Flannel

---

## Implementation Recommendations

### Recommended Sequence

**Phase 1: Service Registry (Highest Priority)** - Week 1
- Critical blocker for DNS functionality
- Relatively simple implementation (etcd key-value storage)
- Enables all downstream DNS work

**Phase 2: DNS Manager Implementation** - Week 1-2
- Leverage existing 404-line test suite
- Implement until all tests pass (TDD approach)
- In-memory cache for performance

**Phase 3: CoreDNS Integration** - Week 2
- Develop CoreDNS plugin for Banyan
- Configure container DNS resolution
- Integration testing

**Phase 4: Observability** - Week 3-4 (Parallel with DNS)
- Can start in parallel with DNS work
- Not blocking other features
- Important for production readiness

**Phase 5: Multi-CNI** - Week 5-7 (Future)
- Lower priority enhancement
- Only after Milestone 4 complete

### Quick Wins

1. **Service Registry**: Can be implemented in 1-2 days, unblocks DNS work
2. **DebugManager.GetIPTablesRules()**: Simple implementation (call `iptables -L`)
3. **Basic Metrics**: Expose simple counters (container count, subnet usage)

### Risk Mitigation

**Risk 1**: CoreDNS integration complexity
- **Mitigation**: Start with simple DNS server (custom Go implementation) before CoreDNS
- **Fallback**: Implement `/etc/hosts` file generation as interim solution

**Risk 2**: Service registry performance at scale
- **Mitigation**: Implement in-memory cache with etcd backend
- **Optimization**: Use etcd watch for cache invalidation

**Risk 3**: Health check implementation
- **Mitigation**: Start with manual health updates via API
- **Enhancement**: Add automatic health probes later

---

## Technical Dependencies

### External Dependencies

| Dependency | Version | Purpose | Milestone |
|------------|---------|---------|-----------|
| etcd | 3.5+ | State storage, service registry | M3 (existing) |
| Flannel | 0.20+ | VXLAN overlay networking | M3 (existing) |
| CoreDNS | 1.10+ | DNS server with plugin support | M4 |
| Prometheus | 2.40+ | Metrics collection (optional) | M5 |
| Calico | 3.25+ | Alternative CNI plugin | M5 |

### Internal Dependencies

**Milestone 4 Dependency Graph**:
```
Service Registry (P0)
    ↓
DNS Manager Implementation (P0)
    ↓
CoreDNS Integration (P1)
```

**Milestone 5 Dependency Graph**:
```
Milestone 4 Complete
    ↓
    ├─→ Observability (P1) [No blockers]
    ├─→ Multi-CNI (P2) [No blockers]
    └─→ Policy Enhancements (P2) [No blockers]
```

---

## Risk Assessment

### High Risk Items

**1. Service Registry Performance** 🔴
- **Risk**: etcd queries for every DNS lookup could cause latency
- **Impact**: High (affects all service communication)
- **Likelihood**: Medium
- **Mitigation**: In-memory cache with etcd watch for updates
- **Monitoring**: Measure DNS query latency, set SLO < 10ms

**2. CoreDNS Plugin Complexity** 🟡
- **Risk**: CoreDNS plugin development may be time-consuming
- **Impact**: Medium (delays Milestone 4 completion)
- **Likelihood**: Medium
- **Mitigation**: Implement simple DNS server first, CoreDNS later
- **Fallback**: Use `/etc/hosts` file generation as interim solution

### Medium Risk Items

**3. Health Check Implementation** 🟡
- **Risk**: Automatic health probes may not detect all failure modes
- **Impact**: Medium (affects DNS load balancing quality)
- **Likelihood**: Low
- **Mitigation**: Start with manual health API, add probes incrementally
- **Validation**: Test with real failure scenarios

**4. Multi-CNI Testing** 🟡
- **Risk**: Calico behavior may differ from Flannel unexpectedly
- **Impact**: Low (only affects Calico users)
- **Likelihood**: Medium
- **Mitigation**: Comprehensive integration tests comparing both CNIs
- **Documentation**: Document known differences

### Low Risk Items

**5. Observability Overhead** 🟢
- **Risk**: Metrics collection may impact performance
- **Impact**: Low (can be disabled if needed)
- **Likelihood**: Low
- **Mitigation**: Make metrics collection optional, use sampling

---

## Conclusion

**Milestones 2 and 3 are complete and working well.** Security rules and multi-host networking provide a solid foundation.

**Milestone 4 (DNS & Service Discovery) has clear path forward:**
1. Implement service registry (1-2 days)
2. Implement DNS Manager to pass existing 404-line test suite (3-5 days)
3. Integrate CoreDNS (3-5 days)
4. Total estimate: **2 weeks**

**Milestone 5 (Production Features) can proceed in parallel:**
- Observability is independent and can start during Milestone 4
- Multi-CNI is lower priority, can wait until after Milestone 4

**Recommended Action**: Start with service registry implementation immediately. It's the critical blocker for all DNS functionality and has the highest ROI.

---

## Appendix: File Reference

### Files Read During Research

**Documentation:**
- docs/vpc/01-vpc-user-guide.md (310 lines)
- docs/vpc/02-vpc-architecture.md (248 lines)
- docs/vpc/05-milestone-2-3-implementation.md (1000+ lines)

**Source Code:**
- pkg/vpc/interfaces.go (95 lines) - All 6 manager interfaces
- pkg/vpc/dns/manager.go (46 lines) - Stub implementation
- pkg/vpc/dns/manager_test.go (404 lines) - Comprehensive tests
- pkg/vpc/security/manager.go (300+ lines) - Full implementation
- pkg/vpc/security/resolver.go (36 lines) - Stub service resolver
- pkg/vpc/cni/runtime.go (300+ lines) - Full implementation
- pkg/vpc/types.go (50+ lines) - Data structures

**Tests:**
- test/integration/vpc/test_cni_docker_integration.go - Single-host CNI tests
- test/integration/vpc/run_multi_host_integration.go - Multi-host tests

### Key Findings by File

| File | Key Finding | Implication |
|------|-------------|-------------|
| dns/manager.go | Stub (46 lines) | Need full implementation |
| dns/manager_test.go | 404 lines complete | Tests guide implementation (TDD) |
| security/resolver.go | Returns "not implemented" | Service registry critical blocker |
| types.go | Missing ServiceName | Must add field to Container struct |
| interfaces.go | DebugManager defined | Observability interface ready |
| cni/runtime.go | Flannel complete | Multi-CNI architecture proven |

---

**Research Complete**: 2025-10-20
**Next Steps**: Review with team, prioritize implementation phases, begin service registry implementation.
