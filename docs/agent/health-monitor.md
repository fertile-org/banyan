# Health Monitor - Detailed Design

## Overview

The Health Monitor is the Agent component responsible for continuously monitoring the health of containers, the node, and other agent components. It collects health metrics, performs health checks, and reports status to the Engine, enabling proactive issue detection and automated remediation.

## Responsibilities

1. **Container Health Checks** - Execute container health probes (liveness, readiness)
2. **Node Health Monitoring** - Monitor node resources (CPU, memory, disk, network)
3. **Component Health** - Monitor other agent components' health
4. **Metrics Collection** - Collect and aggregate health metrics
5. **Alert Generation** - Generate alerts for health issues
6. **Health Reporting** - Report health status to Engine

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                            Health Monitor                                 │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                       Driving Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Ticker/Cron     │  │   gRPC Handler   │  │  Event Subscriber │ │ │
│  │  │  (scheduled)     │  │  (on-demand)     │  │  (container events)│ │ │
│  │  └────────┬─────────┘  └────────┬─────────┘  └─────────┬─────────┘ │ │
│  └───────────┼─────────────────────┼─────────────────────┼───────────┘ │
│              │                     │                     │              │
│              └─────────────────────┴─────────────────────┘              │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Inbound Ports                                 │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                  HealthMonitorService                        │   │ │
│  │  │  - GetNodeHealth() -> NodeHealth                            │   │ │
│  │  │  - GetContainerHealth(id) -> ContainerHealth                │   │ │
│  │  │  - CheckContainer(id) -> HealthCheckResult                  │   │ │
│  │  │  - ListUnhealthy() -> []UnhealthyResource                   │   │ │
│  │  │  - SubscribeHealthEvents() -> EventStream                   │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                   MetricsService                             │   │ │
│  │  │  - GetNodeMetrics() -> NodeMetrics                          │   │ │
│  │  │  - GetContainerMetrics(id) -> ContainerMetrics              │   │ │
│  │  │  - GetHistoricalMetrics(query) -> []MetricPoint             │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                   AlertService                               │   │ │
│  │  │  - GetActiveAlerts() -> []Alert                             │   │ │
│  │  │  - AcknowledgeAlert(id) -> error                            │   │ │
│  │  │  - ConfigureAlertRules(rules) -> error                      │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                          Use Cases                                   │ │
│  │  ┌─────────────────┐ ┌─────────────────┐ ┌───────────────────────┐ │ │
│  │  │ HealthCheck     │ │ MetricsCollect  │ │  AlertManagement      │ │ │
│  │  │   UseCase       │ │   UseCase       │ │     UseCase           │ │ │
│  │  │ - Container     │ │ - Node          │ │ - Evaluate            │ │ │
│  │  │ - Node          │ │ - Container     │ │ - Generate            │ │ │
│  │  │ - Component     │ │ - Aggregate     │ │ - Notify              │ │ │
│  │  └─────────────────┘ └─────────────────┘ └───────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                         Domain Layer                                 │ │
│  │  ┌────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Entities: HealthStatus, NodeHealth, ContainerHealth, Alert   │ │ │
│  │  │  Value Objects: HealthState, ProbeType, AlertSeverity         │ │ │
│  │  │  Domain Logic: Health evaluation, Alert thresholds            │ │ │
│  │  └────────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Outbound Ports                                │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │ ContainerProber   │ │  NodeMetrics      │ │ HealthReporter    │ │ │
│  │  │ (health checks)   │ │  Collector        │ │ (to Engine)       │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │  MetricsStore     │ │  AlertNotifier    │ │  EventPublisher   │ │ │
│  │  │  (time-series)    │ │  (notifications)  │ │  (events)         │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Driven Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Docker Health   │  │  /proc & /sys    │  │   gRPC Reporter   │ │ │
│  │  │  Check Executor  │  │  Metrics Reader  │  │   (to Engine)     │ │ │
│  │  └──────────────────┘  └──────────────────┘  └───────────────────┘ │ │
│  │  ┌──────────────────┐  ┌──────────────────┐                        │ │
│  │  │   HTTP Prober    │  │  In-memory       │                        │ │
│  │  │   (HTTP checks)  │  │  Metrics Store   │                        │ │
│  │  └──────────────────┘  └──────────────────┘                        │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// NodeHealth represents the health status of the node
type NodeHealth struct {
    NodeID        string
    State         HealthState
    LastCheck     time.Time
    CPU           ResourceHealth
    Memory        ResourceHealth
    Disk          ResourceHealth
    Network       NetworkHealth
    Components    map[string]ComponentHealth
    Issues        []HealthIssue
}

