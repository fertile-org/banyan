# Security Audit Report — 2026-03-07

## Scope

Full audit of the Banyan codebase (`feat/code-audit` branch, commit `3ffc260`).

## Methodology

Systematic review against Banyan threat model and secure defaults checklist.
4 parallel audit agents covering: Authentication & gRPC, Storage & Registry, VPC & Networking, CLI & Install.
Serena MCP server used for codebase symbol analysis. Direct code reading for all security-critical paths.

## Remediation Summary

| Status | Count | Details |
|--------|-------|---------|
| **FIXED** | 23 | CRIT-001: WireGuard enforced (encrypted tunnel). CRIT-002: Registry bound to tunnel IP (WireGuard-protected). CRIT-003: Auth enforced. HIGH-001: Custom pubkey auth removed (WireGuard). HIGH-002: Agent identity via tunnel IP. HIGH-003: Session tokens removed. HIGH-004: Per-IP rate limiting (100 req/min). HIGH-005: Registry traffic encrypted by WireGuard. HIGH-007: Deployment network isolation via iptables (cross-agent). HIGH-008: Agent name uniqueness via tunnel IP identity. HIGH-009: Audit logging interceptors. HIGH-010: SHA-256 checksum verification in install script. HIGH-011: Name validation (DNS-safe, underscores allowed). MED-001: All services bind to tunnel IP/localhost. MED-002: etcd data dir permissions 0700. MED-003: Port/restart/service manifest validation. MED-005: Panic recovery interceptors. MED-007: Deployment-scoped DNS namespacing. MED-008: Env vars removed from status/dashboard RPCs. MED-009: Heartbeat rejects unregistered agents. LOW-001: Config/data dirs 0700. LOW-002: Pubkey auth removed (WireGuard handles identity). LOW-003: Install script security note + checksum verification. |
| **WONTFIX** | 4 | HIGH-006: Env vars plaintext by design (secrets management in Milestone 10). MED-004: Container hardening — containerd applies default seccomp/caps; forcing --cap-drop ALL breaks standard images. MED-006: Error details kept for debuggability (WireGuard is the security boundary). LOW-004: Defense-in-depth (read-only rootfs, PID ns) — opt-in via manifest, not platform default. |
| **Open** | 0 | — |
| **Total** | **27** | |

## Findings Summary

| Severity | Count | Platform | Default | Mitigation Gap |
|----------|-------|----------|---------|----------------|
| Critical |   3   |    2     |    1    |       0        |
| High     |  11   |    5     |    1    |       5        |
| Medium   |   9   |    3     |    4    |       2        |
| Low      |   4   |    0     |    2    |       2        |
| **Total**| **27**|  **10**  |  **8**  |     **9**      |

## Critical Findings

| ID | Title | Type |
|----|-------|------|
| [CRIT-001](findings/CRIT-001-no-tls-on-grpc.md) | No TLS on any gRPC connection | Default Issue |
| [CRIT-002](findings/CRIT-002-unauthenticated-oci-registry.md) | OCI registry unauthenticated and unencrypted | Platform Issue |
| [CRIT-003](findings/CRIT-003-auth-disabled-without-keys.md) | Authentication disabled when no whitelisted keys | Platform Issue |

## High Findings

| ID | Title | Type |
|----|-------|------|
| [HIGH-001](findings/HIGH-001-pubkey-auth-not-cryptographic.md) | Public key auth is identity assertion, not cryptographic proof | Platform Issue |
| [HIGH-002](findings/HIGH-002-no-agent-identity-binding.md) | No agent identity binding on RPCs — agents can impersonate each other | Mitigation Gap |
| [HIGH-003](findings/HIGH-003-no-token-expiration.md) | No token expiration, rotation, or revocation | Mitigation Gap |
| [HIGH-004](findings/HIGH-004-no-auth-rate-limiting.md) | No rate limiting on authentication endpoints | Mitigation Gap |
| [HIGH-005](findings/HIGH-005-insecure-registry-flag.md) | Image push/pull bypasses TLS verification (--insecure-registry) | Platform Issue |
| [HIGH-006](findings/HIGH-006-secrets-plaintext-in-etcd.md) | Environment variables (secrets) stored plaintext in etcd | Platform Issue |
| [HIGH-007](findings/HIGH-007-no-deployment-network-isolation.md) | No deployment network isolation — flat overlay + dead security code | Platform Issue |
| [HIGH-008](findings/HIGH-008-agent-name-not-unique.md) | Agent name uniqueness not enforced — agent impersonation | Platform Issue |
| [HIGH-009](findings/HIGH-009-auth-failures-not-logged.md) | Authentication failures not logged | Mitigation Gap |
| [HIGH-010](findings/HIGH-010-no-install-checksum-verification.md) | Install script has no checksum verification | Mitigation Gap |
| [HIGH-011](findings/HIGH-011-service-name-injection.md) | Service/app names not sanitized — injection risk | Mitigation Gap |

## Medium Findings

