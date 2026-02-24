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
sudo banyan-engine init
sudo banyan-engine start
```

The init wizard asks for:
- **Cluster password** — authenticates agents and CLI clients. Stored as a bcrypt hash, never in plain text.
- **Etcd setup** — pick **Managed** (recommended). Banyan runs the data store for you, nothing to configure.

The Engine runs in the foreground. Open a new terminal for the next steps.

## 2. Start an Agent

In a second terminal:

```bash
sudo banyan-agent init
sudo banyan-agent start
```

The init wizard asks for:
- **Engine host** — `localhost` for single-machine setup.
- **Engine port** — default `50051`.
- **Node name** — a name for this worker (default: your hostname).
- **Cluster password** — same password you set on the engine.

The agent connects to the engine, exchanges the password for an auth token, and registers itself. The password is never stored on the agent.

## 3. Configure the CLI

In a third terminal:

```bash
sudo banyan-cli init
```

Same questions: engine host, port, and cluster password. After init, verify the cluster:

```bash
banyan-cli status
```

```
Banyan Cluster - Status
========================================
Engine: RUNNING
Connection: localhost:50051

Agents: 1
  - local-worker (status: ready, last seen: 2s ago)

Deployments: 0
========================================
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
banyan-cli status
```

```
Banyan Cluster - Status
========================================
Engine: RUNNING
Connection: localhost:50051

Agents: 1
  - local-worker (status: ready, last seen: 3s ago)

Deployments: 1
  - my-app (status: running, containers: 3/3 healthy)
    web:
      my-app-web-0 on local-worker: running (checked 8s ago)
      my-app-web-1 on local-worker: running (checked 8s ago)
    redis:
      my-app-redis-0 on local-worker: running (checked 8s ago)

========================================
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
2. The **Engine** scheduled containers across available **Agents** using round-robin.
3. The **Agent** pulled images and started containers using containerd.
4. The CLI waited until all containers reported healthy, then showed success.

Your `banyan.yaml` didn't reference any specific servers. When you add more workers — run `banyan-agent init && banyan-agent start` on another machine — Banyan distributes containers across all of them automatically. **The manifest doesn't change.**

## Next steps

- **[Multi-Node Setup](/guides/multi-node/)** — deploy across multiple servers (your manifest stays the same)
- **[Manifest Reference](/reference/manifest/)** — all banyan.yaml fields including `build`, `environment`, and `command`
- **[CLI Reference](/reference/cli/)** — every command and flag
- **[Authentication](/guides/authentication/)** — how Banyan secures cluster communication
