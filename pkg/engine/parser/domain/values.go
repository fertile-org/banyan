// Package domain defines the core entities for the Compose Parser.
package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// PortConfig represents a port mapping.
type PortConfig struct {
	Target    uint32
	Published string
	Protocol  string
	Mode      string
}

// ParsePort parses a port string like "8080:80/tcp".
func ParsePort(portStr string) (PortConfig, error) {
	pc := PortConfig{Protocol: "tcp"}

	// Handle protocol suffix
	if idx := strings.LastIndex(portStr, "/"); idx != -1 {
		pc.Protocol = portStr[idx+1:]
		portStr = portStr[:idx]
	}

	parts := strings.Split(portStr, ":")
	switch len(parts) {
	case 1:
		port, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return pc, fmt.Errorf("invalid port: %s", parts[0])
		}
		pc.Target = uint32(port)
		pc.Published = parts[0]
	case 2:
		pc.Published = parts[0]
		port, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return pc, fmt.Errorf("invalid target port: %s", parts[1])
		}
		pc.Target = uint32(port)
	default:
		return pc, fmt.Errorf("invalid port format: %s", portStr)
	}

	return pc, nil
}

// VolumeMount represents a volume mount configuration.
type VolumeMount struct {
	Type        string
	Source      string
	Target      string
	ReadOnly    bool
	Consistency string
	Bind        *BindOptions
	Volume      *VolumeOptions
	Tmpfs       *TmpfsOptions
}

// BindOptions represents bind mount options.
type BindOptions struct {
	Propagation string
}

// VolumeOptions represents volume mount options.
type VolumeOptions struct {
	NoCopy bool
}

// TmpfsOptions represents tmpfs mount options.
type TmpfsOptions struct {
	Size int64
	Mode uint32
}

// ServiceNetworkConfig represents service-specific network settings.
type ServiceNetworkConfig struct {
	Aliases     []string
	IPV4Address string
	IPV6Address string
	Priority    int
}

// DependsOnConfig represents service dependency configuration.
type DependsOnConfig struct {
	Condition string
	Restart   bool
}

// DeployConfig represents deployment configuration.
type DeployConfig struct {
	Replicas       *int
	Resources      ResourcesConfig
	RestartPolicy  *RestartPolicy
	Placement      DeployPlacementConfig
	UpdateConfig   *UpdateConfig
	RollbackConfig *UpdateConfig
	Labels         map[string]string
}

// ResourcesConfig represents resource limits and reservations.
type ResourcesConfig struct {
	Limits       ResourceSpec
	Reservations ResourceSpec
}

// ResourceSpec defines CPU and memory specifications.
type ResourceSpec struct {
	CPUs    string
	Memory  string
	Devices []DeviceConfig
}

// DeviceConfig represents device configuration.
type DeviceConfig struct {
	Capabilities []string
	Driver       string
	Count        int
	DeviceIDs    []string
	Options      map[string]string
}

// RestartPolicy represents restart policy configuration.
type RestartPolicy struct {
	Condition   string
	Delay       string
	MaxAttempts int
	Window      string
}

// DeployPlacementConfig represents deployment placement configuration.
type DeployPlacementConfig struct {
	Constraints []string
	Preferences []PlacementPreference
}

// UpdateConfig represents update/rollback configuration.
type UpdateConfig struct {
	Parallelism     int
	Delay           string
	FailureAction   string
	Monitor         string
	MaxFailureRatio float64
	Order           string
}

// HealthCheckConfig represents health check configuration.
type HealthCheckConfig struct {
	Test        []string
	Interval    string
	Timeout     string
	Retries     int
	StartPeriod string
	Disable     bool
}

// BuildConfig represents build configuration.
type BuildConfig struct {
	Context    string
	Dockerfile string
	Args       map[string]string
	Target     string
	CacheFrom  []string
	Labels     map[string]string
	Network    string
	ShmSize    string
}

// IPAMConfig represents IP Address Management configuration.
type IPAMConfig struct {
	Driver string
	Config []IPAMPoolConfig
}

// IPAMPoolConfig represents an IPAM pool.
type IPAMPoolConfig struct {
	Subnet  string
	Gateway string
	IPRange string
}
