// Package usecases implements the business logic for the Plugin Manager.
package usecases

import (
	"context"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// PluginServiceImpl implements the PluginService interface.
type PluginServiceImpl struct {
	registry *RegistryUseCase
	executor *ExecutorUseCase
}

// Ensure PluginServiceImpl implements PluginService.
var _ inbound.PluginService = (*PluginServiceImpl)(nil)

// NewPluginService creates a new PluginServiceImpl.
func NewPluginService(
	repo outbound.PluginRepository,
	runners []outbound.PluginRunner,
	logger *slog.Logger,
) *PluginServiceImpl {
	return &PluginServiceImpl{
		registry: NewRegistryUseCase(repo, logger),
		executor: NewExecutorUseCase(repo, runners, logger),
	}
}

// RegisterPlugin registers a new plugin.
func (s *PluginServiceImpl) RegisterPlugin(ctx context.Context, plugin *domain.Plugin) error {
	return s.registry.RegisterPlugin(ctx, plugin)
}

// UnregisterPlugin removes a plugin.
func (s *PluginServiceImpl) UnregisterPlugin(ctx context.Context, name string) error {
	return s.registry.UnregisterPlugin(ctx, name)
}

// UpdatePlugin updates an existing plugin.
func (s *PluginServiceImpl) UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error {
	return s.registry.UpdatePlugin(ctx, plugin)
}

// GetPlugin retrieves a plugin by name.
func (s *PluginServiceImpl) GetPlugin(ctx context.Context, name string) (*domain.Plugin, error) {
	return s.registry.GetPlugin(ctx, name)
}

// ListPlugins retrieves all plugins.
func (s *PluginServiceImpl) ListPlugins(ctx context.Context) ([]domain.Plugin, error) {
	return s.registry.ListPlugins(ctx)
}

// ListPluginsByHook retrieves all plugins for a specific hook.
func (s *PluginServiceImpl) ListPluginsByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error) {
	return s.registry.ListPluginsByHook(ctx, hook)
}

// ExecuteHook executes all plugins for a given hook.
func (s *PluginServiceImpl) ExecuteHook(
	ctx context.Context,
	hook domain.HookPoint,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.HookResults, error) {
	return s.executor.ExecuteHook(ctx, hook, execCtx)
}

// EnablePlugin enables a plugin.
func (s *PluginServiceImpl) EnablePlugin(ctx context.Context, name string) error {
	return s.registry.EnablePlugin(ctx, name)
}

// DisablePlugin disables a plugin.
func (s *PluginServiceImpl) DisablePlugin(ctx context.Context, name string) error {
	return s.registry.DisablePlugin(ctx, name)
}

// SetPriority sets the priority of a plugin.
func (s *PluginServiceImpl) SetPriority(ctx context.Context, name string, priority int) error {
	return s.registry.SetPriority(ctx, name, priority)
}
