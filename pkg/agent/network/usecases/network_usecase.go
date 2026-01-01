package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/outbound"
)

// NetworkUseCase implements the NetworkService interface.
type NetworkUseCase struct {
	ipAllocator   outbound.IPAllocator
	cniProvider   outbound.CNIProvider
	endpointStore outbound.EndpointStore
	networkStore  outbound.NetworkStore
}

// NewNetworkUseCase creates a new NetworkUseCase.
func NewNetworkUseCase(
	ipAllocator outbound.IPAllocator,
	cniProvider outbound.CNIProvider,
	endpointStore outbound.EndpointStore,
	networkStore outbound.NetworkStore,
) *NetworkUseCase {
	return &NetworkUseCase{
		ipAllocator:   ipAllocator,
		cniProvider:   cniProvider,
		endpointStore: endpointStore,
		networkStore:  networkStore,
	}
}

// ConnectContainer connects a container to a network.
func (uc *NetworkUseCase) ConnectContainer(
	ctx context.Context,
	containerID domain.ContainerID,
	networkID domain.NetworkID,
	opts *inbound.ConnectOptions,
) (*domain.Endpoint, error) {
	if err := uc.validateIDs(containerID, networkID); err != nil {
		return nil, err
	}

	// Ensure opts is not nil
	if opts == nil {
		opts = &inbound.ConnectOptions{}
	}

	// Check if already connected
	existing, err := uc.endpointStore.GetEndpoint(ctx, containerID, networkID)
	if err == nil && existing != nil {
		return nil, domain.ErrAlreadyConnected
	}

	// Get network info
	network, err := uc.networkStore.GetNetwork(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrNetworkNotFound, err)
	}

	// Allocate IP if not provided
	ip := opts.IPAddress
	if ip == nil {
		allocatedIP, err := uc.ipAllocator.AllocateIP(ctx, network.Subnet, string(containerID))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrIPAllocationFailed, err)
		}
		ip = allocatedIP
	}

	// Add container to network via CNI
	if err := uc.cniProvider.AddToNetwork(ctx, string(containerID), string(networkID), ip); err != nil {
		// Release the allocated IP on failure
		if opts.IPAddress == nil {
			_ = uc.ipAllocator.ReleaseIP(ctx, ip)
		}
		return nil, fmt.Errorf("%w: %v", domain.ErrNetworkOperationFailed, err)
	}

	// Create and save endpoint
	endpoint := &domain.Endpoint{
		ContainerID: containerID,
		NetworkID:   networkID,
		IPAddress:   ip,
		MacAddress:  opts.MacAddress,
		Gateway:     network.Gateway,
		Ports:       opts.Ports,
		CreatedAt:   time.Now(),
	}

	if err := uc.endpointStore.SaveEndpoint(ctx, endpoint); err != nil {
		// Cleanup on failure
		_ = uc.cniProvider.RemoveFromNetwork(ctx, string(containerID), string(networkID))
		if opts.IPAddress == nil {
			_ = uc.ipAllocator.ReleaseIP(ctx, ip)
		}
		return nil, fmt.Errorf("failed to save endpoint: %w", err)
	}

	return endpoint, nil
}

// DisconnectContainer disconnects a container from a network.
func (uc *NetworkUseCase) DisconnectContainer(
	ctx context.Context,
	containerID domain.ContainerID,
	networkID domain.NetworkID,
) error {
	if err := uc.validateIDs(containerID, networkID); err != nil {
		return err
	}

	// Get existing endpoint
	endpoint, err := uc.endpointStore.GetEndpoint(ctx, containerID, networkID)
	if err != nil {
		return domain.ErrNotConnected
	}

	// Remove from network via CNI
	if err := uc.cniProvider.RemoveFromNetwork(ctx, string(containerID), string(networkID)); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrNetworkOperationFailed, err)
	}

	// Release IP
	if endpoint.IPAddress != nil {
		if err := uc.ipAllocator.ReleaseIP(ctx, endpoint.IPAddress); err != nil {
			// Log but don't fail - the container is already disconnected
			_ = err
		}
	}

	// Delete endpoint record
	if err := uc.endpointStore.DeleteEndpoint(ctx, containerID, networkID); err != nil {
		return fmt.Errorf("failed to delete endpoint: %w", err)
	}

	return nil
}

// GetEndpoint gets network endpoint information for a container on a specific network.
func (uc *NetworkUseCase) GetEndpoint(
	ctx context.Context,
	containerID domain.ContainerID,
	networkID domain.NetworkID,
) (*domain.Endpoint, error) {
	if err := uc.validateIDs(containerID, networkID); err != nil {
		return nil, err
	}

	endpoint, err := uc.endpointStore.GetEndpoint(ctx, containerID, networkID)
	if err != nil {
		return nil, domain.ErrEndpointNotFound
	}

	return endpoint, nil
}

// ListEndpoints lists all network endpoints for a container.
func (uc *NetworkUseCase) ListEndpoints(
	ctx context.Context,
	containerID domain.ContainerID,
) ([]*domain.Endpoint, error) {
	if containerID == "" {
		return nil, domain.ErrInvalidContainerID
	}

	return uc.endpointStore.ListEndpoints(ctx, containerID)
}

// GetEndpointStatus gets the current status of a container's network connection.
func (uc *NetworkUseCase) GetEndpointStatus(
	ctx context.Context,
	containerID domain.ContainerID,
	networkID domain.NetworkID,
) (*domain.EndpointStatus, error) {
	endpoint, err := uc.GetEndpoint(ctx, containerID, networkID)
	if err != nil {
		return &domain.EndpointStatus{
			State: domain.NetworkStateDisconnected,
			Error: err.Error(),
		}, nil
	}

	return &domain.EndpointStatus{
		State:       domain.NetworkStateConnected,
		IPAddress:   endpoint.IPAddress,
		Gateway:     endpoint.Gateway,
		ConnectedAt: endpoint.CreatedAt,
	}, nil
}

// validateIDs validates container and network IDs.
func (uc *NetworkUseCase) validateIDs(containerID domain.ContainerID, networkID domain.NetworkID) error {
	if containerID == "" {
		return domain.ErrInvalidContainerID
	}
	if networkID == "" {
		return domain.ErrInvalidNetworkID
	}
	return nil
}

// Ensure NetworkUseCase implements inbound.NetworkService
var _ inbound.NetworkService = (*NetworkUseCase)(nil)
