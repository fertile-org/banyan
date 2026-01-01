package outbound

import (
	"context"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
)

// ContainerService is the interface to the Container Runtime component.
type ContainerService interface {
	// CreateContainer creates a new container.
	CreateContainer(ctx context.Context, spec *ContainerSpec) (*ContainerInfo, error)

	// StartContainer starts a container.
	StartContainer(ctx context.Context, containerID string) error

	// StopContainer stops a container.
	StopContainer(ctx context.Context, containerID string, timeout time.Duration) error

	// RemoveContainer removes a container.
	RemoveContainer(ctx context.Context, containerID string, force bool) error

	// RestartContainer restarts a container.
	RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error

	// GetContainer gets container info.
	GetContainer(ctx context.Context, containerID string) (*ContainerInfo, error)
}

// ContainerSpec specifies how to create a container.
type ContainerSpec struct {
	Environment map[string]string
	Labels      map[string]string
	Name        string
	Image       string
	Command     []string
}

// ContainerInfo contains information about a container.
type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	State  string
	Status string
}

// NetworkService is the interface to the Network Node component.
type NetworkService interface {
	// ConnectContainer connects a container to a network.
	ConnectContainer(ctx context.Context, containerID, networkID, ipAddress string) error

	// DisconnectContainer disconnects a container from a network.
	DisconnectContainer(ctx context.Context, containerID, networkID string) error

	// GetContainerNetwork gets the network configuration for a container.
	GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetworkInfo, error)
}

// ContainerNetworkInfo contains network information for a container.
type ContainerNetworkInfo struct {
	ContainerID string
	NetworkID   string
	IPAddress   string
	Gateway     string
	MacAddress  string
}

// SecurityService is the interface to the Security Executor component.
type SecurityService interface {
	// ApplyPolicy applies a security policy.
	ApplyPolicy(ctx context.Context, policy *SecurityPolicy) error

	// RemovePolicy removes a security policy.
	RemovePolicy(ctx context.Context, policyID string) error

	// GetPolicyStatus gets the status of a security policy.
	GetPolicyStatus(ctx context.Context, policyID string) (*PolicyStatus, error)
}

// SecurityPolicy represents a security policy to apply.
type SecurityPolicy struct {
	ID          string
	ContainerID string
	Rules       []SecurityRule
}

// SecurityRule represents a single security rule.
type SecurityRule struct {
	Direction string
	Protocol  string
	PortRange string
	Source    string
	Action    string
	Port      int
}

// PolicyStatus represents the status of a security policy.
type PolicyStatus struct {
	PolicyID string
	Error    string
	Applied  bool
}

// HealthService is the interface to the Health Monitor component.
type HealthService interface {
	// RunHealthCheck runs a health check for a container.
	RunHealthCheck(ctx context.Context, containerID, checkType, target string, timeout time.Duration) (*HealthCheckResult, error)
}

// HealthCheckResult contains the result of a health check.
type HealthCheckResult struct {
	Message  string
	Duration time.Duration
	Healthy  bool
}

// TaskStore persists tasks for durability.
type TaskStore interface {
	// Save saves a task.
	Save(ctx context.Context, task *domain.Task) error

	// Get retrieves a task by ID.
	Get(ctx context.Context, taskID domain.TaskID) (*domain.Task, error)

	// Update updates a task.
	Update(ctx context.Context, task *domain.Task) error

	// Delete deletes a task.
	Delete(ctx context.Context, taskID domain.TaskID) error

	// List lists tasks matching the filter.
	List(ctx context.Context, filter *domain.TaskFilter) ([]*domain.Task, error)

	// GetPending gets all pending tasks.
	GetPending(ctx context.Context) ([]*domain.Task, error)
}

// EventEmitter emits task events.
type EventEmitter interface {
	// Emit emits a task event.
	Emit(event domain.TaskEvent)

	// Subscribe subscribes to task events.
	Subscribe() <-chan domain.TaskEvent

	// Unsubscribe unsubscribes from task events.
	Unsubscribe(ch <-chan domain.TaskEvent)
}