// ContainerHealth represents the health of a container
type ContainerHealth struct {
    ContainerID   string
    ServiceID     string
    State         HealthState
    LastCheck     time.Time
    Liveness      ProbeResult
    Readiness     ProbeResult
    StartupProbe  *ProbeResult
    Resources     ContainerResourceHealth
    RestartCount  int
    Issues        []HealthIssue
}

// Alert represents a health alert
type Alert struct {
    ID          AlertID
    Severity    AlertSeverity
    Source      AlertSource
    ResourceID  string
    Message     string
    Details     map[string]interface{}
    CreatedAt   time.Time
    UpdatedAt   time.Time
    AcknowledgedAt *time.Time
    ResolvedAt  *time.Time
    State       AlertState
}

// ProbeResult contains the result of a health probe
type ProbeResult struct {
    Type        ProbeType
    Success     bool
    Message     string
    Duration    time.Duration
    LastSuccess *time.Time
    LastFailure *time.Time
    ConsecutiveFailures int
    ConsecutiveSuccesses int
}

// ResourceHealth represents health of a resource
type ResourceHealth struct {
    State       HealthState
    Current     float64
    Limit       float64
    Percentage  float64
    Threshold   float64
}

// ComponentHealth represents health of an agent component
type ComponentHealth struct {
    Name        string
    State       HealthState
    Message     string
    LastCheck   time.Time
    Metrics     map[string]float64
}
```

### Value Objects

```go
// HealthState represents overall health state
type HealthState string

const (
    HealthStateHealthy   HealthState = "healthy"
    HealthStateDegraded  HealthState = "degraded"
    HealthStateUnhealthy HealthState = "unhealthy"
    HealthStateUnknown   HealthState = "unknown"
)

// ProbeType defines the type of health probe
type ProbeType string

const (
    ProbeTypeLiveness  ProbeType = "liveness"
    ProbeTypeReadiness ProbeType = "readiness"
    ProbeTypeStartup   ProbeType = "startup"
)

// AlertSeverity defines alert severity levels
type AlertSeverity string

const (
    AlertSeverityCritical AlertSeverity = "critical"
    AlertSeverityWarning  AlertSeverity = "warning"
    AlertSeverityInfo     AlertSeverity = "info"
)

// AlertState represents the state of an alert
type AlertState string

const (
    AlertStateFiring       AlertState = "firing"
    AlertStateAcknowledged AlertState = "acknowledged"
    AlertStateResolved     AlertState = "resolved"
)

// AlertSource identifies the source of an alert
type AlertSource string

const (
    AlertSourceNode      AlertSource = "node"
    AlertSourceContainer AlertSource = "container"
    AlertSourceComponent AlertSource = "component"
)

// HealthIssue represents a specific health issue
type HealthIssue struct {
    Code        string
    Severity    AlertSeverity
    Message     string
    Suggestion  string
    DetectedAt  time.Time
}

// HealthCheckConfig defines how to perform health checks
type HealthCheckConfig struct {
    Type        ProbeType
    HTTP        *HTTPProbeConfig
    TCP         *TCPProbeConfig
    Exec        *ExecProbeConfig
    Interval    time.Duration
    Timeout     time.Duration
    SuccessThreshold int
    FailureThreshold int
    InitialDelay time.Duration
}

// HTTPProbeConfig for HTTP health checks
type HTTPProbeConfig struct {
    Path        string
    Port        int
    Scheme      string
    Headers     map[string]string
    ExpectedStatus []int
}

// TCPProbeConfig for TCP health checks
type TCPProbeConfig struct {
    Port int
}

