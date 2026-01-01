// Package domain contains the core domain entities and value objects for the Agent Registry.
package domain

import "time"

// AgentStatus represents the current state of an agent.
type AgentStatus string

const (
	// AgentStatusOnline indicates the agent is online and healthy.
	AgentStatusOnline AgentStatus = "online"
	// AgentStatusOffline indicates the agent is offline.
	AgentStatusOffline AgentStatus = "offline"
	// AgentStatusDraining indicates the agent is not accepting new tasks.
	AgentStatusDraining AgentStatus = "draining"
	// AgentStatusUnreachable indicates the agent has missed heartbeats.
	AgentStatusUnreachable AgentStatus = "unreachable"
)

// IsValid checks if the status is a valid AgentStatus value.
func (s AgentStatus) IsValid() bool {
	switch s {
	case AgentStatusOnline, AgentStatusOffline, AgentStatusDraining, AgentStatusUnreachable:
		return true
	}
	return false
}

// CapabilityType defines the type of capability an agent has.
type CapabilityType string

const (
	// CapabilityContainerRuntime indicates the agent can run containers.
	CapabilityContainerRuntime CapabilityType = "container_runtime"
	// CapabilityNetworkNode indicates the agent can configure networking.
	CapabilityNetworkNode CapabilityType = "network_node"
	// CapabilitySecurityExecutor indicates the agent can enforce security policies.
	CapabilitySecurityExecutor CapabilityType = "security_executor"
	// CapabilityStorageDriver indicates the agent has storage capabilities.
	CapabilityStorageDriver CapabilityType = "storage_driver"
)

// Capability represents what an agent can do.
type Capability struct {
	Type    CapabilityType
	Version string
	Config  map[string]string
}

// Resources represents available resources on an agent.
type Resources struct {
	CPUCores       int     // Number of CPU cores
	MemoryMB       int64   // Total memory in MB
	CPUAvailable   float64 // Available CPU (0.0-1.0 per core)
	MemoryFreeMB   int64   // Available memory in MB
	ContainerCount int     // Current running containers
	MaxContainers  int     // Maximum containers allowed
}

// HasCapacity checks if agent has capacity for workload.
func (r Resources) HasCapacity(cpuRequired float64, memoryMB int64) bool {
	return r.CPUAvailable >= cpuRequired &&
		r.MemoryFreeMB >= memoryMB &&
		r.ContainerCount < r.MaxContainers
}

// AvailableScore returns a score representing available resources (higher = more available).
func (r Resources) AvailableScore() float64 {
	return r.CPUAvailable + float64(r.MemoryFreeMB)/1024.0
}

// SelectionCriteria specifies requirements for agent selection.
type SelectionCriteria struct {
	RequiredCapabilities []CapabilityType
	MinCPU               float64
	MinMemoryMB          int64
	Labels               map[string]string // Label selectors
	PreferredAgents      []AgentID         // Soft preference
	ExcludedAgents       []AgentID         // Hard exclusion
	Count                int               // Number of agents needed
}

// AgentEvent represents changes to agent state.
type AgentEvent struct {
	Type      AgentEventType
	AgentID   AgentID
	Agent     *Agent
	Timestamp time.Time
}

// AgentEventType defines the type of agent event.
type AgentEventType string

const (
	// AgentEventRegistered indicates an agent was registered.
	AgentEventRegistered AgentEventType = "registered"
	// AgentEventDeregistered indicates an agent was deregistered.
	AgentEventDeregistered AgentEventType = "deregistered"
	// AgentEventOnline indicates an agent came online.
	AgentEventOnline AgentEventType = "online"
	// AgentEventOffline indicates an agent went offline.
	AgentEventOffline AgentEventType = "offline"
	// AgentEventUpdated indicates an agent was updated.
	AgentEventUpdated AgentEventType = "updated"
)

// HeartbeatStatus contains agent's current status from heartbeat.
type HeartbeatStatus struct {
	Status       AgentStatus
	Resources    Resources
	RunningTasks []string
	LastError    string
}

// AgentFilter specifies query filters for listing agents.
type AgentFilter struct {
	Status       *AgentStatus
	Capabilities []CapabilityType
	Labels       map[string]string
}

// RegisterAgentRequest contains registration details.
type RegisterAgentRequest struct {
	Hostname     string
	Address      string
	Capabilities []Capability
	Resources    Resources
	Labels       map[string]string
	Version      string
}

// Validate validates the registration request.
func (r RegisterAgentRequest) Validate() error { //nolint:gocritic // hugeParam - validation method
	if r.Hostname == "" {
		return ErrInvalidHostname
	}
	if r.Address == "" {
		return ErrInvalidAddress
	}
	return nil
}
