# Orchestrator Component Design

> **Implementation Status**: Phase 3 - In Progress
> **Dependencies**: Banyan Parser ✅, Agent Registry ✅, Plugin Manager ✅

## Overview

The Orchestrator manages deployment workflows through a pipeline pattern. It coordinates the entire deployment lifecycle from validation to verification, taking `banyan.yml` configuration and deploying services across the cluster.

## Philosophy

**"Docker Compose that scales"** - The Orchestrator takes a familiar banyan.yml file and handles the complexity of distributed deployment automatically:

```yaml
# User provides this simple file
services:
  api:
    image: myapp:latest
    replicas: 3
    healthcheck:
      test: curl -f http://localhost:3000/health

# Orchestrator handles:
# - Parsing and validation
# - Agent selection (which nodes to deploy to)
# - Dependency ordering (deploy db before api)
# - Network provisioning (DNS-based service discovery)
# - Health check setup
# - Rolling updates and rollbacks
```

## Responsibilities

- Parse and validate banyan.yml deployment requests
- Build execution plans based on service dependencies
- Execute deployment pipelines (Validate → Plan → Deploy → Verify)
- Coordinate with lifecycle plugins at each stage (MVP-2)
- Handle rollbacks on failure

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      Orchestrator Component                      │
│                                                                  │
│  Driving Adapters                                               │
│  ┌──────────────────┐  ┌──────────────────┐                    │
│  │   gRPC Handler   │  │   CLI Handler    │                    │
│  └────────┬─────────┘  └────────┬─────────┘                    │
│           │                     │                               │
│           ▼                     ▼                               │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              Inbound Port: OrchestratorService            │  │
│  │  CreateDeployment | ExecuteDeployment | RollbackDeployment│  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                      Use Cases                            │  │
│  │  DeployUseCase | RollbackUseCase | PlanUseCase           │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                    Domain Layer                           │  │
│  │  Deployment | Service | ExecutionPlan | DependencyGraph  │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                    Outbound Ports                         │  │
│  │  DeploymentRepository | AgentDispatcher | PluginExecutor │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  Driven Adapters               │                               │
│  ┌─────────────┐ ┌─────────────▼──┐ ┌─────────────┐           │
│  │ Etcd Repo   │ │ gRPC Dispatcher│ │ Plugin Client│           │
│  └─────────────┘ └────────────────┘ └─────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// pkg/engine/orchestrator/domain/entities.go

package domain

import "time"

// Deployment represents a deployment workflow
type Deployment struct {
    ID           string
    Name         string
    Services     []Service
    State        DeploymentState
    Plan         *ExecutionPlan
    DesiredState *DesiredState
    ActualState  *ActualState
    CreatedAt    time.Time
    UpdatedAt    time.Time
    Error        string
}

// DeploymentState represents the lifecycle state
type DeploymentState string

const (
    StateCreated     DeploymentState = "created"
    StateValidating  DeploymentState = "validating"
    StatePlanning    DeploymentState = "planning"
    StateDeploying   DeploymentState = "deploying"
    StateVerifying   DeploymentState = "verifying"
    StateActive      DeploymentState = "active"
    StateFailed      DeploymentState = "failed"
    StateRollingBack DeploymentState = "rolling_back"
    StateDestroyed   DeploymentState = "destroyed"
)

// Service represents a service to deploy
type Service struct {
    Name         string
    Image        string
    Replicas     int
    Dependencies []string
    Ports        []PortMapping
    Environment  map[string]string
    Resources    *ResourceSpec
    HealthCheck  *HealthCheckSpec
    Sidecars     []SidecarSpec
}

// ExecutionPlan defines the order and parallelism of deployment
type ExecutionPlan struct {
    DeploymentID string
    Phases       []ExecutionPhase
    CreatedAt    time.Time
}

// ExecutionPhase groups services that can deploy in parallel
type ExecutionPhase struct {
    Order    int
    Services []string
    Parallel bool
}
```

### Value Objects

```go
// pkg/engine/orchestrator/domain/value_objects.go

package domain

// DependencyGraph represents service dependencies
type DependencyGraph struct {
    Nodes map[string]*DependencyNode
}

type DependencyNode struct {
    ServiceName string
    DependsOn   []string
    DependedBy  []string
}