// ExecProbeConfig for exec health checks
type ExecProbeConfig struct {
    Command []string
}
```

### Domain Logic

```go
// Evaluate overall health state from components
func (n *NodeHealth) EvaluateState() {
    // Check resource health
    unhealthyCount := 0
    degradedCount := 0

    resources := []ResourceHealth{n.CPU, n.Memory, n.Disk}
    for _, r := range resources {
        switch r.State {
        case HealthStateUnhealthy:
            unhealthyCount++
        case HealthStateDegraded:
            degradedCount++
        }
    }

    // Check component health
    for _, comp := range n.Components {
        switch comp.State {
        case HealthStateUnhealthy:
            unhealthyCount++
        case HealthStateDegraded:
            degradedCount++
        }
    }

    // Determine overall state
    if unhealthyCount > 0 {
        n.State = HealthStateUnhealthy
    } else if degradedCount > 0 {
        n.State = HealthStateDegraded
    } else {
        n.State = HealthStateHealthy
    }
}

// Evaluate container health from probes
func (c *ContainerHealth) EvaluateState() {
    // Check liveness first
    if !c.Liveness.Success && c.Liveness.ConsecutiveFailures >= 3 {
        c.State = HealthStateUnhealthy
        return
    }

    // Check readiness
    if !c.Readiness.Success && c.Readiness.ConsecutiveFailures >= 3 {
        c.State = HealthStateDegraded
        return
    }

    // Check resource usage
    if c.Resources.CPUPercent > 90 || c.Resources.MemoryPercent > 90 {
        c.State = HealthStateDegraded
        return
    }

    // Check restart count
    if c.RestartCount > 5 {
        c.State = HealthStateDegraded
        c.Issues = append(c.Issues, HealthIssue{
            Code:       "HIGH_RESTART_COUNT",
            Severity:   AlertSeverityWarning,
            Message:    fmt.Sprintf("Container has restarted %d times", c.RestartCount),
            Suggestion: "Check container logs for crash reasons",
            DetectedAt: time.Now(),
        })
        return
    }

    c.State = HealthStateHealthy
}

// Check if resource exceeds threshold
func (r *ResourceHealth) Evaluate(thresholds Thresholds) {
    r.Percentage = (r.Current / r.Limit) * 100

    if r.Percentage >= thresholds.Critical {
        r.State = HealthStateUnhealthy
    } else if r.Percentage >= thresholds.Warning {
        r.State = HealthStateDegraded
    } else {
        r.State = HealthStateHealthy
    }
}

// Update probe result based on check
func (p *ProbeResult) Update(success bool, message string, duration time.Duration) {
    now := time.Now()
    p.Success = success
    p.Message = message
    p.Duration = duration

    if success {
        p.ConsecutiveSuccesses++
        p.ConsecutiveFailures = 0
        p.LastSuccess = &now
    } else {
        p.ConsecutiveFailures++
        p.ConsecutiveSuccesses = 0
        p.LastFailure = &now
    }
}
```

## Inbound Ports

### HealthMonitorService

```go
// HealthMonitorService is the main interface for health operations
type HealthMonitorService interface {
    // Node health
    GetNodeHealth(ctx context.Context) (*NodeHealth, error)
    CheckNodeHealth(ctx context.Context) (*NodeHealth, error)

    // Container health
    GetContainerHealth(ctx context.Context, containerID string) (*ContainerHealth, error)
    CheckContainer(ctx context.Context, containerID string) (*HealthCheckResult, error)
    ListContainerHealth(ctx context.Context) ([]*ContainerHealth, error)

    // Unhealthy resources
    ListUnhealthy(ctx context.Context) ([]*UnhealthyResource, error)

    // Events
    SubscribeHealthEvents(ctx context.Context) (<-chan HealthEvent, error)
}

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
    ResourceID   string
    ResourceType string
    State        HealthState
    Probes       map[ProbeType]ProbeResult
    Issues       []HealthIssue
    CheckedAt    time.Time
}

// UnhealthyResource represents a resource with health issues
type UnhealthyResource struct {
    ID           string
    Type         string
    State        HealthState
    Since        time.Time
    Issues       []HealthIssue
    Suggestions  []string
}

// HealthEvent represents a health status change
type HealthEvent struct {
    ResourceID   string
    ResourceType string
    EventType    HealthEventType
    PreviousState HealthState
    CurrentState  HealthState
    Timestamp    time.Time
    Details      map[string]interface{}
}

