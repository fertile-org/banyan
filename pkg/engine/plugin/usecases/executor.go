// Package usecases implements the business logic for the Plugin Manager.
package usecases

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// ExecutorUseCase handles plugin execution.
type ExecutorUseCase struct {
	repo    outbound.PluginRepository
	runners []outbound.PluginRunner
	logger  *slog.Logger
}

// NewExecutorUseCase creates a new ExecutorUseCase.
func NewExecutorUseCase(
	repo outbound.PluginRepository,
	runners []outbound.PluginRunner,
	logger *slog.Logger,
) *ExecutorUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExecutorUseCase{
		repo:    repo,
		runners: runners,
		logger:  logger,
	}
}

// ExecuteHook executes all plugins for a given hook.
func (uc *ExecutorUseCase) ExecuteHook(
	ctx context.Context,
	hook domain.HookPoint,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - modified during execution
) (*domain.HookResults, error) {
	start := time.Now()

	// Get plugins for this hook
	plugins, err := uc.repo.FindByHook(ctx, hook)
	if err != nil {
		return nil, err
	}

	// Filter enabled plugins and sort by priority
	plugins = uc.filterAndSort(plugins)

	if len(plugins) == 0 {
		return &domain.HookResults{
			Hook:      hook,
			Results:   nil,
			AllPassed: true,
			Duration:  time.Since(start),
		}, nil
	}

	execCtx.Hook = hook
	results := make([]domain.PluginResult, 0, len(plugins))
	allPassed := true

	for i := range plugins {
		plugin := &plugins[i]
		result := uc.executePlugin(ctx, plugin, execCtx)
		results = append(results, *result)

		if !result.Success {
			allPassed = false

			// Fail fast for validation hooks
			if hook.IsValidationHook() {
				uc.logger.Warn("plugin failed, stopping execution",
					"plugin", plugin.Name,
					"hook", hook,
					"error", result.Error,
				)
				break
			}
		}

		// Apply mutations to context for next plugin
		if len(result.Mutations) > 0 {
			execCtx = uc.applyMutations(execCtx, result.Mutations)
		}
	}

	return &domain.HookResults{
		Hook:      hook,
		Results:   results,
		AllPassed: allPassed,
		Duration:  time.Since(start),
	}, nil
}

// executePlugin executes a single plugin.
func (uc *ExecutorUseCase) executePlugin(
	ctx context.Context,
	plugin *domain.Plugin,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - passed to runner
) *domain.PluginResult {
	start := time.Now()

	// Find appropriate runner
	var runner outbound.PluginRunner
	for _, r := range uc.runners {
		if r.CanRun(plugin) {
			runner = r
			break
		}
	}

	if runner == nil {
		uc.logger.Error("no runner available", "plugin", plugin.Name, "type", plugin.Type)
		return &domain.PluginResult{
			PluginName: plugin.Name,
			Hook:       execCtx.Hook,
			Success:    false,
			Error:      pluginerrors.ErrNoRunnerAvailable,
			Duration:   time.Since(start),
		}
	}

	// Execute with timeout
	timeout := plugin.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	execCtx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := runner.Run(execCtx2, plugin, execCtx)
	if err != nil {
		// Check if it's a timeout
		if execCtx2.Err() == context.DeadlineExceeded {
			uc.logger.Warn("plugin execution timed out",
				"plugin", plugin.Name,
				"timeout", timeout,
			)
			return &domain.PluginResult{
				PluginName: plugin.Name,
				Hook:       execCtx.Hook,
				Success:    false,
				Error:      pluginerrors.ErrPluginTimeout,
				Duration:   time.Since(start),
			}
		}

		uc.logger.Error("plugin execution failed",
			"plugin", plugin.Name,
			"error", err,
		)
		return &domain.PluginResult{
			PluginName: plugin.Name,
			Hook:       execCtx.Hook,
			Success:    false,
			Error:      err,
			Duration:   time.Since(start),
		}
	}

	result.Duration = time.Since(start)
	return result
}

// filterAndSort filters enabled plugins and sorts by priority.
func (uc *ExecutorUseCase) filterAndSort(plugins []domain.Plugin) []domain.Plugin {
	var enabled []domain.Plugin
	for i := range plugins {
		if plugins[i].Enabled {
			enabled = append(enabled, plugins[i])
		}
	}

	// Sort by priority (lower = higher priority)
	sort.Slice(enabled, func(i, j int) bool {
		return enabled[i].Priority < enabled[j].Priority
	})

	return enabled
}

// applyMutations applies mutations to the execution context.
func (uc *ExecutorUseCase) applyMutations(
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - returns modified copy
	mutations []domain.Mutation,
) domain.ExecutionContext {
	// Clone the context to avoid modifying the original
	newCtx := *execCtx.Clone()

	for _, m := range mutations {
		uc.logger.Debug("applying mutation",
			"path", m.Path,
			"operation", m.Operation,
			"value", m.Value,
		)
		// TODO: Implement mutation application using JSON path
		// For now, just log the mutations
	}

	return newCtx
}

// AddRunner adds a runner to the executor.
func (uc *ExecutorUseCase) AddRunner(runner outbound.PluginRunner) {
	uc.runners = append(uc.runners, runner)
}
