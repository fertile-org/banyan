package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveAgentEngines(t *testing.T) {
	t.Run("uses engines list when set", func(t *testing.T) {
		cfg := AgentConfig{
			Engines: []EngineEndpoint{
				{Address: "10.0.0.1:50051", WGPublicKey: "key1"},
				{Address: "10.0.0.2:50051", WGPublicKey: "key2"},
			},
			EngineHost:        "ignored",
			EngineWGPublicKey: "ignored",
		}
		engines, primary := ResolveAgentEngines(&cfg)
		if len(engines) != 2 {
			t.Fatalf("expected 2 engines, got %d", len(engines))
		}
		if primary != "10.0.0.1:50051" {
			t.Errorf("expected primary 10.0.0.1:50051, got %s", primary)
		}
	})

	t.Run("falls back to single-engine fields", func(t *testing.T) {
		cfg := AgentConfig{
			EngineHost:        "engine.local",
			EnginePort:        "9999",
			EngineWGPublicKey: "mykey",
		}
		engines, primary := ResolveAgentEngines(&cfg)
		if len(engines) != 1 {
			t.Fatalf("expected 1 engine, got %d", len(engines))
		}
		if engines[0].Address != "engine.local:9999" {
			t.Errorf("expected engine.local:9999, got %s", engines[0].Address)
		}
		if engines[0].WGPublicKey != "mykey" {
			t.Errorf("expected WG key 'mykey', got %s", engines[0].WGPublicKey)
		}
		if primary != "engine.local:9999" {
			t.Errorf("expected primary engine.local:9999, got %s", primary)
		}
	})

	t.Run("default port when not set", func(t *testing.T) {
		cfg := AgentConfig{
			EngineHost:        "engine.local",
			EngineWGPublicKey: "key",
		}
		engines, _ := ResolveAgentEngines(&cfg)
		if len(engines) != 1 {
			t.Fatalf("expected 1 engine, got %d", len(engines))
		}
		if engines[0].Address != "engine.local:50051" {
			t.Errorf("expected default port 50051, got %s", engines[0].Address)
		}
	})

	t.Run("returns nil when no config", func(t *testing.T) {
		cfg := AgentConfig{}
		engines, primary := ResolveAgentEngines(&cfg)
		if engines != nil {
			t.Errorf("expected nil engines, got %v", engines)
		}
		if primary != "" {
			t.Errorf("expected empty primary, got %s", primary)
		}
	})
}

func TestResolveCLIEngines(t *testing.T) {
	t.Run("uses engines list when set", func(t *testing.T) {
		cfg := CLIConfig{
			Engines: []EngineEndpoint{
				{Address: "10.0.0.1:50051", WGPublicKey: "key1"},
			},
		}
		engines := ResolveCLIEngines(&cfg)
		if len(engines) != 1 {
			t.Fatalf("expected 1 engine, got %d", len(engines))
		}
	})

	t.Run("falls back to old fields", func(t *testing.T) {
		cfg := CLIConfig{
			EngineHost:        "myhost",
			EnginePort:        "8080",
			EngineWGPublicKey: "wgkey",
		}
		engines := ResolveCLIEngines(&cfg)
		if len(engines) != 1 {
			t.Fatalf("expected 1 engine, got %d", len(engines))
		}
		if engines[0].Address != "myhost:8080" {
			t.Errorf("expected myhost:8080, got %s", engines[0].Address)
		}
	})

	t.Run("default host and port", func(t *testing.T) {
		cfg := CLIConfig{
			EngineWGPublicKey: "key",
		}
		engines := ResolveCLIEngines(&cfg)
		if len(engines) != 1 {
			t.Fatalf("expected 1 engine, got %d", len(engines))
		}
		if engines[0].Address != "localhost:50051" {
			t.Errorf("expected localhost:50051, got %s", engines[0].Address)
		}
	})

	t.Run("returns nil when no config", func(t *testing.T) {
		cfg := CLIConfig{}
		engines := ResolveCLIEngines(&cfg)
		if engines != nil {
			t.Errorf("expected nil, got %v", engines)
		}
	})
}

