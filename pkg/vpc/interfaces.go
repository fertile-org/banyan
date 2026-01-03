package vpc

import (
	"context"
	"net"
)

// NetworkManager manages VPC networks
type NetworkManager interface {
	// CreateNetwork creates a new VPC network
	CreateNetwork(ctx context.Context, config NetworkConfig) (*Network, error)

	// DeleteNetwork removes a network
	DeleteNetwork(ctx context.Context, networkID string) error

	// GetNetwork retrieves network information
	GetNetwork(ctx context.Context, networkID string) (*Network, error)

	// ListNetworks lists all networks
	ListNetworks(ctx context.Context) ([]*Network, error)
}

// CNIRuntime manages CNI plugin operations
type CNIRuntime interface {
	// AddToNetwork adds a container to a network
	AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error

	// RemoveFromNetwork removes a container from a network
	RemoveFromNetwork(ctx context.Context, containerID, networkID string) error

	// SetupPlugin initializes a CNI plugin
	SetupPlugin(ctx context.Context, plugin string, config []byte) error

	// GetPluginStatus returns the status of a CNI plugin
	GetPluginStatus(ctx context.Context, plugin string) (*PluginStatus, error)
}

// CNIRuntimeWithServiceRegistry extends CNIRuntime with service discovery support
type CNIRuntimeWithServiceRegistry interface {
	CNIRuntime

	// AddToNetworkWithService adds a container to a network and registers it with a service
	AddToNetworkWithService(ctx context.Context, containerID, networkID string, ip net.IP, serviceName string) error
}

// IPAMManager manages IP address allocation
type IPAMManager interface {
	// AllocateHostSubnet allocates a /24 subnet for a host
	AllocateHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error)

	// AllocateIP allocates an IP from a subnet
	AllocateIP(ctx context.Context, subnet *net.IPNet) (net.IP, error)

	// ReleaseIP releases an allocated IP
	ReleaseIP(ctx context.Context, ip net.IP) error

	// RenewLease renews the lease for a host's subnet
	RenewLease(ctx context.Context, hostID string) error

	// GetHostSubnet returns the subnet allocated to a host
	GetHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error)
}

// SecurityManager manages security rules
type SecurityManager interface {
	// AddRule adds a security rule
	AddRule(ctx context.Context, rule *SecurityRule) error

	// RemoveRule removes a security rule
	RemoveRule(ctx context.Context, ruleID string) error

	// ListRules lists all rules for a network
	ListRules(ctx context.Context, networkID string) ([]*SecurityRule, error)

	// ApplyRules applies rules to the system (iptables)
	ApplyRules(ctx context.Context, networkID string) error
}

// DNSManager manages DNS operations
type DNSManager interface {
	// RegisterHost registers a hostname with an IP
	RegisterHost(ctx context.Context, hostname string, ip net.IP) error

	// UnregisterHost removes a hostname registration
	UnregisterHost(ctx context.Context, hostname string) error

	// LookupHost resolves a hostname to IPs
	LookupHost(ctx context.Context, hostname string) ([]net.IP, error)

	// UpdateHealth updates the health status of a host
	UpdateHealth(ctx context.Context, hostname string, healthy bool) error
}

// DebugManager provides debugging utilities
type DebugManager interface {
	// TraceConnection traces connectivity between two IPs
	TraceConnection(ctx context.Context, fromIP, toIP net.IP, port int) (*TraceResult, error)

	// CheckConnectivity checks if a container has network connectivity
	CheckConnectivity(ctx context.Context, containerID string) (*ConnectivityResult, error)

	// GetIPTablesRules returns iptables rules for an IP
	GetIPTablesRules(ctx context.Context, ip net.IP) ([]string, error)
}
