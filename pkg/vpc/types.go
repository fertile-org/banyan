package vpc

import (
	"net"
	"time"
)

// Network represents a VPC network
type Network struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CIDR      string    `json:"cidr"`
	VxlanID   int       `json:"vxlan_id"`
	DNSSuffix string    `json:"dns_suffix"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // active, pending, error
}

// NetworkConfig is used to create a network
type NetworkConfig struct {
	Name      string `json:"name"`
	CIDR      string `json:"cidr"`       // Default: 10.0.0.0/16
	VxlanID   int    `json:"vxlan_id"`   // Optional, auto-assigned if 0
	DNSSuffix string `json:"dns_suffix"` // Default: .internal
	Driver    string `json:"driver"`     // flannel, calico, etc. Default: flannel
}

// SecurityRule represents a network security rule
type SecurityRule struct {
	ID          string `json:"id"`
	NetworkID   string `json:"network_id"`
	ServiceName string `json:"service_name"` // Service this rule applies to
	Direction   string `json:"direction"`    // ingress or egress
	Action      string `json:"action"`       // allow or deny
	From        string `json:"from"`         // service:name, cidr:x.x.x.x/y, or internet
	To          string `json:"to"`           // service:name, cidr:x.x.x.x/y, or internet
	ToPort      string `json:"to_port"`      // Port or port range (e.g., "80" or "8000-8100")
	Protocol    string `json:"protocol"`     // tcp, udp, icmp, or "" for all
}

// Container represents a container attached to the network
type Container struct {
	ID        string    `json:"id"`
	NetworkID string    `json:"network_id"`
	IP        net.IP    `json:"ip"`
	HostID    string    `json:"host_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// PluginStatus represents the status of a CNI plugin
type PluginStatus struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"` // active, inactive, error
	Error   string `json:"error,omitempty"`
}

// SubnetLease represents a subnet allocated to a host
type SubnetLease struct {
	HostID    string     `json:"host_id"`
	Subnet    *net.IPNet `json:"subnet"`
	LeaseTime time.Time  `json:"lease_time"`
	ExpiresAt time.Time  `json:"expires_at"`
}

// TraceHop represents a single hop in a connection trace
type TraceHop struct {
	Type    string `json:"type"`    // gateway, vxlan, container, etc.
	Address string `json:"address"` // IP or hostname
	Latency string `json:"latency,omitempty"`
}

// TraceResult contains the result of a connection trace
type TraceResult struct {
	FromIP           net.IP     `json:"from_ip"`
	ToIP             net.IP     `json:"to_ip"`
	Port             int        `json:"port"`
	Status           string     `json:"status"` // allowed, blocked, error
	Reachable        bool       `json:"reachable"`
	Hops             []TraceHop `json:"hops"`
	BlockedBy        string     `json:"blocked_by,omitempty"`         // Which rule blocked it
	BlockedByDetails string     `json:"blocked_by_details,omitempty"` // Details about blocking rule
	AllowedBy        string     `json:"allowed_by,omitempty"`         // Which rule allowed it
	Latency          string     `json:"latency,omitempty"`
	Error            string     `json:"error,omitempty"`
}

// ConnectivityResult represents connectivity check results
type ConnectivityResult struct {
	ContainerID    string   `json:"container_id"`
	Status         string   `json:"status"` // ok, degraded, error, unreachable
	HasNetwork     bool     `json:"has_network"`
	IP             net.IP   `json:"ip,omitempty"`
	Gateway        net.IP   `json:"gateway,omitempty"`
	DNS            []net.IP `json:"dns,omitempty"`
	InternetAccess bool     `json:"internet_access"` // Can reach internet
	DefaultRoute   string   `json:"default_route,omitempty"`
	ExternalPing   bool     `json:"external_ping"` // Can reach internet
	InternalPing   bool     `json:"internal_ping"` // Can reach other containers
	Errors         []string `json:"errors,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// AttachOptions contains options for attaching a container to network
type AttachOptions struct {
	IP     net.IP `json:"ip,omitempty"` // Specific IP to assign
	HostID string `json:"host_id"`      // Which host the container is on
}
