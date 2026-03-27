# MED-CONS-001: "Agent" vs "Node" naming inconsistency

**Severity**: Medium
**Category**: CONS
**Component**: pkg/types, pkg/engine
**File(s)**: `pkg/types/records.go:126` (NodeRecord), `pkg/engine/grpc_server.go:304` (AgentName)

## Description

The RPC layer uses "agent" terminology (`req.AgentName`, `agentNameFromContext`) while the storage layer uses "node" (`NodeRecord`, `KeyNodes`, `node.Name`). Both refer to the same concept — a machine running banyan-agent.

## Impact

Developer confusion when tracing code from RPC handler to storage. Not a bug, but increases cognitive load.

## Recommendation

Acceptable as-is for v1. If refactoring, rename `NodeRecord` to `AgentRecord` and `KeyNodes` to `KeyAgents` for consistency with the user-facing terminology.
