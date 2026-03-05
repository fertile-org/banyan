package types

import "gopkg.in/yaml.v3"

// BanyanManifest represents the banyan.yaml structure.
type BanyanManifest struct {
	Services map[string]ManifestService `yaml:"services"`
	Networks map[string]ManifestNetwork `yaml:"networks,omitempty"`
	Name     string                     `yaml:"name"`
	Version  string                     `yaml:"version,omitempty"`
}

// ManifestService represents a service in the manifest.
type ManifestService struct {
	Image       string          `yaml:"image"`
	Build       *ManifestBuild  `yaml:"build,omitempty"`
	Deploy      *ManifestDeploy `yaml:"deploy,omitempty"`
	Ports       []string        `yaml:"ports,omitempty"`
	Environment []string        `yaml:"environment,omitempty"`
	EnvFile     EnvFile         `yaml:"env_file,omitempty"`
	Command     []string        `yaml:"command,omitempty"`
	DependsOn   []string        `yaml:"depends_on,omitempty"`
	Restart     string          `yaml:"restart,omitempty"`
	Entrypoint  ShellCommand    `yaml:"entrypoint,omitempty"`
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
