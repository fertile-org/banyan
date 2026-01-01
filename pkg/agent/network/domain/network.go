package domain

import (
	"net"
	"time"
)

// NetworkID is a unique identifier for a network.
type NetworkID string

// ContainerID is a unique identifier for a container.
type ContainerID string

// Network represents a container network.
type Network struct {
	CreatedAt time.Time
	Subnet    *net.IPNet
	Labels    map[string]string
	ID        NetworkID
	Name      string
	Gateway   net.IP
	VxlanID   uint32
}

// Endpoint represents a container's network endpoint.
type Endpoint struct {
	CreatedAt   time.Time
	ContainerID ContainerID
	NetworkID   NetworkID
	MacAddress  string
	IPAddress   net.IP
	Gateway     net.IP
	Ports       []PortMapping
}

// PortMapping represents a port mapping between container and host.
type PortMapping struct {
	HostIP        string
	Protocol      string
	ContainerPort uint16
	HostPort      uint16
}

// NetworkState represents the state of a network connection.
type NetworkState string

const (
	NetworkStateConnecting   NetworkState = "connecting"
	NetworkStateConnected    NetworkState = "connected"
	NetworkStateDisconnected NetworkState = "disconnected"
	NetworkStateFailed       NetworkState = "failed"
)

// EndpointStatus represents the current status of a network endpoint.
type EndpointStatus struct {
	ConnectedAt time.Time
	State       NetworkState
	Error       string
	IPAddress   net.IP
	Gateway     net.IP
}
