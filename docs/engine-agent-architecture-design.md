# Engine and Agent Architecture Design

**Date**: 2025-12-22
**Git Commit**: 18d1f2e
**Branch**: feat/vpc-implementation
**Status**: Proposal

## 1. Executive Summary

This document proposes the detailed architecture design for Banyan's **Engine** (central orchestrator) and **Agent** (server-side runtime) modules. The design follows patterns established in the VPC module and integrates with the two-plugin architecture.

### Key Design Principles
- **Interface-driven**: All components communicate via well-defined interfaces
- **Manager pattern**: Each concern has a dedicated manager with clear responsibilities
- **Dependency injection**: Components receive their dependencies, enabling testability
- **Storage abstraction**: State management through pluggable storage backends
- **Plugin extensibility**: Both lifecycle (engine-level) and service (agent-level) plugins

## 2. System Overview

### 2.1 Control Plane vs Data Plane

The architecture follows a clear separation between **control plane** (Engine) and **data plane** (Agent):

| Concern | Engine (Control Plane) | Agent (Data Plane) |
|---------|------------------------|-------------------|
| Network | NetworkManager, IPAMManager - coordinates topology, allocates IPs/subnets | NetworkNode - executes CNI operations, configures interfaces |
| Security | SecurityManager - defines and manages security rules | SecurityExecutor - applies iptables rules locally |
| DNS | DNSManager - manages DNS records centrally | Reports service IPs for registration |
| Containers | Orchestrates deployments across agents | ContainerRuntime - runs containers locally |

### 2.2 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLI (User Machine)                              │
│  ├─ compose.yaml parser                                                      │
│  ├─ banyan.yaml parser                                                       │
│  └─ command router → gRPC client                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │ gRPC
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ENGINE (Control Plane - Orchestrator Server)              │
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐              │
│  │ Deployment      │  │ State           │  │ Plugin          │              │
│  │ Orchestrator    │  │ Manager         │  │ Manager         │              │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘              │
│           │                    │                    │                        │
│  ┌────────┴────────────────────┴────────────────────┴────────┐              │
│  │                    Lifecycle Hooks (Type 2 Plugins)       │              │
│  │   Validate → Plan → Deploy → Verify → Destroy             │              │
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
│  │ Agent          │  │ Service         │  │ Network         │               │
│  │ Registry       │  │ Discovery       │  │ Topology        │               │
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
│  │ Security        │  │ Status          │  ← Executes commands from Engine  │
│  │ Executor        │  │ Reporter        │  ← Reports state back to Engine   │
│  └─────────────────┘  └─────────────────┘                                   │
│           │                    │                                             │
│  ┌────────┴────────────────────┴────────────────────────────┐               │
│  │                Service Plugins (Type 1 - Sidecars)       │               │
│  │   Load Balancer │ Monitoring Agent │ Backup │ etc.       │               │
│  └──────────────────────────────────────────────────────────┘               │
│           │                                                                  │
│  ┌────────┴────────┐  ┌─────────────────┐                                   │
│  │ Log             │  │ Metrics         │                                   │
│  │ Collector       │  │ Collector       │                                   │
│  └─────────────────┘  └─────────────────┘                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 3. Engine Architecture

### 3.1 Directory Structure

```
cmd/engine/
├── main.go                    # Entry point, wire dependencies
└── config.go                  # Configuration loading

pkg/engine/
├── engine.go                  # Engine interface implementation
├── types.go                   # Engine-specific types
├── orchestrator/
│   ├── orchestrator.go        # Deployment orchestrator
│   ├── scheduler.go           # Deployment scheduling logic
│   └── rollback.go            # Rollback strategies
├── state/
│   ├── manager.go             # State manager interface & impl
│   ├── deployment.go          # Deployment state tracking
│   └── reconciler.go          # State reconciliation
├── registry/
│   ├── agent.go               # Agent registry
│   └── heartbeat.go           # Agent heartbeat handling
├── plugin/
│   ├── manager.go             # Plugin lifecycle manager
│   ├── registry.go            # Plugin registration
│   ├── grpc_host.go           # gRPC plugin host
│   └── hooks.go               # Lifecycle hook definitions
├── vpc/                       # VPC Control Plane (integrates with pkg/vpc)
│   ├── coordinator.go         # Cross-agent network coordination
│   ├── topology.go            # Network topology management
│   └── bridge.go              # Bridge to pkg/vpc managers
├── discovery/
│   └── service.go             # Service discovery coordination
└── api/
    ├── grpc/
    │   ├── server.go          # gRPC server
    │   └── handlers.go        # Request handlers
    └── proto/
        └── engine.proto       # Protocol definitions

# Note: Engine uses existing pkg/vpc managers (NetworkManager, IPAMManager,
# SecurityManager, DNSManager) for control plane operations
```

