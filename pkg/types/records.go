package types

import (
	"context"
	"io"
	"time"
)

// Etcd key prefixes (relative to store prefix "/banyan/").
const (
	KeyDeployments = "deployments/"
	KeyNodes       = "nodes/"
	KeyTasks       = "tasks/"
	KeyRegistry    = "config/registry"
	KeyEngines     = "engines/"
)

// Deployment statuses.
const (
	StatusPending   = "pending"
	StatusDeploying = "deploying"
	StatusRunning   = "running"
	StatusFailed    = "failed"
	StatusCompleted = "completed"
	StatusStopping  = "stopping"
	StatusStopped   = "stopped"
)

// Task types.
const (
	TaskTypeCreateAndStart = "create_and_start"
	TaskTypeStopAndRemove  = "stop_and_remove"
)

// Update strategies for redeployment.
const (
	// UpdateStrategyBlueGreen deploys new containers first, then tears down old ones
	// after the new deployment is healthy. Requires enough resources to run both simultaneously.
	UpdateStrategyBlueGreen = "blue-green"

	// UpdateStrategyRecreate tears down old containers before deploying new ones.
	// Causes downtime but requires no extra resources.
	UpdateStrategyRecreate = "recreate"
)

// DeploymentRecord is stored at /deployments/<id> in etcd.
type DeploymentRecord struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Status         string                   `json:"status"`
	Services       map[string]ServiceRecord `json:"services"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	Error          string                   `json:"error,omitempty"`
	UpdateStrategy string                   `json:"update_strategy,omitempty"`
	ReplacesID     string                   `json:"replaces_id,omitempty"`
	Tags           []string                 `json:"tags,omitempty"`
}

// ServiceRecord describes a service within a deployment.
type ServiceRecord struct {
	DependsOn         DependsOnConfig      `json:"depends_on,omitempty"`
	Healthcheck       *ManifestHealthcheck `json:"healthcheck,omitempty"`
	Autoscale         *ManifestAutoscale   `json:"autoscale,omitempty"`
	LastScaleAt       time.Time            `json:"last_scale_at,omitempty"`
	Image             string               `json:"image"`
	Placement         string               `json:"placement,omitempty"`
	Restart           string               `json:"restart,omitempty"`
	MemoryLimit       string               `json:"memory_limit,omitempty"`
	CPULimit          string               `json:"cpu_limit,omitempty"`
	MemoryReservation string               `json:"memory_reservation,omitempty"`
	StopGracePeriod   string               `json:"stop_grace_period,omitempty"`
	Ports             []string             `json:"ports,omitempty"`
	Environment       []string             `json:"env,omitempty"`
	Command           []string             `json:"command,omitempty"`
	Entrypoint        []string             `json:"entrypoint,omitempty"`
	Volumes           []VolumeMount        `json:"volumes,omitempty"`
	Replicas          int                  `json:"replicas"`
}

// TaskRecord is stored at /tasks/<agent-id>/<task-id> in etcd.
type TaskRecord struct {
	ContainerCheckedAt time.Time            `json:"container_checked_at,omitempty"`
	UpdatedAt          time.Time            `json:"updated_at"`
	CreatedAt          time.Time            `json:"created_at"`
	Result             *TaskResultRecord    `json:"result,omitempty"`
	Healthcheck        *ManifestHealthcheck `json:"healthcheck,omitempty"`
	DeploymentID       string               `json:"deployment_id"`
	DeploymentName     string               `json:"deployment_name,omitempty"`
	ContainerName      string               `json:"container_name"`
	ContainerIP        string               `json:"container_ip,omitempty"`
	Error              string               `json:"error,omitempty"`
	ID                 string               `json:"id"`
	ContainerStatus    string               `json:"container_status,omitempty"`
	HealthStatus       string               `json:"health_status,omitempty"`
	ServiceName        string               `json:"service_name"`
	AgentID            string               `json:"agent_id"`
	Type               string               `json:"type"`
	Status             string               `json:"status"`
	Image              string               `json:"image"`
	Restart            string               `json:"restart,omitempty"`
	MemoryLimit        string               `json:"memory_limit,omitempty"`
	CPULimit           string               `json:"cpu_limit,omitempty"`
	MemoryReservation  string               `json:"memory_reservation,omitempty"`
	Command            []string             `json:"command,omitempty"`
	Entrypoint         []string             `json:"entrypoint,omitempty"`
	Ports              []string             `json:"ports,omitempty"`
	Environment        []string             `json:"env,omitempty"`
	Volumes            []VolumeMount        `json:"volumes,omitempty"`
	ReplicaIndex       int                  `json:"replica_index"`
	CPUPercent         float64              `json:"cpu_percent,omitempty"`
	MemoryUsedBytes    uint64               `json:"memory_used_bytes,omitempty"`
	MemoryLimitBytes   uint64               `json:"memory_limit_bytes,omitempty"`
}

// TaskResultRecord stores the outcome of task execution.
type TaskResultRecord struct {
	ContainerID string `json:"container_id,omitempty"`
}

// NodeRecord is stored at /nodes/<name> in etcd.
type NodeRecord struct {
	LastSeen         time.Time `json:"last_seen"`
	CreatedAt        time.Time `json:"created_at"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	APIAddress       string    `json:"api_address,omitempty"`
	Tags             []string  `json:"tags,omitempty"`
	MemoryTotalBytes uint64    `json:"memory_total_bytes,omitempty"`
	MemoryUsedBytes  uint64    `json:"memory_used_bytes,omitempty"`
	CPUCores         uint32    `json:"cpu_cores,omitempty"`
	CPUUsageRatio    float64   `json:"cpu_usage_ratio,omitempty"`
}

// EngineRecord is stored at /engines/<id> in etcd with a TTL lease.
// Auto-expires if the engine crashes (15s TTL, renewed every 10s).
type EngineRecord struct {
	StartedAt   time.Time `json:"started_at"`
	LastSeen    time.Time `json:"last_seen"`
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	GRPCAddr    string    `json:"grpc_addr"`
	RegistryURL string    `json:"registry_url,omitempty"`
}

// StateStore is a minimal interface for store operations used by helpers.
type StateStore interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Save(ctx context.Context, key string, value interface{}) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// LogProvider retrieves container logs.
type LogProvider interface {
	StreamLogs(ctx context.Context, containerName string, opts LogOptions) (io.ReadCloser, error)
}

// LogOptions configures log retrieval.
type LogOptions struct {
	Follow bool
	Tail   int
}
