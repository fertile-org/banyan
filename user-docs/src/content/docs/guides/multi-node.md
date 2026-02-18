---
title: Multi-Node Setup
description: Deploy containers across multiple servers.
sidebar:
  order: 2
---

This is where Banyan earns its keep. Your `banyan.yaml` doesn't change — you just have more servers running Agents.

## Architecture

```
                    +-----------+
                    |  Engine   |  (control plane)
                    |  + etcd   |
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
        +-----+-----+          +-----+-----+
        |  Worker 1  |          |  Worker 2  |
        |  Agent     |          |  Agent     |
        |  containerd|          |  containerd|
        +------------+          +------------+
```

The Engine orchestrates. Workers run containers. They communicate through etcd.

## Prerequisites

Install `banyan-cli` on all servers. See [Installation](/getting-started/installation/).

- **Engine node**: needs etcd
- **Worker nodes**: need containerd and nerdctl

## 1. Start the Engine

On your Engine server (e.g., `192.168.1.10`):

```bash
sudo banyan-cli engine init
sudo banyan-cli engine start --etcd-client-urls http://0.0.0.0:2379
```

The `--etcd-client-urls http://0.0.0.0:2379` makes etcd listen on all interfaces so workers can connect.

Verify from a worker machine:

```bash
curl http://192.168.1.10:2379/health
# {"health":"true"}
```

## 2. Start the Agents

On Worker 1 (`192.168.1.11`):

```bash
sudo banyan-cli agent init
sudo banyan-cli agent start \
  --engine http://192.168.1.10:2379 \
  --node-name worker-1
```

On Worker 2 (`192.168.1.12`):

```bash
sudo banyan-cli agent init
sudo banyan-cli agent start \
  --engine http://192.168.1.10:2379 \
  --node-name worker-2
```

Each Agent registers with the Engine and starts a heartbeat.

## 3. Verify the cluster

```bash
banyan-cli engine status
```

```
Agents: 2
  - worker-1 (status: ready, last seen: 2s ago)
  - worker-2 (status: ready, last seen: 3s ago)

Deployments: 0
```

## 4. Deploy

The same manifest from the [Quickstart](/getting-started/quickstart/) works here without changes. Banyan distributes replicas across workers automatically.

```yaml
name: my-app

services:
  web:
    image: nginx:alpine
    replicas: 4
    ports:
      - "80:80"

  api:
    image: hashicorp/http-echo:latest
    replicas: 2
    env:
      - APP_ENV=production
```

```bash
banyan-cli deploy -f banyan.yaml --etcd http://192.168.1.10:2379
```

Banyan distributes 6 containers across 2 workers using round-robin:

| Worker 1 | Worker 2 |
|----------|----------|
| my-app-web-0 | my-app-web-1 |
| my-app-web-2 | my-app-web-3 |
| my-app-api-0 | my-app-api-1 |

## 5. Check containers on workers

SSH into each worker and list running containers:

```bash
sudo nerdctl ps
```

## Deploying from a remote machine

You don't need to run `deploy` from the Engine node. Any machine with `banyan-cli` can deploy as long as it can reach etcd:

```bash
banyan-cli deploy -f banyan.yaml --etcd http://192.168.1.10:2379
```

## Adding more workers

1. Install `banyan-cli`, containerd, and nerdctl on the new server.
2. Run `sudo banyan-cli agent init`
3. Run `sudo banyan-cli agent start --engine http://<engine-ip>:2379 --node-name worker-3`

The new worker appears in `engine status` within seconds. Future deployments include it automatically.

## Firewall requirements

| Port | Protocol | Direction | Purpose |
|------|----------|-----------|---------|
| 2379 | TCP | Workers to Engine | etcd client communication |

Workers don't need to communicate with each other directly.