// NewDependencyGraph builds a dependency graph from services
func NewDependencyGraph(services []Service) *DependencyGraph {
    g := &DependencyGraph{
        Nodes: make(map[string]*DependencyNode),
    }

    // Create nodes
    for _, svc := range services {
        g.Nodes[svc.Name] = &DependencyNode{
            ServiceName: svc.Name,
            DependsOn:   svc.Dependencies,
        }
    }

    // Build reverse dependencies
    for _, node := range g.Nodes {
        for _, dep := range node.DependsOn {
            if depNode, exists := g.Nodes[dep]; exists {
                depNode.DependedBy = append(depNode.DependedBy, node.ServiceName)
            }
        }
    }

    return g
}

// HasCycle detects circular dependencies
func (g *DependencyGraph) HasCycle() bool {
    visited := make(map[string]bool)
    recStack := make(map[string]bool)

    var hasCycleFrom func(name string) bool
    hasCycleFrom = func(name string) bool {
        visited[name] = true
        recStack[name] = true

        node := g.Nodes[name]
        for _, dep := range node.DependsOn {
            if !visited[dep] {
                if hasCycleFrom(dep) {
                    return true
                }
            } else if recStack[dep] {
                return true
            }
        }

        recStack[name] = false
        return false
    }

    for name := range g.Nodes {
        if !visited[name] {
            if hasCycleFrom(name) {
                return true
            }
        }
    }

    return false
}

// TopologicalSort returns services in dependency order
func (g *DependencyGraph) TopologicalSort() ([][]string, error) {
    if g.HasCycle() {
        return nil, fmt.Errorf("circular dependency detected")
    }

    var phases [][]string
    remaining := make(map[string]bool)
    for name := range g.Nodes {
        remaining[name] = true
    }

    for len(remaining) > 0 {
        var phase []string

        for name := range remaining {
            node := g.Nodes[name]
            canDeploy := true

            for _, dep := range node.DependsOn {
                if remaining[dep] {
                    canDeploy = false
                    break
                }
            }

            if canDeploy {
                phase = append(phase, name)
            }
        }

        for _, name := range phase {
            delete(remaining, name)
        }

        phases = append(phases, phase)
    }

    return phases, nil
}

// DesiredState represents what should be running
type DesiredState struct {
    Services map[string]ServiceDesiredState
}

type ServiceDesiredState struct {
    Image       string
    Replicas    int
    Environment map[string]string
    Ports       []PortMapping
    AgentIDs    []string // Which agents should run this service
}

// PortMapping defines port exposure
type PortMapping struct {
    ContainerPort int
    HostPort      int
    Protocol      string
}

// ResourceSpec defines resource limits
type ResourceSpec struct {
    CPUShares int64
    MemoryMB  int64
}

// HealthCheckSpec defines health check configuration
type HealthCheckSpec struct {
    Type     string // http, tcp, exec
    Endpoint string
    Interval time.Duration
    Timeout  time.Duration
    Retries  int
}

// SidecarSpec defines a sidecar plugin
type SidecarSpec struct {
    Name       string
    Image      string
    Parameters map[string]interface{}
}
```

## Ports

### Inbound Port

```go
// pkg/engine/orchestrator/ports/inbound.go

package ports

import (
    "context"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
)

// OrchestratorService defines what the orchestrator offers
type OrchestratorService interface {
    // CreateDeployment initializes a new deployment
    CreateDeployment(ctx context.Context, req CreateDeploymentRequest) (*domain.Deployment, error)

    // ExecuteDeployment runs the full deployment pipeline
    ExecuteDeployment(ctx context.Context, deploymentID string) error

    // GetDeploymentPlan returns execution plan without executing
    GetDeploymentPlan(ctx context.Context, req CreateDeploymentRequest) (*domain.ExecutionPlan, error)

    // RollbackDeployment reverts to previous state
    RollbackDeployment(ctx context.Context, deploymentID string, strategy RollbackStrategy) error

    // CancelDeployment stops an in-progress deployment
    CancelDeployment(ctx context.Context, deploymentID string) error

    // GetDeployment retrieves deployment by ID
    GetDeployment(ctx context.Context, deploymentID string) (*domain.Deployment, error)

    // ListDeployments lists all deployments
    ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*domain.Deployment, error)
}

// CreateDeploymentRequest contains deployment parameters
type CreateDeploymentRequest struct {
    Name        string
    BanyanFile  string            // banyan.yml content
    Targets     []string          // Agent selectors (optional, auto-select if empty)
    Variables   map[string]string // Variable substitution (e.g., ${DB_PASSWORD})
}

// RollbackStrategy defines how to rollback
type RollbackStrategy string

