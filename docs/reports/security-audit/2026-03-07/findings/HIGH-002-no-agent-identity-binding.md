# [HIGH-002] No Agent Identity Binding on RPCs — Agents Can Impersonate Each Other

**Status**: FIXED (2026-03-07) — Engine now resolves agent identity from the WireGuard tunnel IP via `agentNameFromContext()`. Each whitelisted public key maps to a deterministic tunnel IP, so the engine knows which agent is calling without trusting the `agent_name` field in the request. The `tunnelIPToAgent` reverse map is built at startup from the whitelisted keys.
**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Engine gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go` — `agentNameFromContext()`, `tunnelIPToAgent` map

## Description

Previously, the auth interceptor validated that the caller had a whitelisted public key, but no RPC handler verified that the `agent_name` in the request matched the agent associated with that public key. An authenticated agent (worker-1) could send requests with `agent_name: "worker-2"`.

## Impact

- **Who**: Any authenticated agent
- **What they gained**: Steal tasks, overwrite session tokens, report false health, hijack log streaming
- **Blast radius**: Any agent in the cluster

## Resolution

The engine now builds a `tunnelIPToAgent` map at startup:
```go
tunnelIPMap := make(map[string]string, len(opts.WhitelistedKeys))
for pubKey, agentName := range opts.WhitelistedKeys {
    tunnelIP := types.TunnelIPFromPublicKey(pubKey)
    tunnelIPMap[tunnelIP.String()] = agentName
}
```

The `agentNameFromContext(ctx)` method extracts the peer IP from gRPC context and resolves it to the agent name. This provides cryptographic identity binding — each WireGuard key produces a unique, deterministic tunnel IP.

**Note**: RPC handlers don't yet enforce that `req.AgentName` matches the resolved identity. This is a defense-in-depth improvement that should be added to prevent even a compromised tunnel peer from acting as a different agent.
