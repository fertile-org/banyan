# Agent & Engine MVP Implementation Plan

**Date**: 2026-01-01
**Status**: Active
**Design Document**: [agent-engine-mvp-design.md](agent-engine-mvp-design.md)

---

## Phase 1: Foundation Layer

**Goal**: Establish shared infrastructure and domain models used by all components.

**Directory**: `pkg/shared/`

### Checklist

- [x] **Shared Domain Models** (`pkg/shared/domain/`)
  - [x] Common value objects (IDs, timestamps, status enums)
  - [x] Error types and error handling patterns
  - [x] Event types for inter-component communication
  - [x] Unit tests

- [x] **Shared Infrastructure** (`pkg/shared/infrastructure/`)
  - [x] Logger interface and implementation
  - [x] Metrics collector interface
  - [x] Configuration loader
  - [x] gRPC utilities
  - [x] Unit tests

**Status**: ✅ COMPLETE

---

## Phase 2: Agent Data Plane

**Goal**: Implement all Agent components that run on target servers.

**Directory**: `pkg/agent/`

### 2.1 Container Runtime

**Design**: [docs/agent/container-runtime.md](agent/container-runtime.md)

- [x] Domain layer (Container entity, ContainerID, ContainerStatus, ContainerConfig)
- [x] Inbound port (ContainerService interface)
- [x] Outbound ports (ContainerRuntime, ImageManager interfaces)
- [x] Use case (ContainerUseCase implementation)
- [x] Adapters (ContainerdRuntime, ContainerdImageRegistry)
- [x] Unit tests
- [x] Integration tests with containerd

### 2.2 Network Node

**Design**: [docs/agent/network-node.md](agent/network-node.md)

- [x] Domain layer (NetworkConfig, IPAddress, NetworkStatus)
- [x] Inbound port (NetworkNodeService interface)
- [x] Outbound ports (CNIRuntime, IPAMClient interfaces)
- [x] Use case (NetworkUseCase implementation)
- [x] Adapters (VPCCNIAdapter, VPCIPAMAdapter) - uses `pkg/vpc/`
- [x] Unit tests

### 2.3 Security Executor

**Design**: [docs/agent/security-executor.md](agent/security-executor.md)

- [x] Domain layer (SecurityPolicy, Rule, RuleSet)
- [x] Inbound port (SecurityExecutorService interface)
- [x] Outbound ports (SecurityManager interface)
- [x] Use case (SecurityUseCase implementation)
- [x] Adapters (VPCSecurityAdapter) - uses `pkg/vpc/`
- [x] Unit tests

### 2.4 Health Monitor

**Design**: [docs/agent/health-monitor.md](agent/health-monitor.md)

- [x] Domain layer (HealthStatus, HealthCheck, ProbeResult, ProbeType)
- [x] Inbound port (HealthMonitorService interface)
- [x] Outbound ports (HealthChecker interface)
- [x] Use case (HealthUseCase implementation)
- [x] Adapters (HTTPChecker, TCPChecker, ExecChecker, SystemMonitor)
- [x] Unit tests

### 2.5 Task Executor

**Design**: [docs/agent/task-executor.md](agent/task-executor.md)

- [x] Domain layer (Task entity, TaskType, TaskStatus, TaskResult)
- [x] Inbound port (TaskExecutorService interface)
- [x] Outbound ports (ContainerService, NetworkService, SecurityService, HealthService, TaskStore, EventEmitter)
- [x] Use case (TaskUseCase implementation)
- [x] Adapters (Service adapters, MemoryTaskStore, MemoryEventEmitter)
- [x] Unit tests
- [x] Integration test with real containerd

### 2.6 Agent gRPC Server

- [x] Server configuration (`pkg/agent/server/config/`)
- [x] gRPC server implementation (`pkg/agent/server/grpc/`)
- [x] Service factory
- [x] Unit tests

**Status**: ✅ COMPLETE

---

## Phase 3: Engine Control Plane

**Goal**: Implement Engine components that orchestrate deployments.

**Directory**: `pkg/engine/`

### 3.1 Banyan Parser

**Design**: [docs/engine/banyan-parser.md](engine/banyan-parser.md)

- [x] Domain layer (BanyanConfig, ServiceConfig, VolumeConfig, Healthcheck)
- [x] Inbound port (ParserService interface)
- [x] Outbound ports (ConfigLoader interface)
- [x] Use case (ParseUseCase implementation)
- [x] Adapters (YAMLAdapter, ComposeGoAdapter, Interpolator)
- [x] Unit tests

### 3.2 Agent Registry

**Design**: [docs/engine/agent-registry.md](engine/agent-registry.md)

- [x] Domain layer (Agent entity, AgentID, AgentStatus, Capability, SelectionCriteria)
- [x] Inbound port (RegistryService interface)
- [x] Outbound ports (AgentRepository, EventPublisher interfaces)
- [x] Use case (RegistryUseCase with selection strategies)
- [x] Adapters (MemoryRepository, MemoryPublisher)
- [x] Selection strategies (RoundRobin, LeastLoaded, Spread, BinPack)
- [x] Unit tests

### 3.3 Plugin Manager

