# Plugin Manager

## Overview

The Plugin Manager handles Type 2 Lifecycle Plugins - plugins that execute at specific points in the deployment lifecycle. These plugins allow users to extend Banyan's behavior with custom logic for validation, transformation, notification, and integration.

## Responsibilities

1. **Plugin Registry** - Register and manage available plugins
2. **Lifecycle Hooks** - Execute plugins at defined lifecycle points
3. **Plugin Execution** - Run plugins with proper isolation and timeout handling
4. **Configuration** - Manage plugin configuration and ordering
5. **Result Aggregation** - Collect and process plugin execution results

## Plugin Types

| Hook Point | Description | Example Use Cases |
|------------|-------------|-------------------|
| **pre-validate** | Before deployment validation | Schema validation, policy checks |
| **post-validate** | After validation passes | Approval workflows, audit logging |
| **pre-deploy** | Before deployment starts | Secret injection, config generation |
| **post-deploy** | After successful deployment | Notifications, DNS registration |
| **pre-stop** | Before stopping a service | Graceful drain, deregistration |
| **post-stop** | After service stopped | Cleanup, resource release |
| **on-failure** | When deployment fails | Alerting, rollback triggers |

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Plugin Manager                                   │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                      Driving Adapters                            │   │
│  │  ┌─────────────────┐  ┌─────────────────┐                       │   │
│  │  │  gRPC Handler   │  │   CLI Handler   │                       │   │
│  │  │ (Plugin Admin)  │  │  (Plugin Mgmt)  │                       │   │
│  │  └────────┬────────┘  └────────┬────────┘                       │   │
│  └───────────┼────────────────────┼────────────────────────────────┘   │
│              │                    │                                     │
│  ┌───────────▼────────────────────▼────────────────────────────────┐   │
│  │                       Inbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              PluginService Interface                     │    │   │
│  │  │  - RegisterPlugin(plugin) → error                       │    │   │
│  │  │  - UnregisterPlugin(name) → error                       │    │   │
│  │  │  - ExecuteHook(hook, context) → []Result                │    │   │
│  │  │  - GetPlugin(name) → Plugin                             │    │   │
│  │  │  - ListPlugins() → []Plugin                             │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                        Use Cases                                 │   │
│  │  ┌──────────────┐ ┌────────────────┐ ┌────────────────────┐    │   │
│  │  │  Registry    │ │   Executor     │ │   Configuration    │    │   │
│  │  │  UseCase     │ │   UseCase      │ │     UseCase        │    │   │
│  │  └──────────────┘ └────────────────┘ └────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                       Domain Layer                               │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │   Plugin    │ │    Hook      │ │   ExecutionContext     │   │   │
│  │  │   Entity    │ │ Value Object │ │    Value Object        │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  │  ┌─────────────┐ ┌──────────────┐ ┌────────────────────────┐   │   │
│  │  │   Result    │ │  PluginSpec  │ │    PluginRunner        │   │   │
│  │  │Value Object │ │ Value Object │ │      Interface         │   │   │
│  │  └─────────────┘ └──────────────┘ └────────────────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Outbound Ports                              │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              PluginRepository Interface                  │    │   │
│  │  │  - Save(plugin) → error                                 │    │   │
│  │  │  - FindByName(name) → Plugin                            │    │   │
│  │  │  - FindByHook(hook) → []Plugin                          │    │   │
│  │  │  - Delete(name) → error                                 │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  │  ┌─────────────────────────────────────────────────────────┐    │   │
│  │  │              PluginRunner Interface                      │    │   │
│  │  │  - Run(plugin, context) → Result                        │    │   │
│  │  └─────────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                │                                        │
│  ┌─────────────────────────────▼───────────────────────────────────┐   │
│  │                      Driven Adapters                             │   │
│  │  ┌──────────────────┐  ┌──────────────────┐ ┌────────────────┐  │   │
│  │  │  etcd Repository │  │  Webhook Runner  │ │ gRPC Runner    │  │   │
│  │  │   (Persistence)  │  │    (HTTP)        │ │ (Remote Call)  │  │   │
│  │  └──────────────────┘  └──────────────────┘ └────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// Plugin represents a registered lifecycle plugin
type Plugin struct {
    Name        string
    Description string
    Type        PluginType
    Hooks       []HookPoint
    Spec        PluginSpec
    Enabled     bool
    Priority    int            // Lower = higher priority
    Timeout     time.Duration
    Config      map[string]string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

// PluginType defines how the plugin is invoked
type PluginType string

const (
    PluginTypeWebhook PluginType = "webhook" // HTTP POST to URL
    PluginTypeGRPC    PluginType = "grpc"    // gRPC call
    PluginTypeBuiltin PluginType = "builtin" // Built-in Go function
)
```

### Value Objects

```go
// HookPoint represents a lifecycle hook
type HookPoint string

const (
    HookPreValidate  HookPoint = "pre-validate"
    HookPostValidate HookPoint = "post-validate"
    HookPreDeploy    HookPoint = "pre-deploy"
    HookPostDeploy   HookPoint = "post-deploy"
    HookPreStop      HookPoint = "pre-stop"
    HookPostStop     HookPoint = "post-stop"
    HookOnFailure    HookPoint = "on-failure"
)

// PluginSpec contains type-specific configuration
type PluginSpec struct {
    // Webhook spec
    WebhookURL     string            `json:"webhook_url,omitempty"`
    WebhookHeaders map[string]string `json:"webhook_headers,omitempty"`

    // gRPC spec
    GRPCAddress string `json:"grpc_address,omitempty"`
    GRPCMethod  string `json:"grpc_method,omitempty"`

    // Builtin spec
    BuiltinName string `json:"builtin_name,omitempty"`
}

// ExecutionContext provides context to plugins during execution
type ExecutionContext struct {
    Hook          HookPoint
    DeploymentID  string
    ServiceName   string
    Namespace     string
    ServiceSpec   *ServiceSpec  // Full service specification
    PreviousSpec  *ServiceSpec  // For updates, the previous spec
    Error         error         // For on-failure hook
    Metadata      map[string]string
}

// PluginResult represents the outcome of plugin execution
type PluginResult struct {
    PluginName string
    Hook       HookPoint
    Success    bool
    Message    string
    Error      error
    Duration   time.Duration
    Output     map[string]any    // Plugin-specific output
    Mutations  []Mutation        // Requested spec changes
}

// Mutation represents a change to the service spec
type Mutation struct {
    Path      string // JSON path (e.g., "spec.replicas")
    Operation string // "set", "add", "remove"
    Value     any
}

// HookResults aggregates results from multiple plugins
type HookResults struct {
    Hook      HookPoint
    Results   []PluginResult
    AllPassed bool
    Duration  time.Duration
}
```

## Ports

### Inbound Ports (Service Interface)

```go
// PluginService defines plugin management operations
type PluginService interface {
    // Registry
    RegisterPlugin(ctx context.Context, plugin *Plugin) error
    UnregisterPlugin(ctx context.Context, name string) error
    UpdatePlugin(ctx context.Context, plugin *Plugin) error
    GetPlugin(ctx context.Context, name string) (*Plugin, error)
    ListPlugins(ctx context.Context) ([]Plugin, error)
    ListPluginsByHook(ctx context.Context, hook HookPoint) ([]Plugin, error)

    // Execution
    ExecuteHook(ctx context.Context, hook HookPoint, execCtx ExecutionContext) (*HookResults, error)

    // Configuration
    EnablePlugin(ctx context.Context, name string) error
    DisablePlugin(ctx context.Context, name string) error
    SetPriority(ctx context.Context, name string, priority int) error
}
```

### Outbound Ports (Repository & Runner Interfaces)

```go
// PluginRepository defines persistence operations
type PluginRepository interface {
    Save(ctx context.Context, plugin *Plugin) error
    FindByName(ctx context.Context, name string) (*Plugin, error)
    FindByHook(ctx context.Context, hook HookPoint) ([]Plugin, error)
    FindAll(ctx context.Context) ([]Plugin, error)
    Delete(ctx context.Context, name string) error
}

// PluginRunner executes a plugin
type PluginRunner interface {
    Run(ctx context.Context, plugin *Plugin, execCtx ExecutionContext) (*PluginResult, error)
    CanRun(plugin *Plugin) bool // Check if runner supports plugin type
}
```

## Use Cases

### Registry Use Case

```go
// RegistryUseCase handles plugin registration
type RegistryUseCase struct {
    repo   PluginRepository
    logger *slog.Logger
}

func (uc *RegistryUseCase) Register(ctx context.Context, plugin *Plugin) error {
    // Validate plugin
    if err := uc.validatePlugin(plugin); err != nil {
        return fmt.Errorf("invalid plugin: %w", err)
    }

    // Check for duplicate
    existing, _ := uc.repo.FindByName(ctx, plugin.Name)
    if existing != nil {
        return ErrPluginAlreadyExists
    }

    // Set defaults
    if plugin.Timeout == 0 {
        plugin.Timeout = 30 * time.Second
    }
    plugin.CreatedAt = time.Now()
    plugin.UpdatedAt = time.Now()

    if err := uc.repo.Save(ctx, plugin); err != nil {
        return fmt.Errorf("failed to save plugin: %w", err)
    }

    uc.logger.Info("plugin registered",
        "name", plugin.Name,
        "type", plugin.Type,
        "hooks", plugin.Hooks,
    )

    return nil
}

func (uc *RegistryUseCase) validatePlugin(plugin *Plugin) error {
    if plugin.Name == "" {
        return errors.New("name required")
    }
    if len(plugin.Hooks) == 0 {
        return errors.New("at least one hook required")
    }

    switch plugin.Type {
    case PluginTypeWebhook:
        if plugin.Spec.WebhookURL == "" {
            return errors.New("webhook_url required for webhook type")
        }
    case PluginTypeGRPC:
        if plugin.Spec.GRPCAddress == "" || plugin.Spec.GRPCMethod == "" {
            return errors.New("grpc_address and grpc_method required for grpc type")
        }
    case PluginTypeBuiltin:
        if plugin.Spec.BuiltinName == "" {
            return errors.New("builtin_name required for builtin type")
        }
    default:
        return fmt.Errorf("unknown plugin type: %s", plugin.Type)
    }

    return nil
}
```

### Executor Use Case

```go
// ExecutorUseCase handles plugin execution
type ExecutorUseCase struct {
    repo    PluginRepository
    runners []PluginRunner
    logger  *slog.Logger
}

func (uc *ExecutorUseCase) ExecuteHook(
    ctx context.Context,
    hook HookPoint,
    execCtx ExecutionContext,
) (*HookResults, error) {
    start := time.Now()

    // Get plugins for this hook
    plugins, err := uc.repo.FindByHook(ctx, hook)
    if err != nil {
        return nil, fmt.Errorf("failed to get plugins: %w", err)
    }

    // Filter enabled plugins and sort by priority
    plugins = uc.filterAndSort(plugins)

    if len(plugins) == 0 {
        return &HookResults{
            Hook:      hook,
            Results:   nil,
            AllPassed: true,
            Duration:  time.Since(start),
        }, nil
    }

    execCtx.Hook = hook
    results := make([]PluginResult, 0, len(plugins))
    allPassed := true

    for _, plugin := range plugins {
        result := uc.executePlugin(ctx, &plugin, execCtx)
        results = append(results, *result)

        if !result.Success {
            allPassed = false

            // Fail fast for validation hooks
            if hook == HookPreValidate || hook == HookPostValidate {
                uc.logger.Warn("plugin failed, stopping execution",
                    "plugin", plugin.Name,
                    "hook", hook,
                    "error", result.Error,
                )
                break
            }
        }

        // Apply mutations to context for next plugin
        if len(result.Mutations) > 0 {
            execCtx = uc.applyMutations(execCtx, result.Mutations)
        }
    }

    return &HookResults{
        Hook:      hook,
        Results:   results,
        AllPassed: allPassed,
        Duration:  time.Since(start),
    }, nil
}

func (uc *ExecutorUseCase) executePlugin(
    ctx context.Context,
    plugin *Plugin,
    execCtx ExecutionContext,
) *PluginResult {
    start := time.Now()

    // Find appropriate runner
    var runner PluginRunner
    for _, r := range uc.runners {
        if r.CanRun(plugin) {
            runner = r
            break
        }
    }

    if runner == nil {
        return &PluginResult{
            PluginName: plugin.Name,
            Hook:       execCtx.Hook,
            Success:    false,
            Error:      fmt.Errorf("no runner for plugin type: %s", plugin.Type),
            Duration:   time.Since(start),
        }
    }

    // Execute with timeout
    execCtx2, cancel := context.WithTimeout(ctx, plugin.Timeout)
    defer cancel()

    result, err := runner.Run(execCtx2, plugin, execCtx)
    if err != nil {
        return &PluginResult{
            PluginName: plugin.Name,
            Hook:       execCtx.Hook,
            Success:    false,
            Error:      err,
            Duration:   time.Since(start),
        }
    }

    result.Duration = time.Since(start)
    return result
}

func (uc *ExecutorUseCase) filterAndSort(plugins []Plugin) []Plugin {
    var enabled []Plugin
    for _, p := range plugins {
        if p.Enabled {
            enabled = append(enabled, p)
        }
    }

    // Sort by priority (lower = higher priority)
    sort.Slice(enabled, func(i, j int) bool {
        return enabled[i].Priority < enabled[j].Priority
    })

    return enabled
}
```

## Adapters

### Driving Adapter: gRPC Handler

```go
// GRPCHandler handles plugin management via gRPC
type GRPCHandler struct {
    pb.UnimplementedPluginManagerServer
    service PluginService
}

func (h *GRPCHandler) RegisterPlugin(
    ctx context.Context,
    req *pb.RegisterPluginRequest,
) (*pb.RegisterPluginResponse, error) {
    plugin := &Plugin{
        Name:        req.Name,
        Description: req.Description,
        Type:        PluginType(req.Type),
        Hooks:       convertHooks(req.Hooks),
        Spec:        convertSpec(req.Spec),
        Enabled:     req.Enabled,
        Priority:    int(req.Priority),
        Timeout:     time.Duration(req.TimeoutMs) * time.Millisecond,
        Config:      req.Config,
    }

    if err := h.service.RegisterPlugin(ctx, plugin); err != nil {
        return nil, status.Errorf(codes.Internal, "registration failed: %v", err)
    }

    return &pb.RegisterPluginResponse{Success: true}, nil
}

func (h *GRPCHandler) ExecuteHook(
    ctx context.Context,
    req *pb.ExecuteHookRequest,
) (*pb.ExecuteHookResponse, error) {
    execCtx := ExecutionContext{
        DeploymentID: req.DeploymentId,
        ServiceName:  req.ServiceName,
        Namespace:    req.Namespace,
        Metadata:     req.Metadata,
    }

    results, err := h.service.ExecuteHook(ctx, HookPoint(req.Hook), execCtx)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "execution failed: %v", err)
    }

    return &pb.ExecuteHookResponse{
        AllPassed: results.AllPassed,
        Results:   convertResultsToPB(results.Results),
    }, nil
}
```

### Driven Adapter: Webhook Runner

```go
// WebhookRunner executes webhook plugins
type WebhookRunner struct {
    client *http.Client
    logger *slog.Logger
}

