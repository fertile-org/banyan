package domain

import "time"

// Deployment represents a deployment workflow.
type Deployment struct {
	ID          string
	Name        string
	Services    []Service
	State       DeploymentState
	Plan        *ExecutionPlan
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Error       string
}

// DeploymentState represents the lifecycle state.
type DeploymentState string

const (
	StateCreated     DeploymentState = "created"
	StateValidating  DeploymentState = "validating"
	StatePlanning    DeploymentState = "planning"
	StateDeploying   DeploymentState = "deploying"
	StateVerifying   DeploymentState = "verifying"
	StateActive      DeploymentState = "active"
	StateFailed      DeploymentState = "failed"
	StateRollingBack DeploymentState = "rolling_back"
	StateDestroyed   DeploymentState = "destroyed"
)

// Service represents a service to deploy.
type Service struct {
	Name         string
	Image        string
	Replicas     int
	Dependencies []string
	Ports        []PortMapping
	Environment  map[string]string
	Resources    *ResourceSpec
	HealthCheck  *HealthCheckSpec
	Sidecars     []SidecarSpec
}

// ExecutionPlan defines the order and parallelism of deployment.
type ExecutionPlan struct {
	DeploymentID string
	Phases       []ExecutionPhase
	CreatedAt    time.Time
}

// ExecutionPhase groups services that can deploy in parallel.
type ExecutionPhase struct {
	Order    int
	Services []string
	Parallel bool
}
