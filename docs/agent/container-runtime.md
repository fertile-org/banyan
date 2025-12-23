# Container Runtime - Detailed Design

## Overview

The Container Runtime is the Agent component responsible for managing the complete container lifecycle on a node. It abstracts the underlying container runtime (Docker, containerd) and provides a unified interface for creating, starting, stopping, and removing containers.

## Responsibilities

1. **Container Lifecycle Management** - Create, start, stop, restart, remove containers
2. **Image Management** - Pull, list, and manage container images
3. **Container Configuration** - Apply resource limits, environment variables, mounts
4. **Network Integration** - Coordinate with Network Node for container networking
5. **Log Management** - Stream and collect container logs
6. **Resource Monitoring** - Report container resource usage

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                           Container Runtime                               │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐ │
│  │                       Driving Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Task Handler    │  │  gRPC Handler    │  │  Event Handler    │ │ │
│  │  │  (from Executor) │  │  (direct calls)  │  │  (Docker events)  │ │ │
│  │  └────────┬─────────┘  └────────┬─────────┘  └─────────┬─────────┘ │ │
│  └───────────┼─────────────────────┼─────────────────────┼───────────┘ │
│              │                     │                     │              │
│              └─────────────────────┴─────────────────────┘              │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Inbound Ports                                 │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                    ContainerService                          │   │ │
│  │  │  - CreateContainer(spec) -> ContainerInfo                   │   │ │
│  │  │  - StartContainer(id) -> error                              │   │ │
│  │  │  - StopContainer(id, timeout) -> error                      │   │ │
│  │  │  - RemoveContainer(id, force) -> error                      │   │ │
│  │  │  - GetContainer(id) -> ContainerInfo                        │   │ │
│  │  │  - ListContainers(filter) -> []ContainerInfo                │   │ │
│  │  │  - StreamLogs(id, opts) -> LogStream                        │   │ │
│  │  │  - GetStats(id) -> ContainerStats                           │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  │  ┌─────────────────────────────────────────────────────────────┐   │ │
│  │  │                     ImageService                             │   │ │
│  │  │  - PullImage(ref) -> ImageInfo                              │   │ │
│  │  │  - ListImages() -> []ImageInfo                              │   │ │
│  │  │  - RemoveImage(id) -> error                                 │   │ │
│  │  │  - InspectImage(id) -> ImageInfo                            │   │ │
│  │  └─────────────────────────────────────────────────────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                          Use Cases                                   │ │
│  │  ┌─────────────────┐ ┌─────────────────┐ ┌───────────────────────┐ │ │
│  │  │ ContainerUseCase│ │  ImageUseCase   │ │  LifecycleUseCase     │ │ │
│  │  │                 │ │                 │ │                       │ │ │
│  │  │ - Create        │ │ - Pull          │ │ - Start/Stop          │ │ │
│  │  │ - Configure     │ │ - List          │ │ - Restart             │ │ │
│  │  │ - Validate      │ │ - Remove        │ │ - Health Check        │ │ │
│  │  └─────────────────┘ └─────────────────┘ └───────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                         Domain Layer                                 │ │
│  │  ┌────────────────────────────────────────────────────────────────┐ │ │
│  │  │  Entities: Container, Image, ContainerConfig, ResourceLimits  │ │ │
│  │  │  Value Objects: ContainerID, ImageRef, MountPoint, Port       │ │ │
│  │  │  Domain Logic: Validation, State Transitions                  │ │ │
│  │  └────────────────────────────────────────────────────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Outbound Ports                                │ │
│  │  ┌───────────────────┐ ┌───────────────────┐ ┌───────────────────┐ │ │
│  │  │ ContainerRuntime  │ │  ImageRegistry    │ │ NetworkConnector  │ │ │
│  │  │ (Docker/containerd)│ │  (pull/push)     │ │ (CNI setup)       │ │ │
│  │  └───────────────────┘ └───────────────────┘ └───────────────────┘ │ │
│  │  ┌───────────────────┐ ┌───────────────────┐                       │ │
│  │  │ VolumeManager     │ │ MetricsCollector  │                       │ │
│  │  │ (mounts)          │ │ (stats)           │                       │ │
│  │  └───────────────────┘ └───────────────────┘                       │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                    │                                     │
│  ┌─────────────────────────────────▼───────────────────────────────────┐ │
│  │                        Driven Adapters                               │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌───────────────────┐ │ │
│  │  │  Docker Client   │  │ Containerd Client│  │   CNI Executor    │ │ │
│  │  │  (docker SDK)    │  │ (containerd SDK) │  │   (CNI plugins)   │ │ │
│  │  └──────────────────┘  └──────────────────┘  └───────────────────┘ │ │
│  └─────────────────────────────────────────────────────────────────────┘ │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Domain Layer

