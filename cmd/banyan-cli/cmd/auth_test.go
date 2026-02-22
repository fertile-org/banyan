package cmd

import (
	"path/filepath"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestValidateCLIConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			CLI: types.CLIConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "50051",
			},
		}
		if err := validateCLIConfig(cfg); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing engine host", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			CLI: types.CLIConfig{
				EnginePort: "50051",
			},
		}
		if err := validateCLIConfig(cfg); err == nil {
			t.Error("expected error for missing engine host")
		}
	})

	t.Run("missing engine port", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			CLI: types.CLIConfig{
				EngineHost: "192.168.1.10",
			},
		}
		if err := validateCLIConfig(cfg); err == nil {
			t.Error("expected error for missing engine port")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := &types.BanyanConfig{}
		if err := validateCLIConfig(cfg); err == nil {
			t.Error("expected error for empty config")
		}
	})
}

func TestRunCLIAuth_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = filepath.Join(t.TempDir(), "nonexistent.yaml")

	err := runCLIAuth(nil, nil)
	if err == nil {
		t.Fatal("expected error when no CLI config exists")
	}
	expected := "no CLI config found"
	if err.Error() != expected+". Run 'banyan-cli init' first" {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestRunCLIAuth_IncompleteConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tests := []struct {
		name string
		cfg  types.BanyanConfig
	}{
		{
			name: "missing engine host",
			cfg: types.BanyanConfig{
				CLI: types.CLIConfig{
					EnginePort: "50051",
				},
			},
		},
		{
			name: "missing engine port",
			cfg: types.BanyanConfig{
				CLI: types.CLIConfig{
					EngineHost: "192.168.1.10",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cfgPath := filepath.Join(tmpDir, "banyan.yaml")
			if err := types.SaveConfig(cfgPath, &tt.cfg); err != nil {
				t.Fatalf("SaveConfig failed: %v", err)
			}
			configPath = cfgPath

			err := runCLIAuth(nil, nil)
			if err == nil {
				t.Error("expected error for incomplete config")
			}
		})
	}
}
