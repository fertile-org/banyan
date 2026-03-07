# [HIGH-DOC-002] env_file Feature Missing From Manifest Reference

**Severity**: High
**Category**: DOC
**Component**: website/docs, pkg/types
**File(s)**: `website/src/content/docs/reference/manifest.md`, `pkg/types/manifest.go:20`, `pkg/types/envfile.go`

## Description

The `env_file` manifest field is fully implemented and production-ready but is not documented in the manifest reference. Users can only discover it by reading the roadmap or blog post.

## Evidence

**Code implementation exists:**
- `pkg/types/manifest.go:20`: `EnvFile EnvFile ...yaml:"env_file,omitempty"`
- `pkg/types/envfile.go`: Complete implementation with `ParseEnvFile()` and `ResolveEnvFiles()`
- `cmd/banyan-cli/cmd/deploy.go:146`: Calls `types.ResolveEnvFiles()`

**Documented in non-reference locations:**
- `website/src/content/docs/roadmap.md` (Milestone 5 — Production Readiness, marked Done)

**Missing from:**
- `website/src/content/docs/reference/manifest.md` — the canonical field reference

## Impact

- **User impact**: Users cannot discover `env_file` from the manifest reference. Must read roadmap to find it.
- **Adoption impact**: Feature goes unused because users don't know it exists.

## Recommendation

Add `env_file` to the Service section of `reference/manifest.md` with type signature, default, Docker Compose compatibility notes, example YAML, and merge order documentation (env_file loaded first, inline environment overrides).
