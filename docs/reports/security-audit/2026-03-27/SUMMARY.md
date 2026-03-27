# Security Audit Report — 2026-03-27

## Scope

Full audit of the Banyan codebase (`feat/secret-manager` branch, commit `2eabd70`). Focus on new features since the 2026-03-07 audit: Auto-Scaling (M9) and Secrets Management (M11).

## Methodology

Systematic review against Banyan threat model and secure defaults checklist.
4 parallel audit agents covering: Secrets Management, Auto-Scaling & gRPC, Networking & Agent, Previous Findings.

## Prior Audit Comparison (2026-03-07)

| Status | Count | Details |
|--------|-------|---------|
| **Previously Fixed** | 23 | All critical, high, and medium fixes from 2026-03-07 remain in place |
| **Previously WONTFIX** | 3 | MED-004 (container hardening), MED-006 (error details), LOW-004 (defense-in-depth) — still WONTFIX, still reasonable |
| **Newly Resolved** | 1 | HIGH-006 (secrets plaintext in etcd) — **FIXED** by M11 secrets management. Secrets now encrypted at rest with AES-256-GCM. Environment variables remain plaintext (by design). |

## New Findings (This Audit)

| Severity | Count | Platform | Default | Mitigation Gap |
|----------|-------|----------|---------|----------------|
| Critical |   0   |    0     |    0    |       0        |
| High     |   2   |    1     |    0    |       1        |
| Medium   |   4   |    1     |    1    |       2        |
| Low      |   3   |    0     |    0    |       3        |
| Info     |   2   |    0     |    0    |       0        |
| **Total**| **11**|  **2**   |  **1**  |     **6**      |

## High Findings

| ID | Title | Type |
|----|-------|------|
| [HIGH-001](findings/HIGH-001-secret-values-in-nerdctl-args.md) | Secret values visible in nerdctl process arguments | Platform Issue |
| [HIGH-002](findings/HIGH-002-scale-rpc-missing-input-validation.md) | Scale RPC accepts negative/unbounded replica counts | Mitigation Gap |

## Medium Findings

| ID | Title | Type |
|----|-------|------|
| [MED-001](findings/MED-001-secrets-key-permissions-not-verified.md) | secrets.key file permissions not verified at runtime | Mitigation Gap |
| [MED-002](findings/MED-002-nfs-mount-options-not-validated.md) | NFS mount options passed unsanitized to mount command | Platform Issue |
| [MED-003](findings/MED-003-missing-autoscale-bounds-validation.md) | Autoscale min/max not validated at deploy time | Mitigation Gap |
| [MED-004](findings/MED-004-dns-server-default-bind-all.md) | DNS server defaults to 0.0.0.0:53 in struct | Default Issue |

## Low Findings

| ID | Title | Type |
|----|-------|------|
| [LOW-001](findings/LOW-001-secret-cli-value-flag.md) | CLI --value flag exposes secrets in shell history | Mitigation Gap |
| [LOW-002](findings/LOW-002-stream-rpc-no-rate-limit.md) | Streaming RPCs bypass rate limiting | Mitigation Gap |
| [LOW-003](findings/LOW-003-no-gcm-additional-data.md) | AES-GCM encryption uses no additional authenticated data | Mitigation Gap |

## Informational

| ID | Title |
|----|-------|
| [INFO-001](findings/INFO-001-secrets-feature-graceful-degradation.md) | Missing secrets.key silently disables secrets feature |
| [INFO-002](findings/INFO-002-secret-error-leaks-names.md) | ResolveSecrets error message lists missing secret names |

## Top Recommendations

1. **Use nerdctl --env-file for secrets** (HIGH-001) — Write secrets to a temp file with 0600 permissions, pass via `--env-file`, delete after container start. Prevents exposure in `ps` output.
2. **Validate Scale RPC inputs** (HIGH-002) — Reject negative replica counts and enforce MaxReplicas upper bound (same as Deploy handler).
3. **Validate NFS mount options** (MED-002) — Whitelist allowed NFS mount flags instead of passing user-provided options directly to `mount`.
4. **Verify secrets.key permissions at load time** (MED-001) — Refuse to load if permissions are not 0600.
5. **Validate autoscale min/max at deploy time** (MED-003) — Reject `min < 0`, `max > MaxReplicas`, `min > max`.

## Components Reviewed

| Component | Status | Notes |
|-----------|--------|-------|
| Secrets Manager (NEW) | Reviewed | `pkg/engine/secrets.go`, gRPC handlers, CLI, agent injection |
| Auto-Scaling (NEW) | Reviewed | `pkg/engine/autoscale.go`, Scale RPC, manifest validation |
| gRPC Server | Reviewed | Interceptors, rate limiting, all RPC handlers |
| Agent | Reviewed | `buildNerdctlRunArgs`, `pbTaskToLocal`, NFS mounts |
| VPC Networking | Reviewed | DNS server, WireGuard tunnel, control tunnel |
| etcd Storage | Reviewed | TLS config, encryption |
| CLI | Reviewed | Secret commands, scale command |
| Engine Init | Reviewed | secrets.key generation, multi-engine HA flow |

## Notes

- The secrets management implementation (M11) is architecturally sound. Secret values are never stored in task records — only decrypted just-in-time during PollTasks. The primary exposure point is nerdctl `-e` flags (inherent to all container runtimes).
- Auto-scaling safeguards (5 rebalancing constraints) effectively prevent abuse. The main gap is input validation on the Scale RPC.
- All 23 fixes from the 2026-03-07 audit remain intact. No regressions detected.
- HIGH-006 from the prior audit (secrets plaintext in etcd) is now resolved by M11.