### 3.2 Core Interfaces

```go
// pkg/engine/engine.go

package engine

import (
    "context"
    "github.com/fertile-org/banyan/pkg/interfaces"
)

// Engine is the main orchestrator interface
type Engine interface {
    // Deployment operations
    Deploy(ctx context.Context, req *DeploymentRequest) (*DeploymentResult, error)
    Rollback(ctx context.Context, deploymentID string) error
    Cancel(ctx context.Context, deploymentID string) error

    // Status and monitoring
    GetDeploymentStatus(ctx context.Context, deploymentID string) (*DeploymentStatus, error)
    ListDeployments(ctx context.Context, filter *DeploymentFilter) ([]*Deployment, error)

    // Agent management
    RegisterAgent(ctx context.Context, agent *AgentInfo) error
    DeregisterAgent(ctx context.Context, agentID string) error
    GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)
    ListAgents(ctx context.Context) ([]*AgentInfo, error)

    // Plugin management (Type 2 - Lifecycle plugins)
    RegisterPlugin(ctx context.Context, plugin *PluginInfo) error
    UnregisterPlugin(ctx context.Context, pluginID string) error
    ListPlugins(ctx context.Context) ([]*PluginInfo, error)

    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HealthCheck(ctx context.Context) (*HealthStatus, error)
}
```

### 3.3 Orchestrator Component

```go
// pkg/engine/orchestrator/orchestrator.go

package orchestrator

import "context"

// Orchestrator manages deployment workflows
type Orchestrator interface {
    // CreateDeployment creates a new deployment workflow
    CreateDeployment(ctx context.Context, req *DeploymentRequest) (*Deployment, error)

    // ExecuteDeployment runs the deployment pipeline
    // Pipeline: Validate → Plan → Deploy → Verify
    ExecuteDeployment(ctx context.Context, deploymentID string) error

    // RollbackDeployment reverts to previous state
    RollbackDeployment(ctx context.Context, deploymentID string, strategy RollbackStrategy) error

    // GetDeploymentPlan returns the planned actions without executing
    GetDeploymentPlan(ctx context.Context, req *DeploymentRequest) (*DeploymentPlan, error)
}

// Scheduler determines deployment order and parallelism
type Scheduler interface {
    // Schedule creates an execution plan for services
    Schedule(ctx context.Context, services []*Service, deps *DependencyGraph) (*ExecutionPlan, error)

    // ValidateDependencies checks for circular dependencies
    ValidateDependencies(ctx context.Context, deps *DependencyGraph) error
}
```

### 3.4 State Management

```go
// pkg/engine/state/manager.go

package state

import "context"

// StateManager handles deployment and agent state
type StateManager interface {
    // Deployment state
    SaveDeployment(ctx context.Context, deployment *Deployment) error
    GetDeployment(ctx context.Context, id string) (*Deployment, error)
    UpdateDeploymentStatus(ctx context.Context, id string, status DeploymentState) error
    ListDeployments(ctx context.Context, filter *Filter) ([]*Deployment, error)

    // Agent state
    SaveAgentState(ctx context.Context, agentID string, state *AgentState) error
    GetAgentState(ctx context.Context, agentID string) (*AgentState, error)

    // Service state (desired vs actual)
    SaveDesiredState(ctx context.Context, deploymentID string, state *DesiredState) error
    GetDesiredState(ctx context.Context, deploymentID string) (*DesiredState, error)
    SaveActualState(ctx context.Context, deploymentID string, state *ActualState) error
    GetActualState(ctx context.Context, deploymentID string) (*ActualState, error)

    // Reconciliation
    GetStateDrift(ctx context.Context, deploymentID string) (*StateDrift, error)
}

// Reconciler continuously reconciles desired vs actual state
type Reconciler interface {
    // Start begins the reconciliation loop
    Start(ctx context.Context) error

    // Stop halts reconciliation
    Stop(ctx context.Context) error

    // TriggerReconcile forces immediate reconciliation
    TriggerReconcile(ctx context.Context, deploymentID string) error
}
```

