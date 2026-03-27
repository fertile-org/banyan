# HIGH-SMELL-001: Autoscale silently ignores store.Save errors

**Severity**: High
**Category**: SMELL
**Component**: pkg/engine/autoscale.go
**File(s)**: `pkg/engine/autoscale.go:155, 177, 186, 396, 423`

## Description

Five `store.Save()` calls in autoscale.go discard errors with `_ = e.store.Save(...)`. These create tasks and update deployment records during scaling and rebalancing — if they fail silently, operations are lost.

## Evidence

```go
_ = e.store.Save(ctx, taskKey, task)           // line 155 — scale-up task
_ = e.store.Save(ctx, taskKey, stopTask)       // line 177 — scale-down task
_ = e.store.Save(ctx, deploymentKey, deployment) // line 186 — deployment update
_ = e.store.Save(ctx, ...stopTask)             // line 396 — migration stop
_ = e.store.Save(ctx, ...startTask)            // line 423 — migration start
```

## Impact

Failed scale operations go unnoticed. The engine believes it scaled a service, but no task was actually created. Users see stale replica counts.

## Recommendation

Log errors instead of discarding:
```go
if err := e.store.Save(ctx, taskKey, task); err != nil {
    e.logger().Error("Failed to create scale task", "task", task.ID, "error", err)
}
```
