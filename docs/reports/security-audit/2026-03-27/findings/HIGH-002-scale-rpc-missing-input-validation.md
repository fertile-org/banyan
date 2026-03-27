---
# HIGH-002: Scale RPC accepts negative/unbounded replica counts

**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: gRPC Server — Scale Handler
**File(s)**: `pkg/engine/grpc_server.go:1035-1155`

## Description

The Scale RPC handler accepts `int32` replica counts without validating bounds. Unlike the Deploy handler (which checks `MaxReplicas`), Scale does not:
- Reject negative values
- Enforce an upper bound
- Validate against `MaxReplicas` constant

```go
for svcName, targetReplicas := range req.Replicas {
    // targetReplicas is int32 — can be negative, can exceed MaxReplicas
    // NO validation here
}
```

## Impact

- **Who**: Any CLI user or automated tool with cluster access
- **What**: Sending `scale my-app api=-1` could cause undefined loop behavior. Sending `scale my-app api=100000` could exhaust cluster resources.
- **Blast radius**: Entire cluster (resource exhaustion) or service disruption (negative counts)

## Evidence

The Deploy handler at `pkg/engine/grpc_server.go:735` validates `svc.Deploy.Replicas > types.MaxReplicas` but the Scale handler at line 1057 has no equivalent check. The CLI validates `count < 0` at `cmd/banyan-cli/cmd/scale.go:44` but gRPC is the trust boundary, not the CLI.

## Recommendation

Add validation at the start of the Scale RPC handler:
```go
if targetReplicas < 0 {
    return nil, status.Errorf(codes.InvalidArgument, "replica count for %q must be >= 0", svcName)
}
if targetReplicas > types.MaxReplicas {
    return nil, status.Errorf(codes.InvalidArgument, "replica count for %q exceeds maximum (%d)", svcName, types.MaxReplicas)
}
```
---
