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
	Volumes  map[string]VolumeConfig    `yaml:"volumes,omitempty"`
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
	DependsOn   DependsOnConfig      `yaml:"depends_on,omitempty"`
	Restart     string               `yaml:"restart,omitempty"`
	Entrypoint  ShellCommand         `yaml:"entrypoint,omitempty"`
	Volumes     VolumeMounts         `yaml:"volumes,omitempty"`
}

// VolumeMount represents a service-level volume mount.
// Supports both Docker Compose short syntax (string) and long syntax (struct).
type VolumeMount struct {
	Tmpfs    *TmpfsOpt `yaml:"tmpfs,omitempty" json:"tmpfs,omitempty"`
	Type     string    `yaml:"type,omitempty" json:"type,omitempty"`
	Source   string    `yaml:"source,omitempty" json:"source,omitempty"`
	Target   string    `yaml:"target" json:"target"`
	ReadOnly bool      `yaml:"read_only,omitempty" json:"read_only,omitempty"`
}

// TmpfsOpt configures tmpfs mount options.
type TmpfsOpt struct {
	Size string `yaml:"size,omitempty" json:"size,omitempty"`
}

// VolumeConfig represents a top-level named volume declaration.
type VolumeConfig struct {
	DriverOpts map[string]string `yaml:"driver_opts,omitempty" json:"driver_opts,omitempty"`
	Driver     string            `yaml:"driver,omitempty" json:"driver,omitempty"`
	Name       string            `yaml:"name,omitempty" json:"name,omitempty"`
	External   bool              `yaml:"external,omitempty" json:"external,omitempty"`
}

// VolumeMounts supports both Docker Compose short syntax (strings) and long syntax (structs).
// Short: "source:target[:ro]"   Long: {type: bind, source: ./x, target: /y, read_only: true}
type VolumeMounts []VolumeMount

// UnmarshalYAML parses the volumes list, supporting both string and mapping items.
func (v *VolumeMounts) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.SequenceNode {
		return fmt.Errorf("volumes must be a list")
	}
	var mounts []VolumeMount
	for _, item := range value.Content {
		if item.Kind == yaml.ScalarNode {
			// Short syntax: "source:target[:mode]"
			m, err := parseVolumeShortSyntax(item.Value)
			if err != nil {
				return fmt.Errorf("invalid volume %q: %w", item.Value, err)
			}
			mounts = append(mounts, m)
		} else {
			// Long syntax: {type: ..., source: ..., target: ...}
			var m VolumeMount
			if err := item.Decode(&m); err != nil {
				return fmt.Errorf("invalid volume mapping: %w", err)
			}
			mounts = append(mounts, m)
		}
	}
	*v = mounts
	return nil
}

// ResolveManifestVolumes resolves volume paths and top-level NFS references.
// Relative bind mount paths are resolved against basePath (the manifest directory).
// Named volumes with NFS driver_opts are converted to type "nfs" with the
// NFS source (addr:device) in the Source field.
func ResolveManifestVolumes(_ string, manifest *BanyanManifest) {
	for svcName := range manifest.Services {
		svc := manifest.Services[svcName]
		for i, vol := range svc.Volumes {
			// Relative bind mount paths are NOT resolved on the CLI.
			// They are resolved on the agent against /var/lib/banyan/data/.
			// This ensures paths work in distributed deployments.

			// Resolve named volumes with NFS driver_opts
			if vol.Type == "volume" && len(manifest.Volumes) > 0 {
				vc, ok := manifest.Volumes[vol.Source]
				if ok && vc.DriverOpts["type"] == "nfs" {
					device := vc.DriverOpts["device"]
					opts := vc.DriverOpts["o"]
					addr, otherOpts := parseNFSOpts(opts)
					if addr != "" && device != "" {
						// Device in Docker Compose NFS has format ":/path" — strip leading colon
						device = strings.TrimPrefix(device, ":")
						svc.Volumes[i].Type = "nfs"
						svc.Volumes[i].Source = addr + ":" + device
						if otherOpts != "" {
							svc.Volumes[i].Tmpfs = &TmpfsOpt{Size: otherOpts}
						}
					}
				}
			}
		}
		manifest.Services[svcName] = svc
	}
}

// parseNFSOpts extracts addr and remaining options from an NFS mount options string.
func parseNFSOpts(opts string) (addr, remaining string) {
	var others []string
	for _, opt := range strings.Split(opts, ",") {
		opt = strings.TrimSpace(opt)
		if strings.HasPrefix(opt, "addr=") {
			addr = strings.TrimPrefix(opt, "addr=")
		} else if opt != "" {
			others = append(others, opt)
		}
	}
	return addr, strings.Join(others, ",")
}

// parseVolumeShortSyntax parses "source:target[:mode]" into a VolumeMount.
func parseVolumeShortSyntax(s string) (VolumeMount, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return VolumeMount{}, fmt.Errorf("expected source:target format")
	}

	source := parts[0]
	target := parts[1]
	readOnly := false
	if len(parts) == 3 {
		readOnly = parts[2] == "ro"
	}

	// Determine type from source path
	volType := "volume" // default: named volume
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		volType = "bind"
	}

	return VolumeMount{
		Type:     volType,
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}, nil
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
	Placement       *ManifestPlacement `yaml:"placement,omitempty"`
	Resources       *ManifestResources `yaml:"resources,omitempty"`
	Autoscale       *ManifestAutoscale `yaml:"autoscale,omitempty"`
	StopGracePeriod string             `yaml:"stop_grace_period,omitempty"` // e.g., "10s"
	Replicas        int                `yaml:"replicas,omitempty"`
}

// ManifestAutoscale configures automatic horizontal scaling for a service.
type ManifestAutoscale struct {
	Cooldown  string `yaml:"cooldown,omitempty"`   // min time between scale events (e.g., "60s")
	Min       int    `yaml:"min"`                  // minimum replicas
	Max       int    `yaml:"max"`                  // maximum replicas
	TargetCPU int    `yaml:"target_cpu,omitempty"` // target CPU % (e.g., 70)
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

// DependsOnCondition represents a dependency condition (matches Docker Compose).
type DependsOnCondition struct {
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`
}

// DependsOnConfig supports both Docker Compose depends_on forms:
//
//	Short form: depends_on: ["db", "redis"]
//	Long form:  depends_on: {db: {condition: service_healthy}}
type DependsOnConfig map[string]DependsOnCondition

// UnmarshalYAML supports both list and map forms for depends_on.
func (d *DependsOnConfig) UnmarshalYAML(value *yaml.Node) error {
	// List form: ["db", "redis"]
	if value.Kind == yaml.SequenceNode {
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		m := make(DependsOnConfig, len(list))
		for _, name := range list {
			m[name] = DependsOnCondition{Condition: "service_started"}
		}
		*d = m
		return nil
	}
	// Map form: {db: {condition: service_healthy}}
	var m map[string]DependsOnCondition
	if err := value.Decode(&m); err != nil {
		return err
	}
	// Default empty conditions to service_started
	for name, cond := range m {
		if cond.Condition == "" {
			m[name] = DependsOnCondition{Condition: "service_started"}
		}
	}
	*d = m
	return nil
}

// ServiceNames returns a sorted list of dependency service names.
func (d DependsOnConfig) ServiceNames() []string {
	if len(d) == 0 {
		return nil
	}
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	return names
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
