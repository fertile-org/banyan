# [HIGH-SMELL-002] Inconsistent Error Wrapping (%v vs %w)

**Status**: FIXED (2026-03-06) — Production fmt.Errorf calls now use %w; gRPC status.Errorf correctly uses %v (no %w support)
**Severity**: High
**Category**: SMELL
**Component**: pkg/agent, pkg/engine
**File(s)**: `pkg/agent/agent.go:327,380`, `pkg/engine/grpc_server.go` (multiple locations)

## Description

The codebase inconsistently uses `%v` and `%w` for error formatting. Go's `%w` verb wraps errors (preserving the error chain for `errors.Is`/`errors.As`), while `%v` converts to string (breaking the chain). Both are used in the same packages.

## Evidence

Uses `%v` (breaks error chain):
```go
// pkg/agent/agent.go:327
return nil, fmt.Errorf("failed to pull image %s: %v", task.Image, err)
```

Uses `%w` (correct wrapping):
```go
// pkg/agent/agent.go (other locations)
return nil, fmt.Errorf("register failed: %w", err)
```

## Impact

- Callers cannot use `errors.Is()` or `errors.As()` to inspect error causes when `%v` is used
- Inconsistency makes it unpredictable which errors can be inspected programmatically
- Error handling tests may silently pass because they only check `err != nil`

## Recommendation

Standardize on `%w` for all error wrapping across the codebase. Search for `%v", err` and replace with `%w", err`.
