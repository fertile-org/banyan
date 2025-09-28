# Banyan Monorepo

Banyan is an infrastructure layer for docker-compose, which will allow us to deploy the docker-compose yaml file into various cloud and on-premises servers, include production-ready CICD and system monitoring.

### 1. Components Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CLI Interface                            │
│  ├─ compose.yaml parser                                     │
│  ├─ command router                                          │
│  └─ configuration manager                                   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                   Core Engine                               │
│  ├─ resource state manager                                  │
│  ├─ deployment orchestrator                                 │
│  ├─ provider abstraction layer                              │
│  └─ plugin system manager                                   │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                Plugin Ecosystem                             │
│  ├─ cloud providers (AWS, GCP, Azure, DigitalOcean)         │
│  ├─ server providers (bare metal, VPS)                      │
│  ├─ deployment strategies (docker, systemd, containers)     │
│  └─ monitoring & observability                              │
└─────────────────────────────────────────────────────────────┘
                              │
┌─────────────────────────────────────────────────────────────┐
│                Agent System                                 │
│  ├─ lightweight deployment agent                            │
│  ├─ health monitoring                                       │
│  ├─ log collection                                          │
│  └─ secure communication layer                              │
└─────────────────────────────────────────────────────────────┘

```

### 2. Usages - Configuration (TBD)


Original `docker-compose.yaml` file
```yaml
version: '3.8'

services:
  web:
    image: "myapp:latest"
    ports:
      - "80:8080"
    environment:
      - DATABASE_URL=${DATABASE_URL}

  database:
    image: "postgres:13"

volumes:
  db_data:

networks:
  default:
```


Banyan config file:
```yaml
version: '3.8'
docker_compose:
    file: ./docker-compose.yaml

# Simple network configuration (optional - uses smart defaults)
networks:
  default:
    dns_suffix: "internal"  # Services accessible as service.internal

services:
  web:
    instances: 2  # Simple scaling
    health:
        path: "/health"
        interval: 30s
    network:
        # DNS: web.internal (auto-generated)
        allow:
            - from: internet
              to_port: 443  # Public HTTPS access

  database:
    health:
        path: "/health"
        interval: 30s
    plugins:
        - plugin_name: database_backup
          parameters:
            schedule: "0 2 * * *"
            retention: 7d
            location: s3://...
    network:
        # DNS: database.internal (auto-generated)
        allow:
            - from: service:web
              to_port: 5432  # Only web can connect to PostgreSQL port
    # Persistent volume (auto-detected for database images)
    volume:
        size: 100GB
```

### 3. Usages - Deployment & Monitoring (TBD)

```
banyan validate -f banyan.yaml
banyan deploy --dry-run
banyan deploy
banyan monitor
```

## For Development

Read [DEVELOPMENT.md](./DEVELOPMENT.md) to get the instruction for development of this project.

## License

TBD