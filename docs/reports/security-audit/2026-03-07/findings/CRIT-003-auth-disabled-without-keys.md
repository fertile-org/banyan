# [CRIT-003] Authentication Disabled When No Whitelisted Keys Configured

**Status**: FIXED (2026-03-07) — Engine now refuses to start without whitelisted keys unless `--allow-insecure` is explicitly set. Both `Engine.Run()` and `startEngineGRPC()` enforce the check. Warning logged in insecure mode.
**Severity**: Critical
**Responsibility**: Platform Issue
**Component**: Engine gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go:83-92`
- `pkg/engine/engine.go:109` (warning log)

## Description

When `len(opts.WhitelistedKeys) == 0`, the engine creates `grpc.NewServer()` with no authentication interceptors at all. A warning is logged, but the server starts and accepts all connections unauthenticated.

```go
// pkg/engine/grpc_server.go:83-92
if len(opts.WhitelistedKeys) > 0 {
    validator := rpc.NewPublicKeyValidator(opts.WhitelistedKeys)
    srv = grpc.NewServer(
        grpc.UnaryInterceptor(rpc.UnaryPublicKeyAuthInterceptor(validator)),
        grpc.StreamInterceptor(rpc.StreamPublicKeyAuthInterceptor(validator)),
    )
} else {
    srv = grpc.NewServer()  // NO AUTH
}
```

## Impact

- **Who**: Anyone who can reach the engine gRPC port (50051 on `0.0.0.0`)
- **What they gain**: Full cluster control — deploy arbitrary containers, stop running deployments, read all deployment data including environment variables (secrets), register as a fake agent, access logs
- **Blast radius**: Entire cluster — every node, every deployment, every secret

This is exploitable with a single `grpcurl` command from any machine on the network.

## Evidence

The engine's `Run()` function logs a warning but does not prevent startup:
```go
// pkg/engine/engine.go:109
e.logger.Warn("No whitelisted public keys configured. gRPC auth is DISABLED.")
```

## Recommendation

**Refuse to start without authentication.** The engine should fail with a clear error:
```
Error: No whitelisted public keys configured. Cannot start without authentication.
Add keys with: banyan-engine init
Or use --allow-insecure for development only.
```

If a development/test mode is needed, require an explicit `--allow-insecure` flag (not just the absence of keys).

## Secure Default Consideration

- **Action**: ENFORCE — the engine must not start without at least one whitelisted key
- **Override**: `--allow-insecure` flag with a prominent warning for development only
- **Warning**: Print "SECURITY WARNING: Running without authentication. Do NOT use in production." on every log line if insecure mode is forced
