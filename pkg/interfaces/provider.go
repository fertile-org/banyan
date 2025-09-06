package interfaces

import "context"

// Provider defines the interface for deployment providers (Docker, Kubernetes, etc.)
type Provider interface {
	// Name returns the provider name
	Name() string
	
	// Deploy executes a deployment
	Deploy(ctx context.Context, config DeploymentConfig) error
	
	// Validate checks if the deployment configuration is valid
	Validate(config DeploymentConfig) error
	
	// Status returns the current deployment status
	Status(ctx context.Context, deploymentID string) (DeploymentStatus, error)
}

// DeploymentConfig represents deployment configuration
type DeploymentConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image,omitempty"`
	Compose     string            `json:"compose,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// DeploymentStatus represents deployment status
type DeploymentStatus struct {
	ID     string `json:"id"`
	State  string `json:"state"` // running, stopped, failed, pending
	Health string `json:"health"` // healthy, unhealthy, unknown
}