### 3.5 Agent Registry

```go
// pkg/engine/registry/agent.go

package registry

import (
    "context"
    "time"
)

// AgentRegistry manages agent registrations
type AgentRegistry interface {
    // Registration
    Register(ctx context.Context, info *AgentInfo) error
    Deregister(ctx context.Context, agentID string) error

    // Queries
    Get(ctx context.Context, agentID string) (*AgentInfo, error)
    List(ctx context.Context, filter *AgentFilter) ([]*AgentInfo, error)
    GetHealthy(ctx context.Context) ([]*AgentInfo, error)

    // Heartbeat
    RecordHeartbeat(ctx context.Context, agentID string, status *AgentHealth) error
    GetLastHeartbeat(ctx context.Context, agentID string) (time.Time, error)

    // Selection (for deployment targeting)
    SelectAgents(ctx context.Context, selector *AgentSelector) ([]*AgentInfo, error)
}

// AgentInfo contains agent metadata
type AgentInfo struct {
    ID          string            `json:"id"`
    Hostname    string            `json:"hostname"`
    Address     string            `json:"address"`     // gRPC address
    Labels      map[string]string `json:"labels"`      // For targeting
    Capacity    *AgentCapacity    `json:"capacity"`    // Resources available
    Version     string            `json:"version"`     // Agent version
    Status      AgentStatus       `json:"status"`      // online, offline, draining
    RegisteredAt time.Time        `json:"registered_at"`
    LastSeen    time.Time         `json:"last_seen"`
}
```

### 3.6 Lifecycle Plugin System (Type 2)

```go
// pkg/engine/plugin/hooks.go

package plugin

import "context"

// LifecycleHook defines when a plugin is invoked
type LifecycleHook string

const (
    HookValidate LifecycleHook = "validate"  // Before deployment starts
    HookPlan     LifecycleHook = "plan"      // After planning, before execution
    HookDeploy   LifecycleHook = "deploy"    // During deployment to each agent
    HookVerify   LifecycleHook = "verify"    // After deployment, before complete
    HookDestroy  LifecycleHook = "destroy"   // During teardown
)

// PluginManager manages lifecycle plugins
type PluginManager interface {
    // Plugin registration
    RegisterPlugin(ctx context.Context, plugin *PluginInfo) error
    UnregisterPlugin(ctx context.Context, pluginID string) error
    ListPlugins(ctx context.Context) ([]*PluginInfo, error)

    // Plugin discovery
    DiscoverPlugins(ctx context.Context, path string) ([]*PluginInfo, error)

    // Hook execution
    ExecuteHook(ctx context.Context, hook LifecycleHook, data *HookData) (*HookResult, error)

    // Plugin lifecycle
    StartPlugin(ctx context.Context, pluginID string) error
    StopPlugin(ctx context.Context, pluginID string) error
}

// PluginInfo describes a lifecycle plugin
type PluginInfo struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Version     string          `json:"version"`
    Hooks       []LifecycleHook `json:"hooks"`        // Which hooks to register for
    GRPCAddress string          `json:"grpc_address"` // Plugin's gRPC endpoint
    Config      map[string]any  `json:"config"`
}

// HookData is passed to plugins during hook execution
type HookData struct {
    DeploymentID string         `json:"deployment_id"`
    Hook         LifecycleHook  `json:"hook"`
    Services     []*ServiceSpec `json:"services"`
    Context      map[string]any `json:"context"` // Hook-specific context
}
```

### 3.7 gRPC API

