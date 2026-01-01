// Package domain defines the core entities for the Compose Parser.
package domain

// ParsedCompose represents a parsed docker-compose.yaml.
type ParsedCompose struct {
	Version  string
	Services map[string]ServiceConfig
	Networks map[string]NetworkConfig
	Volumes  map[string]VolumeConfig
	Configs  map[string]ConfigConfig
	Secrets  map[string]SecretConfig
}

// ServiceConfig represents a service definition from compose file.
type ServiceConfig struct {
	Name        string
	Image       string
	Build       *BuildConfig
	Command     []string
	Entrypoint  []string
	Environment map[string]string
	EnvFile     []string
	Ports       []PortConfig
	Volumes     []VolumeMount
	Networks    map[string]ServiceNetworkConfig
	DependsOn   map[string]DependsOnConfig
	Deploy      *DeployConfig
	HealthCheck *HealthCheckConfig
	Restart     string
	Labels      map[string]string
	Logging     *LoggingConfig
	Ulimits     map[string]UlimitConfig
	Sysctls     map[string]string
	CapAdd      []string
	CapDrop     []string
	Privileged  bool
	ReadOnly    bool
	User        string
	WorkingDir  string
	Hostname    string
	DomainName  string
	ExtraHosts  []string
	DNS         []string
	DNSSearch   []string
}

// NetworkConfig represents a network definition.
type NetworkConfig struct {
	Name       string
	Driver     string
	DriverOpts map[string]string
	IPAM       *IPAMConfig
	External   bool
	Internal   bool
	Labels     map[string]string
}

// VolumeConfig represents a volume definition.
type VolumeConfig struct {
	Name       string
	Driver     string
	DriverOpts map[string]string
	External   bool
	Labels     map[string]string
}

// ConfigConfig represents a config definition.
type ConfigConfig struct {
	Name     string
	File     string
	External bool
}

// SecretConfig represents a secret definition.
type SecretConfig struct {
	Name     string
	File     string
	External bool
}

// LoggingConfig represents logging configuration.
type LoggingConfig struct {
	Driver  string
	Options map[string]string
}

// UlimitConfig represents ulimit configuration.
type UlimitConfig struct {
	Soft int64
	Hard int64
}
