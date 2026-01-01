package domain

import (
	"encoding/json"
	"time"
)

// EventType represents the type of domain event.
type EventType string

// Event types for deployments.
const (
	EventTypeDeploymentCreated   EventType = "deployment.created"
	EventTypeDeploymentStarted   EventType = "deployment.started"
	EventTypeDeploymentCompleted EventType = "deployment.completed"
	EventTypeDeploymentFailed    EventType = "deployment.failed"
	EventTypeDeploymentCancelled EventType = "deployment.cancelled"
	EventTypeDeploymentUpdated   EventType = "deployment.updated"
)

// Event types for agents.
const (
	EventTypeAgentRegistered    EventType = "agent.registered"
	EventTypeAgentDeregistered  EventType = "agent.deregistered"
	EventTypeAgentHealthChanged EventType = "agent.health_changed"
	EventTypeAgentHeartbeat     EventType = "agent.heartbeat"
	EventTypeAgentDraining      EventType = "agent.draining"
)

// Event types for tasks.
const (
	EventTypeTaskCreated   EventType = "task.created"
	EventTypeTaskStarted   EventType = "task.started"
	EventTypeTaskCompleted EventType = "task.completed"
	EventTypeTaskFailed    EventType = "task.failed"
	EventTypeTaskRetrying  EventType = "task.retrying"
	EventTypeTaskCancelled EventType = "task.cancelled"
)

// Event types for containers.
const (
	EventTypeContainerCreated   EventType = "container.created"
	EventTypeContainerStarted   EventType = "container.started"
	EventTypeContainerStopped   EventType = "container.stopped"
	EventTypeContainerRemoved   EventType = "container.removed"
	EventTypeContainerFailed    EventType = "container.failed"
	EventTypeContainerHealthy   EventType = "container.healthy"
	EventTypeContainerUnhealthy EventType = "container.unhealthy"
)

// Event types for networking.
const (
	EventTypeNetworkProvisioned EventType = "network.provisioned"
	EventTypeNetworkReleased    EventType = "network.released"
	EventTypeIPAllocated        EventType = "ip.allocated"
	EventTypeIPReleased         EventType = "ip.released"
	EventTypeSecurityApplied    EventType = "security.applied"
	EventTypeSecurityRemoved    EventType = "security.removed"
)

// Event types for plugins.
const (
	EventTypePluginStarted   EventType = "plugin.started"
	EventTypePluginCompleted EventType = "plugin.completed"
	EventTypePluginFailed    EventType = "plugin.failed"
)

// Event types for state.
const (
	EventTypeStateDriftDetected  EventType = "state.drift_detected"
	EventTypeStateReconciled     EventType = "state.reconciled"
	EventTypeStateUpdateReceived EventType = "state.update_received"
)

func (t EventType) String() string { return string(t) }

// Event represents a domain event that occurred in the system.
type Event struct {
	Timestamp time.Time         `json:"timestamp"`
	Data      any               `json:"data,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	ID        string            `json:"id"`
	Type      EventType         `json:"type"`
	Source    string            `json:"source"`
}

// NewEvent creates a new domain event.
func NewEvent(eventType EventType, source string, data any) *Event {
	return &Event{
		ID:        generateID("evt"),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}

// WithMetadata adds metadata to the event.
func (e *Event) WithMetadata(key, value string) *Event {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// MarshalJSON implements json.Marshaler.
func (e *Event) MarshalJSON() ([]byte, error) {
	type alias Event
	return json.Marshal((*alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *Event) UnmarshalJSON(data []byte) error {
	type alias Event
	return json.Unmarshal(data, (*alias)(e))
}

// DeploymentEventData contains data for deployment-related events.
type DeploymentEventData struct {
	DeploymentID DeploymentID     `json:"deployment_id"`
	Name         string           `json:"name,omitempty"`
	Status       DeploymentStatus `json:"status,omitempty"`
	Error        string           `json:"error,omitempty"`
	Progress     float64          `json:"progress,omitempty"` // 0.0 to 1.0
}

// AgentEventData contains data for agent-related events.
type AgentEventData struct {
	AgentID   AgentID     `json:"agent_id"`
	Hostname  string      `json:"hostname,omitempty"`
	Status    AgentStatus `json:"status,omitempty"`
	OldStatus AgentStatus `json:"old_status,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// TaskEventData contains data for task-related events.
type TaskEventData struct {
	TaskID       TaskID       `json:"task_id"`
	DeploymentID DeploymentID `json:"deployment_id,omitempty"`
	AgentID      AgentID      `json:"agent_id,omitempty"`
	Type         TaskType     `json:"type,omitempty"`
	Status       TaskStatus   `json:"status,omitempty"`
	Error        string       `json:"error,omitempty"`
	RetryCount   int          `json:"retry_count,omitempty"`
}

// ContainerEventData contains data for container-related events.
type ContainerEventData struct {
	ContainerID ContainerID     `json:"container_id"`
	ServiceID   ServiceID       `json:"service_id,omitempty"`
	AgentID     AgentID         `json:"agent_id,omitempty"`
	Name        string          `json:"name,omitempty"`
	Image       string          `json:"image,omitempty"`
	Status      ContainerStatus `json:"status,omitempty"`
	Error       string          `json:"error,omitempty"`
	ExitCode    int             `json:"exit_code,omitempty"`
}

// NetworkEventData contains data for network-related events.
type NetworkEventData struct {
	VPCID           VPCID           `json:"vpc_id,omitempty"`
	SubnetID        SubnetID        `json:"subnet_id,omitempty"`
	SecurityGroupID SecurityGroupID `json:"security_group_id,omitempty"`
	ContainerID     ContainerID     `json:"container_id,omitempty"`
	IPAddress       string          `json:"ip_address,omitempty"`
	Error           string          `json:"error,omitempty"`
}

// PluginEventData contains data for plugin-related events.
type PluginEventData struct {
	PluginID     PluginID     `json:"plugin_id"`
	DeploymentID DeploymentID `json:"deployment_id,omitempty"`
	HookPoint    HookPoint    `json:"hook_point,omitempty"`
	Error        string       `json:"error,omitempty"`
	Duration     Duration     `json:"duration,omitempty"`
}

// StateEventData contains data for state-related events.
type StateEventData struct {
	DeploymentID DeploymentID `json:"deployment_id,omitempty"`
	ServiceID    ServiceID    `json:"service_id,omitempty"`
	DriftType    string       `json:"drift_type,omitempty"`
	Expected     string       `json:"expected,omitempty"`
	Actual       string       `json:"actual,omitempty"`
}

// EventHandler is a function that handles domain events.
type EventHandler func(event *Event) error

// EventPublisher defines the interface for publishing domain events.
type EventPublisher interface {
	Publish(event *Event) error
	PublishAsync(event *Event)
}

// EventSubscriber defines the interface for subscribing to domain events.
type EventSubscriber interface {
	Subscribe(eventType EventType, handler EventHandler) error
	Unsubscribe(eventType EventType, handler EventHandler) error
}

// EventBus combines publisher and subscriber capabilities.
type EventBus interface {
	EventPublisher
	EventSubscriber
	Close() error
}
