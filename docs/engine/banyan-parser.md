# Banyan Parser

## Overview

The Banyan Parser is responsible for parsing `banyan.yml` configuration files. It provides a simple, familiar YAML format for defining distributed services - if you know docker-compose, you already know 90% of banyan.yml.

## Philosophy

**"Docker Compose that scales"** - not "Kubernetes simplified"

Our target users are startups and small teams who:
- Know docker-compose from local development
- Don't have dedicated DevOps teams
- Don't want to learn Kubernetes
- Just want their containers to run on multiple servers

## banyan.yml Specification

### Minimal Example

The simplest banyan.yml that works:

```yaml
services:
  web:
    image: myapp:latest
```

That's it. This deploys your container.

### Adding Scaling

The main thing banyan.yml adds over docker-compose is `replicas`:

```yaml
services:
  web:
    image: myapp:latest
    replicas: 3
```

This runs 3 instances of your web service across your cluster.

### Complete Example

A typical startup production setup:

```yaml
services:
  web:
    image: ghcr.io/mycompany/web:latest
    ports:
      - "3000:3000"
    replicas: 2
    environment:
      - NODE_ENV=production
      - DATABASE_URL=${DATABASE_URL}
    healthcheck:
      test: curl -f http://localhost:3000/health

  api:
    image: ghcr.io/mycompany/api:latest
    replicas: 3
    environment:
      - DATABASE_URL=${DATABASE_URL}

  worker:
    image: ghcr.io/mycompany/worker:latest
    replicas: 2
    environment:
      - REDIS_URL=${REDIS_URL}

  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data
    environment:
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

  redis:
    image: redis:7-alpine

volumes:
  db-data:
```

## Supported Fields

### Service Configuration

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | Container image (required) |
| `replicas` | int | Number of instances (default: 1) |
| `ports` | list | Port mappings ("host:container") |
| `environment` | list/map | Environment variables |
| `volumes` | list | Volume mounts |
| `healthcheck` | object | Health check configuration |
| `command` | string/list | Override container command |
| `entrypoint` | string/list | Override entrypoint |
| `depends_on` | list | Service dependencies |
| `restart` | string | Restart policy |

### Health Check

```yaml
healthcheck:
  test: curl -f http://localhost:3000/health
  interval: 30s
  timeout: 10s
  retries: 3
```

### Environment Variables

Two formats supported (same as docker-compose):

```yaml
# List format
environment:
  - NODE_ENV=production
  - DEBUG=false

# Map format
environment:
  NODE_ENV: production
  DEBUG: "false"
```

### Volumes

```yaml
services:
  db:
    volumes:
      - db-data:/var/lib/postgresql/data
      - ./config:/etc/config:ro

volumes:
  db-data:
```

## What We Don't Support (By Design)

We intentionally keep things simple. These docker-compose features are NOT supported:

- **build** - Use pre-built images, not build-on-deploy
- **networks** - Banyan handles networking automatically
- **configs/secrets** - Use environment variables
- **deploy.resources** - We use sensible defaults
- **deploy.placement** - Banyan distributes automatically
- **profiles** - One file, one environment

If you need these features, you probably need Kubernetes.

## Parser Implementation

### Architecture

```
banyan.yml
    │
    ▼
┌─────────────┐
│   Parser    │  ← Reads YAML, validates structure
└─────────────┘
    │
    ▼
┌─────────────┐
│  Validator  │  ← Checks required fields, valid values
└─────────────┘
    │
    ▼
┌─────────────┐
│  Converter  │  ← Transforms to internal ServiceSpec
└─────────────┘
    │
    ▼
ServiceSpec (internal domain model)
```

### Domain Model

```go
// BanyanConfig represents a parsed banyan.yml file
type BanyanConfig struct {
    Services map[string]ServiceConfig
    Volumes  map[string]VolumeConfig
}

// ServiceConfig represents a service definition
type ServiceConfig struct {
    Image       string
    Replicas    int
    Ports       []string
    Environment []string
    Volumes     []string
    Healthcheck *HealthcheckConfig
    Command     []string
    Entrypoint  []string
    DependsOn   []string
    Restart     string
}

// HealthcheckConfig represents health check settings
type HealthcheckConfig struct {
    Test     string
    Interval string
    Timeout  string
    Retries  int
}

// VolumeConfig represents a volume definition
type VolumeConfig struct {
    Driver string
}
```

### Validation Rules

1. **Required fields**: `image` must be specified for each service
2. **Replicas**: Must be >= 1 if specified
3. **Ports**: Must be valid "host:container" or just "container" format
4. **Health check**: If specified, `test` is required

### Error Handling

Clear, actionable error messages:

```
Error: banyan.yml validation failed
  → services.web: 'image' is required
  → services.api.replicas: must be at least 1, got 0
  → services.db.ports[0]: invalid format "abc", expected "host:container"
```

## CLI Usage

```bash
# Validate a banyan.yml file
banyan validate

# Deploy services
banyan up

# Scale a service
banyan scale web 5

# Check status
banyan ps
```

## Migration from docker-compose

If you have an existing docker-compose.yml:

1. Copy it to banyan.yml
2. Remove unsupported fields (build, networks, etc.)
3. Add `replicas` to services you want to scale
4. Run `banyan validate` to check for issues

Most simple docker-compose files work as banyan.yml with zero changes.

## Future Considerations

We may add these features based on user feedback:

- **Resource limits** - Simple format like `memory: 512` (MB)
- **Labels** - For service discovery and organization
- **Placement hints** - Simple "prefer SSD" style hints

But only if users actually need them. We're not adding complexity preemptively.