### Entities

```go
// Container represents a running or stopped container on the node
type Container struct {
    ID          ContainerID
    Name        string
    ServiceID   string
    ImageRef    ImageRef
    Config      ContainerConfig
    State       ContainerState
    NetworkInfo NetworkInfo
    CreatedAt   time.Time
    StartedAt   *time.Time
    FinishedAt  *time.Time
    ExitCode    *int
}

// Image represents a container image
type Image struct {
    ID        ImageID
    Ref       ImageRef
    Size      int64
    Labels    map[string]string
    CreatedAt time.Time
}

// ContainerConfig contains all configuration for creating a container
type ContainerConfig struct {
    Image         ImageRef
    Command       []string
    Entrypoint    []string
    Env           []EnvVar
    Mounts        []MountPoint
    Ports         []PortBinding
    Resources     ResourceLimits
    Labels        map[string]string
    NetworkMode   NetworkMode
    RestartPolicy RestartPolicy
    HealthCheck   *HealthCheckConfig
}

// ResourceLimits defines resource constraints for a container
type ResourceLimits struct {
    CPUShares     int64   // CPU shares (relative weight)
    CPUQuota      int64   // CPU CFS quota (microseconds per period)
    CPUPeriod     int64   // CPU CFS period (microseconds)
    MemoryLimit   int64   // Memory limit in bytes
    MemorySwap    int64   // Memory + swap limit (-1 for unlimited)
    PidsLimit     int64   // Process limit
    OOMKillDisable bool   // Disable OOM killer
}
```

### Value Objects

```go
// ContainerID is a unique identifier for a container
type ContainerID string

func NewContainerID(id string) (ContainerID, error) {
    if len(id) < 12 {
        return "", ErrInvalidContainerID
    }
    return ContainerID(id), nil
}

// ImageRef represents a container image reference
type ImageRef struct {
    Registry string // e.g., "docker.io"
    Name     string // e.g., "library/nginx"
    Tag      string // e.g., "latest" or "1.21"
    Digest   string // e.g., "sha256:..."
}

func (r ImageRef) String() string {
    ref := r.Registry + "/" + r.Name
    if r.Digest != "" {
        return ref + "@" + r.Digest
    }
    return ref + ":" + r.Tag
}

// ContainerState represents the current state of a container
type ContainerState string

const (
    ContainerStateCreated    ContainerState = "created"
    ContainerStateRunning    ContainerState = "running"
    ContainerStatePaused     ContainerState = "paused"
    ContainerStateRestarting ContainerState = "restarting"
    ContainerStateExited     ContainerState = "exited"
    ContainerStateDead       ContainerState = "dead"
)

// MountPoint defines a volume mount
type MountPoint struct {
    Source      string     // Host path or volume name
    Destination string     // Container path
    Type        MountType  // bind, volume, tmpfs
    ReadOnly    bool
    Propagation string     // rprivate, rshared, rslave
}

// PortBinding maps container port to host
type PortBinding struct {
    ContainerPort uint16
    HostPort      uint16
    Protocol      Protocol // tcp, udp
    HostIP        string   // Optional bind IP
}
```

### Domain Logic