const (
    RollbackImmediate RollbackStrategy = "immediate" // Stop all, redeploy previous
    RollbackGraceful  RollbackStrategy = "graceful"  // Rolling rollback
)

// DeploymentFilter for listing deployments
type DeploymentFilter struct {
    State  *domain.DeploymentState
    Name   *string
    Limit  int
    Offset int
}
```

### Outbound Ports

```go
// pkg/engine/orchestrator/ports/outbound.go

package ports

import (
    "context"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
)

// DeploymentRepository defines persistence operations
type DeploymentRepository interface {
    Save(ctx context.Context, deployment *domain.Deployment) error
    Get(ctx context.Context, id string) (*domain.Deployment, error)
    Update(ctx context.Context, deployment *domain.Deployment) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter DeploymentFilter) ([]*domain.Deployment, error)
}

// AgentDispatcher defines agent communication
type AgentDispatcher interface {
    // DispatchTask sends a task to a specific agent
    DispatchTask(ctx context.Context, agentID string, task *Task) (*TaskResult, error)

    // DispatchBatch sends tasks to multiple agents in parallel
    DispatchBatch(ctx context.Context, tasks map[string]*Task) (map[string]*TaskResult, error)

    // GetAgentStatus retrieves current status of an agent
    GetAgentStatus(ctx context.Context, agentID string) (*AgentStatus, error)
}

// Task represents work to send to an agent
type Task struct {
    ID           string
    DeploymentID string
    Type         TaskType
    Payload      map[string]interface{}
}

type TaskType string

const (
    TaskDeployService TaskType = "deploy_service"
    TaskStopService   TaskType = "stop_service"
    TaskSetupNetwork  TaskType = "setup_network"
    TaskApplyRules    TaskType = "apply_rules"
)

type TaskResult struct {
    TaskID string
    Status string
    Output map[string]interface{}
    Error  string
}

// PluginExecutor defines lifecycle plugin execution
type PluginExecutor interface {
    ExecuteHook(ctx context.Context, hook LifecycleHook, data *HookData) (*HookResult, error)
}

type LifecycleHook string

const (
    HookValidate LifecycleHook = "validate"
    HookPlan     LifecycleHook = "plan"
    HookDeploy   LifecycleHook = "deploy"
    HookVerify   LifecycleHook = "verify"
    HookDestroy  LifecycleHook = "destroy"
)

type HookData struct {
    DeploymentID string
    Hook         LifecycleHook
    Services     []domain.Service
    Context      map[string]interface{}
}

type HookResult struct {
    Continue bool
    Message  string
    Modified map[string]interface{}
    Errors   []string
}

// Scheduler defines deployment scheduling
type Scheduler interface {
    Schedule(ctx context.Context, services []domain.Service, deps *domain.DependencyGraph) (*domain.ExecutionPlan, error)
    ValidateDependencies(ctx context.Context, deps *domain.DependencyGraph) error
}

// BanyanParser defines banyan.yml parsing
type BanyanParser interface {
    Parse(banyanContent string) ([]domain.Service, error)
}
```

## Use Cases

```go
// pkg/engine/orchestrator/usecases/deploy.go

package usecases

import (
    "context"
    "fmt"
    "time"

    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports"
    "github.com/google/uuid"
)

// DeployUseCase implements deployment workflow
type DeployUseCase struct {
    repo       ports.DeploymentRepository
    dispatcher ports.AgentDispatcher
    scheduler  ports.Scheduler
    plugins    ports.PluginExecutor
    parser     ports.BanyanParser
}

func NewDeployUseCase(
    repo ports.DeploymentRepository,
    dispatcher ports.AgentDispatcher,
    scheduler ports.Scheduler,
    plugins ports.PluginExecutor,
    parser ports.BanyanParser,
) *DeployUseCase {
    return &DeployUseCase{
        repo:       repo,
        dispatcher: dispatcher,
        scheduler:  scheduler,
        plugins:    plugins,
        parser:     parser,
    }
}

