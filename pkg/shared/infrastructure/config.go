package infrastructure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fertile-org/banyan/pkg/shared/domain"
)

// ConfigLoader defines the interface for loading configuration.
type ConfigLoader interface {
	// Load loads configuration from the default locations.
	Load(cfg any) error
	// LoadFile loads configuration from a specific file.
	LoadFile(path string, cfg any) error
	// LoadEnv loads configuration from environment variables.
	LoadEnv(cfg any, prefix string) error
}

// configLoader implements ConfigLoader.
type configLoader struct {
	logger Logger
}

// NewConfigLoader creates a new configuration loader.
func NewConfigLoader(logger Logger) ConfigLoader {
	return &configLoader{logger: logger}
}

// Load loads configuration from default locations.
func (c *configLoader) Load(cfg any) error {
	// Try default config paths in order
	paths := []string{
		"banyan.yaml",
		"config/banyan.yaml",
		"/etc/banyan/banyan.yaml",
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			c.logger.Info("loading configuration", F("path", path))
			return c.LoadFile(path, cfg)
		}
	}

	c.logger.Warn("no configuration file found, using defaults")
	return nil
}

// LoadFile loads configuration from a specific file.
func (c *configLoader) LoadFile(path string, cfg any) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in the content
	expanded := os.ExpandEnv(string(data))

	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// LoadEnv loads configuration from environment variables with a prefix.
// This is a simple implementation that should be extended for production use.
func (c *configLoader) LoadEnv(cfg any, prefix string) error {
	// This is a placeholder - in production you'd use a library like envconfig
	// or implement reflection-based environment variable mapping
	c.logger.Debug("loading configuration from environment", F("prefix", prefix))
	return nil
}

// EngineConfig represents the Engine configuration.
type EngineConfig struct {
	Logging  LoggingConfig  `yaml:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics"`
	Etcd     EtcdConfig     `yaml:"etcd"`
	Plugin   PluginConfig   `yaml:"plugin"`
	Server   ServerConfig   `yaml:"server"`
	Registry RegistryConfig `yaml:"registry"`
	State    StateConfig    `yaml:"state"`
}

// AgentConfig represents the Agent configuration.
type AgentConfig struct {
	Container ContainerConfig  `yaml:"container"`
	Network   NetworkConfig    `yaml:"network"`
	Logging   LoggingConfig    `yaml:"logging"`
	Metrics   MetricsConfig    `yaml:"metrics"`
	Engine    EngineConnection `yaml:"engine"`
	Server    ServerConfig     `yaml:"server"`
	Health    HealthConfig     `yaml:"health"`
}

// ServerConfig configures the gRPC server.
type ServerConfig struct {
	Host              string          `yaml:"host"`
	TLS               TLSConfig       `yaml:"tls"`
	Port              int             `yaml:"port"`
	MaxConnections    int             `yaml:"max_connections"`
	ConnectionTimeout domain.Duration `yaml:"connection_timeout"`
}

// DefaultServerConfig returns default server configuration.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Host:              "0.0.0.0",
		Port:              8080,
		MaxConnections:    1000,
		ConnectionTimeout: domain.Duration(30 * time.Second),
	}
}

// Address returns the server address.
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// EtcdConfig configures etcd connection.
type EtcdConfig struct {
	Username    string          `yaml:"username"`
	Password    string          `yaml:"password"`
	TLS         TLSConfig       `yaml:"tls"`
	Endpoints   []string        `yaml:"endpoints"`
	DialTimeout domain.Duration `yaml:"dial_timeout"`
}

// DefaultEtcdConfig returns default etcd configuration.
func DefaultEtcdConfig() EtcdConfig {
	return EtcdConfig{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: domain.Duration(5 * time.Second),
	}
}

// TLSConfig configures TLS settings.
type TLSConfig struct {
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
	CAFile     string `yaml:"ca_file"`
	Enabled    bool   `yaml:"enabled"`
	SkipVerify bool   `yaml:"skip_verify"`
}

// RegistryConfig configures the agent registry.
type RegistryConfig struct {
	HeartbeatInterval domain.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeout  domain.Duration `yaml:"heartbeat_timeout"`
	CleanupInterval   domain.Duration `yaml:"cleanup_interval"`
}

