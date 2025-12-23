# Agent Registry

## Overview

The Agent Registry manages agent registration, heartbeats, capability tracking, and intelligent agent selection for task dispatch. It maintains a real-time view of all agents in the cluster and their current state.

## Responsibilities

1. **Agent Registration** - Handle agent join/leave lifecycle
2. **Heartbeat Processing** - Track agent liveness via periodic heartbeats
3. **Capability Tracking** - Maintain agent capabilities and resource availability
4. **Agent Selection** - Select optimal agents for task placement
5. **Health Monitoring** - Detect and handle agent failures

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Agent Registry                                  │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      Driving Adapters                            │   │
│  │  ┌─────────────────┐  ┌─────────────────┐                       │   │
│  │  │  gRPC Handler   │  │  Event Handler  │                       │   │
│  │  │  (Agent API)    │  │ (Heartbeat Sub) │                       │   │
│  │  └────────┬────────┘  └────────┬────────┘                       │   │
│  └───────────┼────────────────────┼────────────────────────────────┘   │
│              │                    │                                     │
│  ┌───────────▼────────────────────▼────────────────────────────────┐   │
│  │                       Inbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │               RegistryService Interface                  │    │   │
│  │  │  - RegisterAgent(agent) → AgentID                       │    │   │
│  │  │  - DeregisterAgent(agentID) → void                      │    │   │
│  │  │  - ProcessHeartbeat(agentID, status) → void             │    │   │
│  │  │  - SelectAgents(criteria) → []Agent                     │    │   │
│  │  │  - GetAgent(agentID) → Agent                            │    │   │
│  │  │  - ListAgents(filter) → []Agent                         │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                        Use Cases                                 │   │
│  │  ┌──────────────┐ ┌────────────────┐ ┌────────────────────┐    │   │
│  │  │ Registration │ │   Heartbeat    │ │  Agent Selection   │    │   │
│  │  │   UseCase    │ │    UseCase     │ │     UseCase        │    │   │
│  │  └──────────────┘ └────────────────┘ └────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                       Domain Layer                               │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │    Agent    │ │ AgentStatus  │ │  SelectionCriteria     │   │   │
│  │  │   Entity    │ │ Value Object │ │    Value Object        │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │ Capability  │ │  Resources   │ │   SelectionStrategy    │   │   │
│  │  │Value Object │ │ Value Object │ │      Interface         │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Outbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              AgentRepository Interface                   │    │   │
│  │  │  - Save(agent) → error                                  │    │   │
│  │  │  - FindByID(id) → Agent                                 │    │   │
│  │  │  - FindAll() → []Agent                                  │    │   │
│  │  │  - FindByStatus(status) → []Agent                       │    │   │
│  │  │  - FindByCapability(cap) → []Agent                      │    │   │
│  │  │  - Delete(id) → error                                   │    │   │
│  │  │  - Watch() → <-chan AgentEvent                          │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              EventPublisher Interface                    │    │   │
│  │  │  - Publish(event AgentEvent) → error                    │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Driven Adapters                             │   │
│  │  ┌──────────────────┐  ┌──────────────────┐                     │   │
│  │  │  etcd Repository │  │  Event Publisher │                     │   │
│  │  │   (Persistence)  │  │    (Pub/Sub)     │                     │   │
│  │  └──────────────────┘  └──────────────────┘                     │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// Agent represents a registered agent in the cluster
type Agent struct {
    ID           AgentID
    Hostname     string
    Address      string       // gRPC address
    Status       AgentStatus
    Capabilities []Capability
    Resources    Resources
    Labels       map[string]string
    RegisteredAt time.Time
    LastHeartbeat time.Time
    Version      string
}

// AgentID is a unique identifier for an agent
type AgentID string

func NewAgentID() AgentID {
    return AgentID(uuid.New().String())
}
```

### Value Objects

```go
// AgentStatus represents the current state of an agent
type AgentStatus string

