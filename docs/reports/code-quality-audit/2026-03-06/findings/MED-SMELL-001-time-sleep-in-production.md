# [MED-SMELL-001] time.Sleep in Production Code

**Status**: FIXED (2026-03-06) — Replaced with process exit polling loop
**Severity**: Medium
**Category**: SMELL
**Component**: pkg/agent
**File(s)**: `pkg/agent/vpc_networking.go:227`

## Description

Production code uses `time.Sleep(200 * time.Millisecond)` as a fixed delay after killing a process, rather than context-aware synchronization.

## Evidence

```go
// pkg/agent/vpc_networking.go:227
time.Sleep(200 * time.Millisecond)
```

Also found `time.After` usage in `pkg/agent/agent.go:633,655,684` within select statements, which doesn't properly respect context cancellation.

## Impact

- Fixed delays add unnecessary latency
- `time.After` in select can leak timers if context is cancelled first
- Not responsive to context cancellation

## Recommendation

Replace with context-aware patterns:
- Use `time.NewTimer` with explicit `Stop()` cleanup
- Or use `context.WithTimeout` wrapping
