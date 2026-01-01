package usecases

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
)

// TaskUseCase implements the TaskExecutorService interface.
// It routes tasks to the appropriate service based on task type.
type TaskUseCase struct {
	container outbound.ContainerService
	network   outbound.NetworkService
	security  outbound.SecurityService
	health    outbound.HealthService
	store     outbound.TaskStore
	events    outbound.EventEmitter
	cancelFns map[domain.TaskID]context.CancelFunc
	mu        sync.RWMutex
}

// NewTaskUseCase creates a new TaskUseCase.
func NewTaskUseCase(
	container outbound.ContainerService,
	network outbound.NetworkService,
	security outbound.SecurityService,
	health outbound.HealthService,
	store outbound.TaskStore,
	events outbound.EventEmitter,
) *TaskUseCase {
	return &TaskUseCase{
		container: container,
		network:   network,
		security:  security,
		health:    health,
		store:     store,
		events:    events,
		cancelFns: make(map[domain.TaskID]context.CancelFunc),
	}
}

// SubmitTask submits a task and waits for completion (synchronous).
func (uc *TaskUseCase) SubmitTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("invalid task: %w", err)
	}

	// Initialize task
	task.State = domain.TaskStatePending
	task.CreatedAt = time.Now()

	// Save task
	if err := uc.store.Save(ctx, task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	// Queue task
	if err := task.TransitionTo(domain.TaskStateQueued); err != nil {
		return nil, err
	}
	if err := uc.store.Update(ctx, task); err != nil {
		return nil, err
	}

	uc.emitEvent(task.ID, domain.TaskEventQueued, nil)

	// Execute and wait for result
	return uc.executeTask(ctx, task)
}

// SubmitTaskAsync submits a task and returns immediately (asynchronous).
func (uc *TaskUseCase) SubmitTaskAsync(ctx context.Context, task *domain.Task) (domain.TaskID, error) {
	if err := task.Validate(); err != nil {
		return "", fmt.Errorf("invalid task: %w", err)
	}

	// Initialize task
	task.State = domain.TaskStatePending
	task.CreatedAt = time.Now()

	// Save task
	if err := uc.store.Save(ctx, task); err != nil {
		return "", fmt.Errorf("failed to save task: %w", err)
	}

	// Queue task
	if err := task.TransitionTo(domain.TaskStateQueued); err != nil {
		return "", err
	}
	if err := uc.store.Update(ctx, task); err != nil {
		return "", err
	}

	uc.emitEvent(task.ID, domain.TaskEventQueued, nil)

	// Execute asynchronously
	go func() {
		_, _ = uc.executeTask(context.Background(), task)
	}()

	return task.ID, nil
}

// CancelTask cancels a running or queued task.
func (uc *TaskUseCase) CancelTask(ctx context.Context, taskID domain.TaskID) error {
	task, err := uc.store.Get(ctx, taskID)
	if err != nil {
		return domain.ErrTaskNotFound
	}

	if !task.CanTransitionTo(domain.TaskStateCancelled) {
		return domain.ErrCannotCancelTask
	}

	// Cancel the context if running
	uc.mu.Lock()
	if cancel, ok := uc.cancelFns[taskID]; ok {
		cancel()
		delete(uc.cancelFns, taskID)
	}
	uc.mu.Unlock()

	// Update state
	if err := task.TransitionTo(domain.TaskStateCancelled); err != nil {
		return err
	}
	if err := uc.store.Update(ctx, task); err != nil {
		return err
	}

	uc.emitEvent(taskID, domain.TaskEventCancelled, nil)
	return nil
}

// GetTaskStatus gets the current status of a task.
func (uc *TaskUseCase) GetTaskStatus(ctx context.Context, taskID domain.TaskID) (*domain.TaskStatus, error) {
	task, err := uc.store.Get(ctx, taskID)
	if err != nil {
		return nil, domain.ErrTaskNotFound
	}

	status := &domain.TaskStatus{
		TaskID:    task.ID,
		State:     task.State,
		StartedAt: task.StartedAt,
	}

	if task.StartedAt != nil {
		if task.CompletedAt != nil {
			d := task.CompletedAt.Sub(*task.StartedAt)
			status.Duration = &d
		} else {
			d := time.Since(*task.StartedAt)
			status.Duration = &d
		}
	}

	// Set progress based on state
	switch task.State {
	case domain.TaskStatePending, domain.TaskStateQueued:
		status.Progress = 0.0
		status.Message = "Waiting to run"
	case domain.TaskStateRunning:
		status.Progress = 0.5
		status.Message = "Running"
	case domain.TaskStateCompleted:
		status.Progress = 1.0
		status.Message = "Completed"
	case domain.TaskStateFailed:
		status.Progress = 1.0
		status.Message = task.Error
	case domain.TaskStateCancelled:
		status.Progress = 0.0
		status.Message = "Cancelled"
	case domain.TaskStateRetrying:
		status.Progress = 0.25
		status.Message = fmt.Sprintf("Retrying (attempt %d)", task.RetryCount)
	}

	return status, nil
}