const (
    AgentStatusOnline      AgentStatus = "online"
    AgentStatusOffline     AgentStatus = "offline"
    AgentStatusDraining    AgentStatus = "draining"    // Not accepting new tasks
    AgentStatusUnreachable AgentStatus = "unreachable" // Missed heartbeats
)

// Capability represents what an agent can do
type Capability struct {
    Type    CapabilityType
    Version string
    Config  map[string]string
}

type CapabilityType string

const (
    CapabilityContainerRuntime CapabilityType = "container_runtime"
    CapabilityNetworkNode      CapabilityType = "network_node"
    CapabilitySecurityExecutor CapabilityType = "security_executor"
    CapabilityStorageDriver    CapabilityType = "storage_driver"
)

// Resources represents available resources on an agent
type Resources struct {
    CPUCores       int     // Number of CPU cores
    MemoryMB       int64   // Total memory in MB
    CPUAvailable   float64 // Available CPU (0.0-1.0 per core)
    MemoryFreeMB   int64   // Available memory in MB
    ContainerCount int     // Current running containers
    MaxContainers  int     // Maximum containers allowed
}

// HasCapacity checks if agent has capacity for workload
func (r Resources) HasCapacity(cpuRequired float64, memoryMB int64) bool {
    return r.CPUAvailable >= cpuRequired &&
           r.MemoryFreeMB >= memoryMB &&
           r.ContainerCount < r.MaxContainers
}

// SelectionCriteria specifies requirements for agent selection
type SelectionCriteria struct {
    RequiredCapabilities []CapabilityType
    MinCPU               float64
    MinMemoryMB          int64
    Labels               map[string]string // Label selectors
    PreferredAgents      []AgentID         // Soft preference
    ExcludedAgents       []AgentID         // Hard exclusion
    Count                int               // Number of agents needed
}

// AgentEvent represents changes to agent state
type AgentEvent struct {
    Type      AgentEventType
    AgentID   AgentID
    Agent     *Agent
    Timestamp time.Time
}

type AgentEventType string

const (
    AgentEventRegistered   AgentEventType = "registered"
    AgentEventDeregistered AgentEventType = "deregistered"
    AgentEventOnline       AgentEventType = "online"
    AgentEventOffline      AgentEventType = "offline"
    AgentEventUpdated      AgentEventType = "updated"
)
```

### Selection Strategies

```go
// SelectionStrategy defines how agents are selected for tasks
type SelectionStrategy interface {
    Select(candidates []Agent, criteria SelectionCriteria) []Agent
}

// RoundRobinStrategy distributes load evenly
type RoundRobinStrategy struct {
    lastIndex int
    mu        sync.Mutex
}

// LeastLoadedStrategy prefers agents with most available resources
type LeastLoadedStrategy struct{}

// SpreadStrategy ensures containers spread across different agents
type SpreadStrategy struct {
    // Considers existing container placement
}

// BinPackStrategy consolidates containers to fewer agents
type BinPackStrategy struct {
    // Fills agents before using new ones
}
```

## Ports

### Inbound Ports (Service Interface)

```go
// RegistryService defines the agent registry operations
type RegistryService interface {
    // Registration
    RegisterAgent(ctx context.Context, req RegisterAgentRequest) (*Agent, error)
    DeregisterAgent(ctx context.Context, agentID AgentID) error

    // Heartbeat
    ProcessHeartbeat(ctx context.Context, agentID AgentID, status HeartbeatStatus) error

    // Query
    GetAgent(ctx context.Context, agentID AgentID) (*Agent, error)
    ListAgents(ctx context.Context, filter AgentFilter) ([]Agent, error)

    // Selection
    SelectAgents(ctx context.Context, criteria SelectionCriteria) ([]Agent, error)

    // Maintenance
    DrainAgent(ctx context.Context, agentID AgentID) error
    ActivateAgent(ctx context.Context, agentID AgentID) error
}

// RegisterAgentRequest contains registration details
type RegisterAgentRequest struct {
    Hostname     string
    Address      string
    Capabilities []Capability
    Resources    Resources
    Labels       map[string]string
    Version      string
}

