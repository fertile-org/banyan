package adapters

import (
	"sync"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
)

// MemoryEventEmitterAdapter is an in-memory implementation of EventEmitter.
type MemoryEventEmitterAdapter struct {
	subscribers map[chan domain.TaskEvent]struct{}
	mu          sync.RWMutex
}

// NewMemoryEventEmitterAdapter creates a new in-memory event emitter.
func NewMemoryEventEmitterAdapter() *MemoryEventEmitterAdapter {
	return &MemoryEventEmitterAdapter{
		subscribers: make(map[chan domain.TaskEvent]struct{}),
	}
}

// Emit emits a task event to all subscribers.
func (e *MemoryEventEmitterAdapter) Emit(event domain.TaskEvent) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for ch := range e.subscribers {
		// Non-blocking send to avoid blocking on slow consumers
		select {
		case ch <- event:
		default:
			// Drop event if channel is full
		}
	}
}

// Subscribe subscribes to task events.
func (e *MemoryEventEmitterAdapter) Subscribe() <-chan domain.TaskEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Buffer the channel to reduce blocking
	ch := make(chan domain.TaskEvent, 100)
	e.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe unsubscribes from task events.
func (e *MemoryEventEmitterAdapter) Unsubscribe(ch <-chan domain.TaskEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find and remove the channel
	for sub := range e.subscribers {
		if sub == ch {
			delete(e.subscribers, sub)
			close(sub)
			break
		}
	}
}

// Ensure MemoryEventEmitterAdapter implements EventEmitter.
var _ outbound.EventEmitter = (*MemoryEventEmitterAdapter)(nil)
