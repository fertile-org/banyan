package adapters

import (
	"context"
	"net"
	"sync"

	"github.com/fertile-org/banyan/pkg/agent/network/domain"
	"github.com/fertile-org/banyan/pkg/agent/network/ports/outbound"
)

// MemoryEndpointStore implements EndpointStore with in-memory storage.
type MemoryEndpointStore struct {
	endpoints map[string]*domain.Endpoint
	mu        sync.RWMutex
}

// NewMemoryEndpointStore creates a new MemoryEndpointStore.
func NewMemoryEndpointStore() *MemoryEndpointStore {
	return &MemoryEndpointStore{
		endpoints: make(map[string]*domain.Endpoint),
	}
}

func (s *MemoryEndpointStore) key(containerID domain.ContainerID, networkID domain.NetworkID) string {
	return string(containerID) + ":" + string(networkID)
}

// SaveEndpoint saves an endpoint.
func (s *MemoryEndpointStore) SaveEndpoint(_ context.Context, endpoint *domain.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.endpoints[s.key(endpoint.ContainerID, endpoint.NetworkID)] = endpoint
	return nil
}

// GetEndpoint retrieves an endpoint by container and network ID.
func (s *MemoryEndpointStore) GetEndpoint(_ context.Context, containerID domain.ContainerID, networkID domain.NetworkID) (*domain.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	endpoint, ok := s.endpoints[s.key(containerID, networkID)]
	if !ok {
		return nil, domain.ErrEndpointNotFound
	}
	return endpoint, nil
}

// DeleteEndpoint deletes an endpoint.
func (s *MemoryEndpointStore) DeleteEndpoint(_ context.Context, containerID domain.ContainerID, networkID domain.NetworkID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.endpoints, s.key(containerID, networkID))
	return nil
}

// ListEndpoints lists all endpoints for a container.
func (s *MemoryEndpointStore) ListEndpoints(_ context.Context, containerID domain.ContainerID) ([]*domain.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.Endpoint
	for _, ep := range s.endpoints {
		if ep.ContainerID == containerID {
			result = append(result, ep)
		}
	}
	return result, nil
}

// ListNetworkEndpoints lists all endpoints in a network.
func (s *MemoryEndpointStore) ListNetworkEndpoints(_ context.Context, networkID domain.NetworkID) ([]*domain.Endpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.Endpoint
	for _, ep := range s.endpoints {
		if ep.NetworkID == networkID {
			result = append(result, ep)
		}
	}
	return result, nil
}

// Ensure MemoryEndpointStore implements outbound.EndpointStore
var _ outbound.EndpointStore = (*MemoryEndpointStore)(nil)

// MemoryNetworkStore implements NetworkStore with in-memory storage.
type MemoryNetworkStore struct {
	networks map[domain.NetworkID]*domain.Network
	mu       sync.RWMutex
}

// NewMemoryNetworkStore creates a new MemoryNetworkStore.
func NewMemoryNetworkStore() *MemoryNetworkStore {
	return &MemoryNetworkStore{
		networks: make(map[domain.NetworkID]*domain.Network),
	}
}

// AddNetwork adds a network to the store.
func (s *MemoryNetworkStore) AddNetwork(network *domain.Network) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.networks[network.ID] = network
}

// GetNetwork retrieves network info by ID.
func (s *MemoryNetworkStore) GetNetwork(_ context.Context, networkID domain.NetworkID) (*domain.Network, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	network, ok := s.networks[networkID]
	if !ok {
		return nil, domain.ErrNetworkNotFound
	}
	return network, nil
}

// ListNetworks lists all networks.
func (s *MemoryNetworkStore) ListNetworks(_ context.Context) ([]*domain.Network, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*domain.Network, 0, len(s.networks))
	for _, n := range s.networks {
		result = append(result, n)
	}
	return result, nil
}

// Ensure MemoryNetworkStore implements outbound.NetworkStore
var _ outbound.NetworkStore = (*MemoryNetworkStore)(nil)

// DefaultNetworkStore creates a MemoryNetworkStore with a default VPC network.
func DefaultNetworkStore(subnet *net.IPNet, gateway net.IP) *MemoryNetworkStore {
	store := NewMemoryNetworkStore()
	store.AddNetwork(&domain.Network{
		ID:      "vpc-default",
		Name:    "vpc-default",
		Subnet:  subnet,
		Gateway: gateway,
	})
	return store
}
