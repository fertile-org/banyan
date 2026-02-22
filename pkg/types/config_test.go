package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		hash1 := HashPassword("my-secret")
		hash2 := HashPassword("my-secret")
		if hash1 != hash2 {
			t.Errorf("expected identical hashes, got %s and %s", hash1, hash2)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := HashPassword("password-a")
		hash2 := HashPassword("password-b")
		if hash1 == hash2 {
			t.Errorf("expected different hashes for different inputs")
		}
	})

	t.Run("returns 64-char hex string", func(t *testing.T) {
		hash := HashPassword("test")
		if len(hash) != 64 {
			t.Errorf("expected 64-char hex string, got %d chars: %s", len(hash), hash)
		}
	})
}

func TestLoadSaveConfig(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				PasswordHash: HashPassword("my-cluster-secret"),
			},
			Agent: AgentConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "2379",
				AuthToken:  "agent-token-abc",
				NodeName:   "worker-1",
			},
			CLI: CLIConfig{
				AuthToken: "cli-token-xyz",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		info, err := os.Stat(cfgPath)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected 0600 permissions, got %04o", perm)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.PasswordHash != cfg.Engine.PasswordHash {
			t.Errorf("expected password_hash=%s, got %s", cfg.Engine.PasswordHash, loaded.Engine.PasswordHash)
		}
		if loaded.Agent.EngineHost != "192.168.1.10" {
			t.Errorf("expected engine_host=192.168.1.10, got %s", loaded.Agent.EngineHost)
		}
		if loaded.Agent.EnginePort != "2379" {
			t.Errorf("expected engine_port=2379, got %s", loaded.Agent.EnginePort)
		}
		if loaded.Agent.AuthToken != "agent-token-abc" {
			t.Errorf("expected agent auth_token=agent-token-abc, got %s", loaded.Agent.AuthToken)
		}
		if loaded.Agent.NodeName != "worker-1" {
			t.Errorf("expected agent node_name=worker-1, got %s", loaded.Agent.NodeName)
		}
		if loaded.CLI.AuthToken != "cli-token-xyz" {
			t.Errorf("expected cli auth_token=cli-token-xyz, got %s", loaded.CLI.AuthToken)
		}
	})

	t.Run("missing file returns zero value", func(t *testing.T) {
		cfg, err := LoadConfig("/tmp/nonexistent-banyan-test-config.yaml")
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}
		if cfg.Engine.PasswordHash != "" {
			t.Errorf("expected empty password_hash, got %s", cfg.Engine.PasswordHash)
		}
		if cfg.Agent.AuthToken != "" {
			t.Errorf("expected empty agent auth_token, got %s", cfg.Agent.AuthToken)
		}
		if cfg.CLI.AuthToken != "" {
			t.Errorf("expected empty cli auth_token, got %s", cfg.CLI.AuthToken)
		}
	})

	t.Run("round-trip with CLI config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			CLI: CLIConfig{
				EngineHost: "10.0.0.1",
				EnginePort: "8443",
				AuthToken:  "cli-token-123",
			},
			Engine: EngineConfig{
				APIPort: "8443",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.CLI.EngineHost != "10.0.0.1" {
			t.Errorf("expected cli.engine_host=10.0.0.1, got %s", loaded.CLI.EngineHost)
		}
		if loaded.CLI.EnginePort != "8443" {
			t.Errorf("expected cli.engine_port=8443, got %s", loaded.CLI.EnginePort)
		}
		if loaded.CLI.AuthToken != "cli-token-123" {
			t.Errorf("expected cli.auth_token=cli-token-123, got %s", loaded.CLI.AuthToken)
		}
		if loaded.Engine.APIPort != "8443" {
			t.Errorf("expected engine.api_port=8443, got %s", loaded.Engine.APIPort)
		}
	})

	t.Run("round-trip with store backend config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				StoreBackend: "etcd",
				StoreAddress: "http://localhost:2379",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.StoreBackend != "etcd" {
			t.Errorf("expected store_backend=etcd, got %s", loaded.Engine.StoreBackend)
		}
		if loaded.Engine.StoreAddress != "http://localhost:2379" {
			t.Errorf("expected store_address=http://localhost:2379, got %s", loaded.Engine.StoreAddress)
		}
	})

	t.Run("round-trip with etcd auth config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				StoreBackend: "etcd",
				StoreAddress: "https://etcd1.example.com:2379",
				EtcdUsername: "banyan",
				EtcdPassword: "secret",
				EtcdCertFile: "/etc/banyan/etcd-client.crt",
				EtcdKeyFile:  "/etc/banyan/etcd-client.key",
				EtcdCAFile:   "/etc/banyan/etcd-ca.crt",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.EtcdUsername != "banyan" {
			t.Errorf("expected etcd_username=banyan, got %s", loaded.Engine.EtcdUsername)
		}
		if loaded.Engine.EtcdPassword != "secret" {
			t.Errorf("expected etcd_password=secret, got %s", loaded.Engine.EtcdPassword)
		}
		if loaded.Engine.EtcdCertFile != "/etc/banyan/etcd-client.crt" {
			t.Errorf("expected etcd_cert_file=/etc/banyan/etcd-client.crt, got %s", loaded.Engine.EtcdCertFile)
		}
		if loaded.Engine.EtcdKeyFile != "/etc/banyan/etcd-client.key" {
			t.Errorf("expected etcd_key_file=/etc/banyan/etcd-client.key, got %s", loaded.Engine.EtcdKeyFile)
		}
		if loaded.Engine.EtcdCAFile != "/etc/banyan/etcd-ca.crt" {
			t.Errorf("expected etcd_ca_file=/etc/banyan/etcd-ca.crt, got %s", loaded.Engine.EtcdCAFile)
		}
	})
}

