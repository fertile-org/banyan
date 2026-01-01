// Package usecases implements the business logic for the Compose Parser.
package usecases

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/parser/domain"
	"github.com/fertile-org/banyan/pkg/engine/parser/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/parser/ports/outbound"
)

// ParseComposeUseCase implements compose file parsing.
type ParseComposeUseCase struct {
	composeLoader outbound.ComposeLoader
	yamlParser    outbound.YAMLParser
	validator     outbound.SchemaValidator
	interpolator  outbound.EnvironmentInterpolator
	logger        *slog.Logger
}

// NewParseComposeUseCase creates a new ParseComposeUseCase.
func NewParseComposeUseCase(
	composeLoader outbound.ComposeLoader,
	yamlParser outbound.YAMLParser,
	validator outbound.SchemaValidator,
	interpolator outbound.EnvironmentInterpolator,
	logger *slog.Logger,
) *ParseComposeUseCase {
	if logger == nil {
		logger = slog.Default()
	}

	return &ParseComposeUseCase{
		composeLoader: composeLoader,
		yamlParser:    yamlParser,
		validator:     validator,
		interpolator:  interpolator,
		logger:        logger,
	}
}

// Parse parses compose and banyan files.
func (uc *ParseComposeUseCase) Parse(
	ctx context.Context,
	composeContent, banyanContent string,
	opts inbound.ParseOptions,
) (*inbound.ParseResult, error) {
	// Interpolate environment variables
	if !opts.SkipInterpolation && uc.interpolator != nil {
		var err error
		composeContent, err = uc.interpolator.Interpolate(composeContent, opts.Environment)
		if err != nil {
			return nil, fmt.Errorf("interpolation failed: %w", err)
		}
	}

	// Load compose file
	loadOpts := outbound.LoadOptions{
		SkipInterpolation: true, // Already done
		SkipValidation:    opts.SkipValidation,
		WorkingDir:        opts.WorkingDir,
		Environment:       opts.Environment,
	}

	parsedCompose, err := uc.composeLoader.Load(composeContent, loadOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	// Parse banyan extension if provided
	var parsedBanyan *domain.ParsedBanyan
	if banyanContent != "" {
		parsedBanyan, err = uc.ParseBanyan(ctx, banyanContent)
		if err != nil {
			return nil, fmt.Errorf("failed to parse banyan file: %w", err)
		}
	}

	// Validate
	var validation *domain.ValidationResult
	if !opts.SkipValidation {
		composeValidation := parsedCompose.Validate()
		if parsedBanyan != nil {
			banyanValidation := domain.ValidateBanyan(parsedCompose, parsedBanyan)
			// Merge validations
			composeValidation.Errors = append(composeValidation.Errors, banyanValidation.Errors...)
			composeValidation.Warnings = append(composeValidation.Warnings, banyanValidation.Warnings...)
			if !banyanValidation.Valid {
				composeValidation.Valid = false
			}
		}
		validation = &composeValidation

		if !validation.Valid {
			return &inbound.ParseResult{
				Compose:    parsedCompose,
				Banyan:     parsedBanyan,
				Validation: validation,
			}, fmt.Errorf("validation failed with %d errors", len(validation.Errors))
		}
	}

	uc.logger.Info("parsed compose file",
		"services", len(parsedCompose.Services),
		"networks", len(parsedCompose.Networks),
		"volumes", len(parsedCompose.Volumes),
	)

	return &inbound.ParseResult{
		Compose:    parsedCompose,
		Banyan:     parsedBanyan,
		Validation: validation,
	}, nil
}

// ParseCompose parses only the compose file.
func (uc *ParseComposeUseCase) ParseCompose(
	ctx context.Context,
	content string,
) (*domain.ParsedCompose, error) {
	loadOpts := outbound.LoadOptions{
		SkipValidation: true,
	}

	return uc.composeLoader.Load(content, loadOpts)
}

// ParseBanyan parses only the banyan extension file.
func (uc *ParseComposeUseCase) ParseBanyan(
	ctx context.Context,
	content string,
) (*domain.ParsedBanyan, error) {
	var banyan domain.ParsedBanyan
	if err := uc.yamlParser.UnmarshalYAML(content, &banyan); err != nil {
		return nil, fmt.Errorf("failed to parse banyan file: %w", err)
	}
	return &banyan, nil
}

// Validate validates compose content without full parsing.
func (uc *ParseComposeUseCase) Validate(
	ctx context.Context,
	composeContent string,
) (*domain.ValidationResult, error) {
	parsed, err := uc.ParseCompose(ctx, composeContent)
	if err != nil {
		return nil, err
	}

	result := parsed.Validate()
	return &result, nil
}

// ValidateWithBanyan validates both compose and banyan content.
func (uc *ParseComposeUseCase) ValidateWithBanyan(
	ctx context.Context,
	composeContent, banyanContent string,
) (*domain.ValidationResult, error) {
	compose, err := uc.ParseCompose(ctx, composeContent)
	if err != nil {
		return nil, err
	}

	banyan, err := uc.ParseBanyan(ctx, banyanContent)
	if err != nil {
		return nil, err
	}

	composeResult := compose.Validate()
	banyanResult := domain.ValidateBanyan(compose, banyan)

	// Merge results
	composeResult.Errors = append(composeResult.Errors, banyanResult.Errors...)
	composeResult.Warnings = append(composeResult.Warnings, banyanResult.Warnings...)
	if !banyanResult.Valid {
		composeResult.Valid = false
	}

	return &composeResult, nil
}

// GetSupportedVersions returns supported compose file versions.
func (uc *ParseComposeUseCase) GetSupportedVersions() []string {
	return []string{"2", "2.0", "2.1", "2.2", "2.3", "2.4", "3", "3.0", "3.1", "3.2", "3.3", "3.4", "3.5", "3.6", "3.7", "3.8"}
}
