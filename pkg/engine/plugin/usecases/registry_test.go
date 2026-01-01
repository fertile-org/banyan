package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fertile-org/banyan/pkg/engine/plugin/adapters"
	"github.com/fertile-org/banyan/pkg/engine/plugin/domain"
	pluginerrors "github.com/fertile-org/banyan/pkg/engine/plugin/errors"
)

func TestRegistryUseCase_RegisterPlugin(t *testing.T) {
	tests := []struct {
		name    string
		plugin  *domain.Plugin
		wantErr error
	}{
		{
			name: "valid webhook plugin",
			plugin: &domain.Plugin{
				Name:    "test-webhook",
				Type:    domain.PluginTypeWebhook,
				Hooks:   []domain.HookPoint{domain.HookPreDeploy},
				Enabled: true,
				Spec: domain.PluginSpec{
					WebhookURL: "http://example.com/hook",
				},
			},
			wantErr: nil,
		},
		{
			name: "valid builtin plugin",
			plugin: &domain.Plugin{
				Name:    "test-builtin",
				Type:    domain.PluginTypeBuiltin,
				Hooks:   []domain.HookPoint{domain.HookPreValidate},
				Enabled: true,
				Spec: domain.PluginSpec{
					BuiltinName: "resource-validator",
				},
			},
			wantErr: nil,
		},
		{
			name: "valid grpc plugin",
			plugin: &domain.Plugin{
				Name:    "test-grpc",
				Type:    domain.PluginTypeGRPC,
				Hooks:   []domain.HookPoint{domain.HookPostDeploy},
				Enabled: true,
				Spec: domain.PluginSpec{
					GRPCAddress: "localhost:50051",
					GRPCMethod:  "Execute",
				},
			},
			wantErr: nil,
		},
		{
			name: "missing name",
			plugin: &domain.Plugin{
				Type:  domain.PluginTypeWebhook,
				Hooks: []domain.HookPoint{domain.HookPreDeploy},
				Spec: domain.PluginSpec{
					WebhookURL: "http://example.com/hook",
				},
			},
			wantErr: pluginerrors.ErrInvalidPlugin,
		},
		{
			name: "missing hooks",
			plugin: &domain.Plugin{
				Name: "no-hooks",
				Type: domain.PluginTypeWebhook,
				Spec: domain.PluginSpec{
					WebhookURL: "http://example.com/hook",
				},
			},
			wantErr: pluginerrors.ErrInvalidPlugin,
		},
		{
			name: "webhook missing url",
			plugin: &domain.Plugin{
				Name:  "webhook-no-url",
				Type:  domain.PluginTypeWebhook,
				Hooks: []domain.HookPoint{domain.HookPreDeploy},
			},
			wantErr: pluginerrors.ErrInvalidPlugin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := adapters.NewMemoryPluginRepository()
			uc := NewRegistryUseCase(repo, nil)

			err := uc.RegisterPlugin(context.Background(), tt.plugin)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)

				// Verify plugin was saved
				saved, err := uc.GetPlugin(context.Background(), tt.plugin.Name)
				require.NoError(t, err)
				assert.Equal(t, tt.plugin.Name, saved.Name)
				assert.NotZero(t, saved.CreatedAt)
				assert.NotZero(t, saved.UpdatedAt)
			}
		})
	}
}

func TestRegistryUseCase_DuplicatePlugin(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:  "duplicate",
		Type:  domain.PluginTypeBuiltin,
		Hooks: []domain.HookPoint{domain.HookPreValidate},
		Spec: domain.PluginSpec{
			BuiltinName: "resource-validator",
		},
	}

	// First registration should succeed
	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Second registration should fail
	err = uc.RegisterPlugin(context.Background(), plugin)
	assert.ErrorIs(t, err, pluginerrors.ErrPluginAlreadyExists)
}

func TestRegistryUseCase_UnregisterPlugin(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:  "to-remove",
		Type:  domain.PluginTypeBuiltin,
		Hooks: []domain.HookPoint{domain.HookPreValidate},
		Spec: domain.PluginSpec{
			BuiltinName: "resource-validator",
		},
	}

	// Register plugin
	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Unregister plugin
	err = uc.UnregisterPlugin(context.Background(), "to-remove")
	require.NoError(t, err)

	// Plugin should not exist
	_, err = uc.GetPlugin(context.Background(), "to-remove")
	assert.ErrorIs(t, err, pluginerrors.ErrPluginNotFound)
}

func TestRegistryUseCase_UnregisterNonexistent(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	err := uc.UnregisterPlugin(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, pluginerrors.ErrPluginNotFound)
}

