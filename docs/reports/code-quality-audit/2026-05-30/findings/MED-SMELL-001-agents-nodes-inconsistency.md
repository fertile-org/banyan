# [MED-SMELL-001] Agent/Node naming inconsistency across codebase

**Severity**: Medium
**Category**: CONS
**Component**: pkg/engine, pkg/rpc, pkg/storage
**File(s)**: Various files across multiple packages

## Description

The codebase uses both "Agent" and "Node" terminology inconsistently across different layers:

- Proto definitions use `agent_name`, `agent_id`
- Storage layer uses `NodeRecord` and `KeyNodes`
- gRPC handlers reference both concepts

This was flagged in the 2026-03-27 audit as [MED-CONS-001](findings/MED-CONS-001-agent-vs-node-naming.md) but remains unresolved.

## Evidence

**Proto (agent.proto)**:
```protobuf
message RegisterRequest {
  string agent_name = 1;
  ...
}
```

**Storage (records.go)**:
```go
type NodeRecord struct {
    Name string
    ...
}
const KeyNodes = ...  // but agents are registered via RegisterRequest
```

**Handler inconsistency**:
```go
// pkg/engine/grpc_handlers_agent.go
func (s *Server) Register(ctx context.Context, req *banyan.RegisterRequest) {

// But tasks reference agent_id:
message TaskRecord {
  string agent_id = 5;
}
```

## Impact

**Developer impact**: Inconsistent naming requires mental translation when reading code. When a developer sees "node" in storage and "agent" in proto, they must understand these refer to the same concept.

**Maintenance impact**: Bug risk — functions that should handle both may miss one naming variant.

## Recommendation

1. **Pick one term**: "Agent" seems preferred since it's used in binary names (`banyan-agent`), proto messages, and CLI commands (`banyan-cli agent`).
2. **Audit the codebase**: Find all occurrences of "Node" that refer to agents and rename them to "Agent".
3. **Update storage records**: `NodeRecord` should become `AgentRecord`, `KeyNodes` should become `KeyAgents`.
4. **Document the decision**: Update component map to reflect the chosen terminology.

This was flagged previously but not fixed. Prioritize fixing this naming inconsistency.