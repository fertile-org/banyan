package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateerrors "github.com/fertile-org/banyan/pkg/engine/state/errors"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

// MemoryStateRepository is an in-memory implementation of StateRepository for testing.
type MemoryStateRepository struct {
	desiredStates map[string]*domain.DesiredState
	actualStates  map[string]*domain.ActualState
	mu            sync.RWMutex

	desiredWatchers []chan outbound.StateChange
	actualWatchers  []chan outbound.StateChange
	watcherMu       sync.Mutex
}

// NewMemoryStateRepository creates a new in-memory state repository.
func NewMemoryStateRepository() *MemoryStateRepository {
	return &MemoryStateRepository{
		desiredStates: make(map[string]*domain.DesiredState),
		actualStates:  make(map[string]*domain.ActualState),
	}
}

// SaveDesiredState saves the desired state.
func (r *MemoryStateRepository) SaveDesiredState(_ context.Context, state *domain.DesiredState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.desiredStates[state.DeploymentID]
	r.desiredStates[state.DeploymentID] = state

	// Notify watchers
	changeType := outbound.ChangeCreated
	if exists {
		changeType = outbound.ChangeUpdated
	}
	r.notifyDesiredWatchers(outbound.StateChange{
		Type:         changeType,
		DeploymentID: state.DeploymentID,
	})

	return nil
}

// GetDesiredState retrieves the desired state.
func (r *MemoryStateRepository) GetDesiredState(_ context.Context, deploymentID string) (*domain.DesiredState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, ok := r.desiredStates[deploymentID]
	if !ok {
		return nil, stateerrors.ErrDesiredStateNotFound
	}
	return state, nil
}

// DeleteDesiredState deletes the desired state.
func (r *MemoryStateRepository) DeleteDesiredState(_ context.Context, deploymentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.desiredStates, deploymentID)

	// Notify watchers
	r.notifyDesiredWatchers(outbound.StateChange{
		Type:         outbound.ChangeDeleted,
		DeploymentID: deploymentID,
	})

	return nil
}

// ListDesiredStates lists all desired states.
func (r *MemoryStateRepository) ListDesiredStates(_ context.Context) ([]*domain.DesiredState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make([]*domain.DesiredState, 0, len(r.desiredStates))
	for _, state := range r.desiredStates {
		states = append(states, state)
	}
	return states, nil
}

// SaveActualState saves the actual state.
func (r *MemoryStateRepository) SaveActualState(_ context.Context, state *domain.ActualState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.actualStates[state.DeploymentID]
	r.actualStates[state.DeploymentID] = state

	// Notify watchers
	changeType := outbound.ChangeCreated
	if exists {
		changeType = outbound.ChangeUpdated
	}
	r.notifyActualWatchers(outbound.StateChange{
		Type:         changeType,
		DeploymentID: state.DeploymentID,
	})

	return nil
}

// GetActualState retrieves the actual state.
func (r *MemoryStateRepository) GetActualState(_ context.Context, deploymentID string) (*domain.ActualState, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	state, ok := r.actualStates[deploymentID]
	if !ok {
		return nil, stateerrors.ErrActualStateNotFound
	}
	return state, nil
}

// WatchDesiredState returns a channel for watching desired state changes.
func (r *MemoryStateRepository) WatchDesiredState(_ context.Context) (<-chan outbound.StateChange, error) {
	r.watcherMu.Lock()
	defer r.watcherMu.Unlock()

	ch := make(chan outbound.StateChange, 10)
	r.desiredWatchers = append(r.desiredWatchers, ch)
	return ch, nil
}

// WatchActualState returns a channel for watching actual state changes.
func (r *MemoryStateRepository) WatchActualState(_ context.Context) (<-chan outbound.StateChange, error) {
	r.watcherMu.Lock()
	defer r.watcherMu.Unlock()

	ch := make(chan outbound.StateChange, 10)
	r.actualWatchers = append(r.actualWatchers, ch)
	return ch, nil
}

// notifyDesiredWatchers notifies all desired state watchers.
func (r *MemoryStateRepository) notifyDesiredWatchers(change outbound.StateChange) {
	r.watcherMu.Lock()
	defer r.watcherMu.Unlock()

	for _, ch := range r.desiredWatchers {
		select {
		case ch <- change:
		default:
			// Channel full, skip
		}
	}
}

// notifyActualWatchers notifies all actual state watchers.
func (r *MemoryStateRepository) notifyActualWatchers(change outbound.StateChange) {
	r.watcherMu.Lock()
	defer r.watcherMu.Unlock()

	for _, ch := range r.actualWatchers {
		select {
		case ch <- change:
		default:
			// Channel full, skip
		}
	}
}

// Ensure MemoryStateRepository implements StateRepository.
var _ outbound.StateRepository = (*MemoryStateRepository)(nil)
