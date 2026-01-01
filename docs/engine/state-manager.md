# State Manager Component Design

> **Implementation Status**: Phase 3 - Pending
> **Dependencies**: Agent Registry ✅

## Overview

The State Manager handles desired vs actual state tracking and reconciliation. It continuously monitors the system state and triggers corrective actions when drift is detected. The desired state comes from parsed `banyan.yml` configurations.

## Philosophy

**"Self-healing infrastructure"** - When reality drifts from desired state (container crashes, node dies), the State Manager automatically corrects it:

```yaml
# User declares desired state in banyan.yml
services:
  api:
    replicas: 3  # State Manager ensures 3 instances always run
```

If an instance crashes or a node fails, State Manager detects the drift and triggers reconciliation to restore the desired replica count.

## Responsibilities

- Store and manage desired state from banyan.yml (what should be running)
- Collect and track actual state from agents (what is running)
- Detect drift between desired and actual
- Trigger reconciliation to correct drift
- Provide state queries for other components

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     State Manager Component                      │
│                                                                  │
│  Driving Adapters                                               │
│  ┌──────────────────┐  ┌──────────────────┐                    │
│  │  Orchestrator    │  │   Reconciler     │                    │
│  │   (internal)     │  │    Timer         │                    │
│  └────────┬─────────┘  └────────┬─────────┘                    │
│           │                     │                               │
│           ▼                     ▼                               │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │              Inbound Ports                                │  │
│  │  StateService          │     ReconcilerService           │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                      Use Cases                            │  │
│  │  StateUseCase          │     ReconcilerUseCase           │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                    Domain Layer                           │  │
│  │  DesiredState | ActualState | StateDrift | ServiceState  │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  ┌──────────────────────────────▼───────────────────────────┐  │
│  │                    Outbound Ports                         │  │
│  │  StateRepository    |    AgentQuerier   |   Dispatcher   │  │
│  └──────────────────────────────┬───────────────────────────┘  │
│                                 │                               │
│  Driven Adapters               │                               │
│  ┌─────────────┐ ┌─────────────▼──┐ ┌─────────────┐           │
│  │ Etcd Store  │ │ Agent gRPC     │ │ Dispatcher  │           │
│  └─────────────┘ └────────────────┘ └─────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// pkg/engine/state/domain/entities.go

package domain

import "time"

// DesiredState represents what should be running for a deployment
type DesiredState struct {
    DeploymentID string
    Services     map[string]ServiceDesiredState
    UpdatedAt    time.Time
}

// ServiceDesiredState defines the desired state of a service
type ServiceDesiredState struct {
    Name        string
    Image       string
    Replicas    int
    Environment map[string]string
    Ports       []PortMapping
    NetworkID   string
    AgentIDs    []string // Target agents
}

// ActualState represents what is currently running
type ActualState struct {
    DeploymentID string
    Services     map[string]ServiceActualState
    CollectedAt  time.Time
}

// ServiceActualState captures the real state of a service
type ServiceActualState struct {
    Name      string
    Instances []InstanceState
}

// InstanceState represents a single running instance
type InstanceState struct {
    ContainerID string
    AgentID     string
    IP          string
    Status      ContainerStatus
    Health      HealthStatus
    StartedAt   time.Time
}

type ContainerStatus string

const (
    ContainerRunning  ContainerStatus = "running"
    ContainerStopped  ContainerStatus = "stopped"
    ContainerStarting ContainerStatus = "starting"
    ContainerFailed   ContainerStatus = "failed"
)

type HealthStatus string

const (
    HealthHealthy   HealthStatus = "healthy"
    HealthUnhealthy HealthStatus = "unhealthy"
    HealthUnknown   HealthStatus = "unknown"
)

// StateDrift captures differences between desired and actual
type StateDrift struct {
    DeploymentID string
    Drifts       []Drift
    DetectedAt   time.Time
    Severity     DriftSeverity
}

type Drift struct {
    Type        DriftType
    ServiceName string
    Details     string
    AgentID     string // Which agent is affected
}

type DriftType string

const (
    DriftMissing   DriftType = "missing"   // Service should exist but doesn't
    DriftExtra     DriftType = "extra"     // Service exists but shouldn't
    DriftReplicas  DriftType = "replicas"  // Wrong number of replicas
    DriftConfig    DriftType = "config"    // Configuration mismatch
    DriftUnhealthy DriftType = "unhealthy" // Instance not healthy
    DriftWrongHost DriftType = "wrong_host" // Running on wrong agent
)