type HealthEventType string

const (
    HealthEventStateChanged  HealthEventType = "state_changed"
    HealthEventProbeSucceeded HealthEventType = "probe_succeeded"
    HealthEventProbeFailed   HealthEventType = "probe_failed"
    HealthEventAlertRaised   HealthEventType = "alert_raised"
    HealthEventAlertResolved HealthEventType = "alert_resolved"
)
```

### MetricsService

```go
// MetricsService provides access to health metrics
type MetricsService interface {
    // Node metrics
    GetNodeMetrics(ctx context.Context) (*NodeMetrics, error)
    GetNodeMetricsHistory(ctx context.Context, duration time.Duration) ([]*NodeMetrics, error)

    // Container metrics
    GetContainerMetrics(ctx context.Context, containerID string) (*ContainerMetrics, error)
    GetContainerMetricsHistory(ctx context.Context, containerID string, duration time.Duration) ([]*ContainerMetrics, error)

    // Aggregated metrics
    GetServiceMetrics(ctx context.Context, serviceID string) (*ServiceMetrics, error)
}

// NodeMetrics contains node resource metrics
type NodeMetrics struct {
    Timestamp    time.Time
    CPU          CPUMetrics
    Memory       MemoryMetrics
    Disk         DiskMetrics
    Network      NetworkMetrics
    Containers   int
    RunningContainers int
}

// CPUMetrics contains CPU usage metrics
type CPUMetrics struct {
    UsagePercent   float64
    LoadAverage    [3]float64 // 1, 5, 15 minutes
    Cores          int
    UserPercent    float64
    SystemPercent  float64
    IOWaitPercent  float64
}

// MemoryMetrics contains memory usage metrics
type MemoryMetrics struct {
    Total         uint64
    Used          uint64
    Free          uint64
    Available     uint64
    UsagePercent  float64
    SwapTotal     uint64
    SwapUsed      uint64
    SwapPercent   float64
}

// ContainerMetrics contains container resource metrics
type ContainerMetrics struct {
    ContainerID  string
    Timestamp    time.Time
    CPUPercent   float64
    MemoryUsage  uint64
    MemoryLimit  uint64
    MemoryPercent float64
    NetworkRx    uint64
    NetworkTx    uint64
    BlockRead    uint64
    BlockWrite   uint64
    PIDs         int
}
```

### AlertService

```go
// AlertService manages health alerts
type AlertService interface {
    // Alert queries
    GetActiveAlerts(ctx context.Context) ([]*Alert, error)
    GetAlert(ctx context.Context, alertID AlertID) (*Alert, error)
    ListAlerts(ctx context.Context, filter AlertFilter) ([]*Alert, error)

    // Alert management
    AcknowledgeAlert(ctx context.Context, alertID AlertID) error
    ResolveAlert(ctx context.Context, alertID AlertID) error

    // Configuration
    ConfigureAlertRules(ctx context.Context, rules []*AlertRule) error
    GetAlertRules(ctx context.Context) ([]*AlertRule, error)
}

// AlertFilter for querying alerts
type AlertFilter struct {
    Severities []AlertSeverity
    Sources    []AlertSource
    States     []AlertState
    Since      time.Time
    Until      time.Time
}

// AlertRule defines when to generate an alert
type AlertRule struct {
    ID          string
    Name        string
    Enabled     bool
    Severity    AlertSeverity
    Source      AlertSource
    Condition   AlertCondition
    For         time.Duration // Duration condition must be true
    Message     string
    Labels      map[string]string
}

// AlertCondition defines the condition for an alert
type AlertCondition struct {
    Metric      string
    Operator    ComparisonOperator
    Threshold   float64
}

type ComparisonOperator string

