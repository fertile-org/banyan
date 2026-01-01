package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TaskID uniquely identifies a task.
type TaskID string

// NewTaskID creates a new unique TaskID.
func NewTaskID() TaskID {
	return TaskID(uuid.New().String())
}

// TaskType defines the category of task.
type TaskType string

const (
	// Container operations
	TaskTypeContainerCreate  TaskType = "container.create"
	TaskTypeContainerStart   TaskType = "container.start"
	TaskTypeContainerStop    TaskType = "container.stop"
	TaskTypeContainerRemove  TaskType = "container.remove"
	TaskTypeContainerRestart TaskType = "container.restart"

	// Network operations
	TaskTypeNetworkConnect    TaskType = "network.connect"
	TaskTypeNetworkDisconnect TaskType = "network.disconnect"

	// Security operations
	TaskTypeSecurityApply  TaskType = "security.apply"
	TaskTypeSecurityRemove TaskType = "security.remove"

	// Health operations
	TaskTypeHealthCheck TaskType = "health.check"
)

// TaskPriority defines task priority levels.
type TaskPriority int

const (
	PriorityLow      TaskPriority = 0
	PriorityNormal   TaskPriority = 50
	PriorityHigh     TaskPriority = 75
	PriorityCritical TaskPriority = 100
)

// TaskState represents the current state of a task.
type TaskState string

const (
	TaskStatePending   TaskState = "pending"
	TaskStateQueued    TaskState = "queued"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
	TaskStateRetrying  TaskState = "retrying"
)

// Task represents a unit of work to be executed.
type Task struct {
	Payload     TaskPayload
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Result      *TaskResult
	Metadata    map[string]string
	State       TaskState
	Type        TaskType
	ID          TaskID
	Error       string
	Priority    TaskPriority
	RetryCount  int
	MaxRetries  int
	Timeout     time.Duration
}

// TaskResult contains the outcome of a task execution.
type TaskResult struct {
	Output   interface{}
	TaskID   TaskID
	Error    string
	Duration time.Duration
	Success  bool
}

// TaskPayload contains type-specific task data.
type TaskPayload struct {
	Container *ContainerTaskPayload
	Network   *NetworkTaskPayload
	Security  *SecurityTaskPayload
	Health    *HealthTaskPayload
}

// ContainerOperation types.
type ContainerOperation string

const (
	ContainerOpCreate  ContainerOperation = "create"
	ContainerOpStart   ContainerOperation = "start"
	ContainerOpStop    ContainerOperation = "stop"
	ContainerOpRemove  ContainerOperation = "remove"
	ContainerOpRestart ContainerOperation = "restart"
)

// ContainerTaskPayload for container operations.
type ContainerTaskPayload struct {
	Environment map[string]string
	Labels      map[string]string
	Operation   ContainerOperation
	ContainerID string
	Image       string
	Name        string
	Command     []string
	Timeout     time.Duration
}

// NetworkOperation types.
type NetworkOperation string

const (
	NetworkOpConnect    NetworkOperation = "connect"
	NetworkOpDisconnect NetworkOperation = "disconnect"
)

// NetworkTaskPayload for network operations.
type NetworkTaskPayload struct {
	Operation   NetworkOperation
	ContainerID string
	NetworkID   string
	IPAddress   string
}

// SecurityOperation types.
type SecurityOperation string

const (
	SecurityOpApply  SecurityOperation = "apply"
	SecurityOpRemove SecurityOperation = "remove"
)

// SecurityTaskPayload for security operations.
type SecurityTaskPayload struct {
	Operation SecurityOperation
	PolicyID  string
	Rules     []SecurityRulePayload
}

// SecurityRulePayload represents a security rule in a task.
type SecurityRulePayload struct {
	Direction string
	Protocol  string
	PortRange string
	Source    string
	Action    string
	Port      int
}

// HealthTaskPayload for health check operations.
type HealthTaskPayload struct {
	ContainerID string
	CheckType   string // "http", "tcp", "exec"
	Target      string // URL, address, or command
	Timeout     time.Duration
}

// TaskStatus represents current task status.
type TaskStatus struct {
	StartedAt *time.Time
	Duration  *time.Duration
	TaskID    TaskID
	State     TaskState
	Message   string
	Progress  float64
}

// TaskFilter for querying tasks.
type TaskFilter struct {
	Since  time.Time
	Until  time.Time
	States []TaskState
	Types  []TaskType
	Limit  int
	Offset int
}

