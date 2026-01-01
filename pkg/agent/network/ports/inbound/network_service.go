package inbound

import (
	"context"
	"net"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
)

// NetworkService is the inbound port for network operations.
// It provides the interface that other agent components use to manage container networking.
type NetworkService interface {
	// ConnectContainer connects a container to a network with optional IP assignment.
	ConnectContainer(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID, opts *ConnectOptions) (*domain.Endpoint, error)

	// DisconnectContainer disconnects a container from a network.
	DisconnectContainer(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) error

	// GetEndpoint gets network endpoint information for a container on a specific network.
	GetEndpoint(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.Endpoint, error)

	// ListEndpoints lists all network endpoints for a container.
	ListEndpoints(ctx context.Context, containerID domain.ContainerID) ([]*domain.Endpoint, error)

	// GetEndpointStatus gets the current status of a container's network connection.
	GetEndpointStatus(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.EndpointStatus, error)
}

// ConnectOptions contains options for connecting a container to a network.
type ConnectOptions struct {
	MacAddress string
	IPAddress  net.IP
	Ports      []domain.PortMapping
	Aliases    []string
}
