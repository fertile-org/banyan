package types

import (
	"context"
	"io"
	"time"
)

// DefaultCLITokenTTL is how long CLI tokens remain valid before requiring re-authentication.
const DefaultCLITokenTTL = 30 * 24 * time.Hour // 30 days

// Etcd key prefixes (relative to store prefix "/banyan/").
const (
	KeyDeployments = "deployments/"
	KeyNodes       = "nodes/"
	KeyTasks       = "tasks/"
	KeyRegistry    = "config/registry"
	KeyTokens      = "tokens/"
	KeyTokenIndex  = "token-index/"
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

// TokenRecord is stored at /tokens/{sha256-hash} in etcd.
type TokenRecord struct {
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
}

// IsExpired returns true if the token has a non-zero expiry time that is in the past.
func (t *TokenRecord) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(t.ExpiresAt)
}

// DeploymentRecord is stored at /deployments/<id> in etcd.
type DeploymentRecord struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Status    string                   `json:"status"`
	Services  map[string]ServiceRecord `json:"services"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Error     string                   `json:"error,omitempty"`
}

// ServiceRecord describes a service within a deployment.
type ServiceRecord struct {
	Image       string   `json:"image"`
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
