package outbound

import "context"

// NetworkConnector interfaces with the Network Node for container networking.
// This is an outbound port that use cases use to manage container network connections.
type NetworkConnector interface {
	// ConnectContainer connects a container to a network with the given IP.
	ConnectContainer(ctx context.Context, containerID string, networkID string, ip string) error

	// DisconnectContainer disconnects a container from a network.
	DisconnectContainer(ctx context.Context, containerID string, networkID string) error

	// GetContainerNetwork returns network information for a container.
	GetContainerNetwork(ctx context.Context, containerID string) (*ContainerNetworkInfo, error)
}

// ContainerNetworkInfo contains network information for a container.
type ContainerNetworkInfo struct {
	NetworkID  string
	IPAddress  string
	Gateway    string
	MacAddress string
	Ports      []ContainerPort
}

// ContainerPort represents a port mapping.
type ContainerPort struct {
	Protocol      string
	HostIP        string
	ContainerPort uint16
	HostPort      uint16
}
