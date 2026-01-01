package domain

import "fmt"

// DeploymentStatus represents the lifecycle state of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusPending    DeploymentStatus = "pending"
	DeploymentStatusValidating DeploymentStatus = "validating"
	DeploymentStatusScheduling DeploymentStatus = "scheduling"
	DeploymentStatusDeploying  DeploymentStatus = "deploying"
	DeploymentStatusRunning    DeploymentStatus = "running"
	DeploymentStatusStopping   DeploymentStatus = "stopping"
	DeploymentStatusStopped    DeploymentStatus = "stopped"
	DeploymentStatusFailed     DeploymentStatus = "failed"
	DeploymentStatusRollingOut DeploymentStatus = "rolling_out"
	DeploymentStatusRollback   DeploymentStatus = "rollback"
)

// IsTerminal returns true if the status is a terminal state.
func (s DeploymentStatus) IsTerminal() bool {
	return s == DeploymentStatusStopped || s == DeploymentStatusFailed
}

// IsActive returns true if the deployment is actively running.
func (s DeploymentStatus) IsActive() bool {
	return s == DeploymentStatusRunning || s == DeploymentStatusRollingOut
}

// Validate checks if the status is valid.
func (s DeploymentStatus) Validate() error {
	switch s {
	case DeploymentStatusPending, DeploymentStatusValidating, DeploymentStatusScheduling,
		DeploymentStatusDeploying, DeploymentStatusRunning, DeploymentStatusStopping,
		DeploymentStatusStopped, DeploymentStatusFailed, DeploymentStatusRollingOut,
		DeploymentStatusRollback:
		return nil
	default:
		return fmt.Errorf("%w: invalid deployment status: %s", ErrInvalidStatus, s)
	}
}

func (s DeploymentStatus) String() string { return string(s) }

// AgentStatus represents the state of an agent node.
type AgentStatus string

const (
	AgentStatusUnknown      AgentStatus = "unknown"
	AgentStatusRegistering  AgentStatus = "registering"
	AgentStatusHealthy      AgentStatus = "healthy"
	AgentStatusUnhealthy    AgentStatus = "unhealthy"
	AgentStatusDraining     AgentStatus = "draining"
	AgentStatusDisconnected AgentStatus = "disconnected"
	AgentStatusDeregistered AgentStatus = "deregistered"
)

// IsAvailable returns true if the agent can accept new tasks.
func (s AgentStatus) IsAvailable() bool {
	return s == AgentStatusHealthy
}

// IsConnected returns true if the agent is connected to the engine.
func (s AgentStatus) IsConnected() bool {
	return s == AgentStatusHealthy || s == AgentStatusDraining || s == AgentStatusUnhealthy
}

// Validate checks if the status is valid.
func (s AgentStatus) Validate() error {
	switch s {
	case AgentStatusUnknown, AgentStatusRegistering, AgentStatusHealthy,
		AgentStatusUnhealthy, AgentStatusDraining, AgentStatusDisconnected,
		AgentStatusDeregistered:
		return nil
	default:
		return fmt.Errorf("%w: invalid agent status: %s", ErrInvalidStatus, s)
	}
}

func (s AgentStatus) String() string { return string(s) }

// TaskStatus represents the state of an individual task.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusQueued    TaskStatus = "queued"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	TaskStatusRetrying  TaskStatus = "retrying"
	TaskStatusSkipped   TaskStatus = "skipped"
	TaskStatusTimedOut  TaskStatus = "timed_out"
)

// IsTerminal returns true if the task is in a terminal state.
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusSucceeded || s == TaskStatusFailed ||
		s == TaskStatusCancelled || s == TaskStatusSkipped || s == TaskStatusTimedOut
}

// IsActive returns true if the task is currently executing.
func (s TaskStatus) IsActive() bool {
	return s == TaskStatusRunning || s == TaskStatusRetrying
}

