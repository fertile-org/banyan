// Package inbound defines the inbound ports (service interfaces) for the Plugin Manager.
package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
)

// PluginService defines plugin management operations.
type PluginService interface {
	// Registry operations
	RegisterPlugin(ctx context.Context, plugin *domain.Plugin) error
	UnregisterPlugin(ctx context.Context, name string) error
	UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error
	GetPlugin(ctx context.Context, name string) (*domain.Plugin, error)
	ListPlugins(ctx context.Context) ([]domain.Plugin, error)
	ListPluginsByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error)

	// Execution operations
	ExecuteHook(ctx context.Context, hook domain.HookPoint, execCtx domain.ExecutionContext) (*domain.HookResults, error)

	// Configuration operations
	EnablePlugin(ctx context.Context, name string) error
	DisablePlugin(ctx context.Context, name string) error
	SetPriority(ctx context.Context, name string, priority int) error
}

// PluginExecutor defines plugin execution operations.
type PluginExecutor interface {
	ExecuteHook(ctx context.Context, hook domain.HookPoint, execCtx domain.ExecutionContext) (*domain.HookResults, error)
}

// PluginRegistry defines plugin registration operations.
type PluginRegistry interface {
	RegisterPlugin(ctx context.Context, plugin *domain.Plugin) error
	UnregisterPlugin(ctx context.Context, name string) error
	UpdatePlugin(ctx context.Context, plugin *domain.Plugin) error
	GetPlugin(ctx context.Context, name string) (*domain.Plugin, error)
	ListPlugins(ctx context.Context) ([]domain.Plugin, error)
	ListPluginsByHook(ctx context.Context, hook domain.HookPoint) ([]domain.Plugin, error)
	EnablePlugin(ctx context.Context, name string) error
	DisablePlugin(ctx context.Context, name string) error
	SetPriority(ctx context.Context, name string, priority int) error
}
