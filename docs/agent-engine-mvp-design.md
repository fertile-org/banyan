# Agent & Engine MVP Design

**Date**: 2026-01-01
**Status**: Active

---

## 1. Project Philosophy

**"Docker Compose that scales"** - Banyan is for startups and small teams who:
- Know docker-compose from local development
- Don't have dedicated DevOps teams
- Don't want to learn Kubernetes
- Just want their containers to run on multiple servers

---

## 2. MVP Scope

### MVP-1: Core Functionality

Deploy containers with replicas using a simple banyan.yml:

```yaml
services:
  api:
    image: myapp:latest
    replicas: 3
    healthcheck:
      test: curl -f http://localhost:3000/health
  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data
volumes:
  db-data:
```

**Features:**
- Parse banyan.yml (image, replicas, ports, environment, volumes, depends_on, healthcheck)
- DNS-based service discovery (services reach each other by name)
- Agent registration and selection
- Basic deployment flow
- Health checks

**NOT included:** Load balancer plugin, auto-scaling, SSL, backup

### MVP-2: Essential Plugins

```yaml
services:
  api:
    image: myapp:latest
    replicas: 3
    plugins:
      - name: load_balancer
        config:
          port: 443
          ssl:
            auto: true  # Let's Encrypt
  db:
    plugins:
      - name: database_backup
        config:
          schedule: "0 2 * * *"
          destination: s3://bucket/backups
```

**Features:**
- Plugin system for per-service plugins
- `load_balancer` plugin with SSL termination
- `database_backup` plugin

### MVP-3: Auto-Scaling & Advanced Features

```yaml
services:
  api:
    replicas:
      min: 2
      max: 10
    plugins:
      - name: auto_scaler
        config:
          metric: cpu
          target: 70
      - name: network_policy
        config:
          allow:
            - db
          deny_all_others: true
```

**Features:**
- `auto_scaler` plugin
- `network_policy` plugin (explicit allow/deny)
- Dynamic replica scaling

---

## 3. Architecture Overview

### 3.1 Control Plane vs Data Plane

| Concern | Engine (Control Plane) | Agent (Data Plane) |
|---------|------------------------|-------------------|
| Configuration | Parses banyan.yml | Receives service specs |
| Networking | Coordinates topology, manages DNS | Executes CNI, configures interfaces |
| Security | Manages security rules | Applies iptables rules locally |
| Containers | Orchestrates deployments | Runs containers locally |
| State | Tracks desired vs actual | Reports actual state |

### 3.2 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ENGINE (Control Plane)                            │
│                                                                             │
│  ┌──────────────┐                                                           │
│  │ Orchestrator │◄────────────────────────────────────────────────────┐     │
│  └──────┬───────┘                                                     │     │
│         │ depends on                                                  │     │
│         ▼                                                             │     │
│  ┌──────────────┐    ┌────────────────┐    ┌────────────────┐        │     │
│  │State Manager │◄───│ Agent Registry │    │ Plugin Manager │        │     │
│  └──────┬───────┘    └───────┬────────┘    └───────┬────────┘        │     │
│         │                    │                     │                  │     │
│         └────────────────────┼─────────────────────┘                  │     │
│                              │                                        │     │
│  ┌──────────────────┐        │        ┌──────────────────┐           │     │
│  │  Banyan Parser   │        │        │  VPC Coordinator │───────────┘     │
│  │  (independent)   │        │        │  (USES pkg/vpc/) │                 │
│  └──────────────────┘        │        └──────────────────┘                 │
│                              │                                             │
└──────────────────────────────┼─────────────────────────────────────────────┘
                               │ gRPC
                               ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                            AGENT (Data Plane)                               │
│                                                                             │
│  ┌───────────────┐                                                          │
│  │ Task Executor │◄────────────────────────────────────────────────┐        │
│  └───────┬───────┘                                                 │        │
│          │ routes to                                               │        │
│          ▼                                                         │        │
│  ┌─────────────────┐   ┌──────────────┐   ┌───────────────────┐   │        │
│  │Container Runtime│◄──│ Network Node │   │ Security Executor │   │        │
│  └────────┬────────┘   │(USES pkg/vpc)│   │ (USES pkg/vpc)    │   │        │
│           │            └──────────────┘   └───────────────────┘   │        │
│           │                    ┌────────────────┐                  │        │
│           └───────────────────►│ Health Monitor │──────────────────┘        │
│                                └────────────────┘                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.3 VPC Package Relationship

**IMPORTANT**: The `pkg/vpc/` package is **COMPLETE** and contains the full VPC implementation:
- Network Manager
- IPAM Manager
- Security Manager
- DNS Manager
- CNI Runtime

Agent and Engine components **USE** the VPC package - they do NOT modify it. The VPC package provides all networking primitives; Agent/Engine components are adapters that integrate VPC functionality.

### 3.4 Hexagonal Architecture

All components follow hexagonal architecture:

```
pkg/component/
├── domain/           # Entities and value objects
├── ports/
│   ├── inbound/      # Service interfaces (what the component offers)
│   └── outbound/     # Dependency interfaces (what the component needs)
├── usecases/         # Business logic implementation
└── adapters/         # External integrations
```

