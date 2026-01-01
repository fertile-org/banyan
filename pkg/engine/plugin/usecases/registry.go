// Package usecases implements the business logic for the Plugin Manager.
package usecases

import (
	"context"
	"log/slog"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// DefaultTimeout is the default plugin execution timeout.
const DefaultTimeout = 30 * time.Second

// RegistryUseCase handles plugin registration.
type RegistryUseCase struct {
	repo   outbound.PluginRepository
	logger *slog.Logger
}

// NewRegistryUseCase creates a new RegistryUseCase.
func NewRegistryUseCase(repo outbound.PluginRepository, logger *slog.Logger) *RegistryUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &RegistryUseCase{
		repo:   repo,
		logger: logger,
	}
}

// RegisterPlugin registers a new plugin.
func (uc *RegistryUseCase) RegisterPlugin(ctx context.Context, plugin *domain.Plugin) error {
	// Validate plugin
	if errors := plugin.Validate(); len(errors) > 0 {
		uc.logger.Warn("plugin validation failed", "name", plugin.Name, "errors", errors)
		return pluginerrors.ErrInvalidPlugin
	}

	// Check for duplicate
	existing, _ := uc.repo.FindByName(ctx, plugin.Name)
	if existing != nil {
		return pluginerrors.ErrPluginAlreadyExists
	}

	// Set defaults
	if plugin.Timeout == 0 {
		plugin.Timeout = DefaultTimeout
	}
	now := time.Now()
	plugin.CreatedAt = now
	plugin.UpdatedAt = now

	if err := uc.repo.Save(ctx, plugin); err != nil {
		return err
	}

	uc.logger.Info("plugin registered",
		"name", plugin.Name,
		"type", plugin.Type,
		"hooks", plugin.Hooks,
	)

	return nil
}

// UnregisterPlugin removes a plugin.
func (uc *RegistryUseCase) UnregisterPlugin(ctx context.Context, name string) error {
	existing, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return pluginerrors.ErrPluginNotFound
	}

	if err := uc.repo.Delete(ctx, name); err != nil {
		return err
	}

	uc.logger.Info("plugin unregistered", "name", name)
	return nil
}

// UpdatePlugin updates an existing plugin.
func (uc *RegistryUseCase) UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error {
	// Validate plugin
	if errors := plugin.Validate(); len(errors) > 0 {
		uc.logger.Warn("plugin validation failed", "name", plugin.Name, "errors", errors)
		return pluginerrors.ErrInvalidPlugin
	}

	existing, err := uc.repo.FindByName(ctx, plugin.Name)
	if err != nil {
		return err
	}
	if existing == nil {
		return pluginerrors.ErrPluginNotFound
	}

	// Preserve timestamps
	plugin.CreatedAt = existing.CreatedAt
	plugin.UpdatedAt = time.Now()

	if err := uc.repo.Update(ctx, plugin); err != nil {
		return err
	}

	uc.logger.Info("plugin updated", "name", plugin.Name)
	return nil
}

// GetPlugin retrieves a plugin by name.
func (uc *RegistryUseCase) GetPlugin(ctx context.Context, name string) (*domain.Plugin, error) {
	plugin, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if plugin == nil {
		return nil, pluginerrors.ErrPluginNotFound
	}
	return plugin, nil
}

// ListPlugins retrieves all plugins.
func (uc *RegistryUseCase) ListPlugins(ctx context.Context) ([]domain.Plugin, error) {
	return uc.repo.FindAll(ctx)
}

// ListPluginsByHook retrieves all plugins for a specific hook.
func (uc *RegistryUseCase) ListPluginsByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error) {
	return uc.repo.FindByHook(ctx, hook)
}

// EnablePlugin enables a plugin.
func (uc *RegistryUseCase) EnablePlugin(ctx context.Context, name string) error {
	plugin, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return err
	}
	if plugin == nil {
		return pluginerrors.ErrPluginNotFound
	}

	plugin.Enabled = true
	plugin.UpdatedAt = time.Now()

	if err := uc.repo.Update(ctx, plugin); err != nil {
		return err
	}

	uc.logger.Info("plugin enabled", "name", name)
	return nil
}

// DisablePlugin disables a plugin.
func (uc *RegistryUseCase) DisablePlugin(ctx context.Context, name string) error {
	plugin, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return err
	}
	if plugin == nil {
		return pluginerrors.ErrPluginNotFound
	}

	plugin.Enabled = false
	plugin.UpdatedAt = time.Now()

	if err := uc.repo.Update(ctx, plugin); err != nil {
		return err
	}

	uc.logger.Info("plugin disabled", "name", name)
	return nil
}

// SetPriority sets the priority of a plugin.
func (uc *RegistryUseCase) SetPriority(ctx context.Context, name string, priority int) error {
	plugin, err := uc.repo.FindByName(ctx, name)
	if err != nil {
		return err
	}
	if plugin == nil {
		return pluginerrors.ErrPluginNotFound
	}

	plugin.Priority = priority
	plugin.UpdatedAt = time.Now()

	if err := uc.repo.Update(ctx, plugin); err != nil {
		return err
	}

	uc.logger.Info("plugin priority set", "name", name, "priority", priority)
	return nil
}
