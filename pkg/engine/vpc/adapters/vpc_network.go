package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcerrors "github.com/fertile-org/banyan/pkg/engine/vpc/errors"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/network"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

// VPCNetworkManagerAdapter wraps the pkg/vpc NetworkManager.
type VPCNetworkManagerAdapter struct {
	manager *network.Manager
	vpcs    map[string]*domain.VPC // In-memory VPC cache for additional metadata
	mu      sync.RWMutex
}

// NewVPCNetworkManagerAdapter creates a new VPCNetworkManagerAdapter.
func NewVPCNetworkManagerAdapter(store storage.StateStore) *VPCNetworkManagerAdapter {
	return &VPCNetworkManagerAdapter{
		manager: network.NewManager(store),
		vpcs:    make(map[string]*domain.VPC),
	}
}

// CreateVPC creates a new VPC.
func (a *VPCNetworkManagerAdapter) CreateVPC(
	ctx context.Context,
	name, cidr string,
	metadata map[string]string,
) (*domain.VPC, error) {
	config := vpc.NetworkConfig{
		Name:      name,
		CIDR:      cidr,
		DNSSuffix: "banyan.local",
	}

	network, err := a.manager.CreateNetwork(ctx, config)
	if err != nil {
		return nil, err
	}

	vpcEntity := &domain.VPC{
		ID:        network.ID,
		Name:      network.Name,
		CIDR:      network.CIDR,
		Status:    domain.VPCStatusActive,
		CreatedAt: network.CreatedAt,
		Metadata:  metadata,
	}

	a.mu.Lock()
	a.vpcs[network.ID] = vpcEntity
	a.mu.Unlock()

	return vpcEntity, nil
}

// GetVPC retrieves a VPC by ID.
func (a *VPCNetworkManagerAdapter) GetVPC(ctx context.Context, id string) (*domain.VPC, error) {
	network, err := a.manager.GetNetwork(ctx, id)
	if err != nil {
		return nil, vpcerrors.ErrVPCNotFound
	}

	a.mu.RLock()
	cachedVPC, exists := a.vpcs[id]
	a.mu.RUnlock()

	if exists {
		// Update from network manager and return cached metadata
		cachedVPC.Name = network.Name
		cachedVPC.CIDR = network.CIDR
		return cachedVPC, nil
	}

	return &domain.VPC{
		ID:        network.ID,
		Name:      network.Name,
		CIDR:      network.CIDR,
		Status:    domain.VPCStatusActive,
		CreatedAt: network.CreatedAt,
	}, nil
}

// DeleteVPC deletes a VPC.
func (a *VPCNetworkManagerAdapter) DeleteVPC(ctx context.Context, id string) error {
	if err := a.manager.DeleteNetwork(ctx, id); err != nil {
		return err
	}

	a.mu.Lock()
	delete(a.vpcs, id)
	a.mu.Unlock()

	return nil
}

// ListVPCs lists all VPCs.
func (a *VPCNetworkManagerAdapter) ListVPCs(ctx context.Context) ([]*domain.VPC, error) {
	networks, err := a.manager.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}

	vpcs := make([]*domain.VPC, len(networks))
	for i, network := range networks {
		a.mu.RLock()
		cachedVPC, exists := a.vpcs[network.ID]
		a.mu.RUnlock()

		if exists {
			vpcs[i] = cachedVPC
		} else {
			vpcs[i] = &domain.VPC{
				ID:        network.ID,
				Name:      network.Name,
				CIDR:      network.CIDR,
				Status:    domain.VPCStatusActive,
				CreatedAt: network.CreatedAt,
			}
		}
	}

	return vpcs, nil
}


// AddSubnetToVPC adds a subnet reference to a VPC.
func (a *VPCNetworkManagerAdapter) AddSubnetToVPC(vpcID string, subnet domain.Subnet) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	vpc, ok := a.vpcs[vpcID]
	if !ok {
		return vpcerrors.ErrVPCNotFound
	}
	vpc.Subnets = append(vpc.Subnets, subnet)
	return nil
}

// Ensure VPCNetworkManagerAdapter implements NetworkManager.
var _ outbound.NetworkManager = (*VPCNetworkManagerAdapter)(nil)

// MemoryNetworkManagerAdapter is an in-memory implementation for testing.
type MemoryNetworkManagerAdapter struct {
	vpcs   map[string]*domain.VPC
	mu     sync.RWMutex
	nextID int
}

// NewMemoryNetworkManagerAdapter creates a new in-memory network manager.
func NewMemoryNetworkManagerAdapter() *MemoryNetworkManagerAdapter {
	return &MemoryNetworkManagerAdapter{
		vpcs: make(map[string]*domain.VPC),
	}
}

// CreateVPC creates a new VPC in memory.
func (a *MemoryNetworkManagerAdapter) CreateVPC(
	_ context.Context,
	name, cidr string,
	metadata map[string]string,
) (*domain.VPC, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.nextID++
	id := fmt.Sprintf("vpc-%d", a.nextID)

	vpc := &domain.VPC{
		ID:        id,
		Name:      name,
		CIDR:      cidr,
		Status:    domain.VPCStatusActive,
		CreatedAt: time.Now(),
		Metadata:  metadata,
	}

	a.vpcs[id] = vpc
	return vpc, nil
}

// GetVPC retrieves a VPC by ID.
func (a *MemoryNetworkManagerAdapter) GetVPC(_ context.Context, id string) (*domain.VPC, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	vpc, ok := a.vpcs[id]
	if !ok {
		return nil, vpcerrors.ErrVPCNotFound
	}
	return vpc, nil
}

// DeleteVPC deletes a VPC.
func (a *MemoryNetworkManagerAdapter) DeleteVPC(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.vpcs, id)
	return nil
}

// ListVPCs lists all VPCs.
func (a *MemoryNetworkManagerAdapter) ListVPCs(_ context.Context) ([]*domain.VPC, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	vpcs := make([]*domain.VPC, 0, len(a.vpcs))
	for _, vpc := range a.vpcs {
		vpcs = append(vpcs, vpc)
	}
	return vpcs, nil
}

// AddSubnetToVPC adds a subnet reference to a VPC (for testing).
func (a *MemoryNetworkManagerAdapter) AddSubnetToVPC(vpcID string, subnet domain.Subnet) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	vpc, ok := a.vpcs[vpcID]
	if !ok {
		return vpcerrors.ErrVPCNotFound
	}
	vpc.Subnets = append(vpc.Subnets, subnet)
	return nil
}

// Ensure MemoryNetworkManagerAdapter implements NetworkManager.
var _ outbound.NetworkManager = (*MemoryNetworkManagerAdapter)(nil)
