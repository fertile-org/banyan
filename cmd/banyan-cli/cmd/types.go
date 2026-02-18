package cmd

import "time"

// Etcd key prefixes (relative to store prefix "/banyan/")
const (
	keyDeployments = "deployments/"
	keyNodes       = "nodes/"
	keyTasks       = "tasks/"
)

// Deployment statuses
const (
	statusPending   = "pending"
	statusDeploying = "deploying"
	statusRunning   = "running"
	statusFailed    = "failed"
	statusCompleted = "completed"
)

// Task types
const (
	taskTypeCreateAndStart = "create_and_start"
)

// DeploymentRecord is stored at /deployments/<id> in etcd.
// Written by the deploy command, read by the Engine.
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
	Replicas    int      `json:"replicas"`
	Ports       []string `json:"ports,omitempty"`
	Environment []string `json:"env,omitempty"`
	Command     []string `json:"command,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// TaskRecord is stored at /tasks/<agent-id>/<task-id> in etcd.
// Written by the Engine, read and executed by the Agent.
type TaskRecord struct {
	ID            string            `json:"id"`
	DeploymentID  string            `json:"deployment_id"`
	ServiceName   string            `json:"service_name"`
	ReplicaIndex  int               `json:"replica_index"`
	AgentID       string            `json:"agent_id"`
	Type          string            `json:"type"`
	Status        string            `json:"status"`
	Image         string            `json:"image"`
	ContainerName string            `json:"container_name"`
	Ports         []string          `json:"ports,omitempty"`
	Environment   []string          `json:"env,omitempty"`
	Command       []string          `json:"command,omitempty"`
	Result        *TaskResultRecord `json:"result,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Error         string            `json:"error,omitempty"`
}

// TaskResultRecord stores the outcome of task execution.
type TaskResultRecord struct {
	ContainerID string `json:"container_id,omitempty"`
}

// NodeRecord is stored at /nodes/<name> in etcd.
// Written by the Agent, read by the Engine for scheduling.
type NodeRecord struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
}
