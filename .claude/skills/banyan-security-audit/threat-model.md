# Banyan Threat Model

Attack surface map for the Banyan container orchestration platform. This document defines trust boundaries, threat actors, protected assets, and component-specific attack vectors.

**Update this document** when Banyan's architecture changes — new components, new communication paths, new storage, or new user-facing features.

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────┐
│                     External Network                         │
│                                                              │
│  ┌─────────┐    gRPC (50051)    ┌──────────────────────┐    │
│  │   CLI   │───────────────────▶│       Engine          │    │
│  └─────────┘                    │  ┌──────┐ ┌────────┐ │    │
│      │                          │  │ etcd │ │Registry│ │    │
│  config file                    │  └──────┘ └────────┘ │    │
│  /etc/banyan/                   └──────────┬───────────┘    │
│                                     gRPC   │  HTTP (5000)   │
│                                            │                │
│                              ┌─────────────┼──────────┐    │
│                              │             ▼          │    │
│                         ┌────────┐    ┌────────┐      │    │
│                         │Agent 1 │    │Agent N │      │    │
│                         │┌──┐┌──┐│    │┌──┐┌──┐│      │    │
│                         ││C1││C2││    ││C3││C4││      │    │
│                         │└──┘└──┘│    │└──┘└──┘│      │    │
│                         └────────┘    └────────┘      │    │
│                              │    VPC Overlay    │      │    │
│                              └──────────────────┘      │    │
└──────────────────────────────────────────────────────────────┘
```

## Protected Assets

| Asset | Where it lives | Impact if compromised |
|-------|---------------|----------------------|
| **Cluster credentials** | Engine config, CLI config, Agent config (`/etc/banyan/banyan.yaml`) | Full cluster takeover — deploy anything, read anything |
| **Auth tokens** | In-memory (engine), config files (CLI/agent) | Impersonate CLI or agent, deploy malicious workloads |
| **Password hash** | Engine config file | Offline brute-force → cluster access |
| **Deployment manifests** | etcd, transmitted via gRPC | May contain secrets in env vars; reveals application architecture |
| **Container images** | Engine registry (port 5000) | IP theft; inject malicious code into deployments |
| **etcd data** | Engine host disk | All cluster state — deployments, agents, tasks, health |
| **Server access** | Agent hosts | If agent is compromised, attacker has container-level (or root) access to host |
| **Inter-service traffic** | VPC overlay network | Application data, API calls, database queries between containers |
| **DNS records** | VPC DNS server | Redirect inter-service traffic to attacker-controlled containers |

## Threat Actors

### 1. Network Attacker
**Access**: Can reach engine/agent ports from the network (same VPC, adjacent server, or internet if ports are exposed).
**Goal**: Join the cluster, deploy malicious containers, intercept traffic, steal data.
**Capability**: Passive eavesdropping, active MITM, port scanning, service enumeration.

### 2. Rogue Agent
**Access**: Has a valid auth token (stolen or from compromised worker node).
**Goal**: Execute unauthorized containers, exfiltrate data from other agents, disrupt cluster operations.
**Capability**: Register as agent, receive tasks, report false health, access overlay network.

### 3. Malicious Insider
**Access**: Has CLI access with valid credentials.
**Goal**: Deploy malicious workloads, extract secrets from manifests, denial of service.
**Capability**: Deploy, inspect status, read logs, potentially access env vars with secrets.

### 4. Supply Chain Attacker
**Access**: Compromises install script download, binary distribution, or base images.
**Goal**: Backdoor all Banyan installations, inject malicious code into the platform itself.
**Capability**: Modify binaries during download, inject malicious dependencies.

### 5. Adjacent Container
**Access**: Running as a legitimate container in the Banyan cluster.
**Goal**: Escape container, access other containers' data, reach the host, pivot to engine.
**Capability**: Network access to other containers via overlay, potential container escape.

## Trust Boundaries

Each boundary is a point where data crosses from one trust level to another. Every boundary needs authentication, authorization, and input validation.

### TB-1: CLI → Engine (gRPC)

```
CLI ──[token in gRPC metadata]──▶ Engine (port 50051)
```

- **Authentication**: Token-based via `x-banyan-auth-token` gRPC metadata
- **Authorization**: Any valid token can perform any CLI operation (no RBAC)
- **Transport**: Check if TLS is enforced or optional
- **Risk**: Token interception on untrusted network → full cluster control
- **Audit focus**: Token transmission security, TLS enforcement, auth interceptor coverage

### TB-2: Agent → Engine (gRPC)

```
Agent ──[token in gRPC metadata]──▶ Engine (port 50051)
```

- **Authentication**: Token-based, same mechanism as CLI
- **Authorization**: Agent-specific RPCs (Register, Heartbeat, PollTasks, ReportTaskResult, ReportContainerHealth)
- **Transport**: Same as TB-1
- **Risk**: Rogue agent registration, false health reports, task result manipulation
- **Audit focus**: Agent identity verification, session token security, can agents call CLI RPCs?

### TB-3: Engine → Agent (gRPC reverse — log streaming)

```
Engine/CLI ──[session token]──▶ Agent gRPC server
```

- **Authentication**: Session token generated by agent, validated with constant-time comparison
- **Transport**: Check TLS
- **Risk**: Session token interception, unauthorized log access
- **Audit focus**: Session token entropy, transmission security

### TB-4: Agent → Registry (HTTP)

```
Agent ──[HTTP pull]──▶ Engine Registry (port 5000)
```

- **Authentication**: Check if registry requires auth
- **Transport**: HTTP vs HTTPS, insecure-registry flags
- **Risk**: Malicious image injection, image tampering, MITM on pull
- **Audit focus**: Registry authentication, image integrity verification, TLS

### TB-5: Engine → etcd

```
Engine ──[client connection]──▶ etcd (port 2379)
```

- **Authentication**: Optional username/password or mTLS
- **Transport**: HTTP or HTTPS depending on configuration
- **Risk**: Direct etcd access = read/write all cluster state
- **Audit focus**: Default auth configuration, TLS, network exposure (localhost only?)

### TB-6: Container → Container (VPC Overlay)

```
Container A ──[overlay network]──▶ Container B
```

- **Authentication**: None (containers communicate freely on overlay)
- **Transport**: VXLAN — check if encrypted (depends on backend)
- **Risk**: Traffic sniffing, DNS spoofing, unauthorized service access
- **Audit focus**: Overlay encryption, network isolation between deployments, DNS integrity

### TB-7: External → Engine/Agent Ports

```
Internet/Network ──▶ Engine (50051), Registry (5000), Agent ports
```

- **Authentication**: gRPC auth interceptor on 50051; check registry on 5000
- **Risk**: Unauthorized access if ports are exposed without firewall
- **Audit focus**: What listens on 0.0.0.0 vs 127.0.0.1? Are ports documented? Does Banyan warn about exposure?

### TB-8: User → Config Files

```
User/Process ──[filesystem]──▶ /etc/banyan/banyan.yaml
```

- **Authentication**: Unix file permissions (should be 0600)
- **Risk**: Credential theft if permissions are wrong
- **Audit focus**: File creation permissions, ownership, are credentials encrypted at rest?

## Component Attack Surface

### Engine

| Attack Vector | Entry Point | Auth Required | Impact |
|--------------|-------------|---------------|--------|
| Deploy malicious manifest | `Deploy` RPC | Yes (token) | Run arbitrary containers on any node |
| Read deployment secrets | `GetStatus` / `GetInfo` RPC | Yes (token) | Env vars may contain secrets |
| Enumerate cluster | `GetStatus` RPC | Yes (token) | Map all agents, containers, IPs |
| DoS via large manifest | `Deploy` RPC | Yes (token) | Resource exhaustion |
| Auth brute force | `ExchangeToken` RPC | Password | Obtain valid token |
| Direct etcd access | etcd port (2379) | Depends on config | Full state read/write |
| Registry abuse | Registry port (5000) | Check | Push malicious images |

### Agent

| Attack Vector | Entry Point | Auth Required | Impact |
|--------------|-------------|---------------|--------|
| Rogue agent registration | `Register` RPC | Yes (token) | Receive tasks, join overlay, report false health |
| Container escape | Container runtime | Container access | Host-level access |
| Image substitution | Image pull from registry | Network position | Execute malicious code |
| Log exfiltration | Agent gRPC server | Session token | Read application logs |
| Command injection via manifest | `nerdctl run` args | Indirect (via manifest) | Arbitrary command execution on host |

### CLI

| Attack Vector | Entry Point | Auth Required | Impact |
|--------------|-------------|---------------|--------|
| Credential theft | Config file on disk | Filesystem access | Impersonate user |
| Password in process list | `--password` CLI flag | Host access | Steal password |
| Manifest tampering | YAML file on disk | Filesystem access | Deploy malicious workload |
| MITM on gRPC | Network between CLI and engine | Network position | Steal token, inject commands |

### Install Script

| Attack Vector | Entry Point | Auth Required | Impact |
|--------------|-------------|---------------|--------|
| Binary substitution | GitHub download (HTTPS) | MITM or compromised CDN | Backdoored binaries |
| Dependency tampering | Package manager or GitHub downloads | MITM | Compromised containerd/etcd/nerdctl |
| curl\|bash interception | Network | MITM | Arbitrary code execution as root |

## Data Flow: Secrets

Trace where secrets live at every stage:

```
1. User types password (CLI init)
   → Transmitted in gRPC metadata (plaintext if no TLS)
   → Engine validates against bcrypt hash in config
   → Token returned in gRPC response
   → Token written to /etc/banyan/banyan.yaml (0600)

