package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
)

// MemoryAgentDispatcher is an in-memory implementation for testing.
type MemoryAgentDispatcher struct {
	tasks  []*outbound.Task
	mu     sync.Mutex
	agents map[string]*outbound.AgentStatus
}

// NewMemoryAgentDispatcher creates a new in-memory dispatcher.
func NewMemoryAgentDispatcher() *MemoryAgentDispatcher {
	return &MemoryAgentDispatcher{
		tasks: make([]*outbound.Task, 0),
		agents: map[string]*outbound.AgentStatus{
			"agent-1": {AgentID: "agent-1", Available: true, Health: "healthy"},
		},
	}
}

// DispatchTask sends a task to a specific agent.
func (d *MemoryAgentDispatcher) DispatchTask(_ context.Context, _ string, task *outbound.Task) (*outbound.TaskResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tasks = append(d.tasks, task)
	return &outbound.TaskResult{
		TaskID: task.ID,
		Status: "success",
		Output: make(map[string]interface{}),
	}, nil
}

// DispatchBatch sends tasks to multiple agents in parallel.
func (d *MemoryAgentDispatcher) DispatchBatch(_ context.Context, tasks map[string]*outbound.Task) (map[string]*outbound.TaskResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	results := make(map[string]*outbound.TaskResult)
	for agentID, task := range tasks {
		d.tasks = append(d.tasks, task)
		results[agentID] = &outbound.TaskResult{
			TaskID: task.ID,
			Status: "success",
			Output: make(map[string]interface{}),
		}
	}
	return results, nil
}

// GetAgentStatus retrieves current status of an agent.
func (d *MemoryAgentDispatcher) GetAgentStatus(_ context.Context, agentID string) (*outbound.AgentStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if status, ok := d.agents[agentID]; ok {
		return status, nil
	}
	return &outbound.AgentStatus{
		AgentID:   agentID,
		Available: false,
		Health:    "unknown",
	}, nil
}

// GetDispatchedTasks returns all dispatched tasks (for testing).
func (d *MemoryAgentDispatcher) GetDispatchedTasks() []*outbound.Task {
	d.mu.Lock()
	defer d.mu.Unlock()

	result := make([]*outbound.Task, len(d.tasks))
	copy(result, d.tasks)
	return result
}

// ClearTasks clears all dispatched tasks (for testing).
func (d *MemoryAgentDispatcher) ClearTasks() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.tasks = make([]*outbound.Task, 0)
}

// Ensure MemoryAgentDispatcher implements AgentDispatcher.
var _ outbound.AgentDispatcher = (*MemoryAgentDispatcher)(nil)
