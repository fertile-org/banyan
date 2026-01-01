package domain

// ContainerNetworkRequest specifies network requirements for a container.
type ContainerNetworkRequest struct {
	ContainerID    string
	ServiceName    string
	Namespace      string
	VPCID          string
	SubnetID       string // Optional, auto-select if empty
	SecurityGroups []string
	Labels         map[string]string
}

// ContainerNetwork represents allocated network resources for a container.
type ContainerNetwork struct {
	ContainerID    string
	IPAddress      string
	Gateway        string
	SubnetCIDR     string
	MAC            string
	DNS            []string
	SecurityGroups []string
	VPCID          string
	SubnetID       string
}

// ContainerNetworkUpdate specifies updates to container network.
type ContainerNetworkUpdate struct {
	SecurityGroups []string // New security groups to apply
}

// NetworkProvisionSpec specifies network provisioning requirements.
type NetworkProvisionSpec struct {
	Name           string
	CIDR           string
	Subnets        []SubnetSpec
	SecurityGroups []SecurityGroupSpec
	DNSServers     []string
	Metadata       map[string]string
}

// SubnetSpec specifies a subnet to create.
type SubnetSpec struct {
	Name    string
	CIDR    string
	Purpose SubnetPurpose
}

// SecurityGroupSpec specifies a security group to create.
type SecurityGroupSpec struct {
	Name  string
	Rules []SecurityRule
}

// NetworkInfo contains summary information about a provisioned network.
type NetworkInfo struct {
	VPCID          string
	Name           string
	CIDR           string
	Status         VPCStatus
	Subnets        []SubnetInfo
	SecurityGroups []string
	AvailableIPs   int
	AllocatedIPs   int
}

// SubnetInfo contains summary information about a subnet.
type SubnetInfo struct {
	SubnetID     string
	CIDR         string
	Purpose      SubnetPurpose
	AvailableIPs int
	AllocatedIPs int
}

// NetworkPolicy defines network-level access control.
type NetworkPolicy struct {
	Name        string
	Namespace   string
	PodSelector map[string]string // Label selector
	Ingress     []NetworkPolicyRule
	Egress      []NetworkPolicyRule
}

// NetworkPolicyRule defines a single network policy rule.
type NetworkPolicyRule struct {
	From  []NetworkPolicyPeer
	To    []NetworkPolicyPeer
	Ports []PortRange
}

// NetworkPolicyPeer defines a source or destination for network policy.
type NetworkPolicyPeer struct {
	PodSelector       map[string]string
	NamespaceSelector map[string]string
	IPBlock           string
}

// IPAllocation represents an IP allocation record.
type IPAllocation struct {
	IP          string
	SubnetID    string
	ContainerID string
}