// HeartbeatStatus contains agent's current status
type HeartbeatStatus struct {
    Status         AgentStatus
    Resources      Resources
    RunningTasks   []string
    LastError      string
}

// AgentFilter specifies query filters
type AgentFilter struct {
    Status       *AgentStatus
    Capabilities []CapabilityType
    Labels       map[string]string
}
```

### Outbound Ports (Repository Interface)

```go
// AgentRepository defines persistence operations
type AgentRepository interface {
    Save(ctx context.Context, agent *Agent) error
    FindByID(ctx context.Context, id AgentID) (*Agent, error)
    FindAll(ctx context.Context) ([]Agent, error)
    FindByStatus(ctx context.Context, status AgentStatus) ([]Agent, error)
    FindByCapability(ctx context.Context, cap CapabilityType) ([]Agent, error)
    FindByLabels(ctx context.Context, labels map[string]string) ([]Agent, error)
    Delete(ctx context.Context, id AgentID) error
    Watch(ctx context.Context) (<-chan AgentEvent, error)
}

// EventPublisher publishes agent events
type EventPublisher interface {
    Publish(ctx context.Context, event AgentEvent) error
    Subscribe(ctx context.Context) (<-chan AgentEvent, error)
}
```

## Use Cases

### Registration Use Case

```go
// RegistrationUseCase handles agent registration lifecycle
type RegistrationUseCase struct {
    repo      AgentRepository
    publisher EventPublisher
    logger    *slog.Logger
}

func (uc *RegistrationUseCase) Register(
    ctx context.Context,
    req RegisterAgentRequest,
) (*Agent, error) {
    // Validate request
    if err := uc.validateRequest(req); err != nil {
        return nil, fmt.Errorf("invalid request: %w", err)
    }

    // Create agent entity
    agent := &Agent{
        ID:           NewAgentID(),
        Hostname:     req.Hostname,
        Address:      req.Address,
        Status:       AgentStatusOnline,
        Capabilities: req.Capabilities,
        Resources:    req.Resources,
        Labels:       req.Labels,
        RegisteredAt: time.Now(),
        LastHeartbeat: time.Now(),
        Version:      req.Version,
    }

    // Persist
    if err := uc.repo.Save(ctx, agent); err != nil {
        return nil, fmt.Errorf("failed to save agent: %w", err)
    }

    // Publish event
    uc.publisher.Publish(ctx, AgentEvent{
        Type:      AgentEventRegistered,
        AgentID:   agent.ID,
        Agent:     agent,
        Timestamp: time.Now(),
    })

    uc.logger.Info("agent registered",
        "agent_id", agent.ID,
        "hostname", agent.Hostname,
    )

    return agent, nil
}

func (uc *RegistrationUseCase) Deregister(
    ctx context.Context,
    agentID AgentID,
) error {
    agent, err := uc.repo.FindByID(ctx, agentID)
    if err != nil {
        return fmt.Errorf("agent not found: %w", err)
    }

    if err := uc.repo.Delete(ctx, agentID); err != nil {
        return fmt.Errorf("failed to delete agent: %w", err)
    }

    uc.publisher.Publish(ctx, AgentEvent{
        Type:      AgentEventDeregistered,
        AgentID:   agentID,
        Agent:     agent,
        Timestamp: time.Now(),
    })

    return nil
}
```

### Heartbeat Use Case

```go
// HeartbeatUseCase handles agent heartbeat processing
type HeartbeatUseCase struct {
    repo           AgentRepository
    publisher      EventPublisher
    heartbeatTTL   time.Duration
    checkInterval  time.Duration
    logger         *slog.Logger
}

