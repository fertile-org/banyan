# [HIGH-DOC-001] env_file Feature Missing From Manifest Reference

**Status**: FIXED (2026-03-06)
**Severity**: High
**Category**: DOC
**Component**: website/docs, pkg/types
**File(s)**: `website/src/content/docs/reference/manifest.md`, `pkg/types/manifest.go:20`, `pkg/types/envfile.go`

## Description

The `env_file` manifest field is fully implemented (parsing, resolution, merge with inline `environment`), used in the CLI deploy command, and mentioned in the roadmap as "Done" (Milestone 5). However, it is **not documented** in the manifest reference guide.

## Evidence

- `pkg/types/manifest.go:20`: `EnvFile EnvFile ...yaml:"env_file,omitempty"`
- `pkg/types/envfile.go`: Complete implementation with `ParseEnvFile()` and `ResolveEnvFiles()`
- `cmd/banyan-cli/cmd/deploy.go:146`: Calls `types.ResolveEnvFiles()`
- `website/src/content/docs/roadmap.md` Milestone 5: Lists env_file as "Done"
- `website/src/content/docs/reference/manifest.md`: **No mention of env_file**

## Impact

- **User impact**: Users cannot discover the `env_file` feature from the manifest reference. They must stumble upon it in the roadmap or blog.
- **Feature waste**: A production-ready feature gets no usage because it's undiscoverable.

## Recommendation

Add `env_file` to the Service section of `website/src/content/docs/reference/manifest.md`:
- Type: string or list of strings
- Docker Compose compatibility note
- Example YAML showing usage
- Merge order: env_file values loaded first, then inline `environment` overrides