const (
    OpGreaterThan    ComparisonOperator = ">"
    OpLessThan       ComparisonOperator = "<"
    OpGreaterOrEqual ComparisonOperator = ">="
    OpLessOrEqual    ComparisonOperator = "<="
    OpEqual          ComparisonOperator = "=="
    OpNotEqual       ComparisonOperator = "!="
)
```

## Outbound Ports

### ContainerProber

```go
// ContainerProber performs health probes on containers
type ContainerProber interface {
    // Execute health probe
    Probe(ctx context.Context, containerID string, config HealthCheckConfig) (*ProbeResult, error)

    // HTTP probe
    HTTPProbe(ctx context.Context, containerID string, config HTTPProbeConfig) (*ProbeResult, error)

    // TCP probe
    TCPProbe(ctx context.Context, containerID string, config TCPProbeConfig) (*ProbeResult, error)

    // Exec probe
    ExecProbe(ctx context.Context, containerID string, config ExecProbeConfig) (*ProbeResult, error)
}
```

### NodeMetricsCollector

```go
// NodeMetricsCollector collects node-level metrics
type NodeMetricsCollector interface {
    // Collect current metrics
    Collect(ctx context.Context) (*NodeMetrics, error)

    // Collect specific metrics
    CollectCPU(ctx context.Context) (*CPUMetrics, error)
    CollectMemory(ctx context.Context) (*MemoryMetrics, error)
    CollectDisk(ctx context.Context) (*DiskMetrics, error)
    CollectNetwork(ctx context.Context) (*NetworkMetrics, error)
}
```

### HealthReporter

```go
// HealthReporter reports health status to Engine
type HealthReporter interface {
    // Report node health
    ReportNodeHealth(ctx context.Context, health *NodeHealth) error

    // Report container health
    ReportContainerHealth(ctx context.Context, health *ContainerHealth) error

    // Report alert
    ReportAlert(ctx context.Context, alert *Alert) error

    // Heartbeat
    SendHeartbeat(ctx context.Context, status *AgentStatus) error
}

// AgentStatus for heartbeat
type AgentStatus struct {
    AgentID         string
    NodeID          string
    State           HealthState
    Uptime          time.Duration
    ContainersCount int
    LastCheck       time.Time
    Version         string
}
```

### MetricsStore

```go
// MetricsStore stores time-series metrics
type MetricsStore interface {
    // Store metrics
    Store(ctx context.Context, metric Metric) error
    StoreBatch(ctx context.Context, metrics []Metric) error

    // Query metrics
    Query(ctx context.Context, query MetricQuery) ([]MetricPoint, error)

    // Cleanup old data
    Cleanup(ctx context.Context, olderThan time.Duration) error
}

// Metric represents a single metric value
type Metric struct {
    Name      string
    Labels    map[string]string
    Value     float64
    Timestamp time.Time
}

// MetricQuery for querying stored metrics
type MetricQuery struct {
    Name      string
    Labels    map[string]string
    Start     time.Time
    End       time.Time
    Step      time.Duration
}

// MetricPoint represents a point in time-series data
type MetricPoint struct {
    Timestamp time.Time
    Value     float64
}
```

## Use Cases

### HealthCheckUseCase

```go
type HealthCheckUseCase struct {
    prober     ContainerProber
    collector  NodeMetricsCollector
    reporter   HealthReporter
    store      MetricsStore
    events     EventPublisher
    containers ContainerLister
}

func (uc *HealthCheckUseCase) CheckNodeHealth(ctx context.Context) (*NodeHealth, error) {
    // 1. Collect node metrics
    metrics, err := uc.collector.Collect(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to collect metrics: %w", err)
    }

    // 2. Build health status
    health := &NodeHealth{
        LastCheck: time.Now(),
        CPU: ResourceHealth{
            Current: metrics.CPU.UsagePercent,
            Limit:   100,
        },
        Memory: ResourceHealth{
            Current: float64(metrics.Memory.Used),
            Limit:   float64(metrics.Memory.Total),
        },
        Disk: ResourceHealth{
            Current: float64(metrics.Disk.Used),
            Limit:   float64(metrics.Disk.Total),
        },
        Components: make(map[string]ComponentHealth),
    }

    // 3. Evaluate thresholds
    thresholds := Thresholds{Warning: 70, Critical: 90}
    health.CPU.Evaluate(thresholds)
    health.Memory.Evaluate(thresholds)
    health.Disk.Evaluate(thresholds)

    // 4. Check component health
    health.Components["container-runtime"] = uc.checkContainerRuntime(ctx)
    health.Components["network-node"] = uc.checkNetworkNode(ctx)
    health.Components["security-executor"] = uc.checkSecurityExecutor(ctx)

    // 5. Evaluate overall state
    health.EvaluateState()

    // 6. Store metrics
    uc.storeNodeMetrics(ctx, metrics)

    // 7. Report to Engine
    if err := uc.reporter.ReportNodeHealth(ctx, health); err != nil {
        log.Printf("Warning: failed to report node health: %v", err)
    }

    return health, nil
}

