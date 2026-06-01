---
name: banyan-security-audit
description: Security audit for the Banyan container orchestration platform. Performs systematic threat modeling and code review across all components (engine, agent, CLI, gRPC, etcd, registry, VPC). Identifies platform vulnerabilities and insecure defaults. Use when reviewing security, preparing for release, or auditing changes that touch authentication, networking, secrets, or inter-component communication.
---

# Banyan Security Audit

Systematic security review of Banyan's codebase, architecture, and defaults. Identifies vulnerabilities that could lead to cluster takeover, data exposure, or unauthorized access.

**This skill produces reports only. It never modifies source code.**

## Core Principle: Secure by Default

Banyan is infrastructure software that manages servers, containers, and network traffic. A security flaw doesn't just affect one user — it affects every server in their cluster. The audit operates under this rule:

> **If a user can accidentally make their cluster insecure by following the docs, that's a Banyan issue, not a user issue.**

Banyan's target users are small teams without dedicated security engineers. They will not harden defaults. They will not read security advisories. The defaults must protect them.

## Two Modes

### Mode 1: Full Audit

Use when: "security audit", "full security review", "audit the codebase", no specific scope given, or preparing for a release.

Comprehensive review of the entire codebase against the threat model. Produces a complete report.

### Mode 2: Change Audit

Use when: reviewing a PR, a specific file, a feature branch, or a recent change.

Targeted review of changed code against the threat model. Checks whether the change introduces or resolves security issues.

**Auto-detect**: If the user mentions specific files, a PR, or a branch — use Change Audit. Otherwise, use Full Audit.

---

## Full Audit Workflow

### Phase 1: Discovery

Map the current state before auditing.

1. **Read the threat model**: Start with [threat-model.md](threat-model.md) to understand Banyan's attack surface and trust boundaries
2. **Read the secure defaults checklist**: Review [secure-defaults.md](secure-defaults.md) for what must be enforced
3. **Map the codebase** using Serena tools and code exploration:
   - `pkg/engine/` — Engine: gRPC server, scheduling, state management
   - `pkg/agent/` — Agent: container lifecycle, engine client
   - `pkg/rpc/` — Authentication, gRPC interceptors, protobuf definitions
   - `pkg/storage/` — etcd client and data persistence
   - `pkg/vpc/` — Overlay networking, DNS, firewall/security rules
   - `pkg/types/` — Manifest parsing, config handling, shared types
   - `cmd/banyan-engine/` — Engine binary, CLI flags, startup
   - `cmd/banyan-agent/` — Agent binary, CLI flags, startup
   - `cmd/banyan-cli/` — User-facing CLI, all subcommands
   - `install.sh` — Installation and dependency setup
4. **Check for new components**: Look for files or packages not covered by the threat model. If the attack surface has expanded, note it as a finding.

### Phase 2: Threat Modeling

For each trust boundary defined in [threat-model.md](threat-model.md), verify:

1. **The boundary exists in code** — Is authentication actually enforced, or just documented?
2. **Authentication** — Is every entry point authenticated? Are there bypass paths?
3. **Authorization** — Can authenticated entities only perform their intended actions?
4. **Data protection** — Is sensitive data encrypted in transit and at rest?
5. **Input validation** — Is all external input validated and sanitized before use?
6. **Error handling** — Do errors leak internal state, stack traces, or credentials?

### Phase 3: Component Review

Review each component systematically. **Order by blast radius** — highest impact first:

#### 3.1 Authentication & Authorization
Files: `pkg/rpc/auth.go`, auth interceptors, token exchange logic

Check:
- Token generation — entropy, length, predictability
- Password handling — hashing algorithm, salt, cost factor
- Auth bypass — unauthenticated endpoints, interceptor ordering gaps
- Credential storage — file permissions, plaintext vs encrypted
- Session management — token expiry, rotation, revocation
- Timing attacks — constant-time comparison for tokens

#### 3.2 gRPC Server
Files: `pkg/engine/grpc_server.go`, proto definitions

