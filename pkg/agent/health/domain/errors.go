package domain

import "errors"

var (
	// ErrCheckNotFound indicates that a health check was not found.
	ErrCheckNotFound = errors.New("health check not found")

	// ErrInvalidCheck indicates that a health check configuration is invalid.
	ErrInvalidCheck = errors.New("invalid health check configuration")

	// ErrCheckTimeout indicates that a health check timed out.
	ErrCheckTimeout = errors.New("health check timed out")

	// ErrContainerNotFound indicates that the container was not found.
	ErrContainerNotFound = errors.New("container not found")

	// ErrInvalidContainerID indicates that the container ID is invalid.
	ErrInvalidContainerID = errors.New("invalid container ID")

	// ErrCheckAlreadyExists indicates that a health check already exists.
	ErrCheckAlreadyExists = errors.New("health check already exists")

	// ErrCheckFailed indicates that a health check execution failed.
	ErrCheckFailed = errors.New("health check failed")
)
