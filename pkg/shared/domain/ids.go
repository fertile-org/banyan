// Package domain provides shared domain models used across Engine and Agent components.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ID is the base interface for all typed identifiers.
type ID interface {
	String() string
	IsEmpty() bool
}

// DeploymentID uniquely identifies a deployment.
type DeploymentID string

// NewDeploymentID creates a new deployment ID with the given prefix.
func NewDeploymentID() DeploymentID {
	return DeploymentID(generateID("dep"))
}

// ParseDeploymentID parses a string into a DeploymentID.
func ParseDeploymentID(s string) (DeploymentID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return DeploymentID(s), nil
}

func (id DeploymentID) String() string { return string(id) }
func (id DeploymentID) IsEmpty() bool  { return id == "" }

// AgentID uniquely identifies an agent node.
type AgentID string

// NewAgentID creates a new agent ID.
func NewAgentID() AgentID {
	return AgentID(generateID("agent"))
}

// ParseAgentID parses a string into an AgentID.
func ParseAgentID(s string) (AgentID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return AgentID(s), nil
}

func (id AgentID) String() string { return string(id) }
func (id AgentID) IsEmpty() bool  { return id == "" }

// TaskID uniquely identifies a task.
type TaskID string

// NewTaskID creates a new task ID.
func NewTaskID() TaskID {
	return TaskID(generateID("task"))
}

// ParseTaskID parses a string into a TaskID.
func ParseTaskID(s string) (TaskID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return TaskID(s), nil
}

func (id TaskID) String() string { return string(id) }
func (id TaskID) IsEmpty() bool  { return id == "" }

// ContainerID uniquely identifies a container.
type ContainerID string

// NewContainerID creates a new container ID.
func NewContainerID() ContainerID {
	return ContainerID(generateID("ctr"))
}

// ParseContainerID parses a string into a ContainerID.
func ParseContainerID(s string) (ContainerID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return ContainerID(s), nil
}

func (id ContainerID) String() string { return string(id) }
func (id ContainerID) IsEmpty() bool  { return id == "" }

// ServiceID uniquely identifies a service within a deployment.
type ServiceID string

// NewServiceID creates a new service ID.
func NewServiceID() ServiceID {
	return ServiceID(generateID("svc"))
}

// ParseServiceID parses a string into a ServiceID.
func ParseServiceID(s string) (ServiceID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return ServiceID(s), nil
}

func (id ServiceID) String() string { return string(id) }
func (id ServiceID) IsEmpty() bool  { return id == "" }

// PluginID uniquely identifies a plugin.
type PluginID string

// NewPluginID creates a new plugin ID.
func NewPluginID() PluginID {
	return PluginID(generateID("plugin"))
}

// ParsePluginID parses a string into a PluginID.
func ParsePluginID(s string) (PluginID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return PluginID(s), nil
}

func (id PluginID) String() string { return string(id) }
func (id PluginID) IsEmpty() bool  { return id == "" }

// VPCID uniquely identifies a VPC.
type VPCID string

// NewVPCID creates a new VPC ID.
func NewVPCID() VPCID {
	return VPCID(generateID("vpc"))
}

// ParseVPCID parses a string into a VPCID.
func ParseVPCID(s string) (VPCID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return VPCID(s), nil
}

func (id VPCID) String() string { return string(id) }
func (id VPCID) IsEmpty() bool  { return id == "" }

// SubnetID uniquely identifies a subnet.
type SubnetID string

// NewSubnetID creates a new subnet ID.
func NewSubnetID() SubnetID {
	return SubnetID(generateID("subnet"))
}

// ParseSubnetID parses a string into a SubnetID.
func ParseSubnetID(s string) (SubnetID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return SubnetID(s), nil
}

func (id SubnetID) String() string { return string(id) }
func (id SubnetID) IsEmpty() bool  { return id == "" }

// SecurityGroupID uniquely identifies a security group.
type SecurityGroupID string

// NewSecurityGroupID creates a new security group ID.
func NewSecurityGroupID() SecurityGroupID {
	return SecurityGroupID(generateID("sg"))
}

// ParseSecurityGroupID parses a string into a SecurityGroupID.
func ParseSecurityGroupID(s string) (SecurityGroupID, error) {
	if s == "" {
		return "", ErrEmptyID
	}
	return SecurityGroupID(s), nil
}

func (id SecurityGroupID) String() string { return string(id) }
func (id SecurityGroupID) IsEmpty() bool  { return id == "" }

// generateID creates a unique ID with the given prefix.
// Format: prefix-timestamp-random (e.g., dep-1703123456-a1b2c3d4)
func generateID(prefix string) string {
	timestamp := time.Now().Unix()
	randomBytes := make([]byte, 4)
	_, _ = rand.Read(randomBytes)
	random := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s-%d-%s", prefix, timestamp, random)
}

// IDFromString attempts to parse an ID string and determine its type.
func IDFromString(s string) (ID, error) {
	if s == "" {
		return nil, ErrEmptyID
	}

	parts := strings.SplitN(s, "-", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("%w: invalid ID format", ErrInvalidID)
	}

	switch parts[0] {
	case "dep":
		return DeploymentID(s), nil
	case "agent":
		return AgentID(s), nil
	case "task":
		return TaskID(s), nil
	case "ctr":
		return ContainerID(s), nil
	case "svc":
		return ServiceID(s), nil
	case "plugin":
		return PluginID(s), nil
	case "vpc":
		return VPCID(s), nil
	case "subnet":
		return SubnetID(s), nil
	case "sg":
		return SecurityGroupID(s), nil
	default:
		return nil, fmt.Errorf("%w: unknown ID type: %s", ErrInvalidID, parts[0])
	}
}