Check:
- TLS configuration — is it enforced? what versions/ciphers?
- Every RPC method — does each go through the auth interceptor?
- Input validation — every RPC parameter checked before use
- Error responses — no internal state leakage
- Resource limits — message size caps, connection limits, rate limiting

#### 3.3 etcd Storage
Files: `pkg/storage/etcd.go`, etcd configuration

Check:
- Connection security — TLS? authentication?
- Data sensitivity — what's stored? any secrets in plaintext?
- Access control — who can read/write? is RBAC enforced?
- Default configuration — secure out of the box?
- Backup/exposure — is etcd accessible from outside the engine host?

#### 3.4 Image Registry
Files: registry configuration, agent image pull logic

Check:
- Registry authentication — who can push/pull?
- Image integrity — signatures, checksums, digest verification
- Transport security — TLS? insecure-registry flags?
- Image provenance — can an attacker inject malicious images?

#### 3.5 Agent
Files: `pkg/agent/`, agent registration and task execution

Check:
- Registration — can a rogue agent join the cluster?
- Identity verification — is the agent who it claims to be?
- Command execution — what can the engine instruct an agent to do?
- Container isolation — privileges, capabilities, seccomp, namespaces
- Session token — generation, storage, transmission

#### 3.6 VPC & Networking
Files: `pkg/vpc/`, DNS, security rules

Check:
- Overlay traffic — encrypted between nodes?
- DNS — spoofing potential? internal only?
- Network isolation — are deployments isolated from each other?
- Firewall rules — default-deny or default-allow?
- Port exposure — what's listening on public interfaces?

#### 3.7 CLI
Files: `cmd/banyan-cli/`, all subcommands

Check:
- Password handling — visible in process listing? shell history?
- Config file — permissions? credential storage?
- Manifest handling — validation before sending to engine?
- Output — does CLI output leak sensitive information?

#### 3.8 Install Script
Files: `install.sh`

Check:
- Binary verification — checksums? GPG signatures?
- Dependency integrity — verified downloads?
- Default permissions — correct file ownership and modes?
- Configuration defaults — secure initial state?

### Phase 4: Classify Each Finding

Every finding MUST be classified on two dimensions:

#### Severity

| Level | Definition | Example |
|-------|-----------|---------|
| **Critical** | Remote cluster takeover, auth bypass, arbitrary code execution on any node | Unauthenticated gRPC endpoint that accepts Deploy calls |
| **High** | Privilege escalation, widespread data exposure, rogue node joining cluster | Agent registration without identity verification |
| **Medium** | Information disclosure, weak crypto, missing validation on sensitive paths | Tokens transmitted over plaintext gRPC |
| **Low** | Best practice violations, defense-in-depth gaps, hardening opportunities | No rate limiting on auth endpoints |
| **Informational** | Observations, architecture notes, future considerations | curl\|bash install pattern common in the industry |

#### Responsibility

| Type | Definition | Banyan's obligation |
|------|-----------|-------------------|
| **Platform Issue** | A flaw in Banyan's code or default config. Users are exposed regardless of their actions. | **Must fix.** This is a bug. |
| **Default Issue** | The secure option exists but isn't the default. Users who follow the happy path are insecure. | **Must change default.** Secure by default or it doesn't count. |
| **Mitigation Gap** | User can misconfigure, and Banyan doesn't prevent or warn. | **Should add guardrails.** Validation, warnings, or enforcement. |
| **User Responsibility** | User deliberately chooses insecure option despite Banyan's warnings and documentation. | **Document the risk.** Banyan has done its part. |

### Phase 5: Write Report

Create the report directory and files:

```
docs/reports/security-audit/YYYY-MM-DD/
├── SUMMARY.md
├── findings/
│   ├── CRIT-001-<short-name>.md
│   ├── HIGH-001-<short-name>.md
│   ├── MED-001-<short-name>.md
│   ├── LOW-001-<short-name>.md
│   └── INFO-001-<short-name>.md
└── THREAT-MODEL-SNAPSHOT.md      # Full audit only
```

Use the current date for the directory name.

#### SUMMARY.md Format