func TestBuildGRPCEndpoints(t *testing.T) {
	engines := []EngineEndpoint{
		{Address: "10.0.0.1:50051", WGPublicKey: "key1"},
		{Address: "10.0.0.2:50051", WGPublicKey: "key2"},
	}

	t.Run("tunnel active — derives tunnel IPs", func(t *testing.T) {
		endpoints := BuildGRPCEndpoints(engines, true)
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
		}
		// Tunnel IPs are derived from WG keys, should be 10.200.x.x
		for _, ep := range endpoints {
			if !strings.HasPrefix(ep, "10.200.") {
				t.Errorf("expected tunnel IP 10.200.x.x, got %s", ep)
			}
			if !strings.HasSuffix(ep, ":50051") {
				t.Errorf("expected port 50051, got %s", ep)
			}
		}
		// Two different keys should produce different tunnel IPs
		if endpoints[0] == endpoints[1] {
			t.Errorf("expected different tunnel IPs for different keys")
		}
	})

	t.Run("tunnel not active — uses raw addresses", func(t *testing.T) {
		endpoints := BuildGRPCEndpoints(engines, false)
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
		}
		if endpoints[0] != "10.0.0.1:50051" {
			t.Errorf("expected raw address, got %s", endpoints[0])
		}
		if endpoints[1] != "10.0.0.2:50051" {
			t.Errorf("expected raw address, got %s", endpoints[1])
		}
	})

	t.Run("empty list", func(t *testing.T) {
		endpoints := BuildGRPCEndpoints(nil, true)
		if len(endpoints) != 0 {
			t.Errorf("expected empty, got %v", endpoints)
		}
	})

	t.Run("skips invalid addresses when tunnel active", func(t *testing.T) {
		bad := []EngineEndpoint{
			{Address: "no-port", WGPublicKey: "key"},
			{Address: "good:50051", WGPublicKey: "key2"},
		}
		endpoints := BuildGRPCEndpoints(bad, true)
		if len(endpoints) != 1 {
			t.Fatalf("expected 1 (invalid skipped), got %d", len(endpoints))
		}
	})
}

func TestValidateEngineStartConfig(t *testing.T) {
	t.Run("passes with whitelisted keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "agent.pub"), []byte("key\n"), 0o600)

		cfg := EngineConfig{WhitelistedKeysDir: tmpDir}
		err := ValidateEngineStartConfig(&cfg, false)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})

	t.Run("fails without whitelisted keys", func(t *testing.T) {
		tmpDir := t.TempDir() // empty dir

		cfg := EngineConfig{WhitelistedKeysDir: tmpDir}
		err := ValidateEngineStartConfig(&cfg, false)
		if err == nil {
			t.Fatal("expected error for no whitelisted keys")
		}
		if !strings.Contains(err.Error(), "no whitelisted client keys") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("passes with allow-insecure", func(t *testing.T) {
		cfg := EngineConfig{} // no keys dir
		err := ValidateEngineStartConfig(&cfg, true)
		if err != nil {
			t.Errorf("expected no error with allow-insecure, got: %v", err)
		}
	})

	t.Run("multi-engine requires external etcd", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "agent.pub"), []byte("key\n"), 0o600)

		cfg := EngineConfig{
			WhitelistedKeysDir: tmpDir,
			MultiEngine:        true,
			ManagedEtcd:        true,
		}
		err := ValidateEngineStartConfig(&cfg, false)
		if err == nil {
			t.Fatal("expected error for multi-engine with managed etcd")
		}
		if !strings.Contains(err.Error(), "external etcd") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("multi-engine requires external registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "agent.pub"), []byte("key\n"), 0o600)

		cfg := EngineConfig{
			WhitelistedKeysDir: tmpDir,
			MultiEngine:        true,
			ManagedEtcd:        false,
			ManagedRegistry:    true,
		}
		err := ValidateEngineStartConfig(&cfg, false)
		if err == nil {
			t.Fatal("expected error for multi-engine with managed registry")
		}
		if !strings.Contains(err.Error(), "external registry") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("multi-engine passes with external etcd and registry", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "agent.pub"), []byte("key\n"), 0o600)

		cfg := EngineConfig{
			WhitelistedKeysDir:  tmpDir,
			MultiEngine:         true,
			ManagedEtcd:         false,
			ManagedRegistry:     false,
			ExternalRegistryURL: "registry.internal:5000",
		}
		err := ValidateEngineStartConfig(&cfg, false)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	})
}