```go
// Container state machine
func (c *Container) CanTransitionTo(newState ContainerState) bool {
    validTransitions := map[ContainerState][]ContainerState{
        ContainerStateCreated:    {ContainerStateRunning, ContainerStateDead},
        ContainerStateRunning:    {ContainerStatePaused, ContainerStateRestarting, ContainerStateExited, ContainerStateDead},
        ContainerStatePaused:     {ContainerStateRunning, ContainerStateDead},
        ContainerStateRestarting: {ContainerStateRunning, ContainerStateExited, ContainerStateDead},
        ContainerStateExited:     {ContainerStateRunning, ContainerStateDead},
        ContainerStateDead:       {}, // Terminal state
    }

    allowed, exists := validTransitions[c.State]
    if !exists {
        return false
    }

    for _, s := range allowed {
        if s == newState {
            return true
        }
    }
    return false
}

// Validate container configuration
func (c *ContainerConfig) Validate() error {
    if c.Image.Name == "" {
        return ErrImageRequired
    }

    if c.Resources.MemoryLimit < 0 {
        return ErrInvalidMemoryLimit
    }

    if c.Resources.CPUQuota < 0 || c.Resources.CPUPeriod < 0 {
        return ErrInvalidCPULimit
    }

    for _, mount := range c.Mounts {
        if mount.Destination == "" {
            return ErrInvalidMountDestination
        }
    }

    return nil
}
```

## Inbound Ports

### ContainerService

```go
// ContainerService defines the main interface for container operations
type ContainerService interface {
    // Container lifecycle
    CreateContainer(ctx context.Context, spec ContainerSpec) (*ContainerInfo, error)
    StartContainer(ctx context.Context, id ContainerID) error
    StopContainer(ctx context.Context, id ContainerID, timeout time.Duration) error
    RestartContainer(ctx context.Context, id ContainerID, timeout time.Duration) error
    RemoveContainer(ctx context.Context, id ContainerID, force bool) error

    // Container inspection
    GetContainer(ctx context.Context, id ContainerID) (*ContainerInfo, error)
    ListContainers(ctx context.Context, filter ContainerFilter) ([]*ContainerInfo, error)

    // Container operations
    ExecInContainer(ctx context.Context, id ContainerID, cmd ExecConfig) (*ExecResult, error)
    CopyToContainer(ctx context.Context, id ContainerID, path string, content io.Reader) error
    CopyFromContainer(ctx context.Context, id ContainerID, path string) (io.ReadCloser, error)

    // Logs and stats
    StreamLogs(ctx context.Context, id ContainerID, opts LogOptions) (LogStream, error)
    GetStats(ctx context.Context, id ContainerID) (*ContainerStats, error)
    StreamStats(ctx context.Context, id ContainerID) (StatsStream, error)
}

// ContainerSpec is the input for creating a container
type ContainerSpec struct {
    Name        string
    ServiceID   string
    Config      ContainerConfig
    NetworkID   string
    IPAddress   string  // Pre-allocated IP from IPAM
}

// ContainerInfo is the output containing container details
type ContainerInfo struct {
    ID          ContainerID
    Name        string
    ServiceID   string
    State       ContainerState
    Health      HealthStatus
    IPAddress   string
    Ports       []PortBinding
    CreatedAt   time.Time
    StartedAt   *time.Time
}

// ContainerFilter for listing containers
type ContainerFilter struct {
    ServiceID string
    States    []ContainerState
    Labels    map[string]string
    All       bool // Include stopped containers
}
```

### ImageService

```go
// ImageService defines the interface for image operations
type ImageService interface {
    // Image operations
    PullImage(ctx context.Context, ref ImageRef, opts PullOptions) (*ImageInfo, error)
    ListImages(ctx context.Context) ([]*ImageInfo, error)
    RemoveImage(ctx context.Context, id ImageID, force bool) error
    InspectImage(ctx context.Context, id ImageID) (*ImageInfo, error)

    // Image existence
    ImageExists(ctx context.Context, ref ImageRef) (bool, error)
}

// PullOptions for image pull operation
type PullOptions struct {
    AuthConfig *AuthConfig  // Registry authentication
    Platform   string       // Target platform (e.g., "linux/amd64")
    Progress   ProgressFunc // Progress callback
}

// ImageInfo contains image details
type ImageInfo struct {
    ID        ImageID
    Ref       ImageRef
    Size      int64
    Created   time.Time
    Labels    map[string]string
    Digest    string
}
```

## Outbound Ports

### ContainerRuntime (Primary)

