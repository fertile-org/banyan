# VPC User Guide & Configuration

## Overview

Banyan automatically handles networking complexity while keeping configuration simple. This document explains how Banyan manages VPCs, subnets, DNS, and service communication across multiple hosts.

## Core Principles

1. **Deny by Default** - No service can communicate with another unless explicitly allowed
2. **Zero Configuration Network** - VPC and subnets are auto-created, but you MUST define access rules
3. **Progressive Enhancement** - Add complexity only when needed
4. **Docker Compose Compatibility** - Same service names work locally and in production
5. **Explicit Security** - Every service connection must be explicitly allowed via `allow` rules

## How It Works

### Local Development (docker-compose)
```yaml
# Standard docker-compose creates a bridge network
# All services can communicate by default
web → database  # Works automatically (docker default)
```

### Production (Banyan)
```yaml
# Banyan creates isolated VPC with deny-by-default
# Services CANNOT communicate unless explicitly allowed
web.internal → database.internal  # BLOCKED unless database allows web

# Must explicitly allow:
database:
  network:
    allow:
      - from: service:web
        to_port: 5432
```

## Network Architecture

### Default Network Setup

When you deploy without any network configuration, Banyan automatically creates:

- **VPC**: `10.0.0.0/16` (65,536 IP addresses)
  - **Public subnet**: `10.0.1.0/24` for services with `ports` directive
  - **Private subnet**: `10.0.2.0/24` for internal services
- **DNS suffix**: `.internal` (web.internal, database.internal)
- **Overlay network**: VXLAN for cross-host communication

### Basic Configuration

```yaml
# docker-compose.yaml
services:
  web:
    image: "myapp:latest"
    ports:
      - "80:8080"  # Local dev uses port 80

  database:
    image: "postgres:13"

# banyan.yaml - Minimal configuration with security rules
services:
  web:
    ports:
      - "443:8080"  # Override: Production uses port 443 (HTTPS)
    network:
      allow:
        - from: internet
          to_port: 443      # Public HTTPS access

  database:
    network:
      allow:
        - from: service:web
          to_port: 5432     # Only web can access database
```

**Note**: The `ports` directive in banyan.yaml is optional and only needed to override docker-compose values. If not specified, Banyan uses the ports from docker-compose.yaml.

### Custom Networks

Only customize when you need specific DNS or CIDR ranges:

```yaml
networks:
  default:
    dns_suffix: "prod"      # Changes DNS to: web.prod, api.prod
    cidr: "172.16.0.0/16"   # Custom IP range to avoid conflicts
```

## Security Rules Reference

### Source Types

| Source | Format | Example | Use Case |
|--------|--------|---------|----------|
| Service | `service:<name>` | `from: service:web` | Same VPC service |
| Internet | `internet` | `from: internet` | Public access |
| CIDR | `cidr:<range>` | `from: cidr:10.0.0.0/8` | IP range access |
| Cross-VPC | `service:<name>.<dns>` | `from: service:auth.platform` | Different VPC (future) |

### Rule Examples

```yaml
network:
  allow:
    # Specific port access
    - from: service:web
      to_port: 8080

    # All ports (omit to_port)
    - from: service:admin

    # Multiple sources
    - from: internet
      to_port: 443
    - from: cidr:192.168.1.0/24
      to_port: 22
```

## DNS & Service Discovery

Every service automatically gets a DNS name based on the service name + network suffix:

- `web.internal` (default suffix)
- `web.prod` (with `dns_suffix: "prod"`)

Services use DNS names in application code:
```python
DATABASE_URL = "postgresql://database.internal:5432/myapp"
REDIS_URL = "redis://cache.internal:6379"
```

DNS resolution is health-aware - only healthy instances are returned, with automatic load balancing across multiple instances.

## Subnet Management

### Automatic Subnet Placement

Banyan automatically determines subnet placement based on service configuration:

**External Ports Explained**: External ports are ports exposed to the host system using the `ports` directive. For example, `ports: ["80:8080"]` exposes container port 8080 as port 80 on the host, making it accessible from outside the container network. Services without the `ports` directive only have internal container ports and are not directly accessible from outside. Banyan uses this directive (from docker-compose syntax) to determine subnet placement and requires matching `allow` rules for internet access.

