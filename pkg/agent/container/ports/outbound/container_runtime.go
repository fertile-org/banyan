package outbound

import (
	"context"
	"io"
	"time"
)

// ContainerRuntime abstracts the underlying container runtime (Docker, containerd).
// This is the primary outbound port that use cases use to interact with the
// container runtime infrastructure.
type ContainerRuntime interface {
	// Container operations
	Create(ctx context.Context, config *RuntimeContainerConfig) (string, error)
	Start(ctx context.Context, containerID string) error
	Stop(ctx context.Context, containerID string, timeout time.Duration) error
	Remove(ctx context.Context, containerID string, force bool) error
	Kill(ctx context.Context, containerID string, signal string) error
	Pause(ctx context.Context, containerID string) error
	Unpause(ctx context.Context, containerID string) error

	// Container inspection
	Inspect(ctx context.Context, containerID string) (*RuntimeContainerInfo, error)
	List(ctx context.Context, opts ListOptions) ([]*RuntimeContainerInfo, error)

	// Container operations
	Logs(ctx context.Context, containerID string, opts RuntimeLogOptions) (io.ReadCloser, error)
	Stats(ctx context.Context, containerID string) (*RuntimeStats, error)
	Wait(ctx context.Context, containerID string) (<-chan WaitResult, error)

	// Exec
	ExecCreate(ctx context.Context, containerID string, config *RuntimeExecConfig) (string, error)
	ExecStart(ctx context.Context, execID string, opts ExecStartOptions) (io.ReadWriteCloser, error)
	ExecInspect(ctx context.Context, execID string) (*ExecInspect, error)

	// Copy operations
	CopyToContainer(ctx context.Context, containerID, path string, content io.Reader) error
	CopyFromContainer(ctx context.Context, containerID, path string) (io.ReadCloser, error)
}

// RuntimeContainerConfig is the configuration for creating a container.
type RuntimeContainerConfig struct {
	Labels        map[string]string
	Resources     *RuntimeResources
	RestartPolicy *RuntimeRestartPolicy
	HostConfig    *RuntimeHostConfig
	Name          string
	Image         string
	Hostname      string
	WorkingDir    string
	User          string
	NetworkMode   string
	Env           []string
	Mounts        []RuntimeMount
	PortBindings  []RuntimePortBinding
	Cmd           []string
	Entrypoint    []string
}

// RuntimeResources defines resource constraints.
type RuntimeResources struct {
	CPUShares      int64
	CPUQuota       int64
	CPUPeriod      int64
	Memory         int64
	MemorySwap     int64
	PidsLimit      int64
	OOMKillDisable bool
}

// RuntimeMount defines a mount configuration.
type RuntimeMount struct {
	Source      string
	Target      string
	Type        string
	Propagation string
	ReadOnly    bool
}

// RuntimePortBinding defines port binding.
type RuntimePortBinding struct {
	HostIP        string
	Protocol      string
	ContainerPort uint16
	HostPort      uint16
}

// RuntimeRestartPolicy defines restart behavior.
type RuntimeRestartPolicy struct {
	Name              string
	MaximumRetryCount int
}

// RuntimeHostConfig contains host-specific configuration.
type RuntimeHostConfig struct {
	Sysctls     map[string]string
	NetworkMode string
	IpcMode     string
	PidMode     string
	UTSMode     string
	CapAdd      []string
	CapDrop     []string
	SecurityOpt []string
	DNSServers  []string
	DNSSearch   []string
	ExtraHosts  []string
	Privileged  bool
}

// RuntimeContainerInfo contains information about a container.
type RuntimeContainerInfo struct {
	NetworkSettings *RuntimeNetworkSettings
	Created         string
	Started         string
	Finished        string
	ID              string
	Name            string
	Image           string
	Status          string
	State           string
	ExitCode        int
}

// RuntimeNetworkSettings contains network information.
type RuntimeNetworkSettings struct {
	IPAddress string
	Gateway   string
	NetworkID string
	Ports     []RuntimePortBinding
}

// ListOptions contains options for listing containers.
type ListOptions struct {
	Filters map[string][]string
	All     bool
}

// RuntimeLogOptions contains options for streaming logs.
type RuntimeLogOptions struct {
	Since      string
	Until      string
	Tail       string
	ShowStdout bool
	ShowStderr bool
	Follow     bool
	Timestamps bool
}

// RuntimeStats contains container statistics.
type RuntimeStats struct {
	CPUPercent     float64
	MemoryUsage    uint64
	MemoryLimit    uint64
	NetworkRxBytes uint64
	NetworkTxBytes uint64
	BlockRead      uint64
	BlockWrite     uint64
	PIDs           uint64
}

// WaitResult is the result of waiting for a container.
type WaitResult struct {
	Error      *WaitError
	StatusCode int64
}

// WaitError contains error information from Wait.
type WaitError struct {
	Message string
}

// RuntimeExecConfig contains configuration for exec.
type RuntimeExecConfig struct {
	WorkingDir   string
	User         string
	Env          []string
	Cmd          []string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Tty          bool
	Detach       bool
	Privileged   bool
}

// ExecStartOptions contains options for starting exec.
type ExecStartOptions struct {
	Tty    bool
	Detach bool
}

// ExecInspect contains exec inspection result.
type ExecInspect struct {
	ContainerID string
	ExecID      string
	ExitCode    int
	Running     bool
	Pid         int
}