```go
// ContainerRuntime abstracts the underlying container runtime
type ContainerRuntime interface {
    // Container operations
    Create(ctx context.Context, config *RuntimeContainerConfig) (string, error)
    Start(ctx context.Context, containerID string) error
    Stop(ctx context.Context, containerID string, timeout time.Duration) error
    Remove(ctx context.Context, containerID string, force bool) error

    // Container inspection
    Inspect(ctx context.Context, containerID string) (*RuntimeContainerInfo, error)
    List(ctx context.Context, opts ListOptions) ([]*RuntimeContainerInfo, error)

    // Container operations
    Logs(ctx context.Context, containerID string, opts LogOptions) (io.ReadCloser, error)
    Stats(ctx context.Context, containerID string) (*RuntimeStats, error)
    Wait(ctx context.Context, containerID string) (<-chan WaitResult, error)

    // Exec
    ExecCreate(ctx context.Context, containerID string, config ExecConfig) (string, error)
    ExecStart(ctx context.Context, execID string) (io.ReadWriteCloser, error)
    ExecInspect(ctx context.Context, execID string) (*ExecInspect, error)
}
```

### ImageRegistry

```go
// ImageRegistry handles image pull/push operations
type ImageRegistry interface {
    Pull(ctx context.Context, ref string, opts PullOptions) error
    Push(ctx context.Context, ref string, opts PushOptions) error
    List(ctx context.Context) ([]*RuntimeImageInfo, error)
    Remove(ctx context.Context, imageID string, force bool) error
    Inspect(ctx context.Context, imageID string) (*RuntimeImageInfo, error)
}
```

### NetworkConnector

```go
// NetworkConnector interfaces with the Network Node for container networking
type NetworkConnector interface {
    // Setup container network
    ConnectContainer(ctx context.Context, containerID string, networkID string, ip string) error
    DisconnectContainer(ctx context.Context, containerID string, networkID string) error

    // Get container network info
    GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetworkInfo, error)
}
```

## Use Cases

### CreateContainerUseCase

```go
type CreateContainerUseCase struct {
    runtime    ContainerRuntime
    registry   ImageRegistry
    network    NetworkConnector
    validator  ConfigValidator
}

func (uc *CreateContainerUseCase) Execute(ctx context.Context, spec ContainerSpec) (*ContainerInfo, error) {
    // 1. Validate configuration
    if err := uc.validator.Validate(spec.Config); err != nil {
        return nil, fmt.Errorf("invalid container config: %w", err)
    }

    // 2. Ensure image exists locally
    imageExists, err := uc.registry.ImageExists(ctx, spec.Config.Image.String())
    if err != nil {
        return nil, fmt.Errorf("failed to check image: %w", err)
    }

    if !imageExists {
        if err := uc.registry.Pull(ctx, spec.Config.Image.String(), PullOptions{}); err != nil {
            return nil, fmt.Errorf("failed to pull image: %w", err)
        }
    }

    // 3. Convert to runtime config
    runtimeConfig := uc.toRuntimeConfig(spec)

    // 4. Create container
    containerID, err := uc.runtime.Create(ctx, runtimeConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create container: %w", err)
    }

    // 5. Connect to network if specified
    if spec.NetworkID != "" {
        if err := uc.network.ConnectContainer(ctx, containerID, spec.NetworkID, spec.IPAddress); err != nil {
            // Cleanup container on network failure
            _ = uc.runtime.Remove(ctx, containerID, true)
            return nil, fmt.Errorf("failed to connect container to network: %w", err)
        }
    }

    // 6. Get container info
    info, err := uc.runtime.Inspect(ctx, containerID)
    if err != nil {
        return nil, fmt.Errorf("failed to inspect container: %w", err)
    }

    return uc.toContainerInfo(info, spec), nil
}

func (uc *CreateContainerUseCase) toRuntimeConfig(spec ContainerSpec) *RuntimeContainerConfig {
    return &RuntimeContainerConfig{
        Name:    spec.Name,
        Image:   spec.Config.Image.String(),
        Cmd:     spec.Config.Command,
        Env:     uc.envToStrings(spec.Config.Env),
        Mounts:  uc.toRuntimeMounts(spec.Config.Mounts),
        Labels:  uc.addServiceLabels(spec.Config.Labels, spec.ServiceID),
        Resources: &RuntimeResources{
            CPUShares:   spec.Config.Resources.CPUShares,
            CPUQuota:    spec.Config.Resources.CPUQuota,
            CPUPeriod:   spec.Config.Resources.CPUPeriod,
            Memory:      spec.Config.Resources.MemoryLimit,
            MemorySwap:  spec.Config.Resources.MemorySwap,
            PidsLimit:   spec.Config.Resources.PidsLimit,
        },
    }
}
```

