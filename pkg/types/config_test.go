package types

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTunnelIPFromPublicKey(t *testing.T) {
	t.Run("returns IPv4 in 10.200.x.y range", func(t *testing.T) {
		ip := TunnelIPFromPublicKey("dGVzdC1rZXktMQ==")
		ip4 := ip.To4()
		if ip4 == nil {
			t.Fatal("expected IPv4 address")
		}
		if ip4[0] != 10 || ip4[1] != 200 {
			t.Errorf("expected 10.200.x.y, got %s", ip)
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		ip1 := TunnelIPFromPublicKey("dGVzdC1rZXktMQ==")
		ip2 := TunnelIPFromPublicKey("dGVzdC1rZXktMQ==")
		if !ip1.Equal(ip2) {
			t.Errorf("expected identical IPs, got %s and %s", ip1, ip2)
		}
	})

	t.Run("different keys produce different IPs", func(t *testing.T) {
		ip1 := TunnelIPFromPublicKey("a2V5LWFscGhh")
		ip2 := TunnelIPFromPublicKey("a2V5LWJldGE=")
		if ip1.Equal(ip2) {
			t.Errorf("expected different IPs for different keys, both got %s", ip1)
		}
	})

	t.Run("avoids network addresses", func(t *testing.T) {
		// Test with many keys to ensure no network/broadcast addresses
		for i := range 1000 {
			key := fmt.Sprintf("test-key-%d", i)
			ip := TunnelIPFromPublicKey(key).To4()
			// 10.200.0.0 = network address, 10.200.0.1 = reserved low address
			if ip[0] == 10 && ip[1] == 200 && ip[2] == 0 && ip[3] <= 1 {
				t.Errorf("key %q produced reserved address %s", key, ip)
			}
		}
	})
}

func TestLoadSaveConfig(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				GRPCPort: "50051",
			},
			Agent: AgentConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "2379",
				AgentName:  "worker-1",
			},
			CLI: CLIConfig{
				WGPublicKey: "cli-pubkey",
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

		if loaded.Engine.GRPCPort != "50051" {
			t.Errorf("expected grpc_port=50051, got %s", loaded.Engine.GRPCPort)
		}
		if loaded.Agent.EngineHost != "192.168.1.10" {
			t.Errorf("expected engine_host=192.168.1.10, got %s", loaded.Agent.EngineHost)
		}
		if loaded.Agent.EnginePort != "2379" {
			t.Errorf("expected engine_port=2379, got %s", loaded.Agent.EnginePort)
		}
		if loaded.Agent.AgentName != "worker-1" {
			t.Errorf("expected agent agent_name=worker-1, got %s", loaded.Agent.AgentName)
		}
		if loaded.CLI.WGPublicKey != "cli-pubkey" {
			t.Errorf("expected cli wg_public_key=cli-pubkey, got %s", loaded.CLI.WGPublicKey)
		}
	})

	t.Run("missing file returns zero value", func(t *testing.T) {
		cfg, err := LoadConfig("/tmp/nonexistent-banyan-test-config.yaml")
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}
		if cfg.Engine.GRPCPort != "" {
			t.Errorf("expected empty grpc_port, got %s", cfg.Engine.GRPCPort)
		}
		if cfg.Agent.WGPublicKey != "" {
			t.Errorf("expected empty agent wg_public_key, got %s", cfg.Agent.WGPublicKey)
		}
		if cfg.CLI.WGPublicKey != "" {
			t.Errorf("expected empty cli wg_public_key, got %s", cfg.CLI.WGPublicKey)
		}
	})

	t.Run("round-trip with CLI config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			CLI: CLIConfig{
				EngineHost:  "10.0.0.1",
				EnginePort:  "8443",
				WGPublicKey: "cli-key-123",
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
		if loaded.CLI.WGPublicKey != "cli-key-123" {
			t.Errorf("expected cli.wg_public_key=cli-key-123, got %s", loaded.CLI.WGPublicKey)
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

func TestLoadSaveConfig_RegistryFields(t *testing.T) {
	t.Run("round-trip with managed registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				StoreBackend:    "etcd",
				ManagedEtcd:     true,
				ManagedRegistry: true,
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if !loaded.Engine.ManagedRegistry {
			t.Error("expected managed_registry=true")
		}
		if loaded.Engine.ExternalRegistryURL != "" {
			t.Errorf("expected empty external_registry_url, got %s", loaded.Engine.ExternalRegistryURL)
		}
	})

	t.Run("round-trip with external registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				StoreBackend:        "etcd",
				ManagedEtcd:         false,
				StoreAddress:        "http://etcd.example.com:2379",
				ManagedRegistry:     false,
				ExternalRegistryURL: "registry.example.com:5000",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.ManagedRegistry {
			t.Error("expected managed_registry=false")
		}
		if loaded.Engine.ExternalRegistryURL != "registry.example.com:5000" {
			t.Errorf("expected external_registry_url=registry.example.com:5000, got %s", loaded.Engine.ExternalRegistryURL)
		}
	})
}

