---
title: Quickstart
description: Deploy your first application in under 5 minutes.
sidebar:
  order: 2
---

This guide runs everything on a single machine to show you the workflow. For multi-server deployments, see [Multi-Node Setup](/guides/multi-node/).

Haven't installed yet? Start with [Installation](/getting-started/installation/).

## 1. Start the Engine

```bash
sudo banyan-cli engine init
sudo banyan-cli engine start
```

The Engine starts etcd and begins watching for deployments. It runs in the foreground — open a new terminal for the next steps.

## 2. Start an Agent

In a second terminal:

```bash
sudo banyan-cli agent init
sudo banyan-cli agent start --node-name local-worker
```

Verify the connection:

```bash
banyan-cli engine status
```

```
Agents: 1
  - local-worker (status: ready, last seen: 2s ago)
```

## 3. Write a manifest

Create `banyan.yaml`:

```yaml
name: my-app

services:
  web:
    image: nginx:alpine
    replicas: 2
    ports:
      - "8080:80"

  api:
    image: alpine:latest
    replicas: 1
    command:
      - sh
      - -c
      - "echo 'API running' && sleep infinity"
    env:
      - APP_ENV=production
```

If you've written a `docker-compose.yml` before, this should look familiar. The only new concept is `replicas` — it tells Banyan how many instances to run.

## 4. Deploy

```bash
banyan-cli deploy -f banyan.yaml
```

```
Banyan Deploy
========================================
Reading manifest: banyan.yaml
Application: my-app
Services: 2
  - web: nginx:alpine (replicas: 2)
  - api: alpine:latest (replicas: 1)

Connecting to Engine at http://localhost:2379...

Deployment 'my-app' created (ID: my-app-1771339609)
Waiting for deployment to complete...
  Status: deploying (tasks dispatched to agents)
  Status: running

========================================
Deployment 'my-app' is RUNNING!
```

## 5. Verify

```bash
banyan-cli engine status
```

```
Agents: 1
  - local-worker (status: ready, last seen: 3s ago)

Deployments: 1
  - my-app (status: running, services: 2, replicas: 3)
```

List the running containers:

```bash
sudo nerdctl ps
```

```
CONTAINER ID    IMAGE                           CREATED          STATUS    PORTS                  NAMES
a1b2c3d4e5f6    docker.io/library/nginx:alpine  10 seconds ago   Up        0.0.0.0:8080->80/tcp   my-app-web-0
b2c3d4e5f6a1    docker.io/library/nginx:alpine  10 seconds ago   Up        0.0.0.0:8080->80/tcp   my-app-web-1
c3d4e5f6a1b2    docker.io/library/alpine:latest 10 seconds ago   Up                               my-app-api-0
```

## 6. Stop

Stop the Agent and Engine with `Ctrl+C` in their terminals, or:

```bash
sudo banyan-cli agent stop
sudo banyan-cli engine stop
```

## What just happened

1. The **Engine** received your manifest and created tasks for each container replica.
2. It assigned tasks to the **Agent** using round-robin scheduling.
3. The **Agent** pulled images and started containers using containerd.
4. The deploy command polled until all containers were running, then reported success.

On a single machine this looks like overkill. The value shows up when you add more servers — your `banyan.yaml` doesn't change at all.

## Next steps

- [Multi-Node Setup](/guides/multi-node/) — distribute containers across multiple servers
- [Manifest Reference](/guides/manifest-reference/) — all banyan.yaml fields and examples
- [CLI Commands](/reference/cli/) — complete command reference
