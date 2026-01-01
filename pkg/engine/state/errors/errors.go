package errors

import "errors"

var (
	// ErrDesiredStateNotFound indicates that the desired state was not found.
	ErrDesiredStateNotFound = errors.New("desired state not found")

	// ErrActualStateNotFound indicates that the actual state was not found.
	ErrActualStateNotFound = errors.New("actual state not found")

	// ErrDeploymentNotFound indicates that the deployment was not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrAgentNotFound indicates that the agent was not found.
	ErrAgentNotFound = errors.New("agent not found")

	// ErrReconcileInProgress indicates that reconciliation is already in progress.
	ErrReconcileInProgress = errors.New("reconciliation already in progress")

	// ErrReconcilerNotStarted indicates that the reconciler has not been started.
	ErrReconcilerNotStarted = errors.New("reconciler not started")

	// ErrReconcilerAlreadyStarted indicates that the reconciler is already running.
	ErrReconcilerAlreadyStarted = errors.New("reconciler already started")

	// ErrInvalidState indicates an invalid state.
	ErrInvalidState = errors.New("invalid state")

	// ErrDispatchFailed indicates that action dispatch failed.
	ErrDispatchFailed = errors.New("action dispatch failed")
)
