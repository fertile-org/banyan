# Security Audit Report — 2026-05-30

## Scope

Full audit of the Banyan codebase. Focus on verifying fixes from prior audits and checking for new security issues. Components reviewed: Authentication & gRPC, Agent, CLI, Install Script, Storage, VPC Networking.

## Methodology

Systematic review against Banyan threat model and secure defaults checklist.
Parallel audit agents covering: Auth & gRPC layer, Agent security, CLI security, Install script.

## Prior Audit Comparison

### 2026-03-27 (Secrets + Auto-Scaling)
| Status | Count | Details |
|--------|-------|---------|
| **FIXED** | 5 | HIGH-001 (secrets in args), MED-002 (NFS validation), HIGH-002 (scale validation), MED-003 (autoscale bounds), MED-004 (DNS bind) |
| **Still Open** | 5 | LOW-001 (--value flag), LOW-002 (stream rate limit), LOW-003 (GCM AAD), MED-001 (secrets key perms), INFO-002 (error leaks names) |

### 2026-03-07 (Full audit)
| Status | Count | Details |
|--------|-------|---------|
| **FIXED** | 22 | CRIT-001 (TLS - partial), CRIT-002 (registry auth), CRIT-003 (auth disabled), HIGH-001 (pubkey auth), HIGH-002 (agent identity), HIGH-003 (token expiry), HIGH-004 (rate limiting), HIGH-005 (insecure registry), HIGH-007 (network isolation), HIGH-008 (agent uniqueness), HIGH-009 (auth logging), HIGH-010 (checksums), HIGH-011 (name injection), MED-001 (bind all), MED-002 (etcd defaults), MED-003 (manifest validation), MED-005 (panic recovery), MED-007 (DNS cross-deployment), MED-008 (env vars in status), MED-009 (weak registration), LOW-001 (config perms), LOW-002 (constant-time) |
| **WONTFIX** | 4 | HIGH-006 (env vars plaintext - design decision), MED-004 (container hardening), MED-006 (error details for debug), LOW-004 (isolation gaps) |

## Findings Summary

| Severity | Count | Platform | Default | Mitigation Gap |
|----------|-------|----------|---------|----------------|
| Critical |   1   |    1     |    0    |       0        |
| High     |   3   |    1     |    0    |       2        |
| Medium   |   3   |    0     |    1    |       2        |
| Low      |   0   |    0     |    0    |       0        |
| Info     |   3   |    0     |    0    |       0        |
| **Total**| **10**|  **2**   |  **1**  |     **4**      |

## Critical Findings

| ID | Title | Type |
|----|-------|------|
| [CRIT-001](findings/CRIT-001-grpc-no-tls.md) | gRPC server has no TLS — tokens and manifests transmitted in plaintext | Platform Issue |

## High Findings

| ID | Title | Type |
|----|-------|------|
| [HIGH-001](findings/HIGH-001-insecure-registry-hardcoded.md) | `--insecure-registry` hardcoded for all image pulls | Platform Issue |
| [HIGH-002](findings/HIGH-002-agent-rpc-bypass-auth.md) | Agent RPCs bypass auth entirely (WireGuard tunnel assumed) | Mitigation Gap |
| [HIGH-003](findings/HIGH-003-no-checksum-deps.md) | Dependency binaries have no checksum verification | Mitigation Gap |

## Medium Findings

| ID | Title | Type |
|----|-------|------|
| [MED-001](findings/MED-001-cli-value-flag-ps.md) | CLI `--value` flag exposes secrets in process listing | Mitigation Gap |
| [MED-002](findings/MED-002-connect-api-no-tls.md) | Connect API dashboard uses plaintext HTTP/2 (h2c) | Default Issue |
| [MED-003](findings/MED-003-user-enumeration-jwt.md) | JWT auth path allows user enumeration | Platform Issue |

## Informational

| ID | Title |
|----|-------|
| [INFO-001](findings/INFO-001-connect-api-no-rate-limit.md) | Connect API has no rate limiting |
| [INFO-002](findings/INFO-002-token-id-128bit.md) | Token ID uses 128 bits instead of 256 |
| [INFO-003](findings/INFO-003-previous-items-still-open.md) | Previous audit items still open |

## Top Recommendations

1. **Wire TLS into gRPC server** (CRIT-001) — `LoadServerTLSConfig()` exists but is never called. Create `grpc.Creds` from the generated cert bundle and pass to `grpc.NewServer()`.
2. **Remove `--insecure-registry` default** (HIGH-001) — Only use when registry is explicitly configured as insecure. Default should be TLS-verified pulls.
3. **Add checksum verification for dependencies** (HIGH-003) — Apply same SHA-256 verification pattern from `install.sh` to `install-deps.sh` for etcd, nerdctl, CNI, BuildKit, registry.
4. **Warn when using `--value` flag** (MED-001) — Print prominent warning when secret appears in process listing, or remove the flag entirely.
5. **Add rate limiting to Connect API** (INFO-001) — Dashboard login endpoint is brute-forceable without per-IP limits.

## Components Reviewed

| Component | Status | Notes |
|-----------|--------|-------|
| Authentication (pkg/rpc/auth.go, auth/) | Reviewed | Token expiry, rate limiting fixed; TLS not wired |
| gRPC Server (pkg/engine/grpc_server.go) | Reviewed | No TLS configured; panic recovery added |
| Agent (pkg/agent/, cmd/banyan-agent/) | Reviewed | Secrets via env-file fixed; insecure-registry issue found |
| CLI (cmd/banyan-cli/) | Reviewed | Credential storage fixed; --value flag still problematic |
| Install Script (install.sh, install-deps.sh) | Reviewed | Banyan binaries verified; dependencies not |
| Storage (pkg/storage/) | Reviewed | TLS supported for external etcd |
| VPC Networking (pkg/vpc/) | Reviewed | WireGuard encryption in place |
| Connect API (pkg/engine/connect_server.go) | Reviewed | New component — plaintext HTTP/2, no rate limiting |

## Security Controls Verified as Correct

| Control | Status | Notes |
|---------|--------|-------|
| Session token constant-time comparison | ✓ | `subtle.ConstantTimeCompare` used |
| Config file permissions | ✓ | 0600 on credential files, 0700 on dirs |
| Private key file permissions | ✓ | 0600 files, 0700 dirs |
| WireGuard overlay encryption | ✓ | Data plane encrypted |
| etcd localhost binding | ✓ | Managed etcd on 127.0.0.1 only |
| No credentials in logs | ✓ | Tokens/passwords never logged |
| Command/entrypoint safety | ✓ | Passed as arrays, not shell-interpolated |
| Dependency version pinning | ✓ | All versions hardcoded |
| HTTPS for all downloads | ✓ | No HTTP fallback |
| DNS deployment-scoped | ✓ | Resolves within deployment |
| Agent task isolation | ✓ | Tasks filtered by agent name |
| Bcrypt password hashing | ✓ | Cost factor 12 |
| Panic recovery in gRPC | ✓ | Recovery interceptor added |

## Notes

- The TLS infrastructure exists (`GenerateTLSBundle`, `LoadServerTLSConfig`) but is **never wired into the gRPC servers**. The certificates are generated and saved to disk, but no server or client actually uses them.
- WireGuard provides strong network-level encryption for the control tunnel. However, the gRPC layer itself has no TLS, relying entirely on WireGuard for transport security.
- Several prior audit items remain open (--value flag, Connect API rate limiting, token ID bit length). These are lower severity but should be addressed.