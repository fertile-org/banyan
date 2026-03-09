# [LOW-004] No Read-Only Root Filesystem or PID Namespace Enforcement

**Severity**: Low
**Status**: WONTFIX
**Responsibility**: Mitigation Gap
**Component**: Agent — Container Execution
**File(s)**:
- `pkg/agent/agent.go:445-518` (`buildNerdctlRunArgs`)

## Description

Two defense-in-depth container hardening options are not applied:

1. **No `--read-only` flag**: Containers can write to their root filesystem, which can facilitate persistence after compromise.

2. **No explicit PID namespace enforcement**: Containers use the default (isolated) PID namespace, but there is no explicit `--pid=container` to prevent future code from accidentally adding `--pid=host`.

Mitigating factor: These are defense-in-depth measures. The current defaults are not insecure — they just don't apply the most restrictive settings.

## Recommendation

Document these as optional hardening steps. Consider adding manifest fields for `read_only: true` and verifying PID namespace isolation is maintained in future changes.

## Fix

WONTFIX — These are defense-in-depth measures. Containerd already applies default seccomp profiles and isolated PID namespaces. Forcing `--cap-drop ALL` breaks standard images (redis, node, postgres). Read-only root filesystem (`--read-only`) is application-specific and would break most containers. These should be opt-in via manifest fields, not platform defaults.
