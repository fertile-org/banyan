// Package adapters provides implementations of the outbound ports for the Plugin Manager.
package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// MemoryPluginRepository is an in-memory implementation of PluginRepository.
type MemoryPluginRepository struct {
	mu      sync.RWMutex
	plugins map[string]*domain.Plugin
}

// Ensure MemoryPluginRepository implements PluginRepository.
var _ outbound.PluginRepository = (*MemoryPluginRepository)(nil)

// NewMemoryPluginRepository creates a new MemoryPluginRepository.
func NewMemoryPluginRepository() *MemoryPluginRepository {
	return &MemoryPluginRepository{
		plugins: make(map[string]*domain.Plugin),
	}
}

// Save saves a plugin to the repository.
func (r *MemoryPluginRepository) Save(_ context.Context, plugin *domain.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins[plugin.Name] = plugin.Clone()
	return nil
}

// FindByName retrieves a plugin by name.
func (r *MemoryPluginRepository) FindByName(_ context.Context, name string) (*domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, ok := r.plugins[name]
	if !ok {
		return nil, nil
	}
	return plugin.Clone(), nil
}

// FindByHook retrieves all plugins registered for a hook.
func (r *MemoryPluginRepository) FindByHook(_ context.Context, hook domain.HookPoint) ([]domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []domain.Plugin
	for _, plugin := range r.plugins {
		if plugin.HasHook(hook) {
			result = append(result, *plugin.Clone())
		}
	}
	return result, nil
}

// FindAll retrieves all plugins.
func (r *MemoryPluginRepository) FindAll(_ context.Context) ([]domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]domain.Plugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		result = append(result, *plugin.Clone())
	}
	return result, nil
}

// Delete deletes a plugin by name.
func (r *MemoryPluginRepository) Delete(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.plugins[name]; !ok {
		return pluginerrors.ErrPluginNotFound
	}

	delete(r.plugins, name)
	return nil
}

// Update updates an existing plugin.
func (r *MemoryPluginRepository) Update(_ context.Context, plugin *domain.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.plugins[plugin.Name]; !ok {
		return pluginerrors.ErrPluginNotFound
	}

	r.plugins[plugin.Name] = plugin.Clone()
	return nil
}
