package types

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

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the default path to the Banyan config file.
const DefaultConfigPath = "/etc/banyan/banyan.yaml"

// BanyanConfig is the top-level configuration structure.
type BanyanConfig struct {
	Security SecurityConfig `yaml:"security,omitempty"`
	Agent    AgentConfig    `yaml:"agent,omitempty"`
	CLI      CLIConfig      `yaml:"cli,omitempty"`
	Engine   EngineConfig   `yaml:"engine,omitempty"`
}

// SecurityConfig holds authentication settings.
type SecurityConfig struct {
	AuthType string `yaml:"auth_type,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// EngineConfig holds engine-specific settings.
type EngineConfig struct {
	APIPort      string `yaml:"api_port,omitempty"`
	GRPCPort     string `yaml:"grpc_port,omitempty"`
	StoreBackend string `yaml:"store_backend,omitempty"`
	StoreAddress string `yaml:"store_address,omitempty"`
	EtcdUsername string `yaml:"etcd_username,omitempty"`
	EtcdPassword string `yaml:"etcd_password,omitempty"`
	EtcdCertFile string `yaml:"etcd_cert_file,omitempty"`
	EtcdKeyFile  string `yaml:"etcd_key_file,omitempty"`
	EtcdCAFile   string `yaml:"etcd_ca_file,omitempty"`
	ManagedEtcd  bool   `yaml:"managed_etcd,omitempty"`
}

// GetStoreBackend returns the configured store backend, defaulting to "etcd".
func (c *EngineConfig) GetStoreBackend() string {
	if c.StoreBackend == "" {
		return "etcd"
	}
	return c.StoreBackend
}

// AgentConfig holds agent-specific settings.
type AgentConfig struct {
	EngineHost string `yaml:"engine_host,omitempty"`
	EnginePort string `yaml:"engine_port,omitempty"`
}

// CLIConfig holds CLI-specific settings.
type CLIConfig struct {
	EngineHost string `yaml:"engine_host,omitempty"`
	EnginePort string `yaml:"engine_port,omitempty"`
}

// LoadConfig reads and parses the Banyan config file at the given path.
// Returns a zero-value BanyanConfig if the file doesn't exist.
func LoadConfig(path string) (BanyanConfig, error) {
	var cfg BanyanConfig

	data, err := os.ReadFile(path)
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

// SaveConfig writes the Banyan config to disk with 0600 permissions.
func SaveConfig(path string, cfg *BanyanConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// HashPassword returns the SHA-256 hex digest of the given password.
func HashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return fmt.Sprintf("%x", h)
}

// ReadLine reads a single trimmed line from stdin.
func ReadLine() string {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text())
	}
	return ""
}

// BasicAuthMiddleware wraps an HTTP handler with Basic Auth verification.
// Username is ignored; only the password is checked.
func BasicAuthMiddleware(next http.Handler, password string) http.Handler {
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

// GetConfigPassword returns the password from the config file at the given path.
func GetConfigPassword(configPath string) string {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ""
	}
	return cfg.Security.Password
}

// GetConfigEngineEndpoint builds the engine gRPC endpoint from agent config.
// Returns empty string if not configured.
func GetConfigEngineEndpoint(configPath string) string {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ""
	}

	host := cfg.Agent.EngineHost
	if host == "" {
		return ""
	}

	port := cfg.Agent.EnginePort
	if port == "" {
		port = "50051"
	}

	return fmt.Sprintf("%s:%s", host, port)
}

// GetCLIEngineEndpoint builds the engine gRPC endpoint from cli config.
// Returns empty string if not configured.
func GetCLIEngineEndpoint(configPath string) string {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ""
	}

	host := cfg.CLI.EngineHost
	if host == "" {
		return ""
	}

	port := cfg.CLI.EnginePort
	if port == "" {
		port = "50051"
	}

	return fmt.Sprintf("%s:%s", host, port)
}

// StateStore is a minimal interface for etcd operations used by VerifyAuth and helpers.
type StateStore interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Save(ctx context.Context, key string, value interface{}) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// VerifyAuth checks the config password against the hash stored in etcd.
// Returns nil if passwords match or if no auth is configured.
func VerifyAuth(ctx context.Context, store StateStore, configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	password := cfg.Security.Password
	if password == "" {
		return fmt.Errorf("authentication required. Set password in %s", configPath)
	}

	var storedHash string
	err = store.Get(ctx, KeyAuthHash, &storedHash)
	if err != nil {
		return fmt.Errorf("authentication failed: no auth hash found in cluster. Ensure engine has been started with a password")
	}

	localHash := HashPassword(password)
	if subtle.ConstantTimeCompare([]byte(localHash), []byte(storedHash)) != 1 {
		return fmt.Errorf("authentication failed: password does not match cluster")
	}

	return nil
}
