package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHashPassword(t *testing.T) {
	t.Run("deterministic output", func(t *testing.T) {
		hash1 := hashPassword("my-secret")
		hash2 := hashPassword("my-secret")
		if hash1 != hash2 {
			t.Errorf("expected identical hashes, got %s and %s", hash1, hash2)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := hashPassword("password-a")
		hash2 := hashPassword("password-b")
		if hash1 == hash2 {
			t.Errorf("expected different hashes for different inputs")
		}
	})

	t.Run("returns 64-char hex string", func(t *testing.T) {
		hash := hashPassword("test")
		if len(hash) != 64 {
			t.Errorf("expected 64-char hex string, got %d chars: %s", len(hash), hash)
		}
	})
}

func TestLoadSaveConfig(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		tmpDir := t.TempDir()
		origPath := configPath
		configPath = filepath.Join(tmpDir, "banyan.yaml")
		defer func() { configPath = origPath }()

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

		if err := saveConfig(cfg); err != nil {
			t.Fatalf("saveConfig failed: %v", err)
		}

		// Verify file permissions
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("stat failed: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected 0600 permissions, got %04o", perm)
		}

		loaded, err := loadConfig()
		if err != nil {
			t.Fatalf("loadConfig failed: %v", err)
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
		origPath := configPath
		configPath = "/tmp/nonexistent-banyan-test-config.yaml"
		defer func() { configPath = origPath }()

		cfg, err := loadConfig()
		if err != nil {
			t.Fatalf("expected no error for missing file, got %v", err)
		}
		if cfg.Security.Password != "" {
			t.Errorf("expected empty password, got %s", cfg.Security.Password)
		}
	})
}

func TestGetConfigEngineEndpoint(t *testing.T) {
	t.Run("empty config returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		origPath := configPath
		configPath = filepath.Join(tmpDir, "banyan.yaml")
		defer func() { configPath = origPath }()

		cfg := BanyanConfig{}
		if err := saveConfig(cfg); err != nil {
			t.Fatalf("saveConfig failed: %v", err)
		}

		result := getConfigEngineEndpoint()
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("host and port", func(t *testing.T) {
		tmpDir := t.TempDir()
		origPath := configPath
		configPath = filepath.Join(tmpDir, "banyan.yaml")
		defer func() { configPath = origPath }()

		cfg := BanyanConfig{
			Agent: AgentConfig{
				EngineHost: "10.0.0.1",
				EnginePort: "2380",
			},
		}
		if err := saveConfig(cfg); err != nil {
			t.Fatalf("saveConfig failed: %v", err)
		}

		result := getConfigEngineEndpoint()
		if result != "http://10.0.0.1:2380" {
			t.Errorf("expected http://10.0.0.1:2380, got %s", result)
		}
	})

	t.Run("default port when not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		origPath := configPath
		configPath = filepath.Join(tmpDir, "banyan.yaml")
		defer func() { configPath = origPath }()

		cfg := BanyanConfig{
			Agent: AgentConfig{
				EngineHost: "10.0.0.1",
			},
		}
		if err := saveConfig(cfg); err != nil {
			t.Fatalf("saveConfig failed: %v", err)
		}

		result := getConfigEngineEndpoint()
		if result != "http://10.0.0.1:2379" {
			t.Errorf("expected http://10.0.0.1:2379, got %s", result)
		}
	})
}

func TestBasicAuthMiddleware(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	t.Run("rejects request with no auth", func(t *testing.T) {
		handler := basicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("rejects wrong password", func(t *testing.T) {
		handler := basicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("banyan", "wrong")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("accepts correct password", func(t *testing.T) {
		handler := basicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("banyan", "secret")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("accepts any username with correct password", func(t *testing.T) {
		handler := basicAuthMiddleware(okHandler, "secret")
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("anyuser", "secret")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})
}

func TestKeyAuthHash(t *testing.T) {
	if keyAuthHash != "config/auth_hash" {
		t.Errorf("expected config/auth_hash, got %s", keyAuthHash)
	}
}
