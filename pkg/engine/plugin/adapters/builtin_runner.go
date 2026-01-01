// Package adapters provides implementations of the outbound ports for the Plugin Manager.
package adapters

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// BuiltinRunner executes built-in Go plugins.
type BuiltinRunner struct {
	plugins map[string]outbound.BuiltinPlugin
	logger  *slog.Logger
}

// Ensure BuiltinRunner implements PluginRunner.
var _ outbound.PluginRunner = (*BuiltinRunner)(nil)

// NewBuiltinRunner creates a new BuiltinRunner.
func NewBuiltinRunner(logger *slog.Logger) *BuiltinRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &BuiltinRunner{
		plugins: make(map[string]outbound.BuiltinPlugin),
		logger:  logger,
	}
}

// RegisterBuiltin registers a builtin plugin.
func (r *BuiltinRunner) RegisterBuiltin(plugin outbound.BuiltinPlugin) {
	r.plugins[plugin.Name()] = plugin
}

// CanRun checks if the runner can handle this plugin type.
func (r *BuiltinRunner) CanRun(plugin *domain.Plugin) bool {
	return plugin.Type == domain.PluginTypeBuiltin
}

// Run executes a builtin plugin.
func (r *BuiltinRunner) Run(
	ctx context.Context,
	plugin *domain.Plugin,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.PluginResult, error) {
	builtin, ok := r.plugins[plugin.Spec.BuiltinName]
	if !ok {
		r.logger.Error("builtin plugin not found",
			"name", plugin.Spec.BuiltinName,
		)
		return nil, fmt.Errorf("%w: %s", pluginerrors.ErrBuiltinNotFound, plugin.Spec.BuiltinName)
	}

	return builtin.Execute(ctx, execCtx)
}

// ResourceValidatorPlugin validates resource requests.
type ResourceValidatorPlugin struct{}

// Ensure ResourceValidatorPlugin implements BuiltinPlugin.
var _ outbound.BuiltinPlugin = (*ResourceValidatorPlugin)(nil)

// Name returns the plugin name.
func (p *ResourceValidatorPlugin) Name() string {
	return "resource-validator"
}

// Execute validates resource limits on containers.
func (p *ResourceValidatorPlugin) Execute(
	_ context.Context,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.PluginResult, error) {
	if execCtx.ServiceSpec == nil {
		return &domain.PluginResult{
			PluginName: p.Name(),
			Hook:       execCtx.Hook,
			Success:    true,
			Message:    "no spec to validate",
		}, nil
	}

	// Validate CPU and memory limits
	for _, container := range execCtx.ServiceSpec.Containers {
		if container.Resources.CPULimit == 0 {
			return &domain.PluginResult{
				PluginName: p.Name(),
				Hook:       execCtx.Hook,
				Success:    false,
				Message:    fmt.Sprintf("container %s missing CPU limit", container.Name),
			}, nil
		}
		if container.Resources.MemoryLimitMB == 0 {
			return &domain.PluginResult{
				PluginName: p.Name(),
				Hook:       execCtx.Hook,
				Success:    false,
				Message:    fmt.Sprintf("container %s missing memory limit", container.Name),
			}, nil
		}
	}

	return &domain.PluginResult{
		PluginName: p.Name(),
		Hook:       execCtx.Hook,
		Success:    true,
		Message:    "resource validation passed",
	}, nil
}

// AuditLoggerPlugin logs plugin execution for audit purposes.
type AuditLoggerPlugin struct {
	logger *slog.Logger
}

// Ensure AuditLoggerPlugin implements BuiltinPlugin.
var _ outbound.BuiltinPlugin = (*AuditLoggerPlugin)(nil)

// NewAuditLoggerPlugin creates a new AuditLoggerPlugin.
func NewAuditLoggerPlugin(logger *slog.Logger) *AuditLoggerPlugin {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditLoggerPlugin{logger: logger}
}

// Name returns the plugin name.
func (p *AuditLoggerPlugin) Name() string {
	return "audit-logger"
}

// Execute logs audit information.
func (p *AuditLoggerPlugin) Execute(
	_ context.Context,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.PluginResult, error) {
	p.logger.Info("audit log",
		"hook", execCtx.Hook,
		"deployment_id", execCtx.DeploymentID,
		"service_name", execCtx.ServiceName,
		"namespace", execCtx.Namespace,
	)

	return &domain.PluginResult{
		PluginName: p.Name(),
		Hook:       execCtx.Hook,
		Success:    true,
		Message:    "audit log recorded",
	}, nil
}

// NamespaceLabelerPlugin adds labels to services based on namespace.
type NamespaceLabelerPlugin struct{}

// Ensure NamespaceLabelerPlugin implements BuiltinPlugin.
var _ outbound.BuiltinPlugin = (*NamespaceLabelerPlugin)(nil)

// Name returns the plugin name.
func (p *NamespaceLabelerPlugin) Name() string {
	return "namespace-labeler"
}

// Execute adds namespace labels.
func (p *NamespaceLabelerPlugin) Execute(
	_ context.Context,
	execCtx domain.ExecutionContext, //nolint:gocritic // hugeParam - interface requirement
) (*domain.PluginResult, error) {
	if execCtx.ServiceSpec == nil || execCtx.Namespace == "" {
		return &domain.PluginResult{
			PluginName: p.Name(),
			Hook:       execCtx.Hook,
			Success:    true,
			Message:    "no labeling needed",
		}, nil
	}

	// Create a mutation to add namespace label
	mutations := []domain.Mutation{
		{
			Path:      "metadata.labels.namespace",
			Operation: domain.MutationOpSet,
			Value:     execCtx.Namespace,
		},
	}

	return &domain.PluginResult{
		PluginName: p.Name(),
		Hook:       execCtx.Hook,
		Success:    true,
		Message:    "namespace label added",
		Mutations:  mutations,
	}, nil
}
