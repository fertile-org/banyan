# Engine and Agent Architecture Design

**Date**: 2025-12-31
**Status**: Updated for banyan.yml approach
**Implementation**: Phase 3 complete (Agent Registry, Plugin Manager implemented)

## 1. Executive Summary

This document describes the architecture for Banyan's **Engine** (central orchestrator) and **Agent** (server-side runtime) modules.

### Project Philosophy

**"Docker Compose that scales"** - Banyan is for startups and small teams who:
- Know docker-compose from local development
- Don't have dedicated DevOps teams
- Don't want to learn Kubernetes
- Just want their containers to run on multiple servers

### Key Design Principles
- **Simple configuration**: `banyan.yml` uses docker-compose syntax with `replicas` for scaling
- **Implicit networking**: Services talk to each other by name (DNS-based discovery)
- **Plugin extensibility**: Advanced features (LB, auto-scaling, backup) via plugins
- **Interface-driven**: All components communicate via well-defined interfaces
- **Manager pattern**: Each concern has a dedicated manager with clear responsibilities

## 2. Configuration: banyan.yml

### 2.1 Core Configuration (MVP-1)

```yaml
# banyan.yml - Simple, familiar syntax
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - api

  api:
    image: ghcr.io/mycompany/api:latest
    replicas: 3  # ← The only new concept
    environment:
      - DATABASE_URL=postgres://db:5432/app
    healthcheck:
      test: curl -f http://localhost:3000/health
      interval: 30s
    depends_on:
      - db

  worker:
    image: ghcr.io/mycompany/worker:latest
    replicas: 2
    environment:
      - REDIS_URL=redis://redis:6379
    depends_on:
      - redis

  db:
    image: postgres:15
    environment:
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - db-data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine

volumes:
  db-data:
```

### 2.2 With Plugins (MVP-2+)

```yaml
services:
  api:
    image: ghcr.io/mycompany/api:latest
    replicas: 3
    healthcheck:
      test: curl -f http://localhost:3000/health
    plugins:
      - name: load_balancer
        config:
          port: 443
          target_port: 3000
          ssl:
            auto: true  # Let's Encrypt

  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data
    plugins:
      - name: database_backup
        config:
          schedule: "0 2 * * *"
          retention: 7d
          destination: s3://my-bucket/backups

volumes:
  db-data:
```

### 2.3 Supported Fields (MVP-1)

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

### 2.4 NOT Supported (By Design)

| Feature | Reason |
|---------|--------|
| `build` | Use pre-built images |
| `networks` | Auto-networking, all services can reach each other |
| `deploy.resources` | Sensible defaults (can add later) |
| `deploy.placement` | Banyan distributes automatically |
| `secrets/configs` | Use environment variables |

## 3. System Architecture

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
│                              CLI (User Machine)                              │
│  └─ banyan.yml parser → command router → gRPC client                         │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │ gRPC
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ENGINE (Control Plane - Orchestrator Server)              │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │
│  │ Banyan Parser   │  │ Deployment      │  │ Plugin          │              │
│  │ (banyan.yml)    │  │ Orchestrator    │  │ Manager         │              │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘              │
│           │                    │                    │                        │
│  ┌────────┴────────────────────┴────────────────────┴────────┐              │
│  │                    Service Plugins (per-service)          │              │
│  │   load_balancer │ database_backup │ auto_scaler           │              │
│  └───────────────────────────────────────────────────────────┘              │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                    VPC Control Plane (from pkg/vpc)                 │    │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌─────────────┐│    │
│  │  │ Network      │ │ IPAM         │ │ Security     │ │ DNS         ││    │
│  │  │ Manager      │ │ Manager      │ │ Manager      │ │ Manager     ││    │
│  │  └──────────────┘ └──────────────┘ └──────────────┘ └─────────────┘│    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
│  ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐               │
│  │ Agent          │  │ State           │  │ Service         │               │
│  │ Registry       │  │ Manager         │  │ Discovery       │               │
│  └────────────────┘  └─────────────────┘  └─────────────────┘               │
└─────────────────────────────────────────────────────────────────────────────┘
            │ gRPC (bidirectional streaming)
            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AGENT (Data Plane - Target Server)                        │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │
│  │ Container       │  │ Network Node    │  │ Health          │              │
│  │ Runtime         │  │ (CNI Executor)  │  │ Monitor         │              │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘              │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐                                   │
│  │ Security        │  │ Task            │  ← Executes commands from Engine  │
│  │ Executor        │  │ Executor        │  ← Reports state back to Engine   │
│  └─────────────────┘  └─────────────────┘                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 4. Engine Components

### 4.1 Directory Structure

```
pkg/engine/
├── engine.go                  # Main Engine implementation
├── types.go                   # Shared types
├── parser/                    # Banyan Parser (Phase 3.1)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── registry/                  # Agent Registry (Phase 3.2) ✅ IMPLEMENTED
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── plugin/                    # Plugin Manager (Phase 3.3) ✅ IMPLEMENTED
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── state/                     # State Manager (Phase 3.5)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── orchestrator/              # Deployment Orchestrator (Phase 3.6)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
└── vpc/                       # VPC Coordinator (Phase 3.4)
    └── coordinator.go
```

