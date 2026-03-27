# LOW-001: CLI --value flag exposes secrets in shell history

**Severity**: Low
**Responsibility**: Mitigation Gap
**Component**: CLI — Secret Create
**File(s)**: `cmd/banyan-cli/cmd/secret.go:58`

## Description

`banyan-cli secret create DB_PASSWORD --value "secret"` records the plaintext in shell history and is visible in `ps`. The help text warns about this, but the flag is still accepted.

## Impact

Secrets visible in `~/.bash_history`, `ps aux`, `/proc/<pid>/cmdline`.

## Recommendation

The warning is adequate for v1. Consider deprecating the flag in favor of `--from-file` and interactive prompt only.
