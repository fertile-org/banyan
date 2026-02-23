---
title: CLI Reference
description: All commands and flags for banyan-engine, banyan-agent, and banyan-cli.
sidebar:
  order: 1
---

Banyan uses three binaries:

| Binary | Role | Install on |
|--------|------|------------|
| `banyan-engine` | Control plane (store backend, gRPC server, scheduling) | Engine node |
| `banyan-agent` | Worker (task execution, container management) | Worker nodes |
| `banyan-cli` | Client (up, status, logs, down) | Any machine |

---

## banyan-engine

Run on your control plane node.

### init

Prepare the Engine node: creates data directories and walks you through an interactive setup wizard.

```bash
sudo banyan-engine init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--password` | | Cluster password (skips interactive prompt) |

The wizard asks:

1. **Cluster password** — used to authenticate agents and CLI clients. The password is hashed with bcrypt and stored in `/etc/banyan/banyan.yaml` — the plain-text password is never saved.
2. **Etcd setup** — choose **Managed** (Banyan runs etcd for you) or **External** (connect to your own cluster).
3. For **External etcd**: endpoints (e.g. `http://10.0.0.1:2379`) and connection security (None, Username & Password, TLS, or mTLS).

If a password hash is already configured, the wizard skips that step and shows the current setting.

Pass `--password` to set the cluster password non-interactively (useful for automation and CI/CD):

```bash
sudo banyan-engine init --password "my-cluster-secret"
```

### start

Start the Engine. Starts managed etcd (or connects to external etcd), initializes networking, starts the gRPC server, and watches for deployments.

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

### auth

Re-authenticate with the engine. Reads engine connection details from the existing config and prompts only for the cluster password to obtain a new token.

```bash
sudo banyan-agent auth
```

| Flag | Default | Description |
|------|---------|-------------|
| `--password` | | Cluster password (skips interactive prompt) |

Use this when a token has been revoked or the engine has been re-initialized — it avoids re-running the full `init` wizard. Requires an existing config (from a previous `init`).

### init

Prepare the worker node: creates data directories, verifies containerd and nerdctl are installed, and walks you through an interactive setup wizard.

```bash
sudo banyan-agent init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--password` | | Cluster password (skips interactive prompt) |

The wizard asks:

1. **Engine host** — hostname or IP of the Banyan engine (e.g. `192.168.1.10`).
2. **Engine gRPC port** — default `50051`.
3. **Node name** — unique name for this worker (default: hostname).
4. **Cluster password** — must match the engine password.

The wizard connects to the running engine and exchanges the password for an auth token. Only the token and node name are stored in the config — the password is never saved. The engine must be running during agent init.

If an auth token is already configured, the wizard skips and shows the current setting.

Pass `--password` with a pre-written config file to skip the interactive wizard entirely (useful for automation):

```bash
# Write config with engine connection details first
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: 192.168.1.10
    engine_port: "50051"
EOF

# Init exchanges the password for a token non-interactively
sudo banyan-agent init --password "my-cluster-secret"
```

### start

Start the Agent. Connects to the Engine via gRPC, registers the node, and begins executing tasks.

```bash
sudo banyan-agent start --node-name worker-1
```

The engine endpoint is read from `/etc/banyan/banyan.yaml` (set during `init`). Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--engine` | (from config) | Engine gRPC endpoint override (e.g. `192.168.1.10:50051`) |
| `--node-name` | hostname | Name for this node. Must be unique in the cluster. |
| `--pid-file` | `/var/run/banyan-agent.pid` | Agent PID file |
| `--api-port` | `50052` | Agent gRPC server port (used for log streaming from engine) |
| `--api-address` | | Agent API address override (e.g. `192.168.1.10:50052`) |

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

Run on any machine to manage deployments. Before using up/status/down/logs commands, run `banyan-cli init` once to configure the engine connection.

### auth

Re-authenticate with the engine. Reads engine connection details from the existing config and prompts only for the cluster password to obtain a new token.

```bash
sudo banyan-cli auth
```

| Flag | Default | Description |
|------|---------|-------------|
| `--password` | | Cluster password (skips interactive prompt) |

Use this when a token has been revoked or the engine has been re-initialized — it avoids re-running the full `init` wizard. Requires an existing config (from a previous `init`).

### init

Configure the CLI with an interactive setup wizard.

```bash
sudo banyan-cli init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--password` | | Cluster password (skips interactive prompt) |

The wizard asks:

1. **Engine host** — hostname or IP of the Banyan engine.
2. **Engine gRPC port** — default `50051`.
3. **CLI name** — unique name for this CLI client (default: `cli-<hostname>`).
4. **Cluster password** — must match the engine password.

The wizard connects to the running engine and exchanges the password for an auth token. Only the token is stored in `/etc/banyan/banyan.yaml` — the password is never saved. The engine must be running during CLI init.

Run this once on any machine where you want to use `banyan-cli` commands. If an auth token already exists in the config, you'll be asked whether to overwrite it.

Pass `--password` with a pre-written config file to skip the interactive wizard entirely (useful for automation):

```bash
# Write config with engine connection details first
cat > /etc/banyan/banyan.yaml <<EOF
cli:
    engine_host: 192.168.1.10
    engine_port: "50051"
EOF

# Init exchanges the password for a token non-interactively
sudo banyan-cli init --password "my-cluster-secret"
```

### up

Deploy an application from a `banyan.yaml` manifest.

```bash
banyan-cli up -f banyan.yaml
```

Sends the deployment to the Engine via gRPC, then waits for agents to run all containers. Exits when the deployment reaches `running` or `failed` status.

**Redeployment is automatic.** If the application is already running, Banyan uses a blue-green strategy: it starts the new containers alongside the old ones, waits for the new deployment to reach `running` status, then tears down the old containers. If the new deployment fails, the old containers keep running — no downtime.

Services with a `build:` directive are built locally with `nerdctl build`, pushed to the Engine's embedded OCI registry, and deployed with the registry-prefixed image name so agents can pull them.

:::tip
`banyan-cli deploy` still works as an alias for `up`. If you're coming from an older version, your scripts don't need to change.
:::

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-f` | `banyan.yaml` | Path to the manifest file |
| `--dry-run` | | `false` | Validate the manifest without deploying |
| `--no-wait` | | `false` | Submit and exit immediately |

Examples:

```bash
# Deploy an application
banyan-cli up -f banyan.yaml

# Redeploy after code changes (old containers are replaced automatically)
banyan-cli up -f banyan.yaml

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

Creates `stop_and_remove` tasks for each running container and waits for agents to complete them. By default, stops all services. Pass service names as arguments to stop only specific ones.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--name` | | | Application name to stop |
| `--file` | `-f` | | Path to manifest (reads app name from file) |
| `--no-wait` | | `false` | Submit stop tasks and exit immediately |

Examples:

```bash
# Stop all services by name
banyan-cli down --name my-app

# Stop all services (read name from manifest)
banyan-cli down -f banyan.yaml

# Stop specific services only
banyan-cli down --name my-app web db
```

### status

Show cluster status: connected agents, active deployments, and container health.

```bash
banyan-cli status
```

Example output:

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
