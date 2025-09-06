# Banyan Monorepo

Banyan is a solution that will allow us to deploy the docker-compose yaml file into various cloud and on-premises servers, include production-ready CICD and system monitoring.

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

services:
  web:
    auto_scaling:
        min: 1
        max: 5
        cpu_threshold: 70
    health:
        path: "/health"
        interval: 30s
    plugins:
        - alb:
            plugin_name: application_load_balancer
            parameters:
                listener_port: 443
                dest_port: 8080
                ssl:
                    auto: true
    network:
        vpc: default_vpc
        inbound:
            - rule: allow_access_from_internet_to_alb
              source_protocal: tcp
              source: 
                cidr: 0.0.0.0/32
              dest_from_port: 443
              dest_to_port: 443


  database:
    health:
        path: "/health"
        interval: 30s
    network:
        vpc: default_vpc
        inbound:
            - rule: allow_access_from_web_server
              source_protocal: tcp
              source: 
                sg: !service web.sg
              dest_from_port: 5433
              dest_to_port: 5433
    plugins:
        - backup:
            plugin_name: database_backup
            parameters:
                schedule: "0 2 * * *"
                retention: 7d
                location: s3://...


networks:
  default_vpc:
    cidr: "10.0.0.0/16"


domain:
    ...
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