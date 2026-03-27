# HIGH-SMELL-002: Agent staleness timeout duplicated across 3 files

**Severity**: High
**Category**: SMELL
**Component**: pkg/engine
**File(s)**: `pkg/engine/grpc_server.go:1821`, `pkg/engine/engine.go:402`, `pkg/engine/autoscale.go:247`

## Description

The 60-second agent staleness threshold appears as a magic number `60*time.Second` in 3 separate files. Similarly, `30*time.Second` deployment lock timeout appears in engine.go lines 550 and 631.

## Evidence

```go
// Three places checking if agent is stale:
if time.Since(node.LastSeen) > 60*time.Second  // grpc_server.go:1821
if time.Since(node.LastSeen) > 60*time.Second  // engine.go:402
if time.Since(node.LastSeen) > 60*time.Second  // autoscale.go:247
```

## Impact

Changing the staleness threshold requires finding and updating 3 files. Easy to miss one, creating inconsistent behavior.

## Recommendation

Define constants:
```go
const (
    AgentStalenessThreshold = 60 * time.Second
    DeploymentLockTimeout   = 30 * time.Second
)
```
