# Change Audit — Resource-Aware Scheduling (Milestone 6)

**Date**: 2026-03-10
**Branch**: `feat/code-coverage`
**Scope**: Milestone 6 changes — resource-aware scheduling, capacity validation, agent metrics storage

## Files Reviewed

| File | Change | Security-relevant |
|------|--------|-------------------|
| `pkg/types/resources.go` | NEW — Resource parsing (ParseMemoryBytes, ParseCPU, ServiceResourceRequest) | Yes — parses user input from manifests |
| `pkg/types/resources_test.go` | NEW — 23 test cases | No |
| `pkg/types/records.go` | Added 4 metric fields to NodeRecord | Yes — new data stored in etcd |
| `pkg/types/helpers.go` | Resource-aware scheduling (BuildTasksForDeployment, pickAgentByResources, ValidateClusterCapacity) | Yes — scheduling logic uses agent-reported data |
| `pkg/types/helpers_test.go` | Added 15 test cases | No |
| `pkg/engine/grpc_server.go` | Heartbeat stores SystemMetrics on NodeRecord | Yes — trusts agent-reported data |
| `pkg/engine/engine.go` | Capacity validation before scheduling | Minor — error message content |
| `test/e2e/run-e2e.sh` | Added Phase 2d resource scheduling tests | No |
| `test/e2e/examples/banyan.yaml` | Manifest unchanged (resource limits removed due to cgroup/DinD) | No |
| `website/src/content/docs/` | Documentation updates | No |

## Security Impact Assessment

### 1. Authentication / Authorization — No change
No new gRPC endpoints added. The Heartbeat RPC is already authenticated via WireGuard tunnel. No authorization changes.

### 2. User input handling — Safe
`ParseMemoryBytes` and `ParseCPU` parse manifest strings using `strconv.ParseFloat`. No shell interpolation, no command execution. Negative values are rejected. Invalid input falls back to safe defaults (512MB, 1 CPU). The manifest comes from an authenticated CLI user who already has deploy privileges.

### 3. Agent-reported data — Finding LOW-001
The engine trusts `SystemMetrics` from agent heartbeats without validation. A compromised agent could manipulate scheduling by reporting false metrics. See [LOW-001](findings/LOW-001-agent-metrics-trusted-without-validation.md).

### 4. Error messages — Safe
`ValidateClusterCapacity` error messages contain deployment name, requested memory, total cluster memory, and agent count. This information is already available to authenticated users via `banyan-cli agent` and `banyan-cli engine`. No information leakage beyond existing access.

### 5. New etcd data — Safe
Four new fields on NodeRecord (`memory_total_bytes`, `memory_used_bytes`, `cpu_cores`, `cpu_usage_ratio`). This is operational data, not credentials or secrets. The etcd store is already protected by the WireGuard tunnel (localhost-only access).

### 6. Integer overflow — Noted in LOW-001
`pickAgentByResources` casts `uint64` metrics to `int64` for subtraction. Values exceeding `math.MaxInt64` would wrap negative. This is only exploitable by a compromised agent reporting absurd values, and the worst outcome is suboptimal scheduling. See LOW-001 recommendation for bounds checking.

## Findings Summary

| Severity | Count | Type |
|----------|-------|------|
| Critical | 0 | — |
| High | 0 | — |
| Medium | 0 | — |
| Low | 1 | [LOW-001](findings/LOW-001-agent-metrics-trusted-without-validation.md) — Agent metrics trusted without validation — **FIXED** |
| Info | 1 | [INFO-001](findings/INFO-001-capacity-validation-uses-total-not-available.md) — Capacity validation uses total, not available memory |

## Checklist (from threat model)

- [x] All gRPC endpoints go through auth interceptor — no new endpoints added
- [x] No new unauthenticated HTTP endpoints
- [x] File permissions unchanged
- [x] TLS/WireGuard unchanged
- [x] No new CLI flags that accept secrets
- [x] New etcd keys store only operational metrics (no secrets)
- [x] Container execution arguments unchanged
- [x] Install script unchanged
- [x] Default values secure (512MB/1CPU defaults are reasonable scheduling assumptions)
- [x] Error messages don't leak sensitive information

## Conclusion

The resource-aware scheduling changes are **low risk**. The primary security surface is agent-reported metrics being trusted without bounds validation (LOW-001), which is a minor extension of the existing trust relationship with agents. No critical, high, or medium findings. The code handles user input safely and doesn't introduce new attack vectors beyond what already exists in the agent trust model.

### Comparison with prior audit (2026-03-07)
No regression on any previously fixed findings. The 23 findings from the full audit remain in their resolved states. This change does not touch authentication, TLS, registry, or network isolation code.
