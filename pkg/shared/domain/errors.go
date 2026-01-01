package domain

import (
	"errors"
	"fmt"
)

// Sentinel errors for domain operations.
var (
	// ID errors
	ErrEmptyID   = errors.New("id cannot be empty")
	ErrInvalidID = errors.New("invalid id format")

	// Status errors
	ErrInvalidStatus = errors.New("invalid status")

	// Validation errors
	ErrValidation = errors.New("validation error")

	// Not found errors
	ErrNotFound           = errors.New("resource not found")
	ErrDeploymentNotFound = errors.New("deployment not found")
	ErrAgentNotFound      = errors.New("agent not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrContainerNotFound  = errors.New("container not found")
	ErrPluginNotFound     = errors.New("plugin not found")
	ErrVPCNotFound        = errors.New("vpc not found")
	ErrSubnetNotFound     = errors.New("subnet not found")

	// Conflict errors
	ErrConflict               = errors.New("resource conflict")
	ErrAlreadyExists          = errors.New("resource already exists")
	ErrOptimisticLock         = errors.New("optimistic lock conflict")
	ErrConcurrentModification = errors.New("concurrent modification detected")

	// State errors
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrStateDrift             = errors.New("state drift detected")

	// Operation errors
	ErrOperationFailed    = errors.New("operation failed")
	ErrOperationCancelled = errors.New("operation cancelled")
	ErrOperationTimeout   = errors.New("operation timed out")
	ErrRetryExhausted     = errors.New("retry attempts exhausted")

	// Capacity errors
	ErrCapacityExhausted = errors.New("capacity exhausted")
	ErrIPAMExhausted     = errors.New("no IP addresses available")
	ErrNoAgentsAvailable = errors.New("no agents available")

	// Permission errors
	ErrPermissionDenied = errors.New("permission denied")
	ErrUnauthorized     = errors.New("unauthorized")

	// Configuration errors
	ErrInvalidConfiguration = errors.New("invalid configuration")
	ErrMissingConfiguration = errors.New("missing required configuration")

	// Connection errors
	ErrConnectionFailed = errors.New("connection failed")
	ErrConnectionLost   = errors.New("connection lost")

	// Plugin errors
	ErrPluginFailed  = errors.New("plugin execution failed")
	ErrPluginTimeout = errors.New("plugin execution timed out")
	ErrPluginAborted = errors.New("plugin aborted execution")
)

// DomainError wraps domain errors with additional context.
type DomainError struct {
	Kind    error
	Err     error
	Details map[string]any
	Op      string
	Entity  string
	ID      string
}

// NewDomainError creates a new domain error.
func NewDomainError(op string, kind error, entity string) *DomainError {
	return &DomainError{
		Op:     op,
		Kind:   kind,
		Entity: entity,
	}
}

// WithID adds an entity ID to the error.
func (e *DomainError) WithID(id string) *DomainError {
	e.ID = id
	return e
}

// WithErr adds an underlying error.
func (e *DomainError) WithErr(err error) *DomainError {
	e.Err = err
	return e
}

// WithDetail adds a detail to the error.
func (e *DomainError) WithDetail(key string, value any) *DomainError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

func (e *DomainError) Error() string {
	if e.ID != "" {
		if e.Err != nil {
			return fmt.Sprintf("%s %s[%s]: %v: %v", e.Op, e.Entity, e.ID, e.Kind, e.Err)
		}
		return fmt.Sprintf("%s %s[%s]: %v", e.Op, e.Entity, e.ID, e.Kind)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s %s: %v: %v", e.Op, e.Entity, e.Kind, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Entity, e.Kind)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches a target error.
func (e *DomainError) Is(target error) bool {
	return errors.Is(e.Kind, target) || errors.Is(e.Err, target)
}

// ValidationError represents a validation failure with field-level details.
type ValidationError struct {
	Fields map[string]string
	Entity string
}

// NewValidationError creates a new validation error.
func NewValidationError(entity string) *ValidationError {
	return &ValidationError{
		Entity: entity,
		Fields: make(map[string]string),
	}
}

// AddFieldError adds a field validation error.
func (e *ValidationError) AddFieldError(field, message string) *ValidationError {
	e.Fields[field] = message
	return e
}

// HasErrors returns true if there are validation errors.
func (e *ValidationError) HasErrors() bool {
	return len(e.Fields) > 0
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return fmt.Sprintf("validation error for %s: no fields specified", e.Entity)
	}
	return fmt.Sprintf("validation error for %s: %d field(s) failed validation", e.Entity, len(e.Fields))
}

func (e *ValidationError) Unwrap() error {
	return ErrValidation
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) ||
		errors.Is(err, ErrDeploymentNotFound) ||
		errors.Is(err, ErrAgentNotFound) ||
		errors.Is(err, ErrTaskNotFound) ||
		errors.Is(err, ErrContainerNotFound) ||
		errors.Is(err, ErrPluginNotFound) ||
		errors.Is(err, ErrVPCNotFound) ||
		errors.Is(err, ErrSubnetNotFound)
}

// IsConflict checks if an error is a conflict error.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict) ||
		errors.Is(err, ErrAlreadyExists) ||
		errors.Is(err, ErrOptimisticLock) ||
		errors.Is(err, ErrConcurrentModification)
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	return errors.Is(err, ErrOperationTimeout) ||
		errors.Is(err, ErrConnectionLost) ||
		errors.Is(err, ErrOptimisticLock)
}

// IsCapacityError checks if an error is a capacity-related error.
func IsCapacityError(err error) bool {
	return errors.Is(err, ErrCapacityExhausted) ||
		errors.Is(err, ErrIPAMExhausted) ||
		errors.Is(err, ErrNoAgentsAvailable)
}
