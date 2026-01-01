package domain

import "errors"

// Container domain errors.
var (
	ErrContainerNotFound       = errors.New("container not found")
	ErrContainerRunning        = errors.New("container is running")
	ErrContainerNotRunning     = errors.New("container is not running")
	ErrImageNotFound           = errors.New("image not found")
	ErrImagePullFailed         = errors.New("failed to pull image")
	ErrInvalidContainerID      = errors.New("invalid container ID")
	ErrInvalidImageRef         = errors.New("invalid image reference")
	ErrInvalidConfig           = errors.New("invalid container configuration")
	ErrInvalidMemoryLimit      = errors.New("invalid memory limit")
	ErrInvalidCPULimit         = errors.New("invalid CPU limit")
	ErrInvalidMountDestination = errors.New("invalid mount destination")
	ErrNetworkConnectionFailed = errors.New("failed to connect container to network")
	ErrInvalidStateTransition  = errors.New("invalid state transition")
	ErrImageRequired           = errors.New("image is required")
)

// IsRetryableError returns true if the error is retryable.
func IsRetryableError(err error) bool {
	// Add more retryable error checks as needed
	return false
}

// IsNotFoundError returns true if the error indicates a resource was not found.
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrContainerNotFound) ||
		errors.Is(err, ErrImageNotFound)
}
