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
| `deploy.replicas` | integer | No | `1` | Number of container instances. Distributed across available agents. |
| `deploy.placement.node` | string | No | -- | Glob pattern to pin this service to specific nodes. Supports `*`, `?`, and `[abc]`. Example: `gateway-*` matches `gateway-1`, `gateway-2`. |
| `deploy.resources.limits.memory` | string | No | -- | Memory limit (e.g., `512m`, `1g`). Container is killed if it exceeds this. Also used for scheduling if no reservation is set. |
| `deploy.resources.limits.cpus` | string | No | -- | CPU limit (e.g., `"0.5"`, `"2"`). Fractional cores allowed. |
| `deploy.resources.reservations.memory` | string | No | -- | Memory reservation (e.g., `256m`). Used by the scheduler to decide which agent runs this service. Takes priority over `limits.memory` for scheduling. |
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
| `volumes` | list | No | -- | Mount volumes into the container. Same syntax as Docker Compose. See [volumes](#volumes) below. |

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

One container on one agent.

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

This shows `deploy.placement.node` to pin the reverse proxy to gateway servers, `deploy.replicas` to scale the API across agents, `build:` for custom services, `image:` for off-the-shelf databases, `healthcheck:` for container health monitoring, and service DNS (`api.my-app.internal:8080`, `db.my-app.internal`) for cross-service communication. During blue-green redeployments, Banyan waits for healthchecks to pass before tearing down old containers.

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

Banyan uses `deploy.resources` to decide where to place containers. Each task goes to the agent with the most available memory.

**How the scheduler picks an agent:**

1. Agents report CPU, memory, and disk usage to the engine every heartbeat.
2. For each container, the scheduler picks the agent with the most available memory (total − used − already-scheduled-in-this-deployment).
3. If no agent has reported metrics yet (e.g., during the first few seconds after startup), scheduling falls back to round-robin.

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

## Volumes

Mount host directories, named volumes, or temporary filesystems into containers. Same syntax as Docker Compose.

### Short syntax

```yaml
services:
  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data           # named volume
      - /backups:/backups:ro                        # host path, read-only
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql  # relative path
```

| Format | Type | Example |
|--------|------|---------|
| `name:/container/path` | Named volume | `db-data:/var/lib/postgresql/data` |
| `/host/path:/container/path` | Host path (absolute) | `/backups:/backups` |
| `./path:/container/path` | Host path (relative) | `./config:/etc/app/config` |
| `...:ro` | Read-only mount | `db-data:/data:ro` |

### Long syntax

For more control, use the mapping form:

```yaml
services:
  api:
    image: myapp/api
    volumes:
      - type: bind
        source: /host/logs
        target: /var/log/app
        read_only: true
      - type: tmpfs
        target: /tmp
        tmpfs:
          size: 512m
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | `volume`, `bind`, or `tmpfs` |
| `source` | string | No | Volume name (for `volume`) or host path (for `bind`). Not used for `tmpfs`. |
| `target` | string | Yes | Absolute path inside the container. |
| `read_only` | boolean | No | Mount as read-only. Default: `false`. |
| `tmpfs.size` | string | No | Size limit for tmpfs (e.g., `512m`, `1g`). |

### Top-level volumes

Declare named volumes at the top level to configure their driver:

```yaml
volumes:
  db-data:                     # local storage (default)
  shared-uploads:
    driver: local
    driver_opts:
      type: nfs
      o: "addr=nfs.internal,vers=4,soft"
      device: ":/exports/uploads"
```

| Field | Type | Description |
|-------|------|-------------|
| `driver` | string | Volume driver. Default: `local`. |
| `driver_opts` | map | Driver-specific options. For NFS: `type`, `o` (mount options), `device` (server path). |

Named volumes without a top-level declaration default to local storage on the agent.

### Databases and persistent data

Named volumes are **local to each agent**. If Banyan schedules a database on a different agent after a redeployment, the data won't follow.

For databases and other stateful services, pin them to a specific agent:

```yaml
services:
  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data
    deploy:
      placement:
        node: db-*                  # always runs on agents matching "db-*"

volumes:
  db-data:
```

This ensures the database always runs on the same agent, where its named volume lives.

:::caution
Without `deploy.placement.node`, Banyan may schedule a stateful service on a different agent after redeployment. The named volume from the previous agent won't be available. Always pin stateful services to a specific agent.
:::

### NFS shared volumes

For data that multiple services or agents need to access, use NFS:

```yaml
services:
  api:
    image: myapp/api
    deploy:
      replicas: 3
    volumes:
      - uploads:/app/uploads

volumes:
  uploads:
    driver: local
    driver_opts:
      type: nfs
      o: "addr=nfs.internal,vers=4,soft"
      device: ":/exports/uploads"
```

All 3 API replicas — regardless of which agent they run on — mount the same NFS share. You provide the NFS server; Banyan handles the mounting on each agent.

:::note
NFS requires `nfs-common` (Debian/Ubuntu) or `nfs-utils` (RHEL/Fedora) on each agent. The install script installs this automatically.
:::

### Relative paths

In Docker Compose, `./config` is relative to the compose file. In Banyan, the manifest runs on a different machine than the containers.

Relative bind mount paths resolve to `/var/lib/banyan/data/` on the agent machine. To mount `./config.yml`:

1. Place the file at `/var/lib/banyan/data/config.yml` on each agent
2. Use `./config.yml:/etc/app/config.yml` in the manifest

For files that only exist on one agent, combine with placement:

```yaml
services:
  app:
    image: myapp
    volumes:
      - ./config.yml:/etc/app/config.yml:ro
    deploy:
      placement:
        node: app-server
```

## Validation

Check your manifest without deploying:

```bash
banyan-cli up -f banyan.yaml --dry-run
```

This parses the file, checks for errors, and prints the services that would be deployed.
