# [MED-004] Missing Container Hardening Defaults

**Severity**: Medium
**Responsibility**: Default Issue
**Component**: Agent — Container Execution
**File(s)**:
- `pkg/agent/agent.go:445-518` (`buildNerdctlRunArgs`)

## Description

The `buildNerdctlRunArgs` function does not apply several container hardening options:

1. **No explicit seccomp profile**: Does not add `--security-opt seccomp=...`. Relies on containerd's default (which is generally good, but not enforced by Banyan).

2. **No `--privileged=false` guard**: If a future manifest field adds privileged mode, there is no agent-side check to block it.

3. **No capability dropping**: Does not add `--cap-drop ALL --cap-add <needed>`. Containers run with Docker's default capabilities.

4. **Resource limits are optional**: Memory and CPU limits only applied when the manifest includes them. No default limits.

Mitigating factors: containerd applies a default seccomp profile, and the manifest schema does not currently support `privileged`, `pid`, or `cap_add` fields.

## Recommendation

1. Add `--cap-drop ALL` and only `--cap-add` what's needed
2. Add a default memory limit (e.g., 512MB) when none is specified
3. Add an explicit `--security-opt seccomp=default` to enforce the profile
4. If `privileged` or `cap_add` manifest fields are added in the future, validate them against an allowlist