func (uc *HeartbeatUseCase) ProcessHeartbeat(
    ctx context.Context,
    agentID AgentID,
    status HeartbeatStatus,
) error {
    agent, err := uc.repo.FindByID(ctx, agentID)
    if err != nil {
        return fmt.Errorf("agent not found: %w", err)
    }

    previousStatus := agent.Status

    // Update agent state
    agent.LastHeartbeat = time.Now()
    agent.Status = status.Status
    agent.Resources = status.Resources

    if err := uc.repo.Save(ctx, agent); err != nil {
        return fmt.Errorf("failed to update agent: %w", err)
    }

    // Publish status change event if needed
    if previousStatus != agent.Status {
        eventType := AgentEventUpdated
        if agent.Status == AgentStatusOnline {
            eventType = AgentEventOnline
        } else if agent.Status == AgentStatusOffline {
            eventType = AgentEventOffline
        }

        uc.publisher.Publish(ctx, AgentEvent{
            Type:      eventType,
            AgentID:   agentID,
            Agent:     agent,
            Timestamp: time.Now(),
        })
    }

    return nil
}

// StartHealthChecker runs background health monitoring
func (uc *HeartbeatUseCase) StartHealthChecker(ctx context.Context) {
    ticker := time.NewTicker(uc.checkInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            uc.checkAgentHealth(ctx)
        }
    }
}

func (uc *HeartbeatUseCase) checkAgentHealth(ctx context.Context) {
    agents, _ := uc.repo.FindByStatus(ctx, AgentStatusOnline)

    for _, agent := range agents {
        if time.Since(agent.LastHeartbeat) > uc.heartbeatTTL {
            agent.Status = AgentStatusUnreachable
            uc.repo.Save(ctx, &agent)

            uc.publisher.Publish(ctx, AgentEvent{
                Type:      AgentEventOffline,
                AgentID:   agent.ID,
                Agent:     &agent,
                Timestamp: time.Now(),
            })

            uc.logger.Warn("agent unreachable",
                "agent_id", agent.ID,
                "last_heartbeat", agent.LastHeartbeat,
            )
        }
    }
}
```

### Selection Use Case

```go
// SelectionUseCase handles intelligent agent selection
type SelectionUseCase struct {
    repo       AgentRepository
    strategies map[string]SelectionStrategy
    logger     *slog.Logger
}

func (uc *SelectionUseCase) SelectAgents(
    ctx context.Context,
    criteria SelectionCriteria,
    strategyName string,
) ([]Agent, error) {
    // Get all online agents
    candidates, err := uc.repo.FindByStatus(ctx, AgentStatusOnline)
    if err != nil {
        return nil, fmt.Errorf("failed to get agents: %w", err)
    }

    // Filter by capabilities
    candidates = uc.filterByCapabilities(candidates, criteria.RequiredCapabilities)

    // Filter by resources
    candidates = uc.filterByResources(candidates, criteria)

    // Filter by labels
    candidates = uc.filterByLabels(candidates, criteria.Labels)

    // Apply exclusions
    candidates = uc.applyExclusions(candidates, criteria.ExcludedAgents)

    if len(candidates) < criteria.Count {
        return nil, fmt.Errorf("insufficient agents: need %d, found %d",
            criteria.Count, len(candidates))
    }

    // Apply selection strategy
    strategy := uc.strategies[strategyName]
    if strategy == nil {
        strategy = &LeastLoadedStrategy{} // Default
    }

    selected := strategy.Select(candidates, criteria)

    return selected[:criteria.Count], nil
}

func (uc *SelectionUseCase) filterByCapabilities(
    agents []Agent,
    required []CapabilityType,
) []Agent {
    if len(required) == 0 {
        return agents
    }

    var filtered []Agent
    for _, agent := range agents {
        if agent.HasCapabilities(required) {
            filtered = append(filtered, agent)
        }
    }
    return filtered
}

func (uc *SelectionUseCase) filterByResources(
    agents []Agent,
    criteria SelectionCriteria,
) []Agent {
    var filtered []Agent
    for _, agent := range agents {
        if agent.Resources.HasCapacity(criteria.MinCPU, criteria.MinMemoryMB) {
            filtered = append(filtered, agent)
        }
    }
    return filtered
}

