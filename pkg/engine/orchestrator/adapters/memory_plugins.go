package adapters

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
)

// MemoryPluginExecutor is an in-memory implementation for testing.
type MemoryPluginExecutor struct {
	hooks []outbound.LifecycleHook
}

// NewMemoryPluginExecutor creates a new in-memory plugin executor.
func NewMemoryPluginExecutor() *MemoryPluginExecutor {
	return &MemoryPluginExecutor{
		hooks: make([]outbound.LifecycleHook, 0),
	}
}

// ExecuteHook executes a lifecycle hook.
func (e *MemoryPluginExecutor) ExecuteHook(_ context.Context, hook outbound.LifecycleHook, _ *outbound.HookData) (*outbound.HookResult, error) {
	e.hooks = append(e.hooks, hook)
	return &outbound.HookResult{
		Continue: true,
		Message:  "hook executed successfully",
	}, nil
}

// GetExecutedHooks returns all executed hooks (for testing).
func (e *MemoryPluginExecutor) GetExecutedHooks() []outbound.LifecycleHook {
	result := make([]outbound.LifecycleHook, len(e.hooks))
	copy(result, e.hooks)
	return result
}

// ClearHooks clears all executed hooks (for testing).
func (e *MemoryPluginExecutor) ClearHooks() {
	e.hooks = make([]outbound.LifecycleHook, 0)
}

// Ensure MemoryPluginExecutor implements PluginExecutor.
var _ outbound.PluginExecutor = (*MemoryPluginExecutor)(nil)
