// Package domain contains the core domain entities and value objects for the Agent Registry.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Agent represents a registered agent in the cluster.
type Agent struct {
	ID            AgentID
	Hostname      string
	Address       string // gRPC address
	Status        AgentStatus
	Capabilities  []Capability
	Resources     Resources
	Labels        map[string]string
	RegisteredAt  time.Time
	LastHeartbeat time.Time
	Version       string
}

// AgentID is a unique identifier for an agent.
type AgentID string

// NewAgentID generates a new unique agent ID.
func NewAgentID() AgentID {
	return AgentID(uuid.New().String())
}

// String returns the string representation of the AgentID.
func (id AgentID) String() string {
	return string(id)
}

// IsEmpty checks if the AgentID is empty.
func (id AgentID) IsEmpty() bool {
	return id == ""
}

// HasCapabilities checks if the agent has all the required capabilities.
func (a *Agent) HasCapabilities(required []CapabilityType) bool {
	if len(required) == 0 {
		return true
	}

	capMap := make(map[CapabilityType]bool)
	for _, cap := range a.Capabilities {
		capMap[cap.Type] = true
	}

	for _, req := range required {
		if !capMap[req] {
			return false
		}
	}
	return true
}

// IsHealthy checks if the agent is in a healthy state.
func (a *Agent) IsHealthy() bool {
	return a.Status == AgentStatusOnline
}

// CanAcceptTasks checks if the agent can accept new tasks.
func (a *Agent) CanAcceptTasks() bool {
	return a.Status == AgentStatusOnline && a.Resources.HasCapacity(0, 0)
}

// MarkOnline sets the agent status to online.
func (a *Agent) MarkOnline() {
	a.Status = AgentStatusOnline
	a.LastHeartbeat = time.Now()
}

// MarkOffline sets the agent status to offline.
func (a *Agent) MarkOffline() {
	a.Status = AgentStatusOffline
}

// MarkUnreachable sets the agent status to unreachable.
func (a *Agent) MarkUnreachable() {
	a.Status = AgentStatusUnreachable
}

// MarkDraining sets the agent status to draining.
func (a *Agent) MarkDraining() {
	a.Status = AgentStatusDraining
}

// UpdateHeartbeat updates the agent's heartbeat timestamp and resources.
func (a *Agent) UpdateHeartbeat(status AgentStatus, resources Resources) {
	a.Status = status
	a.Resources = resources
	a.LastHeartbeat = time.Now()
}