// LeastLoadedStrategy implementation
func (s *LeastLoadedStrategy) Select(
    candidates []Agent,
    criteria SelectionCriteria,
) []Agent {
    // Sort by available resources (most to least)
    sort.Slice(candidates, func(i, j int) bool {
        scoreI := candidates[i].Resources.CPUAvailable +
                  float64(candidates[i].Resources.MemoryFreeMB)/1024
        scoreJ := candidates[j].Resources.CPUAvailable +
                  float64(candidates[j].Resources.MemoryFreeMB)/1024
        return scoreI > scoreJ
    })
    return candidates
}
```

## Adapters

### Driving Adapter: gRPC Handler

```go
// GRPCHandler handles gRPC requests for agent registry
type GRPCHandler struct {
    pb.UnimplementedAgentRegistryServer
    service RegistryService
}

func (h *GRPCHandler) RegisterAgent(
    ctx context.Context,
    req *pb.RegisterAgentRequest,
) (*pb.RegisterAgentResponse, error) {
    agent, err := h.service.RegisterAgent(ctx, RegisterAgentRequest{
        Hostname:     req.Hostname,
        Address:      req.Address,
        Capabilities: convertCapabilities(req.Capabilities),
        Resources:    convertResources(req.Resources),
        Labels:       req.Labels,
        Version:      req.Version,
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "registration failed: %v", err)
    }

    return &pb.RegisterAgentResponse{
        AgentId: string(agent.ID),
    }, nil
}

func (h *GRPCHandler) Heartbeat(
    ctx context.Context,
    req *pb.HeartbeatRequest,
) (*pb.HeartbeatResponse, error) {
    err := h.service.ProcessHeartbeat(ctx, AgentID(req.AgentId), HeartbeatStatus{
        Status:    AgentStatus(req.Status),
        Resources: convertResources(req.Resources),
    })
    if err != nil {
        return nil, status.Errorf(codes.Internal, "heartbeat failed: %v", err)
    }

    return &pb.HeartbeatResponse{
        Acknowledged: true,
    }, nil
}

func (h *GRPCHandler) SelectAgents(
    ctx context.Context,
    req *pb.SelectAgentsRequest,
) (*pb.SelectAgentsResponse, error) {
    agents, err := h.service.SelectAgents(ctx, SelectionCriteria{
        RequiredCapabilities: convertCapabilityTypes(req.RequiredCapabilities),
        MinCPU:               req.MinCpu,
        MinMemoryMB:          req.MinMemoryMb,
        Count:                int(req.Count),
    })
    if err != nil {
        return nil, status.Errorf(codes.ResourceExhausted,
            "selection failed: %v", err)
    }

    return &pb.SelectAgentsResponse{
        Agents: convertAgentsToPB(agents),
    }, nil
}
```

### Driven Adapter: etcd Repository

```go
// EtcdAgentRepository implements AgentRepository using etcd
type EtcdAgentRepository struct {
    client *clientv3.Client
    prefix string
}

func NewEtcdAgentRepository(client *clientv3.Client) *EtcdAgentRepository {
    return &EtcdAgentRepository{
        client: client,
        prefix: "/banyan/agents/",
    }
}

func (r *EtcdAgentRepository) Save(ctx context.Context, agent *Agent) error {
    data, err := json.Marshal(agent)
    if err != nil {
        return fmt.Errorf("marshal error: %w", err)
    }

    key := r.prefix + string(agent.ID)
    _, err = r.client.Put(ctx, key, string(data))
    return err
}

func (r *EtcdAgentRepository) FindByID(
    ctx context.Context,
    id AgentID,
) (*Agent, error) {
    key := r.prefix + string(id)
    resp, err := r.client.Get(ctx, key)
    if err != nil {
        return nil, err
    }

    if len(resp.Kvs) == 0 {
        return nil, ErrAgentNotFound
    }

    var agent Agent
    if err := json.Unmarshal(resp.Kvs[0].Value, &agent); err != nil {
        return nil, err
    }

    return &agent, nil
}

func (r *EtcdAgentRepository) FindByStatus(
    ctx context.Context,
    status AgentStatus,
) ([]Agent, error) {
    resp, err := r.client.Get(ctx, r.prefix, clientv3.WithPrefix())
    if err != nil {
        return nil, err
    }

    var agents []Agent
    for _, kv := range resp.Kvs {
        var agent Agent
        if err := json.Unmarshal(kv.Value, &agent); err != nil {
            continue
        }
        if agent.Status == status {
            agents = append(agents, agent)
        }
    }

    return agents, nil
}

