package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcerrors "github.com/fertile-org/banyan/pkg/engine/vpc/errors"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
)

// MemoryContainerNetworkStoreAdapter is an in-memory implementation of ContainerNetworkStore.
type MemoryContainerNetworkStoreAdapter struct {
	networks map[string]*domain.ContainerNetwork
	mu       sync.RWMutex
}

// NewMemoryContainerNetworkStoreAdapter creates a new in-memory container network store.
func NewMemoryContainerNetworkStoreAdapter() *MemoryContainerNetworkStoreAdapter {
	return &MemoryContainerNetworkStoreAdapter{
		networks: make(map[string]*domain.ContainerNetwork),
	}
}

// Save saves a container network allocation.
func (s *MemoryContainerNetworkStoreAdapter) Save(_ context.Context, network *domain.ContainerNetwork) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep copy to prevent external mutations
	networkCopy := *network
	s.networks[network.ContainerID] = &networkCopy
	return nil
}

// Get retrieves a container network allocation by container ID.
func (s *MemoryContainerNetworkStoreAdapter) Get(_ context.Context, containerID string) (*domain.ContainerNetwork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	network, ok := s.networks[containerID]
	if !ok {
		return nil, vpcerrors.ErrAllocationNotFound
	}

	// Return a copy
	networkCopy := *network
	return &networkCopy, nil
}

// Delete deletes a container network allocation.
func (s *MemoryContainerNetworkStoreAdapter) Delete(_ context.Context, containerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.networks, containerID)
	return nil
}

// FindByVPC finds all container networks in a VPC.
func (s *MemoryContainerNetworkStoreAdapter) FindByVPC(_ context.Context, vpcID string) ([]domain.ContainerNetwork, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var networks []domain.ContainerNetwork
	for _, network := range s.networks {
		if network.VPCID == vpcID {
			networks = append(networks, *network)
		}
	}
	return networks, nil
}

// Ensure MemoryContainerNetworkStoreAdapter implements ContainerNetworkStore.
var _ outbound.ContainerNetworkStore = (*MemoryContainerNetworkStoreAdapter)(nil)