type DriftSeverity string

const (
    SeverityCritical DriftSeverity = "critical" // Service completely down
    SeverityHigh     DriftSeverity = "high"     // Partial outage
    SeverityMedium   DriftSeverity = "medium"   // Degraded
    SeverityLow      DriftSeverity = "low"      // Minor drift
)
```

### Value Objects

```go
// pkg/engine/state/domain/value_objects.go

package domain

// PortMapping defines port exposure
type PortMapping struct {
    ContainerPort int
    HostPort      int
    Protocol      string
}

// ReconcileAction represents a corrective action
type ReconcileAction struct {
    Type       ActionType
    ServiceName string
    AgentID    string
    Details    map[string]interface{}
}

type ActionType string

const (
    ActionDeploy  ActionType = "deploy"
    ActionStop    ActionType = "stop"
    ActionRestart ActionType = "restart"
    ActionScale   ActionType = "scale"
    ActionMigrate ActionType = "migrate"
)

// DriftReport summarizes drift across all deployments
type DriftReport struct {
    TotalDeployments   int
    DeploymentsWithDrift int
    CriticalDrifts     int
    HighDrifts         int
    MediumDrifts       int
    LowDrifts          int
    GeneratedAt        time.Time
}
```

## Ports

### Inbound Ports

```go
// pkg/engine/state/ports/inbound.go

package ports

import (
    "context"
    "time"

    "github.com/fertile-org/banyan/pkg/engine/state/domain"
)

// StateService defines state management operations
type StateService interface {
    // Desired state management
    SetDesiredState(ctx context.Context, state *domain.DesiredState) error
    GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error)
    DeleteDesiredState(ctx context.Context, deploymentID string) error

    // Actual state management
    UpdateActualState(ctx context.Context, state *domain.ActualState) error
    GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error)

    // Drift detection
    DetectDrift(ctx context.Context, deploymentID string) (*domain.StateDrift, error)
    GetDriftReport(ctx context.Context) (*domain.DriftReport, error)
}

// ReconcilerService defines reconciliation operations
type ReconcilerService interface {
    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error

    // Manual triggers
    TriggerReconcile(ctx context.Context, deploymentID string) error
    TriggerReconcileAll(ctx context.Context) error

    // Configuration
    SetReconcileInterval(interval time.Duration)
    GetReconcileInterval() time.Duration

    // Status
    GetLastReconcileTime(ctx context.Context, deploymentID string) (time.Time, error)
    IsReconciling(ctx context.Context, deploymentID string) bool
}
```

### Outbound Ports

```go
// pkg/engine/state/ports/outbound.go

package ports

import (
    "context"

    "github.com/fertile-org/banyan/pkg/engine/state/domain"
)

// StateRepository defines state persistence
type StateRepository interface {
    // Desired state
    SaveDesiredState(ctx context.Context, state *domain.DesiredState) error
    GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error)
    DeleteDesiredState(ctx context.Context, deploymentID string) error
    ListDesiredStates(ctx context.Context) ([]*domain.DesiredState, error)

    // Actual state
    SaveActualState(ctx context.Context, state *domain.ActualState) error
    GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error)

    // Watch for changes
    WatchDesiredState(ctx context.Context) (<-chan StateChange, error)
    WatchActualState(ctx context.Context) (<-chan StateChange, error)
}

type StateChange struct {
    Type         ChangeType
    DeploymentID string
}

type ChangeType string

const (
    ChangeCreated ChangeType = "created"
    ChangeUpdated ChangeType = "updated"
    ChangeDeleted ChangeType = "deleted"
)

// AgentQuerier defines agent state querying
type AgentQuerier interface {
    // Query agent for running containers
    GetAgentState(ctx context.Context, agentID string) (*AgentState, error)

    // Query all agents
    ListAgentStates(ctx context.Context) ([]*AgentState, error)
}

type AgentState struct {
    AgentID    string
    Containers []ContainerInfo
    Health     string
    CollectedAt time.Time
}

type ContainerInfo struct {
    ID          string
    Name        string
    Image       string
    Status      string
    Health      string
    Labels      map[string]string
    IP          string
}

