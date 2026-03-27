# MED-003: Autoscale min/max not validated at deploy time

**Severity**: Medium
**Responsibility**: Mitigation Gap
**Component**: gRPC Server — Deploy Handler
**File(s)**: `pkg/engine/grpc_server.go:745` (Deploy), `pkg/engine/autoscale.go:36`

## Description

The Deploy handler validates `deploy.replicas` against MaxReplicas but does not validate `deploy.autoscale.min` or `deploy.autoscale.max`. Invalid values (negative min, min > max, max > MaxReplicas) are accepted silently.

## Impact

- **Who**: Any deployer
- **What**: Undefined autoscale behavior
- **Blast radius**: One deployment

## Recommendation

Add validation: reject `min < 0`, `max < 1`, `min > max`, `max > MaxReplicas`. Return actionable error message.
