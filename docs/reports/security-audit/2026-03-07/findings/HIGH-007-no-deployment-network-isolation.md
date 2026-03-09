# [HIGH-007] No Deployment Network Isolation — Flat Overlay + Dead Security Code

**Status**: FIXED (2026-03-08) — Deployment-level network isolation enforced via iptables. Each deployment gets its own iptables chain (`BN-DEP-<name>`). Containers within the same deployment can communicate; cross-deployment traffic is blocked by default. Rules are reconciled on every heartbeat cycle (~15s) using the complete set of service backends from the engine. Works cross-agent: each agent builds isolation rules from the cluster-wide backend list received via heartbeat.
**Severity**: High
**Responsibility**: Platform Issue
**Component**: VPC Networking, Security Rules
**File(s)**:
- `pkg/agent/vpc_networking.go` — `setupOverlayForwarding()` creates `BANYAN-ISOLATION` chain with jump rules; `reconcileNetworkIsolation()` rebuilds per-deployment chains on each heartbeat
- `pkg/agent/agent.go` — calls `reconcileNetworkIsolation()` from heartbeat loop
- `pkg/engine/grpc_server.go` — `collectServiceBackends()` populates `DeploymentName` for each backend
- `pkg/rpc/proto/banyan/v1/engine.proto` — `ServiceBackend.deployment_name` field

## Description

All containers across all deployments share a single flat overlay network (`banyan` CNI network on `banyan0` bridge). Previously, the iptables FORWARD rules accepted ALL traffic between bridge and WireGuard interfaces with no isolation between deployments.

## Fix

Replaced blanket ACCEPT rules with deployment-scoped isolation:

1. **`BANYAN-ISOLATION` chain** — Created during overlay setup, jumped to from FORWARD for `banyan0 ↔ banyan-wg` traffic
2. **Per-deployment chains** (`BN-DEP-<name>`) — Created per deployment, allow traffic only to IPs within the same deployment
3. **Default deny** — Unknown sources and cross-deployment destinations are DROPped
4. **Conntrack** — Established/related connections are always allowed
5. **DNS passthrough** — Containers can always reach the gateway IP for DNS resolution
6. **Cross-agent support** — Each agent receives all backends cluster-wide via heartbeat, so isolation rules cover containers on any agent

### Chain structure

```
FORWARD:
  -i banyan0 -o banyan-wg -j BANYAN-ISOLATION
  -i banyan-wg -o banyan0 -j BANYAN-ISOLATION

BANYAN-ISOLATION:
  -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  -d <gateway_ip> -p udp --dport 53 -j ACCEPT
  -d <gateway_ip> -p tcp --dport 53 -j ACCEPT
  -s <ip> -j BN-DEP-<deployment>     (per container)
  -j DROP                              (unknown source)

BN-DEP-<deployment>:
  -d <ip> -j ACCEPT                    (same deployment)
  -j DROP                              (cross-deployment)
```

## Limitations

- **Same-host isolation**: Containers on the same bridge (`banyan0`) communicate at L2 without going through iptables FORWARD. Same-host, cross-deployment traffic is not isolated. This matches Docker's default behavior and can be improved later with `br_netfilter`.
