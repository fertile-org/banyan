package domain

import (
	"fmt"
	"time"

	shareddomain "github.com/fertile-org/banyan/pkg/shared/domain"
)

// Container represents a running or stopped container on the node.
type Container struct {
	CreatedAt   time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
	ExitCode    *int
	ImageRef    ImageRef
	ID          ContainerID
	Name        string
	ServiceID   string
	State       ContainerState
	NetworkInfo NetworkInfo
	Config      ContainerConfig
}

// NetworkInfo contains network information for a container.
type NetworkInfo struct {
	NetworkID  string
	IPAddress  string
	Gateway    string
	MacAddress string
	Ports      []PortBinding
}

// ContainerConfig contains all configuration for creating a container.
type ContainerConfig struct {
	Labels        map[string]string
	HealthCheck   *HealthCheckConfig
	Image         ImageRef
	User          string
	Hostname      string
	WorkingDir    string
	NetworkMode   NetworkMode
	RestartPolicy RestartPolicy
	Ports         []PortBinding
	Mounts        []MountPoint
	Env           []EnvVar
	Entrypoint    []string
	Command       []string
	Resources     ResourceLimits
}

// ResourceLimits defines resource constraints for a container.
type ResourceLimits struct {
	CPUShares      int64 // CPU shares (relative weight)
	CPUQuota       int64 // CPU CFS quota (microseconds per period)
	CPUPeriod      int64 // CPU CFS period (microseconds)
	MemoryLimit    int64 // Memory limit in bytes
	MemorySwap     int64 // Memory + swap limit (-1 for unlimited)
	PidsLimit      int64 // Process limit
	OOMKillDisable bool  // Disable OOM killer
}

// IsEmpty returns true if no resource limits are set.
func (r ResourceLimits) IsEmpty() bool {
	return r.CPUShares == 0 && r.CPUQuota == 0 && r.CPUPeriod == 0 &&
		r.MemoryLimit == 0 && r.MemorySwap == 0 && r.PidsLimit == 0
}

// HealthCheckConfig defines health check configuration.
type HealthCheckConfig struct {
	Test        []string      // Command to run
	Interval    time.Duration // Time between running the check
	Timeout     time.Duration // Maximum time to allow one check to run
	Retries     int           // Consecutive failures needed to report unhealthy
	StartPeriod time.Duration // Start period for the container to initialize
}

// validTransitions defines the valid state transitions for a container.
var validTransitions = map[ContainerState][]ContainerState{
	ContainerStateCreated:    {ContainerStateRunning, ContainerStateDead},
	ContainerStateRunning:    {ContainerStatePaused, ContainerStateRestarting, ContainerStateExited, ContainerStateDead},
	ContainerStatePaused:     {ContainerStateRunning, ContainerStateDead},
	ContainerStateRestarting: {ContainerStateRunning, ContainerStateExited, ContainerStateDead},
	ContainerStateExited:     {ContainerStateRunning, ContainerStateDead},
	ContainerStateDead:       {}, // Terminal state
}

// CanTransitionTo checks if the container can transition to the new state.
func (c *Container) CanTransitionTo(newState ContainerState) bool {
	allowed, exists := validTransitions[c.State]
	if !exists {
		return false
	}

	for _, s := range allowed {
		if s == newState {
			return true
		}
	}
	return false
}

// TransitionTo transitions the container to a new state if valid.
func (c *Container) TransitionTo(newState ContainerState) error {
	if !c.CanTransitionTo(newState) {
		return ErrInvalidStateTransition
	}
	c.State = newState
	return nil
}

// Validate validates the container configuration.
func (c *ContainerConfig) Validate() error {
	v := shareddomain.NewValidationError("ContainerConfig")

	if c.Image.Name == "" {
		_ = v.AddFieldError("image", "image is required")
	}

	if c.Resources.MemoryLimit < 0 {
		_ = v.AddFieldError("resources.memory_limit", "must be non-negative")
	}

	if c.Resources.CPUQuota < 0 {
		_ = v.AddFieldError("resources.cpu_quota", "must be non-negative")
	}

	if c.Resources.CPUPeriod < 0 {
		_ = v.AddFieldError("resources.cpu_period", "must be non-negative")
	}

	if c.Resources.PidsLimit < 0 {
		_ = v.AddFieldError("resources.pids_limit", "must be non-negative")
	}

	for i, mount := range c.Mounts {
		if err := mount.Validate(); err != nil {
			_ = v.AddFieldError("mounts", fmt.Sprintf("mount %d: %v", i, err))
		}
	}

	if v.HasErrors() {
		return v
	}
	return nil
}