---

## 4. Supported Configuration (MVP-1)

### 4.1 banyan.yml Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | Yes | - | Container image |
| `replicas` | int | No | 1 | Number of instances |
| `ports` | list | No | - | Port mappings "host:container" |
| `environment` | list/map | No | - | Environment variables |
| `volumes` | list | No | - | Volume mounts |
| `depends_on` | list | No | - | Service dependencies |
| `healthcheck.test` | string | No | - | Health check command |
| `healthcheck.interval` | duration | No | 30s | Check interval |
| `command` | string/list | No | - | Override command |
| `restart` | string | No | unless-stopped | Restart policy |

### 4.2 NOT Supported (By Design)

| Feature | Reason |
|---------|--------|
| `build` | Use pre-built images |
| `networks` | Auto-networking, all services can reach each other |
| `deploy.resources` | Sensible defaults (can add later) |
| `deploy.placement` | Banyan distributes automatically |
| `secrets/configs` | Use environment variables |

---

## 5. Deployment Flow

```
User                CLI                 Engine              Agent(s)
 │                   │                    │                    │
 │ banyan up         │                    │                    │
 │──────────────────►│                    │                    │
 │                   │ Parse banyan.yml   │                    │
 │                   │───────────────────►│                    │
 │                   │                    │                    │
 │                   │                    │ Select Agents      │
 │                   │                    │ (Agent Registry)   │
 │                   │                    │                    │
 │                   │                    │ Execute Plugins    │
 │                   │                    │ (pre_deploy hook)  │
 │                   │                    │                    │
 │                   │                    │ Allocate IPs       │
 │                   │                    │ (VPC Coordinator)  │
 │                   │                    │                    │
 │                   │                    │ Register DNS       │
 │                   │                    │ (VPC Coordinator)  │
 │                   │                    │                    │
 │                   │                    │ Dispatch Tasks     │
 │                   │                    │────────────────────►
 │                   │                    │                    │
 │                   │                    │              Pull Images
 │                   │                    │              Setup Network
 │                   │                    │              Start Containers
 │                   │                    │              Apply Security
 │                   │                    │                    │
 │                   │                    │◄────────────────────
 │                   │                    │ Task Results       │
 │                   │                    │                    │
 │                   │                    │ Execute Plugins    │
 │                   │                    │ (post_deploy hook) │
 │                   │                    │                    │
 │                   │◄───────────────────│                    │
 │◄──────────────────│ Deployment Complete│                    │
```

---

## 6. Component Reference

| Component | Directory | Dependencies |
|-----------|-----------|--------------|
| **Agent Components** | | |
| Container Runtime | `pkg/agent/container/` | containerd |
| Network Node | `pkg/agent/network/` | **pkg/vpc/** |
| Security Executor | `pkg/agent/security/` | **pkg/vpc/** |
| Health Monitor | `pkg/agent/health/` | Container Runtime |
| Task Executor | `pkg/agent/task/` | All Agent components |
| Agent Server | `pkg/agent/server/` | Task Executor |
| **Engine Components** | | |
| Banyan Parser | `pkg/engine/parser/` | None |
| Agent Registry | `pkg/engine/registry/` | None |
| Plugin Manager | `pkg/engine/plugin/` | None |
| VPC Coordinator | `pkg/engine/vpc/` | **pkg/vpc/**, Agent Registry |
| State Manager | `pkg/engine/state/` | Agent Registry |
| Orchestrator | `pkg/engine/orchestrator/` | All Engine components |
| **VPC Package** | | |
| VPC (Complete) | `pkg/vpc/` | **DO NOT MODIFY** - only use |

---

## 7. Testing Strategy

### Test Categories

| Category | Location | Tags | Run With |
|----------|----------|------|----------|
| Unit | `*_test.go` next to code | none | `go test ./...` |
| Integration | `test/integration/` | `integration` | `go test -tags=integration` |
| E2E | `test/e2e/` | `e2e` | `go test -tags=e2e` |

### Mock Strategy

Each component has mocks for its outbound ports:
```
pkg/agent/container/
├── ports/outbound/
│   └── container_runtime.go    # Interface
├── mocks/
│   └── container_runtime_mock.go   # Mock implementation
```

---

## 8. Configuration Examples

### Engine Configuration

```yaml
# /etc/banyan/engine.yaml
engine:
  listen_addr: "0.0.0.0:7777"

storage:
  type: "etcd"
  endpoints:
    - "localhost:2379"

reconciliation:
  interval: 30s

logging:
  level: "info"
```

### Agent Configuration

```yaml
# /etc/banyan/agent.yaml
agent:
  id: "${HOSTNAME}"
  labels:
    region: "us-east-1"

engine:
  address: "engine.banyan.local:7777"

runtime:
  type: "containerd"
  socket: "/run/containerd/containerd.sock"

health:
  check_interval: 10s
  report_interval: 30s
```

---

*Document Version: 1.0*
*Date: 2026-01-01*
