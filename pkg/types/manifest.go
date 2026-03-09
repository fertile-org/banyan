package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// validRestartPolicies are the allowed restart policy values (Docker Compose compatible).
var validRestartPolicies = map[string]bool{
	"":               true, // omitted is fine
	"no":             true,
	"always":         true,
	"unless-stopped": true,
	"on-failure":     true,
}

// MaxReplicas is the upper bound for replica count per service.
const MaxReplicas = 100

// namePattern matches safe infrastructure names: lowercase alphanumeric with
// hyphens and underscores, must start and end with alphanumeric.
// Allows underscores for Docker Compose compatibility (e.g., my_service).
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`)

// ValidateName checks that a name is safe for use in DNS, etcd keys,
// and container names. Returns an error if the name is invalid.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if len(name) > 63 {
		return fmt.Errorf("name %q too long: max 63 characters", name)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name %q is invalid: must be lowercase alphanumeric with hyphens or underscores, starting and ending with alphanumeric", name)
	}
	return nil
}

// ValidatePort checks that a port mapping string is valid (e.g., "80:80", "8080:80").
func ValidatePort(port string) error {
	parts := strings.SplitN(port, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("port %q must be in format host:container", port)
	}
	for _, p := range parts {
		// Strip /tcp, /udp protocol suffix if present
		p = strings.Split(p, "/")[0]
		num, err := strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("port %q: %q is not a valid number", port, p)
		}
		if num < 1 || num > 65535 {
			return fmt.Errorf("port %q: %d is out of range (1-65535)", port, num)
		}
	}
	return nil
}

// ValidateRestartPolicy checks that a restart policy is one of the allowed values.
func ValidateRestartPolicy(policy string) error {
	// Handle on-failure:N format
	base := policy
	if strings.HasPrefix(policy, "on-failure:") {
		parts := strings.SplitN(policy, ":", 2)
		base = parts[0]
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return fmt.Errorf("restart policy %q: retry count must be a number", policy)
		}
	}
	if !validRestartPolicies[base] {
		return fmt.Errorf("restart policy %q is invalid: must be one of: no, always, unless-stopped, on-failure, on-failure:N", policy)
	}
	return nil
}

// ValidateService validates a single service definition in the manifest.
func ValidateService(name string, svc *ManifestService) error {
	if err := ValidateName(name); err != nil {
		return fmt.Errorf("service name: %w", err)
	}
	if svc.Image == "" && svc.Build == nil {
		return fmt.Errorf("service %q must have either 'image' or 'build'", name)
	}
	for _, port := range svc.Ports {
		if err := ValidatePort(port); err != nil {
			return fmt.Errorf("service %q: %w", name, err)
		}
	}
	if svc.Deploy != nil && svc.Deploy.Replicas > MaxReplicas {
		return fmt.Errorf("service %q: replicas %d exceeds maximum (%d)", name, svc.Deploy.Replicas, MaxReplicas)
	}
	if err := ValidateRestartPolicy(svc.Restart); err != nil {
		return fmt.Errorf("service %q: %w", name, err)
	}
	return nil
}

// BanyanManifest represents the banyan.yaml structure.
type BanyanManifest struct {
	Services map[string]ManifestService `yaml:"services"`
	Networks map[string]ManifestNetwork `yaml:"networks,omitempty"`
	Name     string                     `yaml:"name"`
	Version  string                     `yaml:"version,omitempty"`
}

// ManifestService represents a service in the manifest.
type ManifestService struct {
	Image       string               `yaml:"image"`
	Build       *ManifestBuild       `yaml:"build,omitempty"`
	Deploy      *ManifestDeploy      `yaml:"deploy,omitempty"`
	Healthcheck *ManifestHealthcheck `yaml:"healthcheck,omitempty"`
	Ports       []string             `yaml:"ports,omitempty"`
	Environment []string             `yaml:"environment,omitempty"`
	EnvFile     EnvFile              `yaml:"env_file,omitempty"`
	Command     []string             `yaml:"command,omitempty"`
	DependsOn   []string             `yaml:"depends_on,omitempty"`
	Restart     string               `yaml:"restart,omitempty"`
	Entrypoint  ShellCommand         `yaml:"entrypoint,omitempty"`
}

// ManifestHealthcheck represents healthcheck configuration (matches Docker Compose).
type ManifestHealthcheck struct {
	Interval    string       `yaml:"interval,omitempty"`
	Timeout     string       `yaml:"timeout,omitempty"`
	StartPeriod string       `yaml:"start_period,omitempty"`
	Test        ShellCommand `yaml:"test,omitempty"`
	Retries     int          `yaml:"retries,omitempty"`
	Disable     bool         `yaml:"disable,omitempty"`
}

// ManifestDeploy represents deploy configuration (matches Docker Compose).
type ManifestDeploy struct {
	Placement *ManifestPlacement `yaml:"placement,omitempty"`
	Resources *ManifestResources `yaml:"resources,omitempty"`
	Replicas  int                `yaml:"replicas,omitempty"`
}

// ManifestResources represents resource limits and reservations.
type ManifestResources struct {
	Limits       *ResourceSpec `yaml:"limits,omitempty"`
	Reservations *ResourceSpec `yaml:"reservations,omitempty"`
}

// ResourceSpec represents a resource limit or reservation.
type ResourceSpec struct {
	Memory string `yaml:"memory,omitempty"`
	CPUs   string `yaml:"cpus,omitempty"`
}

// ManifestPlacement represents placement constraints for a service.
type ManifestPlacement struct {
	Node string `yaml:"node,omitempty"`
}

// GetReplicas returns the replica count from deploy config, defaulting to 0 (caller handles default).
func (s *ManifestService) GetReplicas() int {
	if s.Deploy != nil {
		return s.Deploy.Replicas
	}
	return 0
}

// ManifestBuild represents build configuration for a service.
// Supports both string form (build: ./path) and object form (build: {context: ./path}).
type ManifestBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

// UnmarshalYAML supports both string and object forms for build config.
func (b *ManifestBuild) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		b.Context = value.Value
		return nil
	}
	type raw ManifestBuild
	return value.Decode((*raw)(b))
}

// ManifestNetwork represents network configuration.
type ManifestNetwork struct {
	CIDR   string `yaml:"cidr,omitempty"`
	Driver string `yaml:"driver,omitempty"`
}

// ShellCommand supports both string and list forms for entrypoint/command.
// String form: entrypoint: "/bin/sh -c 'echo hello'"
// List form:   entrypoint: ["/bin/sh", "-c", "echo hello"]
type ShellCommand []string

// UnmarshalYAML supports both string and sequence forms.
func (s *ShellCommand) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*s = []string{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}
