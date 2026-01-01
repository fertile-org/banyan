package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateerrors "github.com/fertile-org/banyan/pkg/engine/state/errors"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

// StateUseCase implements state management operations.
type StateUseCase struct {
	repo outbound.StateRepository
}

// NewStateUseCase creates a new StateUseCase.
func NewStateUseCase(repo outbound.StateRepository) *StateUseCase {
	return &StateUseCase{repo: repo}
}

// SetDesiredState sets the desired state for a deployment.
func (uc *StateUseCase) SetDesiredState(ctx context.Context, state *domain.DesiredState) error {
	if state == nil {
		return stateerrors.ErrInvalidState
	}
	state.UpdatedAt = time.Now()
	return uc.repo.SaveDesiredState(ctx, state)
}

// GetDesiredState retrieves the desired state for a deployment.
func (uc *StateUseCase) GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error) {
	return uc.repo.GetDesiredState(ctx, deploymentID)
}

// DeleteDesiredState deletes the desired state for a deployment.
func (uc *StateUseCase) DeleteDesiredState(ctx context.Context, deploymentID string) error {
	return uc.repo.DeleteDesiredState(ctx, deploymentID)
}

// UpdateActualState updates the actual state for a deployment.
func (uc *StateUseCase) UpdateActualState(ctx context.Context, state *domain.ActualState) error {
	if state == nil {
		return stateerrors.ErrInvalidState
	}
	state.CollectedAt = time.Now()
	return uc.repo.SaveActualState(ctx, state)
}

// GetActualState retrieves the actual state for a deployment.
func (uc *StateUseCase) GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error) {
	return uc.repo.GetActualState(ctx, deploymentID)
}

// DetectDrift detects drift between desired and actual state.
func (uc *StateUseCase) DetectDrift(ctx context.Context, deploymentID string) (*domain.StateDrift, error) {
	desired, err := uc.repo.GetDesiredState(ctx, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get desired state: %w", err)
	}

	actual, err := uc.repo.GetActualState(ctx, deploymentID)
	if err != nil {
		// If no actual state, create an empty one for comparison
		actual = &domain.ActualState{
			DeploymentID: deploymentID,
			Services:     make(map[string]domain.ServiceActualState),
		}
	}

	return uc.compareStates(desired, actual), nil
}

// GetDriftReport generates a drift report across all deployments.
func (uc *StateUseCase) GetDriftReport(ctx context.Context) (*domain.DriftReport, error) {
	states, err := uc.repo.ListDesiredStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list desired states: %w", err)
	}

	report := &domain.DriftReport{
		TotalDeployments: len(states),
		GeneratedAt:      time.Now(),
	}

	for _, state := range states {
		drift, err := uc.DetectDrift(ctx, state.DeploymentID)
		if err != nil {
			continue
		}

		if len(drift.Drifts) > 0 {
			report.DeploymentsWithDrift++

			switch drift.Severity {
			case domain.SeverityCritical:
				report.CriticalDrifts++
			case domain.SeverityHigh:
				report.HighDrifts++
			case domain.SeverityMedium:
				report.MediumDrifts++
			case domain.SeverityLow:
				report.LowDrifts++
			}
		}
	}

	return report, nil
}

// compareStates compares desired and actual state to detect drift.
func (uc *StateUseCase) compareStates(desired *domain.DesiredState, actual *domain.ActualState) *domain.StateDrift {
	drift := &domain.StateDrift{
		DeploymentID: desired.DeploymentID,
		DetectedAt:   time.Now(),
	}

	// Check for missing services
	for name, desiredSvc := range desired.Services {
		actualSvc, exists := actual.Services[name]
		if !exists {
			drift.Drifts = append(drift.Drifts, domain.Drift{
				Type:        domain.DriftMissing,
				ServiceName: name,
				Details:     "service not found",
			})
			continue
		}

		// Check replica count
		actualReplicas := len(actualSvc.Instances)
		if actualReplicas != desiredSvc.Replicas {
			drift.Drifts = append(drift.Drifts, domain.Drift{
				Type:        domain.DriftReplicas,
				ServiceName: name,
				Details:     fmt.Sprintf("expected %d replicas, got %d", desiredSvc.Replicas, actualReplicas),
			})
		}

		// Check instance health
		for _, inst := range actualSvc.Instances {
			if inst.Health == domain.HealthUnhealthy {
				drift.Drifts = append(drift.Drifts, domain.Drift{
					Type:        domain.DriftUnhealthy,
					ServiceName: name,
					AgentID:     inst.AgentID,
					Details:     fmt.Sprintf("instance %s unhealthy", inst.ContainerID),
				})
			}
		}

		// Check agent placement
		for _, inst := range actualSvc.Instances {
			if len(desiredSvc.AgentIDs) > 0 && !contains(desiredSvc.AgentIDs, inst.AgentID) {
				drift.Drifts = append(drift.Drifts, domain.Drift{
					Type:        domain.DriftWrongHost,
					ServiceName: name,
					AgentID:     inst.AgentID,
					Details:     "running on non-target agent",
				})
			}
		}
	}

	// Check for extra services
	for name := range actual.Services {
		if _, exists := desired.Services[name]; !exists {
			drift.Drifts = append(drift.Drifts, domain.Drift{
				Type:        domain.DriftExtra,
				ServiceName: name,
				Details:     "service should not exist",
			})
		}
	}

	// Calculate severity
	drift.Severity = uc.calculateSeverity(drift.Drifts)

	return drift
}

// calculateSeverity determines the overall severity of drifts.
func (uc *StateUseCase) calculateSeverity(drifts []domain.Drift) domain.DriftSeverity {
	if len(drifts) == 0 {
		return domain.SeverityLow
	}

	hasMissing := false
	hasUnhealthy := false

	for _, d := range drifts {
		if d.Type == domain.DriftMissing {
			hasMissing = true
		}
		if d.Type == domain.DriftUnhealthy {
			hasUnhealthy = true
		}
	}

	if hasMissing {
		return domain.SeverityCritical
	}
	if hasUnhealthy {
		return domain.SeverityHigh
	}
	return domain.SeverityMedium
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Ensure StateUseCase implements StateService.
var _ inbound.StateService = (*StateUseCase)(nil)
