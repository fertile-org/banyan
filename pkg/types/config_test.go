package types

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// mockStateStore is an in-memory StateStore for testing.
type mockStateStore struct {
	data map[string]any
}

func (m *mockStateStore) Save(ctx context.Context, key string, value any) error {
	m.data[key] = value
	return nil
}

func (m *mockStateStore) Get(ctx context.Context, key string, dest any) error {
	v, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func (m *mockStateStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	for k := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

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
			Security: SecurityConfig{
				AuthType: "password",
				Password: "my-cluster-secret",
			},
			Agent: AgentConfig{
				EngineHost: "192.168.1.10",
				EnginePort: "2379",
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

		if loaded.Security.AuthType != "password" {
			t.Errorf("expected auth_type=password, got %s", loaded.Security.AuthType)
		}
		if loaded.Security.Password != "my-cluster-secret" {
			t.Errorf("expected password=my-cluster-secret, got %s", loaded.Security.Password)
		}
		if loaded.Agent.EngineHost != "192.168.1.10" {
			t.Errorf("expected engine_host=192.168.1.10, got %s", loaded.Agent.EngineHost)
		}
		if loaded.Agent.EnginePort != "2379" {
			t.Errorf("expected engine_port=2379, got %s", loaded.Agent.EnginePort)
		}
	})

	t.Run("missing file returns zero value", func(t *testing.T) {
		cfg, err := LoadConfig("/tmp/nonexistent-banyan-test-config.yaml")
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}
		if cfg.Security.Password != "" {
			t.Errorf("expected empty password, got %s", cfg.Security.Password)
		}
	})

	t.Run("round-trip with CLI config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")

		cfg := BanyanConfig{
			CLI: CLIConfig{
				EngineHost: "10.0.0.1",
				EnginePort: "8443",
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

func TestVerifyAuth(t *testing.T) {
	t.Run("valid password matches store hash", func(t *testing.T) {
		store := &mockStateStore{data: map[string]any{}}
		password := "my-secret"
		store.data[KeyAuthHash] = HashPassword(password)

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{Security: SecurityConfig{Password: password}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		err := VerifyAuth(context.Background(), store, cfgPath)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("wrong password fails", func(t *testing.T) {
		store := &mockStateStore{data: map[string]any{}}
		store.data[KeyAuthHash] = HashPassword("correct-password")

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{Security: SecurityConfig{Password: "wrong-password"}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		err := VerifyAuth(context.Background(), store, cfgPath)
		if err == nil {
			t.Error("expected error for wrong password")
		}
	})

	t.Run("missing hash in store fails", func(t *testing.T) {
		store := &mockStateStore{data: map[string]any{}}

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{Security: SecurityConfig{Password: "any-password"}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		err := VerifyAuth(context.Background(), store, cfgPath)
		if err == nil {
			t.Error("expected error when hash not in store")
		}
	})

	t.Run("missing password in config fails", func(t *testing.T) {
		store := &mockStateStore{data: map[string]any{}}

		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		err := VerifyAuth(context.Background(), store, cfgPath)
		if err == nil {
			t.Error("expected error when no password in config")
		}
	})
}

func TestGetConfigPassword(t *testing.T) {
	t.Run("returns password from config", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, "banyan.yaml")
		cfg := BanyanConfig{Security: SecurityConfig{Password: "my-secret"}}
		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		got := GetConfigPassword(cfgPath)
		if got != "my-secret" {
			t.Errorf("expected 'my-secret', got %q", got)
		}
	})

	t.Run("missing config returns empty", func(t *testing.T) {
		got := GetConfigPassword("/tmp/nonexistent-banyan-test-config.yaml")
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	// Use content that fails to unmarshal into BanyanConfig struct
	os.WriteFile(cfgPath, []byte("security: [unterminated"), 0o644)

	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_UnreadableFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "unreadable.yaml")
	os.WriteFile(cfgPath, []byte("security:\n  password: secret\n"), 0o644)

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

	cfg := BanyanConfig{Security: SecurityConfig{Password: "test"}}
	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig should create nested dirs: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Security.Password != "test" {
		t.Errorf("expected password 'test', got %q", loaded.Security.Password)
	}
}

func TestGetConfigEngineEndpoint_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("security: [unterminated"), 0o644)

	result := GetConfigEngineEndpoint(cfgPath)
	if result != "" {
		t.Errorf("expected empty string for invalid config, got %q", result)
	}
}

func TestGetCLIEngineEndpoint_InvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("security: [unterminated"), 0o644)

	result := GetCLIEngineEndpoint(cfgPath)
	if result != "" {
		t.Errorf("expected empty string for invalid config, got %q", result)
	}
}

func TestVerifyAuth_InvalidConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("security: [unterminated"), 0o644)

	store := &mockStateStore{data: map[string]any{}}
	err := VerifyAuth(context.Background(), store, cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid config")
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
	cfg := BanyanConfig{Security: SecurityConfig{Password: "test"}}
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
	cfg := BanyanConfig{Security: SecurityConfig{Password: "test"}}
	err := SaveConfig(cfgPath, &cfg)
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestGetConfigPassword_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.yaml")
	os.WriteFile(cfgPath, []byte("security: [unterminated"), 0o644)

	got := GetConfigPassword(cfgPath)
	if got != "" {
		t.Errorf("expected empty string for invalid config, got %q", got)
	}
}

func TestBasicAuthMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	t.Run("rejects request with no auth", func(t *testing.T) {
		handler := BasicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("rejects wrong password", func(t *testing.T) {
		handler := BasicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("banyan", "wrong")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("accepts correct password", func(t *testing.T) {
		handler := BasicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("banyan", "secret")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("accepts any username with correct password", func(t *testing.T) {
		handler := BasicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("anyuser", "secret")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}