// ActionDispatcher defines remediation actions
type ActionDispatcher interface {
    Dispatch(ctx context.Context, action *domain.ReconcileAction) error
    DispatchBatch(ctx context.Context, actions []*domain.ReconcileAction) error
}
```

## Use Cases

### State Use Case

```go
// pkg/engine/state/usecases/state.go

package usecases

import (
    "context"
    "fmt"
    "time"

    "github.com/fertile-org/banyan/pkg/engine/state/domain"
    "github.com/fertile-org/banyan/pkg/engine/state/ports"
)

type StateUseCase struct {
    repo ports.StateRepository
}

func NewStateUseCase(repo ports.StateRepository) *StateUseCase {
    return &StateUseCase{repo: repo}
}

func (uc *StateUseCase) SetDesiredState(ctx context.Context, state *domain.DesiredState) error {
    state.UpdatedAt = time.Now()
    return uc.repo.SaveDesiredState(ctx, state)
}

func (uc *StateUseCase) GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error) {
    return uc.repo.GetDesiredState(ctx, deploymentID)
}

func (uc *StateUseCase) UpdateActualState(ctx context.Context, state *domain.ActualState) error {
    state.CollectedAt = time.Now()
    return uc.repo.SaveActualState(ctx, state)
}

func (uc *StateUseCase) GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error) {
    return uc.repo.GetActualState(ctx, deploymentID)
}

func (uc *StateUseCase) DetectDrift(ctx context.Context, deploymentID string) (*domain.StateDrift, error) {
    desired, err := uc.repo.GetDesiredState(ctx, deploymentID)
    if err != nil {
        return nil, fmt.Errorf("failed to get desired state: %w", err)
    }

    actual, err := uc.repo.GetActualState(ctx, deploymentID)
    if err != nil {
        return nil, fmt.Errorf("failed to get actual state: %w", err)
    }

    return uc.comparStates(desired, actual), nil
}

func (uc *StateUseCase) compareStates(desired *domain.DesiredState, actual *domain.ActualState) *domain.StateDrift {
    drift := &domain.StateDrift{
        DeploymentID: desired.DeploymentID,
        DetectedAt:   time.Now(),
    }

    // Check for missing services
    for name, desiredSvc := range desired.Services {
        actualSvc, exists := actual.Services[name]
        if !exists {
            drift.Drifts = append(drift.Drifts, domain.Drift{
                Type:        domain.DriftMissing,
                ServiceName: name,
                Details:     "service not found",
            })
            continue
        }

        // Check replica count
        actualReplicas := len(actualSvc.Instances)
        if actualReplicas != desiredSvc.Replicas {
            drift.Drifts = append(drift.Drifts, domain.Drift{
                Type:        domain.DriftReplicas,
                ServiceName: name,
                Details:     fmt.Sprintf("expected %d, got %d", desiredSvc.Replicas, actualReplicas),
            })
        }

        // Check instance health
        for _, inst := range actualSvc.Instances {
            if inst.Health == domain.HealthUnhealthy {
                drift.Drifts = append(drift.Drifts, domain.Drift{
                    Type:        domain.DriftUnhealthy,
                    ServiceName: name,
                    AgentID:     inst.AgentID,
                    Details:     fmt.Sprintf("instance %s unhealthy", inst.ContainerID),
                })
            }
        }

        // Check agent placement
        for _, inst := range actualSvc.Instances {
            if !contains(desiredSvc.AgentIDs, inst.AgentID) {
                drift.Drifts = append(drift.Drifts, domain.Drift{
                    Type:        domain.DriftWrongHost,
                    ServiceName: name,
                    AgentID:     inst.AgentID,
                    Details:     "running on non-target agent",
                })
            }
        }
    }

    // Check for extra services
    for name := range actual.Services {
        if _, exists := desired.Services[name]; !exists {
            drift.Drifts = append(drift.Drifts, domain.Drift{
                Type:        domain.DriftExtra,
                ServiceName: name,
                Details:     "service should not exist",
            })
        }
    }

    // Calculate severity
    drift.Severity = uc.calculateSeverity(drift.Drifts)

    return drift
}

