package outbound

import "time"

// EventEmitter emits domain events.
// This is an outbound port for publishing events to the event bus.
type EventEmitter interface {
	// Emit emits an event.
	Emit(event interface{})
}

// ContainerStartedEvent is emitted when a container starts.
type ContainerStartedEvent struct {
	StartedAt   time.Time
	ContainerID string
	ServiceID   string
}

// ContainerStoppedEvent is emitted when a container stops.
type ContainerStoppedEvent struct {
	StoppedAt   time.Time
	ContainerID string
	ServiceID   string
	ExitCode    int
}

// ContainerRemovedEvent is emitted when a container is removed.
type ContainerRemovedEvent struct {
	RemovedAt   time.Time
	ContainerID string
	ServiceID   string
}

// ContainerHealthChangedEvent is emitted when a container's health status changes.
type ContainerHealthChangedEvent struct {
	ChangedAt   time.Time
	ContainerID string
	ServiceID   string
	OldStatus   string
	NewStatus   string
}
