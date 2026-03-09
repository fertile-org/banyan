# [MED-006] gRPC Error Messages Leak Internal Details

**Status**: WONTFIX — Detailed error messages kept for debuggability. Security boundary is WireGuard (only authenticated peers can see errors), not error obscurity.
**Severity**: Medium
**Responsibility**: Platform Issue
**Component**: Engine gRPC Server, Agent gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go:139,153,240,300,371,400,526,606,911`
- `pkg/agent/grpc_server.go:66`

## Description

Error responses use `status.Errorf(codes.Internal, "failed to ...: %v", err)`, which includes the underlying Go error message. These can contain etcd connection details, file paths, nerdctl error output, or other internal information.

Example: `"failed to register node: context deadline exceeded: etcd endpoint http://127.0.0.1:2379 unreachable"`

## Impact

Aids attacker reconnaissance by revealing internal architecture (etcd endpoints, file paths, runtime details).

## Recommendation

Log the full error server-side, return generic messages to clients:

```go
s.logger().Error("Failed to register node", "err", err)
return nil, status.Errorf(codes.Internal, "registration failed")
```
