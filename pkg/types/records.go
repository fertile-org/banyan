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
	Image       string   `json:"image"`
	Placement   string   `json:"placement,omitempty"`
	Ports       []string `json:"ports,omitempty"`
	Environment []string `json:"env,omitempty"`
	Command     []string `json:"command,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Replicas    int      `json:"replicas"`
}

// TaskRecord is stored at /tasks/<agent-id>/<task-id> in etcd.
type TaskRecord struct {
	ContainerCheckedAt time.Time         `json:"container_checked_at,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at"`
	CreatedAt          time.Time         `json:"created_at"`
	Result             *TaskResultRecord `json:"result,omitempty"`
	DeploymentID       string            `json:"deployment_id"`
	ContainerName      string            `json:"container_name"`
	ContainerIP        string            `json:"container_ip,omitempty"`
	Error              string            `json:"error,omitempty"`
	ID                 string            `json:"id"`
	ContainerStatus    string            `json:"container_status,omitempty"`
	ServiceName        string            `json:"service_name"`
	AgentID            string            `json:"agent_id"`
	Type               string            `json:"type"`
	Status             string            `json:"status"`
	Image              string            `json:"image"`
	Command            []string          `json:"command,omitempty"`
	Ports              []string          `json:"ports,omitempty"`
	Environment        []string          `json:"env,omitempty"`
	ReplicaIndex       int               `json:"replica_index"`
}

// TaskResultRecord stores the outcome of task execution.
type TaskResultRecord struct {
	ContainerID string `json:"container_id,omitempty"`
}

// NodeRecord is stored at /nodes/<name> in etcd.
type NodeRecord struct {
	LastSeen   time.Time `json:"last_seen"`
	CreatedAt  time.Time `json:"created_at"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	APIAddress string    `json:"api_address,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
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
