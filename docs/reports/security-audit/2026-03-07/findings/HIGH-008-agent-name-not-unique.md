# [HIGH-008] Agent Name Uniqueness Not Enforced

**Status**: FIXED (2026-03-07) — Register handler now enforces that `req.AgentName` matches the caller's WireGuard tunnel IP identity (resolved via `tunnelIPToAgent` map). An agent cannot claim a name that doesn't match its whitelisted key. Also added `types.ValidateName()` check on agent names.
**Severity**: High
**Responsibility**: Platform Issue
**Component**: Engine — Agent Registration
**File(s)**:
- `pkg/engine/grpc_server.go:118-143` (Register handler)
- `pkg/engine/grpc_server.go:127` (session token overwrite)

## Description

The `Register` RPC saves the node record keyed by `req.AgentName`. If two agents register with the same name, the second silently overwrites the first's record — including session token, API address, allocated subnet, and host IP.

There is no check for existing registrations or duplicate names.

## Impact

- **Who**: Any authenticated client (or unauthenticated if auth is disabled — CRIT-003)
- **What they gain**: Hijack an existing agent — steal its tasks, overwrite its session token (locking out the real agent from log streaming), claim its VPC subnet
- **Blast radius**: Per-agent — one agent's identity is fully compromised

## Recommendation

Check for existing active agents with the same name during registration. Reject duplicates unless the registration comes from the same public key (re-registration after restart).

```go
existing, err := s.store.GetNode(req.AgentName)
if err == nil && existing.PublicKey != callerPublicKey {
    return nil, status.Errorf(codes.AlreadyExists, "agent %q already registered", req.AgentName)
}
```