// TaskEventType defines types of task events.
type TaskEventType string

const (
	TaskEventQueued    TaskEventType = "queued"
	TaskEventStarted   TaskEventType = "started"
	TaskEventProgress  TaskEventType = "progress"
	TaskEventCompleted TaskEventType = "completed"
	TaskEventFailed    TaskEventType = "failed"
	TaskEventRetrying  TaskEventType = "retrying"
	TaskEventCancelled TaskEventType = "cancelled"
)

// TaskEvent represents a task lifecycle event.
type TaskEvent struct {
	Timestamp time.Time
	Data      map[string]interface{}
	TaskID    TaskID
	EventType TaskEventType
}

// CanTransitionTo checks if the task can transition to the new state.
func (t *Task) CanTransitionTo(newState TaskState) bool {
	validTransitions := map[TaskState][]TaskState{
		TaskStatePending:   {TaskStateQueued, TaskStateCancelled},
		TaskStateQueued:    {TaskStateRunning, TaskStateCancelled},
		TaskStateRunning:   {TaskStateCompleted, TaskStateFailed, TaskStateCancelled},
		TaskStateFailed:    {TaskStateRetrying, TaskStateCancelled},
		TaskStateRetrying:  {TaskStateQueued},
		TaskStateCompleted: {}, // Terminal
		TaskStateCancelled: {}, // Terminal
	}

	allowed, exists := validTransitions[t.State]
	if !exists {
		return false
	}

	for _, s := range allowed {
		if s == newState {
			return true
		}
	}
	return false
}

// TransitionTo transitions the task to a new state.
func (t *Task) TransitionTo(newState TaskState) error {
	if !t.CanTransitionTo(newState) {
		return fmt.Errorf("invalid transition from %s to %s", t.State, newState)
	}

	now := time.Now()
	switch newState {
	case TaskStateRunning:
		t.StartedAt = &now
	case TaskStateCompleted, TaskStateFailed, TaskStateCancelled:
		t.CompletedAt = &now
	case TaskStateRetrying:
		t.RetryCount++
	}

	t.State = newState
	return nil
}

// ShouldRetry checks if the task should be retried.
func (t *Task) ShouldRetry() bool {
	return t.State == TaskStateFailed &&
		t.RetryCount < t.MaxRetries &&
		t.isRetryableError()
}

// isRetryableError checks if the error is retryable.
func (t *Task) isRetryableError() bool {
	if t.Error == "" {
		return false
	}
	// Network errors, timeouts are retryable
	retryablePatterns := []string{
		"connection refused",
		"timeout",
		"temporary",
		"unavailable",
		"deadline exceeded",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(t.Error), pattern) {
			return true
		}
	}
	return false
}

// RetryDelay calculates retry delay with exponential backoff.
func (t *Task) RetryDelay() time.Duration {
	base := 1 * time.Second
	maxDelay := 30 * time.Second

	// Guard against overflow - limit shift amount
	shift := t.RetryCount
	if shift > 30 {
		shift = 30
	}
	delay := base * time.Duration(1<<uint(shift)) //nolint:gosec // guarded above
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// Validate validates the task.
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task ID is required")
	}
	if t.Type == "" {
		return fmt.Errorf("task type is required")
	}
	if t.Timeout <= 0 {
		t.Timeout = 5 * time.Minute // Default timeout
	}
	if t.MaxRetries < 0 {
		t.MaxRetries = 0
	}

	// Validate payload based on type
	switch {
	case strings.HasPrefix(string(t.Type), "container."):
		if t.Payload.Container == nil {
			return fmt.Errorf("container payload is required for task type %s", t.Type)
		}
	case strings.HasPrefix(string(t.Type), "network."):
		if t.Payload.Network == nil {
			return fmt.Errorf("network payload is required for task type %s", t.Type)
		}
	case strings.HasPrefix(string(t.Type), "security."):
		if t.Payload.Security == nil {
			return fmt.Errorf("security payload is required for task type %s", t.Type)
		}
	case strings.HasPrefix(string(t.Type), "health."):
		if t.Payload.Health == nil {
			return fmt.Errorf("health payload is required for task type %s", t.Type)
		}
	}

	return nil
}

// IsTerminal returns true if the task is in a terminal state.
// Failed is not terminal because it can transition to retrying.
func (t *Task) IsTerminal() bool {
	return t.State == TaskStateCompleted ||
		t.State == TaskStateCancelled
}
