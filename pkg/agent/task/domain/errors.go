package domain

import "errors"

var (
	// ErrTaskNotFound indicates that a task was not found.
	ErrTaskNotFound = errors.New("task not found")

	// ErrInvalidTask indicates that a task configuration is invalid.
	ErrInvalidTask = errors.New("invalid task configuration")

	// ErrTaskAlreadyExists indicates that a task already exists.
	ErrTaskAlreadyExists = errors.New("task already exists")

	// ErrTaskTimeout indicates that a task execution timed out.
	ErrTaskTimeout = errors.New("task execution timed out")

	// ErrTaskCancelled indicates that a task was cancelled.
	ErrTaskCancelled = errors.New("task was cancelled")

	// ErrTaskFailed indicates that a task execution failed.
	ErrTaskFailed = errors.New("task execution failed")

	// ErrInvalidTaskID indicates that the task ID is invalid.
	ErrInvalidTaskID = errors.New("invalid task ID")

	// ErrInvalidPayload indicates that the task payload is invalid.
	ErrInvalidPayload = errors.New("invalid task payload")

	// ErrCannotCancelTask indicates that the task cannot be cancelled in its current state.
	ErrCannotCancelTask = errors.New("cannot cancel task in current state")

	// ErrUnknownTaskType indicates that the task type is not recognized.
	ErrUnknownTaskType = errors.New("unknown task type")

	// ErrInvalidTaskState indicates that the task state transition is invalid.
	ErrInvalidTaskState = errors.New("invalid task state transition")
)