func (r *EtcdAgentRepository) Watch(
    ctx context.Context,
) (<-chan AgentEvent, error) {
    events := make(chan AgentEvent, 100)

    watchChan := r.client.Watch(ctx, r.prefix, clientv3.WithPrefix())

    go func() {
        defer close(events)

        for resp := range watchChan {
            for _, ev := range resp.Events {
                var eventType AgentEventType
                var agent Agent

                switch ev.Type {
                case clientv3.EventTypePut:
                    json.Unmarshal(ev.Kv.Value, &agent)
                    eventType = AgentEventUpdated
                case clientv3.EventTypeDelete:
                    eventType = AgentEventDeregistered
                }

                events <- AgentEvent{
                    Type:      eventType,
                    AgentID:   agent.ID,
                    Agent:     &agent,
                    Timestamp: time.Now(),
                }
            }
        }
    }()

    return events, nil
}
```

## Configuration

```go
type AgentRegistryConfig struct {
    // Heartbeat settings
    HeartbeatInterval time.Duration `yaml:"heartbeat_interval"` // Default: 10s
    HeartbeatTimeout  time.Duration `yaml:"heartbeat_timeout"`  // Default: 30s

    // Selection defaults
    DefaultStrategy   string `yaml:"default_strategy"` // Default: "least_loaded"

    // etcd settings
    EtcdEndpoints []string `yaml:"etcd_endpoints"`
    EtcdPrefix    string   `yaml:"etcd_prefix"`
}
```

## Error Handling

```go
var (
    ErrAgentNotFound       = errors.New("agent not found")
    ErrAgentAlreadyExists  = errors.New("agent already exists")
    ErrInsufficientAgents  = errors.New("insufficient agents available")
    ErrAgentDraining       = errors.New("agent is draining")
    ErrHeartbeatTimeout    = errors.New("heartbeat timeout")
)
```

## Testing

```go
func TestRegistrationUseCase_Register(t *testing.T) {
    repo := NewMockAgentRepository()
    publisher := NewMockEventPublisher()
    uc := NewRegistrationUseCase(repo, publisher)

    req := RegisterAgentRequest{
        Hostname:     "worker-1",
        Address:      "192.168.1.10:9090",
        Capabilities: []Capability{{Type: CapabilityContainerRuntime}},
        Resources:    Resources{CPUCores: 4, MemoryMB: 8192},
    }

    agent, err := uc.Register(context.Background(), req)

    assert.NoError(t, err)
    assert.NotEmpty(t, agent.ID)
    assert.Equal(t, AgentStatusOnline, agent.Status)
    assert.Len(t, publisher.Events, 1)
    assert.Equal(t, AgentEventRegistered, publisher.Events[0].Type)
}

func TestSelectionUseCase_SelectAgents(t *testing.T) {
    repo := NewMockAgentRepository()
    // Add test agents
    repo.agents = []Agent{
        {ID: "agent-1", Status: AgentStatusOnline, Resources: Resources{CPUAvailable: 2.0}},
        {ID: "agent-2", Status: AgentStatusOnline, Resources: Resources{CPUAvailable: 3.0}},
        {ID: "agent-3", Status: AgentStatusOffline},
    }

    uc := NewSelectionUseCase(repo, nil)

    agents, err := uc.SelectAgents(context.Background(), SelectionCriteria{
        MinCPU: 1.0,
        Count:  2,
    }, "least_loaded")

    assert.NoError(t, err)
    assert.Len(t, agents, 2)
    // Least loaded strategy returns highest resources first
    assert.Equal(t, AgentID("agent-2"), agents[0].ID)
}
```

## Related Documents

- [Orchestrator](./orchestrator.md) - Uses registry for agent selection
- [State Manager](./state-manager.md) - Considers agent state in reconciliation
