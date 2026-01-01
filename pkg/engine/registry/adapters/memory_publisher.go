// Package adapters provides implementations of the outbound ports for the Agent Registry.
package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
)

// MemoryEventPublisher implements EventPublisher using in-memory channels.
type MemoryEventPublisher struct {
	subscribers []chan domain.AgentEvent
	mu          sync.RWMutex
}

// NewMemoryEventPublisher creates a new MemoryEventPublisher.
func NewMemoryEventPublisher() *MemoryEventPublisher {
	return &MemoryEventPublisher{}
}

// Publish publishes an agent event to all subscribers.
func (p *MemoryEventPublisher) Publish(ctx context.Context, event domain.AgentEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, ch := range p.subscribers {
		select {
		case ch <- event:
		default:
			// Drop event if subscriber is slow
		}
	}

	return nil
}

// Subscribe subscribes to agent events.
func (p *MemoryEventPublisher) Subscribe(ctx context.Context) (<-chan domain.AgentEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan domain.AgentEvent, 100)
	p.subscribers = append(p.subscribers, ch)

	// Clean up when context is cancelled
	go func() {
		<-ctx.Done()
		p.mu.Lock()
		defer p.mu.Unlock()
		for i, s := range p.subscribers {
			if s == ch {
				p.subscribers = append(p.subscribers[:i], p.subscribers[i+1:]...)
				close(ch)
				break
			}
		}
	}()

	return ch, nil
}