| ID | Title | Type |
|----|-------|------|
| [MED-001](findings/MED-001-services-bind-all-interfaces.md) | Services bind to 0.0.0.0 by default | Default Issue |
| [MED-002](findings/MED-002-managed-etcd-insecure-defaults.md) | Managed etcd insecure defaults | Default Issue |
| [MED-003](findings/MED-003-insufficient-manifest-validation.md) | Insufficient manifest input validation | Mitigation Gap |
| [MED-004](findings/MED-004-missing-container-hardening.md) | Missing container hardening defaults | Default Issue |
| [MED-005](findings/MED-005-no-grpc-panic-recovery.md) | No panic recovery in gRPC handlers | Mitigation Gap |
| [MED-006](findings/MED-006-grpc-errors-leak-internals.md) | gRPC error messages leak internal details | Platform Issue |
| [MED-007](findings/MED-007-dns-cross-deployment.md) | DNS resolves services across all deployments | Platform Issue |
| [MED-008](findings/MED-008-env-vars-exposed-in-status.md) | Environment variables exposed in status/dashboard RPCs | Platform Issue |
| [MED-009](findings/MED-009-weak-agent-registration.md) | Weak agent registration validation | Platform Issue |

## Low Findings

| ID | Title | Type |
|----|-------|------|
| [LOW-001](findings/LOW-001-config-dirs-world-readable.md) | Config/data directories world-readable (0755) | Default Issue |
| [LOW-002](findings/LOW-002-pubkey-comparison-not-constant-time.md) | Public key validation not constant-time | Mitigation Gap |
| [LOW-003](findings/LOW-003-curl-bash-install-pattern.md) | curl\|bash install pattern | Default Issue |
| [LOW-004](findings/LOW-004-container-isolation-gaps.md) | No read-only root filesystem or PID namespace enforcement | Mitigation Gap |

## Positive Findings

These security controls are correctly implemented:

| Control | Assessment |
|---------|-----------|
| Session token generation | 256-bit, `crypto/rand` |
| Session token comparison | `subtle.ConstantTimeCompare` |
| Config file permissions | `0600` on all credential files |
| Private key file permissions | `0600` files, `0700` directory |
| WireGuard overlay encryption | Data plane traffic encrypted |
| etcd localhost binding | Managed etcd on `127.0.0.1` only |
| No credentials in logs | Tokens/passwords never logged |
| Command/entrypoint safety | Passed as arrays, not shell-interpolated |
| Dependency version pinning | Install script pins all versions |
| HTTPS for all downloads | Install script uses HTTPS throughout |
| DNS bound to internal interface | DNS server binds to bridge gateway IP |
| Agent task type validation | Only container create/stop tasks accepted |

## Top Recommendations

1. **Add TLS to gRPC** (or enforce WireGuard tunnel — refuse to start without it). This is the single highest-impact fix: it protects tokens, manifests, and secrets in transit.

2. **Secure the OCI registry**: Add authentication (at minimum basic auth), add TLS, and bind to the WireGuard tunnel interface instead of 0.0.0.0.

3. **Enforce authentication**: Refuse to start the engine without at least one whitelisted key. Remove the no-auth fallback.

4. **Add agent identity binding**: Verify that the agent name in each RPC matches the public key used to authenticate. This prevents agent impersonation.

5. **Sanitize manifest inputs**: Validate service names, app names, port ranges, and replica counts before accepting deployments. This is a single validation function that closes multiple injection vectors.

## Components Reviewed

| Component | Status | Key Findings |
|-----------|--------|-------------|
| `pkg/rpc/` (auth) | Reviewed | No TLS, no rate limiting, no token expiry |
| `pkg/engine/grpc_server.go` | Reviewed | Auth optional, no panic recovery, errors leak internals |
| `pkg/engine/engine.go` | Reviewed | Registry unauth/unencrypted, metrics exposed |
| `pkg/agent/agent.go` | Reviewed | --insecure-registry, no container hardening |
| `pkg/agent/engine_client.go` | Reviewed | insecure.NewCredentials() |
| `pkg/agent/grpc_server.go` | Reviewed | No TLS on agent server |
| `pkg/storage/etcd.go` | Reviewed | External etcd TLS supported (good) |
| `pkg/vpc/overlay/` | Reviewed | WireGuard encryption (good), peer validation weak |
| `pkg/vpc/dns/` | Reviewed | Cross-deployment resolution |
| `pkg/vpc/security/` | Reviewed | Complete implementation but NEVER CALLED (dead code) |
| `pkg/types/manifest.go` | Reviewed | No input sanitization |
| `pkg/types/config.go` | Reviewed | File permissions correct |
| `cmd/banyan-engine/cmd/` | Reviewed | etcd data dir 0755, managed etcd no auth |
| `cmd/banyan-agent/cmd/` | Reviewed | Clean |
| `cmd/banyan-cli/cmd/` | Reviewed | Minimal manifest validation |
| `install.sh` | Reviewed | No checksum verification |

## Notes

- The WireGuard control tunnel is a strong architectural choice that mitigates many transport security issues. However, it is optional and the fallback path provides zero protection. The security posture depends entirely on correct WireGuard configuration.
- `pkg/vpc/security/` contains a complete iptables-based network policy implementation with default-deny support, but it is never called from the agent or engine. This is the most significant dead security code in the project.
- No previous security audits exist for comparison.

---

**Audit Date**: 2026-03-07
**Branch**: `feat/code-audit`
**Status**: Complete — 27 findings: 3 Critical, 11 High, 9 Medium, 4 Low (23 FIXED, 4 WONTFIX, 0 Open)
