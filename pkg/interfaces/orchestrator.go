package interfaces

import "context"

// Orchestrator defines the interface for the deployment orchestrator
type Orchestrator interface {
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
	
	// GetCapabilities returns what this agent can deploy
	GetCapabilities() AgentCapabilities
}

// DeploymentRequest represents a deployment request from CLI to Engine
type DeploymentRequest struct {
	DeploymentConfig
	Targets []string `json:"targets"` // List of agent IDs or hostnames
}

// AgentStatus represents status from a specific agent
type AgentStatus struct {
	AgentID    string           `json:"agent_id"`
	Deployment DeploymentStatus `json:"deployment"`
}

// AgentCapabilities represents what an agent can deploy
type AgentCapabilities struct {
	Providers []string `json:"providers"` // docker, kubernetes, etc.
	Version   string   `json:"version"`
}