### LifecycleUseCase

```go
type LifecycleUseCase struct {
    runtime ContainerRuntime
    network NetworkConnector
    events  EventEmitter
}

func (uc *LifecycleUseCase) StartContainer(ctx context.Context, id ContainerID) error {
    // 1. Get current state
    info, err := uc.runtime.Inspect(ctx, string(id))
    if err != nil {
        return fmt.Errorf("container not found: %w", err)
    }

    // 2. Validate state transition
    if info.State == "running" {
        return nil // Already running
    }

    if info.State != "created" && info.State != "exited" {
        return fmt.Errorf("cannot start container in state: %s", info.State)
    }

    // 3. Start container
    if err := uc.runtime.Start(ctx, string(id)); err != nil {
        return fmt.Errorf("failed to start container: %w", err)
    }

    // 4. Emit event
    uc.events.Emit(ContainerStartedEvent{
        ContainerID: string(id),
        StartedAt:   time.Now(),
    })

    return nil
}

func (uc *LifecycleUseCase) StopContainer(ctx context.Context, id ContainerID, timeout time.Duration) error {
    // 1. Get current state
    info, err := uc.runtime.Inspect(ctx, string(id))
    if err != nil {
        return fmt.Errorf("container not found: %w", err)
    }

    // 2. Skip if already stopped
    if info.State == "exited" || info.State == "dead" {
        return nil
    }

    // 3. Stop with timeout
    if err := uc.runtime.Stop(ctx, string(id), timeout); err != nil {
        return fmt.Errorf("failed to stop container: %w", err)
    }

    // 4. Emit event
    uc.events.Emit(ContainerStoppedEvent{
        ContainerID: string(id),
        StoppedAt:   time.Now(),
    })

    return nil
}

func (uc *LifecycleUseCase) RemoveContainer(ctx context.Context, id ContainerID, force bool) error {
    // 1. Get current state
    info, err := uc.runtime.Inspect(ctx, string(id))
    if err != nil {
        if IsNotFoundError(err) {
            return nil // Already removed
        }
        return fmt.Errorf("failed to inspect container: %w", err)
    }

    // 2. Stop if running and force is true
    if info.State == "running" {
        if !force {
            return ErrContainerRunning
        }
        if err := uc.runtime.Stop(ctx, string(id), 10*time.Second); err != nil {
            return fmt.Errorf("failed to stop container: %w", err)
        }
    }

    // 3. Disconnect from network
    if info.NetworkSettings != nil && info.NetworkSettings.NetworkID != "" {
        if err := uc.network.DisconnectContainer(ctx, string(id), info.NetworkSettings.NetworkID); err != nil {
            // Log but continue with removal
            log.Printf("Warning: failed to disconnect container from network: %v", err)
        }
    }

    // 4. Remove container
    if err := uc.runtime.Remove(ctx, string(id), force); err != nil {
        return fmt.Errorf("failed to remove container: %w", err)
    }

    // 5. Emit event
    uc.events.Emit(ContainerRemovedEvent{
        ContainerID: string(id),
        RemovedAt:   time.Now(),
    })

    return nil
}
```

## Driven Adapters

### DockerClient