### 4.2 Agent Registry (Implemented)

The Agent Registry tracks all agents in the cluster and provides agent selection for deployments.

```go
// pkg/engine/registry/ports/inbound/service.go
type RegistryService interface {
    RegisterAgent(ctx context.Context, req *domain.RegisterAgentRequest) (*domain.Agent, error)
    DeregisterAgent(ctx context.Context, agentID domain.AgentID) error
    ProcessHeartbeat(ctx context.Context, agentID domain.AgentID, status *domain.HeartbeatStatus) error
    GetAgent(ctx context.Context, agentID domain.AgentID) (*domain.Agent, error)
    ListAgents(ctx context.Context, filter domain.AgentFilter) ([]domain.Agent, error)
    SelectAgents(ctx context.Context, criteria domain.SelectionCriteria, strategy string) ([]domain.Agent, error)
    DrainAgent(ctx context.Context, agentID domain.AgentID) error
    ActivateAgent(ctx context.Context, agentID domain.AgentID) error
}
```

**Selection Strategies:**
- `round_robin` - Distribute evenly across agents
- `least_loaded` - Prefer agents with most available resources
- `spread` - Maximize distribution across hosts
- `bin_pack` - Minimize number of hosts used

### 4.3 Plugin Manager (Implemented)

The Plugin Manager handles lifecycle hooks and plugin execution.

```go
// pkg/engine/plugin/ports/inbound/service.go
type PluginService interface {
    RegisterPlugin(ctx context.Context, plugin *domain.Plugin) error
    UnregisterPlugin(ctx context.Context, name string) error
    GetPlugin(ctx context.Context, name string) (*domain.Plugin, error)
    ListPlugins(ctx context.Context) ([]domain.Plugin, error)
    ListPluginsByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error)
    ExecuteHook(ctx context.Context, hook domain.HookPoint, execCtx domain.ExecutionContext) (*domain.HookResults, error)
    EnablePlugin(ctx context.Context, name string) error
    DisablePlugin(ctx context.Context, name string) error
    SetPriority(ctx context.Context, name string, priority int) error
}
```

**Hook Points:**
- `pre_deploy` - Before deployment starts
- `post_deploy` - After successful deployment
- `pre_destroy` - Before teardown
- `post_destroy` - After teardown
- `on_error` - When deployment fails
- `health_check` - During health monitoring

**Plugin Types:**
- `webhook` - HTTP webhooks (implemented)
- `grpc` - gRPC plugins (future)
- `script` - Shell scripts (future)
- `builtin` - Built-in Go plugins (future)

### 4.4 Banyan Parser (To Implement)

Parses banyan.yml into internal service specifications.

```go
// pkg/engine/parser/ports/inbound/service.go
type ParserService interface {
    Parse(ctx context.Context, content []byte) (*domain.BanyanConfig, error)
    ParseFile(ctx context.Context, path string) (*domain.BanyanConfig, error)
    Validate(ctx context.Context, config *domain.BanyanConfig) error
}
```

### 4.5 State Manager (To Implement)

Tracks desired vs actual state for reconciliation.

```go
// pkg/engine/state/ports/inbound/service.go
type StateService interface {
    SaveDesiredState(ctx context.Context, deploymentID string, state *domain.DesiredState) error
    GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error)
    UpdateActualState(ctx context.Context, deploymentID string, state *domain.ActualState) error
    GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error)
    GetStateDrift(ctx context.Context, deploymentID string) (*domain.StateDrift, error)
    TriggerReconcile(ctx context.Context, deploymentID string) error
}
```

### 4.6 Deployment Orchestrator (To Implement)

Coordinates the full deployment workflow.

```go
// pkg/engine/orchestrator/ports/inbound/service.go
type OrchestratorService interface {
    CreateDeployment(ctx context.Context, config *domain.BanyanConfig) (*domain.Deployment, error)
    ExecuteDeployment(ctx context.Context, deploymentID string) error
    GetDeploymentStatus(ctx context.Context, deploymentID string) (*domain.DeploymentStatus, error)
    RollbackDeployment(ctx context.Context, deploymentID string) error
    CancelDeployment(ctx context.Context, deploymentID string) error
}
```

## 5. Agent Components

### 5.1 Directory Structure

```
pkg/agent/
├── agent.go                   # Main Agent implementation
├── container/                 # Container Runtime (Phase 2.1)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── network/                   # Network Node (Phase 2.2)
│   └── node.go
├── security/                  # Security Executor (Phase 2.3)
│   └── executor.go
├── health/                    # Health Monitor (Phase 2.4)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
├── executor/                  # Task Executor (Phase 2.5)
│   ├── domain/
│   ├── ports/
│   ├── usecases/
│   └── adapters/
└── api/
    └── grpc/
```

### 5.2 Container Runtime

Manages container lifecycle via containerd.

