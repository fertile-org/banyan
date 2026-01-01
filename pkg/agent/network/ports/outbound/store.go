package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
)

// EndpointStore persists network endpoint information.
type EndpointStore interface {
	// SaveEndpoint saves an endpoint.
	SaveEndpoint(ctx context.Context, endpoint *domain.Endpoint) error

	// GetEndpoint retrieves an endpoint by container and network ID.
	GetEndpoint(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.Endpoint, error)

	// DeleteEndpoint deletes an endpoint.
	DeleteEndpoint(ctx context.Context, containerID domain.ContainerID, networkID domain.NetworkID) error

	// ListEndpoints lists all endpoints for a container.
	ListEndpoints(ctx context.Context, containerID domain.ContainerID) ([]*domain.Endpoint, error)

	// ListNetworkEndpoints lists all endpoints in a network.
	ListNetworkEndpoints(ctx context.Context, networkID domain.NetworkID) ([]*domain.Endpoint, error)
}

// NetworkStore persists network information.
type NetworkStore interface {
	// GetNetwork retrieves network info by ID.
	GetNetwork(ctx context.Context, networkID domain.NetworkID) (*domain.Network, error)

	// ListNetworks lists all networks.
	ListNetworks(ctx context.Context) ([]*domain.Network, error)
}
