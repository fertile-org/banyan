package cmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"gopkg.in/yaml.v3"
)

// configPath is the default path to the Banyan config file.
// Declared as var so tests can override it.
var configPath = "/etc/banyan/banyan.yaml"

// BanyanConfig is the top-level configuration structure.
type BanyanConfig struct {
	Security SecurityConfig `yaml:"security,omitempty"`
	Engine   EngineConfig   `yaml:"engine,omitempty"`
	Agent    AgentConfig    `yaml:"agent,omitempty"`
}

// SecurityConfig holds authentication settings.
type SecurityConfig struct {
	AuthType string `yaml:"auth_type,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// EngineConfig holds engine-specific settings (reserved for future use).
type EngineConfig struct{}

// AgentConfig holds agent-specific settings.
type AgentConfig struct {
	EngineHost string `yaml:"engine_host,omitempty"`
	EnginePort string `yaml:"engine_port,omitempty"`
}

// loadConfig reads and parses the Banyan config file.
// Returns a zero-value BanyanConfig if the file doesn't exist.
func loadConfig() (BanyanConfig, error) {
	var cfg BanyanConfig

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse config: %w", err)
	}

	return cfg, nil
}

// saveConfig writes the Banyan config to disk with 0600 permissions.
func saveConfig(cfg BanyanConfig) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// hashPassword returns the SHA-256 hex digest of the given password.
func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}

// verifyAuth checks the local config password against the hash stored in etcd.
// Returns nil if auth is not configured (backward compatible).
func verifyAuth(ctx context.Context, store *storage.EtcdStore) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	password := cfg.Security.Password

	// No config or no password → require authentication
	if password == "" {
		return fmt.Errorf("authentication required. Set password in %s (run 'banyan-cli engine init' or 'banyan-cli agent init')", configPath)
	}

	// Check if engine has a stored hash
	var storedHash string
	err = store.Get(ctx, keyAuthHash, &storedHash)
	if err != nil {
		// Password configured but no hash in etcd — cluster not set up
		return fmt.Errorf("authentication failed: no auth hash found in cluster. Ensure engine has been started with a password")
	}

	// Compare hashes
	localHash := hashPassword(password)
	if subtle.ConstantTimeCompare([]byte(localHash), []byte(storedHash)) != 1 {
		return fmt.Errorf("authentication failed: password does not match cluster")
	}

	return nil
}

// getConfigPassword returns the password from the config file, or empty string.
func getConfigPassword() string {
	cfg, err := loadConfig()
	if err != nil {
		return ""
	}
	return cfg.Security.Password
}

// getConfigEngineEndpoint builds the engine endpoint URL from agent config.
// Returns empty string if not configured.
func getConfigEngineEndpoint() string {
	cfg, err := loadConfig()
	if err != nil {
		return ""
	}

	host := cfg.Agent.EngineHost
	if host == "" {
		return ""
	}

	port := cfg.Agent.EnginePort
	if port == "" {
		port = "2379"
	}

	return fmt.Sprintf("http://%s:%s", host, port)
}

// readLine reads a single trimmed line from stdin.
func readLine() string {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// basicAuthMiddleware wraps an HTTP handler with Basic Auth verification.
// Username is ignored; only the password is checked.
func basicAuthMiddleware(next http.Handler, password string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="banyan"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