```go
type DockerClientAdapter struct {
    client *client.Client
    ctx    context.Context
}

func NewDockerClientAdapter(ctx context.Context) (*DockerClientAdapter, error) {
    cli, err := client.NewClientWithOpts(
        client.FromEnv,
        client.WithAPIVersionNegotiation(),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create Docker client: %w", err)
    }

    // Verify connection
    if _, err := cli.Ping(ctx); err != nil {
        return nil, fmt.Errorf("failed to connect to Docker: %w", err)
    }

    return &DockerClientAdapter{
        client: cli,
        ctx:    ctx,
    }, nil
}

func (d *DockerClientAdapter) Create(ctx context.Context, config *RuntimeContainerConfig) (string, error) {
    // Convert to Docker types
    containerConfig := &container.Config{
        Image:      config.Image,
        Cmd:        config.Cmd,
        Env:        config.Env,
        Labels:     config.Labels,
        Hostname:   config.Hostname,
        WorkingDir: config.WorkingDir,
    }

    hostConfig := &container.HostConfig{
        Binds:       d.toBinds(config.Mounts),
        PortBindings: d.toPortBindings(config.Ports),
        Resources: container.Resources{
            CPUShares:  config.Resources.CPUShares,
            CPUQuota:   config.Resources.CPUQuota,
            CPUPeriod:  config.Resources.CPUPeriod,
            Memory:     config.Resources.Memory,
            MemorySwap: config.Resources.MemorySwap,
            PidsLimit:  &config.Resources.PidsLimit,
        },
        RestartPolicy: d.toRestartPolicy(config.RestartPolicy),
    }

    networkConfig := &network.NetworkingConfig{}

    resp, err := d.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, config.Name)
    if err != nil {
        return "", err
    }

    return resp.ID, nil
}

func (d *DockerClientAdapter) Start(ctx context.Context, containerID string) error {
    return d.client.ContainerStart(ctx, containerID, container.StartOptions{})
}

func (d *DockerClientAdapter) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
    timeoutSec := int(timeout.Seconds())
    return d.client.ContainerStop(ctx, containerID, container.StopOptions{
        Timeout: &timeoutSec,
    })
}

func (d *DockerClientAdapter) Inspect(ctx context.Context, containerID string) (*RuntimeContainerInfo, error) {
    info, err := d.client.ContainerInspect(ctx, containerID)
    if err != nil {
        if client.IsErrNotFound(err) {
            return nil, ErrContainerNotFound
        }
        return nil, err
    }

    return &RuntimeContainerInfo{
        ID:      info.ID,
        Name:    strings.TrimPrefix(info.Name, "/"),
        Image:   info.Image,
        State:   info.State.Status,
        Created: info.Created,
        Started: info.State.StartedAt,
        NetworkSettings: &RuntimeNetworkSettings{
            IPAddress: info.NetworkSettings.IPAddress,
            Ports:     d.fromPortMap(info.NetworkSettings.Ports),
        },
    }, nil
}

func (d *DockerClientAdapter) Logs(ctx context.Context, containerID string, opts LogOptions) (io.ReadCloser, error) {
    return d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: opts.Stdout,
        ShowStderr: opts.Stderr,
        Since:      opts.Since,
        Until:      opts.Until,
        Timestamps: opts.Timestamps,
        Follow:     opts.Follow,
        Tail:       opts.Tail,
    })
}

func (d *DockerClientAdapter) Stats(ctx context.Context, containerID string) (*RuntimeStats, error) {
    statsResp, err := d.client.ContainerStats(ctx, containerID, false)
    if err != nil {
        return nil, err
    }
    defer statsResp.Body.Close()

    var stats types.StatsJSON
    if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
        return nil, err
    }

    return &RuntimeStats{
        CPUPercent:    calculateCPUPercent(&stats),
        MemoryUsage:   stats.MemoryStats.Usage,
        MemoryLimit:   stats.MemoryStats.Limit,
        NetworkRxBytes: stats.Networks["eth0"].RxBytes,
        NetworkTxBytes: stats.Networks["eth0"].TxBytes,
        BlockRead:     calculateBlockIO(&stats, true),
        BlockWrite:    calculateBlockIO(&stats, false),
        PIDs:          stats.PidsStats.Current,
    }, nil
}
```

## Container Creation Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Container Creation Flow                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Task Executor                                                           │
│       │                                                                  │
│       │ 1. CreateContainer(spec)                                        │
│       ▼                                                                  │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │                   Container Service                              │    │
│  │                                                                  │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Validate   │ 2. Validate spec                              │    │
│  │  │    Config    │    - Image ref format                         │    │
│  │  └──────┬───────┘    - Resource limits                          │    │
│  │         │            - Mount paths                               │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │  Check/Pull  │ 3. Ensure image available                     │    │
│  │  │    Image     │    - Check local cache                        │    │
│  │  └──────┬───────┘    - Pull if needed                           │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Create     │ 4. Create container via runtime               │    │
│  │  │  Container   │    - Set labels, env, mounts                  │    │
│  │  └──────┬───────┘    - Configure resources                      │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Connect    │ 5. Setup networking                           │    │
│  │  │   Network    │    - Connect to VPC network                   │    │
│  │  └──────┬───────┘    - Assign pre-allocated IP                  │    │
│  │         │                                                        │    │
│  │         ▼                                                        │    │
│  │  ┌──────────────┐                                               │    │
│  │  │   Return     │ 6. Return container info                      │    │
│  │  │    Info      │    - Container ID, state, IP                  │    │
│  │  └──────────────┘                                               │    │
│  │                                                                  │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Error Handling

