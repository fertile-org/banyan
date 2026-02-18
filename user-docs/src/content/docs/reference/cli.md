---
title: CLI Reference
description: All banyan-cli commands and flags.
sidebar:
  order: 1
---

## banyan-cli

```
banyan-cli [command]
```

| Command | Description |
|---------|-------------|
| `engine` | Manage the Engine (control plane) |
| `agent` | Manage the Agent (worker node) |
| `deploy` | Deploy applications from a manifest |
| `down` | Stop and remove deployed services |
| `logs` | Stream container logs |

One binary, three roles.

---

## engine

Run on your control plane node.

### engine init

Prepare the Engine node: creates data directories and verifies etcd is installed.

```bash
sudo banyan-cli engine init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

### engine start

Start the Engine. Launches etcd, initializes networking, and watches for deployments.

```bash
sudo banyan-cli engine start
```

Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--etcd` | `http://localhost:2379` | Etcd endpoint |
| `--etcd-client-urls` | `http://0.0.0.0:2379` | Etcd listen address. Use `http://0.0.0.0:2379` for remote access. |
| `--etcd-data-dir` | `/var/lib/banyan/etcd` | Etcd data directory |
| `--etcd-pid-file` | `/var/run/banyan-etcd.pid` | Etcd PID file |
| `--etcd-log-file` | `/var/log/banyan-etcd.log` | Etcd log file |
| `--vpc-cidr` | `10.0.0.0/16` | VPC network CIDR range |
| `--registry-port` | `5000` | Embedded OCI registry port for built images |

### engine stop

Stop the Engine and etcd.

```bash
sudo banyan-cli engine stop
```

### engine status

Show connected agents and active deployments.

```bash
banyan-cli engine status
```

| Flag | Default | Description |
|------|---------|-------------|
| `--etcd` | `http://localhost:2379` | Etcd endpoint |

Example output:

```
Banyan Engine - Status
========================================
etcd: RUNNING
Connection: OK

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

---

## agent

Run on each worker node.

### agent init

Prepare the worker node: creates data directories and verifies containerd and nerdctl are installed.

```bash
sudo banyan-cli agent init
```

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |

### agent start

Start the Agent. Connects to the Engine, registers the node, and begins executing tasks.

```bash
sudo banyan-cli agent start --engine http://192.168.1.10:2379 --node-name worker-1
```

Runs in the foreground. Stop with `Ctrl+C`.

| Flag | Default | Description |
|------|---------|-------------|
| `--data-dir` | `/var/lib/banyan` | Data directory |
| `--engine` | `http://localhost:2379` | Engine etcd endpoint |
| `--node-name` | hostname | Name for this node. Must be unique in the cluster. |
| `--pid-file` | `/var/run/banyan-agent.pid` | Agent PID file |
| `--api-port` | `9090` | Agent API server port (used for remote log streaming) |
| `--api-address` | | Agent API address override (e.g. `192.168.1.10:9090`) |

### agent stop

Stop the Agent.

```bash
sudo banyan-cli agent stop
```

### agent status

Show the Agent's connection status.

```bash
banyan-cli agent status --engine http://192.168.1.10:2379
```

| Flag | Default | Description |
|------|---------|-------------|
| `--engine` | `http://localhost:2379` | Engine etcd endpoint |

---

## deploy

Deploy an application from a `banyan.yaml` manifest.

```bash
banyan-cli deploy -f banyan.yaml
```

Writes the deployment to etcd, then waits for the Engine to schedule and Agents to run all containers. Exits when the deployment reaches `running` or `failed` status.

Services with a `build:` directive are built locally with `nerdctl build`, pushed to the Engine's embedded OCI registry, and deployed with the registry-prefixed image name so agents can pull them.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--file` | `-f` | `banyan.yaml` | Path to the manifest file |
| `--etcd` | | `http://localhost:2379` | Engine etcd endpoint |
| `--dry-run` | | `false` | Validate the manifest without deploying |
| `--no-wait` | | `false` | Submit and exit immediately |

Examples:

```bash
# Deploy locally
banyan-cli deploy -f banyan.yaml

# Deploy to a remote engine
banyan-cli deploy -f banyan.yaml --etcd http://192.168.1.10:2379

# Validate without deploying
banyan-cli deploy -f banyan.yaml --dry-run

# Submit and return immediately
banyan-cli deploy -f banyan.yaml --no-wait
```

---

## down

Stop and remove services from a deployment.

```bash
banyan-cli down --name my-app
```

Creates `stop_and_remove` tasks for each running container and waits for agents to complete them. By default, stops all services. Pass service names as arguments to stop only specific ones.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--name` | | | Application name to stop |
| `--file` | `-f` | | Path to manifest (reads app name from file) |
| `--etcd` | | `http://localhost:2379` | Engine etcd endpoint |
| `--no-wait` | | `false` | Submit stop tasks and exit immediately |

Examples:

```bash
# Stop all services by name
banyan-cli down --name my-app

# Stop all services (read name from manifest)
banyan-cli down -f banyan.yaml

# Stop specific services only
banyan-cli down --name my-app web db

# Stop specific services (read name from manifest)
banyan-cli down -f banyan.yaml web
```

---

## logs

Stream container logs by name.

```bash
banyan-cli logs <container-name>
```

Tries to read logs locally first. If the container is not found on the local machine, queries the cluster via etcd to find which agent runs it, then streams logs from that agent's API.

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--follow` | `-f` | `false` | Follow log output (like `tail -f`) |
| `--tail` | | `0` | Number of lines from the end (`0` means all) |
| `--etcd` | | `http://localhost:2379` | etcd endpoint for cluster lookup |

Examples:

```bash
# View all logs for a container
banyan-cli logs my-app-web-0

# Follow logs in real time
banyan-cli logs my-app-web-0 -f

# Show last 100 lines and follow
banyan-cli logs my-app-web-0 -f --tail 100

# Query a remote cluster
banyan-cli logs my-app-web-0 --etcd http://192.168.1.10:2379
```