| Service Type | Detection | Placement |
|-------------|-----------|-----------|
| Public Services | Has `ports` directive (external ports exposed to host) | Public subnet with internet gateway |
| Private Services | No `ports` directive (internal only) | Private subnet with NAT only |
| Databases | PostgreSQL/MySQL/MongoDB images | Private subnet + persistent storage |
| Load Balancers | Multiple instances + external ports | Public subnet + ALB/NLB |

### Team Isolation and Multiple VPCs

**Most applications only need one VPC!** The `allow` rules provide sufficient isolation for services within a single VPC.

In larger organizations, different teams may run their own VPCs:
- **Platform Team**: Shared services (auth, monitoring, logging)
- **Product Teams**: Individual application VPCs
- **Data Team**: Analytics and data warehouse

For cross-VPC communication, use API endpoints with proper authentication:

```yaml
services:
  api:
    environment:
      AUTH_SERVICE: "https://auth.platform.company.com"
      AUTH_API_KEY: "${secrets.platform_auth_key}"
```

### Manual Subnet Control

When needed, explicitly control subnet placement:

```yaml
networks:
  production:
    subnets:
      dmz:
        cidr: "10.0.1.0/24"
        public: true
      app:
        cidr: "10.0.2.0/24"
        public: false
      data:
        cidr: "10.0.3.0/24"
        public: false

services:
  web:
    network:
      subnet: dmz  # Explicit subnet placement
```


## Multi-Host Networking

Banyan uses overlay networks (VXLAN) for seamless container-to-container communication across hosts:

```yaml
networks:
  production:
    driver: overlay  # Default for multi-host
    encryption: true  # Optional encryption
```

Services communicate transparently across hosts using DNS names (e.g., `web` on host-1 can reach `api.internal` on host-2).

### Network Driver Options

| Driver | Use Case | Performance |
|--------|----------|-------------|
| overlay (default) | Multi-host, simple setup | Good |
| calico | Large scale, network policies | Excellent |
| flannel | Simple overlay | Good |
| weave | Mesh network, encryption | Fair |

## Provider-Specific Implementation

| Provider | Components |
|----------|------------|
| **AWS** | VPC with public/private subnets, Security Groups, Route53 DNS, NAT Gateways |
| **Google Cloud** | VPC with regional subnets, Firewall Rules, Cloud DNS, Cloud NAT |
| **On-Premises/VPS** | VXLAN overlay, iptables/nftables, CoreDNS, WireGuard/IPSec |

## Migration from Docker Compose

Docker Compose allows all services to communicate by default. Banyan requires explicit security rules:

**Docker Compose**:
```yaml
# All services can communicate freely
services:
  web:
    networks: [frontend]
  api:
    networks: [frontend, backend]
  db:
    networks: [backend]
```

**Banyan**:
```yaml
# Explicit allow rules required
services:
  web:
    network:
      allow:
        - from: internet
          to_port: 443

  api:
    network:
      allow:
        - from: service:web
          to_port: 8080

  db:
    network:
      allow:
        - from: service:api
          to_port: 5432
```

## Best Practices

1. **Start Simple**: Use defaults, let Banyan handle subnet placement
2. **Security First**: Always define explicit `allow` rules
3. **Use DNS**: Reference services by DNS names, not IP addresses
4. **Scale Gradually**: Start with overlay network, add complexity only when needed

## Troubleshooting

### Common Issues and Solutions

| Issue | Check | Command |
|-------|-------|---------|
| Service can't connect | Verify `allow` rules exist | `banyan network rules <service>` |
| DNS not resolving | Check service health | `banyan exec web -- nslookup database.internal` |
| Cross-host failed | Test connectivity | `banyan network test web database` |

### Debug Commands

```bash
# Network inspection
banyan network ls
banyan network inspect production

# Connectivity testing
banyan network test <from-service> <to-service>
banyan network dns-lookup <service-name>

# Security rules
banyan network rules <service>
banyan network flow-logs <service>
```

## Summary

Banyan's networking is designed to be:

- **Simple by default** - Zero configuration required
- **Secure by default** - Isolated unless explicitly allowed
- **Scalable** - From single host to hundreds of nodes
- **Compatible** - Works with existing docker-compose files
- **Flexible** - Progressive enhancement when needed

The key insight: **You don't need to understand VPCs, subnets, or security groups to deploy production applications. Banyan handles the complexity while you focus on your application.**