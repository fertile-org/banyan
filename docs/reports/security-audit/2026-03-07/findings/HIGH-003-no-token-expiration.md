# [HIGH-003] No Token Expiration, Rotation, or Revocation

**Status**: FIXED (2026-03-07) — Session tokens removed entirely. The custom session token system (`sync.Map`, `SessionTokenCredentials`, `SessionTokenAuthInterceptor`) was eliminated. Authentication now relies on WireGuard's cryptographic handshake. Agent-to-engine auth is via WireGuard tunnel. Engine-to-agent auth for log streaming is via engine IP verification (agent verifies peer IP is `10.200.0.1`).
**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Authentication — Session Tokens
**File(s)**:
- `pkg/rpc/auth.go` (old: `SessionTokenCredentials`, interceptors — now removed)
- `pkg/engine/grpc_server.go` (old: `sync.Map sessions` — now removed)
- `pkg/agent/agent.go` (old: `sessionToken` field, `crypto/rand` generation — now removed)
- `pkg/agent/grpc_server.go` (old: `SessionTokenAuth` interceptors — replaced with engine IP verification)

## Description

Session tokens were stored in a `sync.Map` with no TTL, no expiration check, and no rotation mechanism. Tokens were created once at agent startup and persisted indefinitely.

## Impact

- **Who**: Attacker who obtained a session token
- **What they gained**: Permanent access to log streaming for that agent
- **Blast radius**: Per-agent

## Resolution

The entire session token system was removed. There are no more application-layer tokens to expire, rotate, or revoke. WireGuard key revocation is handled by removing the key from the engine's whitelist — the tunnel simply cannot be established.

For engine→agent log streaming, the agent now verifies that the caller's IP is the engine's control tunnel IP (`10.200.0.1`) via `verifyEngineIP()`.
