# [HIGH-SMELL-003] Function With 7 Parameters

**Severity**: High
**Category**: SMELL
**Component**: pkg/agent
**File(s)**: `pkg/agent/engine_client.go:60`

## Description

`EngineClient.Register()` accepts 7 parameters, making it difficult to use, test, and extend.

## Evidence

```go
func (c *EngineClient) Register(ctx context.Context, name, apiAddr, sessionToken string,
    tags []string, wgPublicKey, hostIP string) (string, *VPCConfig, []ActiveContainer, error)
```

## Impact

- **Maintenance**: Adding a new registration field requires changing the function signature and all callers
- **Readability**: Call sites are hard to understand without named parameters
- **Testing**: Tests must construct long argument lists

## Recommendation

Bundle parameters into a `RegisterRequest` struct:
```go
type RegisterRequest struct {
    Name         string
    APIAddr      string
    SessionToken string
    Tags         []string
    WGPublicKey  string
    HostIP       string
}
func (c *EngineClient) Register(ctx context.Context, req *RegisterRequest) (...)
```
