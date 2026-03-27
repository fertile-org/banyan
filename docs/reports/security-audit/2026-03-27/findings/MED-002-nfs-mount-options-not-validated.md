# MED-002: NFS mount options passed unsanitized to mount command

**Severity**: Medium
**Responsibility**: Platform Issue
**Component**: Agent — NFS Mounts
**File(s)**: `pkg/agent/nfs_mounts.go:100-106`

## Description

NFS mount options from `driver_opts.o` in the manifest are passed directly to the `mount -t nfs -o <opts>` command without validation. A malicious manifest could inject dangerous mount flags like `exec`, `suid`, or `dev`.

## Impact

- **Who**: Anyone who can deploy a manifest (CLI user)
- **What**: Mount NFS with unsafe options, potentially enabling privilege escalation via suid binaries
- **Blast radius**: One agent per NFS mount

## Recommendation

Whitelist allowed NFS mount options: `addr`, `vers`, `soft`, `hard`, `timeo`, `retrans`, `rsize`, `wsize`, `nolock`, `ro`, `rw`. Reject unknown options.
