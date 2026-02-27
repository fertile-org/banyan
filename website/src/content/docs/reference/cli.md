---
title: CLI Reference
description: All commands and flags for banyan-engine, banyan-agent, and banyan-cli.
sidebar:
  order: 1
---

Banyan uses three binaries. Install only what each machine needs.

| Binary | Role | Install on |
|--------|------|------------|
| `banyan-engine` | Control plane (state store, gRPC server, scheduling) | Engine node |
| `banyan-agent` | Worker (task execution, container management) | Worker nodes |
| `banyan-cli` | Client (up, status, logs, dashboard, down) | Any machine |

---

## banyan-engine

Run on your control plane node.

### init

Set up the Engine node: creates data directories and walks you through an interactive setup wizard.

```bash
sudo banyan-engine init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

The wizard generates a WireGuard keypair for authentication and asks:

1. **Etcd setup** — choose **Managed** (Banyan runs etcd for you) or **External** (connect to your own cluster).
2. For **External etcd**: endpoints (e.g. `http://10.0.0.1:2379`) and connection security (None, Username & Password, TLS, or mTLS).

The engine's public key is displayed during init. Share this key with agents and CLI clients that want to use the encrypted WireGuard control tunnel. See [Authentication](/guides/authentication/) for details.

### start

Start the Engine. Launches managed etcd (or connects to external etcd), initializes networking, starts the gRPC server, and watches for deployments.

```bash
sudo banyan-engine start
```

Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--store-backend` | (from config) | Store backend (`etcd`) |
| `--store-address` | (from config) | Etcd endpoint address |
| `--grpc-port` | `50051` | Engine gRPC server port |
| `--vpc-cidr` | `10.0.0.0/16` | VPC network CIDR range |
| `--registry-port` | `5000` | Embedded OCI registry port |

### stop

Stop the Engine.

```bash
sudo banyan-engine stop
```

### status

Show Engine status (agents, deployments, containers). Connects to the configured store backend.

```bash
banyan-engine status
```

| Flag | Default | Description |
|------|---------|-------------|
| `--store-backend` | (from config) | Store backend (`etcd`) |
| `--store-address` | (from config) | Etcd endpoint address |

---

## banyan-agent

Run on each worker node.

### init

Prepare the worker node: creates data directories, verifies containerd and nerdctl are installed, generates a WireGuard keypair, and walks you through an interactive setup wizard.

```bash
sudo banyan-agent init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

The wizard generates a WireGuard keypair and asks:

1. **Engine host** — hostname or IP of the Banyan engine (e.g., `192.168.1.10`).
2. **Engine gRPC port** — default `50051`.
3. **Node name** — unique name for this worker (default: hostname).
4. **Engine WireGuard public key** — displayed during `banyan-engine init` (optional, enables encrypted tunnel).
5. **Tags** — comma-separated tags for environment isolation (optional).

After init, the agent's public key is displayed. Copy it to the engine's whitelisted keys directory. See [Authentication](/guides/authentication/) for details.

If a keypair and engine host are already configured, the wizard skips and shows the current setting.

Pre-write a config file to skip the interactive wizard (useful for automation):

```bash
# Write config with engine connection details
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: 192.168.1.10
    engine_port: "50051"
    tags:
        - staging
EOF

# Init generates a keypair and skips prompts since config exists
sudo banyan-agent init
```

### start

Start the Agent. Connects to the Engine, registers the node, and begins executing tasks.

```bash
sudo banyan-agent start --node-name worker-1
```

The engine endpoint is read from `/etc/banyan/banyan.yaml` (set during `init`). Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--engine` | (from config) | Engine gRPC endpoint override (e.g., `192.168.1.10:50051`) |
| `--node-name` | hostname | Name for this node. Must be unique in the cluster. |
| `--pid-file` | `/var/run/banyan-agent.pid` | Agent PID file |
| `--api-port` | `50052` | Agent gRPC server port (used for log streaming from engine) |
| `--api-address` | | Agent API address override (e.g., `192.168.1.10:50052`) |

### stop

Stop the Agent.

```bash
sudo banyan-agent stop
```

### status

Show the Agent's connection status.

```bash
banyan-agent status
```

| Flag | Default | Description |
|------|---------|-------------|
| `--engine` | (from config) | Engine gRPC endpoint override |

---

## banyan-cli

Run on any machine to manage deployments. Run `banyan-cli init` once to configure the engine connection, then use `up`, `status`, `down`, and `logs` freely.

### init

Configure the CLI with an interactive setup wizard. Generates a WireGuard keypair for authentication.

```bash
sudo banyan-cli init
```

The wizard generates a WireGuard keypair and asks:

1. **Engine host** — hostname or IP of the Banyan engine.
2. **Engine gRPC port** — default `50051`.
3. **CLI name** — unique name for this CLI client (default: `cli-<hostname>`).
4. **Engine WireGuard public key** — displayed during `banyan-engine init` (optional, enables encrypted tunnel).

After init, the CLI's public key is displayed. Copy it to the engine's whitelisted keys directory. See [Authentication](/guides/authentication/) for details.

Run this once on any machine where you want to use `banyan-cli` commands. If a keypair and engine host already exist in the config, you'll be asked whether to overwrite.

### up

Deploy or redeploy an application from a manifest.

```bash
banyan-cli up -f banyan.yaml
```

Sends the deployment to the Engine, then waits for agents to run all containers. Exits when the deployment reaches `running` or `failed` status.

**Redeployment is automatic.** If the application is already running, Banyan uses a blue-green strategy: new containers start alongside old ones, and old containers are torn down only after the new deployment is healthy. If the new deployment fails, old containers keep running. See [Redeployment](/guides/redeployment/) for details.

**Per-service deployment.** Pass service names as arguments to redeploy only those services. Per-service deploys use the same blue-green strategy as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy. Zero downtime. Services not listed are untouched.

`depends_on` is validated: if a service being deployed depends on another service, that dependency must already be running or be included in the same deploy command.

Services with `build:` are built locally, pushed to the Engine's embedded OCI registry, and deployed with the registry-prefixed image name so agents can pull them.

:::tip
`banyan-cli deploy` still works as an alias for `up`. If you're coming from an older version, your scripts don't need to change.
:::

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-f` | `banyan.yaml` | Path to the manifest file |
| `--dry-run` | | `false` | Validate the manifest without deploying |
| `--no-wait` | | `false` | Submit and exit immediately |
| `--tags` | | | Deployment tags for agent matching (comma-separated) |

