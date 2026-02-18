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
sudo banyan-engine init
sudo banyan-engine start
```

The Engine starts etcd, the gRPC server, and begins watching for deployments. It runs in the foreground — open a new terminal for the next steps.

## 2. Start an Agent

In a second terminal:

```bash
sudo banyan-agent init
sudo banyan-agent start --node-name local-worker
```

In a third terminal, initialize the CLI and verify the connection:

```bash
sudo banyan-cli init
banyan-cli status
```

```
Agents: 1
  - local-worker (status: ready, last seen: 2s ago)
```

## 3. Write a manifest

The repository includes an example at `examples/banyan.yml`. Create your own `banyan.yaml` with the same structure:

```yaml
name: my-app

services:
  web:
    build: ./web
    ports:
      - "80:80"
    depends_on:
      - api

  api:
    build: ./api
    deploy:
      replicas: 3
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=my-app-db-0
      - DB_PORT=5432
    depends_on:
      - db

  db:
    image: postgres:15-alpine
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=banyan
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=app
```

If you've written a `docker-compose.yml` before, this should look familiar. Services with `build:` are built locally and pushed to the Engine's registry. Services with `image:` are pulled directly. `deploy.replicas` tells Banyan how many instances to run.

## 4. Deploy

```bash
banyan-cli deploy -f banyan.yaml
```

```
Banyan Deploy
========================================
Reading manifest: banyan.yaml

Building images...
  Building web → my-app-web:latest
  Building api → my-app-api:latest
Images built successfully.

Pushing images to registry 192.168.1.10:5000...
  Tagging my-app-web:latest → 192.168.1.10:5000/my-app-web:latest
  Pushing 192.168.1.10:5000/my-app-web:latest...
  Tagging my-app-api:latest → 192.168.1.10:5000/my-app-api:latest
  Pushing 192.168.1.10:5000/my-app-api:latest...
Images pushed successfully.

Application: my-app
Services: 3
  - web: 192.168.1.10:5000/my-app-web:latest (replicas: 1)
  - api: 192.168.1.10:5000/my-app-api:latest (replicas: 3)
  - db: postgres:15-alpine (replicas: 1)

Connecting to Engine at localhost:50051...
Deployment 'my-app' created (ID: my-app-1771339609)
Waiting for deployment to complete...
  Status: deploying (tasks dispatched to agents)
  Status: running

========================================
Deployment 'my-app' is RUNNING!
```

## 5. Verify

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
  - my-app (status: running, containers: 5/5 healthy)
    web:
      my-app-web-0 on local-worker: running (checked 8s ago)
    api:
      my-app-api-0 on local-worker: running (checked 8s ago)
      my-app-api-1 on local-worker: running (checked 8s ago)
      my-app-api-2 on local-worker: running (checked 8s ago)
    db:
      my-app-db-0 on local-worker: running (checked 8s ago)

========================================
```

## 6. View logs

Stream logs from any container by name:

```bash
banyan-cli logs my-app-web-0
```

Follow logs in real time:

```bash
banyan-cli logs my-app-web-0 -f
```

If the container runs on a different node, logs are streamed automatically from the remote agent.

## 7. Tear down

Stop and remove all containers for the deployment:

```bash
banyan-cli down --name my-app
```

```
  Stopping: my-app-web-0 on local-worker
  Stopping: my-app-api-0 on local-worker
  Stopping: my-app-api-1 on local-worker
  Stopping: my-app-api-2 on local-worker
  Stopping: my-app-db-0 on local-worker

Created 5 stop task(s) for deployment 'my-app'
Waiting for services to stop...

========================================
All 5 service(s) stopped successfully.
```

You can also stop specific services: `banyan-cli down --name my-app web`

## 8. Stop the cluster

Stop the Agent and Engine with `Ctrl+C` in their terminals, or:

```bash
sudo banyan-agent stop
sudo banyan-engine stop
```

## What just happened

1. The **CLI** sent your manifest to the **Engine** via gRPC.
2. The **Engine** created tasks for each container replica and assigned them to the **Agent** using round-robin scheduling.
3. The **Agent** polled the Engine for tasks, pulled images, and started containers using containerd.
4. The deploy command polled the Engine until all containers were running, then reported success.

On a single machine this looks like overkill. The value shows up when you add more servers — your `banyan.yaml` doesn't change at all.

## Next steps

- [Multi-Node Setup](/guides/multi-node/) — distribute containers across multiple servers
- [Manifest Reference](/guides/manifest-reference/) — all banyan.yaml fields and examples
- [CLI Commands](/reference/cli/) — complete command reference