```protobuf
// pkg/engine/api/proto/engine.proto

syntax = "proto3";
package banyan.engine.v1;

service EngineService {
    // Deployment operations
    rpc Deploy(DeployRequest) returns (DeployResponse);
    rpc GetDeploymentStatus(GetStatusRequest) returns (DeploymentStatus);
    rpc CancelDeployment(CancelRequest) returns (CancelResponse);
    rpc RollbackDeployment(RollbackRequest) returns (RollbackResponse);

    // Agent operations (called by agents)
    rpc RegisterAgent(RegisterAgentRequest) returns (RegisterAgentResponse);
    rpc Heartbeat(stream HeartbeatRequest) returns (stream HeartbeatResponse);
    rpc ReportStatus(StatusReport) returns (StatusAck);

    // Watch for deployment tasks (agent pulls work)
    rpc WatchTasks(WatchTasksRequest) returns (stream DeploymentTask);
}

message DeployRequest {
    string deployment_id = 1;
    string compose_file = 2;    // docker-compose.yaml content
    string banyan_config = 3;   // banyan.yaml content
    repeated string targets = 4; // Agent selectors
    map<string, string> variables = 5;
}

message DeploymentTask {
    string task_id = 1;
    string deployment_id = 2;
    TaskType type = 3;
    bytes payload = 4;  // Task-specific payload
}

enum TaskType {
    TASK_UNKNOWN = 0;
    TASK_DEPLOY_SERVICE = 1;
    TASK_STOP_SERVICE = 2;
    TASK_UPDATE_NETWORK = 3;
    TASK_HEALTH_CHECK = 4;
}
```

## 4. Agent Architecture

### 4.1 Directory Structure

```
cmd/agent/
├── main.go                    # Entry point
└── config.go                  # Configuration

pkg/agent/
├── agent.go                   # Agent interface implementation
├── types.go                   # Agent-specific types
├── runtime/
│   ├── docker.go              # Docker runtime implementation
│   ├── containerd.go          # containerd runtime (future)
│   └── interface.go           # Container runtime interface
├── network/
│   ├── node.go                # NetworkNode - executes CNI operations locally
│   └── cni_executor.go        # CNI plugin execution (uses pkg/vpc/cni)
├── security/
│   └── executor.go            # SecurityExecutor - applies iptables locally
├── status/
│   └── reporter.go            # Reports local state to Engine
├── health/
│   ├── monitor.go             # Health monitoring
│   ├── checker.go             # Health check execution
│   └── reporter.go            # Health reporting to engine
├── plugin/
│   ├── manager.go             # Service plugin manager (Type 1)
│   ├── sidecar.go             # Sidecar lifecycle management
│   └── registry.go            # Plugin registration
├── collector/
│   ├── logs.go                # Log collection
│   └── metrics.go             # Metrics collection
├── executor/
│   ├── executor.go            # Task executor
│   └── task_handlers.go       # Individual task handlers
└── api/
    ├── grpc/
    │   ├── client.go          # gRPC client to engine
    │   └── handlers.go        # Local API handlers
    └── proto/
        └── agent.proto

# Note: Agent is DATA PLANE only - it executes operations commanded by Engine.
# It does NOT manage network state - it receives instructions and executes them.
```

### 4.2 Core Interfaces

```go
// pkg/agent/agent.go

package agent

import "context"

// Agent is the main agent interface
type Agent interface {
    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Engine connection
    ConnectToEngine(ctx context.Context, engineAddr string) error
    Disconnect(ctx context.Context) error

    // Task execution
    ExecuteTask(ctx context.Context, task *Task) (*TaskResult, error)

    // Service management
    DeployService(ctx context.Context, spec *ServiceSpec) error
    StopService(ctx context.Context, serviceName string) error
    GetServiceStatus(ctx context.Context, serviceName string) (*ServiceStatus, error)
    ListServices(ctx context.Context) ([]*ServiceStatus, error)

    // Health
    HealthCheck(ctx context.Context) (*HealthStatus, error)

    // Plugin management (Type 1 - Service plugins)
    DeploySidecar(ctx context.Context, serviceName string, plugin *SidecarSpec) error
    StopSidecar(ctx context.Context, serviceName string, pluginName string) error
}
```

### 4.3 Container Runtime

