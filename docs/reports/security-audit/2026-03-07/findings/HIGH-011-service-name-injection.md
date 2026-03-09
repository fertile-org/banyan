# [HIGH-011] Service/App Names Not Sanitized — Injection Risk

**Status**: FIXED (2026-03-07) — Added `types.ValidateName()` that enforces DNS-safe pattern (`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`, max 63 chars). Validation applied in both CLI `validateManifest()` and engine `Deploy` handler. Agent names also validated in `Register` handler.
**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Manifest Parsing, Engine
**File(s)**:
- `pkg/types/manifest.go` (no name validation)
- `cmd/banyan-cli/cmd/deploy.go:86-99` (`validateManifest` — name only checked for emptiness)
- `pkg/engine/grpc_server.go` (names used in etcd keys, container names)

## Description

Service names and application names from the manifest are used as-is in:
- Container names (e.g., `appname-servicename-0`)
- DNS names (`servicename.internal`)
- etcd key paths (`/banyan/deployments/...`)
- gRPC responses

There is no validation that names match a safe pattern (e.g., `^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`). A service name containing special characters (slashes, dots, spaces, null bytes) could cause:
- DNS record injection (e.g., `web.evil.com` as a service name)
- etcd key path traversal (e.g., `../` patterns)
- Container naming violations
- Log injection

## Impact

- **Who**: Any user who can deploy (authenticated or unauthenticated if CRIT-003 applies)
- **What they gain**: DNS poisoning, etcd key manipulation, potential container name collision
- **Blast radius**: Cluster DNS, etcd state

## Recommendation

Add a validation function for names used in infrastructure:

```go
func ValidateName(name string) error {
    if !regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`).MatchString(name) {
        return fmt.Errorf("invalid name %q: must be lowercase alphanumeric with hyphens", name)
    }
    if len(name) > 63 { // DNS label limit
        return fmt.Errorf("name %q too long: max 63 characters", name)
    }
    return nil
}
```

Apply to both `manifest.Name` and all service names before processing.
