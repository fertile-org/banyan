# [HIGH-SMELL-003] Function With 7 Parameters

**Status**: FIXED (2026-03-06) — Refactored to use RegisterRequest struct
**Severity**: High
**Category**: SMELL
**Component**: pkg/agent
**File(s)**: `pkg/agent/engine_client.go:60`

## Description

`EngineClient.Register()` takes 7 parameters, exceeding the recommended maximum of 5. This makes the function difficult to call correctly and hard to extend.

## Evidence

```go
func (c *EngineClient) Register(ctx context.Context, name, apiAddr, sessionToken string,
    tags []string, wgPublicKey, hostIP string) (string, *VPCConfig, []ActiveContainer, error)
```

7 parameters: ctx, name, apiAddr, sessionToken, tags, wgPublicKey, hostIP

## Impact

- Easy to swap parameter positions (e.g., `sessionToken` and `apiAddr` are both strings)
- Adding new registration fields requires changing every call site
- Hard to read and maintain

## Recommendation

Bundle related parameters into a `RegisterRequest` struct:
```go
type RegisterRequest struct {
    Name         string
    APIAddr      string
    SessionToken string
    Tags         []string
    WGPublicKey  string
    HostIP       string
}
```
