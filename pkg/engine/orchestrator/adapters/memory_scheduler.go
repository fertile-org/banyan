package adapters

import (
	"context"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	orcerrors "github.com/fertile-org/banyan/pkg/engine/orchestrator/errors"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
)

// MemoryScheduler is an in-memory implementation for testing.
type MemoryScheduler struct{}

// NewMemoryScheduler creates a new in-memory scheduler.
func NewMemoryScheduler() *MemoryScheduler {
	return &MemoryScheduler{}
}

// Schedule creates an execution plan based on service dependencies.
func (s *MemoryScheduler) Schedule(_ context.Context, services []domain.Service, deps *domain.DependencyGraph) (*domain.ExecutionPlan, error) {
	if deps.HasCycle() {
		return nil, orcerrors.ErrCircularDependency
	}

	phases, err := deps.TopologicalSort()
	if err != nil {
		return nil, err
	}

	plan := &domain.ExecutionPlan{
		CreatedAt: time.Now(),
	}

	for i, phaseServices := range phases {
		plan.Phases = append(plan.Phases, domain.ExecutionPhase{
			Order:    i,
			Services: phaseServices,
			Parallel: len(phaseServices) > 1, // Parallelize if multiple services in phase
		})
	}

	return plan, nil
}

// ValidateDependencies validates that all dependencies exist and there are no cycles.
func (s *MemoryScheduler) ValidateDependencies(_ context.Context, deps *domain.DependencyGraph) error {
	if deps.HasCycle() {
		return orcerrors.ErrCircularDependency
	}

	// Check all dependencies exist
	for _, node := range deps.Nodes {
		for _, dep := range node.DependsOn {
			if _, exists := deps.Nodes[dep]; !exists {
				return orcerrors.ErrInvalidBanyanFile
			}
		}
	}

	return nil
}

// Ensure MemoryScheduler implements Scheduler.
var _ outbound.Scheduler = (*MemoryScheduler)(nil)
