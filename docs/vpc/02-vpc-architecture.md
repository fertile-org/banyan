# VPC Architecture

## Overview

This document outlines the implementation strategy for Banyan's networking layer, using CNI (Container Network Interface) from the start while keeping the user experience simple.

## Guiding Principles

1. **CNI from Day One** - Use Flannel CNI plugin for proven networking
2. **Hide Complexity** - Users only see simple `allow` rules, never CNI details
3. **Smart Defaults** - Auto-generate configs from docker-compose
4. **Progressive Enhancement** - Advanced features available when needed

## Architecture Design

### Network Provider Interface

```
User Config (banyan.yaml)
         ↓
    Banyan Engine
         ↓
  NetworkProvider Interface  ← Abstraction Layer
         ↓
   CNI Runtime
         ↓
   CNI Plugins:
   - Flannel (Default - VXLAN backend)
   - Calico (Advanced - BGP routing)
   - Cilium (Future - eBPF)
```

### Key Components

1. **CNI Runtime** - Manages CNI plugin lifecycle
2. **IPAM (Hierarchical Leases)** - Two-tier IP allocation
3. **SecurityProvider** - Translates allow rules to iptables
4. **DNSProvider** - Service discovery via CoreDNS
5. **State Store** - Embedded etcd for coordination

## Implementation Phases

### Phase 1: Flannel CNI with VXLAN (MVP)

**Goals:**
- Working multi-host networking with proven CNI plugin
- Hierarchical IPAM with subnet leases
- Basic security rules
- Auto-config generation from docker-compose

**Components:**
- Flannel CNI plugin (VXLAN backend)
- Hierarchical IPAM (host gets /24, allocates IPs locally)
- IptablesSecurity for allow/deny rules
- CoreDNS for service discovery
- Embedded etcd for state coordination

**Deliverables:**
- CNI-based overlay networking
- Service-to-service communication with security
- DNS-based discovery
- `banyan init` command for auto-config
- Debug commands (`banyan network trace`)

### Phase 2: Advanced CNI Support (Production)

**Goals:**
- Support for Calico (BGP routing, network policies)
- Egress control rules
- Observability integration
- Performance optimization

**Components:**
- Multiple CNI plugin support
- Advanced security policies
- Metrics and logging framework
- Network flow tracking

**Deliverables:**
- Calico plugin option for large scale
- Egress traffic control
- Prometheus metrics export
- Network flow logs

## Technical Decisions

### Why Flannel CNI First?

1. **Proven & Simple** - Most widely deployed simple CNI plugin
2. **VXLAN Backend** - Gets the overlay networking we want
3. **Minimal Dependencies** - Works without Kubernetes
4. **Easy Migration** - Can switch to Calico/Cilium later

### Hierarchical IPAM Design

```
VPC CIDR: 10.0.0.0/16
    ↓
Host-1: 10.0.1.0/24 (254 IPs)
Host-2: 10.0.2.0/24 (254 IPs)
Host-3: 10.0.3.0/24 (254 IPs)
```

**Benefits:**
- No IP conflicts between hosts
- Fast local allocation
- Survives network partitions
- Simple garbage collection via lease expiry

### State Management with Embedded etcd

```yaml
# Each host runs embedded etcd
state:
  type: embedded-etcd
  data_dir: /var/lib/banyan/etcd
  initial_cluster: "host-1=http://10.1.1.1:2380,host-2=http://10.1.1.2:2380"
```

**What's stored:**
- Subnet leases (which host owns which /24)
- Service registry (name → IPs mapping)
- Security rules
- Health status

## User Experience

### Auto-Config Generation

```bash
# Automatically generate banyan.yaml from docker-compose
banyan init --from docker-compose.yml

# Detects service dependencies and generates:
services:
  web:
    network:
      allow:
        - from: internet
          to_port: 443
      egress: all  # Default: allow all outbound

  database:
    network:
      allow:
        - from: service:web
          to_port: 5432
      egress: internal  # Database typically needs only internal
```

### What Users Configure (Simple)

```yaml
services:
  web:
    network:
      allow:
        - from: internet
          to_port: 443

  database:
    network:
      allow:
        - from: service:web
          to_port: 5432
```

### Power Users (Advanced)

```yaml
# Phase 2: Select different CNI backends
networks:
  production:
    driver: calico  # For BGP routing at scale

# Phase 2: Egress control
services:
  database:
    network:
      egress:
        - to: cidr:10.0.0.0/8
        - to: domain:*.amazonaws.com
```

## Risk Mitigation

### Technical Risks

| Risk | Mitigation |
|------|------------|
| CNI plugin compatibility | Start with Flannel, well-tested and simple |
| State management complexity | Use embedded etcd with hierarchical IPAM |
| Performance overhead | Profile early, Flannel is proven at scale |
| Debugging difficulties | Build debug tools from day one |

### Design Risks

| Risk | Mitigation |
|------|------------|
| Wrong abstractions | Study CNI spec early, design for it |
| Leaky abstractions | Keep provider interface minimal |
| Feature creep | Focus on core networking only |

## Implementation Checklist

### Phase 1: Flannel CNI + Core Features
- [x] CNI runtime integration with Flannel
- [x] Hierarchical IPAM implementation
- [ ] Security rules translator (allow → iptables)
- [ ] Embedded etcd for state management
- [ ] DNS integration (CoreDNS)
- [ ] Auto-config from docker-compose (`banyan init`)
- [ ] Debug tools (`banyan network trace`)
- [x] Integration testing framework

### Phase 2: Advanced Features
- [ ] Calico CNI integration
- [ ] Egress control rules
- [ ] Observability (metrics, flow logs)
- [ ] Performance benchmarking
- [ ] Multi-region support

## Next Steps

1. Set up Flannel CNI in standalone mode
2. Implement hierarchical IPAM with lease management
3. Build security rule translator
4. Integrate embedded etcd cluster
5. Create `banyan init` command for auto-config
6. Build debug tools from day one

## Summary

By starting with Flannel CNI from day one, Banyan gets proven networking while maintaining simplicity for users. The hierarchical IPAM design solves state management elegantly, and auto-config from docker-compose ensures easy adoption. Users see simple `allow` rules while we leverage battle-tested CNI plugins underneath.