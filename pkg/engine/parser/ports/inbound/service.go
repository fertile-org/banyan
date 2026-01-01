// Package inbound defines the inbound ports (service interfaces) for the Compose Parser.
package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/parser/domain"
)

// ComposeParserService defines the compose parsing service interface.
type ComposeParserService interface {
	// Parse parses compose and banyan files, returning a ParseResult.
	Parse(ctx context.Context, composeContent, banyanContent string, opts ParseOptions) (*ParseResult, error)

	// ParseCompose parses only the compose file.
	ParseCompose(ctx context.Context, content string) (*domain.ParsedCompose, error)

	// ParseBanyan parses only the banyan extension file.
	ParseBanyan(ctx context.Context, content string) (*domain.ParsedBanyan, error)

	// Validate validates compose content without full parsing.
	Validate(ctx context.Context, composeContent string) (*domain.ValidationResult, error)

	// ValidateWithBanyan validates both compose and banyan content.
	ValidateWithBanyan(ctx context.Context, composeContent, banyanContent string) (*domain.ValidationResult, error)

	// GetSupportedVersions returns supported compose file versions.
	GetSupportedVersions() []string
}

// ParseOptions contains options for parsing.
type ParseOptions struct {
	// SkipValidation skips validation during parsing.
	SkipValidation bool

	// SkipInterpolation skips environment variable interpolation.
	SkipInterpolation bool

	// WorkingDir is the working directory for relative paths.
	WorkingDir string

	// Environment variables for interpolation.
	Environment map[string]string

	// ProjectName overrides the project name.
	ProjectName string
}

// ParseResult contains the result of parsing.
type ParseResult struct {
	Compose    *domain.ParsedCompose
	Banyan     *domain.ParsedBanyan
	Validation *domain.ValidationResult
}
