---
title: Secure Your Cluster
description: How Banyan authenticates engines, agents, and CLI clients.
sidebar:
  order: 1
---

Banyan authenticates all communication between components. You set a cluster password once during setup — Banyan handles the rest. The password is exchanged for a token, and the token is used for all subsequent calls. The plain-text password is never stored on disk.

## How authentication works

| Method | Status | How it works |
|--------|--------|--------------|
| **Password + Token Exchange** | Available | Password used once at init to obtain a long-lived auth token |
| **mTLS** | Planned | Mutual TLS with client certificates |
| **OIDC / SSO** | Planned | Delegate authentication to an identity provider |

---

## Password + Token Exchange

This is Banyan's default and only current authentication method. Here's the full picture.

### Init time — one-time setup

During `init`, each component exchanges the cluster password for an auth token:

```mermaid
sequenceDiagram
    participant User
    participant Engine as banyan-engine
    participant etcd as etcd store
    participant Agent as banyan-agent
    participant CLI as banyan-cli

    Note over User,Engine: Step 1: Engine init
    User->>Engine: banyan-engine init [--password secret]
    Engine->>Engine: Get password (prompt or --password flag)
    Engine->>Engine: bcrypt(password) → hash
    Engine->>Engine: Save hash to /etc/banyan/banyan.yaml

    Note over User,Engine: Step 2: Engine start
    User->>Engine: banyan-engine start
    Engine->>Engine: Load password_hash from config
    Engine->>Engine: Start gRPC server with dual-mode auth

    Note over User,Agent: Step 3: Agent init (engine must be running)
    User->>Agent: banyan-agent init [--password secret]
    Agent->>Agent: Get config (prompt or config file + --password flag)
    Agent->>Engine: ExchangeToken(name, password)
    Engine->>Engine: bcrypt.Compare(password, stored_hash)
    Engine->>Engine: Generate random 256-bit token
    Engine->>etcd: Store sha256(token) → name
    Engine->>etcd: Store name → sha256(token)
    Engine-->>Agent: Return token
    Agent->>Agent: Save token + node_name to config (NOT password)

    Note over User,CLI: Step 4: CLI init (same flow as agent)
    User->>CLI: banyan-cli init
    CLI->>Engine: ExchangeToken(name, password)
    Engine-->>CLI: Return token
    CLI->>CLI: Save token to config (NOT password)
```

After init, each component has an auth token. The password isn't needed again until you add a new agent or CLI client.

### Runtime — ongoing operation

Every gRPC call carries the auth token. The engine validates it against the token store:

```mermaid
sequenceDiagram
    participant Agent as banyan-agent
    participant Engine as banyan-engine
    participant etcd as etcd store

    Agent->>Engine: gRPC call + token in x-banyan-auth-token header
    Engine->>Engine: sha256(token) → token_hash
    Engine->>etcd: Lookup /banyan/tokens/{token_hash}
    alt Token found
        etcd-->>Engine: name
        Engine->>Engine: Process RPC
        Engine-->>Agent: Response
    else Token not found
        Engine-->>Agent: Unauthenticated error
    end
```

### What gets stored where

After initialization, each component stores only what it needs:

**Engine** (`/etc/banyan/banyan.yaml`):
```yaml
engine:
  password_hash: "$2a$10$..."   # bcrypt hash — never the plain password
  grpc_port: "50051"
  store_backend: "etcd"
```

**Agent** (`/etc/banyan/banyan.yaml`):
```yaml
agent:
  engine_host: "192.168.1.10"
  engine_port: "50051"
  auth_token: "a1b2c3d4e5f6..."  # random token from ExchangeToken
  node_name: "worker-1"
```

**CLI** (`/etc/banyan/banyan.yaml`):
```yaml
cli:
  engine_host: "192.168.1.10"
  engine_port: "50051"
  auth_token: "e5f6g7h8i9j0..."  # random token from ExchangeToken
```

### Token lifecycle

