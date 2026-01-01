package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
)

// TaskExecutorService is the main interface for task operations.
type TaskExecutorService interface {
	// SubmitTask submits a task and waits for completion (synchronous).
	SubmitTask(ctx context.Context, task *domain.Task) (*domain.TaskResult, error)

	// SubmitTaskAsync submits a task and returns immediately (asynchronous).
	SubmitTaskAsync(ctx context.Context, task *domain.Task) (domain.TaskID, error)

	// CancelTask cancels a running or queued task.
	CancelTask(ctx context.Context, taskID domain.TaskID) error

	// GetTaskStatus gets the current status of a task.
	GetTaskStatus(ctx context.Context, taskID domain.TaskID) (*domain.TaskStatus, error)

	// GetTaskResult gets the result of a completed task.
	GetTaskResult(ctx context.Context, taskID domain.TaskID) (*domain.TaskResult, error)

	// ListTasks lists tasks matching the filter.
	ListTasks(ctx context.Context, filter *domain.TaskFilter) ([]*domain.Task, error)

	// SubscribeTaskEvents subscribes to task lifecycle events.
	SubscribeTaskEvents(ctx context.Context) (<-chan domain.TaskEvent, error)
}
