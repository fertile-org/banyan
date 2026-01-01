# Banyan

**Docker Compose that scales.** Deploy your containers across multiple servers with a single YAML file.

## Why Banyan?

You know docker-compose. You use it for local development. But when it's time to deploy to production across multiple servers... suddenly you need Kubernetes, Helm charts, and a DevOps team.

**Banyan bridges that gap.** Write a `banyan.yml` that looks almost identical to docker-compose, add `replicas: 3`, and deploy across your servers.

## Quick Example

```yaml
# banyan.yml - that's it, one file
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    replicas: 3  # ← runs on 3 servers
    environment:
      - DATABASE_URL=${DATABASE_URL}

  api:
    image: myapi:latest
    replicas: 2

  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:
```

Deploy:
```bash
banyan up
```

That's it. No YAML templating. No resource quotas. No node selectors. Just containers on servers.

## Who Is This For?

- **Startups** deploying their first production setup
- **Small teams** without dedicated DevOps
- **Developers** who know docker-compose and don't want to learn Kubernetes
- **Anyone** who thinks "I just want to run 3 instances of my app"

## What Banyan Does

1. **Parses your banyan.yml** (familiar docker-compose syntax)
2. **Distributes containers** across your servers
3. **Handles networking** so services can talk to each other
4. **Manages health** and restarts failed containers
5. **Scales up/down** when you change replicas

## What Banyan Doesn't Do

- Complex resource scheduling (use Kubernetes)
- Multi-region deployments (use Kubernetes)
- Automatic scaling based on metrics (use Kubernetes)
- Service mesh, sidecars, operators (use Kubernetes)

**If you need those features, you need Kubernetes. That's okay.**

Banyan is for teams who don't need Kubernetes complexity but do need to run containers on more than one server.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Banyan Engine                             │
│  ├─ banyan.yml parser                                       │
│  ├─ deployment orchestrator                                  │
│  ├─ agent registry                                          │
│  └─ plugin system                                           │
└─────────────────────────────────────────────────────────────┘
                              │
                        gRPC/REST
                              │
┌─────────────────────────────────────────────────────────────┐
│                    Banyan Agent (per server)                │
│  ├─ container runtime (containerd)                          │
│  ├─ health monitoring                                       │
│  ├─ networking (VPC overlay)                                │
│  └─ log collection                                          │
└─────────────────────────────────────────────────────────────┘
```

## Getting Started

```bash
# Install banyan CLI
curl -sSL https://get.banyan.dev | sh

# Install agent on each server
banyan agent install

# Deploy your services
banyan up
```

## Documentation

- [Development Guide](./DEVELOPMENT.md)
- [Engine Design](./docs/engine/README.md)
- [banyan.yml Specification](./docs/engine/banyan-parser.md)

## License

TBD
