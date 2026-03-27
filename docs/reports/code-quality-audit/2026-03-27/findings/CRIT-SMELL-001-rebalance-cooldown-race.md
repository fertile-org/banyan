# CRIT-SMELL-001: Race condition — rebalanceMigrationCooldown map unprotected

**Severity**: Critical
**Category**: SMELL
**Component**: pkg/engine/autoscale.go
**File(s)**: `pkg/engine/autoscale.go:203, 282-284, 302, 336`

## Description

`rebalanceMigrationCooldown` is a package-level `map[string]time.Time` accessed from the engine's run loop without synchronization. Map operations (read, write, delete) in `evaluateRebalance()` are concurrent with the main scheduling goroutine.

## Evidence

```go
var rebalanceMigrationCooldown = make(map[string]time.Time)  // line 203

// Used in evaluateRebalance():
for name, t := range rebalanceMigrationCooldown { ... }      // line 282
delete(rebalanceMigrationCooldown, name)                      // line 284
rebalanceMigrationCooldown[candidate.ContainerName] = ...     // line 336
```

Go's race detector will flag this. In production, concurrent map access causes a panic: "concurrent map read and map write".

## Impact

Engine crash during rebalancing evaluation if map is accessed concurrently. Low probability (rebalance runs every 60s) but catastrophic when it hits — engine restarts, interrupting all scheduling.

## Recommendation

Add sync.Mutex:
```go
var (
    rebalanceMigrationCooldown   = make(map[string]time.Time)
    rebalanceMigrationCooldownMu sync.Mutex
)
```
Lock around all map operations in evaluateRebalance().
