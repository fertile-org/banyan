package domain

import (
	"time"
)

// VPCStatus represents the state of a VPC.
type VPCStatus string

const (
	VPCStatusProvisioning VPCStatus = "provisioning"
	VPCStatusActive       VPCStatus = "active"
	VPCStatusDeleting     VPCStatus = "deleting"
	VPCStatusFailed       VPCStatus = "failed"
)

// SubnetPurpose defines what the subnet is used for.
type SubnetPurpose string

const (
	SubnetPurposeContainer  SubnetPurpose = "container"  // Container IP allocation
	SubnetPurposeService    SubnetPurpose = "service"    // Service VIPs
	SubnetPurposeManagement SubnetPurpose = "management" // Management traffic
)

// RuleDirection defines the direction of a security rule.
type RuleDirection string

const (
	RuleDirectionIngress RuleDirection = "ingress"
	RuleDirectionEgress  RuleDirection = "egress"
)

// RuleAction defines the action of a security rule.
type RuleAction string

const (
	RuleActionAllow RuleAction = "allow"
	RuleActionDeny  RuleAction = "deny"
)

// VPC represents a virtual private cloud network.
type VPC struct {
	ID        string
	Name      string
	CIDR      string
	Status    VPCStatus
	Subnets   []Subnet
	CreatedAt time.Time
	Metadata  map[string]string
}

// Subnet represents a network subnet within a VPC.
type Subnet struct {
	ID           string
	VPCID        string
	CIDR         string
	Gateway      string
	AvailableIPs int
	TotalIPs     int
	Purpose      SubnetPurpose
}

// SecurityGroup represents a group of security rules.
type SecurityGroup struct {
	ID        string
	Name      string
	VPCID     string
	Rules     []SecurityRule
	CreatedAt time.Time
}

// SecurityRule defines a single security rule.
type SecurityRule struct {
	Direction   RuleDirection
	Protocol    string // tcp, udp, icmp, all
	PortRange   PortRange
	Source      string // CIDR or security group reference
	Destination string
	Action      RuleAction
	Priority    int
}

// PortRange defines a range of ports.
type PortRange struct {
	Start int
	End   int
}
