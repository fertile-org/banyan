package adapters

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

const (
	defaultNamespace = "banyan"
	defaultSocket    = "/run/containerd/containerd.sock"
)

// ContainerdRuntime implements outbound.ContainerRuntime using containerd.
type ContainerdRuntime struct {
	client      *containerd.Client
	namespace   string
	snapshotter string
}

// ContainerdRuntimeConfig contains configuration for the containerd runtime.
type ContainerdRuntimeConfig struct {
	Address     string
	Namespace   string
	Snapshotter string // e.g., "native", "overlayfs" - default is "native" for DinD compatibility
}

// NewContainerdRuntime creates a new containerd runtime adapter.
func NewContainerdRuntime(cfg ContainerdRuntimeConfig) (*ContainerdRuntime, error) {
	address := cfg.Address
	if address == "" {
		address = defaultSocket
	}

	namespace := cfg.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}

	snapshotter := cfg.Snapshotter
	if snapshotter == "" {
		snapshotter = "native" // Default to native for DinD compatibility
	}

	client, err := containerd.New(address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}

	return &ContainerdRuntime{
		client:      client,
		namespace:   namespace,
		snapshotter: snapshotter,
	}, nil
}

// Close closes the containerd client connection.
func (r *ContainerdRuntime) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Client returns the underlying containerd client.
// Useful for creating related adapters like ImageRegistry.
func (r *ContainerdRuntime) Client() *containerd.Client {
	return r.client
}

// Namespace returns the containerd namespace.
func (r *ContainerdRuntime) Namespace() string {
	return r.namespace
}

// Create creates a new container.
func (r *ContainerdRuntime) Create(ctx context.Context, config *outbound.RuntimeContainerConfig) (string, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	// Get or pull the image
	image, err := r.client.GetImage(ctx, config.Image)
	if err != nil {
		return "", fmt.Errorf("failed to get image %s: %w", config.Image, err)
	}

	// Build container spec
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
	}

	if len(config.Cmd) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(config.Cmd...))
	}

	if config.WorkingDir != "" {
		specOpts = append(specOpts, oci.WithProcessCwd(config.WorkingDir))
	}

	if len(config.Env) > 0 {
		specOpts = append(specOpts, oci.WithEnv(config.Env))
	}

	if config.User != "" {
		specOpts = append(specOpts, oci.WithUsername(config.User))
	}

	if config.Hostname != "" {
		specOpts = append(specOpts, oci.WithHostname(config.Hostname))
	}

	// Apply resource limits
	if config.Resources != nil {
		specOpts = append(specOpts, r.withResources(config.Resources))
	}

	// Apply mounts
	for _, mount := range config.Mounts {
		specOpts = append(specOpts, oci.WithMounts([]specs.Mount{
			{
				Source:      mount.Source,
				Destination: mount.Target,
				Type:        mount.Type,
				Options:     r.getMountOptions(mount),
			},
		}))
	}

	// Apply host config options
	if config.HostConfig != nil {
		specOpts = append(specOpts, r.withHostConfig(config.HostConfig))
	}

	// Create container options with configured snapshotter
	containerOpts := []containerd.NewContainerOpts{
		containerd.WithImage(image),
		containerd.WithSnapshotter(r.snapshotter),
		containerd.WithNewSnapshot(config.Name+"-snapshot", image),
		containerd.WithNewSpec(specOpts...),
	}

	if len(config.Labels) > 0 {
		containerOpts = append(containerOpts, containerd.WithContainerLabels(config.Labels))
	}

	// Create the container
	container, err := r.client.NewContainer(ctx, config.Name, containerOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	return container.ID(), nil
}

// Start starts a container.
func (r *ContainerdRuntime) Start(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Create task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// Start the task
	if err := task.Start(ctx); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	return nil
}

// Stop stops a container.
func (r *ContainerdRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil // No task, already stopped
		}
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Create context with timeout for graceful shutdown
	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Start a fresh wait channel (not tied to timeout context)
	exitCh, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for task: %w", err)
	}

	// Send SIGTERM first for graceful shutdown (ignore errors if task already gone)
	_ = task.Kill(stopCtx, syscall.SIGTERM)

	select {
	case <-exitCh:
		// Task exited gracefully after SIGTERM
	case <-stopCtx.Done():
		// Timeout - force kill with SIGKILL (ignore errors if task already gone)
		_ = task.Kill(ctx, syscall.SIGKILL)
		// Wait a moment for SIGKILL to take effect
		select {
		case <-exitCh:
			// Task exited after SIGKILL
		case <-time.After(3 * time.Second):
			// Proceed to delete anyway (will use WithProcessKill)
		}
	}

	// Delete task with force kill option as backup
	if _, err := task.Delete(ctx, containerd.WithProcessKill); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("failed to delete task: %w", err)
		}
	}

	return nil
}

