# Banyan Engine & Agent Implementation Plan

This document provides a detailed implementation plan for the Banyan Engine and Agent components. The implementation order is designed to enable incremental testing and validation at each stage.

> **Important**: This plan follows the detailed design documents in `docs/engine/` and `docs/agent/` folders. Refer to those documents for complete specifications.

## Table of Contents

1. [Implementation Overview](#implementation-overview)
2. [Phase 1: Foundation Layer](#phase-1-foundation-layer)
3. [Phase 2: Agent Data Plane](#phase-2-agent-data-plane)
4. [Phase 3: Engine Control Plane](#phase-3-engine-control-plane)
5. [Phase 4: Integration & Orchestration](#phase-4-integration--orchestration)
6. [Testing Strategy](#testing-strategy)
7. [Milestones & Validation Criteria](#milestones--validation-criteria)

---

## Implementation Overview

### Dependency Graph

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
│  │  Compose Parser  │        │        │  VPC Coordinator │───────────┘     │
│  │  (independent)   │        │        │                  │                 │
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
│  └────────┬────────┘   └──────────────┘   └───────────────────┘   │        │
│           │                                                        │        │
│           │                    ┌────────────────┐                  │        │
│           └───────────────────►│ Health Monitor │──────────────────┘        │
│                                └────────────────┘                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Implementation Order Rationale

The implementation follows a **bottom-up, dependency-first** approach:

1. **Independent components first** - Start with components that have no internal dependencies
2. **Data plane before control plane** - Agent components must exist before Engine can coordinate them
3. **Infrastructure before business logic** - Foundation layers (domain, ports) before adapters
4. **Incremental integration** - Each phase produces a testable, working system

---

## Phase 1: Foundation Layer

**Duration**: Foundation for all subsequent phases
**Goal**: Establish shared infrastructure and domain models

### 1.1 Shared Domain Models

**Target**: Define core value objects and entities used across components

**Directory**: `pkg/shared/domain/`

**What to Implement**:
- Common value objects (IDs, timestamps, status enums)
- Error types and error handling patterns
- Event types for inter-component communication

**Standalone Testing**:
```bash
go test ./pkg/shared/domain/... -v
```
- Unit tests for value object validation
- Unit tests for error type behavior
- Test serialization/deserialization

**Integration**: Used by all subsequent components

---

### 1.2 Shared Infrastructure

**Target**: Common adapters and utilities

**Directory**: `pkg/shared/infrastructure/`

**What to Implement**:
- Logger interface and implementation
- Metrics collector interface
- Configuration loader
- gRPC utilities and interceptors

**Standalone Testing**:
```bash
go test ./pkg/shared/infrastructure/... -v
```
- Unit tests with mocked dependencies
- Test configuration parsing
- Test logging output formats

**Integration**: Import in all component adapters

---

## Phase 2: Agent Data Plane

Implement Agent components in dependency order. Each component is independently deployable and testable.

### 2.1 Container Runtime

**Design Document**: [docs/agent/container-runtime.md](agent/container-runtime.md)

**Target**: Manage container lifecycle via containerd

**Directory**: `pkg/agent/container/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `Container` entity
   - `ContainerID`, `ContainerStatus` value objects
   - `ContainerConfig`, `ResourceLimits` value objects
2. Inbound ports (`ports/inbound/`)
   - `ContainerService` interface
3. Outbound ports (`ports/outbound/`)
   - `ContainerRuntime` interface
   - `ImageManager` interface
   - `ContainerStore` interface
4. Use cases (`usecases/`)
   - `ContainerManager` implementation
5. Adapters (`adapters/`)
   - `ContainerdAdapter` (outbound)
   - `ImagePullAdapter` (outbound)
   - `BoltDBContainerStore` (outbound)

**Standalone Testing**:
```bash
# Unit tests with mocked containerd
go test ./pkg/agent/container/... -v

# Integration test with actual containerd (requires containerd running)
go test ./pkg/agent/container/adapters/... -v -tags=integration
```

**Test Scenarios**:
- Create container with resource limits
- Start/stop/restart container lifecycle
- Pull image from registry
- Container state persistence and recovery
- Resource constraint enforcement

**Integration Point**: Exposes `ContainerService` for Task Executor

---

### 2.2 Network Node

**Design Document**: [docs/agent/network-node.md](agent/network-node.md)

**Target**: Configure container networking at node level

**Directory**: `pkg/agent/network/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `NetworkNamespace` entity
   - `VethPair`, `IPAddress`, `Route` value objects
2. Inbound ports (`ports/inbound/`)
   - `NetworkNodeService` interface
3. Outbound ports (`ports/outbound/`)
   - `NamespaceManager` interface
   - `InterfaceManager` interface
   - `RouteManager` interface
   - `VXLANManager` interface
4. Use cases (`usecases/`)
   - `NetworkConfigurator` implementation
5. Adapters (`adapters/`)
   - `NetlinkNamespaceAdapter` (outbound)
   - `NetlinkInterfaceAdapter` (outbound)
   - `NetlinkRouteAdapter` (outbound)
   - `VXLANAdapter` (outbound)

**Standalone Testing**:
```bash
# Unit tests with mocked netlink
go test ./pkg/agent/network/... -v

# Integration test (requires root privileges, network namespaces)
sudo go test ./pkg/agent/network/adapters/... -v -tags=integration
```

**Test Scenarios**:
- Create/delete network namespace
- Configure veth pair between host and container namespace
- Assign IP addresses and routes
- Setup VXLAN tunnel for cross-node communication
- Verify network isolation

**Integration Point**: Called by Task Executor for container network setup

**Dependency**: Requires Container Runtime to be running (for namespace attachment)

---

### 2.3 Security Executor

**Design Document**: [docs/agent/security-executor.md](agent/security-executor.md)

**Target**: Enforce network security policies via iptables/nftables

**Directory**: `pkg/agent/security/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `SecurityPolicy` entity
   - `Rule`, `RuleSet` value objects
   - `IPSet` value object
2. Inbound ports (`ports/inbound/`)
   - `SecurityExecutorService` interface
3. Outbound ports (`ports/outbound/`)
   - `FirewallManager` interface
   - `IPSetManager` interface
   - `PolicyStore` interface
4. Use cases (`usecases/`)
   - `SecurityEnforcer` implementation
5. Adapters (`adapters/`)
   - `IptablesAdapter` (outbound)
   - `NftablesAdapter` (outbound)
   - `IPSetAdapter` (outbound)
   - `BoltDBPolicyStore` (outbound)

**Standalone Testing**:
```bash
# Unit tests with mocked iptables
go test ./pkg/agent/security/... -v

# Integration test (requires root privileges)
sudo go test ./pkg/agent/security/adapters/... -v -tags=integration
```

**Test Scenarios**:
- Apply ingress/egress rules
- Create and populate IP sets
- Rule ordering and priority
- Policy persistence and recovery
- Atomic rule updates (transactional)

**Integration Point**: Called by Task Executor for security policy enforcement

---

### 2.4 Health Monitor

**Design Document**: [docs/agent/health-monitor.md](agent/health-monitor.md)

**Target**: Monitor container and node health

**Directory**: `pkg/agent/health/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `HealthStatus` entity
   - `HealthCheck`, `ProbeResult` value objects
   - `ProbeType` enum (HTTP, TCP, Exec)
2. Inbound ports (`ports/inbound/`)
   - `HealthMonitorService` interface
3. Outbound ports (`ports/outbound/`)
   - `ProbeExecutor` interface
   - `ContainerInspector` interface
   - `HealthStore` interface
   - `AlertPublisher` interface
4. Use cases (`usecases/`)
   - `HealthChecker` implementation
5. Adapters (`adapters/`)
   - `HTTPProbeAdapter` (outbound)
   - `TCPProbeAdapter` (outbound)
   - `ExecProbeAdapter` (outbound)
   - `ContainerdInspectorAdapter` (outbound)

**Standalone Testing**:
```bash
# Unit tests with mocked probes
go test ./pkg/agent/health/... -v

# Integration test with test containers
go test ./pkg/agent/health/adapters/... -v -tags=integration
```

**Test Scenarios**:
- HTTP probe success/failure/timeout
- TCP probe connection check
- Exec probe command execution
- Liveness vs readiness probe behavior
- Health state transitions and alerts
- Startup probe grace period

**Integration Point**: Reports to Task Executor; queries Container Runtime

**Dependency**: Requires Container Runtime for container inspection

---

### 2.5 Task Executor

**Design Document**: [docs/agent/task-executor.md](agent/task-executor.md)

**Target**: Central task coordination and routing

**Directory**: `pkg/agent/executor/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `Task` entity
   - `TaskType`, `TaskStatus`, `TaskPriority` value objects
   - `TaskResult` value object
2. Inbound ports (`ports/inbound/`)
   - `TaskExecutorService` interface
3. Outbound ports (`ports/outbound/`)
   - `TaskQueue` interface
   - `TaskStore` interface
   - `ContainerService` interface (calls Container Runtime)
   - `NetworkService` interface (calls Network Node)
   - `SecurityService` interface (calls Security Executor)
   - `StatusReporter` interface
4. Use cases (`usecases/`)
   - `TaskRouter` implementation
   - `TaskScheduler` implementation
5. Adapters (`adapters/`)
   - `PriorityQueueAdapter` (outbound)
   - `BoltDBTaskStore` (outbound)
   - `gRPCStatusReporter` (outbound)
   - Inbound adapters connecting to other Agent components

**Standalone Testing**:
```bash
# Unit tests with mocked component interfaces
go test ./pkg/agent/executor/... -v
```

**Test Scenarios**:
- Task queue priority ordering
- Task routing to correct component
- Retry logic with exponential backoff
- Task state persistence
- Concurrent task execution limits
- Task cancellation and cleanup

**Integration Testing**:
```bash
# Full agent integration test
go test ./test/integration/agent/... -v -tags=integration
```

**Integration Scenarios**:
- Deploy container end-to-end (Task Executor → Container Runtime → Network Node → Security Executor)
- Health check triggers container restart
- Task failure handling and retry

---

### 2.6 Agent gRPC Server

**Target**: Expose Agent services to Engine via gRPC

**Directory**: `pkg/agent/grpc/`

**Implementation Order**:
1. Proto definitions (`api/proto/agent/`)
   - `agent.proto` - Task submission
   - `health.proto` - Health reporting
   - `status.proto` - Status updates
2. gRPC server implementation (`pkg/agent/grpc/`)
   - `AgentServer` implementing all service interfaces
   - Request validation and error mapping

**Standalone Testing**:
```bash
# gRPC server unit tests
go test ./pkg/agent/grpc/... -v
```

**Integration Testing**:
```bash
# Start agent, send gRPC requests
go test ./test/integration/agent/grpc/... -v -tags=integration
```

---

## Phase 3: Engine Control Plane

Implement Engine components. Some can be developed in parallel.

### 3.1 Compose Parser (Parallel Track A)

**Design Document**: [docs/engine/compose-parser.md](engine/compose-parser.md)

**Target**: Parse docker-compose.yaml and banyan.yaml files

**Directory**: `pkg/engine/parser/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `ComposeProject` entity
   - `ServiceDefinition`, `NetworkDefinition`, `VolumeDefinition` value objects
   - `BanyanExtension` value object
2. Inbound ports (`ports/inbound/`)
   - `ParserService` interface
3. Outbound ports (`ports/outbound/`)
   - `FileReader` interface
   - `SchemaValidator` interface
4. Use cases (`usecases/`)
   - `ComposeParser` implementation
   - `BanyanExtensionParser` implementation
5. Adapters (`adapters/`)
   - `ComposeGoAdapter` (using compose-go library)
   - `YAMLFileReader` (outbound)
   - `JSONSchemaValidator` (outbound)

**Standalone Testing**:
```bash
go test ./pkg/engine/parser/... -v
```

**Test Scenarios**:
- Parse valid docker-compose.yaml
- Parse banyan.yaml extensions
- Handle invalid YAML syntax
- Handle missing required fields
- Merge multiple compose files
- Variable substitution

**Integration Point**: Output feeds into Orchestrator deployment workflow

---

### 3.2 Agent Registry (Parallel Track B)

**Design Document**: [docs/engine/agent-registry.md](engine/agent-registry.md)

**Target**: Track registered agents, capabilities, and health

**Directory**: `pkg/engine/registry/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `Agent` entity
   - `AgentID`, `AgentStatus` value objects
   - `Capability`, `Resources` value objects
   - `SelectionCriteria` value object
2. Inbound ports (`ports/inbound/`)
   - `RegistryService` interface
3. Outbound ports (`ports/outbound/`)
   - `AgentRepository` interface
   - `EventPublisher` interface
4. Use cases (`usecases/`)
   - `AgentRegistrar` implementation
   - `AgentSelector` implementation (with strategies)
5. Adapters (`adapters/`)
   - `EtcdAgentRepository` (outbound)
   - `EventBusPublisher` (outbound)
   - `gRPCHeartbeatReceiver` (inbound)
6. Selection strategies (`usecases/strategies/`)
   - `RoundRobinStrategy`
   - `LeastLoadedStrategy`
   - `SpreadStrategy`
   - `BinPackStrategy`

**Standalone Testing**:
```bash
go test ./pkg/engine/registry/... -v
```

**Test Scenarios**:
- Agent registration and deregistration
- Heartbeat processing and timeout
- Capability-based agent filtering
- Selection strategy behavior
- Agent status transitions
- Concurrent registration handling

**Integration Testing**:
```bash
# With etcd
go test ./pkg/engine/registry/adapters/... -v -tags=integration
```

**Integration Point**: Used by State Manager, VPC Coordinator, Orchestrator

---

### 3.3 Plugin Manager (Parallel Track C)

**Design Document**: [docs/engine/plugin-manager.md](engine/plugin-manager.md)

**Target**: Manage lifecycle plugins (Type 2)

**Directory**: `pkg/engine/plugin/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `Plugin` entity
   - `HookPoint` enum
   - `ExecutionContext`, `PluginResult` value objects
2. Inbound ports (`ports/inbound/`)
   - `PluginService` interface
3. Outbound ports (`ports/outbound/`)
   - `PluginRepository` interface
   - `PluginRunner` interface
4. Use cases (`usecases/`)
   - `PluginExecutor` implementation
   - `PluginChain` implementation
5. Adapters (`adapters/`)
   - `EtcdPluginRepository` (outbound)
   - `WebhookRunner` (outbound)
   - `GRPCPluginRunner` (outbound)
   - `BuiltinRunner` (outbound)

**Standalone Testing**:
```bash
go test ./pkg/engine/plugin/... -v
```

**Test Scenarios**:
- Plugin registration and discovery
- Hook point execution order
- Webhook plugin with HTTP mock
- Plugin timeout handling
- Plugin failure modes (continue vs abort)
- Context passing between plugins

**Integration Testing**:
```bash
# With actual webhook server
go test ./pkg/engine/plugin/adapters/... -v -tags=integration
```

**Integration Point**: Called by Orchestrator at each lifecycle hook point

---

### 3.4 VPC Coordinator (Depends on 3.2)

**Design Document**: [docs/engine/vpc-coordinator.md](engine/vpc-coordinator.md)

**Target**: Bridge Engine to VPC networking layer

**Directory**: `pkg/engine/vpc/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `VPC`, `Subnet`, `SecurityGroup` entities
   - `ContainerNetwork` value object
   - `NetworkProvisionSpec` value object
2. Inbound ports (`ports/inbound/`)
   - `VPCCoordinatorService` interface
3. Outbound ports (`ports/outbound/`)
   - `NetworkManager` interface
   - `IPAMManager` interface
   - `SecurityManager` interface
   - `ContainerNetworkStore` interface
4. Use cases (`usecases/`)
   - `NetworkProvisioner` implementation
   - `SecurityPolicyManager` implementation
5. Adapters (`adapters/`)
   - Adapters connecting to existing VPC pkg (`pkg/vpc/`)
   - `EtcdContainerNetworkStore` (outbound)

**Standalone Testing**:
```bash
go test ./pkg/engine/vpc/... -v
```

**Test Scenarios**:
- Provision network for container
- Allocate IP from subnet
- Apply security group to container
- Release network resources
- Handle IPAM exhaustion

**Integration Testing**:
```bash
# With VPC infrastructure
go test ./pkg/engine/vpc/adapters/... -v -tags=integration
```

**Integration Point**: Called by Orchestrator; uses Agent Registry for agent selection

**Dependency**: Requires Agent Registry (3.2) for selecting target agents

---

### 3.5 State Manager (Depends on 3.2)

**Design Document**: [docs/engine/state-manager.md](engine/state-manager.md)

**Target**: Track desired vs actual state, detect drift

**Directory**: `pkg/engine/state/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `DesiredState`, `ActualState` entities
   - `StateDrift` value object
   - `ServiceState` value object
2. Inbound ports (`ports/inbound/`)
   - `StateService` interface
   - `ReconcilerService` interface
3. Outbound ports (`ports/outbound/`)
   - `StateRepository` interface
   - `AgentQuerier` interface
   - `ActionDispatcher` interface
4. Use cases (`usecases/`)
   - `StateTracker` implementation
   - `DriftDetector` implementation
   - `Reconciler` implementation
5. Adapters (`adapters/`)
   - `EtcdStateRepository` (outbound)
   - `gRPCAgentQuerier` (outbound)
   - `ActionDispatcherAdapter` (outbound)

**Standalone Testing**:
```bash
go test ./pkg/engine/state/... -v
```

**Test Scenarios**:
- Store and retrieve desired state
- Update actual state from agent reports
- Detect state drift
- Generate reconciliation actions
- Handle conflicting updates
- State versioning and optimistic locking

**Integration Testing**:
```bash
# With etcd and mock agents
go test ./pkg/engine/state/adapters/... -v -tags=integration
```

**Integration Point**: Core component used by Orchestrator

**Dependency**: Requires Agent Registry (3.2) for querying agent states

---

### 3.6 Orchestrator (Depends on 3.1-3.5)

**Design Document**: [docs/engine/orchestrator.md](engine/orchestrator.md)

**Target**: Coordinate deployment workflows

**Directory**: `pkg/engine/orchestrator/`

**Implementation Order**:
1. Domain layer (`domain/`)
   - `Deployment` entity
   - `DeploymentStatus`, `DeploymentPhase` value objects
   - `ServiceInstance` value object
2. Inbound ports (`ports/inbound/`)
   - `OrchestratorService` interface
3. Outbound ports (`ports/outbound/`)
   - `ParserService` interface (calls Compose Parser)
   - `RegistryService` interface (calls Agent Registry)
   - `PluginService` interface (calls Plugin Manager)
   - `VPCService` interface (calls VPC Coordinator)
   - `StateService` interface (calls State Manager)
   - `TaskDispatcher` interface (sends tasks to agents)
4. Use cases (`usecases/`)
   - `DeploymentWorkflow` implementation
   - `RollingUpdateStrategy` implementation
   - `BlueGreenStrategy` implementation
5. Adapters (`adapters/`)
   - Adapters connecting to other Engine components
   - `gRPCTaskDispatcher` (outbound to agents)

**Standalone Testing**:
```bash
# Unit tests with all dependencies mocked
go test ./pkg/engine/orchestrator/... -v
```

**Test Scenarios**:
- Full deployment workflow execution
- Plugin hook execution at each phase
- Agent selection and task dispatch
- State update during deployment
- Rollback on failure
- Concurrent deployment handling

**Integration Testing**: See Phase 4

---

### 3.7 Engine gRPC Server

**Target**: Expose Engine API and receive agent connections

**Directory**: `pkg/engine/grpc/`

**Implementation Order**:
1. Proto definitions (`api/proto/engine/`)
   - `deploy.proto` - Deployment API
   - `status.proto` - Status queries
   - `agent.proto` - Agent registration/heartbeat
2. gRPC server implementation
   - `EngineServer` implementing all service interfaces

**Standalone Testing**:
```bash
go test ./pkg/engine/grpc/... -v
```

---

## Phase 4: Integration & Orchestration

### 4.1 Engine-Agent Integration

**Target**: Verify complete Engine-Agent communication

**Test Directory**: `test/integration/`

**Test Scenarios**:

1. **Agent Lifecycle**
   ```
   Agent starts → Registers with Engine → Sends heartbeats →
   Engine tracks in registry → Agent disconnects → Engine marks unhealthy
   ```

2. **Simple Deployment**
   ```
   User submits compose file → Engine parses → Selects agent →
   Dispatches container task → Agent creates container →
   Reports status → Engine updates state
   ```

3. **Network Provisioning**
   ```
   Deployment includes network config → VPC Coordinator provisions →
   Agent configures network namespace → Container gets IP →
   Security rules applied
   ```

4. **Health Monitoring**
   ```
   Container deployed with health check → Agent monitors →
   Health check fails → Agent reports → Engine triggers restart
   ```

5. **State Reconciliation**
   ```
   Container crashes externally → Agent detects → Reports to Engine →
   State Manager detects drift → Orchestrator reconciles
   ```

**Integration Test Commands**:
```bash
# Full integration test suite
go test ./test/integration/... -v -tags=integration

# Specific scenario
go test ./test/integration/... -v -tags=integration -run TestDeploymentWorkflow
```

---

### 4.2 End-to-End Testing

**Target**: Complete system validation

**Test Directory**: `test/e2e/`

**Prerequisites**:
- Multi-node environment (3+ nodes)
- etcd cluster
- containerd on each node
- Network connectivity between nodes

**Test Scenarios**:

1. **Multi-Service Deployment**
   - Deploy multi-container application
   - Verify inter-service communication
   - Test service discovery

2. **Rolling Update**
   - Deploy v1 of service
   - Update to v2 with rolling strategy
   - Verify zero-downtime

3. **Failure Recovery**
   - Kill agent node
   - Verify workload migration
   - Node recovery and rejoin

4. **Network Policies**
   - Deploy with security groups
   - Verify traffic isolation
   - Test policy updates

**E2E Test Commands**:
```bash
# E2E test suite (requires full environment)
go test ./test/e2e/... -v -tags=e2e -timeout=30m
```

---

## Testing Strategy

### Test Categories

| Category | Location | Tags | Run With |
|----------|----------|------|----------|
| Unit | `*_test.go` next to code | none | `go test ./...` |
| Integration | `test/integration/` | `integration` | `go test -tags=integration` |
| E2E | `test/e2e/` | `e2e` | `go test -tags=e2e` |

### Mock Strategy

Each component should have mocks for its outbound ports:

```
pkg/agent/container/
├── ports/
│   └── outbound/
│       └── container_runtime.go    # Interface
├── mocks/
│   └── container_runtime_mock.go   # Mock implementation
```

Use `go generate` with mockgen for generating mocks:
```go
//go:generate mockgen -source=ports/outbound/container_runtime.go -destination=mocks/container_runtime_mock.go
```

### Test Data

Test fixtures location: `testdata/`

```
testdata/
├── compose/
│   ├── valid-simple.yaml
│   ├── valid-complex.yaml
│   └── invalid-syntax.yaml
├── banyan/
│   └── extensions.yaml
└── fixtures/
    └── agent_state.json
```

---

## Milestones & Validation Criteria

### Milestone 1: Agent Foundation
**Components**: Container Runtime, Network Node, Security Executor
**Validation**:
- [ ] Can create/start/stop container via Container Runtime
- [ ] Can configure network namespace via Network Node
- [ ] Can apply iptables rules via Security Executor
- [ ] All unit tests pass

### Milestone 2: Agent Complete
**Components**: Health Monitor, Task Executor, Agent gRPC Server
**Validation**:
- [ ] Task Executor routes tasks to correct components
- [ ] Health Monitor detects container health status
- [ ] Agent accepts gRPC connections
- [ ] End-to-end agent integration test passes

### Milestone 3: Engine Foundation
**Components**: Compose Parser, Agent Registry, Plugin Manager
**Validation**:
- [ ] Compose Parser handles standard docker-compose files
- [ ] Agent Registry tracks agent lifecycle
- [ ] Plugin Manager executes webhook plugins
- [ ] All unit tests pass

### Milestone 4: Engine Complete
**Components**: VPC Coordinator, State Manager, Orchestrator
**Validation**:
- [ ] VPC Coordinator provisions container networks
- [ ] State Manager detects and reports drift
- [ ] Orchestrator executes full deployment workflow
- [ ] All Engine unit tests pass

### Milestone 5: System Integration
**Validation**:
- [ ] Engine can register and track agents
- [ ] Full deployment workflow from compose file to running containers
- [ ] Health check failures trigger container restart
- [ ] State reconciliation restores desired state
- [ ] All integration tests pass

### Milestone 6: Production Ready
**Validation**:
- [ ] Multi-node deployment works
- [ ] Rolling updates with zero downtime
- [ ] Network policies enforced across nodes
- [ ] E2E test suite passes
- [ ] Documentation complete

---

## Appendix: Component Quick Reference

| Component | Directory | Design Doc | Dependencies |
|-----------|-----------|------------|--------------|
| Container Runtime | `pkg/agent/container/` | [container-runtime.md](agent/container-runtime.md) | None |
| Network Node | `pkg/agent/network/` | [network-node.md](agent/network-node.md) | Container Runtime |
| Security Executor | `pkg/agent/security/` | [security-executor.md](agent/security-executor.md) | None |
| Health Monitor | `pkg/agent/health/` | [health-monitor.md](agent/health-monitor.md) | Container Runtime |
| Task Executor | `pkg/agent/executor/` | [task-executor.md](agent/task-executor.md) | All Agent components |
| Compose Parser | `pkg/engine/parser/` | [compose-parser.md](engine/compose-parser.md) | None |
| Agent Registry | `pkg/engine/registry/` | [agent-registry.md](engine/agent-registry.md) | None |
| Plugin Manager | `pkg/engine/plugin/` | [plugin-manager.md](engine/plugin-manager.md) | None |
| VPC Coordinator | `pkg/engine/vpc/` | [vpc-coordinator.md](engine/vpc-coordinator.md) | Agent Registry |
| State Manager | `pkg/engine/state/` | [state-manager.md](engine/state-manager.md) | Agent Registry |
| Orchestrator | `pkg/engine/orchestrator/` | [orchestrator.md](engine/orchestrator.md) | All Engine components |

---

*This implementation plan follows the detailed designs in `docs/engine/` and `docs/agent/`. Refer to those documents for complete specifications of each component.*
