package cmd

import (
	"path/filepath"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestValidateAgentConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			Agent: types.AgentConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "50051",
				NodeName:   "worker-1",
			},
		}
		if err := validateAgentConfig(cfg); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing engine host", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			Agent: types.AgentConfig{
				EnginePort: "50051",
				NodeName:   "worker-1",
			},
		}
		if err := validateAgentConfig(cfg); err == nil {
			t.Error("expected error for missing engine host")
		}
	})

	t.Run("missing engine port", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			Agent: types.AgentConfig{
				EngineHost: "192.168.1.10",
				NodeName:   "worker-1",
			},
		}
		if err := validateAgentConfig(cfg); err == nil {
			t.Error("expected error for missing engine port")
		}
	})

	t.Run("missing node name", func(t *testing.T) {
		cfg := &types.BanyanConfig{
			Agent: types.AgentConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "50051",
			},
		}
		if err := validateAgentConfig(cfg); err == nil {
			t.Error("expected error for missing node name")
		}
	})

	t.Run("empty config", func(t *testing.T) {
		cfg := &types.BanyanConfig{}
		if err := validateAgentConfig(cfg); err == nil {
			t.Error("expected error for empty config")
		}
	})
}

func TestRunAgentAuth_NoConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	configPath = filepath.Join(t.TempDir(), "nonexistent.yaml")

	err := runAgentAuth(nil, nil)
	if err == nil {
		t.Fatal("expected error when no agent config exists")
	}
	expected := "no agent config found"
	if err.Error() != expected+". Run 'banyan-agent init' first" {
		t.Errorf("expected error containing %q, got %q", expected, err.Error())
	}
}

func TestRunAgentAuth_IncompleteConfig(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tests := []struct {
		name string
		cfg  types.BanyanConfig
	}{
		{
			name: "missing engine host",
			cfg: types.BanyanConfig{
				Agent: types.AgentConfig{
					EnginePort: "50051",
					NodeName:   "worker-1",
				},
			},
		},
		{
			name: "missing engine port",
			cfg: types.BanyanConfig{
				Agent: types.AgentConfig{
					EngineHost: "192.168.1.10",
					NodeName:   "worker-1",
				},
			},
		},
		{
			name: "missing node name",
			cfg: types.BanyanConfig{
				Agent: types.AgentConfig{
					EngineHost: "192.168.1.10",
					EnginePort: "50051",
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

			err := runAgentAuth(nil, nil)
			if err == nil {
				t.Error("expected error for incomplete config")
			}
		})
	}
}
