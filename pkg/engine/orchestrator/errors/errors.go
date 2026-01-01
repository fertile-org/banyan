package errors

import "errors"

var (
	// ErrDeploymentNotFound indicates that the deployment was not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrInvalidBanyanFile indicates an invalid banyan.yml file.
	ErrInvalidBanyanFile = errors.New("invalid banyan.yml file")

	// ErrCircularDependency indicates circular dependencies in services.
	ErrCircularDependency = errors.New("circular dependency detected")

	// ErrDeploymentFailed indicates that the deployment failed.
	ErrDeploymentFailed = errors.New("deployment failed")

	// ErrNoExecutionPlan indicates missing execution plan.
	ErrNoExecutionPlan = errors.New("no execution plan")

	// ErrInvalidState indicates an invalid deployment state.
	ErrInvalidState = errors.New("invalid deployment state")

	// ErrHookRejected indicates that a lifecycle hook rejected the deployment.
	ErrHookRejected = errors.New("lifecycle hook rejected deployment")

	// ErrAgentUnavailable indicates that the target agent is unavailable.
	ErrAgentUnavailable = errors.New("agent unavailable")

	// ErrRollbackFailed indicates that the rollback operation failed.
	ErrRollbackFailed = errors.New("rollback failed")
)