func (uc *StateUseCase) calculateSeverity(drifts []domain.Drift) domain.DriftSeverity {
    if len(drifts) == 0 {
        return domain.SeverityLow
    }

    hasMissing := false
    hasUnhealthy := false

    for _, d := range drifts {
        if d.Type == domain.DriftMissing {
            hasMissing = true
        }
        if d.Type == domain.DriftUnhealthy {
            hasUnhealthy = true
        }
    }

    if hasMissing {
        return domain.SeverityCritical
    }
    if hasUnhealthy {
        return domain.SeverityHigh
    }
    return domain.SeverityMedium
}

var _ ports.StateService = (*StateUseCase)(nil)
```

### Reconciler Use Case

```go
// pkg/engine/state/usecases/reconciler.go

package usecases

import (
    "context"
    "sync"
    "time"

    "github.com/fertile-org/banyan/pkg/engine/state/domain"
    "github.com/fertile-org/banyan/pkg/engine/state/ports"
)

type ReconcilerUseCase struct {
    stateService  ports.StateService
    stateRepo     ports.StateRepository
    agentQuerier  ports.AgentQuerier
    dispatcher    ports.ActionDispatcher

    interval      time.Duration
    stopCh        chan struct{}
    wg            sync.WaitGroup
    reconciling   map[string]bool
    mu            sync.RWMutex
}

func NewReconcilerUseCase(
    stateService ports.StateService,
    stateRepo ports.StateRepository,
    agentQuerier ports.AgentQuerier,
    dispatcher ports.ActionDispatcher,
) *ReconcilerUseCase {
    return &ReconcilerUseCase{
        stateService: stateService,
        stateRepo:    stateRepo,
        agentQuerier: agentQuerier,
        dispatcher:   dispatcher,
        interval:     30 * time.Second,
        stopCh:       make(chan struct{}),
        reconciling:  make(map[string]bool),
    }
}

func (uc *ReconcilerUseCase) Start(ctx context.Context) error {
    uc.wg.Add(1)
    go uc.reconcileLoop(ctx)
    return nil
}

func (uc *ReconcilerUseCase) Stop(ctx context.Context) error {
    close(uc.stopCh)
    uc.wg.Wait()
    return nil
}

func (uc *ReconcilerUseCase) reconcileLoop(ctx context.Context) {
    defer uc.wg.Done()

    ticker := time.NewTicker(uc.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            uc.TriggerReconcileAll(ctx)
        case <-uc.stopCh:
            return
        case <-ctx.Done():
            return
        }
    }
}

func (uc *ReconcilerUseCase) TriggerReconcileAll(ctx context.Context) error {
    states, err := uc.stateRepo.ListDesiredStates(ctx)
    if err != nil {
        return err
    }

    for _, state := range states {
        go uc.TriggerReconcile(ctx, state.DeploymentID)
    }

    return nil
}

func (uc *ReconcilerUseCase) TriggerReconcile(ctx context.Context, deploymentID string) error {
    // Check if already reconciling
    uc.mu.Lock()
    if uc.reconciling[deploymentID] {
        uc.mu.Unlock()
        return nil
    }
    uc.reconciling[deploymentID] = true
    uc.mu.Unlock()

    defer func() {
        uc.mu.Lock()
        delete(uc.reconciling, deploymentID)
        uc.mu.Unlock()
    }()

    // Collect actual state from agents
    if err := uc.collectActualState(ctx, deploymentID); err != nil {
        return err
    }

    // Detect drift
    drift, err := uc.stateService.DetectDrift(ctx, deploymentID)
    if err != nil {
        return err
    }

    if len(drift.Drifts) == 0 {
        return nil // No drift
    }

    // Generate and execute remediation actions
    actions := uc.generateActions(drift)
    return uc.dispatcher.DispatchBatch(ctx, actions)
}

