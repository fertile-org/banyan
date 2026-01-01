package inbound

import (
	"context"
	"io"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
)

// ContainerService defines the main interface for container operations.
// This is the primary inbound port that driving adapters use to interact
// with the container runtime domain.
type ContainerService interface {
	// Container lifecycle
	CreateContainer(ctx context.Context, spec *ContainerSpec) (*ContainerInfo, error)
	StartContainer(ctx context.Context, id domain.ContainerID) error
	StopContainer(ctx context.Context, id domain.ContainerID, timeout time.Duration) error
	RestartContainer(ctx context.Context, id domain.ContainerID, timeout time.Duration) error
	RemoveContainer(ctx context.Context, id domain.ContainerID, force bool) error

	// Container inspection
	GetContainer(ctx context.Context, id domain.ContainerID) (*ContainerInfo, error)
	ListContainers(ctx context.Context, filter *ContainerFilter) ([]*ContainerInfo, error)

	// Container operations
	ExecInContainer(ctx context.Context, id domain.ContainerID, cmd *ExecConfig) (*ExecResult, error)
	CopyToContainer(ctx context.Context, id domain.ContainerID, path string, content io.Reader) error
	CopyFromContainer(ctx context.Context, id domain.ContainerID, path string) (io.ReadCloser, error)

	// Logs and stats
	StreamLogs(ctx context.Context, id domain.ContainerID, opts *LogOptions) (LogStream, error)
	GetStats(ctx context.Context, id domain.ContainerID) (*domain.ContainerStats, error)
	StreamStats(ctx context.Context, id domain.ContainerID) (StatsStream, error)
}

// ContainerSpec is the input for creating a container.
type ContainerSpec struct {
	Name      string
	ServiceID string
	NetworkID string
	IPAddress string
	Config    domain.ContainerConfig
}

// ContainerInfo is the output containing container details.
type ContainerInfo struct {
	CreatedAt time.Time
	StartedAt *time.Time
	ID        domain.ContainerID
	Name      string
	ServiceID string
	IPAddress string
	State     domain.ContainerState
	Health    domain.HealthStatus
	Ports     []domain.PortBinding
}

// ContainerFilter for listing containers.
type ContainerFilter struct {
	Labels    map[string]string
	ServiceID string
	States    []domain.ContainerState
	All       bool // Include stopped containers
}

// ExecConfig contains configuration for executing a command in a container.
type ExecConfig struct {
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

// ExecResult contains the result of executing a command in a container.
type ExecResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// LogOptions contains options for streaming logs.
type LogOptions struct {
	Since      string
	Until      string
	Tail       string
	Stdout     bool
	Stderr     bool
	Follow     bool
	Timestamps bool
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time
	Message   string
	Stream    string // stdout or stderr
}

// LogStream is a stream of log entries.
type LogStream interface {
	// Next returns the next log entry. Returns io.EOF when done.
	Next() (*LogEntry, error)
	// Close closes the stream.
	Close() error
}

// StatsStream is a stream of container stats.
type StatsStream interface {
	// Next returns the next stats snapshot. Returns io.EOF when done.
	Next() (*domain.ContainerStats, error)
	// Close closes the stream.
	Close() error
}
