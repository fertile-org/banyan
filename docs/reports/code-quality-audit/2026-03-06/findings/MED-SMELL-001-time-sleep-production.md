# [MED-SMELL-001] time.Sleep in Production Code

**Severity**: Medium
**Category**: SMELL
**Component**: pkg/agent
**File(s)**: `pkg/agent/vpc_networking.go:227`

## Description

Production code uses `time.Sleep(200 * time.Millisecond)` as a fixed delay after forcefully killing a process, instead of using context-aware timeouts or event-based synchronization.

## Evidence

```go
// vpc_networking.go:227
time.Sleep(200 * time.Millisecond)
```

Additionally, `time.After` is used in select statements in `pkg/agent/agent.go:633,655,684` instead of context-aware deadlines.

## Impact

- **Reliability**: Fixed delays don't adapt to system load
- **Observability**: No way to instrument or configure the delay
- **Cancellation**: `time.Sleep` doesn't respect context cancellation

## Recommendation

Replace with `time.NewTimer` + context awareness, or use a channel-based notification when the process actually terminates.
