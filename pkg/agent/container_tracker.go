package agent

import "sync"

// trackedContainer holds info about containers created by this agent.
type trackedContainer struct {
	containerName string
	taskID        string
}

// containerTracker tracks containers created by this agent.
type containerTracker struct {
	containers []trackedContainer
	mu         sync.Mutex
}

func (t *containerTracker) Add(containerName, taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.containers = append(t.containers, trackedContainer{
		containerName: containerName,
		taskID:        taskID,
	})
}

func (t *containerTracker) List() []trackedContainer {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]trackedContainer, len(t.containers))
	copy(result, t.containers)
	return result
}