// GetTaskResult gets the result of a completed task.
func (uc *TaskUseCase) GetTaskResult(ctx context.Context, taskID domain.TaskID) (*domain.TaskResult, error) {
	task, err := uc.store.Get(ctx, taskID)
	if err != nil {
		return nil, domain.ErrTaskNotFound
	}

	if !task.IsTerminal() {
		return nil, fmt.Errorf("task not yet completed")
	}

	return task.Result, nil
}

// ListTasks lists tasks matching the filter.
func (uc *TaskUseCase) ListTasks(ctx context.Context, filter *domain.TaskFilter) ([]*domain.Task, error) {
	return uc.store.List(ctx, filter)
}

// SubscribeTaskEvents subscribes to task lifecycle events.
func (uc *TaskUseCase) SubscribeTaskEvents(ctx context.Context) (<-chan domain.TaskEvent, error) {
	if uc.events == nil {
		return nil, fmt.Errorf("event emitter not configured")
	}
	return uc.events.Subscribe(), nil
}

// executeTask executes a task and returns the result.
func (uc *TaskUseCase) executeTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	// Transition to running
	if err := task.TransitionTo(domain.TaskStateRunning); err != nil {
		return nil, err
	}
	if err := uc.store.Update(ctx, task); err != nil {
		return nil, err
	}

	uc.emitEvent(task.ID, domain.TaskEventStarted, nil)

	// Create timeout context
	taskCtx := ctx
	if task.Timeout > 0 {
		var cancel context.CancelFunc
		taskCtx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()

		uc.mu.Lock()
		uc.cancelFns[task.ID] = cancel
		uc.mu.Unlock()

		defer func() {
			uc.mu.Lock()
			delete(uc.cancelFns, task.ID)
			uc.mu.Unlock()
		}()
	}

	// Route to appropriate handler
	startTime := time.Now()
	var result *domain.TaskResult
	var err error

	switch {
	case strings.HasPrefix(string(task.Type), "container."):
		result, err = uc.executeContainerTask(taskCtx, task)
	case strings.HasPrefix(string(task.Type), "network."):
		result, err = uc.executeNetworkTask(taskCtx, task)
	case strings.HasPrefix(string(task.Type), "security."):
		result, err = uc.executeSecurityTask(taskCtx, task)
	case strings.HasPrefix(string(task.Type), "health."):
		result, err = uc.executeHealthTask(taskCtx, task)
	default:
		err = fmt.Errorf("%w: %s", domain.ErrUnknownTaskType, task.Type)
	}

	duration := time.Since(startTime)

	// Handle result
	if err != nil {
		return uc.handleTaskError(ctx, task, err, duration)
	}

	// Complete successfully
	result.TaskID = task.ID
	result.Success = true
	result.Duration = duration
	task.Result = result

	if err := task.TransitionTo(domain.TaskStateCompleted); err != nil {
		return nil, err
	}
	if err := uc.store.Update(ctx, task); err != nil {
		return nil, err
	}

	uc.emitEvent(task.ID, domain.TaskEventCompleted, map[string]interface{}{
		"duration": duration.String(),
	})

	return result, nil
}

// handleTaskError handles task execution errors.
func (uc *TaskUseCase) handleTaskError(ctx context.Context, task *domain.Task, err error, duration time.Duration) (*domain.TaskResult, error) {
	task.Error = err.Error()

	// Check if we should retry
	if task.ShouldRetry() {
		if transErr := task.TransitionTo(domain.TaskStateRetrying); transErr == nil {
			_ = uc.store.Update(ctx, task)
			uc.emitEvent(task.ID, domain.TaskEventRetrying, map[string]interface{}{
				"retry_count": task.RetryCount,
			})

			// Wait for retry delay then requeue
			time.Sleep(task.RetryDelay())
			if transErr := task.TransitionTo(domain.TaskStateQueued); transErr == nil {
				_ = uc.store.Update(ctx, task)
				// Re-execute
				return uc.executeTask(ctx, task)
			}
		}
	}

	// Mark as failed
	if transErr := task.TransitionTo(domain.TaskStateFailed); transErr == nil {
		_ = uc.store.Update(ctx, task)
	}

	uc.emitEvent(task.ID, domain.TaskEventFailed, map[string]interface{}{
		"error": err.Error(),
	})

	return &domain.TaskResult{
		TaskID:   task.ID,
		Success:  false,
		Error:    err.Error(),
		Duration: duration,
	}, err
}

