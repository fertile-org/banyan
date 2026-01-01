package domain

import (
	"fmt"
	"strings"
)

// ContainerID is a unique identifier for a container.
type ContainerID string

// NewContainerID creates a new ContainerID after validation.
func NewContainerID(id string) (ContainerID, error) {
	if len(id) < 12 {
		return "", ErrInvalidContainerID
	}
	return ContainerID(id), nil
}

// String returns the string representation of ContainerID.
func (id ContainerID) String() string {
	return string(id)
}

// Short returns the first 12 characters of the container ID.
func (id ContainerID) Short() string {
	if len(id) >= 12 {
		return string(id[:12])
	}
	return string(id)
}

// Validate checks if the container ID is valid.
func (id ContainerID) Validate() error {
	if len(id) < 12 {
		return ErrInvalidContainerID
	}
	return nil
}

// ImageID is a unique identifier for an image.
type ImageID string

// String returns the string representation of ImageID.
func (id ImageID) String() string {
	return string(id)
}

// Validate checks if the image ID is valid.
func (id ImageID) Validate() error {
	if id == "" {
		return ErrInvalidImageRef
	}
	return nil
}

// ImageRef represents a container image reference.
type ImageRef struct {
	Registry string // e.g., "docker.io"
	Name     string // e.g., "library/nginx"
	Tag      string // e.g., "latest" or "1.21"
	Digest   string // e.g., "sha256:..."
}

// ParseImageRef parses an image reference string.
func ParseImageRef(ref string) (ImageRef, error) {
	if ref == "" {
		return ImageRef{}, ErrInvalidImageRef
	}

	result := ImageRef{
		Registry: "docker.io",
		Tag:      "latest",
	}

	// Handle digest
	if idx := strings.Index(ref, "@"); idx != -1 {
		result.Digest = ref[idx+1:]
		ref = ref[:idx]
	}

	// Handle tag
	if idx := strings.LastIndex(ref, ":"); idx != -1 && !strings.Contains(ref[idx:], "/") {
		result.Tag = ref[idx+1:]
		ref = ref[:idx]
	}

	// Handle registry
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		result.Registry = parts[0]
		result.Name = parts[1]
	} else {
		// No registry specified, use default
		if len(parts) == 1 {
			result.Name = "library/" + parts[0]
		} else {
			result.Name = ref
		}
	}

	if result.Name == "" {
		return ImageRef{}, ErrInvalidImageRef
	}

	return result, nil
}

// String returns the full image reference string.
func (r ImageRef) String() string {
	var ref string
	if r.Registry != "" {
		ref = r.Registry + "/" + r.Name
	} else {
		ref = r.Name
	}

	if r.Digest != "" {
		return ref + "@" + r.Digest
	}
	if r.Tag != "" {
		return ref + ":" + r.Tag
	}
	return ref + ":latest"
}

// Validate checks if the image reference is valid.
func (r ImageRef) Validate() error {
	if r.Name == "" {
		return ErrInvalidImageRef
	}
	return nil
}

// ContainerState represents the current state of a container.
type ContainerState string

const (
	ContainerStateCreated    ContainerState = "created"
	ContainerStateRunning    ContainerState = "running"
	ContainerStatePaused     ContainerState = "paused"
	ContainerStateRestarting ContainerState = "restarting"
	ContainerStateStopped    ContainerState = "stopped"
	ContainerStateExited     ContainerState = "exited"
	ContainerStateRemoving   ContainerState = "removing"
	ContainerStateDead       ContainerState = "dead"
)

// String returns the string representation of ContainerState.
func (s ContainerState) String() string {
	return string(s)
}

// IsRunning returns true if the container is in a running state.
func (s ContainerState) IsRunning() bool {
	return s == ContainerStateRunning
}

// IsTerminal returns true if the container is in a terminal state.
func (s ContainerState) IsTerminal() bool {
	return s == ContainerStateExited || s == ContainerStateDead
}

// MountType represents the type of mount.
type MountType string

const (
	MountTypeBind   MountType = "bind"
	MountTypeVolume MountType = "volume"
	MountTypeTmpfs  MountType = "tmpfs"
)

// MountPoint defines a volume mount.
type MountPoint struct {
	Source      string
	Destination string
	Type        MountType
	Propagation string
	ReadOnly    bool
}

// Validate checks if the mount point is valid.
func (m MountPoint) Validate() error {
	if m.Destination == "" {
		return ErrInvalidMountDestination
	}
	return nil
}

// Protocol represents a network protocol.
type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
)

// PortBinding maps container port to host.
type PortBinding struct {
	Protocol      Protocol
	HostIP        string
	ContainerPort uint16
	HostPort      uint16
}

// String returns a string representation of the port binding.
func (p PortBinding) String() string {
	proto := p.Protocol
	if proto == "" {
		proto = ProtocolTCP
	}
	if p.HostIP != "" {
		return fmt.Sprintf("%s:%d->%d/%s", p.HostIP, p.HostPort, p.ContainerPort, proto)
	}
	return fmt.Sprintf("%d->%d/%s", p.HostPort, p.ContainerPort, proto)
}

// EnvVar represents an environment variable.
type EnvVar struct {
	Name  string
	Value string
}

// String returns the environment variable in KEY=VALUE format.
func (e EnvVar) String() string {
	return e.Name + "=" + e.Value
}

// NetworkMode represents the network mode for a container.
type NetworkMode string

const (
	NetworkModeBridge NetworkMode = "bridge"
	NetworkModeHost   NetworkMode = "host"
	NetworkModeNone   NetworkMode = "none"
	NetworkModeCustom NetworkMode = "custom"
)

// RestartPolicy defines the restart behavior for a container.
type RestartPolicy struct {
	Name              string // no, always, on-failure, unless-stopped
	MaximumRetryCount int
}

// HealthStatus represents the health status of a container.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusStarting  HealthStatus = "starting"
	HealthStatusNone      HealthStatus = "none"
)
