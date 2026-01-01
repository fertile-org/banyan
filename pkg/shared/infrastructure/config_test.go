package infrastructure

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/shared/domain"
)

func TestConfigLoader(t *testing.T) {
	logger := &NopLogger{}

	t.Run("NewConfigLoader", func(t *testing.T) {
		loader := NewConfigLoader(logger)
		if loader == nil {
			t.Error("expected non-nil loader")
		}
	})

	t.Run("LoadFile with valid YAML", func(t *testing.T) {
		// Create a temp config file
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "test.yaml")

		content := `
server:
  host: "localhost"
  port: 9090
logging:
  level: "debug"
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		loader := NewConfigLoader(logger)

		var cfg EngineConfig
		if err := loader.LoadFile(configPath, &cfg); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if cfg.Server.Host != "localhost" {
			t.Errorf("expected host 'localhost', got '%s'", cfg.Server.Host)
		}
		if cfg.Server.Port != 9090 {
			t.Errorf("expected port 9090, got %d", cfg.Server.Port)
		}
		if cfg.Logging.Level != "debug" {
			t.Errorf("expected level 'debug', got '%s'", cfg.Logging.Level)
		}
	})

	t.Run("LoadFile with env expansion", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "env.yaml")

		content := `
server:
  host: "${TEST_HOST}"
  port: 8080
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		os.Setenv("TEST_HOST", "expanded-host")
		defer os.Unsetenv("TEST_HOST")

		loader := NewConfigLoader(logger)

		var cfg EngineConfig
		if err := loader.LoadFile(configPath, &cfg); err != nil {
			t.Fatalf("failed to load config: %v", err)
		}

		if cfg.Server.Host != "expanded-host" {
			t.Errorf("expected host 'expanded-host', got '%s'", cfg.Server.Host)
		}
	})

	t.Run("LoadFile with non-existent file", func(t *testing.T) {
		loader := NewConfigLoader(logger)

		var cfg EngineConfig
		err := loader.LoadFile("/non/existent/path.yaml", &cfg)
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("LoadFile with invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.yaml")

		content := `
invalid: yaml: content: [
`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		loader := NewConfigLoader(logger)

		var cfg EngineConfig
		err := loader.LoadFile(configPath, &cfg)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("Load with no config file", func(t *testing.T) {
		// Change to temp dir where no config exists
		originalDir, _ := os.Getwd()
		tmpDir := t.TempDir()
		os.Chdir(tmpDir)
		defer os.Chdir(originalDir)

		loader := NewConfigLoader(logger)

		var cfg EngineConfig
		// Should not error, just use defaults
		if err := loader.Load(&cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestServerConfig(t *testing.T) {
	t.Run("DefaultServerConfig", func(t *testing.T) {
		cfg := DefaultServerConfig()

		if cfg.Host != "0.0.0.0" {
			t.Errorf("expected host '0.0.0.0', got '%s'", cfg.Host)
		}
		if cfg.Port != 8080 {
			t.Errorf("expected port 8080, got %d", cfg.Port)
		}
		if cfg.MaxConnections != 1000 {
			t.Errorf("expected max connections 1000, got %d", cfg.MaxConnections)
		}
	})

	t.Run("Address", func(t *testing.T) {
		cfg := ServerConfig{Host: "127.0.0.1", Port: 9000}
		if cfg.Address() != "127.0.0.1:9000" {
			t.Errorf("expected '127.0.0.1:9000', got '%s'", cfg.Address())
		}
	})
}

func TestEtcdConfig(t *testing.T) {
	cfg := DefaultEtcdConfig()

	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0] != "localhost:2379" {
		t.Errorf("unexpected endpoints: %v", cfg.Endpoints)
	}

	if cfg.DialTimeout != domain.Duration(5*time.Second) {
		t.Errorf("unexpected dial timeout: %v", cfg.DialTimeout)
	}
}

func TestRegistryConfig(t *testing.T) {
	cfg := DefaultRegistryConfig()

	if cfg.HeartbeatInterval != domain.Duration(10*time.Second) {
		t.Errorf("unexpected heartbeat interval: %v", cfg.HeartbeatInterval)
	}
	if cfg.HeartbeatTimeout != domain.Duration(30*time.Second) {
		t.Errorf("unexpected heartbeat timeout: %v", cfg.HeartbeatTimeout)
	}
}

func TestStateConfig(t *testing.T) {
	cfg := DefaultStateConfig()

	if cfg.ReconcileInterval != domain.Duration(30*time.Second) {
		t.Errorf("unexpected reconcile interval: %v", cfg.ReconcileInterval)
	}
}

func TestPluginConfig(t *testing.T) {
	cfg := DefaultPluginConfig()

	if cfg.Directory != "/etc/banyan/plugins" {
		t.Errorf("unexpected directory: %s", cfg.Directory)
	}
	if cfg.MaxConcurrent != 10 {
		t.Errorf("unexpected max concurrent: %d", cfg.MaxConcurrent)
	}
}

func TestContainerConfig(t *testing.T) {
	cfg := DefaultContainerConfig()

	if cfg.Runtime != "containerd" {
		t.Errorf("unexpected runtime: %s", cfg.Runtime)
	}
	if cfg.Namespace != "banyan" {
		t.Errorf("unexpected namespace: %s", cfg.Namespace)
	}
}

func TestNetworkConfig(t *testing.T) {
	cfg := DefaultNetworkConfig()

	if cfg.CNIConfigDir != "/etc/cni/net.d" {
		t.Errorf("unexpected CNI config dir: %s", cfg.CNIConfigDir)
	}
	if cfg.DefaultCIDR != "10.244.0.0/16" {
		t.Errorf("unexpected default CIDR: %s", cfg.DefaultCIDR)
	}
}

func TestHealthConfig(t *testing.T) {
	cfg := DefaultHealthConfig()

	if cfg.FailureThreshold != 3 {
		t.Errorf("unexpected failure threshold: %d", cfg.FailureThreshold)
	}
	if cfg.SuccessThreshold != 1 {
		t.Errorf("unexpected success threshold: %d", cfg.SuccessThreshold)
	}
}

func TestLoggingConfig(t *testing.T) {
	cfg := DefaultLoggingConfig()

	if cfg.Level != "info" {
		t.Errorf("unexpected level: %s", cfg.Level)
	}
	if cfg.Format != "json" {
		t.Errorf("unexpected format: %s", cfg.Format)
	}
}

func TestMetricsConfig(t *testing.T) {
	cfg := DefaultMetricsConfig()

	if !cfg.Enabled {
		t.Error("expected metrics enabled by default")
	}
	if cfg.Path != "/metrics" {
		t.Errorf("unexpected path: %s", cfg.Path)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		value := GetEnvOrDefault("TEST_VAR", "default")
		if value != "test_value" {
			t.Errorf("expected 'test_value', got '%s'", value)
		}
	})

	t.Run("returns default when not set", func(t *testing.T) {
		value := GetEnvOrDefault("NON_EXISTENT_VAR", "default")
		if value != "default" {
			t.Errorf("expected 'default', got '%s'", value)
		}
	})
}

func TestGetEnvIntOrDefault(t *testing.T) {
	t.Run("returns env value when set and valid", func(t *testing.T) {
		os.Setenv("TEST_INT", "42")
		defer os.Unsetenv("TEST_INT")

		value := GetEnvIntOrDefault("TEST_INT", 10)
		if value != 42 {
			t.Errorf("expected 42, got %d", value)
		}
	})

	t.Run("returns default when not set", func(t *testing.T) {
		value := GetEnvIntOrDefault("NON_EXISTENT_INT", 10)
		if value != 10 {
			t.Errorf("expected 10, got %d", value)
		}
	})

	t.Run("returns default when invalid", func(t *testing.T) {
		os.Setenv("TEST_INVALID_INT", "not_a_number")
		defer os.Unsetenv("TEST_INVALID_INT")

		value := GetEnvIntOrDefault("TEST_INVALID_INT", 10)
		if value != 10 {
			t.Errorf("expected 10, got %d", value)
		}
	})
}

func TestGetEnvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		def      bool
		expected bool
	}{
		{"true string", "true", false, true},
		{"TRUE string", "TRUE", false, true},
		{"1 string", "1", false, true},
		{"false string", "false", true, false},
		{"0 string", "0", true, false},
		{"empty returns default true", "", true, true},
		{"empty returns default false", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("TEST_BOOL", tt.envValue)
				defer os.Unsetenv("TEST_BOOL")
			}

			key := "TEST_BOOL"
			if tt.envValue == "" {
				key = "NON_EXISTENT_BOOL"
			}

			value := GetEnvBoolOrDefault(key, tt.def)
			if value != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, value)
			}
		})
	}
}

func TestGetEnvDurationOrDefault(t *testing.T) {
	t.Run("returns env value when set and valid", func(t *testing.T) {
		os.Setenv("TEST_DURATION", "5m")
		defer os.Unsetenv("TEST_DURATION")

		value := GetEnvDurationOrDefault("TEST_DURATION", time.Second)
		if value != 5*time.Minute {
			t.Errorf("expected 5m, got %v", value)
		}
	})

	t.Run("returns default when not set", func(t *testing.T) {
		value := GetEnvDurationOrDefault("NON_EXISTENT_DURATION", 30*time.Second)
		if value != 30*time.Second {
			t.Errorf("expected 30s, got %v", value)
		}
	})

	t.Run("returns default when invalid", func(t *testing.T) {
		os.Setenv("TEST_INVALID_DURATION", "not_a_duration")
		defer os.Unsetenv("TEST_INVALID_DURATION")

		value := GetEnvDurationOrDefault("TEST_INVALID_DURATION", 30*time.Second)
		if value != 30*time.Second {
			t.Errorf("expected 30s, got %v", value)
		}
	})
}
