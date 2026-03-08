# [MED-003] Insufficient Manifest Input Validation

**Severity**: Medium
**Responsibility**: Mitigation Gap
**Component**: CLI, Engine, Types
**File(s)**:
- `cmd/banyan-cli/cmd/deploy.go:86-99` (`validateManifest` — minimal checks)
- `pkg/types/manifest.go` (no port/replica/restart validation)

## Description

The CLI `validateManifest` function only checks: name not empty, at least one service, each service has image or build. It does not validate:

1. **Port ranges**: Ports stored as `[]string` (e.g., `"80:80"`) with no format or range validation (1-65535)
2. **Replica count**: No upper bound — `replicas: 999999` is accepted
3. **Restart policy**: Passed directly to `nerdctl run --restart` without validation against valid values
4. **Image references**: No OCI naming validation
5. **Environment variable keys**: No POSIX naming validation

## Impact

Invalid or extreme values pass through to agents, causing runtime errors or resource exhaustion instead of early validation errors.

## Recommendation

Add a `Validate()` method to the manifest type that checks all fields:

```go
func (m *Manifest) Validate() error {
    // Port format: "host:container" with valid ranges
    // Replicas: 1-100 (configurable max)
    // Restart: no, always, unless-stopped, on-failure, on-failure:N
    // Image: OCI reference format
    // Env keys: ^[A-Za-z_][A-Za-z0-9_]*$
}
```