```go
// pkg/agent/runtime/interface.go

package runtime

import (
    "context"
    "io"
)

// ContainerRuntime abstracts container operations
type ContainerRuntime interface {
    // Container lifecycle
    Create(ctx context.Context, spec *ContainerSpec) (string, error)
    Start(ctx context.Context, containerID string) error
    Stop(ctx context.Context, containerID string, timeout int) error
    Remove(ctx context.Context, containerID string, force bool) error

    // Container inspection
    Inspect(ctx context.Context, containerID string) (*ContainerInfo, error)
    List(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error)

    // Logs and exec
    Logs(ctx context.Context, containerID string, opts *LogOptions) (io.ReadCloser, error)
    Exec(ctx context.Context, containerID string, cmd []string) (*ExecResult, error)

    // Image management
    PullImage(ctx context.Context, image string) error
    ImageExists(ctx context.Context, image string) (bool, error)

    // Network
    ConnectNetwork(ctx context.Context, containerID, networkID string) error
    DisconnectNetwork(ctx context.Context, containerID, networkID string) error
}

// ContainerSpec defines a container to create
type ContainerSpec struct {
    Name        string            `json:"name"`
    Image       string            `json:"image"`
    Command     []string          `json:"command,omitempty"`
    Env         map[string]string `json:"env,omitempty"`
    Labels      map[string]string `json:"labels,omitempty"`
    Ports       []PortMapping     `json:"ports,omitempty"`
    Volumes     []VolumeMount     `json:"volumes,omitempty"`
    NetworkMode string            `json:"network_mode,omitempty"`
    Resources   *ResourceSpec     `json:"resources,omitempty"`
    HealthCheck *HealthCheckSpec  `json:"health_check,omitempty"`
}
```

### 4.4 Network Node (Data Plane)

The Agent's network component is a **NetworkNode** - it executes network operations locally but does NOT manage network state. All coordination and state management happens in the Engine's control plane.

```go
// pkg/agent/network/node.go

package network

import (
    "context"
    "net"
)

// NetworkNode executes network operations on the local host.
// It receives commands from Engine and applies them locally.
// It does NOT manage network state - that's the Engine's responsibility.
type NetworkNode interface {
    // Initialize prepares the local network stack
    // Called once when agent starts, using config from Engine
    Initialize(ctx context.Context, config *NodeConfig) error

    // AttachContainer connects a container to the network
    // Engine provides the allocated IP; NetworkNode configures the interface
    AttachContainer(ctx context.Context, req *AttachRequest) (*AttachResult, error)

    // DetachContainer removes a container from the network
    DetachContainer(ctx context.Context, containerID string, networkID string) error

    // GetStatus reports local network state to Engine
    GetStatus(ctx context.Context) (*NodeStatus, error)
}

// AttachRequest contains Engine-provided network config for a container
type AttachRequest struct {
    ContainerID string `json:"container_id"`
    NetworkID   string `json:"network_id"`
    IP          net.IP `json:"ip"`      // Allocated by Engine's IPAMManager
    Gateway     net.IP `json:"gateway"` // Provided by Engine
    DNS         []net.IP `json:"dns"`   // Provided by Engine
}

// AttachResult confirms the attachment
type AttachResult struct {
    Success     bool   `json:"success"`
    Interface   string `json:"interface"`   // e.g., "eth0"
    MacAddress  string `json:"mac_address"`
    Error       string `json:"error,omitempty"`
}

// NodeConfig is provided by Engine during initialization
type NodeConfig struct {
    HostID        string   `json:"host_id"`
    NetworkID     string   `json:"network_id"`
    Subnet        string   `json:"subnet"`        // This host's subnet (from Engine)
    Gateway       net.IP   `json:"gateway"`
    DNS           []net.IP `json:"dns"`
    VxlanID       int      `json:"vxlan_id"`
    CNIPluginPath string   `json:"cni_plugin_path"`
}

// NodeStatus reports local state to Engine for reconciliation
type NodeStatus struct {
    HostID           string            `json:"host_id"`
    Containers       []ContainerNet    `json:"containers"`        // Active container networks
    InterfaceStatus  string            `json:"interface_status"`  // up, down, error
    LastError        string            `json:"last_error,omitempty"`
}
```

### 4.5 Security Executor (Data Plane)