func NewWebhookRunner(timeout time.Duration) *WebhookRunner {
    return &WebhookRunner{
        client: &http.Client{Timeout: timeout},
        logger: slog.Default(),
    }
}

func (r *WebhookRunner) CanRun(plugin *Plugin) bool {
    return plugin.Type == PluginTypeWebhook
}

func (r *WebhookRunner) Run(
    ctx context.Context,
    plugin *Plugin,
    execCtx ExecutionContext,
) (*PluginResult, error) {
    // Build request payload
    payload := map[string]any{
        "hook":          execCtx.Hook,
        "deployment_id": execCtx.DeploymentID,
        "service_name":  execCtx.ServiceName,
        "namespace":     execCtx.Namespace,
        "service_spec":  execCtx.ServiceSpec,
        "metadata":      execCtx.Metadata,
    }

    if execCtx.Error != nil {
        payload["error"] = execCtx.Error.Error()
    }

    body, _ := json.Marshal(payload)

    req, err := http.NewRequestWithContext(
        ctx,
        http.MethodPost,
        plugin.Spec.WebhookURL,
        bytes.NewReader(body),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    for k, v := range plugin.Spec.WebhookHeaders {
        req.Header.Set(k, v)
    }

    resp, err := r.client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("webhook request failed: %w", err)
    }
    defer resp.Body.Close()

    // Parse response
    var response WebhookResponse
    if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
        return nil, fmt.Errorf("failed to parse response: %w", err)
    }

    result := &PluginResult{
        PluginName: plugin.Name,
        Hook:       execCtx.Hook,
        Success:    resp.StatusCode >= 200 && resp.StatusCode < 300 && response.Success,
        Message:    response.Message,
        Output:     response.Output,
        Mutations:  convertMutations(response.Mutations),
    }

    if !result.Success {
        result.Error = fmt.Errorf("webhook returned failure: %s", response.Message)
    }

    return result, nil
}