// Validate checks if the status is valid.
func (s TaskStatus) Validate() error {
	switch s {
	case TaskStatusPending, TaskStatusQueued, TaskStatusRunning,
		TaskStatusSucceeded, TaskStatusFailed, TaskStatusCancelled,
		TaskStatusRetrying, TaskStatusSkipped, TaskStatusTimedOut:
		return nil
	default:
		return fmt.Errorf("%w: invalid task status: %s", ErrInvalidStatus, s)
	}
}

func (s TaskStatus) String() string { return string(s) }

// ContainerStatus represents the state of a container.
type ContainerStatus string

const (
	ContainerStatusCreating   ContainerStatus = "creating"
	ContainerStatusCreated    ContainerStatus = "created"
	ContainerStatusStarting   ContainerStatus = "starting"
	ContainerStatusRunning    ContainerStatus = "running"
	ContainerStatusPaused     ContainerStatus = "paused"
	ContainerStatusStopping   ContainerStatus = "stopping"
	ContainerStatusStopped    ContainerStatus = "stopped"
	ContainerStatusRestarting ContainerStatus = "restarting"
	ContainerStatusRemoving   ContainerStatus = "removing"
	ContainerStatusRemoved    ContainerStatus = "removed"
	ContainerStatusFailed     ContainerStatus = "failed"
)

// IsRunning returns true if the container is actively running.
func (s ContainerStatus) IsRunning() bool {
	return s == ContainerStatusRunning
}

// IsTerminal returns true if the container is in a terminal state.
func (s ContainerStatus) IsTerminal() bool {
	return s == ContainerStatusStopped || s == ContainerStatusRemoved || s == ContainerStatusFailed
}

// Validate checks if the status is valid.
func (s ContainerStatus) Validate() error {
	switch s {
	case ContainerStatusCreating, ContainerStatusCreated, ContainerStatusStarting,
		ContainerStatusRunning, ContainerStatusPaused, ContainerStatusStopping,
		ContainerStatusStopped, ContainerStatusRestarting, ContainerStatusRemoving,
		ContainerStatusRemoved, ContainerStatusFailed:
		return nil
	default:
		return fmt.Errorf("%w: invalid container status: %s", ErrInvalidStatus, s)
	}
}

func (s ContainerStatus) String() string { return string(s) }

// HealthStatus represents the health state of a component.
type HealthStatus string

const (
	HealthStatusUnknown   HealthStatus = "unknown"
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusStarting  HealthStatus = "starting"
)

// IsHealthy returns true if the health status indicates healthy state.
func (s HealthStatus) IsHealthy() bool {
	return s == HealthStatusHealthy
}

// Validate checks if the status is valid.
func (s HealthStatus) Validate() error {
	switch s {
	case HealthStatusUnknown, HealthStatusHealthy, HealthStatusUnhealthy,
		HealthStatusDegraded, HealthStatusStarting:
		return nil
	default:
		return fmt.Errorf("%w: invalid health status: %s", ErrInvalidStatus, s)
	}
}

func (s HealthStatus) String() string { return string(s) }

// TaskType represents the type of task to be executed.
type TaskType string

const (
	TaskTypeContainerCreate  TaskType = "container_create"
	TaskTypeContainerStart   TaskType = "container_start"
	TaskTypeContainerStop    TaskType = "container_stop"
	TaskTypeContainerRemove  TaskType = "container_remove"
	TaskTypeContainerRestart TaskType = "container_restart"
	TaskTypeNetworkSetup     TaskType = "network_setup"
	TaskTypeNetworkTeardown  TaskType = "network_teardown"
	TaskTypeSecurityApply    TaskType = "security_apply"
	TaskTypeSecurityRemove   TaskType = "security_remove"
	TaskTypeHealthCheck      TaskType = "health_check"
)