```go
type ContainerRuntime interface {
    Create(ctx context.Context, spec *ContainerSpec) (string, error)
    Start(ctx context.Context, containerID string) error
    Stop(ctx context.Context, containerID string, timeout int) error
    Remove(ctx context.Context, containerID string, force bool) error
    Inspect(ctx context.Context, containerID string) (*ContainerInfo, error)
    List(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error)
    PullImage(ctx context.Context, image string) error
}
```

### 5.3 Network Node

Executes network operations locally. Receives commands from Engine.

```go
type NetworkNode interface {
    Initialize(ctx context.Context, config *NodeConfig) error
    AttachContainer(ctx context.Context, req *AttachRequest) (*AttachResult, error)
    DetachContainer(ctx context.Context, containerID string, networkID string) error
    GetStatus(ctx context.Context) (*NodeStatus, error)
}
```

### 5.4 Health Monitor

Tracks service health and reports to Engine.

```go
type HealthMonitor interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    RegisterService(ctx context.Context, spec *HealthSpec) error
    UnregisterService(ctx context.Context, serviceName string) error
    CheckService(ctx context.Context, serviceName string) (*HealthResult, error)
    GetServiceHealth(ctx context.Context, serviceName string) (*HealthStatus, error)
}
```

### 5.5 Task Executor

Handles deployment tasks from Engine.

```go
type TaskExecutor interface {
    Execute(ctx context.Context, task *Task) (*TaskResult, error)
    ExecuteAsync(ctx context.Context, task *Task) (TaskHandle, error)
    GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error)
    CancelTask(ctx context.Context, taskID string) error
}
```

## 6. Networking (Implicit)

### 6.1 Service Discovery

All services can reach each other by name. The VPC DNS handles resolution.

```yaml
# In banyan.yml
services:
  api:
    environment:
      - DATABASE_URL=postgres://db:5432/app  # "db" resolves via DNS
```

How it works:
1. When `api` service starts, Engine registers it with DNS
2. DNS returns IP(s) for service name lookups
3. For replicated services (`replicas: 3`), DNS returns all replica IPs (round-robin)

### 6.2 Network Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                         Engine                                   │
│  1. Parse banyan.yml                                             │
│  2. Allocate IPs from IPAM                                       │
│  3. Register DNS records                                         │
│  4. Send network config to agents                                │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Agent                                    │
│  5. Configure container network (CNI)                            │
│  6. Apply security rules (iptables)                              │
│  7. Container can reach other services by name                   │
└─────────────────────────────────────────────────────────────────┘
```

## 7. Plugin System

### 7.1 Per-Service Plugins

Plugins are defined per-service in banyan.yml:

```yaml
services:
  api:
    plugins:
      - name: load_balancer
        config:
          port: 443
          ssl:
            auto: true
```

### 7.2 Plugin Categories

| Plugin | Purpose | MVP Phase |
|--------|---------|-----------|
| `load_balancer` | Expose service with LB + SSL | MVP-2 |
| `auto_scaler` | Scale based on metrics | MVP-3 |
| `database_backup` | Scheduled backups | MVP-2 |
| `network_policy` | Explicit allow/deny rules | MVP-3 |

### 7.3 Plugin Interface

```go
// Per-service plugin interface
type ServicePlugin interface {
    Name() string
    Initialize(ctx context.Context, config map[string]any) error
    OnServiceCreate(ctx context.Context, service *ServiceSpec) error
    OnServiceUpdate(ctx context.Context, service *ServiceSpec) error
    OnServiceDelete(ctx context.Context, serviceName string) error
    HealthCheck(ctx context.Context) error
}
```

## 8. Deployment Flow

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
 │                   │                    │ (IPAM Manager)     │
 │                   │                    │                    │
 │                   │                    │ Register DNS       │
 │                   │                    │ (DNS Manager)      │
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

## 9. Implementation Status

### Phase 1: Foundation ✅
- [x] Shared domain models
- [x] Logger and configuration

### Phase 2: Agent Data Plane (In Progress)
- [ ] Container Runtime
- [ ] Network Node
- [ ] Security Executor
- [ ] Health Monitor
- [ ] Task Executor

### Phase 3: Engine Control Plane ✅
- [ ] Banyan Parser (partial - uses compose-go)
- [x] Agent Registry (complete with tests)
- [x] Plugin Manager (complete with webhook runner)
- [ ] VPC Coordinator
- [ ] State Manager
- [ ] Deployment Orchestrator

### Phase 4: Integration
- [ ] Engine-Agent gRPC communication
- [ ] Full deployment flow
- [ ] Health monitoring
- [ ] State reconciliation

### Future: MVP-2 Plugins
- [ ] load_balancer plugin
- [ ] database_backup plugin
- [ ] SSL/TLS with Let's Encrypt

### Future: MVP-3 Features
- [ ] auto_scaler plugin
- [ ] network_policy plugin

## 10. Configuration

### 10.1 Engine Configuration

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

### 10.2 Agent Configuration

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

*Document Version: 2.0*
*Updated: 2025-12-31*
*Architecture aligned with banyan.yml approach and MVP phases*
