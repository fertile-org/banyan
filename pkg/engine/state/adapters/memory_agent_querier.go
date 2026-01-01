package adapters

import (
	"context"
	"sync"
	"time"

	stateerrors "github.com/fertile-org/banyan/pkg/engine/state/errors"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

// MemoryAgentQuerier is an in-memory implementation of AgentQuerier for testing.
type MemoryAgentQuerier struct {
	agents map[string]*outbound.AgentState
	mu     sync.RWMutex
}

// NewMemoryAgentQuerier creates a new in-memory agent querier.
func NewMemoryAgentQuerier() *MemoryAgentQuerier {
	return &MemoryAgentQuerier{
		agents: make(map[string]*outbound.AgentState),
	}
}

// GetAgentState retrieves the state of an agent.
func (q *MemoryAgentQuerier) GetAgentState(_ context.Context, agentID string) (*outbound.AgentState, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	state, ok := q.agents[agentID]
	if !ok {
		return nil, stateerrors.ErrAgentNotFound
	}
	return state, nil
}

// ListAgentStates lists all agent states.
func (q *MemoryAgentQuerier) ListAgentStates(_ context.Context) ([]*outbound.AgentState, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	states := make([]*outbound.AgentState, 0, len(q.agents))
	for _, state := range q.agents {
		states = append(states, state)
	}
	return states, nil
}

// SetAgentState sets the state of an agent (for testing).
func (q *MemoryAgentQuerier) SetAgentState(agentID string, containers []outbound.ContainerInfo) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.agents[agentID] = &outbound.AgentState{
		AgentID:     agentID,
		Containers:  containers,
		Health:      "healthy",
		CollectedAt: time.Now(),
	}
}

// RemoveAgent removes an agent (for testing).
func (q *MemoryAgentQuerier) RemoveAgent(agentID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.agents, agentID)
}

// Ensure MemoryAgentQuerier implements AgentQuerier.
var _ outbound.AgentQuerier = (*MemoryAgentQuerier)(nil)
