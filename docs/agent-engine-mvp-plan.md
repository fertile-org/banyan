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

- [x] Domain layer (ContainerNetwork, NetworkProvisionSpec)
- [x] Inbound port (VPCCoordinatorService interface)
- [x] Outbound ports (interfaces wrapping `pkg/vpc/` types)
- [x] Use cases (NetworkProvisioner, IPAllocator, SecurityPolicyManager)
- [x] Adapters (thin wrappers around `pkg/vpc/` implementations)
- [x] Unit tests

### 3.5 State Manager

**Design**: [docs/engine/state-manager.md](engine/state-manager.md)

- [x] Domain layer (DesiredState, ActualState, StateDrift, ServiceState)
- [x] Inbound ports (StateService, ReconcilerService interfaces)
- [x] Outbound ports (StateRepository, AgentQuerier, ActionDispatcher)
- [x] Use cases (StateTracker, DriftDetector, Reconciler)
- [x] Adapters (MemoryStateRepository, MemoryAgentQuerier, MemoryDispatcher)
- [x] Unit tests

### 3.6 Orchestrator

**Design**: [docs/engine/orchestrator.md](engine/orchestrator.md)

- [x] Domain layer (Deployment entity, DeploymentStatus, DeploymentPhase, ServiceInstance)
- [x] Inbound port (OrchestratorService interface)
- [x] Outbound ports (ParserService, RegistryService, PluginService, VPCService, StateService, TaskDispatcher)
- [x] Use case (DeploymentWorkflow implementation)
- [x] Adapters (MemoryDeploymentRepository, MemoryAgentDispatcher, MemoryScheduler, MemoryPluginExecutor, MemoryBanyanParser)
- [x] Unit tests

### 3.7 Engine gRPC Server

- [x] Server configuration (`pkg/engine/server/config/`)
- [x] gRPC server implementation (`pkg/engine/server/grpc/`)
- [x] Service factory
- [x] Unit tests

**Note**: Proto definitions for Engine-Agent communication (deploy.proto, status.proto, agent.proto) are not needed for MVP — etcd-based communication is used instead.

**Status**: ✅ COMPLETE

---

## Phase 4: Integration & Orchestration

**Goal**: Wire CLI binaries to real components and validate end-to-end.

### 4.1 Integration Tests (In-Memory) ✅ COMPLETE

- [x] Agent lifecycle test (`test/integration/integration/run_agent_lifecycle_integration.go`)
- [x] Simple deployment test (`test/integration/integration/run_simple_deployment_integration.go`)
- [x] Network provisioning test (`test/integration/integration/run_network_provisioning_integration.go`)
- [x] Health monitoring test (`test/integration/integration/run_health_monitoring_integration.go`)
- [x] State reconciliation test (`test/integration/integration/run_state_reconciliation_integration.go`)
- [x] Engine component tests (`test/integration/engine/`)
- [x] Agent task executor test (`test/integration/agent/`)

**Note**: These tests use in-memory adapters. They prove the component logic works but do not test real Engine-Agent communication.

### 4.2 CLI Wiring (Real Adapters) ✅ COMPLETE

The CLI binaries now use etcd-based communication for the full task execution loop:

```
deploy writes to etcd → Engine polls & orchestrates → Agent polls & executes → containers run
```

**4.2.1 Wire Engine Start** (`cmd/banyan-cli/cmd/engine.go`):
- [x] Replaced Memory* adapters with etcd polling loop (3s interval)
- [x] Scheduler reads agents from etcd, round-robin task assignment
- [x] Engine polls `/deployments/` prefix for pending deployments
- [x] Engine creates tasks at `/tasks/<agent>/<task-id>` for agents
- [x] Engine checks task completion and updates deployment status

**4.2.2 Wire Agent Start** (`cmd/banyan-cli/cmd/agent.go`):
- [x] Agent polls `/tasks/<node-name>/` for pending tasks (2s interval)
- [x] Executes tasks via nerdctl (pull + run)
- [x] Reports task results to etcd (completed/failed)
- [x] Heartbeat loop updates node LastSeen every 15s
- [x] Registers node on start, marks offline on shutdown

**4.2.3 Wire Deploy Command** (`cmd/banyan-cli/cmd/deploy.go`):
- [x] Writes DeploymentRecord with status "pending" to etcd
- [x] Polls for deployment status changes (2s interval, 2min timeout)
- [x] Reports progress to user (deploying → running/failed)

**4.2.4 Shared Types & Helpers** (`cmd/banyan-cli/cmd/types.go`, `helpers.go`):
- [x] Shared etcd protocol types (DeploymentRecord, TaskRecord, NodeRecord, ServiceRecord)
- [x] Extracted pure logic into testable functions (buildServiceRecords, buildTasksForDeployment, determineDeploymentStatus, buildNerdctlRunArgs)
- [x] Unit tests for all helper functions (14 tests)

### 4.3 E2E Tests ❌ NOT STARTED

**Infrastructure exists**: `test/e2e/docker-compose.yml` (1 Engine + 2 Workers)

- [ ] Multi-node deployment
- [ ] Rolling updates
- [ ] Failure recovery

**Status**: 🔶 IN PROGRESS (4.1-4.2 complete, 4.3 not started)

---

## Quick Reference

| Phase | Components | Status |
|-------|------------|--------|
| 1. Foundation | Shared Domain, Infrastructure | ✅ Complete |
| 2. Agent | Container, Network, Security, Health, Task, Server | ✅ Complete |
| 3. Engine | Parser, Registry, Plugin, VPC, State, Orchestrator, Server | ✅ Complete |
| 4. Integration | In-memory tests ✅, CLI wiring ✅, E2E ❌ | 🔶 In Progress |
| 5. DNS | Service Registry, DNS Manager, CoreDNS | ✅ Complete |
| 6. Production | Debug, Metrics, Flow Logs, Multi-CNI, Policies | ❌ Not Started |

---

## Next Steps (MVP-1 Release)

1. **Phase 4.3**: E2E tests — validate full flow in Docker-in-Docker
2. Update E2E `docker-compose.yml` and entrypoint scripts for the new CLI commands
3. Test: `banyan-cli engine start` → `banyan-cli agent start` → `banyan-cli deploy -f banyan.yaml`

---

*Document Version: 2.1*
*Last Updated: 2026-02-14*