**Design**: [docs/engine/plugin-manager.md](engine/plugin-manager.md)

- [x] Domain layer (Plugin entity, HookPoint, ExecutionContext, PluginResult)
- [x] Inbound port (PluginService interface)
- [x] Outbound ports (PluginRepository, PluginRunner interfaces)
- [x] Use cases (PluginRegistry, PluginExecutor, PluginService)
- [x] Adapters (MemoryRepository, WebhookRunner, BuiltinRunner)
- [x] Unit tests

### 3.4 VPC Coordinator

**Design**: [docs/engine/vpc-coordinator.md](engine/vpc-coordinator.md)

- [ ] Domain layer (ContainerNetwork, NetworkProvisionSpec)
- [ ] Inbound port (VPCCoordinatorService interface)
- [ ] Outbound ports (interfaces wrapping `pkg/vpc/` types)
- [ ] Use cases (NetworkProvisioner, IPAllocator, SecurityPolicyManager)
- [ ] Adapters (thin wrappers around `pkg/vpc/` implementations)
- [ ] Unit tests

**Note**: VPC Coordinator is an ADAPTER that uses `pkg/vpc/`. It does NOT re-implement VPC.

### 3.5 State Manager

**Design**: [docs/engine/state-manager.md](engine/state-manager.md)

- [ ] Domain layer (DesiredState, ActualState, StateDrift, ServiceState)
- [ ] Inbound ports (StateService, ReconcilerService interfaces)
- [ ] Outbound ports (StateRepository, AgentQuerier, ActionDispatcher)
- [ ] Use cases (StateTracker, DriftDetector, Reconciler)
- [ ] Adapters (MemoryStateRepository or EtcdStateRepository, gRPCAgentQuerier)
- [ ] Unit tests

### 3.6 Orchestrator

**Design**: [docs/engine/orchestrator.md](engine/orchestrator.md)

- [ ] Domain layer (Deployment entity, DeploymentStatus, DeploymentPhase, ServiceInstance)
- [ ] Inbound port (OrchestratorService interface)
- [ ] Outbound ports (ParserService, RegistryService, PluginService, VPCService, StateService, TaskDispatcher)
- [ ] Use case (DeploymentWorkflow implementation)
- [ ] Adapters (internal adapters to other Engine components, gRPCTaskDispatcher)
- [ ] Unit tests

### 3.7 Engine gRPC Server

- [ ] Proto definitions (`api/proto/engine/`)
  - [ ] `deploy.proto` - Deployment API
  - [ ] `status.proto` - Status queries
  - [ ] `agent.proto` - Agent registration/heartbeat
- [ ] gRPC server implementation (`pkg/engine/grpc/`)
- [ ] Unit tests

**Status**: 🔶 IN PROGRESS (3/7 complete)

**Note on VPC Coordinator**: This component uses `pkg/vpc/` package (which is complete). The VPC Coordinator is a **facade** that coordinates IP allocation, DNS registration, and security policy management at the Engine level. It does NOT re-implement VPC - it orchestrates the existing VPC managers.

---

## Phase 4: Integration & Orchestration

**Goal**: Full system integration and end-to-end testing.

### 4.1 Engine-Agent Communication

- [ ] Agent registration flow (Agent → Engine)
- [ ] Heartbeat mechanism
- [ ] Task dispatch (Engine → Agent)
- [ ] Status reporting (Agent → Engine)

### 4.2 Deployment Flow

- [ ] Parse banyan.yml → Select agents → Provision network → Dispatch tasks
- [ ] Plugin hooks (pre_deploy, post_deploy)
- [ ] Container lifecycle management
- [ ] Service discovery via DNS

### 4.3 Health & Reconciliation

- [ ] Health check failures trigger restart
- [ ] State drift detection
- [ ] Automatic reconciliation

### 4.4 Integration Tests

- [ ] Agent lifecycle test (`test/integration/agent/`)
- [ ] Engine component integration (`test/integration/engine/`)
- [ ] Full deployment workflow (`test/integration/`)

### 4.5 E2E Tests

- [ ] Multi-node deployment (`test/e2e/`)
- [ ] Rolling updates
- [ ] Failure recovery

**Status**: ❌ NOT STARTED

---

## Quick Reference

| Phase | Components | Status |
|-------|------------|--------|
| 1. Foundation | Shared Domain, Infrastructure | ✅ Complete |
| 2. Agent | Container, Network, Security, Health, Task, Server | ✅ Complete |
| 3. Engine | Parser ✅, Registry ✅, Plugin ✅, VPC ❌, State ❌, Orchestrator ❌, gRPC ❌ | 🔶 In Progress |
| 4. Integration | Communication, Deployment, Reconciliation, Tests | ❌ Not Started |

---

## Next Steps

1. **Phase 3.4**: Implement VPC Coordinator - [design doc](engine/vpc-coordinator.md)
2. **Phase 3.5**: Implement State Manager - [design doc](engine/state-manager.md)
3. **Phase 3.6**: Implement Orchestrator - [design doc](engine/orchestrator.md)
4. **Phase 3.7**: Implement Engine gRPC Server

---

*Document Version: 1.0*
*Date: 2026-01-01*
