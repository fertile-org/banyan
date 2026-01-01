package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	orcerrors "github.com/fertile-org/banyan/pkg/engine/orchestrator/errors"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
)

// MemoryDeploymentRepository is an in-memory implementation for testing.
type MemoryDeploymentRepository struct {
	deployments map[string]*domain.Deployment
	mu          sync.RWMutex
}

// NewMemoryDeploymentRepository creates a new in-memory repository.
func NewMemoryDeploymentRepository() *MemoryDeploymentRepository {
	return &MemoryDeploymentRepository{
		deployments: make(map[string]*domain.Deployment),
	}
}

// Save saves a deployment.
func (r *MemoryDeploymentRepository) Save(_ context.Context, deployment *domain.Deployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.deployments[deployment.ID] = deployment
	return nil
}

// Get retrieves a deployment by ID.
func (r *MemoryDeploymentRepository) Get(_ context.Context, id string) (*domain.Deployment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	deployment, ok := r.deployments[id]
	if !ok {
		return nil, orcerrors.ErrDeploymentNotFound
	}
	return deployment, nil
}

// Update updates a deployment.
func (r *MemoryDeploymentRepository) Update(_ context.Context, deployment *domain.Deployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.deployments[deployment.ID] = deployment
	return nil
}

// Delete deletes a deployment.
func (r *MemoryDeploymentRepository) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.deployments, id)
	return nil
}

// List lists all deployments.
func (r *MemoryDeploymentRepository) List(_ context.Context, filter inbound.DeploymentFilter) ([]*domain.Deployment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Deployment
	for _, d := range r.deployments {
		// Apply filters
		if filter.State != nil && d.State != *filter.State {
			continue
		}
		if filter.Name != nil && d.Name != *filter.Name {
			continue
		}
		result = append(result, d)
	}

	// Apply pagination
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}

	return result, nil
}

// Ensure MemoryDeploymentRepository implements DeploymentRepository.
var _ outbound.DeploymentRepository = (*MemoryDeploymentRepository)(nil)
