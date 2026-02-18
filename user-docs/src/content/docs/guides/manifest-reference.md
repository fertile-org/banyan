---
title: Manifest Reference
description: Complete reference for the banyan.yaml configuration file.
sidebar:
  order: 1
---

## If you know Docker Compose

Banyan's manifest format is based on Docker Compose. Here's what carries over and what's different:

| Concept | Docker Compose | Banyan | Notes |
|---------|---------------|--------|-------|
| Services | `services:` | `services:` | Same |
| Image | `image:` | `image:` | Same |
| Ports | `ports: ["80:80"]` | `ports: ["80:80"]` | Same format |
| Environment | `environment:` | `env:` | Shorter key name |
| Command | `command:` | `command:` | Same |
| Dependencies | `depends_on:` | `depends_on:` | Same (informational) |
| Replicas | `deploy.replicas:` | `replicas:` | Top-level, not nested |
| App name | Inferred from directory | `name:` | Explicit in Banyan |
| Volumes | `volumes:` | -- | Not yet supported |
| Networks | `networks:` | -- | Managed automatically |

The biggest difference: `replicas` is a top-level field on each service, not buried under `deploy`. This keeps the manifest flat and readable.

## Structure

```yaml
name: <application-name>    # Required

services:
  <service-name>:           # One or more services
    image: <image>          # Required
    replicas: <number>      # Default: 1
    ports:
      - "<host>:<container>"
    env:
      - KEY=value
    command:
      - <arg1>
      - <arg2>
    depends_on:
      - <other-service>
```

## Fields

### Top-level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Application name. Used as a prefix for container names. |
| `services` | map | Yes | Map of service definitions. At least one service required. |

### Service

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | Yes | -- | Container image. Any registry works: `nginx:alpine`, `ghcr.io/org/app:v1`. |
| `replicas` | integer | No | `1` | Number of container instances. Distributed across available workers. |
| `ports` | list | No | -- | Port mappings in `host:container` format. |
| `env` | list | No | -- | Environment variables in `KEY=value` format. |
| `command` | list | No | -- | Override the container's default command. Each argument is a list item. |
| `depends_on` | list | No | -- | Services that should start first. Currently informational only. |

## Container naming

Containers follow the pattern: `<app-name>-<service-name>-<replica-index>`

For `name: my-app` with service `web` and 3 replicas:
- `my-app-web-0`
- `my-app-web-1`
- `my-app-web-2`

## Examples

### Minimal

```yaml
name: hello

services:
  web:
    image: nginx:alpine
```

One container on one worker.

### Web application with database

```yaml
name: webapp

services:
  frontend:
    image: nginx:alpine
    replicas: 3
    ports:
      - "80:80"

  api:
    image: hashicorp/http-echo:latest
    replicas: 2
    ports:
      - "3000:3000"
    env:
      - DATABASE_URL=postgres://db:5432/app
      - REDIS_URL=redis://cache:6379
    depends_on:
      - db

  db:
    image: postgres:16-alpine
    replicas: 1
    ports:
      - "5432:5432"
    env:
      - POSTGRES_DB=app
      - POSTGRES_USER=admin
      - POSTGRES_PASSWORD=secret

  cache:
    image: redis:7-alpine
    replicas: 1
    ports:
      - "6379:6379"
```

### Background workers

```yaml
name: pipeline

services:
  worker:
    image: hashicorp/http-echo:latest
    replicas: 5
    env:
      - QUEUE_URL=amqp://rabbitmq:5672
      - CONCURRENCY=4
    command:
      - ./worker
      - --queue
      - jobs

  scheduler:
    image: hashicorp/http-echo:latest
    replicas: 1
    env:
      - SCHEDULE_INTERVAL=60s
```

## Validation

Check your manifest without deploying:

```bash
banyan-cli deploy -f banyan.yaml --dry-run
```

This parses the file, checks for errors, and prints the services that would be deployed.
