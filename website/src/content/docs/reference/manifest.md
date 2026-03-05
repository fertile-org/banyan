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
| Environment | `environment:` | `environment:` | Same |
| Command | `command:` | `command:` | Same |
| Dependencies | `depends_on:` | `depends_on:` | Same (informational for full deploys; validated for per-service deploys) |
| Replicas | `deploy.replicas:` | `deploy.replicas:` | Same |
| Placement | `deploy.placement.constraints:` | `deploy.placement.node:` | Glob pattern for node name matching |
| App name | Inferred from directory | `name:` | Explicit in Banyan |
| Build | `build:` | `build:` | Same syntax (context + dockerfile) |
| Restart | `restart:` | `restart:` | Same |
| Entrypoint | `entrypoint:` | `entrypoint:` | Same |
| Resource limits | `deploy.resources:` | `deploy.resources:` | Same (memory, cpus) |
| Healthcheck | `healthcheck:` | -- | Planned |
| Volumes | `volumes:` | -- | Planned |
| Networks | `networks:` | -- | Managed automatically |
| Labels | `labels:` | -- | Not supported — Banyan uses built-in service DNS and load balancing instead of label-based service discovery |

If you already write Docker Compose files, you already know most of this.

## Structure

```yaml
name: <application-name>    # Required

services:
  <service-name>:           # One or more services
    image: <image>          # Required (unless build is set)
    build: <context-path>   # Build from Dockerfile
    restart: unless-stopped # Restart policy
    entrypoint:             # Override ENTRYPOINT
      - <binary>
    deploy:
      replicas: <number>    # Default: 1
      placement:
        node: <pattern>     # Glob pattern for node name
      resources:
        limits:
          memory: 512m
          cpus: "0.5"
        reservations:
          memory: 256m
    ports:
      - "<host>:<container>"
    environment:
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
| `name` | string | Yes | Application name. Used as a prefix for container names and to identify the app during redeployment. |
| `services` | map | Yes | Map of service definitions. At least one service required. |

### Service

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | Conditional | -- | Container image. Required unless `build` is set. Any registry works: `nginx:alpine`, `ghcr.io/org/app:v1`. |
| `build` | string or object | No | -- | Build from a Dockerfile. See [Build](#build) below. |
| `deploy.replicas` | integer | No | `1` | Number of container instances. Distributed across available workers. |
| `deploy.placement.node` | string | No | -- | Glob pattern to pin this service to specific nodes. Supports `*`, `?`, and `[abc]`. Example: `gateway-*` matches `gateway-1`, `gateway-2`. |
| `deploy.resources.limits.memory` | string | No | -- | Memory limit (e.g., `512m`, `1g`). Container is killed if it exceeds this. |
| `deploy.resources.limits.cpus` | string | No | -- | CPU limit (e.g., `"0.5"`, `"2"`). Fractional cores allowed. |
| `deploy.resources.reservations.memory` | string | No | -- | Soft memory limit (e.g., `256m`). Used as a reservation hint. |
| `restart` | string | No | `no` | Restart policy: `no`, `always`, `unless-stopped`, `on-failure`, or `on-failure:N`. |
| `entrypoint` | string or list | No | -- | Override the container's ENTRYPOINT. Supports string or list form. |
| `ports` | list | No | -- | Port mappings in `host:container` format. |
| `environment` | list | No | -- | Environment variables in `KEY=value` format. |
| `command` | list | No | -- | Override the container's default command. Each argument is a list item. |
| `depends_on` | list | No | -- | Services that should start first. Validated during [per-service deploys](/guides/redeployment/#dependency-validation). |

## Container naming

Containers follow the pattern: `<app-name>-<service-name>-<replica-index>`

For `name: my-app` with service `web` and 3 replicas:
- `my-app-web-0`
- `my-app-web-1`
- `my-app-web-2`

:::note
During a [blue-green redeployment](/guides/redeployment/), new containers get a deployment-ID prefix (e.g., `my-app-1708123456-web-0`) while both old and new containers run simultaneously.
:::

## Examples

### Minimal

```yaml
name: hello

services:
  web:
    image: nginx:alpine
```

One container on one worker.

### Full example

A production-style manifest with a reverse proxy, scaled API, and database:

```yaml
name: my-app

services:
  caddy:
    image: caddy:latest
    restart: unless-stopped
    command: caddy reverse-proxy --from example.com --to api:8080
    deploy:
      placement:
        node: gateway-*
    ports:
      - "80:80"
      - "443:443"

  api:
    build: ./api
    restart: unless-stopped
    deploy:
      replicas: 3
      resources:
        limits:
          memory: 512m
          cpus: "1"
        reservations:
          memory: 256m
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=db
      - DB_PORT=5432
    depends_on:
      - db

  db:
    image: postgres:15-alpine
    restart: unless-stopped
    entrypoint:
      - docker-entrypoint.sh
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=banyan
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=app
```

This shows `deploy.placement.node` to pin the reverse proxy to gateway servers, `deploy.replicas` to scale the API across workers, `build:` for custom services, `image:` for off-the-shelf databases, and service DNS (`api:8080`, `db`) for cross-service communication.

### Build from source

Use `build:` to build images from a Dockerfile instead of pulling a pre-built image. Built images are pushed to the Engine's embedded OCI registry so all agents can pull them.

**String form** — just the build context path:

```yaml
services:
  web:
    build: ./web
    ports:
      - "80:80"
```

**Object form** — specify a custom Dockerfile:

```yaml
services:
  api:
    build:
      context: ./api
      dockerfile: Dockerfile.prod
```

If `image` is not set, Banyan generates a name: `<app-name>-<service-name>:latest`. You can set `image` explicitly to control the tag:

```yaml
services:
  api:
    image: my-api:v2
    build: ./api
```

Each service must have either `image` or `build` (or both).

The [full example](#full-example-examplesbanyanyml) above demonstrates mixing `build:` and `image:` services. Services with `build:` are built locally and pushed to the Engine's registry. Services with only `image:` are pulled directly by agents.

## Validation

Check your manifest without deploying:

```bash
banyan-cli up -f banyan.yaml --dry-run
```

This parses the file, checks for errors, and prints the services that would be deployed.
