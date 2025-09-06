package interfaces

import "context"

// Engine defines the interface for the deployment engine
type Engine interface {
	// Deploy schedules a deployment across agents
	Deploy(ctx context.Context, request DeploymentRequest) error
	
	// GetStatus retrieves deployment status from all agents
	GetStatus(ctx context.Context, deploymentID string) ([]AgentStatus, error)
	
	// Cancel cancels an ongoing deployment
	Cancel(ctx context.Context, deploymentID string) error
}

// Agent defines the interface for deployment agents
type Agent interface {
	// Execute runs a deployment on this agent
	Execute(ctx context.Context, config DeploymentConfig) error
	
	// HealthCheck returns agent health status
	HealthCheck(ctx context.Context) error
}

// DeploymentConfig represents deployment configuration
type DeploymentConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}

// DeploymentRequest represents a deployment request from CLI to Engine
type DeploymentRequest struct {
	DeploymentConfig
	Targets []string `json:"targets"` // List of agent IDs or hostnames
}

// DeploymentStatus represents deployment status
type DeploymentStatus struct {
	ID    string `json:"id"`
	State string `json:"state"` // running, stopped, failed, pending
}

// AgentStatus represents status from a specific agent
type AgentStatus struct {
	AgentID    string           `json:"agent_id"`
	Deployment DeploymentStatus `json:"deployment"`
}