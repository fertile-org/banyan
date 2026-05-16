# Banyan Secure Defaults Checklist

Every item on this list represents a security decision. For each, Banyan must either **enforce** the secure option, **default** to it, or **warn** when the user deviates.

The philosophy: Banyan's users are small teams without dedicated security engineers. If they have to opt-in to security, they won't. The platform must protect them by default.

## Classification

| Action | Meaning |
|--------|---------|
| **ENFORCE** | Banyan must require this. No opt-out. Users cannot disable it even if they try. |
| **DEFAULT** | Banyan defaults to the secure option. Users can override with explicit configuration and receive a warning. |
| **WARN** | Banyan allows the insecure option but prints a clear warning explaining the risk. |
| **DOCUMENT** | The risk is inherent and can't be mitigated technically. Documentation must explain the risk clearly. |

---

## Authentication

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| A1 | Cluster password required on engine init | **ENFORCE** | Without a password, anyone with network access owns the cluster. There is no valid reason to run without auth. |
| A2 | Password minimum length / complexity | **ENFORCE** | Short passwords are brute-forceable. Minimum 8 characters, reject empty/trivial passwords. |
| A3 | Password hashed with bcrypt (cost >= 12) | **ENFORCE** | bcrypt with adequate cost factor resists offline brute force. SHA-256 alone is not sufficient for password hashing. |
| A4 | Auth token minimum entropy (256-bit) | **ENFORCE** | Tokens must be cryptographically random and long enough to resist guessing. Use `crypto/rand`. |
| A5 | Token expiry / rotation mechanism | **DEFAULT** | Tokens should expire. Default to a reasonable lifetime (e.g., 30 days). Allow override for long-running automation. |
| A6 | Auth required on every gRPC endpoint | **ENFORCE** | No endpoint should be reachable without authentication. Health checks can return minimal info (up/down) without auth, but nothing else. |
| A7 | Failed auth rate limiting | **DEFAULT** | Slow down brute-force attempts. Default to rate limiting after N failures. |
| A8 | Credentials never logged | **ENFORCE** | Passwords, tokens, and session tokens must never appear in logs at any log level. |

## Transport Security (TLS)

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| T1 | TLS on Engine gRPC server | **DEFAULT** | All gRPC traffic carries tokens and manifests (with env vars). Plaintext means network eavesdroppers see everything. Auto-generate self-signed certs on init if none provided. |
| T2 | TLS minimum version 1.2 | **ENFORCE** | TLS 1.0/1.1 have known vulnerabilities. No reason to support them. |
| T3 | TLS on Registry | **DEFAULT** | Image pulls without TLS allow MITM image substitution. |
| T4 | TLS on etcd connection | **DEFAULT** | etcd contains all cluster state. If etcd supports TLS, use it by default. |
| T5 | CLI warns on insecure connection | **WARN** | If TLS is disabled, CLI should print: "WARNING: Connection to engine is not encrypted. Credentials and manifests will be transmitted in plaintext." |
| T6 | Agent warns on insecure connection | **WARN** | Same as T5 for agent-to-engine communication. |

## Credential Storage

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| S1 | Config files written with 0600 permissions | **ENFORCE** | Config contains auth tokens. Only the owner should read it. |
| S2 | Config file ownership matches running user | **ENFORCE** | Prevent other users from reading credentials. |
| S3 | Password not stored after initial exchange | **ENFORCE** | Only the bcrypt hash lives on the engine. CLI and agent store tokens, never the password. |
| S4 | Password not accepted as CLI flag in production | **WARN** | `--password` flag is visible in process listing (`ps aux`). Warn users and recommend interactive prompt or environment variable. |
| S5 | Tokens not printed to stdout | **ENFORCE** | Token values should never be printed to terminal output. Show masked versions if needed. |

## Agent Security

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| G1 | Agent registration requires valid auth token | **ENFORCE** | Without this, any machine can join the cluster and receive workloads. |
| G2 | Agent name uniqueness enforced | **ENFORCE** | Duplicate agent names cause confusion and could allow impersonation. Reject registration if name already exists. |
| G3 | Session token generated with crypto/rand | **ENFORCE** | Session tokens authenticate log streaming. Must be cryptographically random. |
| G4 | Session token constant-time comparison | **ENFORCE** | Prevent timing attacks on session token validation. Use `subtle.ConstantTimeCompare`. |
| G5 | Agent can only execute tasks assigned to it | **ENFORCE** | An agent should not be able to request or execute tasks assigned to other agents. |
| G6 | Agent deregistration on disconnect | **DEFAULT** | Stale agents should be cleaned up. Default to marking agents as unavailable after heartbeat timeout. |