func TestGetConfigEngineEndpoint(t *testing.T) {
	t.Run("empty config returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetConfigEngineEndpoint(cfgPath)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("host and port", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Agent: AgentConfig{
				EngineHost: "10.0.0.1",
				EnginePort: "50053",
			},
		}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetConfigEngineEndpoint(cfgPath)
		if result != "10.0.0.1:50053" {
			t.Errorf("expected 10.0.0.1:50053, got %s", result)
		}
	})

	t.Run("default port when not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Agent: AgentConfig{
				EngineHost: "10.0.0.1",
			},
		}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetConfigEngineEndpoint(cfgPath)
		if result != "10.0.0.1:50051" {
			t.Errorf("expected 10.0.0.1:50051, got %s", result)
		}
	})
}

func TestGetCLIEngineEndpoint(t *testing.T) {
	t.Run("empty config returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetCLIEngineEndpoint(cfgPath)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("host and port", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			CLI: CLIConfig{
				EngineHost: "10.0.0.1",
				EnginePort: "8443",
			},
		}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetCLIEngineEndpoint(cfgPath)
		if result != "10.0.0.1:8443" {
			t.Errorf("expected 10.0.0.1:8443, got %s", result)
		}
	})

	t.Run("default port when not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			CLI: CLIConfig{
				EngineHost: "10.0.0.1",
			},
		}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetCLIEngineEndpoint(cfgPath)
		if result != "10.0.0.1:50051" {
			t.Errorf("expected 10.0.0.1:50051, got %s", result)
		}
	})

	t.Run("ignores engine config without cli config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				APIPort: "9999",
			},
		}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		result := GetCLIEngineEndpoint(cfgPath)
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})
}