```go
// pkg/agent/security/executor.go

package security

import (
    "context"

    "github.com/fertile-org/banyan/pkg/vpc"
)

// SecurityExecutor applies security rules locally.
// Rules are defined by Engine's SecurityManager; Executor just applies them.
type SecurityExecutor interface {
    // ApplyRules applies iptables rules received from Engine
    ApplyRules(ctx context.Context, rules []*vpc.SecurityRule) error

    // RemoveRules removes specific rules
    RemoveRules(ctx context.Context, ruleIDs []string) error

    // GetAppliedRules returns currently applied rules (for status reporting)
    GetAppliedRules(ctx context.Context) ([]*AppliedRule, error)

    // Flush removes all banyan-managed rules (used during cleanup)
    Flush(ctx context.Context) error
}

// AppliedRule represents a rule that has been applied locally
type AppliedRule struct {
    RuleID    string `json:"rule_id"`
    IPTables  string `json:"iptables"`  // The actual iptables rule
    AppliedAt string `json:"applied_at"`
}
```

### 4.6 Status Reporter

```go
// pkg/agent/status/reporter.go

package status

import (
    "context"
    "time"
)

// StatusReporter sends local state to Engine for reconciliation
type StatusReporter interface {
    // Start begins periodic status reporting
    Start(ctx context.Context) error

    // Stop halts reporting
    Stop(ctx context.Context) error

    // ReportNow sends an immediate status update
    ReportNow(ctx context.Context) error

    // SetReportInterval configures reporting frequency
    SetReportInterval(interval time.Duration)
}

// AgentStatus is sent to Engine periodically
type AgentStatus struct {
    AgentID        string                 `json:"agent_id"`
    Timestamp      time.Time              `json:"timestamp"`
    NetworkStatus  *NetworkNodeStatus     `json:"network_status"`
    SecurityStatus *SecurityStatus        `json:"security_status"`
    Containers     []ContainerStatus      `json:"containers"`
    Resources      *ResourceStatus        `json:"resources"`
}
```

### 4.7 Health Monitoring

```go
// pkg/agent/health/monitor.go

package health

import (
    "context"
    "time"
)

// HealthMonitor tracks service health
type HealthMonitor interface {
    // Start/stop monitoring
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Register services for monitoring
    RegisterService(ctx context.Context, spec *HealthSpec) error
    UnregisterService(ctx context.Context, serviceName string) error

    // Manual checks
    CheckService(ctx context.Context, serviceName string) (*HealthResult, error)
    CheckAll(ctx context.Context) ([]*HealthResult, error)

    // Status
    GetServiceHealth(ctx context.Context, serviceName string) (*HealthStatus, error)
    GetAgentHealth(ctx context.Context) (*AgentHealth, error)
}

// HealthSpec defines health check configuration
type HealthSpec struct {
    ServiceName string        `json:"service_name"`
    Type        HealthType    `json:"type"`         // http, tcp, exec
    Endpoint    string        `json:"endpoint"`     // URL, host:port, or command
    Interval    time.Duration `json:"interval"`
    Timeout     time.Duration `json:"timeout"`
    Retries     int           `json:"retries"`
}

type HealthType string

const (
    HealthHTTP HealthType = "http"
    HealthTCP  HealthType = "tcp"
    HealthExec HealthType = "exec"
)
```

### 4.8 Service Plugin System (Type 1 - Sidecars)

```go
// pkg/agent/plugin/manager.go

package plugin

import "context"

// SidecarManager manages service-level plugins
type SidecarManager interface {
    // Plugin lifecycle
    Deploy(ctx context.Context, serviceName string, spec *SidecarSpec) error
    Stop(ctx context.Context, serviceName string, pluginName string) error

    // Status
    GetStatus(ctx context.Context, serviceName string, pluginName string) (*SidecarStatus, error)
    ListSidecars(ctx context.Context, serviceName string) ([]*SidecarStatus, error)

    // Communication
    SendToSidecar(ctx context.Context, serviceName, pluginName string, msg *Message) (*Response, error)
}

// SidecarSpec defines a sidecar plugin
type SidecarSpec struct {
    Name       string            `json:"name"`       // e.g., "application_load_balancer"
    Image      string            `json:"image"`      // Container image
    Parameters map[string]any    `json:"parameters"` // Plugin-specific config
    Placement  PlacementStrategy `json:"placement"`  // with-service or standalone
}

// PlacementStrategy determines sidecar deployment
type PlacementStrategy string

const (
    // PlacementWithService deploys sidecar on same host as service
    PlacementWithService PlacementStrategy = "with-service"
    // PlacementStandalone deploys sidecar on dedicated host (e.g., ALB)
    PlacementStandalone PlacementStrategy = "standalone"
)
```

