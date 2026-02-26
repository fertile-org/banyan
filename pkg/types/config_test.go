package types

import (
	"fmt"
	"net"
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

	t.Run("avoids engine IP 10.200.0.1", func(t *testing.T) {
		engineIP := net.ParseIP(ControlTunnelEngineIP).To4()
		// Test with many keys to increase collision chance
		for i := range 1000 {
			key := fmt.Sprintf("test-key-%d", i)
			ip := TunnelIPFromPublicKey(key).To4()
			if ip.Equal(engineIP) {
				t.Errorf("key %q produced engine IP %s", key, ip)
			}
			// Also check network address 10.200.0.0
			if ip[0] == 10 && ip[1] == 200 && ip[2] == 0 && ip[3] == 0 {
				t.Errorf("key %q produced network address %s", key, ip)
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
				NodeName:   "worker-1",
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
		if loaded.Agent.NodeName != "worker-1" {
			t.Errorf("expected agent node_name=worker-1, got %s", loaded.Agent.NodeName)
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
