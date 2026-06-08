---
title: Authentication
description: How Banyan authenticates components and users.
sidebar:
  order: 1
---

Banyan has two layers of authentication:

| Layer | Purpose | Method |
|-------|---------|--------|
| **Component Auth** | Engine ↔ Agent ↔ CLI | WireGuard public key whitelist |
| **User Auth** | Human users via CLI | JWT tokens (username + password) |

---

## User Authentication (JWT)

Banyan supports multi-user access with role-based permissions. Users authenticate with a username and password, receiving JWT tokens that authorize their CLI commands.

### Quick start

```bash
# Login as admin (created during engine init)
banyan login
# Username: admin
# Password: <your-admin-password>

# Check your identity
banyan whoami
# Username: admin
# Role:     admin
```

### Roles

Banyan has three built-in roles. Each role grants a specific set of permissions:

| Role | Permissions |
|------|-------------|
| **admin** | Full access — manage users, deploy, scale, secrets, everything |
| **deployer** | Deploy and manage deployments, read logs/status, read secrets, change own password |
| **viewer** | Read-only — view deployments, containers, logs, status, change own password |

Roles are **not hierarchical** — a viewer does not inherit deployer permissions. Each role has explicit grants.

### Login and session

When you log in, Banyan issues two tokens:

| Token | Lifetime | Purpose |
|-------|----------|---------|
| **Access token** | 1 hour | Attached to every CLI command for authorization |
| **Refresh token** | 7 days | Used to get a new access token when it expires |

Tokens are stored locally at `~/.config/banyan/credentials.json`. The CLI automatically refreshes your access token when it expires — you don't need to re-login for a week.

```bash
# Login
banyan login

# Login non-interactively (for scripts/CI)
banyan login --username admin --password 'your-password'

# Logout (revokes refresh token on the engine)
banyan logout

# Check current identity
banyan whoami
```

### User management

User management requires **admin** role.

```bash
# List all users
banyan user list
# USERNAME  ROLE      CREATED                    CREATED BY  STATUS
# admin     admin     2026-05-25T17:24:26+07:00  init        active
# alice     deployer  2026-05-28T09:00:00+07:00  admin       active

# Create a new user (default role: viewer)
banyan user add alice --role deployer
# Password for alice: <hidden input>
# User "alice" created with role "deployer"

# Change a user's role
banyan user set-role alice viewer

# Delete a user
banyan user remove alice

# Change your own password
banyan change-password
```

### Security properties

| Property | How |
|----------|-----|
| Passwords hashed with bcrypt (cost 12) | Never stored or transmitted in plaintext |
| Token rotation on refresh | Old refresh token is revoked when a new one is issued |
| Instant role changes | Role is checked against the user store on every request, not cached in the token |
| Account disable | Admins can disable a user — takes effect immediately |
| Last-admin protection | Cannot delete or demote the last active admin |
| Login rate limiting | 10 attempts per minute per IP to prevent brute force |

### How it works

```mermaid
sequenceDiagram
    participant User
    participant CLI as banyan-cli
    participant Engine as banyan-engine
    participant etcd

    Note over User,etcd: Login flow
    User->>CLI: banyan login
    CLI->>Engine: POST /Login (username + password)
    Engine->>Engine: Verify password (bcrypt)
    Engine->>Engine: Create JWT access + refresh tokens
    Engine->>etcd: Store refresh token JTI for revocation
    Engine-->>CLI: Return tokens
    CLI->>CLI: Save to ~/.config/banyan/credentials.json

    Note over User,etcd: Command flow (every CLI command)
    User->>CLI: banyan deploy --file app.yaml
    CLI->>Engine: gRPC + Bearer <access-token>
    Engine->>Engine: Validate JWT signature + expiry
    Engine->>etcd: Check user exists and is not disabled
    Engine->>Engine: Check role has permission for this RPC
    Engine->>Engine: Execute command
    Engine-->>CLI: Response
```

### Password storage

Passwords are stored as bcrypt hashes in etcd. The hash is never exposed — `banyan user list` omits password fields entirely.

### Token lifecycle