```mermaid
stateDiagram-v2
    [*] --> NoToken: Fresh install
    NoToken --> Active: banyan-agent init / banyan-cli init<br/>(ExchangeToken RPC)
    Active --> Active: Normal operation<br/>(token in every gRPC call)
    Active --> Revoked: Re-run init<br/>(new token replaces old)
    Revoked --> Active: New token issued
    Active --> Invalid: Engine re-initialized<br/>(all tokens wiped)
    Invalid --> Active: Re-run init
    Active --> Expired: CLI token TTL exceeded<br/>(30 days)
    Expired --> Active: banyan-cli auth
```

- **Issue**: Running `init` on an agent or CLI generates a new random token and stores it in etcd.
- **Re-issue**: Running `init` again for the same name revokes the old token and issues a new one. The old token stops working immediately.
- **CLI token TTL**: CLI tokens expire after **30 days**. Run `banyan-cli auth` to re-authenticate. Agent tokens don't expire because agents run on controlled infrastructure.
- **Revoke all**: Re-initializing the engine (or wiping etcd) invalidates all tokens. All agents and CLI clients must re-authenticate.

### Re-authentication

If a token is revoked or the engine is re-initialized, you don't need to re-run the full `init` wizard. The `auth` command reads the existing config and only prompts for the cluster password:

```bash
# Agent
sudo banyan-agent auth

# CLI
sudo banyan-cli auth
```

This is faster than `init` — it skips directory creation, dependency checks, and connection prompts. Requires an existing config from a previous `init`.

### Automation and CI/CD

All `init` commands accept `--password` to skip interactive prompts:

```bash
# Engine
sudo banyan-engine init --password "my-cluster-secret"

# Agent (write connection details first, then init exchanges for a token)
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: 192.168.1.10
    engine_port: "50051"
EOF
sudo banyan-agent init --password "my-cluster-secret"

# CLI (same pattern)
cat > /etc/banyan/banyan.yaml <<EOF
cli:
    engine_host: 192.168.1.10
    engine_port: "50051"
EOF
sudo banyan-cli init --password "my-cluster-secret"
```

The password is used once for the token exchange and never written to disk.

### Security properties

| Property | How |
|----------|-----|
| Password never stored in plain text | Engine stores bcrypt hash; agents/CLI never store the password |
| Tokens are random and unique | 256-bit cryptographically random, hex-encoded |
| Token hashing in etcd | Stored as SHA-256 hashes — etcd compromise doesn't leak usable tokens |
| Per-name revocation | Re-running init for a name revokes only that name's token |
| No replay across names | Each token is bound to a specific name via dual index |
| CLI token TTL | CLI tokens expire after 30 days, limiting exposure from compromised workstations |

---

## mTLS (planned)

Mutual TLS authentication will allow components to authenticate using X.509 client certificates instead of password-derived tokens.

```mermaid
sequenceDiagram
    participant CA as Certificate Authority
    participant Engine as banyan-engine
    participant Agent as banyan-agent

    Note over CA,Agent: Setup (one-time)
    CA->>Engine: Issue server cert + key
    CA->>Agent: Issue client cert + key

    Note over Engine,Agent: Runtime
    Agent->>Engine: gRPC + TLS handshake (client cert)
    Engine->>Engine: Verify client cert against CA
    Engine-->>Agent: Authenticated connection
```

This will be suitable for environments with existing PKI infrastructure or stricter security requirements. See the [roadmap](/roadmap/) for status.

---

## OIDC / SSO (planned)

OpenID Connect integration will allow Banyan to delegate authentication to an external identity provider (e.g., Google, Okta, Keycloak).

```mermaid
sequenceDiagram
    participant User
    participant CLI as banyan-cli
    participant IdP as Identity Provider
    participant Engine as banyan-engine

    User->>CLI: banyan-cli init --auth oidc
    CLI->>IdP: Redirect to login
    User->>IdP: Authenticate
    IdP-->>CLI: ID token + access token
    CLI->>Engine: ExchangeToken(id_token)
    Engine->>IdP: Verify token
    Engine-->>CLI: Banyan auth token
    CLI->>CLI: Save token to config
```

This will be suitable for teams with centralized identity management. See the [roadmap](/roadmap/) for status.
