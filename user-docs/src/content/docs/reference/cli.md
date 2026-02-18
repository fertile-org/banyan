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
  - my-app (status: running, services: 2, replicas: 6)

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
