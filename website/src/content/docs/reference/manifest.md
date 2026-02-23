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
| Dependencies | `depends_on:` | `depends_on:` | Same (informational) |
| Replicas | `deploy.replicas:` | `deploy.replicas:` | Same |
| App name | Inferred from directory | `name:` | Explicit in Banyan |
| Build | `build:` | `build:` | Same syntax (context + dockerfile) |
| Volumes | `volumes:` | -- | Not yet supported |
| Networks | `networks:` | -- | Managed automatically |

## Structure

```yaml
name: <application-name>    # Required

services:
  <service-name>:           # One or more services
    image: <image>          # Required (unless build is set)
    build: <context-path>   # Build from Dockerfile
    deploy:
      replicas: <number>    # Default: 1
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
| `name` | string | Yes | Application name. Used as a prefix for container names. |
| `services` | map | Yes | Map of service definitions. At least one service required. |

### Service

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | Conditional | -- | Container image. Required unless `build` is set. Any registry works: `nginx:alpine`, `ghcr.io/org/app:v1`. |
| `build` | string or object | No | -- | Build from a Dockerfile. See [Build](#build) below. |
| `deploy.replicas` | integer | No | `1` | Number of container instances. Distributed across available workers. |
| `ports` | list | No | -- | Port mappings in `host:container` format. |
| `environment` | list | No | -- | Environment variables in `KEY=value` format. |
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

### Full example (examples/banyan.yml)

This is the example manifest included in the repository:

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

This shows `build:` for custom services, `image:` for off-the-shelf databases, `deploy.replicas` for scaling, `environment` for configuration, and `depends_on` for ordering.

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
banyan-cli deploy -f banyan.yaml --dry-run
```

This parses the file, checks for errors, and prints the services that would be deployed.