### 4.9 Task Executor

```go
// pkg/agent/executor/executor.go

package executor

import "context"

// TaskExecutor handles deployment tasks from engine
type TaskExecutor interface {
    // Task execution
    Execute(ctx context.Context, task *Task) (*TaskResult, error)

    // Async execution
    ExecuteAsync(ctx context.Context, task *Task) (TaskHandle, error)
    GetTaskStatus(ctx context.Context, taskID string) (*TaskStatus, error)
    CancelTask(ctx context.Context, taskID string) error

    // Task queue
    QueueTask(ctx context.Context, task *Task) error
    DrainQueue(ctx context.Context) error
}

// Task represents work from the engine
type Task struct {
    ID           string                 `json:"id"`
    DeploymentID string                 `json:"deployment_id"`
    Type         TaskType               `json:"type"`
    Payload      map[string]interface{} `json:"payload"`
    Priority     int                    `json:"priority"`
    Timeout      time.Duration          `json:"timeout"`
}

type TaskType string

const (
    TaskDeployService  TaskType = "deploy_service"
    TaskStopService    TaskType = "stop_service"
    TaskUpdateConfig   TaskType = "update_config"
    TaskSetupNetwork   TaskType = "setup_network"
    TaskApplyRules     TaskType = "apply_rules"
    TaskHealthCheck    TaskType = "health_check"
    TaskDeploySidecar  TaskType = "deploy_sidecar"
)
```

## 5. Communication Patterns

### 5.1 Engine ↔ Agent Communication

```
┌─────────────┐                           ┌─────────────┐
│   Engine    │                           │    Agent    │
└──────┬──────┘                           └──────┬──────┘
       │                                         │
       │  Agent Registration (gRPC)              │
       │◄────────────────────────────────────────│
       │                                         │
       │  Heartbeat Stream (bidirectional)       │
       │◄───────────────────────────────────────►│
       │                                         │
       │  Task Assignment (server push)          │
       │────────────────────────────────────────►│
       │                                         │
       │  Task Result (client report)            │
       │◄────────────────────────────────────────│
       │                                         │
       │  Status Updates (streaming)             │
       │◄────────────────────────────────────────│
```

### 5.2 Plugin Communication

```
Type 2 Plugins (Lifecycle - Engine Level):
┌────────────┐     gRPC      ┌──────────────────┐
│   Engine   │◄─────────────►│ Compliance Check │
└────────────┘               └──────────────────┘
                             ┌──────────────────┐
                 ◄───────────►│  Cloud Provider  │
                             └──────────────────┘

Type 1 Plugins (Service - Agent Level):
┌────────────┐    Container    ┌──────────────┐
│  Service   │◄───Network────►│  ALB Sidecar │
│ Container  │                └──────────────┘
└────────────┘                ┌──────────────┐
                              │ Metrics Agent│
                              └──────────────┘
```

## 6. Deployment Flow

### 6.1 Full Deployment Sequence

```
User                CLI                 Engine              Agent(s)
 │                   │                    │                    │
 │ banyan deploy     │                    │                    │
 │──────────────────►│                    │                    │
 │                   │ Parse compose.yaml │                    │
 │                   │ Parse banyan.yaml  │                    │
 │                   │                    │                    │
 │                   │ Deploy Request     │                    │
 │                   │───────────────────►│                    │
 │                   │                    │                    │
 │                   │                    │ [VALIDATE Hook]    │
 │                   │                    │ ───► Plugins       │
 │                   │                    │                    │
 │                   │                    │ [PLAN Hook]        │
 │                   │                    │ ───► Plugins       │
 │                   │                    │                    │
 │                   │                    │ Schedule Tasks     │
 │                   │                    │────────────────────►
 │                   │                    │                    │
 │                   │                    │ [DEPLOY Hook]      │
 │                   │                    │ ───► Plugins       │
 │                   │                    │                    │
 │                   │                    │              Setup Network
 │                   │                    │              Pull Images
 │                   │                    │              Start Containers
 │                   │                    │              Deploy Sidecars
 │                   │                    │                    │
 │                   │                    │◄────────────────────
 │                   │                    │ Task Results       │
 │                   │                    │                    │
 │                   │                    │ [VERIFY Hook]      │
 │                   │                    │ ───► Plugins       │
 │                   │                    │                    │
 │                   │◄───────────────────│                    │
 │◄──────────────────│ Deployment Complete│                    │
```