func TestLoadSaveConfig_EngineIdentityFields(t *testing.T) {
	t.Run("round-trip with engine ID and multi-engine", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			Engine: EngineConfig{
				StoreBackend:        "etcd",
				StoreAddress:        "http://etcd.example.com:2379",
				EngineID:            "prod-web-1-a3f2",
				MultiEngine:         true,
				ManagedEtcd:         false,
				ManagedRegistry:     false,
				ExternalRegistryURL: "registry.example.com:5000",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.EngineID != "prod-web-1-a3f2" {
			t.Errorf("expected engine_id=prod-web-1-a3f2, got %s", loaded.Engine.EngineID)
		}
		if !loaded.Engine.MultiEngine {
			t.Error("expected multi_engine=true")
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
	os.WriteFile(cfgPath, []byte("engine:\n  grpc_port: \"50051\"\n"), 0o644)

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

	cfg := BanyanConfig{Engine: EngineConfig{GRPCPort: "50051"}}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig should create nested dirs: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Engine.GRPCPort != "50051" {
		t.Errorf("expected grpc_port '50051', got %q", loaded.Engine.GRPCPort)
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
	cfg := BanyanConfig{Engine: EngineConfig{GRPCPort: "50051"}}
	err := SaveConfig(cfgPath, &cfg)
	if err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestWritePrivateKeyFile(t *testing.T) {
	t.Run("writes key file with correct permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		keysDir := filepath.Join(tmpDir, "keys")

		path, err := WritePrivateKeyFile(keysDir, "engine", "test-private-key-content")
		if err != nil {
			t.Fatalf("WritePrivateKeyFile failed: %v", err)
		}

		expectedPath := filepath.Join(keysDir, "engine.key")
		if path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, path)
		}

		// Check file permissions
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected 0600 permissions, got %04o", perm)
		}

		// Check directory permissions
		dirInfo, err := os.Stat(keysDir)
		if err != nil {
			t.Fatalf("stat dir failed: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("expected 0700 directory permissions, got %04o", perm)
		}

		// Check content
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(data) != "test-private-key-content" {
			t.Errorf("expected 'test-private-key-content', got %q", string(data))
		}
	})

	t.Run("creates nested directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		keysDir := filepath.Join(tmpDir, "deep", "nested", "keys")

		path, err := WritePrivateKeyFile(keysDir, "agent", "key-data")
		if err != nil {
			t.Fatalf("WritePrivateKeyFile failed: %v", err)
		}

		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("key file should exist at %s: %v", path, statErr)
		}
	})
}

func TestReadPrivateKeyFile(t *testing.T) {
	t.Run("reads and trims key", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "test.key")
		os.WriteFile(keyPath, []byte("  my-secret-key  \n"), 0o600)

		key, err := ReadPrivateKeyFile(keyPath)
		if err != nil {
			t.Fatalf("ReadPrivateKeyFile failed: %v", err)
		}
		if key != "my-secret-key" {
			t.Errorf("expected 'my-secret-key', got %q", key)
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := ReadPrivateKeyFile("/tmp/nonexistent-key-file-test.key")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("returns error for empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "empty.key")
		os.WriteFile(keyPath, []byte(""), 0o600)

		_, err := ReadPrivateKeyFile(keyPath)
		if err == nil {
			t.Fatal("expected error for empty key file")
		}
	})

	t.Run("returns error for whitespace-only file", func(t *testing.T) {
		tmpDir := t.TempDir()
		keyPath := filepath.Join(tmpDir, "spaces.key")
		os.WriteFile(keyPath, []byte("   \n  \n"), 0o600)

		_, err := ReadPrivateKeyFile(keyPath)
		if err == nil {
			t.Fatal("expected error for whitespace-only key file")
		}
	})
}