// Remove removes a container.
func (r *ContainerdRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to load container: %w", err)
	}

	// Try to stop task if exists
	if task, err := container.Task(ctx, nil); err == nil {
		if force {
			_ = task.Kill(ctx, syscall.SIGKILL)
		}
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	// Delete container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return nil
}

// Kill sends a signal to a container.
func (r *ContainerdRuntime) Kill(ctx context.Context, containerID, signal string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	sig := parseSignal(signal)
	if err := task.Kill(ctx, sig); err != nil {
		return fmt.Errorf("failed to send signal: %w", err)
	}

	return nil
}

// Pause pauses a container.
func (r *ContainerdRuntime) Pause(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if err := task.Pause(ctx); err != nil {
		return fmt.Errorf("failed to pause task: %w", err)
	}

	return nil
}

// Unpause unpauses a container.
func (r *ContainerdRuntime) Unpause(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	if err := task.Resume(ctx); err != nil {
		return fmt.Errorf("failed to resume task: %w", err)
	}

	return nil
}

// Inspect inspects a container.
func (r *ContainerdRuntime) Inspect(ctx context.Context, containerID string) (*outbound.RuntimeContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	info, err := container.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info: %w", err)
	}

	state := "created"
	status := "Created"

	// Check if task exists and get its status
	if task, err := container.Task(ctx, nil); err == nil {
		taskStatus, err := task.Status(ctx)
		if err == nil {
			state = string(taskStatus.Status)
			status = string(taskStatus.Status)
		}
	}

	return &outbound.RuntimeContainerInfo{
		ID:      info.ID,
		Name:    info.ID,
		Image:   info.Image,
		Created: info.CreatedAt.Format(time.RFC3339),
		State:   state,
		Status:  status,
	}, nil
}

// List lists containers.
func (r *ContainerdRuntime) List(ctx context.Context, opts outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	var filters []string
	for key, values := range opts.Filters {
		for _, value := range values {
			filters = append(filters, fmt.Sprintf("labels.%s==%s", key, value))
		}
	}

	containerList, err := r.client.Containers(ctx, filters...)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]*outbound.RuntimeContainerInfo, 0, len(containerList))
	for _, container := range containerList {
		info, err := r.Inspect(ctx, container.ID())
		if err != nil {
			continue
		}
		result = append(result, info)
	}

	return result, nil
}

// Logs retrieves container logs.
func (r *ContainerdRuntime) Logs(_ context.Context, _ string, _ outbound.RuntimeLogOptions) (io.ReadCloser, error) {
	// Containerd doesn't provide native log streaming like Docker
	// Logs are typically handled by the runtime (runc) and stored on the filesystem
	return nil, fmt.Errorf("logs not implemented for containerd adapter")
}

// Stats retrieves container statistics.
func (r *ContainerdRuntime) Stats(ctx context.Context, containerID string) (*outbound.RuntimeStats, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	metrics, err := task.Metrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metrics: %w", err)
	}

	// Parse metrics - this is simplified; actual implementation would decode protobuf
	_ = metrics

	return &outbound.RuntimeStats{
		CPUPercent:  0,
		MemoryUsage: 0,
		MemoryLimit: 0,
	}, nil
}

// Wait waits for a container to exit.
func (r *ContainerdRuntime) Wait(ctx context.Context, containerID string) (<-chan outbound.WaitResult, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	exitCh, err := task.Wait(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for task: %w", err)
	}

	resultCh := make(chan outbound.WaitResult, 1)
	go func() {
		defer close(resultCh)
		exitStatus := <-exitCh
		result := outbound.WaitResult{
			StatusCode: int64(exitStatus.ExitCode()),
		}
		if exitStatus.Error() != nil {
			result.Error = &outbound.WaitError{
				Message: exitStatus.Error().Error(),
			}
		}
		resultCh <- result
	}()

	return resultCh, nil
}

// ExecCreate creates an exec instance.
func (r *ContainerdRuntime) ExecCreate(ctx context.Context, containerID string, config *outbound.RuntimeExecConfig) (string, error) {
	// Containerd handles exec differently - we'll return the container ID
	// and handle exec in ExecStart
	_ = ctx
	_ = containerID
	_ = config
	return containerID + "-exec", nil
}

// ExecStart starts an exec instance.
func (r *ContainerdRuntime) ExecStart(ctx context.Context, execID string, opts outbound.ExecStartOptions) (io.ReadWriteCloser, error) {
	_ = ctx
	_ = execID
	_ = opts
	return nil, fmt.Errorf("exec not fully implemented for containerd adapter")
}