## 7. State Management

### 7.1 Distributed State (via etcd)

```
etcd Keys:
├── /banyan/agents/{agent_id}           # Agent registration
├── /banyan/agents/{agent_id}/health    # Agent health
├── /banyan/deployments/{deploy_id}     # Deployment metadata
├── /banyan/deployments/{deploy_id}/state  # Deployment state
├── /banyan/services/{service_name}     # Service definitions
├── /banyan/services/{service_name}/instances  # Running instances
└── /banyan/networks/...                # VPC state (existing)
```

### 7.2 State Store Interface (Reusing VPC Pattern)

```go
// pkg/engine/storage/interface.go

package storage

import "context"

// EngineStore extends StateStore with engine-specific operations
type EngineStore interface {
    storage.StateStore  // Reuse VPC storage interface

    // Watch for changes (for reconciliation)
    Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)

    // Transactions
    Transaction(ctx context.Context, ops []Operation) error

    // Leases (for agent TTL)
    CreateLease(ctx context.Context, ttl int64) (LeaseID, error)
    KeepAlive(ctx context.Context, leaseID LeaseID) error
}
```

## 8. Implementation Phases

### Phase 1: Core Engine & Agent (Foundation)
- [ ] Basic gRPC communication
- [ ] Agent registration and heartbeat
- [ ] Simple deployment flow (no plugins)
- [ ] Docker runtime integration
- [ ] State management with etcd

### Phase 2: VPC Integration
- [ ] Bridge agent network manager to pkg/vpc
- [ ] Container network attachment
- [ ] DNS registration on deploy
- [ ] Security rules application

### Phase 3: Plugin System
- [ ] Lifecycle plugin framework (Type 2)
- [ ] Service plugin framework (Type 1)
- [ ] Plugin discovery and registration
- [ ] Plugin SDK enhancements

### Phase 4: Advanced Features
- [ ] Multi-agent deployment scheduling
- [ ] Rollback strategies
- [ ] Health-based routing
- [ ] Log and metrics collection

## 9. Configuration

### 9.1 Engine Configuration

```yaml
# /etc/banyan/engine.yaml
engine:
  listen_addr: "0.0.0.0:7777"
  grpc:
    max_connections: 1000
    keepalive_interval: 30s

storage:
  type: "etcd"
  endpoints:
    - "localhost:2379"

plugins:
  discovery_path: "/opt/banyan/plugins"

reconciliation:
  interval: 30s

logging:
  level: "info"
  format: "json"
```

### 9.2 Agent Configuration

```yaml
# /etc/banyan/agent.yaml
agent:
  id: "${HOSTNAME}"
  labels:
    region: "us-east-1"
    type: "worker"

engine:
  address: "engine.banyan.local:7777"

runtime:
  type: "docker"
  socket: "/var/run/docker.sock"

network:
  etcd_endpoints:
    - "localhost:2379"

health:
  check_interval: 10s
  report_interval: 30s
```

## 10. Security Considerations

1. **mTLS for gRPC**: All engine-agent communication uses mutual TLS
2. **Agent authentication**: Agents authenticate with certificates or tokens
3. **Plugin isolation**: Plugins run as separate processes with limited privileges
4. **Secret management**: Integration with external secret stores (HashiCorp Vault, etc.)

## 11. Open Questions

1. **Plugin discovery**: How should lifecycle plugins be discovered and started?
   - Option A: Engine starts plugins as subprocesses
   - Option B: Plugins register with engine on startup (current design)

2. **Multi-engine HA**: How to handle engine failover?
   - Leader election via etcd
   - State is already distributed

3. **Agent updates**: How to update agents without disruption?
   - Rolling updates with drain/cordon

---

*Document Version: 1.0*
*Author: Research Assistant*
*Based on existing patterns from pkg/vpc module*