// Validate checks if the task type is valid.
func (t TaskType) Validate() error {
	switch t {
	case TaskTypeContainerCreate, TaskTypeContainerStart, TaskTypeContainerStop,
		TaskTypeContainerRemove, TaskTypeContainerRestart, TaskTypeNetworkSetup,
		TaskTypeNetworkTeardown, TaskTypeSecurityApply, TaskTypeSecurityRemove,
		TaskTypeHealthCheck:
		return nil
	default:
		return fmt.Errorf("%w: invalid task type: %s", ErrInvalidStatus, t)
	}
}

func (t TaskType) String() string { return string(t) }

// TaskPriority represents the priority level of a task.
type TaskPriority int

const (
	TaskPriorityLow      TaskPriority = 0
	TaskPriorityNormal   TaskPriority = 50
	TaskPriorityHigh     TaskPriority = 100
	TaskPriorityCritical TaskPriority = 200
)

// HookPoint represents lifecycle hook points for plugins.
type HookPoint string

const (
	HookPointPreValidate  HookPoint = "pre-validate"
	HookPointPostValidate HookPoint = "post-validate"
	HookPointPreDeploy    HookPoint = "pre-deploy"
	HookPointPostDeploy   HookPoint = "post-deploy"
	HookPointPreStop      HookPoint = "pre-stop"
	HookPointPostStop     HookPoint = "post-stop"
	HookPointOnFailure    HookPoint = "on-failure"
)

// Validate checks if the hook point is valid.
func (h HookPoint) Validate() error {
	switch h {
	case HookPointPreValidate, HookPointPostValidate, HookPointPreDeploy,
		HookPointPostDeploy, HookPointPreStop, HookPointPostStop, HookPointOnFailure:
		return nil
	default:
		return fmt.Errorf("%w: invalid hook point: %s", ErrInvalidStatus, h)
	}
}

func (h HookPoint) String() string { return string(h) }

// PluginType represents the type of plugin.
type PluginType string

const (
	PluginTypeWebhook PluginType = "webhook"
	PluginTypeGRPC    PluginType = "grpc"
	PluginTypeBuiltin PluginType = "builtin"
)

// Validate checks if the plugin type is valid.
func (p PluginType) Validate() error {
	switch p {
	case PluginTypeWebhook, PluginTypeGRPC, PluginTypeBuiltin:
		return nil
	default:
		return fmt.Errorf("%w: invalid plugin type: %s", ErrInvalidStatus, p)
	}
}

func (p PluginType) String() string { return string(p) }

// ProbeType represents the type of health probe.
type ProbeType string

const (
	ProbeTypeHTTP ProbeType = "http"
	ProbeTypeTCP  ProbeType = "tcp"
	ProbeTypeExec ProbeType = "exec"
)

// Validate checks if the probe type is valid.
func (p ProbeType) Validate() error {
	switch p {
	case ProbeTypeHTTP, ProbeTypeTCP, ProbeTypeExec:
		return nil
	default:
		return fmt.Errorf("%w: invalid probe type: %s", ErrInvalidStatus, p)
	}
}

func (p ProbeType) String() string { return string(p) }

// SelectionStrategy represents agent selection strategies.
type SelectionStrategy string

const (
	SelectionStrategyRoundRobin  SelectionStrategy = "round_robin"
	SelectionStrategyLeastLoaded SelectionStrategy = "least_loaded"
	SelectionStrategySpread      SelectionStrategy = "spread"
	SelectionStrategyBinPack     SelectionStrategy = "bin_pack"
)

// Validate checks if the selection strategy is valid.
func (s SelectionStrategy) Validate() error {
	switch s {
	case SelectionStrategyRoundRobin, SelectionStrategyLeastLoaded,
		SelectionStrategySpread, SelectionStrategyBinPack:
		return nil
	default:
		return fmt.Errorf("%w: invalid selection strategy: %s", ErrInvalidStatus, s)
	}
}

func (s SelectionStrategy) String() string { return string(s) }