type WebhookResponse struct {
    Success   bool              `json:"success"`
    Message   string            `json:"message"`
    Output    map[string]any    `json:"output,omitempty"`
    Mutations []MutationRequest `json:"mutations,omitempty"`
}

type MutationRequest struct {
    Path      string `json:"path"`
    Operation string `json:"operation"`
    Value     any    `json:"value"`
}
```

### Driven Adapter: gRPC Runner

```go
// GRPCPluginRunner executes gRPC plugins
type GRPCPluginRunner struct {
    connections map[string]*grpc.ClientConn
    mu          sync.RWMutex
    logger      *slog.Logger
}

func (r *GRPCPluginRunner) CanRun(plugin *Plugin) bool {
    return plugin.Type == PluginTypeGRPC
}

func (r *GRPCPluginRunner) Run(
    ctx context.Context,
    plugin *Plugin,
    execCtx ExecutionContext,
) (*PluginResult, error) {
    conn, err := r.getConnection(plugin.Spec.GRPCAddress)
    if err != nil {
        return nil, fmt.Errorf("failed to connect: %w", err)
    }

    // Use reflection or typed client based on method
    req := &pb.PluginExecuteRequest{
        Hook:         string(execCtx.Hook),
        DeploymentId: execCtx.DeploymentID,
        ServiceName:  execCtx.ServiceName,
        Namespace:    execCtx.Namespace,
        Metadata:     execCtx.Metadata,
    }

    // Dynamic method invocation using reflection
    resp, err := r.invokeMethod(ctx, conn, plugin.Spec.GRPCMethod, req)
    if err != nil {
        return nil, fmt.Errorf("gRPC call failed: %w", err)
    }

    return &PluginResult{
        PluginName: plugin.Name,
        Hook:       execCtx.Hook,
        Success:    resp.Success,
        Message:    resp.Message,
        Output:     resp.Output,
    }, nil
}
```

### Driven Adapter: Builtin Runner

```go
// BuiltinRunner executes built-in Go plugins
type BuiltinRunner struct {
    plugins map[string]BuiltinPlugin
}

