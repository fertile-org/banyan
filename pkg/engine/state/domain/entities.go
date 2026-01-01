package domain

import "time"

// DesiredState represents what should be running for a deployment.
type DesiredState struct {
	DeploymentID string
	Services     map[string]ServiceDesiredState
	UpdatedAt    time.Time
}

// ServiceDesiredState defines the desired state of a service.
type ServiceDesiredState struct {
	Name        string
	Image       string
	Replicas    int
	Environment map[string]string
	Ports       []PortMapping
	NetworkID   string
	AgentIDs    []string // Target agents for placement
}

// ActualState represents what is currently running.
type ActualState struct {
	DeploymentID string
	Services     map[string]ServiceActualState
	CollectedAt  time.Time
}

// ServiceActualState captures the real state of a service.
type ServiceActualState struct {
	Name      string
	Instances []InstanceState
}

// InstanceState represents a single running instance.
type InstanceState struct {
	ContainerID string
	AgentID     string
	IP          string
	Status      ContainerStatus
	Health      HealthStatus
	StartedAt   time.Time
}

// ContainerStatus represents the status of a container.
type ContainerStatus string

const (
	ContainerRunning  ContainerStatus = "running"
	ContainerStopped  ContainerStatus = "stopped"
	ContainerStarting ContainerStatus = "starting"
	ContainerFailed   ContainerStatus = "failed"
)

// HealthStatus represents the health status of an instance.
type HealthStatus string

const (
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
	HealthUnknown   HealthStatus = "unknown"
)

// StateDrift captures differences between desired and actual state.
type StateDrift struct {
	DeploymentID string
	Drifts       []Drift
	DetectedAt   time.Time
	Severity     DriftSeverity
}

// Drift represents a single difference between desired and actual state.
type Drift struct {
	Type        DriftType
	ServiceName string
	Details     string
	AgentID     string // Which agent is affected
}

// DriftType categorizes the type of drift.
type DriftType string

const (
	DriftMissing   DriftType = "missing"    // Service should exist but doesn't
	DriftExtra     DriftType = "extra"      // Service exists but shouldn't
	DriftReplicas  DriftType = "replicas"   // Wrong number of replicas
	DriftConfig    DriftType = "config"     // Configuration mismatch
	DriftUnhealthy DriftType = "unhealthy"  // Instance not healthy
	DriftWrongHost DriftType = "wrong_host" // Running on wrong agent
)

// DriftSeverity indicates the severity of drift.
type DriftSeverity string

const (
	SeverityCritical DriftSeverity = "critical" // Service completely down
	SeverityHigh     DriftSeverity = "high"     // Partial outage
	SeverityMedium   DriftSeverity = "medium"   // Degraded
	SeverityLow      DriftSeverity = "low"      // Minor drift
)
