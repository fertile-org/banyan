// Package outbound defines the outbound ports (repository and runner interfaces) for the Plugin Manager.
package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
)

// PluginRepository defines persistence operations for plugins.
type PluginRepository interface {
	// Save saves a plugin to the repository.
	Save(ctx context.Context, plugin *domain.Plugin) error

	// FindByName retrieves a plugin by name.
	FindByName(ctx context.Context, name string) (*domain.Plugin, error)

	// FindByHook retrieves all plugins registered for a hook.
	FindByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error)

	// FindAll retrieves all plugins.
	FindAll(ctx context.Context) ([]domain.Plugin, error)

	// Delete deletes a plugin by name.
	Delete(ctx context.Context, name string) error

	// Update updates an existing plugin.
	Update(ctx context.Context, plugin *domain.Plugin) error
}

// PluginRunner executes a plugin.
type PluginRunner interface {
	// Run executes a plugin with the given context.
	Run(ctx context.Context, plugin *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error)

	// CanRun checks if the runner can handle this plugin type.
	CanRun(plugin *domain.Plugin) bool
}

// BuiltinPlugin is the interface for built-in plugins.
type BuiltinPlugin interface {
	// Execute executes the builtin plugin logic.
	Execute(ctx context.Context, execCtx domain.ExecutionContext) (*domain.PluginResult, error)

	// Name returns the builtin plugin name.
	Name() string
}
