package outbound

import (
	"context"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
)

// StateRepository defines state persistence.
type StateRepository interface {
	// Desired state
	SaveDesiredState(ctx context.Context, state *domain.DesiredState) error
	GetDesiredState(ctx context.Context, deploymentID string) (*domain.DesiredState, error)
	DeleteDesiredState(ctx context.Context, deploymentID string) error
	ListDesiredStates(ctx context.Context) ([]*domain.DesiredState, error)

	// Actual state
	SaveActualState(ctx context.Context, state *domain.ActualState) error
	GetActualState(ctx context.Context, deploymentID string) (*domain.ActualState, error)

	// Watch for changes
	WatchDesiredState(ctx context.Context) (<-chan StateChange, error)
	WatchActualState(ctx context.Context) (<-chan StateChange, error)
}

// StateChange represents a change to state.
type StateChange struct {
	Type         ChangeType
	DeploymentID string
}

// ChangeType categorizes the type of change.
type ChangeType string

const (
	ChangeCreated ChangeType = "created"
	ChangeUpdated ChangeType = "updated"
	ChangeDeleted ChangeType = "deleted"
)

// AgentQuerier defines agent state querying.
type AgentQuerier interface {
	// Query agent for running containers
	GetAgentState(ctx context.Context, agentID string) (*AgentState, error)

	// Query all agents
	ListAgentStates(ctx context.Context) ([]*AgentState, error)
}

// AgentState represents the state reported by an agent.
type AgentState struct {
	AgentID     string
	Containers  []ContainerInfo
	Health      string
	CollectedAt time.Time
}

// ContainerInfo represents information about a container.
type ContainerInfo struct {
	ID     string
	Name   string
	Image  string
	Status string
	Health string
	Labels map[string]string
	IP     string
}

// ActionDispatcher defines remediation actions.
type ActionDispatcher interface {
	Dispatch(ctx context.Context, action *domain.ReconcileAction) error
	DispatchBatch(ctx context.Context, actions []*domain.ReconcileAction) error
}
