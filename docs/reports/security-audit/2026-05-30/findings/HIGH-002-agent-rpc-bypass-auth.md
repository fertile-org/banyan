# [HIGH-002] Agent RPCs bypass auth entirely (WireGuard tunnel assumed)

**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Authentication, gRPC Server
**File(s)**: `pkg/engine/auth/roles.go:80-93`, `pkg/engine/grpc_server.go:178-181`

## Description

Agent RPCs (Register, Heartbeat, PollTasks, ReportTaskResult, ReportContainerHealth) are excluded from the auth interceptor entirely, relying on WireGuard tunnel IP for authentication. This is a defense-in-depth gap.

## Evidence

**Auth bypass map** (`pkg/engine/auth/roles.go:80-93`):
```go
var bypassMethods = map[string]bool{
    "/banyan.v1.EngineService/Register":              true,
    "/banyan.v1.EngineService/Heartbeat":             true,
    "/banyan.v1.EngineService/PollTasks":             true,
    "/banyan.v1.EngineService/ReportTaskResult":      true,
    "/banyan.v1.EngineService/ReportContainerHealth": true,
    // ...
}
```

**No auth interceptor when no keys** (`pkg/engine/grpc_server.go:178-181`):
```go
if opts.AuthDeps != nil {
    unaryInterceptors = append(unaryInterceptors, auth.UnaryAuthInterceptor(opts.AuthDeps))
    // ...
}
```

The comment at `roles.go:81` says "authenticated by WireGuard tunnel IP" — meaning the assumption is that only WireGuard peers can reach the engine's tunnel IP. However, if an attacker finds another path to the engine (e.g., multi-engine with `AllowInsecure`), agent RPCs have zero authentication.

## Impact

**Who can exploit**: Network attacker who can reach the engine's gRPC port without WireGuard (e.g., misconfigured multi-engine or adjacent network).

**What they gain**: 
- Register a fake agent and receive tasks
- Report false health metrics
- Submit fake task results

**Blast radius**: Cluster integrity — fake agents can receive and execute workloads.

## Recommendation

1. Require authentication even for agent RPCs — the WireGuard tunnel IP is a nice-to-have, not a security boundary.
2. If performance is a concern, use a lightweight token (not full JWT) for agent RPCs.
3. Ensure `AllowInsecure` flag also disables agent RPC bypass.

## Secure Default Consideration

**Checklist A6**: "Auth required on every gRPC endpoint — ENFORCE — No endpoint should be reachable without authentication."

Agent RPCs are the only RPCs that bypass auth. Even if WireGuard is the intended security boundary, explicit auth should still be enforced as defense-in-depth.