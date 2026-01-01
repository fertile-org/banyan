package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fertile-org/banyan/pkg/engine/plugin/adapters"
	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	"github.com/fertile-org/banyan/pkg/engine/plugin/ports/outbound"
)

// MockPluginRunner is a mock implementation of PluginRunner.
type MockPluginRunner struct {
	canRunFunc func(plugin *domain.Plugin) bool
	runFunc    func(ctx context.Context, plugin *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error)
}

func (m *MockPluginRunner) CanRun(plugin *domain.Plugin) bool {
	if m.canRunFunc != nil {
		return m.canRunFunc(plugin)
	}
	return true
}

func (m *MockPluginRunner) Run(ctx context.Context, plugin *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, plugin, execCtx)
	}
	return &domain.PluginResult{
		PluginName: plugin.Name,
		Hook:       execCtx.Hook,
		Success:    true,
		Message:    "mock success",
	}, nil
}

func TestExecutorUseCase_ExecuteHook_Success(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	// Register a test plugin
	plugin := &domain.Plugin{
		Name:    "test-plugin",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Enabled: true,
		Spec:    domain.PluginSpec{BuiltinName: "resource-validator"},
	}
	err := registryUC.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Create executor with mock runner
	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
				Message:    "validation passed",
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.Len(t, results.Results, 1)
	assert.Equal(t, "test-plugin", results.Results[0].PluginName)
	assert.True(t, results.Results[0].Success)
}

func TestExecutorUseCase_ExecuteHook_NoPlugins(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	executorUC := NewExecutorUseCase(repo, nil, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.Len(t, results.Results, 0)
}

func TestExecutorUseCase_ExecuteHook_DisabledPlugins(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	// Register a disabled plugin
	plugin := &domain.Plugin{
		Name:    "disabled-plugin",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Enabled: false, // Disabled
		Spec:    domain.PluginSpec{BuiltinName: "resource-validator"},
	}
	err := registryUC.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	mockRunner := &MockPluginRunner{}
	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.Len(t, results.Results, 0) // Disabled plugin not executed
}

func TestExecutorUseCase_ExecuteHook_PriorityOrder(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	// Register plugins with different priorities
	plugins := []*domain.Plugin{
		{
			Name:     "low-priority",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPreValidate},
			Enabled:  true,
			Priority: 100, // Lower priority (higher number)
			Spec:     domain.PluginSpec{BuiltinName: "audit-logger"},
		},
		{
			Name:     "high-priority",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPreValidate},
			Enabled:  true,
			Priority: 1, // Higher priority (lower number)
			Spec:     domain.PluginSpec{BuiltinName: "resource-validator"},
		},
		{
			Name:     "medium-priority",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPreValidate},
			Enabled:  true,
			Priority: 50,
			Spec:     domain.PluginSpec{BuiltinName: "namespace-labeler"},
		},
	}

	for _, p := range plugins {
		err := registryUC.RegisterPlugin(context.Background(), p)
		require.NoError(t, err)
	}

	executionOrder := make([]string, 0)
	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			executionOrder = append(executionOrder, p.Name)
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.Len(t, results.Results, 3)

	// Verify execution order (sorted by priority)
	assert.Equal(t, []string{"high-priority", "medium-priority", "low-priority"}, executionOrder)
}

func TestExecutorUseCase_ExecuteHook_FailFastOnValidation(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	// Register multiple plugins
	plugins := []*domain.Plugin{
		{
			Name:     "first",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPreValidate},
			Enabled:  true,
			Priority: 1,
			Spec:     domain.PluginSpec{BuiltinName: "first"},
		},
		{
			Name:     "second",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPreValidate},
			Enabled:  true,
			Priority: 2,
			Spec:     domain.PluginSpec{BuiltinName: "second"},
		},
	}

	for _, p := range plugins {
		err := registryUC.RegisterPlugin(context.Background(), p)
		require.NoError(t, err)
	}

	executionCount := 0
	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			executionCount++
			// First plugin fails
			if p.Name == "first" {
				return &domain.PluginResult{
					PluginName: p.Name,
					Hook:       execCtx.Hook,
					Success:    false,
					Error:      errors.New("validation failed"),
				}, nil
			}
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.False(t, results.AllPassed)
	assert.Len(t, results.Results, 1) // Only first plugin executed due to fail-fast
	assert.Equal(t, 1, executionCount)
}