```markdown
# Security Audit Report — YYYY-MM-DD

## Scope

[Full audit / Change audit of <scope>]

## Methodology

Systematic review against Banyan threat model ([threat-model.md]). Components reviewed: [list].

## Findings Summary

| Severity | Count | Platform | Default | Mitigation Gap | User |
|----------|-------|----------|---------|----------------|------|
| Critical |       |          |         |                |      |
| High     |       |          |         |                |      |
| Medium   |       |          |         |                |      |
| Low      |       |          |         |                |      |
| Info     |       |          |         |                |      |

## Critical Findings

[One-line descriptions with links to finding files]

## High Findings

[One-line descriptions with links to finding files]

## Top Recommendations

[3-5 prioritized actions, ordered by risk reduction]

## Components Reviewed

[List with status: reviewed / partially reviewed / skipped]

## Notes

[Anything relevant — new attack surface, changes since last audit, etc.]
```

#### Finding File Format

```markdown
# [SEV-NNN] Short descriptive title

**Severity**: Critical / High / Medium / Low / Informational
**Responsibility**: Platform Issue / Default Issue / Mitigation Gap / User Responsibility
**Component**: e.g., Authentication, gRPC Server, Agent
**File(s)**: `path/to/file.go:line-number`

## Description

What the issue is. Reference exact code — file paths, line numbers, function names.

## Impact

What an attacker could do. Be concrete:
- **Who** can exploit this? (network attacker, rogue agent, malicious user, adjacent container)
- **What** do they gain? (cluster access, data, code execution, denial of service)
- **Blast radius**: one container / one node / entire cluster / all deployments

## Evidence

Code references or steps that demonstrate the issue exists. Not a full exploit — just enough to verify.

## Recommendation

How to fix it. Be specific enough that a developer can act on it without further research.

## Secure Default Consideration

For Default Issues and Mitigation Gaps only:
- What should the default be?
- Should Banyan enforce or warn?
- What's the user override if they need the insecure option?
```

---

## Change Audit Workflow

### Step 1: Scope the Change

- What files changed? (`git diff`, PR description)
- Which components are affected? (Map to Phase 3 components above)
- Does it touch any security-sensitive area from [threat-model.md](threat-model.md)?

### Step 2: Security Impact Assessment

For each changed file, answer:

- Does this change authentication or authorization logic?
- Does this handle user input, external data, or untrusted manifests?
- Does this change network communication, TLS, or encryption?
- Does this modify file permissions or credential storage?
- Does this change container execution, isolation, or privileges?
- Does this add or modify gRPC endpoints?
- Does this change install, setup, or default configuration?

If **none** apply: state "No security-relevant changes identified" and stop.

### Step 3: Targeted Review

For security-relevant changes:
1. Read the changed code and its surrounding context
2. Check against the specific component section in [threat-model.md](threat-model.md)
3. Verify: Does the change introduce a new attack vector? Weaken an existing control? Leak information? Change defaults?

### Step 4: Report

Write a single file for change audits:

```
docs/reports/security-audit/YYYY-MM-DD/CHANGE-AUDIT-<short-description>.md
```

Include the same severity/responsibility classification and finding format for any issues found.

---

## Rules

1. **Never modify source code.** This skill produces reports only.
2. **Read before judging.** Always read the actual code. Don't assume from file names or signatures.
3. **Be specific.** Every finding must reference exact files and line numbers.
4. **No false alarms.** Uncertain? Write "Potential issue — needs verification" with your reasoning.
5. **Assume network access.** Banyan runs on servers that may be on shared networks, cloud VPCs, or the public internet.
6. **Think about defaults.** The question isn't "can this be configured securely?" but "is it secure if the user does nothing special?"
7. **Consider blast radius.** A CLI config bug < an agent bug < an engine auth bug. Rank accordingly.
8. **Check the roadmap.** Planned fixes are still findings — planned doesn't mean fixed.
9. **Don't audit user applications.** Audit Banyan itself, not the containers users run on it.
10. **Compare to prior audits.** If previous reports exist in `docs/reports/security-audit/`, note what's been fixed and what's regressed.