func TestWriteReadPrivateKeyFile_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	keysDir := filepath.Join(tmpDir, "keys")

	originalKey := "dGVzdC1wcml2YXRlLWtleQ==" // base64 encoded
	path, err := WritePrivateKeyFile(keysDir, "roundtrip", originalKey)
	if err != nil {
		t.Fatalf("WritePrivateKeyFile failed: %v", err)
	}

	readKey, err := ReadPrivateKeyFile(path)
	if err != nil {
		t.Fatalf("ReadPrivateKeyFile failed: %v", err)
	}
	if readKey != originalKey {
		t.Errorf("round-trip failed: wrote %q, read %q", originalKey, readKey)
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
	cfg := BanyanConfig{Engine: EngineConfig{GRPCPort: "50051"}}
	err := SaveConfig(cfgPath, &cfg)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestGetConfigEngineEndpoint_EnginesList(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")

	cfg := BanyanConfig{
		Agent: AgentConfig{
			Engines: []EngineEndpoint{
				{Address: "10.0.0.1:50051", WGPublicKey: "key1"},
				{Address: "10.0.0.2:50051", WGPublicKey: "key2"},
			},
		},
	}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	result := GetConfigEngineEndpoint(cfgPath)
	if result != "10.0.0.1:50051" {
		t.Errorf("expected first engine address, got %s", result)
	}
}

func TestGetCLIEngineEndpoint_EnginesList(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")

	cfg := BanyanConfig{
		CLI: CLIConfig{
			Engines: []EngineEndpoint{
				{Address: "10.0.0.1:50051", WGPublicKey: "key1"},
				{Address: "10.0.0.2:50051", WGPublicKey: "key2"},
			},
		},
	}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	result := GetCLIEngineEndpoint(cfgPath)
	if result != "10.0.0.1:50051" {
		t.Errorf("expected first engine address, got %s", result)
	}
}

func TestLoadWhitelistedKeys(t *testing.T) {
	t.Run("returns nil for nonexistent directory", func(t *testing.T) {
		keys, err := LoadWhitelistedKeys("/nonexistent/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if keys != nil {
			t.Errorf("expected nil keys, got %v", keys)
		}
	})

	t.Run("loads keys from .pub files", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "worker-1.pub"), []byte("pubkey1\n"), 0o600)
		os.WriteFile(filepath.Join(tmpDir, "worker-2.pub"), []byte("pubkey2\n"), 0o600)
		os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("not a key"), 0o600)

		keys, err := LoadWhitelistedKeys(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(keys))
		}
		if keys["pubkey1"] != "worker-1" {
			t.Errorf("expected pubkey1 -> worker-1, got %s", keys["pubkey1"])
		}
		if keys["pubkey2"] != "worker-2" {
			t.Errorf("expected pubkey2 -> worker-2, got %s", keys["pubkey2"])
		}
	})

	t.Run("skips empty files", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "empty.pub"), []byte(""), 0o600)
		os.WriteFile(filepath.Join(tmpDir, "valid.pub"), []byte("realkey"), 0o600)

		keys, err := LoadWhitelistedKeys(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key (empty skipped), got %d", len(keys))
		}
	})

	t.Run("skips directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "subdir.pub"), 0o700)
		os.WriteFile(filepath.Join(tmpDir, "valid.pub"), []byte("key"), 0o600)

		keys, err := LoadWhitelistedKeys(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("expected 1 key (dir skipped), got %d", len(keys))
		}
	})
}

func TestEngineEndpoint_Serialization(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")

	cfg := BanyanConfig{
		Agent: AgentConfig{
			AgentName: "test",
			Engines: []EngineEndpoint{
				{Address: "10.0.0.1:50051", WGPublicKey: "abc123"},
			},
		},
	}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(loaded.Agent.Engines) != 1 {
		t.Fatalf("expected 1 engine, got %d", len(loaded.Agent.Engines))
	}
	if loaded.Agent.Engines[0].Address != "10.0.0.1:50051" {
		t.Errorf("expected address 10.0.0.1:50051, got %s", loaded.Agent.Engines[0].Address)
	}
	if loaded.Agent.Engines[0].WGPublicKey != "abc123" {
		t.Errorf("expected wg key abc123, got %s", loaded.Agent.Engines[0].WGPublicKey)
	}
}
