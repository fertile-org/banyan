package types

import (
	"net/http"
	"net/http/httptest"
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
				StoreBackend: "redis",
				StoreAddress: "localhost:6379",
			},
		}

		if err := SaveConfig(cfgPath, &cfg); err != nil {
			t.Fatalf("SaveConfig failed: %v", err)
		}

		loaded, err := LoadConfig(cfgPath)
		if err != nil {
			t.Fatalf("LoadConfig failed: %v", err)
		}

		if loaded.Engine.StoreBackend != "redis" {
			t.Errorf("expected store_backend=redis, got %s", loaded.Engine.StoreBackend)
		}
		if loaded.Engine.StoreAddress != "localhost:6379" {
			t.Errorf("expected store_address=localhost:6379, got %s", loaded.Engine.StoreAddress)
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
	t.Run("defaults to badger when empty", func(t *testing.T) {
		cfg := EngineConfig{}
		if got := cfg.GetStoreBackend(); got != "badger" {
			t.Errorf("expected 'badger', got %q", got)
		}
	})

	t.Run("returns configured backend", func(t *testing.T) {
		cfg := EngineConfig{StoreBackend: "etcd"}
		if got := cfg.GetStoreBackend(); got != "etcd" {
			t.Errorf("expected 'etcd', got %q", got)
		}
	})
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
