# [MED-CONS-002] Logging Pattern Inconsistency

**Status**: ACKNOWLEDGED — Both patterns (function-scoped vs method-based) work correctly with slog. Deferring broader standardization to avoid churn.
**Severity**: Medium
**Category**: CONS
**Component**: pkg/agent, pkg/engine
**File(s)**: `pkg/agent/grpc_server.go:28`, `pkg/engine/engine.go`

## Description

Logger creation and usage patterns differ between packages:

- Agent gRPC server creates logger inline with `logging.New()`
- Engine uses `s.logger()` method pattern
- Field names in log statements vary: sometimes `"node"`, sometimes `"agent"`, sometimes `"nodeName"`

## Evidence

```go
// Agent: inline creation
log := logging.New("agent-grpc")
log.Error("Failed to listen", "port", port, "error", err)

// Engine: method-based
s.logger().Info("Agent registered", "agent", req.AgentName)
```

## Impact

- Inconsistent log field names make log aggregation and searching harder
- Different logger creation patterns make it unclear which is "correct"
- Structured logging loses value if field names aren't standardized

## Recommendation

1. Standardize on dependency injection (pass logger to struct) or method-based (`s.logger()`)
2. Create a logging convention doc defining standard field names
3. Use consistent field names: always `"agent"` (not `"node"` or `"nodeName"`)
