// Package outbound defines the outbound ports (repository interfaces) for the Agent Registry.
package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
)

// AgentRepository defines persistence operations for agents.
type AgentRepository interface {
	// Save saves an agent to the repository.
	Save(ctx context.Context, agent *domain.Agent) error
	// FindByID finds an agent by its ID.
	FindByID(ctx context.Context, id domain.AgentID) (*domain.Agent, error)
	// FindAll returns all agents.
	FindAll(ctx context.Context) ([]domain.Agent, error)
	// FindByStatus returns all agents with the given status.
	FindByStatus(ctx context.Context, status domain.AgentStatus) ([]domain.Agent, error)
	// FindByCapability returns all agents with the given capability.
	FindByCapability(ctx context.Context, cap domain.CapabilityType) ([]domain.Agent, error)
	// FindByLabels returns all agents matching the given labels.
	FindByLabels(ctx context.Context, labels map[string]string) ([]domain.Agent, error)
	// Delete removes an agent from the repository.
	Delete(ctx context.Context, id domain.AgentID) error
	// Watch returns a channel that receives agent events.
	Watch(ctx context.Context) (<-chan domain.AgentEvent, error)
}

// EventPublisher publishes agent events.
type EventPublisher interface {
	// Publish publishes an agent event.
	Publish(ctx context.Context, event domain.AgentEvent) error
	// Subscribe subscribes to agent events.
	Subscribe(ctx context.Context) (<-chan domain.AgentEvent, error)
}