1. **Login** → engine creates access token (1h) + refresh token (7d)
2. **Every command** → CLI attaches access token as `Authorization: Bearer <token>`
3. **Token expires** → CLI detects `Unauthenticated` error, calls `RefreshToken` with refresh token
4. **Refresh** → engine validates refresh token, issues new token pair, revokes old refresh token
5. **Logout** → CLI revokes refresh token on engine, deletes local credentials file

---

## Component Authentication (WireGuard)

<span style="display: inline-flex; align-items: center; gap: 0.5rem; padding: 0.5rem 1rem; border: 1px solid var(--sl-color-gray-5); border-radius: 8px; margin: 1rem 0;">
  <img src="/wireguard.webp" alt="WireGuard" style="height: 24px;" />
  <span><strong>Secured by WireGuard&reg;</strong> — All control plane and container traffic encrypted end-to-end.</span>
</span>

Each component (engine, agent, CLI) generates a WireGuard keypair during `init`. The engine validates public keys against a whitelist directory.

### How it works

There are two distinct phases: **init time** (one-time setup) and **runtime** (ongoing operation).

#### Init-time flow

During `init`, each component generates a WireGuard keypair. The admin then copies agent/CLI public keys to the engine's whitelisted keys directory:

```mermaid
sequenceDiagram
    participant Admin
    participant Engine as banyan-engine
    participant Agent as banyan-agent
    participant CLI as banyan-cli

    Note over Admin,Engine: Step 1: Engine init
    Admin->>Engine: banyan-engine init
    Engine->>Engine: Generate X25519 keypair
    Engine->>Engine: Save private key + public key to config
    Engine->>Engine: Create /etc/banyan/whitelisted-keys/

    Note over Admin,Agent: Step 2: Agent init
    Admin->>Agent: banyan-agent init
    Agent->>Agent: Generate X25519 keypair
    Agent->>Agent: Save private key + public key to config
    Agent->>Agent: Display public key

    Note over Admin,Engine: Step 3: Whitelist agent key
    Admin->>Engine: Copy agent public key to engine
    Note right of Admin: echo '<pubkey>' > /etc/banyan/whitelisted-keys/worker-1.pub

    Note over Admin,CLI: Step 4: CLI init
    Admin->>CLI: banyan-cli init
    CLI->>CLI: Generate X25519 keypair
    CLI->>CLI: Save private key + public key to config
    CLI->>CLI: Display public key

    Note over Admin,Engine: Step 5: Whitelist CLI key
    Admin->>Engine: Copy CLI public key to engine
    Note right of Admin: echo '<pubkey>' > /etc/banyan/whitelisted-keys/cli-1.pub

    Note over Admin,Engine: Step 6: Start engine
    Admin->>Engine: sudo systemctl start banyan-engine
    Engine->>Engine: Load all *.pub from whitelisted-keys/
    Engine->>Engine: Start gRPC server with pubkey auth
```

#### Runtime flow

After init, all communication uses the public key in gRPC metadata:

```mermaid
sequenceDiagram
    participant Agent as banyan-agent
    participant Engine as banyan-engine

    Agent->>Engine: gRPC call + public key in x-banyan-public-key header
    Engine->>Engine: Look up public key in whitelist
    alt Key found in whitelist
        Engine->>Engine: Process RPC
        Engine-->>Agent: Response
    else Key not found
        Engine-->>Agent: Unauthenticated error
    end
```

### Config files

After initialization, each component stores its keypair:

**Engine** (`/etc/banyan/banyan.yaml`):
```yaml
engine:
  wg_private_key: "base64-encoded-private-key"
  wg_public_key: "base64-encoded-public-key"
  whitelisted_keys_dir: "/etc/banyan/whitelisted-keys"
  grpc_port: "50051"
  store_backend: "etcd"
```

**Agent** (`/etc/banyan/banyan.yaml`):
```yaml
agent:
  engine_host: "192.168.1.10"
  engine_port: "50051"
  wg_private_key: "base64-encoded-private-key"
  wg_public_key: "base64-encoded-public-key"
```