// BuiltinPlugin is the interface for built-in plugins
type BuiltinPlugin interface {
    Execute(ctx context.Context, execCtx ExecutionContext) (*PluginResult, error)
}

func NewBuiltinRunner() *BuiltinRunner {
    return &BuiltinRunner{
        plugins: map[string]BuiltinPlugin{
            "resource-validator": &ResourceValidatorPlugin{},
            "namespace-labeler":  &NamespaceLabelerPlugin{},
            "audit-logger":       &AuditLoggerPlugin{},
        },
    }
}

func (r *BuiltinRunner) CanRun(plugin *Plugin) bool {
    return plugin.Type == PluginTypeBuiltin
}

func (r *BuiltinRunner) Run(
    ctx context.Context,
    plugin *Plugin,
    execCtx ExecutionContext,
) (*PluginResult, error) {
    builtin, ok := r.plugins[plugin.Spec.BuiltinName]
    if !ok {
        return nil, fmt.Errorf("unknown builtin plugin: %s", plugin.Spec.BuiltinName)
    }

    return builtin.Execute(ctx, execCtx)
}

// ResourceValidatorPlugin validates resource requests
type ResourceValidatorPlugin struct{}

func (p *ResourceValidatorPlugin) Execute(
    ctx context.Context,
    execCtx ExecutionContext,
) (*PluginResult, error) {
    if execCtx.ServiceSpec == nil {
        return &PluginResult{Success: true, Message: "no spec to validate"}, nil
    }

    // Validate CPU and memory limits
    for _, container := range execCtx.ServiceSpec.Containers {
        if container.Resources.CPULimit == 0 {
            return &PluginResult{
                Success: false,
                Message: fmt.Sprintf("container %s missing CPU limit", container.Name),
            }, nil
        }
        if container.Resources.MemoryLimitMB == 0 {
            return &PluginResult{
                Success: false,
                Message: fmt.Sprintf("container %s missing memory limit", container.Name),
            }, nil
        }
    }

    return &PluginResult{Success: true, Message: "resource validation passed"}, nil
}
```

## Configuration

```go
type PluginManagerConfig struct {
    // Execution settings
    DefaultTimeout  time.Duration `yaml:"default_timeout"`  // Default: 30s
    MaxConcurrent   int           `yaml:"max_concurrent"`   // Default: 10
    FailFastOnError bool          `yaml:"fail_fast"`        // Default: true for validation

    // Webhook settings
    WebhookRetries    int           `yaml:"webhook_retries"`     // Default: 3
    WebhookRetryDelay time.Duration `yaml:"webhook_retry_delay"` // Default: 1s

    // etcd settings
    EtcdEndpoints []string `yaml:"etcd_endpoints"`
    EtcdPrefix    string   `yaml:"etcd_prefix"`
}
```

## Error Handling

```go
var (
    ErrPluginNotFound      = errors.New("plugin not found")
    ErrPluginAlreadyExists = errors.New("plugin already exists")
    ErrPluginDisabled      = errors.New("plugin is disabled")
    ErrPluginTimeout       = errors.New("plugin execution timeout")
    ErrInvalidPlugin       = errors.New("invalid plugin configuration")
)
```

## Testing

```go
func TestExecutorUseCase_ExecuteHook(t *testing.T) {
    repo := NewMockPluginRepository()
    runner := NewMockPluginRunner()

    // Register a test plugin
    repo.plugins = []Plugin{
        {
            Name:     "test-validator",
            Type:     PluginTypeBuiltin,
            Hooks:    []HookPoint{HookPreValidate},
            Enabled:  true,
            Priority: 1,
        },
    }

    runner.result = &PluginResult{
        Success: true,
        Message: "validation passed",
    }

    uc := NewExecutorUseCase(repo, []PluginRunner{runner})

    results, err := uc.ExecuteHook(context.Background(), HookPreValidate, ExecutionContext{
        DeploymentID: "deploy-1",
        ServiceName:  "web",
    })

    assert.NoError(t, err)
    assert.True(t, results.AllPassed)
    assert.Len(t, results.Results, 1)
}

func TestWebhookRunner_Run(t *testing.T) {
    // Mock HTTP server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(WebhookResponse{
            Success: true,
            Message: "hook executed",
        })
    }))
    defer server.Close()

    runner := NewWebhookRunner(10 * time.Second)
    plugin := &Plugin{
        Name: "test-webhook",
        Type: PluginTypeWebhook,
        Spec: PluginSpec{
            WebhookURL: server.URL,
        },
    }

    result, err := runner.Run(context.Background(), plugin, ExecutionContext{
        Hook:        HookPostDeploy,
        ServiceName: "web",
    })

    assert.NoError(t, err)
    assert.True(t, result.Success)
}
```

## Related Documents

- [Orchestrator](./orchestrator.md) - Invokes plugins during deployment
- [State Manager](./state-manager.md) - May trigger on-failure hooks
