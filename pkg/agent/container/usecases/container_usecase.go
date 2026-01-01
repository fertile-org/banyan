package usecases

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// ContainerUseCase implements the ContainerService interface.
type ContainerUseCase struct {
	runtime   outbound.ContainerRuntime
	network   outbound.NetworkConnector
	events    outbound.EventEmitter
	images    outbound.ImageRegistry
	labelBase string
}

// NewContainerUseCase creates a new ContainerUseCase.
func NewContainerUseCase(
	runtime outbound.ContainerRuntime,
	network outbound.NetworkConnector,
	events outbound.EventEmitter,
	images outbound.ImageRegistry,
) *ContainerUseCase {
	return &ContainerUseCase{
		runtime:   runtime,
		network:   network,
		events:    events,
		images:    images,
		labelBase: "com.fertile.banyan",
	}
}

// CreateContainer creates a new container.
func (uc *ContainerUseCase) CreateContainer(ctx context.Context, spec *inbound.ContainerSpec) (*inbound.ContainerInfo, error) {
	// Validate the spec
	if err := uc.validateSpec(spec); err != nil {
		return nil, err
	}

	// Check if image exists locally, pull if not
	imageRef := spec.Config.Image.String()
	exists, err := uc.images.ImageExists(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to check image existence: %w", err)
	}
	if !exists {
		if pullErr := uc.images.Pull(ctx, imageRef, outbound.ImagePullOptions{}); pullErr != nil {
			return nil, fmt.Errorf("failed to pull image: %w", pullErr)
		}
	}

	// Build runtime config
	runtimeConfig := uc.buildRuntimeConfig(spec)

	// Create the container
	containerID, err := uc.runtime.Create(ctx, runtimeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Connect to network if specified
	if spec.NetworkID != "" {
		if connectErr := uc.network.ConnectContainer(ctx, containerID, spec.NetworkID, spec.IPAddress); connectErr != nil {
			// Cleanup on failure
			_ = uc.runtime.Remove(ctx, containerID, true)
			return nil, fmt.Errorf("failed to connect to network: %w", connectErr)
		}
	}

	// Inspect the created container
	info, err := uc.runtime.Inspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	return uc.toContainerInfo(info, spec.ServiceID), nil
}

// StartContainer starts a container.
func (uc *ContainerUseCase) StartContainer(ctx context.Context, id domain.ContainerID) error {
	if err := id.Validate(); err != nil {
		return err
	}

	info, err := uc.runtime.Inspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if info.State == "running" {
		return domain.ErrContainerRunning
	}

	if err := uc.runtime.Start(ctx, string(id)); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Emit event
	if uc.events != nil {
		uc.events.Emit(outbound.ContainerStartedEvent{
			ContainerID: string(id),
			ServiceID:   uc.extractServiceID(info),
			StartedAt:   time.Now(),
		})
	}

	return nil
}

// StopContainer stops a container.
func (uc *ContainerUseCase) StopContainer(ctx context.Context, id domain.ContainerID, timeout time.Duration) error {
	if err := id.Validate(); err != nil {
		return err
	}

	info, err := uc.runtime.Inspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	if info.State != "running" {
		return domain.ErrContainerNotRunning
	}

	if err := uc.runtime.Stop(ctx, string(id), timeout); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}

	// Emit event
	if uc.events != nil {
		uc.events.Emit(outbound.ContainerStoppedEvent{
			ContainerID: string(id),
			ServiceID:   uc.extractServiceID(info),
			StoppedAt:   time.Now(),
			ExitCode:    info.ExitCode,
		})
	}

	return nil
}