**CLI** (`/etc/banyan/banyan.yaml`):
```yaml
cli:
  engine_host: "192.168.1.10"
  engine_port: "50051"
  wg_private_key: "base64-encoded-private-key"
  wg_public_key: "base64-encoded-public-key"
```

### Key management

- **Generate**: Running `init` on any component generates a new X25519 keypair.
- **Whitelist**: Copy the public key (displayed during init) to the engine's whitelisted keys directory as a `.pub` file.
- **Revoke**: Delete the `.pub` file from the engine's whitelisted keys directory and restart the engine.
- **Rotate**: Re-run `init` on the component to generate a new keypair, then update the `.pub` file on the engine.

### Whitelisting keys

After running `init` on an agent or CLI, copy the displayed public key to the engine:

```bash
# On the agent machine (after banyan-agent init)
cat /etc/banyan/banyan.yaml | grep wg_public_key
# Output: wg_public_key: "abc123..."

# On the engine machine
sudo banyan-engine add-client --name worker-1 --pubkey 'abc123...'
```

Or copy directly between machines:

```bash
# From agent to engine
ssh engine-host "echo '$(grep wg_public_key /etc/banyan/banyan.yaml | awk '{print $2}')' > /etc/banyan/whitelisted-keys/worker-1.pub"
```

The filename (minus `.pub`) becomes the agent's display name in logs.

### Non-interactive init

All `init` commands can be run non-interactively by pre-writing the config file:

```bash
# Write config first
cat > /etc/banyan/banyan.yaml <<EOF
agent:
  engine_host: "192.168.1.10"
  engine_port: "50051"
EOF

# Run init (generates keypair, skips prompts since config exists)
sudo banyan-agent init
```

### WireGuard control tunnel

When the engine's WireGuard public key is provided during agent/CLI init, Banyan creates a `wg-control` WireGuard interface that encrypts all gRPC traffic at the network layer. No TLS is needed — the tunnel handles encryption transparently.

```
Control plane tunnel (wg-control):
  Engine (10.200.0.1) <-> Agent (10.200.X.Y)    # encrypted gRPC
  Engine (10.200.0.1) <-> CLI   (10.200.X.Y)    # encrypted gRPC

Data plane tunnel (banyan-wg):
  Agent <-> Agent                                 # encrypted container traffic
```

**How it works:**
1. Engine generates a keypair during `banyan-engine init` and displays its public key.
2. Agents/CLI provide the engine's public key during their `init`.
3. On start, each component creates a `wg-control` interface with a deterministic tunnel IP derived from its public key.
4. gRPC traffic routes through the tunnel automatically.
5. If the tunnel is unavailable (e.g., WireGuard not installed), components fall back to direct TCP with public key metadata auth.

**Port:** `51821/UDP` on the engine (agents/CLI connect to this port).

**Config fields:**
```yaml
agent:
  engine_wg_public_key: "engine-public-key-base64"  # enables control tunnel

cli:
  engine_wg_public_key: "engine-public-key-base64"  # enables control tunnel
```

### Security properties

| Property | How |
|----------|-----|
| No passwords or shared secrets | Each component has a unique keypair. No cluster-wide password. |
| Keys are standard X25519 | Same format as WireGuard. 32 bytes, base64-encoded. |
| Dual-purpose keypair | Same key for gRPC auth (control plane) and WireGuard tunnels (data plane). |
| Per-component revocation | Delete a single `.pub` file to revoke access for one agent/CLI. |
| No token storage in etcd | Public keys are stored as flat files on the engine. No etcd dependency for auth. |
| Private keys never leave the machine | Only the public key is copied to the engine. |
| Control plane encryption | When enabled, all gRPC traffic is encrypted via WireGuard tunnel. |

---

## mTLS (planned)

Mutual TLS authentication will allow components to authenticate using X.509 client certificates instead of public keys.

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
    IdP-->>CLI: ID token
    CLI->>Engine: Authenticate with ID token
    Engine->>IdP: Verify token
    Engine-->>CLI: Whitelisted
    CLI->>CLI: Save config
```

This will be suitable for teams with centralized identity management. See the [roadmap](/roadmap/) for status.