```go
// Domain errors
var (
    ErrContainerNotFound    = errors.New("container not found")
    ErrContainerRunning     = errors.New("container is running")
    ErrImageNotFound        = errors.New("image not found")
    ErrImagePullFailed      = errors.New("failed to pull image")
    ErrInvalidContainerID   = errors.New("invalid container ID")
    ErrInvalidConfig        = errors.New("invalid container configuration")
    ErrInvalidMemoryLimit   = errors.New("invalid memory limit")
    ErrInvalidCPULimit      = errors.New("invalid CPU limit")
    ErrInvalidMountDestination = errors.New("invalid mount destination")
    ErrNetworkConnectionFailed = errors.New("failed to connect container to network")
)

// Error classification
func IsRetryableError(err error) bool {
    // Network errors are typically retryable
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }

    // Docker daemon temporary errors
    if strings.Contains(err.Error(), "connection refused") {
        return true
    }

    return false
}

func IsNotFoundError(err error) bool {
    return errors.Is(err, ErrContainerNotFound) ||
           errors.Is(err, ErrImageNotFound)
}
```

## Testing Strategy

```go
// Unit test with mock runtime
func TestCreateContainerUseCase(t *testing.T) {
    mockRuntime := &MockContainerRuntime{}
    mockRegistry := &MockImageRegistry{}
    mockNetwork := &MockNetworkConnector{}

    uc := &CreateContainerUseCase{
        runtime:  mockRuntime,
        registry: mockRegistry,
        network:  mockNetwork,
        validator: NewConfigValidator(),
    }

    spec := ContainerSpec{
        Name:      "test-container",
        ServiceID: "svc-123",
        Config: ContainerConfig{
            Image: ImageRef{
                Registry: "docker.io",
                Name:     "library/nginx",
                Tag:      "latest",
            },
            Resources: ResourceLimits{
                MemoryLimit: 512 * 1024 * 1024, // 512MB
                CPUShares:   1024,
            },
        },
        NetworkID: "net-123",
        IPAddress: "10.0.1.5",
    }

    // Setup mocks
    mockRegistry.On("ImageExists", mock.Anything, "docker.io/library/nginx:latest").Return(true, nil)
    mockRuntime.On("Create", mock.Anything, mock.Anything).Return("container-123", nil)
    mockNetwork.On("ConnectContainer", mock.Anything, "container-123", "net-123", "10.0.1.5").Return(nil)
    mockRuntime.On("Inspect", mock.Anything, "container-123").Return(&RuntimeContainerInfo{
        ID:    "container-123",
        Name:  "test-container",
        State: "created",
    }, nil)

    // Execute
    info, err := uc.Execute(context.Background(), spec)

    // Assert
    assert.NoError(t, err)
    assert.Equal(t, ContainerID("container-123"), info.ID)
    assert.Equal(t, "test-container", info.Name)
    mockRegistry.AssertExpectations(t)
    mockRuntime.AssertExpectations(t)
    mockNetwork.AssertExpectations(t)
}

// Integration test with real Docker
func TestDockerClientIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    ctx := context.Background()
    client, err := NewDockerClientAdapter(ctx)
    require.NoError(t, err)

    // Create test container
    containerID, err := client.Create(ctx, &RuntimeContainerConfig{
        Name:  "test-integration-" + uuid.New().String()[:8],
        Image: "alpine:latest",
        Cmd:   []string{"sleep", "30"},
    })
    require.NoError(t, err)

    // Cleanup
    defer func() {
        _ = client.Stop(ctx, containerID, 5*time.Second)
        _ = client.Remove(ctx, containerID, true)
    }()

    // Start container
    err = client.Start(ctx, containerID)
    require.NoError(t, err)

    // Verify running
    info, err := client.Inspect(ctx, containerID)
    require.NoError(t, err)
    assert.Equal(t, "running", info.State)
}
```

## Related Documents

- [Task Executor](./task-executor.md) - Dispatches tasks to Container Runtime
- [Network Node](./network-node.md) - Manages container networking
- [Health Monitor](./health-monitor.md) - Monitors container health
