package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
)

// MemoryTaskStoreAdapter is an in-memory implementation of TaskStore.
type MemoryTaskStoreAdapter struct {
	tasks map[domain.TaskID]*domain.Task
	mu    sync.RWMutex
}

// NewMemoryTaskStoreAdapter creates a new in-memory task store.
func NewMemoryTaskStoreAdapter() *MemoryTaskStoreAdapter {
	return &MemoryTaskStoreAdapter{
		tasks: make(map[domain.TaskID]*domain.Task),
	}
}

// Save saves a task.
func (s *MemoryTaskStoreAdapter) Save(_ context.Context, task *domain.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Deep copy to prevent external mutations
	taskCopy := *task
	s.tasks[task.ID] = &taskCopy
	return nil
}

// Get retrieves a task by ID.
func (s *MemoryTaskStoreAdapter) Get(_ context.Context, taskID domain.TaskID) (*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	// Return a copy
	taskCopy := *task
	return &taskCopy, nil
}

// Update updates a task.
func (s *MemoryTaskStoreAdapter) Update(_ context.Context, task *domain.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.tasks[task.ID]; !ok {
		return domain.ErrTaskNotFound
	}
	// Deep copy to prevent external mutations
	taskCopy := *task
	s.tasks[task.ID] = &taskCopy
	return nil
}

// Delete deletes a task.
func (s *MemoryTaskStoreAdapter) Delete(_ context.Context, taskID domain.TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, taskID)
	return nil
}

// List lists tasks matching the filter.
func (s *MemoryTaskStoreAdapter) List(_ context.Context, filter *domain.TaskFilter) ([]*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*domain.Task
	for _, task := range s.tasks {
		if s.matchesFilter(task, filter) {
			// Return copies
			taskCopy := *task
			tasks = append(tasks, &taskCopy)
		}
	}

	// Apply pagination
	if filter.Offset > 0 && filter.Offset < len(tasks) {
		tasks = tasks[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(tasks) {
		tasks = tasks[:filter.Limit]
	}

	return tasks, nil
}

// GetPending gets all pending tasks.
func (s *MemoryTaskStoreAdapter) GetPending(_ context.Context) ([]*domain.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tasks []*domain.Task
	for _, task := range s.tasks {
		if task.State == domain.TaskStatePending || task.State == domain.TaskStateQueued {
			taskCopy := *task
			tasks = append(tasks, &taskCopy)
		}
	}
	return tasks, nil
}

// matchesFilter checks if a task matches the given filter.
func (s *MemoryTaskStoreAdapter) matchesFilter(task *domain.Task, filter *domain.TaskFilter) bool {
	// Check states
	if len(filter.States) > 0 {
		found := false
		for _, state := range filter.States {
			if task.State == state {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check types
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if task.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check time range
	if !filter.Since.IsZero() && task.CreatedAt.Before(filter.Since) {
		return false
	}
	if !filter.Until.IsZero() && task.CreatedAt.After(filter.Until) {
		return false
	}

	return true
}

// Ensure MemoryTaskStoreAdapter implements TaskStore.
var _ outbound.TaskStore = (*MemoryTaskStoreAdapter)(nil)
