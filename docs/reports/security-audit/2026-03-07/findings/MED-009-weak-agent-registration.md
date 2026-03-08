# [MED-009] Weak Agent Registration Validation

**Severity**: Medium
**Responsibility**: Platform Issue
**Component**: Engine gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go:160-161` (self-reported host IP)
- `pkg/engine/grpc_server.go:222-225` (heartbeat overwrites session token)
- `pkg/engine/grpc_server.go:229-235` (heartbeat creates phantom nodes)

## Description

Three related issues in agent registration/heartbeat:

1. **Self-reported host IP**: `agentHostIP(req.HostIp, ctx)` prefers the agent-reported IP over the gRPC connection IP. A malicious agent can claim any host IP, which is distributed to all other agents as a WireGuard endpoint.

2. **Session token overwrite on heartbeat**: The heartbeat handler unconditionally stores the session token from the request (line 224), replacing the existing one. Combined with HIGH-002, an agent can overwrite another agent's token.

3. **Heartbeat creates phantom nodes**: If a heartbeat arrives for an unregistered agent, the handler creates a new node record instead of rejecting it (line 229-235). An attacker can populate the cluster state with phantom agents.

## Recommendation

1. Use the gRPC peer address as the authoritative host IP; only use self-reported IP as a fallback
2. Only update session tokens on Register, not Heartbeat
3. Reject heartbeats from unregistered agents
