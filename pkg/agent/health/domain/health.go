package domain

import "time"

// ContainerID is a unique identifier for a container.
type ContainerID string

// CheckID is a unique identifier for a health check.
type CheckID string

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
	HealthStatusStarting  HealthStatus = "starting"
)

// CheckType represents the type of health check.
type CheckType string

const (
	CheckTypeHTTP    CheckType = "http"
	CheckTypeTCP     CheckType = "tcp"
	CheckTypeExec    CheckType = "exec"
	CheckTypeGRPC    CheckType = "grpc"
	CheckTypeProcess CheckType = "process"
)

// HealthCheck represents a health check configuration.
type HealthCheck struct {
	ID          CheckID
	ContainerID ContainerID
	Type        CheckType
	Target      string        // URL, address:port, or command
	Interval    time.Duration // How often to run the check
	Timeout     time.Duration // Max time for check to complete
	Retries     int           // Number of retries before marking unhealthy
	StartPeriod time.Duration // Grace period for container startup
}

// CheckResult represents the result of a health check execution.
type CheckResult struct {
	Timestamp   time.Time
	CheckID     CheckID
	ContainerID ContainerID
	Status      HealthStatus
	Output      string
	Duration    time.Duration
	Attempt     int
}

// ContainerHealth represents the overall health state of a container.
type ContainerHealth struct {
	LastCheck    time.Time
	LastFailure  *CheckResult
	LastSuccess  *CheckResult
	ContainerID  ContainerID
	Status       HealthStatus
	Checks       []HealthCheck
	FailCount    int
	SuccessCount int
}

// AgentHealth represents the health state of the agent itself.
type AgentHealth struct {
	LastHeartbeat time.Time
	Components    map[string]ComponentHealth
	Status        HealthStatus
	Uptime        time.Duration
	CPU           float64
	Memory        uint64
	Containers    int
	Healthy       bool
}

// ComponentHealth represents the health of an agent component.
type ComponentHealth struct {
	Name    string
	Status  HealthStatus
	Message string
	Version string
}
