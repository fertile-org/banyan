# INFO-001: Missing secrets.key silently disables secrets feature

**Severity**: Informational
**Responsibility**: Observation
**Component**: Engine — Initialization
**File(s)**: `pkg/engine/engine.go:113-120`

## Description

If `secrets.key` is missing at engine startup, the `secrets` field remains `nil`. Secret RPCs return "secrets not enabled" errors. This is correct behavior but could confuse operators who expect secrets to work after re-initialization or engine migration.

## Impact

No security impact. Operational clarity concern only.

## Recommendation

Log a startup warning when secrets.key is not found: "Secrets encryption key not found at /etc/banyan/keys/secrets.key — secret management is disabled. Run 'banyan-engine init' to generate a key."