Examples:

```bash
# Deploy an application
banyan-cli up -f banyan.yaml

# Redeploy after code changes (old containers are replaced automatically)
banyan-cli up -f banyan.yaml

# Redeploy only the web service
banyan-cli up -f banyan.yaml web

# Redeploy web and api together
banyan-cli up -f banyan.yaml web api

# Deploy to staging agents only
banyan-cli up -f banyan.yaml --tags staging

# Validate without deploying
banyan-cli up -f banyan.yaml --dry-run

# Submit and return immediately
banyan-cli up -f banyan.yaml --no-wait
```

### down

Stop and remove services from a deployment.

```bash
banyan-cli down --name my-app
```

Creates `stop_and_remove` tasks for each running container and waits for agents to complete them. By default, stops all services. Pass service names as arguments to stop specific ones.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--name` | | | Application name to stop |
| `--file` | `-f` | | Path to manifest (reads app name from file) |
| `--no-wait` | | `false` | Submit stop tasks and exit immediately |
| `--tags` | | | Deployment tags for matching (comma-separated) |

Examples:

```bash
# Stop all services by name
banyan-cli down --name my-app

# Stop all services (read name from manifest)
banyan-cli down -f banyan.yaml

# Stop specific services only
banyan-cli down --name my-app web db

# Stop a tagged deployment
banyan-cli down --name my-app --tags staging
```

### status

Show cluster status: connected agents, active deployments, and container health.

```bash
banyan-cli status
```

```
Banyan Cluster - Status
========================================
Engine: RUNNING
Connection: 192.168.1.10:50051

Agents: 2
  - worker-1 (status: ready, last seen: 3s ago)
  - worker-2 (status: ready, last seen: 5s ago)

Deployments: 1
  - my-app (status: running, containers: 5/5 healthy)
    web:
      my-app-web-0 on worker-1: running (checked 8s ago)
    api:
      my-app-api-0 on worker-1: running (checked 8s ago)
      my-app-api-1 on worker-2: running (checked 6s ago)
      my-app-api-2 on worker-1: running (checked 8s ago)
    db:
      my-app-db-0 on worker-2: running (checked 6s ago)

========================================
```

### logs

Stream container logs by name.

```bash
banyan-cli logs <container-name>
```

The CLI requests logs from the Engine via gRPC. The Engine proxies them from the agent running the container.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--follow` | `-f` | `false` | Follow log output (like `tail -f`) |
| `--tail` | | `0` | Number of lines from the end (`0` means all) |

Examples:

```bash
# View all logs for a container
banyan-cli logs my-app-web-0

# Follow logs in real time
banyan-cli logs my-app-web-0 -f

# Show last 100 lines and follow
banyan-cli logs my-app-web-0 -f --tail 100
```

### dashboard

Open a live terminal dashboard showing real-time cluster status.

```bash
banyan-cli dashboard
```

The dashboard auto-refreshes and provides six views you can switch between with number keys:

| View | Key | Shows |
|------|-----|-------|
| Overview | `1` | Engine health, cluster summary, agents, deployments, and recent events — all on one screen |
| Agents | `2` | All connected agents with CPU, memory, disk usage, and container count |
| Deploys | `3` | All deployments grouped by name, with health status and version history |
| Containers | `4` | Flat list of all containers across the cluster |
| Engine | `5` | Detailed engine metrics — CPU, memory, disk with progress bars |
| Events | `6` | Full event log — every cluster event from newest to oldest |

Navigate lists with arrow keys or `j`/`k`, press `Enter` to drill into agent or deployment details, and `Esc` to go back. Press `p` to open the command palette for quick view switching, or `?` for keyboard shortcuts.

| Flag | Default | Description |
|------|---------|-------------|
| `--refresh` | `5s` | Auto-refresh interval |

![Dashboard overview screen](/dashboard/dashboard-overview.png)

Examples:

```bash
# Open the dashboard with default 5s refresh
banyan-cli dashboard

# Use a slower refresh interval
banyan-cli dashboard --refresh 30s
```
