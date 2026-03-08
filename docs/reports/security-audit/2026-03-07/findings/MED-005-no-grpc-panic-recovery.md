# [MED-005] No Panic Recovery in gRPC Handlers

**Status**: FIXED (2026-03-07) — Added `panicRecoveryInterceptor()` and `panicRecoveryStreamInterceptor()` to engine gRPC server. Panics are caught, logged with method name, and return `codes.Internal` without leaking details.
**Severity**: Medium
**Responsibility**: Mitigation Gap
**Component**: Engine gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go:62-108` (server setup — no recovery interceptor)

## Description

The gRPC server has no panic recovery interceptor. An unhandled panic in any RPC handler crashes the entire engine process. While systemd `Restart=on-failure` would restart it, repeated panics cause service flapping.

## Impact

A crafted request that triggers a nil pointer dereference or index out-of-range in any handler can crash the engine. This is a denial-of-service vector.

## Recommendation

Add the `grpc_recovery` interceptor from `go-grpc-middleware`:

```go
import grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"

recovery := grpc.ChainUnaryInterceptor(
    grpc_recovery.UnaryServerInterceptor(),
    rpc.UnaryPublicKeyAuthInterceptor(validator),
)
```
