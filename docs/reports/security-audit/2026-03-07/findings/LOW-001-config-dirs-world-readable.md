# [LOW-001] Config/Data Directories World-Readable (0755)

**Severity**: Low
**Status**: FIXED
**Responsibility**: Default Issue
**Component**: Config, Engine
**File(s)**:
- `pkg/types/config.go:112` (`/etc/banyan/` — `0o755`)
- `cmd/banyan-engine/cmd/engine.go:118-133` (`/var/lib/banyan/etcd` — `0o755`)
- `cmd/banyan-engine/cmd/engine.go:184` (whitelisted-keys dir — `0o755`)

## Description

Config and data directories are created with `0o755`, making directory listings readable by all users. While the files themselves are `0o600`, any local user can enumerate config file names, whitelisted key file names, and see the etcd data directory structure.

Mitigating factor: file contents are not readable (0600). The risk is limited to metadata enumeration.

## Recommendation

Use `0o700` for directories that contain sensitive files.

## Fix

Changed all directory creation calls from `0755` to `0700` in `cmd/banyan-engine/cmd/engine.go`, `cmd/banyan-agent/cmd/agent.go`, and `pkg/types/config.go`. Config and data directories are now only accessible by the owning user.
