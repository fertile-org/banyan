package domain

import (
	"errors"
	"testing"
)

func TestDomainError(t *testing.T) {
	t.Run("basic error", func(t *testing.T) {
		err := NewDomainError("Deploy", ErrNotFound, "Deployment")
		if err.Error() != "Deploy Deployment: resource not found" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("error with ID", func(t *testing.T) {
		err := NewDomainError("Get", ErrNotFound, "Agent").WithID("agent-123")
		if err.Error() != "Get Agent[agent-123]: resource not found" {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	})

	t.Run("error with underlying error", func(t *testing.T) {
		underlying := errors.New("connection refused")
		err := NewDomainError("Connect", ErrConnectionFailed, "Engine").WithErr(underlying)

		expected := "Connect Engine: connection failed: connection refused"
		if err.Error() != expected {
			t.Errorf("unexpected error message: got %s, want %s", err.Error(), expected)
		}
	})

	t.Run("error with ID and underlying", func(t *testing.T) {
		underlying := errors.New("timeout")
		err := NewDomainError("Start", ErrOperationTimeout, "Container").
			WithID("ctr-123").
			WithErr(underlying)

		expected := "Start Container[ctr-123]: operation timed out: timeout"
		if err.Error() != expected {
			t.Errorf("unexpected error message: got %s, want %s", err.Error(), expected)
		}
	})

	t.Run("error with detail", func(t *testing.T) {
		err := NewDomainError("Create", ErrValidation, "Task").
			WithDetail("reason", "invalid config")

		if err.Details["reason"] != "invalid config" {
			t.Error("expected detail to be set")
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		underlying := errors.New("disk full")
		err := NewDomainError("Write", ErrOperationFailed, "State").WithErr(underlying)

		if errors.Unwrap(err) != underlying {
			t.Error("Unwrap should return underlying error")
		}
	})

	t.Run("Is", func(t *testing.T) {
		err := NewDomainError("Find", ErrNotFound, "Plugin")

		if !errors.Is(err, ErrNotFound) {
			t.Error("expected error to match ErrNotFound")
		}

		if errors.Is(err, ErrConflict) {
			t.Error("expected error not to match ErrConflict")
		}
	})
}

func TestValidationError(t *testing.T) {
	t.Run("empty validation error", func(t *testing.T) {
		err := NewValidationError("Container")
		if err.HasErrors() {
			t.Error("expected no errors")
		}
	})

	t.Run("validation error with fields", func(t *testing.T) {
		err := NewValidationError("Container").
			AddFieldError("name", "name is required").
			AddFieldError("image", "image must be valid")

		if !err.HasErrors() {
			t.Error("expected errors")
		}

		if len(err.Fields) != 2 {
			t.Errorf("expected 2 fields, got %d", len(err.Fields))
		}

		if err.Fields["name"] != "name is required" {
			t.Error("expected name field error")
		}
	})

	t.Run("validation error unwraps to ErrValidation", func(t *testing.T) {
		err := NewValidationError("Task").AddFieldError("type", "invalid")

		if !errors.Is(err, ErrValidation) {
			t.Error("expected error to match ErrValidation")
		}
	})
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{ErrNotFound, true},
		{ErrDeploymentNotFound, true},
		{ErrAgentNotFound, true},
		{ErrTaskNotFound, true},
		{ErrContainerNotFound, true},
		{ErrPluginNotFound, true},
		{ErrVPCNotFound, true},
		{ErrSubnetNotFound, true},
		{ErrConflict, false},
		{errors.New("other error"), false},
	}

	for _, tt := range tests {
		if got := IsNotFound(tt.err); got != tt.expected {
			t.Errorf("IsNotFound(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{ErrConflict, true},
		{ErrAlreadyExists, true},
		{ErrOptimisticLock, true},
		{ErrConcurrentModification, true},
		{ErrNotFound, false},
		{errors.New("other error"), false},
	}

	for _, tt := range tests {
		if got := IsConflict(tt.err); got != tt.expected {
			t.Errorf("IsConflict(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{ErrOperationTimeout, true},
		{ErrConnectionLost, true},
		{ErrOptimisticLock, true},
		{ErrNotFound, false},
		{ErrPermissionDenied, false},
	}

	for _, tt := range tests {
		if got := IsRetryable(tt.err); got != tt.expected {
			t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}

func TestIsCapacityError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{ErrCapacityExhausted, true},
		{ErrIPAMExhausted, true},
		{ErrNoAgentsAvailable, true},
		{ErrNotFound, false},
		{ErrConflict, false},
	}

	for _, tt := range tests {
		if got := IsCapacityError(tt.err); got != tt.expected {
			t.Errorf("IsCapacityError(%v) = %v, want %v", tt.err, got, tt.expected)
		}
	}
}