// DefaultRegistryConfig returns default registry configuration.
func DefaultRegistryConfig() RegistryConfig {
	return RegistryConfig{
		HeartbeatInterval: domain.Duration(10 * time.Second),
		HeartbeatTimeout:  domain.Duration(30 * time.Second),
		CleanupInterval:   domain.Duration(60 * time.Second),
	}
}

// StateConfig configures the state manager.
type StateConfig struct {
	ReconcileInterval  domain.Duration `yaml:"reconcile_interval"`
	DriftCheckInterval domain.Duration `yaml:"drift_check_interval"`
}

// DefaultStateConfig returns default state configuration.
func DefaultStateConfig() StateConfig {
	return StateConfig{
		ReconcileInterval:  domain.Duration(30 * time.Second),
		DriftCheckInterval: domain.Duration(60 * time.Second),
	}
}

// PluginConfig configures the plugin manager.
type PluginConfig struct {
	Directory        string          `yaml:"directory"`
	ExecutionTimeout domain.Duration `yaml:"execution_timeout"`
	MaxConcurrent    int             `yaml:"max_concurrent"`
}

// DefaultPluginConfig returns default plugin configuration.
func DefaultPluginConfig() PluginConfig {
	return PluginConfig{
		Directory:        "/etc/banyan/plugins",
		ExecutionTimeout: domain.Duration(30 * time.Second),
		MaxConcurrent:    10,
	}
}

// EngineConnection configures connection to the Engine.
type EngineConnection struct {
	Address           string          `yaml:"address"`
	TLS               TLSConfig       `yaml:"tls"`
	ReconnectInterval domain.Duration `yaml:"reconnect_interval"`
}

// ContainerConfig configures the container runtime.
type ContainerConfig struct {
	Runtime      string `yaml:"runtime"`
	SocketPath   string `yaml:"socket_path"`
	Namespace    string `yaml:"namespace"`
	DefaultImage string `yaml:"default_image"`
}

// DefaultContainerConfig returns default container configuration.
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		Runtime:    "containerd",
		SocketPath: "/run/containerd/containerd.sock",
		Namespace:  "banyan",
	}
}

// NetworkConfig configures networking.
type NetworkConfig struct {
	CNIConfigDir string `yaml:"cni_config_dir"`
	CNIBinDir    string `yaml:"cni_bin_dir"`
	DefaultCIDR  string `yaml:"default_cidr"`
}

// DefaultNetworkConfig returns default network configuration.
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		CNIConfigDir: "/etc/cni/net.d",
		CNIBinDir:    "/opt/cni/bin",
		DefaultCIDR:  "10.244.0.0/16",
	}
}

// HealthConfig configures health monitoring.
type HealthConfig struct {
	CheckInterval    domain.Duration `yaml:"check_interval"`
	FailureThreshold int             `yaml:"failure_threshold"`
	SuccessThreshold int             `yaml:"success_threshold"`
	ProbeTimeout     domain.Duration `yaml:"probe_timeout"`
}

// DefaultHealthConfig returns default health configuration.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		CheckInterval:    domain.Duration(10 * time.Second),
		FailureThreshold: 3,
		SuccessThreshold: 1,
		ProbeTimeout:     domain.Duration(5 * time.Second),
	}
}

// LoggingConfig configures logging.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// DefaultLoggingConfig returns default logging configuration.
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
	}
}

// MetricsConfig configures metrics collection.
type MetricsConfig struct {
	Address string `yaml:"address"`
	Path    string `yaml:"path"`
	Enabled bool   `yaml:"enabled"`
}

// DefaultMetricsConfig returns default metrics configuration.
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled: true,
		Address: ":9090",
		Path:    "/metrics",
	}
}

// GetEnvOrDefault returns environment variable or default value.
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvIntOrDefault returns environment variable as int or default value.
func GetEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvBoolOrDefault returns environment variable as bool or default value.
func GetEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.EqualFold(value, "true") || value == "1"
	}
	return defaultValue
}

// GetEnvDurationOrDefault returns environment variable as duration or default.
func GetEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
