---
title: CLI Reference
description: All commands and flags for banyan-engine, banyan-agent, and banyan-cli.
sidebar:
  order: 1
---

Banyan uses three binaries:

| Binary | Role | Install on |
|--------|------|------------|
| `banyan-engine` | Control plane (etcd, gRPC server, scheduling) | Engine node |
| `banyan-agent` | Worker (task execution, container management) | Worker nodes |
| `banyan-cli` | Client (deploy, status, logs) | Any machine |

---

## banyan-engine

Run on your control plane node.

### init

Prepare the Engine node: creates data directories, verifies etcd is installed, and configures the cluster password.

```bash
sudo banyan-engine init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

During init, you'll be prompted to set a cluster password. This password is stored in `/etc/banyan/banyan.yaml` and must match on all agents and CLI clients.

### start

Start the Engine. Launches etcd, initializes networking, starts the gRPC server, and watches for deployments.

```bash
sudo banyan-engine start
```

Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--etcd` | `http://localhost:2379` | Etcd endpoint |
| `--etcd-client-urls` | `http://0.0.0.0:2379` | Etcd listen address |
| `--etcd-data-dir` | `/var/lib/banyan/etcd` | Etcd data directory |
| `--etcd-pid-file` | `/var/run/banyan-etcd.pid` | Etcd PID file |
| `--etcd-log-file` | `/var/log/banyan-etcd.log` | Etcd log file |
| `--grpc-port` | `50051` | Engine gRPC server port |
| `--vpc-cidr` | `10.0.0.0/16` | VPC network CIDR range |
| `--registry-port` | `5000` | Embedded OCI registry port |

### stop

Stop the Engine and etcd.

```bash
sudo banyan-engine stop
```

### status

Show Engine status (agents, deployments, containers). Connects to etcd directly.

```bash
banyan-engine status
```

| Flag | Default | Description |
|------|---------|-------------|
| `--etcd` | `http://localhost:2379` | Etcd endpoint |
| `--etcd-pid-file` | `/var/run/banyan-etcd.pid` | Etcd PID file |

---

## banyan-agent

Run on each worker node.

### init

Prepare the worker node: creates data directories, verifies containerd and nerdctl are installed, and configures the engine connection and password.

```bash
sudo banyan-agent init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

During init, you'll be prompted for the engine host, gRPC port (default: 50051), and the cluster password.

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

Run on any machine to manage deployments. Before using deploy/status/down/logs commands, run `banyan-cli init` once to configure the engine connection.

### init

Configure the CLI: prompts for engine host, gRPC port, and cluster password.

```bash
sudo banyan-cli init
```

Configuration is stored in `/etc/banyan/banyan.yaml`. Run this once on any machine where you want to use `banyan-cli` commands.

### deploy

Deploy an application from a `banyan.yaml` manifest.

```bash
banyan-cli deploy -f banyan.yaml
```

Sends the deployment to the Engine via gRPC, then waits for agents to run all containers. Exits when the deployment reaches `running` or `failed` status.

Services with a `build:` directive are built locally with `nerdctl build`, pushed to the Engine's embedded OCI registry, and deployed with the registry-prefixed image name so agents can pull them.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-f` | `banyan.yaml` | Path to the manifest file |
| `--dry-run` | | `false` | Validate the manifest without deploying |
| `--no-wait` | | `false` | Submit and exit immediately |

Examples:

```bash
# Deploy locally
banyan-cli deploy -f banyan.yaml

# Validate without deploying
banyan-cli deploy -f banyan.yaml --dry-run

# Submit and return immediately
banyan-cli deploy -f banyan.yaml --no-wait
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