func (uc *HealthCheckUseCase) CheckContainerHealth(ctx context.Context, containerID string) (*ContainerHealth, error) {
    // 1. Get container info
    container, err := uc.containers.Get(ctx, containerID)
    if err != nil {
        return nil, fmt.Errorf("container not found: %w", err)
    }

    health := &ContainerHealth{
        ContainerID:  containerID,
        ServiceID:    container.ServiceID,
        LastCheck:    time.Now(),
        RestartCount: container.RestartCount,
    }

    // 2. Execute liveness probe
    if container.HealthCheck != nil && container.HealthCheck.Liveness != nil {
        result, err := uc.prober.Probe(ctx, containerID, *container.HealthCheck.Liveness)
        if err != nil {
            health.Liveness = ProbeResult{
                Type:    ProbeTypeLiveness,
                Success: false,
                Message: err.Error(),
            }
        } else {
            health.Liveness = *result
            health.Liveness.Type = ProbeTypeLiveness
        }
    } else {
        // Default: check if container is running
        health.Liveness = ProbeResult{
            Type:    ProbeTypeLiveness,
            Success: container.State == "running",
            Message: fmt.Sprintf("Container is %s", container.State),
        }
    }

    // 3. Execute readiness probe
    if container.HealthCheck != nil && container.HealthCheck.Readiness != nil {
        result, err := uc.prober.Probe(ctx, containerID, *container.HealthCheck.Readiness)
        if err != nil {
            health.Readiness = ProbeResult{
                Type:    ProbeTypeReadiness,
                Success: false,
                Message: err.Error(),
            }
        } else {
            health.Readiness = *result
            health.Readiness.Type = ProbeTypeReadiness
        }
    } else {
        // Default: same as liveness
        health.Readiness = health.Liveness
        health.Readiness.Type = ProbeTypeReadiness
    }

    // 4. Collect resource metrics
    health.Resources = uc.collectContainerResources(ctx, containerID)

    // 5. Evaluate overall state
    health.EvaluateState()

    // 6. Report to Engine
    if err := uc.reporter.ReportContainerHealth(ctx, health); err != nil {
        log.Printf("Warning: failed to report container health: %v", err)
    }

    // 7. Emit health event if state changed
    previousState := uc.getPreviousState(containerID)
    if previousState != health.State {
        uc.events.Publish(HealthEvent{
            ResourceID:    containerID,
            ResourceType:  "container",
            EventType:     HealthEventStateChanged,
            PreviousState: previousState,
            CurrentState:  health.State,
            Timestamp:     time.Now(),
        })
    }

    return health, nil
}

func (uc *HealthCheckUseCase) RunHealthCheckLoop(ctx context.Context) {
    nodeCheckTicker := time.NewTicker(30 * time.Second)
    containerCheckTicker := time.NewTicker(10 * time.Second)
    heartbeatTicker := time.NewTicker(5 * time.Second)

    defer nodeCheckTicker.Stop()
    defer containerCheckTicker.Stop()
    defer heartbeatTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            return

        case <-nodeCheckTicker.C:
            if _, err := uc.CheckNodeHealth(ctx); err != nil {
                log.Printf("Node health check failed: %v", err)
            }

        case <-containerCheckTicker.C:
            containers, err := uc.containers.List(ctx)
            if err != nil {
                log.Printf("Failed to list containers: %v", err)
                continue
            }

            for _, container := range containers {
                if _, err := uc.CheckContainerHealth(ctx, container.ID); err != nil {
                    log.Printf("Container health check failed for %s: %v", container.ID, err)
                }
            }

        case <-heartbeatTicker.C:
            status := &AgentStatus{
                State:     HealthStateHealthy,
                LastCheck: time.Now(),
            }
            if err := uc.reporter.SendHeartbeat(ctx, status); err != nil {
                log.Printf("Heartbeat failed: %v", err)
            }
        }
    }
}
```

### AlertManagementUseCase

```go
type AlertManagementUseCase struct {
    store    AlertStore
    notifier AlertNotifier
    rules    []*AlertRule
    events   EventPublisher
    mu       sync.RWMutex
}

