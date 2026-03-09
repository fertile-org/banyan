# [HIGH-009] Authentication Failures Not Logged

**Status**: FIXED (2026-03-07) — Added `auditLogInterceptor()` and `auditLogStreamInterceptor()` to engine gRPC server. Unknown peers logged at Warn level with method and peer IP. Failed RPCs logged at Debug level. Agent name mismatch in Register also logged.
**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Authentication
**File(s)**:
- `pkg/rpc/auth.go:64-68` (`Validate` — returns error but never logs)
- `pkg/rpc/auth.go:74-84` (unary interceptor — no logging on failure)
- `pkg/rpc/auth.go:89-94` (stream interceptor — no logging on failure)

## Description

Failed authentication attempts (invalid public keys, missing metadata, wrong session tokens) return gRPC errors but are never logged server-side. There is no audit trail of unauthorized access attempts.

Successful authentication is also not logged (no audit trail of which keys are used).

## Impact

- **Who**: Security operators, incident responders
- **What they lose**: Visibility into brute-force attempts, unauthorized access attempts, misconfigured agents, and compromised keys
- **Blast radius**: Organizational — attacks go undetected

## Recommendation

Log all auth events at the interceptor level:

```go
// On failure:
logger.Warn("Auth failed", "method", info.FullMethod, "peer", peer.Addr, "reason", err)

// On success (debug level):
logger.Debug("Auth success", "method", info.FullMethod, "agent", agentName)
```

Include the peer IP address for correlation.
