package interfaces

import "errors"

// Common errors for Banyan interfaces
var (
	ErrMissingDeploymentID   = errors.New("deployment ID is required")
	ErrMissingDeploymentName = errors.New("deployment name is required")
	ErrInvalidConfiguration  = errors.New("invalid deployment configuration")
	ErrProviderNotSupported  = errors.New("provider not supported")
	ErrDeploymentFailed      = errors.New("deployment failed")
	ErrDeploymentNotFound    = errors.New("deployment not found")
)
