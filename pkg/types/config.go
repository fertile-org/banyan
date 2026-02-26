package types

import (
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Control tunnel constants.
const (
	ControlTunnelCIDR     = "10.200.0.0/16"
	ControlTunnelEngineIP = "10.200.0.1"
	ControlTunnelPort     = 51821

	// Per-component interface names (max 15 chars for Linux IFNAMSIZ).
	ControlIfaceEngine = "wg-ctl-eng"
	ControlIfaceAgent  = "wg-ctl-agt"
	ControlIfaceCLI    = "wg-ctl-cli"
)

// DefaultConfigPath is the default path to the Banyan config file.
const DefaultConfigPath = "/etc/banyan/banyan.yaml"

// DefaultWhitelistedKeysDir is the default directory for whitelisted agent public keys.
const DefaultWhitelistedKeysDir = "/etc/banyan/whitelisted-keys"

// DefaultKeysDir is the default directory for private key files.
const DefaultKeysDir = "/etc/banyan/keys"

// BanyanConfig is the top-level configuration structure.
type BanyanConfig struct {
	Agent  AgentConfig  `yaml:"agent,omitempty"`
	CLI    CLIConfig    `yaml:"cli,omitempty"`
	Engine EngineConfig `yaml:"engine,omitempty"`
}

// EngineConfig holds engine-specific settings.
type EngineConfig struct {
	APIPort            string `yaml:"api_port,omitempty"`
	GRPCPort           string `yaml:"grpc_port,omitempty"`
	StoreBackend       string `yaml:"store_backend,omitempty"`
	StoreAddress       string `yaml:"store_address,omitempty"`
	EtcdUsername       string `yaml:"etcd_username,omitempty"`
	EtcdPassword       string `yaml:"etcd_password,omitempty"`
	EtcdCertFile       string `yaml:"etcd_cert_file,omitempty"`
	EtcdKeyFile        string `yaml:"etcd_key_file,omitempty"`
	EtcdCAFile         string `yaml:"etcd_ca_file,omitempty"`
	WhitelistedKeysDir string `yaml:"whitelisted_keys_dir,omitempty"`
	OverlayType        string `yaml:"overlay_type,omitempty"` // "wireguard" (default) or "vxlan"
	WGPrivateKeyFile   string `yaml:"wg_private_key_file,omitempty"`
	WGPublicKey        string `yaml:"wg_public_key,omitempty"`
	ManagedEtcd        bool   `yaml:"managed_etcd,omitempty"`
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
	EngineHost        string   `yaml:"engine_host,omitempty"`
	EnginePort        string   `yaml:"engine_port,omitempty"`
	NodeName          string   `yaml:"node_name,omitempty"`
	WGPrivateKeyFile  string   `yaml:"wg_private_key_file,omitempty"`
	WGPublicKey       string   `yaml:"wg_public_key,omitempty"`
	EngineWGPublicKey string   `yaml:"engine_wg_public_key,omitempty"`
	Tags              []string `yaml:"tags,omitempty"`
}

// CLIConfig holds CLI-specific settings.
type CLIConfig struct {
	EngineHost        string `yaml:"engine_host,omitempty"`
	EnginePort        string `yaml:"engine_port,omitempty"`
	Name              string `yaml:"name,omitempty"`
	WGPrivateKeyFile  string `yaml:"wg_private_key_file,omitempty"`
	WGPublicKey       string `yaml:"wg_public_key,omitempty"`
	EngineWGPublicKey string `yaml:"engine_wg_public_key,omitempty"`
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

// TunnelIPFromPublicKey derives a deterministic tunnel IP in 10.200.0.0/16
// from a base64-encoded WireGuard public key. The engine always uses
// 10.200.0.1; agents and CLI clients get IPs derived from their public key hash.
func TunnelIPFromPublicKey(pubKeyB64 string) net.IP {
	h := sha256.Sum256([]byte(pubKeyB64))
	o3, o4 := h[0], h[1]
	// Avoid 10.200.0.0 (network) and 10.200.0.1 (engine)
	if o3 == 0 && o4 <= 1 {
		o4 += 2
	}
	return net.IPv4(10, 200, o3, o4)
}

// WritePrivateKeyFile writes a private key to a file in the given directory
// with 0600 permissions. Returns the full path to the key file.
func WritePrivateKeyFile(dir, name, key string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create keys directory: %w", err)
	}
	path := filepath.Join(dir, name+".key")
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return "", fmt.Errorf("failed to write private key file: %w", err)
	}
	return path, nil
}

// ReadPrivateKeyFile reads a private key from a file and returns the trimmed content.
func ReadPrivateKeyFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read private key file: %w", err)
	}
	key := strings.TrimSpace(string(data))
	if key == "" {
		return "", fmt.Errorf("private key file is empty: %s", path)
	}
	return key, nil
}

// LoadWhitelistedKeys reads all *.pub files from a directory and returns
// a map of publicKey → agentName (filename without extension).
func LoadWhitelistedKeys(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read whitelisted keys dir: %w", err)
	}

	keys := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		pubKey := strings.TrimSpace(string(data))
		if pubKey == "" {
			continue
		}
		agentName := strings.TrimSuffix(entry.Name(), ".pub")
		keys[pubKey] = agentName
	}

	return keys, nil
}
