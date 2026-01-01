// Package errors defines error types for the Compose Parser.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for the parser.
var (
	ErrInvalidCompose   = errors.New("invalid compose file")
	ErrInvalidBanyan    = errors.New("invalid banyan file")
	ErrValidationFailed = errors.New("validation failed")
	ErrInterpolation    = errors.New("interpolation failed")
	ErrSchemaNotFound   = errors.New("schema not found")
)

// ParseError represents a parsing error.
type ParseError struct {
	File    string
	Line    int
	Column  int
	Message string
	Cause   error
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

func (e *ParseError) Unwrap() error {
	return e.Cause
}

// ValidationError represents a validation error.
type ValidationError struct {
	Path    string
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s.%s: %s", e.Path, e.Field, e.Message)
}

// InterpolationError represents an environment interpolation error.
type InterpolationError struct {
	Variable string
	Message  string
}

func (e *InterpolationError) Error() string {
	return fmt.Sprintf("interpolation error for ${%s}: %s", e.Variable, e.Message)
}

// SchemaError represents a schema validation error.
type SchemaError struct {
	Version string
	Message string
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema error (version %s): %s", e.Version, e.Message)
}

// NewParseError creates a new ParseError.
func NewParseError(file, message string, cause error) *ParseError {
	return &ParseError{File: file, Message: message, Cause: cause}
}

// NewParseErrorWithLocation creates a new ParseError with location.
func NewParseErrorWithLocation(file string, line, col int, message string) *ParseError {
	return &ParseError{File: file, Line: line, Column: col, Message: message}
}

// NewValidationError creates a new ValidationError.
func NewValidationError(path, field, message string) *ValidationError {
	return &ValidationError{Path: path, Field: field, Message: message}
}

// NewInterpolationError creates a new InterpolationError.
func NewInterpolationError(variable, message string) *InterpolationError {
	return &InterpolationError{Variable: variable, Message: message}
}

// NewSchemaError creates a new SchemaError.
func NewSchemaError(version, message string) *SchemaError {
	return &SchemaError{Version: version, Message: message}
}