// executeContainerTask executes a container operation.
func (uc *TaskUseCase) executeContainerTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	if uc.container == nil {
		return nil, fmt.Errorf("container service not configured")
	}

	payload := task.Payload.Container
	if payload == nil {
		return nil, domain.ErrInvalidPayload
	}

	switch task.Type {
	case domain.TaskTypeContainerCreate:
		spec := &outbound.ContainerSpec{
			Name:        payload.Name,
			Image:       payload.Image,
			Command:     payload.Command,
			Environment: payload.Environment,
			Labels:      payload.Labels,
		}
		info, err := uc.container.CreateContainer(ctx, spec)
		if err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: info}, nil

	case domain.TaskTypeContainerStart:
		if err := uc.container.StartContainer(ctx, payload.ContainerID); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "started"}, nil

	case domain.TaskTypeContainerStop:
		timeout := payload.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		if err := uc.container.StopContainer(ctx, payload.ContainerID, timeout); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "stopped"}, nil

	case domain.TaskTypeContainerRemove:
		if err := uc.container.RemoveContainer(ctx, payload.ContainerID, true); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "removed"}, nil

	case domain.TaskTypeContainerRestart:
		timeout := payload.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		if err := uc.container.RestartContainer(ctx, payload.ContainerID, timeout); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "restarted"}, nil

	default:
		return nil, fmt.Errorf("unknown container operation: %s", task.Type)
	}
}

// executeNetworkTask executes a network operation.
func (uc *TaskUseCase) executeNetworkTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	if uc.network == nil {
		return nil, fmt.Errorf("network service not configured")
	}

	payload := task.Payload.Network
	if payload == nil {
		return nil, domain.ErrInvalidPayload
	}

	switch task.Type {
	case domain.TaskTypeNetworkConnect:
		if err := uc.network.ConnectContainer(ctx, payload.ContainerID, payload.NetworkID, payload.IPAddress); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "connected"}, nil

	case domain.TaskTypeNetworkDisconnect:
		if err := uc.network.DisconnectContainer(ctx, payload.ContainerID, payload.NetworkID); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "disconnected"}, nil

	default:
		return nil, fmt.Errorf("unknown network operation: %s", task.Type)
	}
}

// executeSecurityTask executes a security operation.
func (uc *TaskUseCase) executeSecurityTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	if uc.security == nil {
		return nil, fmt.Errorf("security service not configured")
	}

	payload := task.Payload.Security
	if payload == nil {
		return nil, domain.ErrInvalidPayload
	}

	switch task.Type {
	case domain.TaskTypeSecurityApply:
		// Convert payload rules to outbound rules
		rules := make([]outbound.SecurityRule, len(payload.Rules))
		for i, r := range payload.Rules {
			rules[i] = outbound.SecurityRule{
				Direction: r.Direction,
				Protocol:  r.Protocol,
				Port:      r.Port,
				PortRange: r.PortRange,
				Source:    r.Source,
				Action:    r.Action,
			}
		}
		policy := &outbound.SecurityPolicy{
			ID:    payload.PolicyID,
			Rules: rules,
		}
		if err := uc.security.ApplyPolicy(ctx, policy); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "applied"}, nil

	case domain.TaskTypeSecurityRemove:
		if err := uc.security.RemovePolicy(ctx, payload.PolicyID); err != nil {
			return nil, err
		}
		return &domain.TaskResult{Output: "removed"}, nil

	default:
		return nil, fmt.Errorf("unknown security operation: %s", task.Type)
	}
}

// executeHealthTask executes a health check operation.
func (uc *TaskUseCase) executeHealthTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error) {
	if uc.health == nil {
		return nil, fmt.Errorf("health service not configured")
	}

	payload := task.Payload.Health
	if payload == nil {
		return nil, domain.ErrInvalidPayload
	}

	switch task.Type {
	case domain.TaskTypeHealthCheck:
		result, err := uc.health.RunHealthCheck(ctx, payload.ContainerID, payload.CheckType, payload.Target, payload.Timeout)
		if err != nil {
			return nil, err
		}
		return &domain.TaskResult{
			Output: map[string]interface{}{
				"healthy":  result.Healthy,
				"message":  result.Message,
				"duration": result.Duration.String(),
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown health operation: %s", task.Type)
	}
}

// emitEvent emits a task event if the event emitter is configured.
func (uc *TaskUseCase) emitEvent(taskID domain.TaskID, eventType domain.TaskEventType, data map[string]interface{}) {
	if uc.events == nil {
		return
	}
	uc.events.Emit(domain.TaskEvent{
		TaskID:    taskID,
		EventType: eventType,
		Timestamp: time.Now(),
		Data:      data,
	})
}

// Ensure TaskUseCase implements inbound.TaskExecutorService
var _ inbound.TaskExecutorService = (*TaskUseCase)(nil)
