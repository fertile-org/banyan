// Package domain contains the core domain entities and value objects for the Agent Registry.
package domain

import "errors"

// Domain errors for the Agent Registry.
var (
	// ErrAgentNotFound indicates the agent was not found.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrAgentAlreadyExists indicates the agent already exists.
	ErrAgentAlreadyExists = errors.New("agent already exists")
	// ErrInsufficientAgents indicates not enough agents are available.
	ErrInsufficientAgents = errors.New("insufficient agents available")
	// ErrAgentDraining indicates the agent is draining.
	ErrAgentDraining = errors.New("agent is draining")
	// ErrHeartbeatTimeout indicates a heartbeat timeout.
	ErrHeartbeatTimeout = errors.New("heartbeat timeout")
	// ErrInvalidHostname indicates an invalid hostname.
	ErrInvalidHostname = errors.New("invalid hostname")
	// ErrInvalidAddress indicates an invalid address.
	ErrInvalidAddress = errors.New("invalid address")
	// ErrInvalidStatus indicates an invalid status.
	ErrInvalidStatus = errors.New("invalid status")
)
