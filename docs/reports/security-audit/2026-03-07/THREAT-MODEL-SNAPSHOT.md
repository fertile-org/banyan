# Threat Model Snapshot — 2026-03-07

Captures the actual security posture at audit time against each trust boundary.

## Trust Boundary Status

| Boundary | Expected Control | Actual Status | Findings |
|----------|-----------------|---------------|----------|
| **TB-1: CLI -> Engine** | Token + TLS | Token (no TLS). WireGuard optional. No fallback warning. | CRIT-001, HIGH-001 |
| **TB-2: Agent -> Engine** | Token + TLS | Public key whitelist (no TLS). No identity binding. No rate limiting. | CRIT-001, CRIT-003, HIGH-001, HIGH-002, HIGH-004 |
| **TB-3: Engine -> Agent** | Session token + TLS | Session token (no TLS). No token expiry. | CRIT-001, HIGH-003 |
| **TB-4: Agent -> Registry** | Auth + TLS | No auth, no TLS, `--insecure-registry`. Open to all. | CRIT-002, HIGH-005 |
| **TB-5: Engine -> etcd** | Auth + TLS | No auth (managed), localhost only. Plaintext secrets stored. | MED-002, HIGH-006 |
| **TB-6: Container -> Container** | Network isolation | Flat network, no isolation. Security module is dead code. | HIGH-007, MED-007 |
| **TB-7: External -> Ports** | Firewall + binding | All services on 0.0.0.0. No firewall guidance. | MED-001 |
| **TB-8: User -> Config** | File permissions | Files 0600 (good). Directories 0755. | LOW-001 |

## Attack Surface Changes Since Threat Model

The threat model accurately describes the architecture. No new components were found outside the model. Key observations:

1. **WireGuard is the primary security mechanism**, but it is optional. The threat model should be updated to distinguish "with WireGuard" and "without WireGuard" security postures.

2. **`pkg/vpc/security/` exists but is dead code.** The threat model assumes container isolation may exist; in reality, there is none.

3. **The registry is the widest-open attack surface** — no auth, no TLS, no binding restriction, no image verification. This is not adequately highlighted in the threat model.

4. **Auth event logging is absent.** The threat model assumes logging exists for detection; it does not for auth events.