func (uc *DeployUseCase) CreateDeployment(ctx context.Context, req ports.CreateDeploymentRequest) (*domain.Deployment, error) {
    // Parse banyan.yml file
    services, err := uc.parser.Parse(req.BanyanFile)
    if err != nil {
        return nil, fmt.Errorf("failed to parse banyan.yml: %w", err)
    }

    deployment := &domain.Deployment{
        ID:        uuid.New().String(),
        Name:      req.Name,
        Services:  services,
        State:     domain.StateCreated,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    if err := uc.repo.Save(ctx, deployment); err != nil {
        return nil, fmt.Errorf("failed to save deployment: %w", err)
    }

    return deployment, nil
}

func (uc *DeployUseCase) ExecuteDeployment(ctx context.Context, deploymentID string) error {
    deployment, err := uc.repo.Get(ctx, deploymentID)
    if err != nil {
        return fmt.Errorf("deployment not found: %w", err)
    }

    // Define pipeline stages
    stages := []struct {
        name   string
        state  domain.DeploymentState
        hook   ports.LifecycleHook
        action func(context.Context, *domain.Deployment) error
    }{
        {"validate", domain.StateValidating, ports.HookValidate, uc.validate},
        {"plan", domain.StatePlanning, ports.HookPlan, uc.plan},
        {"deploy", domain.StateDeploying, ports.HookDeploy, uc.deploy},
        {"verify", domain.StateVerifying, ports.HookVerify, uc.verify},
    }

    for _, stage := range stages {
        // Update state
        deployment.State = stage.state
        deployment.UpdatedAt = time.Now()
        if err := uc.repo.Update(ctx, deployment); err != nil {
            return err
        }

        // Execute lifecycle hook (before)
        hookData := &ports.HookData{
            DeploymentID: deployment.ID,
            Hook:         stage.hook,
            Services:     deployment.Services,
            Context:      make(map[string]interface{}),
        }

        result, err := uc.plugins.ExecuteHook(ctx, stage.hook, hookData)
        if err != nil {
            return uc.fail(ctx, deployment, stage.name, err)
        }
        if !result.Continue {
            return uc.fail(ctx, deployment, stage.name, fmt.Errorf("hook rejected: %s", result.Message))
        }

        // Execute stage action
        if err := stage.action(ctx, deployment); err != nil {
            return uc.fail(ctx, deployment, stage.name, err)
        }
    }

    // Success
    deployment.State = domain.StateActive
    deployment.UpdatedAt = time.Now()
    return uc.repo.Update(ctx, deployment)
}

func (uc *DeployUseCase) validate(ctx context.Context, d *domain.Deployment) error {
    // Build and validate dependency graph
    deps := domain.NewDependencyGraph(d.Services)
    return uc.scheduler.ValidateDependencies(ctx, deps)
}

func (uc *DeployUseCase) plan(ctx context.Context, d *domain.Deployment) error {
    deps := domain.NewDependencyGraph(d.Services)

    plan, err := uc.scheduler.Schedule(ctx, d.Services, deps)
    if err != nil {
        return err
    }

    d.Plan = plan
    return nil
}

func (uc *DeployUseCase) deploy(ctx context.Context, d *domain.Deployment) error {
    if d.Plan == nil {
        return fmt.Errorf("no execution plan")
    }

    // Execute phases in order
    for _, phase := range d.Plan.Phases {
        if err := uc.executePhase(ctx, d, phase); err != nil {
            return err
        }
    }

    return nil
}

func (uc *DeployUseCase) executePhase(ctx context.Context, d *domain.Deployment, phase domain.ExecutionPhase) error {
    if phase.Parallel {
        return uc.executeParallel(ctx, d, phase.Services)
    }
    return uc.executeSequential(ctx, d, phase.Services)
}

func (uc *DeployUseCase) executeParallel(ctx context.Context, d *domain.Deployment, serviceNames []string) error {
    tasks := make(map[string]*ports.Task)

    for _, serviceName := range serviceNames {
        service := uc.findService(d.Services, serviceName)
        if service == nil {
            continue
        }

        // TODO: Get agent assignment from scheduler
        agentID := "agent-1" // Placeholder

        tasks[agentID] = &ports.Task{
            ID:           uuid.New().String(),
            DeploymentID: d.ID,
            Type:         ports.TaskDeployService,
            Payload: map[string]interface{}{
                "service": service,
            },
        }
    }

    results, err := uc.dispatcher.DispatchBatch(ctx, tasks)
    if err != nil {
        return err
    }

    // Check results
    for agentID, result := range results {
        if result.Error != "" {
            return fmt.Errorf("agent %s failed: %s", agentID, result.Error)
        }
    }

    return nil
}

func (uc *DeployUseCase) executeSequential(ctx context.Context, d *domain.Deployment, serviceNames []string) error {
    for _, serviceName := range serviceNames {
        service := uc.findService(d.Services, serviceName)
        if service == nil {
            continue
        }

        agentID := "agent-1" // Placeholder

        task := &ports.Task{
            ID:           uuid.New().String(),
            DeploymentID: d.ID,
            Type:         ports.TaskDeployService,
            Payload: map[string]interface{}{
                "service": service,
            },
        }

        result, err := uc.dispatcher.DispatchTask(ctx, agentID, task)
        if err != nil {
            return err
        }
        if result.Error != "" {
            return fmt.Errorf("deployment of %s failed: %s", serviceName, result.Error)
        }
    }

    return nil
}

func (uc *DeployUseCase) verify(ctx context.Context, d *domain.Deployment) error {
    // Check all services are healthy
    for _, service := range d.Services {
        // TODO: Query agent for service health
        _ = service
    }
    return nil
}

func (uc *DeployUseCase) fail(ctx context.Context, d *domain.Deployment, stage string, err error) error {
    d.State = domain.StateFailed
    d.Error = fmt.Sprintf("%s failed: %v", stage, err)
    d.UpdatedAt = time.Now()
    uc.repo.Update(ctx, d)
    return err
}

func (uc *DeployUseCase) findService(services []domain.Service, name string) *domain.Service {
    for i := range services {
        if services[i].Name == name {
            return &services[i]
        }
    }
    return nil
}

func (uc *DeployUseCase) GetDeployment(ctx context.Context, deploymentID string) (*domain.Deployment, error) {
    return uc.repo.Get(ctx, deploymentID)
}

func (uc *DeployUseCase) ListDeployments(ctx context.Context, filter ports.DeploymentFilter) ([]*domain.Deployment, error) {
    return uc.repo.List(ctx, filter)
}

var _ ports.OrchestratorService = (*DeployUseCase)(nil)
```

## Adapters

### gRPC Handler (Driving Adapter)

```go
// pkg/engine/orchestrator/adapters/grpc_handler.go

package adapters

import (
    "context"

    pb "github.com/fertile-org/banyan/pkg/engine/api/proto"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports"
)

// GRPCHandler handles gRPC requests for deployment operations
type GRPCHandler struct {
    pb.UnimplementedEngineServiceServer
    orchestrator ports.OrchestratorService
}

func NewGRPCHandler(orchestrator ports.OrchestratorService) *GRPCHandler {
    return &GRPCHandler{orchestrator: orchestrator}
}

func (h *GRPCHandler) Deploy(ctx context.Context, req *pb.DeployRequest) (*pb.DeployResponse, error) {
    createReq := ports.CreateDeploymentRequest{
        Name:        req.DeploymentId,
        BanyanFile:  req.BanyanConfig,
        Targets:     req.Targets,
        Variables:   req.Variables,
    }

    deployment, err := h.orchestrator.CreateDeployment(ctx, createReq)
    if err != nil {
        return &pb.DeployResponse{
            Success: false,
            Error:   err.Error(),
        }, nil
    }

    // Execute async
    go h.orchestrator.ExecuteDeployment(context.Background(), deployment.ID)

    return &pb.DeployResponse{
        Success:      true,
        DeploymentId: deployment.ID,
        Status:       string(deployment.State),
    }, nil
}

func (h *GRPCHandler) GetDeploymentStatus(ctx context.Context, req *pb.GetStatusRequest) (*pb.DeploymentStatus, error) {
    deployment, err := h.orchestrator.GetDeployment(ctx, req.DeploymentId)
    if err != nil {
        return nil, err
    }

    return &pb.DeploymentStatus{
        DeploymentId: deployment.ID,
        State:        string(deployment.State),
        Error:        deployment.Error,
    }, nil
}
```

### Etcd Repository (Driven Adapter)

```go
// pkg/engine/orchestrator/adapters/etcd_repository.go

package adapters

import (
    "context"
    "encoding/json"
    "fmt"

    clientv3 "go.etcd.io/etcd/client/v3"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
    "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports"
)

// EtcdDeploymentRepository stores deployments in etcd
type EtcdDeploymentRepository struct {
    client *clientv3.Client
    prefix string
}

func NewEtcdDeploymentRepository(client *clientv3.Client) *EtcdDeploymentRepository {
    return &EtcdDeploymentRepository{
        client: client,
        prefix: "/banyan/deployments/",
    }
}

func (r *EtcdDeploymentRepository) Save(ctx context.Context, d *domain.Deployment) error {
    data, err := json.Marshal(d)
    if err != nil {
        return fmt.Errorf("failed to marshal deployment: %w", err)
    }

    key := r.prefix + d.ID
    _, err = r.client.Put(ctx, key, string(data))
    return err
}

func (r *EtcdDeploymentRepository) Get(ctx context.Context, id string) (*domain.Deployment, error) {
    key := r.prefix + id
    resp, err := r.client.Get(ctx, key)
    if err != nil {
        return nil, err
    }
    if len(resp.Kvs) == 0 {
        return nil, fmt.Errorf("deployment not found: %s", id)
    }

    var d domain.Deployment
    if err := json.Unmarshal(resp.Kvs[0].Value, &d); err != nil {
        return nil, fmt.Errorf("failed to unmarshal deployment: %w", err)
    }
    return &d, nil
}

func (r *EtcdDeploymentRepository) Update(ctx context.Context, d *domain.Deployment) error {
    return r.Save(ctx, d)
}

func (r *EtcdDeploymentRepository) Delete(ctx context.Context, id string) error {
    key := r.prefix + id
    _, err := r.client.Delete(ctx, key)
    return err
}

func (r *EtcdDeploymentRepository) List(ctx context.Context, filter ports.DeploymentFilter) ([]*domain.Deployment, error) {
    resp, err := r.client.Get(ctx, r.prefix, clientv3.WithPrefix())
    if err != nil {
        return nil, err
    }

    var deployments []*domain.Deployment
    for _, kv := range resp.Kvs {
        var d domain.Deployment
        if err := json.Unmarshal(kv.Value, &d); err != nil {
            continue
        }

        // Apply filters
        if filter.State != nil && d.State != *filter.State {
            continue
        }
        if filter.Name != nil && d.Name != *filter.Name {
            continue
        }

        deployments = append(deployments, &d)
    }

    // Apply pagination
    if filter.Offset > 0 && filter.Offset < len(deployments) {
        deployments = deployments[filter.Offset:]
    }
    if filter.Limit > 0 && filter.Limit < len(deployments) {
        deployments = deployments[:filter.Limit]
    }

    return deployments, nil
}

var _ ports.DeploymentRepository = (*EtcdDeploymentRepository)(nil)
```

## Pipeline Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                    Deployment Pipeline                            │
│                                                                   │
│  ┌───────────┐    ┌───────────┐    ┌───────────┐    ┌──────────┐│
│  │ VALIDATE  │───►│   PLAN    │───►│  DEPLOY   │───►│  VERIFY  ││
│  └─────┬─────┘    └─────┬─────┘    └─────┬─────┘    └────┬─────┘│
│        │                │                │                │      │
│   ┌────▼────┐      ┌────▼────┐      ┌────▼────┐     ┌────▼────┐ │
│   │ Plugin  │      │ Plugin  │      │ Plugin  │     │ Plugin  │ │
│   │  Hook   │      │  Hook   │      │  Hook   │     │  Hook   │ │
│   └─────────┘      └─────────┘      └─────────┘     └─────────┘ │
│                                                                   │
│  On Failure:                                                      │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │                     ROLLBACK                               │   │
│  │  Stop new services → Restore previous → Verify rollback   │   │
│  └───────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

## Networking

Services in a banyan.yml deployment can reach each other by name through implicit DNS-based service discovery:

```yaml
services:
  api:
    image: myapi:latest
    environment:
      - DATABASE_URL=postgres://db:5432/app  # "db" resolves via DNS
  db:
    image: postgres:15
```

The Orchestrator coordinates with VPC Coordinator to:
1. Allocate container IPs from the VPC subnet
2. Register service names in DNS
3. Apply security policies (MVP-2 plugin)

## Plugin Integration (MVP-2)

Per-service plugins are defined in banyan.yml and executed by the Plugin Manager:

```yaml
services:
  api:
    image: myapi:latest
    replicas: 3
    plugins:
      - name: load_balancer
        config:
          port: 443
          ssl:
            auto: true
```

The Orchestrator calls plugin hooks at each pipeline stage:
- `validate` - Before deployment starts
- `plan` - After execution plan is created
- `deploy` - During container deployment
- `verify` - After deployment completes

## Related Components

- [State Manager](./state-manager.md) - Tracks desired vs actual state
- [Agent Registry](./agent-registry.md) - Agent selection for deployment ✅
- [Plugin Manager](./plugin-manager.md) - Lifecycle hook execution ✅
- [Banyan Parser](./banyan-parser.md) - Parse banyan.yml configuration ✅
- [VPC Coordinator](./vpc-coordinator.md) - Network provisioning
