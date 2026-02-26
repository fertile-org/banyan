package agent

import "sync"

// trackedContainer holds info about containers created by this agent.
type trackedContainer struct {
	containerName string
	taskID        string
	containerIP   string
}

// containerTracker tracks containers created by this agent.
type containerTracker struct {
	containers []trackedContainer
	mu         sync.Mutex
}

func (t *containerTracker) Add(containerName, taskID, containerIP string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.containers = append(t.containers, trackedContainer{
		containerName: containerName,
		taskID:        taskID,
		containerIP:   containerIP,
	})
}

func (t *containerTracker) List() []trackedContainer {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]trackedContainer, len(t.containers))
	copy(result, t.containers)
	return result
}

// GetIP returns the container IP for the given container name, or empty string if not found.
func (t *containerTracker) GetIP(containerName string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, c := range t.containers {
		if c.containerName == containerName {
			return c.containerIP
		}
	}
	return ""
}