// RestartContainer restarts a container.
func (uc *ContainerUseCase) RestartContainer(ctx context.Context, id domain.ContainerID, timeout time.Duration) error {
	if err := id.Validate(); err != nil {
		return err
	}

	info, err := uc.runtime.Inspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	// Stop if running
	if info.State == "running" {
		if err := uc.runtime.Stop(ctx, string(id), timeout); err != nil {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	// Start the container
	if err := uc.runtime.Start(ctx, string(id)); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// RemoveContainer removes a container.
func (uc *ContainerUseCase) RemoveContainer(ctx context.Context, id domain.ContainerID, force bool) error {
	if err := id.Validate(); err != nil {
		return err
	}

	info, err := uc.runtime.Inspect(ctx, string(id))
	if err != nil {
		return fmt.Errorf("failed to inspect container: %w", err)
	}

	// Disconnect from network first
	if info.NetworkSettings != nil && info.NetworkSettings.NetworkID != "" {
		_ = uc.network.DisconnectContainer(ctx, string(id), info.NetworkSettings.NetworkID)
	}

	if err := uc.runtime.Remove(ctx, string(id), force); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	// Emit event
	if uc.events != nil {
		uc.events.Emit(outbound.ContainerRemovedEvent{
			ContainerID: string(id),
			ServiceID:   uc.extractServiceID(info),
			RemovedAt:   time.Now(),
		})
	}

	return nil
}

// GetContainer retrieves container information.
func (uc *ContainerUseCase) GetContainer(ctx context.Context, id domain.ContainerID) (*inbound.ContainerInfo, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	info, err := uc.runtime.Inspect(ctx, string(id))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	return uc.toContainerInfo(info, uc.extractServiceID(info)), nil
}

// ListContainers lists containers matching the filter.
func (uc *ContainerUseCase) ListContainers(ctx context.Context, filter *inbound.ContainerFilter) ([]*inbound.ContainerInfo, error) {
	opts := outbound.ListOptions{
		Filters: make(map[string][]string),
	}

	if filter != nil {
		opts.All = filter.All

		// Add label filters
		if filter.ServiceID != "" {
			opts.Filters["label"] = append(opts.Filters["label"], fmt.Sprintf("%s.service-id=%s", uc.labelBase, filter.ServiceID))
		}
		for k, v := range filter.Labels {
			opts.Filters["label"] = append(opts.Filters["label"], fmt.Sprintf("%s=%s", k, v))
		}

		// Add state filters
		for _, state := range filter.States {
			opts.Filters["status"] = append(opts.Filters["status"], uc.stateToStatus(state))
		}
	}

	containers, err := uc.runtime.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]*inbound.ContainerInfo, 0, len(containers))
	for _, c := range containers {
		result = append(result, uc.toContainerInfo(c, uc.extractServiceID(c)))
	}

	return result, nil
}

// ExecInContainer executes a command in a container.
func (uc *ContainerUseCase) ExecInContainer(ctx context.Context, id domain.ContainerID, cmd *inbound.ExecConfig) (*inbound.ExecResult, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	execConfig := outbound.RuntimeExecConfig{
		Cmd:          cmd.Cmd,
		Env:          cmd.Env,
		WorkingDir:   cmd.WorkingDir,
		User:         cmd.User,
		Tty:          cmd.Tty,
		AttachStdin:  cmd.AttachStdin,
		AttachStdout: cmd.AttachStdout,
		AttachStderr: cmd.AttachStderr,
		Detach:       cmd.Detach,
		Privileged:   cmd.Privileged,
	}

	execID, err := uc.runtime.ExecCreate(ctx, string(id), &execConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create exec: %w", err)
	}

	stream, err := uc.runtime.ExecStart(ctx, execID, outbound.ExecStartOptions{
		Tty:    cmd.Tty,
		Detach: cmd.Detach,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start exec: %w", err)
	}
	defer stream.Close()

	// Read output
	var stdout, stderr []byte
	if !cmd.Detach {
		stdout, err = io.ReadAll(stream)
		if err != nil {
			return nil, fmt.Errorf("failed to read exec output: %w", err)
		}
	}

	// Get exit code
	inspect, err := uc.runtime.ExecInspect(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect exec: %w", err)
	}

	return &inbound.ExecResult{
		ExitCode: inspect.ExitCode,
		Stdout:   stdout,
		Stderr:   stderr,
	}, nil
}

// CopyToContainer copies content to a container.
func (uc *ContainerUseCase) CopyToContainer(ctx context.Context, id domain.ContainerID, path string, content io.Reader) error {
	if err := id.Validate(); err != nil {
		return err
	}
	return uc.runtime.CopyToContainer(ctx, string(id), path, content)
}

// CopyFromContainer copies content from a container.
func (uc *ContainerUseCase) CopyFromContainer(ctx context.Context, id domain.ContainerID, path string) (io.ReadCloser, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	return uc.runtime.CopyFromContainer(ctx, string(id), path)
}

// StreamLogs streams container logs.
func (uc *ContainerUseCase) StreamLogs(ctx context.Context, id domain.ContainerID, opts *inbound.LogOptions) (inbound.LogStream, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	runtimeOpts := outbound.RuntimeLogOptions{}
	if opts != nil {
		runtimeOpts.Since = opts.Since
		runtimeOpts.Until = opts.Until
		runtimeOpts.Tail = opts.Tail
		runtimeOpts.ShowStdout = opts.Stdout
		runtimeOpts.ShowStderr = opts.Stderr
		runtimeOpts.Follow = opts.Follow
		runtimeOpts.Timestamps = opts.Timestamps
	}

	reader, err := uc.runtime.Logs(ctx, string(id), runtimeOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return &logStream{reader: reader}, nil
}

// GetStats gets container statistics.
func (uc *ContainerUseCase) GetStats(ctx context.Context, id domain.ContainerID) (*domain.ContainerStats, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	stats, err := uc.runtime.Stats(ctx, string(id))
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return &domain.ContainerStats{
		ContainerID: id,
		Timestamp:   time.Now(),
		CPU: domain.CPUStats{
			UsagePercent: stats.CPUPercent,
		},
		Memory: domain.MemoryStats{
			Usage: stats.MemoryUsage,
			Limit: stats.MemoryLimit,
		},
		Network: domain.NetworkStats{
			RxBytes: stats.NetworkRxBytes,
			TxBytes: stats.NetworkTxBytes,
		},
		BlockIO: domain.BlockIOStats{
			ReadBytes:  stats.BlockRead,
			WriteBytes: stats.BlockWrite,
		},
		PIDs: stats.PIDs,
	}, nil
}

// StreamStats streams container statistics.
func (uc *ContainerUseCase) StreamStats(ctx context.Context, id domain.ContainerID) (inbound.StatsStream, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	// For now, return a simple polling-based stats stream
	return &statsStream{
		ctx:       ctx,
		runtime:   uc.runtime,
		container: string(id),
	}, nil
}

// validateSpec validates the container spec.
func (uc *ContainerUseCase) validateSpec(spec *inbound.ContainerSpec) error {
	if spec == nil {
		return fmt.Errorf("container spec is required")
	}
	if spec.Config.Image.Name == "" {
		return domain.ErrImageRequired
	}
	return nil
}

// buildRuntimeConfig builds a runtime config from the spec.
func (uc *ContainerUseCase) buildRuntimeConfig(spec *inbound.ContainerSpec) *outbound.RuntimeContainerConfig {
	config := &outbound.RuntimeContainerConfig{
		Name:       spec.Name,
		Image:      spec.Config.Image.String(),
		Cmd:        spec.Config.Command,
		Entrypoint: spec.Config.Entrypoint,
		Env:        uc.envToStrings(spec.Config.Env),
		WorkingDir: spec.Config.WorkingDir,
		User:       spec.Config.User,
		Hostname:   spec.Config.Hostname,
		Labels: map[string]string{
			uc.labelBase + ".service-id": spec.ServiceID,
			uc.labelBase + ".managed":    "true",
		},
	}

	// Add custom labels
	for k, v := range spec.Config.Labels {
		config.Labels[k] = v
	}

	// Convert mounts
	config.Mounts = make([]outbound.RuntimeMount, 0, len(spec.Config.Mounts))
	for _, m := range spec.Config.Mounts {
		config.Mounts = append(config.Mounts, outbound.RuntimeMount{
			Source:   m.Source,
			Target:   m.Destination,
			Type:     string(m.Type),
			ReadOnly: m.ReadOnly,
		})
	}

	// Convert port bindings
	config.PortBindings = make([]outbound.RuntimePortBinding, 0, len(spec.Config.Ports))
	for _, p := range spec.Config.Ports {
		config.PortBindings = append(config.PortBindings, outbound.RuntimePortBinding{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			HostIP:        p.HostIP,
			Protocol:      string(p.Protocol),
		})
	}

	// Convert resources
	if !spec.Config.Resources.IsEmpty() {
		config.Resources = &outbound.RuntimeResources{
			Memory:    spec.Config.Resources.MemoryLimit,
			CPUShares: spec.Config.Resources.CPUShares,
			CPUQuota:  spec.Config.Resources.CPUQuota,
			CPUPeriod: spec.Config.Resources.CPUPeriod,
			PidsLimit: spec.Config.Resources.PidsLimit,
		}
	}

	// Convert restart policy
	if spec.Config.RestartPolicy.Name != "" {
		config.RestartPolicy = &outbound.RuntimeRestartPolicy{
			Name:              spec.Config.RestartPolicy.Name,
			MaximumRetryCount: spec.Config.RestartPolicy.MaximumRetryCount,
		}
	}

	// Convert network mode
	if spec.Config.NetworkMode != "" {
		config.NetworkMode = string(spec.Config.NetworkMode)
	}

	return config
}

// envToStrings converts EnvVar slice to string slice.
func (uc *ContainerUseCase) envToStrings(env []domain.EnvVar) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		result = append(result, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}
	return result
}

// toContainerInfo converts runtime info to inbound info.
func (uc *ContainerUseCase) toContainerInfo(info *outbound.RuntimeContainerInfo, serviceID string) *inbound.ContainerInfo {
	result := &inbound.ContainerInfo{
		ID:        domain.ContainerID(info.ID),
		Name:      info.Name,
		ServiceID: serviceID,
		State:     uc.statusToState(info.State),
	}

	if info.NetworkSettings != nil {
		result.IPAddress = info.NetworkSettings.IPAddress
		result.Ports = make([]domain.PortBinding, 0, len(info.NetworkSettings.Ports))
		for _, p := range info.NetworkSettings.Ports {
			result.Ports = append(result.Ports, domain.PortBinding{
				ContainerPort: p.ContainerPort,
				HostPort:      p.HostPort,
				HostIP:        p.HostIP,
				Protocol:      domain.Protocol(p.Protocol),
			})
		}
	}

	return result
}

// extractServiceID extracts service ID from container labels.
func (uc *ContainerUseCase) extractServiceID(info *outbound.RuntimeContainerInfo) string {
	return "" // Labels are not exposed in RuntimeContainerInfo yet
}

// stateToStatus converts domain state to runtime status.
func (uc *ContainerUseCase) stateToStatus(state domain.ContainerState) string {
	switch state {
	case domain.ContainerStateCreated:
		return "created"
	case domain.ContainerStateRunning:
		return "running"
	case domain.ContainerStatePaused:
		return "paused"
	case domain.ContainerStateStopped:
		return "exited"
	case domain.ContainerStateRestarting:
		return "restarting"
	case domain.ContainerStateRemoving:
		return "removing"
	case domain.ContainerStateDead:
		return "dead"
	default:
		return ""
	}
}

// statusToState converts runtime status to domain state.
func (uc *ContainerUseCase) statusToState(status string) domain.ContainerState {
	switch status {
	case "created":
		return domain.ContainerStateCreated
	case "running":
		return domain.ContainerStateRunning
	case "paused":
		return domain.ContainerStatePaused
	case "exited", "stopped":
		return domain.ContainerStateStopped
	case "restarting":
		return domain.ContainerStateRestarting
	case "removing":
		return domain.ContainerStateRemoving
	case "dead":
		return domain.ContainerStateDead
	default:
		return domain.ContainerStateCreated
	}
}

// logStream implements the LogStream interface.
type logStream struct {
	reader io.ReadCloser
}

func (s *logStream) Next() (*inbound.LogEntry, error) {
	buf := make([]byte, 4096)
	n, err := s.reader.Read(buf)
	if err != nil {
		return nil, err
	}
	return &inbound.LogEntry{
		Timestamp: time.Now(),
		Message:   string(buf[:n]),
		Stream:    "stdout",
	}, nil
}

func (s *logStream) Close() error {
	return s.reader.Close()
}

// statsStream implements the StatsStream interface.
type statsStream struct {
	ctx       context.Context
	runtime   outbound.ContainerRuntime
	container string
}

func (s *statsStream) Next() (*domain.ContainerStats, error) {
	select {
	case <-s.ctx.Done():
		return nil, io.EOF
	default:
	}

	stats, err := s.runtime.Stats(s.ctx, s.container)
	if err != nil {
		return nil, err
	}

	return &domain.ContainerStats{
		ContainerID: domain.ContainerID(s.container),
		Timestamp:   time.Now(),
		CPU: domain.CPUStats{
			UsagePercent: stats.CPUPercent,
		},
		Memory: domain.MemoryStats{
			Usage: stats.MemoryUsage,
			Limit: stats.MemoryLimit,
		},
		Network: domain.NetworkStats{
			RxBytes: stats.NetworkRxBytes,
			TxBytes: stats.NetworkTxBytes,
		},
		BlockIO: domain.BlockIOStats{
			ReadBytes:  stats.BlockRead,
			WriteBytes: stats.BlockWrite,
		},
		PIDs: stats.PIDs,
	}, nil
}

func (s *statsStream) Close() error {
	return nil
}