func (uc *AlertManagementUseCase) EvaluateAlerts(ctx context.Context, metrics *NodeMetrics) error {
    uc.mu.RLock()
    rules := uc.rules
    uc.mu.RUnlock()

    for _, rule := range rules {
        if !rule.Enabled {
            continue
        }

        value := uc.getMetricValue(metrics, rule.Condition.Metric)
        triggered := uc.evaluateCondition(value, rule.Condition)

        if triggered {
            alert := &Alert{
                ID:         AlertID(uuid.New().String()),
                Severity:   rule.Severity,
                Source:     rule.Source,
                Message:    rule.Message,
                CreatedAt:  time.Now(),
                UpdatedAt:  time.Now(),
                State:      AlertStateFiring,
                Details: map[string]interface{}{
                    "rule_id":   rule.ID,
                    "metric":    rule.Condition.Metric,
                    "value":     value,
                    "threshold": rule.Condition.Threshold,
                },
            }

            // Check if alert already exists
            existing, _ := uc.store.FindByRule(ctx, rule.ID)
            if existing != nil && existing.State == AlertStateFiring {
                // Update existing alert
                existing.UpdatedAt = time.Now()
                uc.store.Update(ctx, existing)
            } else {
                // Create new alert
                if err := uc.store.Create(ctx, alert); err != nil {
                    log.Printf("Failed to create alert: %v", err)
                    continue
                }

                // Notify
                if err := uc.notifier.Notify(ctx, alert); err != nil {
                    log.Printf("Failed to notify alert: %v", err)
                }

                // Emit event
                uc.events.Publish(HealthEvent{
                    ResourceType: string(rule.Source),
                    EventType:    HealthEventAlertRaised,
                    Timestamp:    time.Now(),
                    Details:      map[string]interface{}{"alert": alert},
                })
            }
        } else {
            // Check if there's an existing alert to resolve
            existing, _ := uc.store.FindByRule(ctx, rule.ID)
            if existing != nil && existing.State == AlertStateFiring {
                existing.State = AlertStateResolved
                existing.ResolvedAt = &time.Time{}
                *existing.ResolvedAt = time.Now()
                existing.UpdatedAt = time.Now()
                uc.store.Update(ctx, existing)

                uc.events.Publish(HealthEvent{
                    ResourceType: string(rule.Source),
                    EventType:    HealthEventAlertResolved,
                    Timestamp:    time.Now(),
                    Details:      map[string]interface{}{"alert": existing},
                })
            }
        }
    }

    return nil
}

