package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

// MemoryActionDispatcher is an in-memory implementation of ActionDispatcher for testing.
type MemoryActionDispatcher struct {
	actions []*domain.ReconcileAction
	mu      sync.Mutex
}

// NewMemoryActionDispatcher creates a new in-memory action dispatcher.
func NewMemoryActionDispatcher() *MemoryActionDispatcher {
	return &MemoryActionDispatcher{
		actions: make([]*domain.ReconcileAction, 0),
	}
}

// Dispatch dispatches a single action.
func (d *MemoryActionDispatcher) Dispatch(_ context.Context, action *domain.ReconcileAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.actions = append(d.actions, action)
	return nil
}

// DispatchBatch dispatches multiple actions.
func (d *MemoryActionDispatcher) DispatchBatch(_ context.Context, actions []*domain.ReconcileAction) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.actions = append(d.actions, actions...)
	return nil
}

// GetDispatchedActions returns all dispatched actions (for testing).
func (d *MemoryActionDispatcher) GetDispatchedActions() []*domain.ReconcileAction {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]*domain.ReconcileAction, len(d.actions))
	copy(result, d.actions)
	return result
}

// ClearActions clears all dispatched actions (for testing).
func (d *MemoryActionDispatcher) ClearActions() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.actions = make([]*domain.ReconcileAction, 0)
}

// Ensure MemoryActionDispatcher implements ActionDispatcher.
var _ outbound.ActionDispatcher = (*MemoryActionDispatcher)(nil)
