# [MED-002] Managed etcd Insecure Defaults

**Severity**: Medium
**Status**: FIXED
**Responsibility**: Default Issue
**Component**: Engine — etcd
**File(s)**:
- `cmd/banyan-engine/cmd/engine.go:547-553` (managed etcd — no auth, no TLS)
- `cmd/banyan-engine/cmd/engine.go:118-133,543` (data dir created with `0o755`)
- `cmd/banyan-engine/cmd/engine.go:252,526` (default external endpoint `http://`)
- `pkg/types/config.go:50` (etcd password stored plaintext)

## Description

Three issues with etcd defaults:

1. **Managed etcd has no authentication**: Any local process can read/write all Banyan state via `http://127.0.0.1:2379`.

2. **Data directory is world-readable**: `/var/lib/banyan/etcd` is created with `0o755`. Other users can read raw etcd data files containing deployment records with environment variables.

3. **External etcd defaults to plaintext**: The default etcd endpoint is `http://localhost:2379` and the default connection security is "None" in the init wizard.

Mitigating factor: managed etcd binds to `127.0.0.1` only (good), and config files are `0o600` (good).

## Recommendation

1. Create data directories with `0o700`
2. Default the init wizard's etcd security to "TLS" (not "None")
3. Document that managed etcd is single-user only

## Fix

Changed the etcd data directory permissions from `0755` to `0700` in `cmd/banyan-engine/cmd/engine.go`. This prevents other local users from reading raw etcd data files containing deployment records.