func (uc *ReconcilerUseCase) collectActualState(ctx context.Context, deploymentID string) error {
    desired, err := uc.stateRepo.GetDesiredState(ctx, deploymentID)
    if err != nil {
        return err
    }

    // Collect unique agent IDs
    agentSet := make(map[string]bool)
    for _, svc := range desired.Services {
        for _, agentID := range svc.AgentIDs {
            agentSet[agentID] = true
        }
    }

    // Query each agent
    actual := &domain.ActualState{
        DeploymentID: deploymentID,
        Services:     make(map[string]domain.ServiceActualState),
        CollectedAt:  time.Now(),
    }

    for agentID := range agentSet {
        agentState, err := uc.agentQuerier.GetAgentState(ctx, agentID)
        if err != nil {
            continue // Agent might be offline
        }

        // Map containers to services
        for _, container := range agentState.Containers {
            serviceName := container.Labels["banyan.service"]
            if serviceName == "" {
                continue
            }
            if container.Labels["banyan.deployment"] != deploymentID {
                continue
            }

            svcState := actual.Services[serviceName]
            svcState.Name = serviceName
            svcState.Instances = append(svcState.Instances, domain.InstanceState{
                ContainerID: container.ID,
                AgentID:     agentID,
                IP:          container.IP,
                Status:      domain.ContainerStatus(container.Status),
                Health:      domain.HealthStatus(container.Health),
            })
            actual.Services[serviceName] = svcState
        }
    }

    return uc.stateService.UpdateActualState(ctx, actual)
}

func (uc *ReconcilerUseCase) generateActions(drift *domain.StateDrift) []*domain.ReconcileAction {
    var actions []*domain.ReconcileAction

    for _, d := range drift.Drifts {
        switch d.Type {
        case domain.DriftMissing:
            actions = append(actions, &domain.ReconcileAction{
                Type:        domain.ActionDeploy,
                ServiceName: d.ServiceName,
            })

        case domain.DriftExtra:
            actions = append(actions, &domain.ReconcileAction{
                Type:        domain.ActionStop,
                ServiceName: d.ServiceName,
            })

        case domain.DriftReplicas:
            actions = append(actions, &domain.ReconcileAction{
                Type:        domain.ActionScale,
                ServiceName: d.ServiceName,
            })

        case domain.DriftUnhealthy:
            actions = append(actions, &domain.ReconcileAction{
                Type:        domain.ActionRestart,
                ServiceName: d.ServiceName,
                AgentID:     d.AgentID,
            })

        case domain.DriftWrongHost:
            actions = append(actions, &domain.ReconcileAction{
                Type:        domain.ActionMigrate,
                ServiceName: d.ServiceName,
                AgentID:     d.AgentID,
            })
        }
    }

    return actions
}

func (uc *ReconcilerUseCase) SetReconcileInterval(interval time.Duration) {
    uc.interval = interval
}

func (uc *ReconcilerUseCase) GetReconcileInterval() time.Duration {
    return uc.interval
}

func (uc *ReconcilerUseCase) IsReconciling(ctx context.Context, deploymentID string) bool {
    uc.mu.RLock()
    defer uc.mu.RUnlock()
    return uc.reconciling[deploymentID]
}

var _ ports.ReconcilerService = (*ReconcilerUseCase)(nil)
```

## Reconciliation Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                    Reconciliation Loop                            │
│                                                                   │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐          │
│  │   Timer     │───►│  Collect    │───►│  Compare    │          │
│  │  (30s)      │    │  Actual     │    │  States     │          │
│  └─────────────┘    └─────────────┘    └──────┬──────┘          │
│                                               │                  │
│                          ┌────────────────────┴──────────────┐  │
│                          │         Drift Detected?           │  │
│                          └────────────────────┬──────────────┘  │
│                                               │                  │
│                     No   │                    │ Yes              │
│                    ┌─────┴─────┐    ┌─────────▼─────────┐       │
│                    │   Done    │    │ Generate Actions  │       │
│                    └───────────┘    └─────────┬─────────┘       │
│                                               │                  │
│                                     ┌─────────▼─────────┐       │
│                                     │ Dispatch to Agents│       │
│                                     └─────────┬─────────┘       │
│                                               │                  │
│                                     ┌─────────▼─────────┐       │
│                                     │  Wait & Verify    │       │
│                                     └───────────────────┘       │
└──────────────────────────────────────────────────────────────────┘
```

## Health Check Integration

The State Manager tracks health status from banyan.yml health check definitions:

```yaml
services:
  api:
    healthcheck:
      test: curl -f http://localhost:3000/health
      interval: 30s
      timeout: 10s
      retries: 3
```

When health checks fail:
1. Agent reports unhealthy status
2. State Manager detects `DriftUnhealthy`
3. Reconciler generates `ActionRestart`
4. Agent restarts the container

## Related Components

- [Orchestrator](./orchestrator.md) - Sets desired state during deployment
- [Agent Registry](./agent-registry.md) - Provides agent information for state collection ✅
- [VPC Coordinator](./vpc-coordinator.md) - Network state tracking
