# LOW-002: Streaming RPCs bypass rate limiting

**Severity**: Low
**Responsibility**: Mitigation Gap
**Component**: gRPC Server — Interceptors
**File(s)**: `pkg/engine/grpc_server.go:167-170`

## Description

Stream interceptors do not include rate limiting. Only `GetLogs` uses streaming, and it requires WireGuard access, limiting the attack surface.

## Impact

A WireGuard-authenticated client could open many concurrent log streams to exhaust engine resources.

## Recommendation

Add connection-count limiting for streaming RPCs or add rate limiting to the stream interceptor chain.