func (uc *AlertManagementUseCase) evaluateCondition(value float64, condition AlertCondition) bool {
    switch condition.Operator {
    case OpGreaterThan:
        return value > condition.Threshold
    case OpLessThan:
        return value < condition.Threshold
    case OpGreaterOrEqual:
        return value >= condition.Threshold
    case OpLessOrEqual:
        return value <= condition.Threshold
    case OpEqual:
        return value == condition.Threshold
    case OpNotEqual:
        return value != condition.Threshold
    default:
        return false
    }
}
```

## Health Check Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                       Health Monitoring Flow                             │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                    Health Check Loop                             │    │
│  │                                                                  │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │    │
│  │  │ Node Check   │  │ Container    │  │  Heartbeat   │           │    │
│  │  │ (30s)        │  │ Check (10s)  │  │  (5s)        │           │    │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘           │    │
│  └─────────┼─────────────────┼─────────────────┼───────────────────┘    │
│            │                 │                 │                         │
│            ▼                 ▼                 ▼                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                   │
│  │  Collect     │  │  Execute     │  │   Report     │                   │
│  │  Metrics     │  │  Probes      │  │   Status     │                   │
│  │  (CPU,RAM,..)│  │  (HTTP,TCP)  │  │   to Engine  │                   │
│  └──────┬───────┘  └──────┬───────┘  └──────────────┘                   │
│         │                 │                                              │
│         ▼                 ▼                                              │
│  ┌──────────────┐  ┌──────────────┐                                     │
│  │  Evaluate    │  │   Update     │                                     │
│  │  Thresholds  │  │   State      │                                     │
│  └──────┬───────┘  └──────┬───────┘                                     │
│         │                 │                                              │
│         ▼                 ▼                                              │
│  ┌──────────────────────────────────────────────────────────────────┐   │
│  │                    Alert Evaluation                               │   │
│  │                                                                   │   │
│  │   ┌─────────────┐   ┌─────────────┐   ┌─────────────────────┐   │   │
│  │   │ Check Rules │──►│ Fire/Resolve│──►│ Notify & Emit Event │   │   │
│  │   └─────────────┘   └─────────────┘   └─────────────────────┘   │   │
│  │                                                                   │   │
│  └──────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Error Handling

```go
// Domain errors
var (
    ErrContainerNotFound    = errors.New("container not found")
    ErrProbeTimeout         = errors.New("probe timed out")
    ErrProbeFailed          = errors.New("probe failed")
    ErrMetricsUnavailable   = errors.New("metrics unavailable")
    ErrAlertNotFound        = errors.New("alert not found")
    ErrInvalidAlertRule     = errors.New("invalid alert rule")
)

// Health check result handling
func handleProbeError(err error) ProbeResult {
    result := ProbeResult{
        Success: false,
        Message: err.Error(),
    }

    if errors.Is(err, context.DeadlineExceeded) {
        result.Message = "probe timed out"
    }

    return result
}
```

## Testing Strategy

```go
// Unit test for health evaluation
func TestContainerHealth_Evaluate(t *testing.T) {
    tests := []struct {
        name     string
        health   ContainerHealth
        expected HealthState
    }{
        {
            name: "healthy container",
            health: ContainerHealth{
                Liveness:  ProbeResult{Success: true},
                Readiness: ProbeResult{Success: true},
                Resources: ContainerResourceHealth{CPUPercent: 50, MemoryPercent: 50},
            },
            expected: HealthStateHealthy,
        },
        {
            name: "unhealthy liveness",
            health: ContainerHealth{
                Liveness:  ProbeResult{Success: false, ConsecutiveFailures: 3},
                Readiness: ProbeResult{Success: true},
            },
            expected: HealthStateUnhealthy,
        },
        {
            name: "degraded due to resources",
            health: ContainerHealth{
                Liveness:  ProbeResult{Success: true},
                Readiness: ProbeResult{Success: true},
                Resources: ContainerResourceHealth{CPUPercent: 95, MemoryPercent: 50},
            },
            expected: HealthStateDegraded,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tt.health.EvaluateState()
            assert.Equal(t, tt.expected, tt.health.State)
        })
    }
}

// Integration test for health monitoring
func TestHealthMonitor_Integration(t *testing.T) {
    mockProber := &MockContainerProber{}
    mockCollector := &MockNodeMetricsCollector{}
    mockReporter := &MockHealthReporter{}

    uc := &HealthCheckUseCase{
        prober:    mockProber,
        collector: mockCollector,
        reporter:  mockReporter,
    }

    mockCollector.On("Collect", mock.Anything).Return(&NodeMetrics{
        CPU:    CPUMetrics{UsagePercent: 45},
        Memory: MemoryMetrics{Used: 4 * 1024 * 1024 * 1024, Total: 16 * 1024 * 1024 * 1024},
    }, nil)
    mockReporter.On("ReportNodeHealth", mock.Anything, mock.Anything).Return(nil)

    health, err := uc.CheckNodeHealth(context.Background())

    require.NoError(t, err)
    assert.Equal(t, HealthStateHealthy, health.State)
    mockCollector.AssertExpectations(t)
    mockReporter.AssertExpectations(t)
}
```

## Related Documents

- [Container Runtime](./container-runtime.md) - Provides container information
- [Task Executor](./task-executor.md) - Coordinates health tasks
- [State Manager](../engine/state-manager.md) - Uses health data for reconciliation
- [Agent Registry](../engine/agent-registry.md) - Reports agent health
