package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
)

// DeploymentRepository defines persistence operations.
type DeploymentRepository interface {
	Save(ctx context.Context, deployment *domain.Deployment) error
	Get(ctx context.Context, id string) (*domain.Deployment, error)
	Update(ctx context.Context, deployment *domain.Deployment) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter inbound.DeploymentFilter) ([]*domain.Deployment, error)
}

// AgentDispatcher defines agent communication.
type AgentDispatcher interface {
	// DispatchTask sends a task to a specific agent.
	DispatchTask(ctx context.Context, agentID string, task *Task) (*TaskResult, error)

	// DispatchBatch sends tasks to multiple agents in parallel.
	DispatchBatch(ctx context.Context, tasks map[string]*Task) (map[string]*TaskResult, error)

	// GetAgentStatus retrieves current status of an agent.
	GetAgentStatus(ctx context.Context, agentID string) (*AgentStatus, error)
}

// Task represents work to send to an agent.
type Task struct {
	ID           string
	DeploymentID string
	Type         TaskType
	Payload      map[string]interface{}
}

// TaskType categorizes the type of task.
type TaskType string

const (
	TaskDeployService TaskType = "deploy_service"
	TaskStopService   TaskType = "stop_service"
	TaskSetupNetwork  TaskType = "setup_network"
	TaskApplyRules    TaskType = "apply_rules"
)

// TaskResult contains the result of a task execution.
type TaskResult struct {
	TaskID string
	Status string
	Output map[string]interface{}
	Error  string
}

// AgentStatus represents the status of an agent.
type AgentStatus struct {
	AgentID   string
	Available bool
	Health    string
}

// PluginExecutor defines lifecycle plugin execution.
type PluginExecutor interface {
	ExecuteHook(ctx context.Context, hook LifecycleHook, data *HookData) (*HookResult, error)
}

// LifecycleHook defines a lifecycle hook type.
type LifecycleHook string

const (
	HookValidate LifecycleHook = "validate"
	HookPlan     LifecycleHook = "plan"
	HookDeploy   LifecycleHook = "deploy"
	HookVerify   LifecycleHook = "verify"
	HookDestroy  LifecycleHook = "destroy"
)

// HookData contains data passed to lifecycle hooks.
type HookData struct {
	DeploymentID string
	Hook         LifecycleHook
	Services     []domain.Service
	Context      map[string]interface{}
}

// HookResult contains the result of a lifecycle hook.
type HookResult struct {
	Continue bool
	Message  string
	Modified map[string]interface{}
	Errors   []string
}

// Scheduler defines deployment scheduling.
type Scheduler interface {
	Schedule(ctx context.Context, services []domain.Service, deps *domain.DependencyGraph) (*domain.ExecutionPlan, error)
	ValidateDependencies(ctx context.Context, deps *domain.DependencyGraph) error
}

// BanyanParser defines banyan.yml parsing.
type BanyanParser interface {
	Parse(banyanContent string) ([]domain.Service, error)
}
