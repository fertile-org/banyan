# [HIGH-004] No Rate Limiting on Authentication Endpoints

**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Authentication
**File(s)**:
- `pkg/rpc/auth.go` (interceptors — no rate limiting)
- `pkg/engine/grpc_server.go:85-92` (server setup — no middleware)

## Description

Neither the auth interceptors nor the gRPC server configuration include any rate limiting, throttling, or connection limits. An attacker can make unlimited authentication attempts without backoff.

## Impact

- **Who**: Network attacker
- **What they gain**: Enumerate valid public keys via brute-force; DoS the engine by flooding auth requests
- **Blast radius**: Engine availability — affects all agents and CLI users

## Recommendation

Add per-IP rate limiting on the gRPC auth interceptor. After N failed attempts (e.g., 10), block the IP for a cooldown period (e.g., 60 seconds). Log rate-limited IPs.