// ExecInspect inspects an exec instance.
func (r *ContainerdRuntime) ExecInspect(_ context.Context, execID string) (*outbound.ExecInspect, error) {
	return &outbound.ExecInspect{
		ExecID:   execID,
		ExitCode: 0,
		Running:  false,
	}, nil
}

// CopyToContainer copies content to a container.
func (r *ContainerdRuntime) CopyToContainer(_ context.Context, _, _ string, _ io.Reader) error {
	return fmt.Errorf("copy to container not implemented for containerd adapter")
}

// CopyFromContainer copies content from a container.
func (r *ContainerdRuntime) CopyFromContainer(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("copy from container not implemented for containerd adapter")
}

// withResources creates resource limit options.
func (r *ContainerdRuntime) withResources(res *outbound.RuntimeResources) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		if s.Linux == nil {
			s.Linux = &specs.Linux{}
		}
		if s.Linux.Resources == nil {
			s.Linux.Resources = &specs.LinuxResources{}
		}

		if res.Memory > 0 {
			if s.Linux.Resources.Memory == nil {
				s.Linux.Resources.Memory = &specs.LinuxMemory{}
			}
			s.Linux.Resources.Memory.Limit = &res.Memory
		}

		if res.CPUShares > 0 || res.CPUQuota > 0 || res.CPUPeriod > 0 {
			if s.Linux.Resources.CPU == nil {
				s.Linux.Resources.CPU = &specs.LinuxCPU{}
			}
			if res.CPUShares > 0 {
				shares := uint64(res.CPUShares) // #nosec G115 - value is validated > 0
				s.Linux.Resources.CPU.Shares = &shares
			}
			if res.CPUQuota > 0 {
				s.Linux.Resources.CPU.Quota = &res.CPUQuota
			}
			if res.CPUPeriod > 0 {
				period := uint64(res.CPUPeriod) // #nosec G115 - value is validated > 0
				s.Linux.Resources.CPU.Period = &period
			}
		}

		if res.PidsLimit > 0 {
			if s.Linux.Resources.Pids == nil {
				s.Linux.Resources.Pids = &specs.LinuxPids{}
			}
			s.Linux.Resources.Pids.Limit = res.PidsLimit
		}

		return nil
	}
}

// withHostConfig applies host configuration options.
func (r *ContainerdRuntime) withHostConfig(hc *outbound.RuntimeHostConfig) oci.SpecOpts {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
		if hc.Privileged {
			// Set privileged mode
			if s.Linux == nil {
				s.Linux = &specs.Linux{}
			}
			s.Linux.MaskedPaths = nil
			s.Linux.ReadonlyPaths = nil
		}

		// Add capabilities
		if len(hc.CapAdd) > 0 && s.Process != nil && s.Process.Capabilities != nil {
			s.Process.Capabilities.Bounding = append(s.Process.Capabilities.Bounding, hc.CapAdd...)
			s.Process.Capabilities.Effective = append(s.Process.Capabilities.Effective, hc.CapAdd...)
			s.Process.Capabilities.Permitted = append(s.Process.Capabilities.Permitted, hc.CapAdd...)
		}

		// Apply sysctls
		if len(hc.Sysctls) > 0 {
			if s.Linux == nil {
				s.Linux = &specs.Linux{}
			}
			s.Linux.Sysctl = hc.Sysctls
		}

		return nil
	}
}

// getMountOptions returns mount options based on mount configuration.
func (r *ContainerdRuntime) getMountOptions(mount outbound.RuntimeMount) []string {
	var options []string
	if mount.ReadOnly {
		options = append(options, "ro")
	} else {
		options = append(options, "rw")
	}
	if mount.Propagation != "" {
		options = append(options, mount.Propagation)
	}
	return options
}

// parseSignal parses a signal string and returns the syscall.Signal.
func parseSignal(signal string) syscall.Signal {
	signal = strings.ToUpper(signal)
	// Remove SIG prefix if present
	signal = strings.TrimPrefix(signal, "SIG")

	switch signal {
	case "KILL", "9":
		return syscall.SIGKILL
	case "TERM", "15":
		return syscall.SIGTERM
	case "HUP", "1":
		return syscall.SIGHUP
	case "INT", "2":
		return syscall.SIGINT
	case "QUIT", "3":
		return syscall.SIGQUIT
	case "USR1", "10":
		return syscall.SIGUSR1
	case "USR2", "12":
		return syscall.SIGUSR2
	default:
		return syscall.SIGTERM
	}
}
