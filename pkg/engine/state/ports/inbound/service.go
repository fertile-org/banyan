package inbound

import (
	"context"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
)

// StateService defines state management operations.
type StateService interface {
	// Desired state management
	SetDesiredState(ctx context.Context, state *domain.DesiredState) error
	GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error)
	DeleteDesiredState(ctx context.Context, deploymentID string) error

	// Actual state management
	UpdateActualState(ctx context.Context, state *domain.ActualState) error
	GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error)

	// Drift detection
	DetectDrift(ctx context.Context, deploymentID string) (*domain.StateDrift, error)
	GetDriftReport(ctx context.Context) (*domain.DriftReport, error)
}

// ReconcilerService defines reconciliation operations.
type ReconcilerService interface {
	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Manual triggers
	TriggerReconcile(ctx context.Context, deploymentID string) error
	TriggerReconcileAll(ctx context.Context) error

	// Configuration
	SetReconcileInterval(interval time.Duration)
	GetReconcileInterval() time.Duration

	// Status
	GetLastReconcileTime(ctx context.Context, deploymentID string) (time.Time, error)
	IsReconciling(ctx context.Context, deploymentID string) bool
}
