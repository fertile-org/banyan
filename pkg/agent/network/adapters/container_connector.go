package adapters

import (
	"context"
	"net"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"

	containerOutbound "github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// ContainerNetworkConnector implements the container module's NetworkConnector interface.
// It bridges the container module with the network module.
type ContainerNetworkConnector struct {
	networkService inbound.NetworkService
}

// NewContainerNetworkConnector creates a new ContainerNetworkConnector.
func NewContainerNetworkConnector(networkService inbound.NetworkService) *ContainerNetworkConnector {
	return &ContainerNetworkConnector{
		networkService: networkService,
	}
}

// ConnectContainer connects a container to a network with the given IP.
func (c *ContainerNetworkConnector) ConnectContainer(
	ctx context.Context,
	containerID, networkID, ip string,
) error {
	opts := &inbound.ConnectOptions{}

	if ip != "" {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			return domain.ErrInvalidIPAddress
		}
		opts.IPAddress = parsedIP
	}

	_, err := c.networkService.ConnectContainer(
		ctx,
		domain.ContainerID(containerID),
		domain.NetworkID(networkID),
		opts,
	)
	return err
}

// DisconnectContainer disconnects a container from a network.
func (c *ContainerNetworkConnector) DisconnectContainer(
	ctx context.Context,
	containerID, networkID string,
) error {
	return c.networkService.DisconnectContainer(
		ctx,
		domain.ContainerID(containerID),
		domain.NetworkID(networkID),
	)
}

// GetContainerNetwork returns network information for a container.
func (c *ContainerNetworkConnector) GetContainerNetwork(
	ctx context.Context,
	containerID string,
) (*containerOutbound.ContainerNetworkInfo, error) {
	endpoints, err := c.networkService.ListEndpoints(ctx, domain.ContainerID(containerID))
	if err != nil {
		return nil, err
	}

	if len(endpoints) == 0 {
		return nil, domain.ErrEndpointNotFound
	}

	// Return info for the first network (primary)
	endpoint := endpoints[0]

	ports := make([]containerOutbound.ContainerPort, len(endpoint.Ports))
	for i, p := range endpoint.Ports {
		ports[i] = containerOutbound.ContainerPort{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			HostIP:        p.HostIP,
			Protocol:      p.Protocol,
		}
	}

	return &containerOutbound.ContainerNetworkInfo{
		NetworkID:  string(endpoint.NetworkID),
		IPAddress:  endpoint.IPAddress.String(),
		Gateway:    endpoint.Gateway.String(),
		MacAddress: endpoint.MacAddress,
		Ports:      ports,
	}, nil
}

// Ensure ContainerNetworkConnector implements containerOutbound.NetworkConnector
var _ containerOutbound.NetworkConnector = (*ContainerNetworkConnector)(nil)