func TestGetStoreBackend(t *testing.T) {
	t.Run("defaults to etcd when empty", func(t *testing.T) {
		cfg := EngineConfig{}
		if got := cfg.GetStoreBackend(); got != "etcd" {
			t.Errorf("expected 'etcd', got %q", got)
		}
	})

	t.Run("returns configured backend", func(t *testing.T) {
		cfg := EngineConfig{StoreBackend: "etcd"}
		if got := cfg.GetStoreBackend(); got != "etcd" {
			t.Errorf("expected 'etcd', got %q", got)
		}
	})
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("engine: [unterminated"), 0o644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_UnreadableFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "unreadable.yaml")
	os.WriteFile(cfgPath, []byte("engine:\n  password_hash: secret\n"), 0o644)

	// Make file unreadable (skip if root)
	if os.Geteuid() == 0 {
		t.Skip("cannot test unreadable file as root")
	}
	os.Chmod(cfgPath, 0o000)
	t.Cleanup(func() { os.Chmod(cfgPath, 0o644) })

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestSaveConfig_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subdir", "nested", "banyan.yaml")

	cfg := BanyanConfig{Engine: EngineConfig{PasswordHash: "testhash"}}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig should create nested dirs: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Engine.PasswordHash != "testhash" {
		t.Errorf("expected password_hash 'testhash', got %q", loaded.Engine.PasswordHash)
	}
}

func TestGetConfigEngineEndpoint_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("engine: [unterminated"), 0o644)

	result := GetConfigEngineEndpoint(cfgPath)
	if result != "" {
		t.Errorf("expected empty string for invalid config, got %q", result)
	}
}

func TestGetCLIEngineEndpoint_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("engine: [unterminated"), 0o644)

	result := GetCLIEngineEndpoint(cfgPath)
	if result != "" {
		t.Errorf("expected empty string for invalid config, got %q", result)
	}
}

func TestSaveConfig_WriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test write error as root")
	}

	tmpDir := t.TempDir()
	// Make directory read-only so WriteFile fails
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	os.Chmod(readOnlyDir, 0o555)
	t.Cleanup(func() { os.Chmod(readOnlyDir, 0o755) })

	cfgPath := filepath.Join(readOnlyDir, "banyan.yaml")
	cfg := BanyanConfig{Engine: EngineConfig{PasswordHash: "testhash"}}
	err := SaveConfig(cfgPath, &cfg)
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestSaveConfig_MkdirAllError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test mkdir error as root")
	}

	tmpDir := t.TempDir()
	// Create a file where a directory is expected so MkdirAll fails
	blockingFile := filepath.Join(tmpDir, "blocker")
	os.WriteFile(blockingFile, []byte("x"), 0o644)

	cfgPath := filepath.Join(blockingFile, "subdir", "banyan.yaml")
	cfg := BanyanConfig{Engine: EngineConfig{PasswordHash: "testhash"}}
	err := SaveConfig(cfgPath, &cfg)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestGetAgentAuthToken(t *testing.T) {
	t.Run("returns token from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{Agent: AgentConfig{AuthToken: "agent-secret-token"}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		got := GetAgentAuthToken(cfgPath)
		if got != "agent-secret-token" {
			t.Errorf("expected 'agent-secret-token', got %q", got)
		}
	})

	t.Run("missing config returns empty", func(t *testing.T) {
		got := GetAgentAuthToken("/tmp/nonexistent-banyan-test-config.yaml")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("invalid YAML returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "bad.yaml")
		os.WriteFile(cfgPath, []byte("agent: [unterminated"), 0o644)

		got := GetAgentAuthToken(cfgPath)
		if got != "" {
			t.Errorf("expected empty string for invalid config, got %q", got)
		}
	})

	t.Run("empty agent config returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		got := GetAgentAuthToken(cfgPath)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestGetCLIAuthToken(t *testing.T) {
	t.Run("returns token from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{CLI: CLIConfig{AuthToken: "cli-secret-token"}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		got := GetCLIAuthToken(cfgPath)
		if got != "cli-secret-token" {
			t.Errorf("expected 'cli-secret-token', got %q", got)
		}
	})

	t.Run("missing config returns empty", func(t *testing.T) {
		got := GetCLIAuthToken("/tmp/nonexistent-banyan-test-config.yaml")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("invalid YAML returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "bad.yaml")
		os.WriteFile(cfgPath, []byte("cli: [unterminated"), 0o644)

		got := GetCLIAuthToken(cfgPath)
		if got != "" {
			t.Errorf("expected empty string for invalid config, got %q", got)
		}
	})

	t.Run("empty cli config returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		got := GetCLIAuthToken(cfgPath)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}
