---
title: Quickstart
description: Run containers across a Banyan cluster.
sidebar:
  order: 2
---

Deploy your first application on Banyan. This guide runs everything on a single machine to show the workflow — the same manifest works unchanged across multiple servers.

Haven't installed yet? Start with [Installation](/getting-started/installation/).

## 1. Start the Engine

```bash
sudo banyan-engine init    # one-time setup + interactive wizard
sudo banyan-engine start
```

`init` runs once. It creates `/etc/banyan/` directories, enables IP forwarding, and walks you through the setup wizard. Both `init` and `start` require `sudo` because the engine manages network interfaces and system services.

The init wizard asks for:
- **Etcd setup** — pick **Managed** (recommended). Banyan runs the data store for you, nothing to configure.

During init, Banyan generates a WireGuard keypair for the engine and creates the whitelisted keys directory at `/etc/banyan/whitelisted-keys/`. The engine's **public key** is displayed — save it if you plan to enable encrypted control tunnels on agents and CLI (see [Authentication](/guides/authentication/#wireguard-control-tunnel)).

The Engine runs in the foreground so you can see its logs. Open a new terminal for the next steps. In production, use `sudo systemctl enable --now banyan-engine` to run as a background service instead.

## 2. Start an Agent

In a second terminal:

```bash
sudo banyan-agent init     # one-time setup + interactive wizard
sudo banyan-agent start
```

`init` runs once. It creates config directories, enables IP forwarding, and walks you through the setup wizard. Both `init` and `start` require `sudo` because the agent manages network interfaces and containers.

The init wizard asks for:
- **Engine host** — `localhost` for single-machine setup.
- **Engine port** — default `50051`.
- **Node name** — a name for this worker (default: your hostname).

During init, Banyan generates a WireGuard keypair and displays the agent's public key. For a single-machine setup, the key is automatically whitelisted. For multi-node setups, copy the public key to the engine (see [Authentication](/guides/authentication/)).

## 3. Configure the CLI

In a third terminal:

```bash
sudo banyan-cli init
```

The wizard asks for engine host and port. It generates a WireGuard keypair and displays whitelisting instructions. `init` and `login` are the only CLI commands that need `sudo` (they create WireGuard kernel interfaces). After init, all other commands run as your normal user:

:::note
The WireGuard tunnel doesn't survive machine reboots. After a restart, run `sudo banyan-cli login` to reconnect — no prompts, no key regeneration.
:::

```bash
banyan-cli engine
```

```
Engine
==================================================
  Status:    running
  Uptime:    5m
  CPU:       2.1% (4 cores)
  Memory:    0.3GB / 4.0GB
  Disk:      8.2GB / 50.0GB

Cluster Summary
--------------------------------------------------
  Agents:       1/1 connected
  Deployments:  0/0 running
  Containers:   0/0 healthy
  Tasks:        0 completed, 0 failed
```

One engine, one agent, ready to deploy.

## 4. Write a manifest

Create a file called `banyan.yaml`:

```yaml
name: my-app

services:
  web:
    image: nginx:alpine
    deploy:
      replicas: 2
    ports:
      - "80:80"

  redis:
    image: redis:7-alpine
```

If you've written a `docker-compose.yml`, this looks familiar. Same `services`, same `image`, same `ports`. The additions: `name` identifies the deployment, and `deploy.replicas` tells Banyan to run 2 instances of the web service.

:::tip
Want to deploy your own code? Use `build: ./your-app` instead of `image:` — Banyan builds the Dockerfile, pushes to its built-in registry, and distributes the image to all workers. See [Manifest Reference](/reference/manifest/#build-from-source).
:::

## 5. Deploy

```bash
banyan-cli up -f banyan.yaml
```

```
Banyan Up
========================================
Application: my-app
Services: 2
  - web: nginx:alpine (replicas: 2)
  - redis: redis:7-alpine (replicas: 1)

Connecting to Engine at localhost:50051...
Deployment 'my-app' created (ID: my-app-1771339609)
Waiting for deployment to complete...
  Status: deploying (tasks dispatched to agents)
  Status: running

========================================
Deployment 'my-app' is RUNNING!
```

:::tip
Run `banyan-cli up` again after changing your manifest or images — Banyan replaces the running containers automatically with zero downtime.
:::

## 6. Verify

```bash
banyan-cli deployment my-app
```

```
Deployment: my-app
============================================================
  ID:       dep-abc123
  Status:   running
  Healthy:  3/3
  Created:  1m ago
  Updated:  30s ago

Services
------------------------------------------------------------
  web
    Image:     nginx:alpine
    Replicas:  2
    Ports:     80:80
  redis
    Image:     redis:7-alpine
    Replicas:  1

Containers
------------------------------------------------------------
  NAME                      STATUS       AGENT           IMAGE
  my-app-web-0              running      local-worker    nginx:alpine
  my-app-web-1              running      local-worker    nginx:alpine
  my-app-redis-0            running      local-worker    redis:7-alpine
```

Three containers running. On a single machine this looks like overkill — the value shows up when you add more servers.

## 7. View logs

Stream logs from any container by name:

```bash
banyan-cli logs my-app-web-0
```

Follow in real time:

```bash
banyan-cli logs my-app-web-0 -f
```

Logs are streamed transparently, even when containers run on remote nodes.

## 8. Tear down

Stop and remove all containers for the deployment:

```bash
banyan-cli down --name my-app
```

```
  Stopping: my-app-web-0 on local-worker
  Stopping: my-app-web-1 on local-worker
  Stopping: my-app-redis-0 on local-worker

Created 3 stop task(s) for deployment 'my-app'
Waiting for services to stop...

========================================
All 3 service(s) stopped successfully.
```

Stop the Agent and Engine with `Ctrl+C` in their terminals, or:

```bash
sudo banyan-agent stop
sudo banyan-engine stop
```

## What just happened

1. The **CLI** sent your manifest to the **Engine**.
2. The **Engine** scheduled containers across available **Agents** — placing each task on the worker with the most available memory.
3. The **Agent** pulled images and started containers using containerd.
4. The CLI waited until all containers reported healthy, then showed success.

Your `banyan.yaml` didn't reference any specific servers. When you add more workers — run `sudo banyan-agent init` on another machine, whitelist its public key on the engine, then `sudo banyan-agent start` — Banyan distributes containers across all of them automatically. **The manifest doesn't change.**

## Next steps

- **[Multi-Node Setup](/guides/multi-node/)** — deploy across multiple servers (your manifest stays the same)
- **[Manifest Reference](/reference/manifest/)** — all banyan.yaml fields including `build`, `environment`, and `command`
- **[CLI Reference](/reference/cli/)** — every command and flag
- **[Authentication](/guides/authentication/)** — how Banyan secures cluster communication
