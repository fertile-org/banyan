// Package outbound defines the outbound ports (adapter interfaces) for the Compose Parser.
package outbound

import (
	"github.com/fertile-org/banyan/pkg/engine/parser/domain"
)

// YAMLParser defines the interface for YAML parsing.
type YAMLParser interface {
	// ParseYAML parses YAML content into a generic map.
	ParseYAML(content string) (map[string]interface{}, error)

	// UnmarshalYAML unmarshals YAML into a specific type.
	UnmarshalYAML(content string, v interface{}) error
}

// ComposeLoader defines the interface for loading compose files.
type ComposeLoader interface {
	// Load loads a compose file from content.
	Load(content string, opts LoadOptions) (*domain.ParsedCompose, error)

	// LoadFromFile loads a compose file from path.
	LoadFromFile(path string, opts LoadOptions) (*domain.ParsedCompose, error)
}

// LoadOptions contains options for loading compose files.
type LoadOptions struct {
	// SkipInterpolation skips environment variable interpolation.
	SkipInterpolation bool

	// SkipValidation skips schema validation.
	SkipValidation bool

	// WorkingDir is the working directory.
	WorkingDir string

	// Environment variables.
	Environment map[string]string
}

// SchemaValidator defines the interface for schema validation.
type SchemaValidator interface {
	// ValidateAgainstSchema validates content against compose schema.
	ValidateAgainstSchema(content string, version string) error

	// GetSchema returns the schema for a version.
	GetSchema(version string) ([]byte, error)
}

// EnvironmentInterpolator defines the interface for env var interpolation.
type EnvironmentInterpolator interface {
	// Interpolate replaces environment variables in content.
	Interpolate(content string, env map[string]string) (string, error)
}
