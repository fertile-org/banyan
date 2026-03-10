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
| Dependencies | `depends_on:` | `depends_on:` | Same — short form and long form with `condition: service_healthy` |
| Replicas | `deploy.replicas:` | `deploy.replicas:` | Same |
| Placement | `deploy.placement.constraints:` | `deploy.placement.node:` | Glob pattern for agent name matching |
| App name | Inferred from directory | `name:` | Explicit in Banyan |
| Build | `build:` | `build:` | Same syntax (context + dockerfile) |
| Env files | `env_file:` | `env_file:` | Same (string or list of paths) |
| Restart | `restart:` | `restart:` | Same |
| Entrypoint | `entrypoint:` | `entrypoint:` | Same |
| Resource limits | `deploy.resources:` | `deploy.resources:` | Same (memory, cpus). Also used for scheduling decisions. |
| Healthcheck | `healthcheck:` | `healthcheck:` | Same (test, interval, timeout, retries, start_period, disable) |
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
        node: <pattern>     # Glob pattern for agent name
      resources:
        limits:
          memory: 512m
          cpus: "0.5"
        reservations:
          memory: 256m
    healthcheck:
      test: ["CMD", "<command>"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    ports:
      - "<host>:<container>"
    environment:
      - KEY=value
    env_file: .env            # Or a list of files
    command:
      - <arg1>
      - <arg2>
    depends_on:
      <other-service>:
        condition: service_healthy  # or service_started (default)
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
| `deploy.resources.limits.memory` | string | No | -- | Memory limit (e.g., `512m`, `1g`). Container is killed if it exceeds this. Also used for scheduling if no reservation is set. |
| `deploy.resources.limits.cpus` | string | No | -- | CPU limit (e.g., `"0.5"`, `"2"`). Fractional cores allowed. |
| `deploy.resources.reservations.memory` | string | No | -- | Memory reservation (e.g., `256m`). Used by the scheduler to decide which worker runs this service. Takes priority over `limits.memory` for scheduling. |
| `restart` | string | No | `no` | Restart policy: `no`, `always`, `unless-stopped`, `on-failure`, or `on-failure:N`. |
| `entrypoint` | string or list | No | -- | Override the container's ENTRYPOINT. Supports string or list form. |
| `ports` | list | No | -- | Port mappings in `host:container` format. |
| `environment` | list | No | -- | Environment variables in `KEY=value` format. |
| `env_file` | string or list | No | -- | Load environment variables from file(s). Supports `.env` format. See [env_file](#env_file) below. |
| `command` | list | No | -- | Override the container's default command. Each argument is a list item. |
| `healthcheck.test` | string or list | No | -- | Health check command. List form: `["CMD", "pg_isready"]` or `["CMD-SHELL", "curl -f http://localhost"]`. String form: `curl -f http://localhost` (treated as CMD-SHELL). `["NONE"]` disables. |
| `healthcheck.interval` | string | No | -- | Time between checks (e.g., `10s`, `1m`). |
| `healthcheck.timeout` | string | No | -- | Timeout per check (e.g., `5s`). |
| `healthcheck.retries` | integer | No | -- | Consecutive failures before marking unhealthy. |
| `healthcheck.start_period` | string | No | -- | Grace period for startup (e.g., `30s`). Failures during this period don't count toward retries. |
| `healthcheck.disable` | boolean | No | `false` | Set `true` to disable any healthcheck defined in the image. |
| `depends_on` | list or map | No | -- | Service dependencies. Short form: `["db", "redis"]`. Long form with conditions: `{db: {condition: service_healthy}}`. Conditions: `service_started` (default), `service_healthy`. See [depends_on](#depends_on) below. |

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
    command: caddy reverse-proxy --from example.com --to api.my-app.internal:8080
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
      - DB_HOST=db.my-app.internal
      - DB_PORT=5432
    depends_on:
      - db

  db:
    image: postgres:15-alpine
    restart: unless-stopped
    entrypoint:
      - docker-entrypoint.sh
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "banyan"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=banyan
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=app
```

This shows `deploy.placement.node` to pin the reverse proxy to gateway servers, `deploy.replicas` to scale the API across workers, `build:` for custom services, `image:` for off-the-shelf databases, `healthcheck:` for container health monitoring, and service DNS (`api.my-app.internal:8080`, `db.my-app.internal`) for cross-service communication. During blue-green redeployments, Banyan waits for healthchecks to pass before tearing down old containers.

### Service DNS

Banyan provides built-in DNS for service discovery. Every service gets a DNS name that other containers can use to connect to it.

**Two forms are available:**

| Form | Example | When to use |
|------|---------|-------------|
| **Full name** (recommended) | `db.my-app.internal` | Always works. Use this in production. |
| **Short name** | `db` | Convenience shorthand. Works only when no other deployment has a service with the same name. |

The full DNS name follows the pattern `<service>.<app-name>.internal`, where `<app-name>` is the `name:` field in your manifest.

:::tip[Use the full name]
Always use `<service>.<app-name>.internal` in environment variables and configuration. The short form (`db`) is convenient for quick testing, but can break if you run multiple deployments with services that share a name (e.g., two apps both have a `db` service).
:::

```yaml
# Recommended — always works
environment:
  - DB_HOST=db.my-app.internal

# Also works, but fragile with multiple deployments
environment:
  - DB_HOST=db
```

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

### env_file

Load environment variables from one or more files, using the same syntax as Docker Compose.

**String form** — single file:

```yaml
services:
  api:
    image: my-api:latest
    env_file: .env
```

**List form** — multiple files (later files override earlier ones):

```yaml
services:
  api:
    image: my-api:latest
    env_file:
      - .env
      - .env.production
```

Files use standard `.env` format: `KEY=VALUE` pairs, one per line. Comments (`#`), blank lines, quoted values, and `export` prefixes are all supported.

**Merge order:** Variables from `env_file` are loaded first, then inline `environment` values override them. This lets you keep defaults in a file and override specific values in the manifest:

```yaml
services:
  api:
    image: my-api:latest
    env_file: .env              # Loads DB_HOST=localhost, DB_PORT=5432
    environment:
      - DB_HOST=production-db   # Overrides DB_HOST from .env
```

Paths are relative to the manifest file's directory.

### depends_on

Control startup order and declare service dependencies, using the same syntax as Docker Compose.

**Short form** — services start first, no health requirement:

```yaml
services:
  api:
    image: my-api:latest
    depends_on:
      - db
      - redis
```

**Long form** — with health conditions:

```yaml
services:
  api:
    image: my-api:latest
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_started
```

| Condition | Meaning |
|-----------|---------|
| `service_started` | Dependency must be running (default) |
| `service_healthy` | Dependency must be running **and** its [healthcheck](#service) must report healthy |

The short form (`["db"]`) is equivalent to `{db: {condition: service_started}}`.

During [per-service deploys](/guides/redeployment/#dependency-validation), Banyan validates that all dependencies are either already running or included in the same deploy command.

### Resource-aware scheduling

Banyan uses `deploy.resources` to decide where to place containers. Each task goes to the worker with the most available memory.

**How the scheduler picks a worker:**

1. Workers report CPU, memory, and disk usage to the engine every heartbeat.
2. For each container, the scheduler picks the worker with the most available memory (total − used − already-scheduled-in-this-deployment).
3. If no worker has reported metrics yet (e.g., during the first few seconds after startup), scheduling falls back to round-robin.

**What counts as the resource request:**

| Manifest field | Scheduling behavior |
|---|---|
| `reservations.memory` set | Scheduler uses the reservation value |
| Only `limits.memory` set | Scheduler uses the limit value |
| Neither set | Scheduler assumes **512 MB** per service |

CPU values are tracked but memory is the primary scheduling dimension.

**Cluster capacity validation:** Before scheduling, the engine checks that the total resource requests for the deployment don't exceed the total cluster memory. If they do, the deployment fails immediately with a clear error message instead of partially scheduling.

:::tip
For most workloads, you don't need to set `deploy.resources` at all. The defaults (512 MB, 1 CPU per service) work well for typical web services. Add explicit resources when you have services with significantly different needs — a memory-heavy database alongside lightweight API workers, for example.
:::

## Validation

Check your manifest without deploying:

```bash
banyan-cli up -f banyan.yaml --dry-run
```

This parses the file, checks for errors, and prints the services that would be deployed.