func TestExecutorUseCase_ExecuteHook_NoFailFastOnNonValidation(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	// Register multiple plugins for post-deploy (non-validation)
	plugins := []*domain.Plugin{
		{
			Name:     "first",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPostDeploy},
			Enabled:  true,
			Priority: 1,
			Spec:     domain.PluginSpec{BuiltinName: "first"},
		},
		{
			Name:     "second",
			Type:     domain.PluginTypeBuiltin,
			Hooks:    []domain.HookPoint{domain.HookPostDeploy},
			Enabled:  true,
			Priority: 2,
			Spec:     domain.PluginSpec{BuiltinName: "second"},
		},
	}

	for _, p := range plugins {
		err := registryUC.RegisterPlugin(context.Background(), p)
		require.NoError(t, err)
	}

	executionCount := 0
	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			executionCount++
			// First plugin fails, but should continue to second
			if p.Name == "first" {
				return &domain.PluginResult{
					PluginName: p.Name,
					Hook:       execCtx.Hook,
					Success:    false,
					Error:      errors.New("failed"),
				}, nil
			}
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPostDeploy, execCtx)
	require.NoError(t, err)
	assert.False(t, results.AllPassed)
	assert.Len(t, results.Results, 2) // Both plugins executed
	assert.Equal(t, 2, executionCount)
}

func TestExecutorUseCase_ExecuteHook_NoRunner(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:    "test-plugin",
		Type:    domain.PluginTypeGRPC, // gRPC type
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Enabled: true,
		Spec: domain.PluginSpec{
			GRPCAddress: "localhost:50051",
			GRPCMethod:  "Execute",
		},
	}
	err := registryUC.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Create executor with no runners for gRPC
	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin // Only builtin
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.False(t, results.AllPassed)
	assert.Len(t, results.Results, 1)
	assert.False(t, results.Results[0].Success)
	assert.NotNil(t, results.Results[0].Error)
}

func TestExecutorUseCase_ExecuteHook_WithMutations(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:    "mutating-plugin",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreDeploy},
		Enabled: true,
		Spec:    domain.PluginSpec{BuiltinName: "namespace-labeler"},
	}
	err := registryUC.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
				Mutations: []domain.Mutation{
					{
						Path:      "metadata.labels.env",
						Operation: domain.MutationOpSet,
						Value:     "production",
					},
				},
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreDeploy, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.Len(t, results.AllMutations(), 1)
	assert.Equal(t, "metadata.labels.env", results.AllMutations()[0].Path)
}

func TestExecutorUseCase_ExecuteHook_Duration(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	registryUC := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:    "slow-plugin",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Enabled: true,
		Spec:    domain.PluginSpec{BuiltinName: "slow"},
	}
	err := registryUC.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	mockRunner := &MockPluginRunner{
		canRunFunc: func(p *domain.Plugin) bool {
			return p.Type == domain.PluginTypeBuiltin
		},
		runFunc: func(_ context.Context, p *domain.Plugin, execCtx domain.ExecutionContext) (*domain.PluginResult, error) {
			time.Sleep(10 * time.Millisecond)
			return &domain.PluginResult{
				PluginName: p.Name,
				Hook:       execCtx.Hook,
				Success:    true,
			}, nil
		},
	}

	executorUC := NewExecutorUseCase(repo, []outbound.PluginRunner{mockRunner}, nil)

	execCtx := domain.ExecutionContext{
		DeploymentID: "deploy-1",
		ServiceName:  "web",
	}

	results, err := executorUC.ExecuteHook(context.Background(), domain.HookPreValidate, execCtx)
	require.NoError(t, err)
	assert.True(t, results.AllPassed)
	assert.GreaterOrEqual(t, results.Duration, 10*time.Millisecond)
	assert.GreaterOrEqual(t, results.Results[0].Duration, 10*time.Millisecond)
}
