package types

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the default path to the Banyan config file.
const DefaultConfigPath = "/etc/banyan/banyan.yaml"

// BanyanConfig is the top-level configuration structure.
type BanyanConfig struct {
	Agent  AgentConfig  `yaml:"agent,omitempty"`
	CLI    CLIConfig    `yaml:"cli,omitempty"`
	Engine EngineConfig `yaml:"engine,omitempty"`
}

// EngineConfig holds engine-specific settings.
type EngineConfig struct {
	PasswordHash string `yaml:"password_hash,omitempty"`
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
	AuthToken  string `yaml:"auth_token,omitempty"`
	NodeName   string `yaml:"node_name,omitempty"`
}

// CLIConfig holds CLI-specific settings.
type CLIConfig struct {
	EngineHost string `yaml:"engine_host,omitempty"`
	EnginePort string `yaml:"engine_port,omitempty"`
	AuthToken  string `yaml:"auth_token,omitempty"`
	Name       string `yaml:"name,omitempty"`
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

// GetAgentAuthToken returns the agent auth token from the config file.
func GetAgentAuthToken(configPath string) string {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ""
	}
	return cfg.Agent.AuthToken
}

// GetCLIAuthToken returns the CLI auth token from the config file.
func GetCLIAuthToken(configPath string) string {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return ""
	}
	return cfg.CLI.AuthToken
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
