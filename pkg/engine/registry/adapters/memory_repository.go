// Package adapters provides implementations of the outbound ports for the Agent Registry.
package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
)

// MemoryAgentRepository implements AgentRepository using in-memory storage.
type MemoryAgentRepository struct {
	agents   map[domain.AgentID]*domain.Agent
	mu       sync.RWMutex
	watchers []chan domain.AgentEvent
}

// NewMemoryAgentRepository creates a new MemoryAgentRepository.
func NewMemoryAgentRepository() *MemoryAgentRepository {
	return &MemoryAgentRepository{
		agents: make(map[domain.AgentID]*domain.Agent),
	}
}

// Save saves an agent to the repository.
func (r *MemoryAgentRepository) Save(ctx context.Context, agent *domain.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Make a copy to avoid external mutations
	agentCopy := *agent
	r.agents[agent.ID] = &agentCopy

	return nil
}

// FindByID finds an agent by its ID.
func (r *MemoryAgentRepository) FindByID(ctx context.Context, id domain.AgentID) (*domain.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, domain.ErrAgentNotFound
	}

	// Return a copy to avoid external mutations
	agentCopy := *agent
	return &agentCopy, nil
}

// FindAll returns all agents.
func (r *MemoryAgentRepository) FindAll(ctx context.Context) ([]domain.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]domain.Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		agents = append(agents, *agent)
	}
	return agents, nil
}

// FindByStatus returns all agents with the given status.
func (r *MemoryAgentRepository) FindByStatus(ctx context.Context, status domain.AgentStatus) ([]domain.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []domain.Agent
	for _, agent := range r.agents {
		if agent.Status == status {
			agents = append(agents, *agent)
		}
	}
	return agents, nil
}

// FindByCapability returns all agents with the given capability.
func (r *MemoryAgentRepository) FindByCapability(ctx context.Context, capType domain.CapabilityType) ([]domain.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []domain.Agent
	for _, agent := range r.agents {
		for _, c := range agent.Capabilities {
			if c.Type == capType {
				agents = append(agents, *agent)
				break
			}
		}
	}
	return agents, nil
}

// FindByLabels returns all agents matching the given labels.
func (r *MemoryAgentRepository) FindByLabels(ctx context.Context, labels map[string]string) ([]domain.Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var agents []domain.Agent
	for _, agent := range r.agents {
		if matchesLabels(agent.Labels, labels) {
			agents = append(agents, *agent)
		}
	}
	return agents, nil
}

// Delete removes an agent from the repository.
func (r *MemoryAgentRepository) Delete(ctx context.Context, id domain.AgentID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.agents[id]; !ok {
		return domain.ErrAgentNotFound
	}

	delete(r.agents, id)
	return nil
}

// Watch returns a channel that receives agent events.
func (r *MemoryAgentRepository) Watch(ctx context.Context) (<-chan domain.AgentEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan domain.AgentEvent, 100)
	r.watchers = append(r.watchers, ch)

	// Clean up when context is cancelled
	go func() {
		<-ctx.Done()
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, w := range r.watchers {
			if w == ch {
				r.watchers = append(r.watchers[:i], r.watchers[i+1:]...)
				close(ch)
				break
			}
		}
	}()

	return ch, nil
}

func matchesLabels(agentLabels, required map[string]string) bool {
	for k, v := range required {
		if agentLabels[k] != v {
			return false
		}
	}
	return true
}