2. User writes manifest with env vars
   → YAML file on disk (user-controlled permissions)
   → Transmitted in Deploy RPC (plaintext if no TLS)
   → Stored in etcd (plaintext)
   → Sent to agent in task assignment (gRPC)
   → Passed to container as environment variables

3. Agent session token
   → Generated by agent (random hex)
   → Sent to engine in Register RPC (gRPC metadata)
   → Stored in engine memory (sync.Map)
   → Used by CLI/engine to authenticate to agent's gRPC server
```

**Key observation**: Secrets in environment variables are visible at every stage — manifest file, gRPC transit, etcd storage, agent task, container process. There is no encryption at any point. This is the most common path for credential exposure.

## What to Check on Every Audit

Quick reference for recurring checks:

- [ ] All gRPC endpoints go through auth interceptor (no gaps after new RPCs added)
- [ ] No new unauthenticated HTTP endpoints
- [ ] File permissions on any new config or credential files
- [ ] TLS status (still enforced? new connections using insecure?)
- [ ] New CLI flags that accept secrets (visible in process list?)
- [ ] New etcd keys that store sensitive data
- [ ] Container execution arguments (new flags that weaken isolation?)
- [ ] Install script changes (new downloads without verification?)
- [ ] Default values for new configuration options (secure by default?)
- [ ] Error messages (do they leak paths, versions, internal state?)
