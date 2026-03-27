---
# HIGH-001: Secret values visible in nerdctl process arguments

**Severity**: High
**Responsibility**: Platform Issue
**Component**: Agent — Container Launch
**File(s)**: `pkg/agent/agent.go:561-563`

## Description

Secrets are passed to `nerdctl run` as `-e NAME=VALUE` command-line arguments. These are visible in:
- Process listings (`ps aux`, `/proc/<pid>/cmdline`)
- Kernel audit logs
- Container runtime debug output

```go
for name, value := range task.ResolvedSecrets {
    args = append(args, "-e", name+"="+value)
}
```

## Impact

- **Who**: Any user with shell access on the agent host, or monitoring tools reading process lists
- **What**: Plaintext secret values during container startup
- **Blast radius**: One agent host per exposure window (secrets are in args only during nerdctl execution)

## Evidence

The `buildNerdctlRunArgs()` function at `pkg/agent/agent.go:561-563` appends secrets as `-e` flags. The `commandRunner` at line 417 executes `nerdctl` with these args visible in the process tree.

## Recommendation

Write secrets to a temporary file with 0600 permissions and pass via `nerdctl run --env-file /tmp/banyan-secrets-XXXX`. Delete the file immediately after container start. This removes secrets from the process argument list.

Alternative: Use nerdctl's stdin or environment inheritance if supported.
---
