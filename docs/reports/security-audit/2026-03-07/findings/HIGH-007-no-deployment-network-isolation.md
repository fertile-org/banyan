# [HIGH-007] No Deployment Network Isolation — Flat Overlay + Dead Security Code

**Severity**: High
**Responsibility**: Platform Issue
**Component**: VPC Networking, Security Rules
**File(s)**:
- `pkg/vpc/overlay/wireguard.go:147-181` (single flat overlay)
- `pkg/agent/vpc_networking.go:63-81` (permissive FORWARD rules)
- `pkg/vpc/security/manager.go` (complete implementation, never called)
- `pkg/vpc/security/iptables.go` (iptables rule translation, never called)

## Description

All containers across all deployments share a single flat overlay network (`banyan` CNI network on `banyan0` bridge). The iptables FORWARD rules accept ALL traffic between bridge and WireGuard interfaces:

```go
// pkg/agent/vpc_networking.go:68-70
{"-i", "banyan0", "-o", "banyan-wg", "-j", "ACCEPT"},
{"-i", "banyan-wg", "-o", "banyan0", "-j", "ACCEPT"},
```

A complete network security implementation exists in `pkg/vpc/security/` with:
- Default-deny model (`AddDefaultDeny()`)
- Per-network iptables chain management
- Rule translation from security rules to iptables commands

**However, `SecurityManager.ApplyRules()` is never called from the agent or engine.** The security module is dead code.

## Impact

- **Who**: Any container in the cluster
- **What they gain**: Unrestricted network access to every other container across all deployments
- **Blast radius**: All deployments — a compromised container in deployment A can attack deployment B's database

## Recommendation

1. Wire `pkg/vpc/security/` into the agent lifecycle — call `ApplyRules()` when deployments are created/updated
2. Apply default-deny between deployments
3. Allow intra-deployment communication by default
4. Allow users to define cross-deployment access rules in the manifest
