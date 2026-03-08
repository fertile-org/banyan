# [HIGH-005] Image Push/Pull Bypasses TLS Verification (--insecure-registry)

**Severity**: High
**Responsibility**: Platform Issue
**Component**: Agent, CLI
**File(s)**:
- `pkg/agent/agent.go:355` (`nerdctl pull --insecure-registry`)
- `cmd/banyan-cli/cmd/deploy.go:379` (`nerdctl push --insecure-registry`)

## Description

Both the CLI (image push) and agent (image pull) use the `--insecure-registry` flag with nerdctl. This flag disables TLS certificate verification and allows plain HTTP communication with the registry.

The flag applies globally to the registry connection — not just for the built-in registry. If an image reference points to an external registry, the insecure flag still applies.

## Impact

- **Who**: Network attacker between CLI/agent and registry
- **What they gain**: Man-in-the-middle attack on image transfer — serve modified container images
- **Blast radius**: All nodes that pull the tampered image

## Recommendation

1. Add TLS to the embedded registry (see CRIT-002)
2. Once TLS is in place, remove `--insecure-registry` from both push and pull
3. For the embedded registry only, configure nerdctl to trust the auto-generated CA
4. Never apply `--insecure-registry` to external registries
