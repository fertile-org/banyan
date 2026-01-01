// Package inbound defines the inbound ports (service interfaces) for the Agent Registry.
package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
)

// RegistryService defines the agent registry operations.
type RegistryService interface {
	// Registration
	RegisterAgent(ctx context.Context, req domain.RegisterAgentRequest) (*domain.Agent, error)
	DeregisterAgent(ctx context.Context, agentID domain.AgentID) error

	// Heartbeat
	ProcessHeartbeat(ctx context.Context, agentID domain.AgentID, status domain.HeartbeatStatus) error

	// Query
	GetAgent(ctx context.Context, agentID domain.AgentID) (*domain.Agent, error)
	ListAgents(ctx context.Context, filter domain.AgentFilter) ([]domain.Agent, error)

	// Selection
	SelectAgents(ctx context.Context, criteria domain.SelectionCriteria, strategy string) ([]domain.Agent, error)

	// Maintenance
	DrainAgent(ctx context.Context, agentID domain.AgentID) error
	ActivateAgent(ctx context.Context, agentID domain.AgentID) error
}
