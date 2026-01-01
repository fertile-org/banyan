package domain

import (
	"testing"
	"time"
)

func TestNewTaskID(t *testing.T) {
	id1 := NewTaskID()
	id2 := NewTaskID()

	if id1 == "" {
		t.Error("expected non-empty task ID")
	}

	if id1 == id2 {
		t.Error("expected unique task IDs")
	}
}

func TestTask_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name      string
		from      TaskState
		to        TaskState
		canChange bool
	}{
		{"pending to queued", TaskStatePending, TaskStateQueued, true},
		{"pending to cancelled", TaskStatePending, TaskStateCancelled, true},
		{"pending to running", TaskStatePending, TaskStateRunning, false},
		{"queued to running", TaskStateQueued, TaskStateRunning, true},
		{"queued to cancelled", TaskStateQueued, TaskStateCancelled, true},
		{"running to completed", TaskStateRunning, TaskStateCompleted, true},
		{"running to failed", TaskStateRunning, TaskStateFailed, true},
		{"running to cancelled", TaskStateRunning, TaskStateCancelled, true},
		{"running to queued", TaskStateRunning, TaskStateQueued, false},
		{"failed to retrying", TaskStateFailed, TaskStateRetrying, true},
		{"failed to cancelled", TaskStateFailed, TaskStateCancelled, true},
		{"retrying to queued", TaskStateRetrying, TaskStateQueued, true},
		{"completed to any", TaskStateCompleted, TaskStateRunning, false},
		{"cancelled to any", TaskStateCancelled, TaskStateRunning, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{State: tt.from}
			result := task.CanTransitionTo(tt.to)
			if result != tt.canChange {
				t.Errorf("CanTransitionTo(%s) = %v, want %v", tt.to, result, tt.canChange)
			}
		})
	}
}

func TestTask_TransitionTo(t *testing.T) {
	t.Run("valid transition updates state", func(t *testing.T) {
		task := &Task{State: TaskStatePending}
		err := task.TransitionTo(TaskStateQueued)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if task.State != TaskStateQueued {
			t.Errorf("expected state queued, got %s", task.State)
		}
	})

	t.Run("transition to running sets StartedAt", func(t *testing.T) {
		task := &Task{State: TaskStateQueued}
		err := task.TransitionTo(TaskStateRunning)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if task.StartedAt == nil {
			t.Error("expected StartedAt to be set")
		}
	})

	t.Run("transition to completed sets CompletedAt", func(t *testing.T) {
		task := &Task{State: TaskStateRunning}
		err := task.TransitionTo(TaskStateCompleted)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if task.CompletedAt == nil {
			t.Error("expected CompletedAt to be set")
		}
	})

	t.Run("transition to retrying increments RetryCount", func(t *testing.T) {
		task := &Task{State: TaskStateFailed, RetryCount: 1}
		err := task.TransitionTo(TaskStateRetrying)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if task.RetryCount != 2 {
			t.Errorf("expected RetryCount 2, got %d", task.RetryCount)
		}
	})

	t.Run("invalid transition returns error", func(t *testing.T) {
		task := &Task{State: TaskStatePending}
		err := task.TransitionTo(TaskStateRunning)
		if err == nil {
			t.Error("expected error for invalid transition")
		}
	})
}

func TestTask_ShouldRetry(t *testing.T) {
	tests := []struct {
		name        string
		state       TaskState
		retryCount  int
		maxRetries  int
		errMsg      string
		shouldRetry bool
	}{
		{"failed with retries left and retryable error", TaskStateFailed, 0, 3, "connection refused", true},
		{"failed with timeout error", TaskStateFailed, 0, 3, "context deadline exceeded", true},
		{"failed with temporary error", TaskStateFailed, 1, 3, "temporary failure", true},
		{"failed with no retries left", TaskStateFailed, 3, 3, "connection refused", false},
		{"failed with non-retryable error", TaskStateFailed, 0, 3, "invalid configuration", false},
		{"not failed state", TaskStateRunning, 0, 3, "connection refused", false},
		{"failed with empty error", TaskStateFailed, 0, 3, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				State:      tt.state,
				RetryCount: tt.retryCount,
				MaxRetries: tt.maxRetries,
				Error:      tt.errMsg,
			}
			result := task.ShouldRetry()
			if result != tt.shouldRetry {
				t.Errorf("ShouldRetry() = %v, want %v", result, tt.shouldRetry)
			}
		})
	}
}

func TestTask_RetryDelay(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		expected   time.Duration
	}{
		{"first retry", 0, 1 * time.Second},
		{"second retry", 1, 2 * time.Second},
		{"third retry", 2, 4 * time.Second},
		{"fourth retry", 3, 8 * time.Second},
		{"max delay cap", 10, 30 * time.Second},
		{"very high retry count", 100, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{RetryCount: tt.retryCount}
			result := task.RetryDelay()
			if result != tt.expected {
				t.Errorf("RetryDelay() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestTask_Validate(t *testing.T) {
	tests := []struct {
		name    string
		task    *Task
		wantErr bool
	}{
		{
			name:    "empty ID",
			task:    &Task{Type: TaskTypeContainerCreate},
			wantErr: true,
		},
		{
			name:    "empty Type",
			task:    &Task{ID: "task-1"},
			wantErr: true,
		},
		{
			name: "container task without payload",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeContainerCreate,
			},
			wantErr: true,
		},
		{
			name: "network task without payload",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeNetworkConnect,
			},
			wantErr: true,
		},
		{
			name: "security task without payload",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeSecurityApply,
			},
			wantErr: true,
		},
		{
			name: "health task without payload",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeHealthCheck,
			},
			wantErr: true,
		},
		{
			name: "valid container task",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeContainerCreate,
				Payload: TaskPayload{
					Container: &ContainerTaskPayload{Name: "test", Image: "nginx"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid network task",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeNetworkConnect,
				Payload: TaskPayload{
					Network: &NetworkTaskPayload{ContainerID: "c1", NetworkID: "n1"},
				},
			},
			wantErr: false,
		},
		{
			name: "sets default timeout",
			task: &Task{
				ID:   "task-1",
				Type: TaskTypeContainerCreate,
				Payload: TaskPayload{
					Container: &ContainerTaskPayload{Name: "test", Image: "nginx"},
				},
				Timeout: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTask_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		state    TaskState
		terminal bool
	}{
		{"pending", TaskStatePending, false},
		{"queued", TaskStateQueued, false},
		{"running", TaskStateRunning, false},
		{"retrying", TaskStateRetrying, false},
		{"completed", TaskStateCompleted, true},
		{"failed", TaskStateFailed, false},
		{"cancelled", TaskStateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{State: tt.state}
			result := task.IsTerminal()
			if result != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", result, tt.terminal)
			}
		})
	}
}
