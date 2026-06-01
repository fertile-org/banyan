# [HIGH-001] `--insecure-registry` hardcoded for all image pulls

**Severity**: High
**Responsibility**: Platform Issue
**Component**: Agent, Image Registry
**File(s)**: `pkg/agent/agent.go:430`

## Description

The agent unconditionally passes `--insecure-registry` to `nerdctl pull` for ALL image pulls, regardless of whether the registry actually requires insecure communication. This silently downgrades image pull security.

## Evidence

```go
// pkg/agent/agent.go:430
if err := commandRunner(ctx, "nerdctl", "pull", "--insecure-registry", task.Image); err != nil {
```

The flag is hardcoded and unconditional. It should only be used when the registry is explicitly configured as insecure.

## Impact

**Who can exploit**: Network attacker performing MITM on image pulls.

**What they gain**: Inject malicious container images into any deployment by intercepting the HTTP connection to the registry.

**Blast radius**: Any deployment pulling from a TLS-capable registry that gets MITM'd.

## Recommendation

1. Track registry security configuration per registry:
```go
// In agent config or task, determine if --insecure-registry is needed
needsInsecure := registryRequiresInsecure(task.Image) // based on registry URL
if needsInsecure {
    args = append(args, "--insecure-registry")
}
args = append(args, task.Image)
```

2. Or remove the flag entirely and only add it when user explicitly configures an insecure registry in the manifest.

## Secure Default Consideration

**Checklist T3**: "TLS on Registry — DEFAULT — Image pulls without TLS allow MITM image substitution."

The registry is hardcoded to use insecure mode for all pulls — the opposite of the secure default.