package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
)

// OrchestratorService defines what the orchestrator offers.
type OrchestratorService interface {
	// CreateDeployment initializes a new deployment.
	CreateDeployment(ctx context.Context, req CreateDeploymentRequest) (*domain.Deployment, error)

	// ExecuteDeployment runs the full deployment pipeline.
	ExecuteDeployment(ctx context.Context, deploymentID string) error

	// GetDeploymentPlan returns execution plan without executing.
	GetDeploymentPlan(ctx context.Context, req CreateDeploymentRequest) (*domain.ExecutionPlan, error)

	// RollbackDeployment reverts to previous state.
	RollbackDeployment(ctx context.Context, deploymentID string, strategy RollbackStrategy) error

	// CancelDeployment stops an in-progress deployment.
	CancelDeployment(ctx context.Context, deploymentID string) error

	// GetDeployment retrieves deployment by ID.
	GetDeployment(ctx context.Context, deploymentID string) (*domain.Deployment, error)

	// ListDeployments lists all deployments.
	ListDeployments(ctx context.Context, filter DeploymentFilter) ([]*domain.Deployment, error)

	// DeleteDeployment removes a deployment.
	DeleteDeployment(ctx context.Context, deploymentID string) error
}

// CreateDeploymentRequest contains deployment parameters.
type CreateDeploymentRequest struct {
	Name       string
	BanyanFile string            // banyan.yml content
	Targets    []string          // Agent selectors (optional, auto-select if empty)
	Variables  map[string]string // Variable substitution (e.g., ${DB_PASSWORD})
}

// RollbackStrategy defines how to rollback.
type RollbackStrategy string

const (
	RollbackImmediate RollbackStrategy = "immediate" // Stop all, redeploy previous
	RollbackGraceful  RollbackStrategy = "graceful"  // Rolling rollback
)

// DeploymentFilter for listing deployments.
type DeploymentFilter struct {
	State  *domain.DeploymentState
	Name   *string
	Limit  int
	Offset int
}
