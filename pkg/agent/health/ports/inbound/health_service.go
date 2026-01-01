package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
)

// HealthService is the inbound port for health monitoring operations.
type HealthService interface {
	// RegisterCheck registers a health check for a container.
	RegisterCheck(ctx context.Context, check *domain.HealthCheck) error

	// UnregisterCheck removes a health check.
	UnregisterCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) error

	// GetContainerHealth gets the health status of a container.
	GetContainerHealth(ctx context.Context, containerID domain.ContainerID) (*domain.ContainerHealth, error)

	// ListContainerHealths lists health status for all containers.
	ListContainerHealths(ctx context.Context) ([]*domain.ContainerHealth, error)

	// RunCheck manually runs a health check and returns the result.
	RunCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.CheckResult, error)

	// GetAgentHealth gets the health status of the agent itself.
	GetAgentHealth(ctx context.Context) (*domain.AgentHealth, error)
}
