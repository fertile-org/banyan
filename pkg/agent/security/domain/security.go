package domain

// RuleID is a unique identifier for a security rule.
type RuleID string

// NetworkID is a unique identifier for a network.
type NetworkID string

// ContainerID is a unique identifier for a container.
type ContainerID string

// Direction indicates traffic direction.
type Direction string

const (
	DirectionIngress Direction = "ingress"
	DirectionEgress  Direction = "egress"
)

// Action indicates what to do with matching traffic.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// Protocol represents a network protocol.
type Protocol string

const (
	ProtocolTCP  Protocol = "tcp"
	ProtocolUDP  Protocol = "udp"
	ProtocolICMP Protocol = "icmp"
	ProtocolAny  Protocol = ""
)

// SecurityRule represents a network security rule.
type SecurityRule struct {
	ID          RuleID
	NetworkID   NetworkID
	ContainerID ContainerID // Optional: if set, rule applies only to this container
	ServiceName string      // Service this rule applies to
	Direction   Direction
	Action      Action
	From        string // source: service:name, cidr:x.x.x.x/y, or "internet"
	To          string // destination: service:name, cidr:x.x.x.x/y, or "internet"
	ToPort      string // Port or port range (e.g., "80" or "8000-8100")
	Protocol    Protocol
	Priority    int // Lower priority rules are evaluated first
}

// SecurityPolicy represents a collection of security rules for a network or container.
type SecurityPolicy struct {
	NetworkID   NetworkID
	ContainerID ContainerID // Optional: if set, policy applies only to this container
	Rules       []SecurityRule
	Enabled     bool
}

// PolicyStatus represents the current status of a security policy.
type PolicyStatus struct {
	NetworkID   NetworkID
	ContainerID ContainerID
	Error       string
	RuleCount   int
	Applied     bool
}
