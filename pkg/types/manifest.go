package types

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

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