## Container Isolation

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| C1 | No privileged containers by default | **ENFORCE** | `--privileged` gives full host access. Never set this unless the manifest explicitly requests it. |
| C2 | Default seccomp profile applied | **DEFAULT** | Containerd's default seccomp profile blocks dangerous syscalls. Don't disable it. |
| C3 | No host network by default | **ENFORCE** | Host networking bypasses all container network isolation. Only allow if explicitly requested in manifest. |
| C4 | No host PID namespace by default | **ENFORCE** | Host PID namespace allows seeing and signaling host processes. |
| C5 | Read-only root filesystem option | **DOCUMENT** | Recommend but don't enforce — many applications need writable filesystem. Document the security benefit. |
| C6 | Resource limits (CPU, memory) | **DEFAULT** | Prevent container resource exhaustion from affecting the host or other containers. Default to reasonable limits if not specified. |

## Network Security

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| N1 | Engine gRPC listens on configurable address | **DEFAULT** | Default to `0.0.0.0` (agents need to connect), but document firewall requirements prominently. |
| N2 | etcd listens on localhost only | **DEFAULT** | etcd should only be accessed by the engine process on the same host. Binding to 0.0.0.0 exposes all cluster state. |
| N3 | Registry listens on configurable address | **DEFAULT** | Same as N1 — agents need to pull images, but exposure should be intentional. |
| N4 | VPC overlay encryption | **DEFAULT** | Inter-node container traffic should be encrypted by default (e.g., WireGuard backend for Flannel). |
| N5 | No cross-deployment network access by default | **DEFAULT** | Containers from deployment A should not reach containers from deployment B unless explicitly configured. |
| N6 | DNS only resolves services within same deployment | **ENFORCE** | Prevent DNS-based reconnaissance across deployments. |

## Manifest & Input Validation

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| M1 | Manifest schema validation before deploy | **ENFORCE** | Reject malformed manifests early. Validate types, required fields, value ranges. |
| M2 | Service name sanitization | **ENFORCE** | Service names become container names, DNS names, and CLI arguments. Restrict to alphanumeric + hyphens. Reject injection characters. |
| M3 | Port range validation | **ENFORCE** | Ports must be 1-65535. Privileged ports (< 1024) should be allowed but documented. |
| M4 | Environment variable key validation | **ENFORCE** | Env var keys should match `[A-Z_][A-Z0-9_]*`. Reject keys that could cause shell injection. |
| M5 | Image reference validation | **ENFORCE** | Image names should be validated against OCI naming rules. Reject strings that could cause command injection in `nerdctl pull`. |
| M6 | Replica count limits | **DEFAULT** | Set a sensible maximum (e.g., 100) to prevent accidental resource exhaustion. Allow override. |
| M7 | Command/entrypoint passed safely | **ENFORCE** | Container commands must be passed as arrays to the runtime, never interpolated into shell strings. |

## Install & Supply Chain

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| I1 | Binary checksum verification in install script | **ENFORCE** | Download checksums file, verify binary integrity before installation. |
| I2 | HTTPS for all downloads | **ENFORCE** | All binary and dependency downloads must use HTTPS. No HTTP fallback. |
| I3 | Dependency version pinning | **ENFORCE** | Pin exact versions of etcd, containerd, nerdctl, BuildKit, CNI plugins. Don't install "latest". |
| I4 | Install script integrity | **DOCUMENT** | The curl\|bash pattern is industry-standard for dev tools but inherently risky. Document: "Review the script before running, or install manually." |

## Logging & Observability

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| L1 | Auth events logged (success and failure) | **ENFORCE** | Failed auth attempts are the first signal of an attack. Must be logged with source IP. |
| L2 | Deployment events logged | **ENFORCE** | Who deployed what and when. Essential for incident investigation. |
| L3 | Agent registration/deregistration logged | **ENFORCE** | Unexpected agent joins could indicate compromise. |
| L4 | No secrets in logs | **ENFORCE** | Env vars, tokens, passwords must be redacted from all log output. |
| L5 | Log to stderr by default | **DEFAULT** | Standard practice. Allow configuration to file or syslog. |

## Error Handling

| # | Requirement | Action | Rationale |
|---|------------|--------|-----------|
| E1 | gRPC errors don't leak internals | **ENFORCE** | Return generic error codes to clients. Log details server-side. Don't expose file paths, stack traces, or internal state in gRPC error messages. |
| E2 | Auth failures return same error regardless of cause | **ENFORCE** | Don't distinguish "invalid token" from "token expired" from "no token" in the response. This prevents enumeration. |
| E3 | Panic recovery in gRPC handlers | **ENFORCE** | A panic in one RPC handler shouldn't crash the engine. Use gRPC recovery interceptor. |

---

## How to Use This Checklist

During an audit:

1. **For each item**, check whether the current codebase satisfies it
2. **If satisfied**: Note as "PASS" — no finding needed
3. **If not satisfied**: Create a finding with:
   - The checklist item number (e.g., T1, A3, M5)
   - The expected behavior (from this checklist)
   - The actual behavior (from the code)
   - The severity (based on the threat model)
   - The responsibility classification (Platform Issue / Default Issue / Mitigation Gap)
4. **If partially satisfied**: Note what's done and what's missing

This checklist grows as Banyan adds features. When a new component is added, add corresponding requirements here before the next audit.