func TestRegistryUseCase_UpdatePlugin(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:    "to-update",
		Type:    domain.PluginTypeWebhook,
		Hooks:   []domain.HookPoint{domain.HookPreDeploy},
		Enabled: true,
		Spec: domain.PluginSpec{
			WebhookURL: "http://example.com/hook",
		},
	}

	// Register plugin
	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Update plugin
	plugin.Spec.WebhookURL = "http://example.com/new-hook"
	plugin.Priority = 10

	err = uc.UpdatePlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Verify update
	updated, err := uc.GetPlugin(context.Background(), "to-update")
	require.NoError(t, err)
	assert.Equal(t, "http://example.com/new-hook", updated.Spec.WebhookURL)
	assert.Equal(t, 10, updated.Priority)
}

func TestRegistryUseCase_ListPluginsByHook(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	// Register plugins with different hooks
	plugins := []*domain.Plugin{
		{
			Name:  "pre-validate-1",
			Type:  domain.PluginTypeBuiltin,
			Hooks: []domain.HookPoint{domain.HookPreValidate},
			Spec:  domain.PluginSpec{BuiltinName: "resource-validator"},
		},
		{
			Name:  "pre-validate-2",
			Type:  domain.PluginTypeBuiltin,
			Hooks: []domain.HookPoint{domain.HookPreValidate, domain.HookPostDeploy},
			Spec:  domain.PluginSpec{BuiltinName: "audit-logger"},
		},
		{
			Name:  "post-deploy-only",
			Type:  domain.PluginTypeBuiltin,
			Hooks: []domain.HookPoint{domain.HookPostDeploy},
			Spec:  domain.PluginSpec{BuiltinName: "audit-logger"},
		},
	}

	for _, p := range plugins {
		err := uc.RegisterPlugin(context.Background(), p)
		require.NoError(t, err)
	}

	// List pre-validate plugins
	preValidatePlugins, err := uc.ListPluginsByHook(context.Background(), domain.HookPreValidate)
	require.NoError(t, err)
	assert.Len(t, preValidatePlugins, 2)

	// List post-deploy plugins
	postDeployPlugins, err := uc.ListPluginsByHook(context.Background(), domain.HookPostDeploy)
	require.NoError(t, err)
	assert.Len(t, postDeployPlugins, 2)

	// List pre-deploy plugins (none)
	preDeployPlugins, err := uc.ListPluginsByHook(context.Background(), domain.HookPreDeploy)
	require.NoError(t, err)
	assert.Len(t, preDeployPlugins, 0)
}

func TestRegistryUseCase_EnableDisablePlugin(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:    "toggle",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Enabled: true,
		Spec:    domain.PluginSpec{BuiltinName: "resource-validator"},
	}

	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Disable plugin
	err = uc.DisablePlugin(context.Background(), "toggle")
	require.NoError(t, err)

	disabled, err := uc.GetPlugin(context.Background(), "toggle")
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)

	// Enable plugin
	err = uc.EnablePlugin(context.Background(), "toggle")
	require.NoError(t, err)

	enabled, err := uc.GetPlugin(context.Background(), "toggle")
	require.NoError(t, err)
	assert.True(t, enabled.Enabled)
}

func TestRegistryUseCase_SetPriority(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:     "priority-test",
		Type:     domain.PluginTypeBuiltin,
		Hooks:    []domain.HookPoint{domain.HookPreValidate},
		Priority: 0,
		Spec:     domain.PluginSpec{BuiltinName: "resource-validator"},
	}

	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	// Set priority
	err = uc.SetPriority(context.Background(), "priority-test", 100)
	require.NoError(t, err)

	updated, err := uc.GetPlugin(context.Background(), "priority-test")
	require.NoError(t, err)
	assert.Equal(t, 100, updated.Priority)
}

func TestRegistryUseCase_DefaultTimeout(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	plugin := &domain.Plugin{
		Name:  "no-timeout",
		Type:  domain.PluginTypeBuiltin,
		Hooks: []domain.HookPoint{domain.HookPreValidate},
		Spec:  domain.PluginSpec{BuiltinName: "resource-validator"},
		// No timeout set
	}

	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	saved, err := uc.GetPlugin(context.Background(), "no-timeout")
	require.NoError(t, err)
	assert.Equal(t, DefaultTimeout, saved.Timeout)
}

func TestRegistryUseCase_PreserveCustomTimeout(t *testing.T) {
	repo := adapters.NewMemoryPluginRepository()
	uc := NewRegistryUseCase(repo, nil)

	customTimeout := 5 * time.Minute
	plugin := &domain.Plugin{
		Name:    "custom-timeout",
		Type:    domain.PluginTypeBuiltin,
		Hooks:   []domain.HookPoint{domain.HookPreValidate},
		Spec:    domain.PluginSpec{BuiltinName: "resource-validator"},
		Timeout: customTimeout,
	}

	err := uc.RegisterPlugin(context.Background(), plugin)
	require.NoError(t, err)

	saved, err := uc.GetPlugin(context.Background(), "custom-timeout")
	require.NoError(t, err)
	assert.Equal(t, customTimeout, saved.Timeout)
}
