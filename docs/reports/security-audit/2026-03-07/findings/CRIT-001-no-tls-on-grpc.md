# [CRIT-001] No TLS on Any gRPC Connection

**Status**: FIXED (2026-03-08) — WireGuard control tunnel is enforced: engine refuses to start without whitelisted keys (unless `--allow-insecure`). All gRPC, registry, and metrics services bind to WireGuard tunnel IP (`10.200.0.1`) when keys are configured, or `127.0.0.1` in insecure mode. WireGuard provides authenticated encryption (ChaCha20-Poly1305) at the network layer — no TLS needed on gRPC itself.
**Severity**: Critical
**Responsibility**: Default Issue
**Component**: gRPC Transport (Engine, Agent, CLI)
**File(s)**:
- `pkg/engine/grpc_server.go:85-92` (server created without TLS)
- `pkg/agent/engine_client.go:24` (`insecure.NewCredentials()`)
- `cmd/banyan-cli/cmd/client.go:27` (`insecure.NewCredentials()`)
- `pkg/engine/agent_client.go:18` (`insecure.NewCredentials()`)
- `pkg/rpc/auth.go:34-36,52-54` (`RequireTransportSecurity()` returns `false`)

## Description

All gRPC connections in the Banyan platform use `insecure.NewCredentials()`:
- CLI to Engine
- Agent to Engine
- Engine to Agent (log streaming)

The gRPC server is created with `grpc.NewServer()` — no TLS certificate is configured. Both credential types (`SessionTokenCredentials` and `PublicKeyCredentials`) explicitly return `false` from `RequireTransportSecurity()`.

## Impact

- **Who**: Any network attacker on the same LAN, VPC, or internet path between components
- **What they gain**: Intercept authentication tokens, session tokens, deployment manifests (including environment variables with secrets like `DATABASE_PASSWORD`, `API_KEY`), and container logs
- **Blast radius**: Entire cluster — all communications between all components are unprotected

The WireGuard control tunnel encrypts traffic at the network layer when active. However:
1. WireGuard is optional — the system falls back to direct TCP without warning
2. The agent gRPC server (log streaming) binds to the data-plane IP, which may not be on the WireGuard tunnel
3. No defense-in-depth — if the tunnel is misconfigured, there is zero protection

## Evidence

```go
// pkg/agent/engine_client.go:24
conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), ...)

// pkg/rpc/auth.go:34-36
func (c *SessionTokenCredentials) RequireTransportSecurity() bool {
    return false
}
```

## Recommendation

Option A (preferred): Add TLS to the gRPC layer with auto-generated self-signed certificates on init. This provides defense-in-depth alongside WireGuard.

Option B: Enforce WireGuard — refuse to start the engine/agent without WireGuard keys configured. Remove the fallback to direct TCP.

Either way, `RequireTransportSecurity()` should return `true` when TLS is available.

## Secure Default Consideration

- **Default**: TLS should be enabled by default with auto-generated certificates
- **Override**: Allow `--insecure` flag for development/testing with a clear warning
- **Warning**: CLI and agent should print "WARNING: Connection is not encrypted" if TLS is disabled
