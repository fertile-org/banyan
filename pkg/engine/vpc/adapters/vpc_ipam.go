package adapters

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcerrors "github.com/fertile-org/banyan/pkg/engine/vpc/errors"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

// VPCIPAMManagerAdapter wraps the pkg/vpc IPAMManager.
type VPCIPAMManagerAdapter struct {
	manager *ipam.Manager
	subnets map[string]*domain.Subnet
	allocs  map[string]*domain.IPAllocation // containerID -> allocation
	mu      sync.RWMutex
}

// NewVPCIPAMManagerAdapter creates a new VPCIPAMManagerAdapter.
func NewVPCIPAMManagerAdapter(store storage.StateStore, vpcCIDR string) (*VPCIPAMManagerAdapter, error) {
	manager, err := ipam.NewManager(store, vpcCIDR)
	if err != nil {
		return nil, err
	}

	return &VPCIPAMManagerAdapter{
		manager: manager,
		subnets: make(map[string]*domain.Subnet),
		allocs:  make(map[string]*domain.IPAllocation),
	}, nil
}

// CreateSubnet creates a new subnet within a VPC.
func (a *VPCIPAMManagerAdapter) CreateSubnet(
	ctx context.Context,
	vpcID, cidr string,
	purpose domain.SubnetPurpose,
) (*domain.Subnet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Parse CIDR to calculate available IPs
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, vpcerrors.ErrInvalidCIDR
	}

	// Calculate total IPs (minus network and broadcast)
	ones, bits := ipNet.Mask.Size()
	totalIPs := (1 << (bits - ones)) - 2

	// Calculate gateway (first usable IP)
	gateway := make(net.IP, len(ipNet.IP))
	copy(gateway, ipNet.IP)
	gateway[len(gateway)-1]++

	subnetID := fmt.Sprintf("%s-subnet-%d", vpcID, len(a.subnets)+1)

	subnet := &domain.Subnet{
		ID:           subnetID,
		VPCID:        vpcID,
		CIDR:         cidr,
		Gateway:      gateway.String(),
		AvailableIPs: totalIPs,
		TotalIPs:     totalIPs,
		Purpose:      purpose,
	}

	a.subnets[subnetID] = subnet
	return subnet, nil
}

// GetSubnet retrieves a subnet by ID.
func (a *VPCIPAMManagerAdapter) GetSubnet(_ context.Context, id string) (*domain.Subnet, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subnet, ok := a.subnets[id]
	if !ok {
		return nil, vpcerrors.ErrSubnetNotFound
	}
	return subnet, nil
}

// DeleteSubnet deletes a subnet.
func (a *VPCIPAMManagerAdapter) DeleteSubnet(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.subnets, id)
	return nil
}

// AllocateIP allocates an IP from a subnet.
func (a *VPCIPAMManagerAdapter) AllocateIP(
	ctx context.Context,
	subnetID, containerID string,
) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	subnet, ok := a.subnets[subnetID]
	if !ok {
		return "", vpcerrors.ErrSubnetNotFound
	}

	if subnet.AvailableIPs <= 0 {
		return "", vpcerrors.ErrIPExhausted
	}

	// Parse subnet CIDR
	_, ipNet, err := net.ParseCIDR(subnet.CIDR)
	if err != nil {
		return "", err
	}

	// Allocate from underlying IPAM manager
	ip, err := a.manager.AllocateIP(ctx, ipNet)
	if err != nil {
		return "", err
	}

	// Update availability
	subnet.AvailableIPs--

	// Track allocation
	a.allocs[containerID] = &domain.IPAllocation{
		IP:          ip.String(),
		SubnetID:    subnetID,
		ContainerID: containerID,
	}

	return ip.String(), nil
}

// ReleaseIP releases an allocated IP.
func (a *VPCIPAMManagerAdapter) ReleaseIP(ctx context.Context, ipStr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", ipStr)
	}

	// Find and remove allocation
	for containerID, alloc := range a.allocs {
		if alloc.IP == ipStr {
			delete(a.allocs, containerID)

			// Update subnet availability
			if subnet, ok := a.subnets[alloc.SubnetID]; ok {
				subnet.AvailableIPs++
			}
			break
		}
	}

	return a.manager.ReleaseIP(ctx, ip)
}

// GetIPAllocation retrieves the IP allocation for a container.
func (a *VPCIPAMManagerAdapter) GetIPAllocation(_ context.Context, containerID string) (*domain.IPAllocation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alloc, ok := a.allocs[containerID]
	if !ok {
		return nil, vpcerrors.ErrAllocationNotFound
	}
	return alloc, nil
}

// GetSubnetCapacity returns the capacity of a subnet.
func (a *VPCIPAMManagerAdapter) GetSubnetCapacity(_ context.Context, subnetID string) (available, total int, err error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subnet, ok := a.subnets[subnetID]
	if !ok {
		return 0, 0, vpcerrors.ErrSubnetNotFound
	}
	return subnet.AvailableIPs, subnet.TotalIPs, nil
}

// Ensure VPCIPAMManagerAdapter implements IPAMManager.
var _ outbound.IPAMManager = (*VPCIPAMManagerAdapter)(nil)

// MemoryIPAMManagerAdapter is an in-memory implementation for testing.
type MemoryIPAMManagerAdapter struct {
	subnets map[string]*domain.Subnet
	allocs  map[string]*domain.IPAllocation
	nextIP  map[string]int // subnetID -> next IP offset
	mu      sync.RWMutex
}

// NewMemoryIPAMManagerAdapter creates a new in-memory IPAM manager.
func NewMemoryIPAMManagerAdapter() *MemoryIPAMManagerAdapter {
	return &MemoryIPAMManagerAdapter{
		subnets: make(map[string]*domain.Subnet),
		allocs:  make(map[string]*domain.IPAllocation),
		nextIP:  make(map[string]int),
	}
}

// CreateSubnet creates a new subnet in memory.
func (a *MemoryIPAMManagerAdapter) CreateSubnet(
	_ context.Context,
	vpcID, cidr string,
	purpose domain.SubnetPurpose,
) (*domain.Subnet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, vpcerrors.ErrInvalidCIDR
	}

	ones, bits := ipNet.Mask.Size()
	totalIPs := (1 << (bits - ones)) - 2

	gateway := make(net.IP, len(ipNet.IP))
	copy(gateway, ipNet.IP)
	gateway[len(gateway)-1]++

	subnetID := fmt.Sprintf("%s-subnet-%d", vpcID, len(a.subnets)+1)

	subnet := &domain.Subnet{
		ID:           subnetID,
		VPCID:        vpcID,
		CIDR:         cidr,
		Gateway:      gateway.String(),
		AvailableIPs: totalIPs,
		TotalIPs:     totalIPs,
		Purpose:      purpose,
	}

	a.subnets[subnetID] = subnet
	a.nextIP[subnetID] = 2 // Start after gateway
	return subnet, nil
}

// GetSubnet retrieves a subnet by ID.
func (a *MemoryIPAMManagerAdapter) GetSubnet(_ context.Context, id string) (*domain.Subnet, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subnet, ok := a.subnets[id]
	if !ok {
		return nil, vpcerrors.ErrSubnetNotFound
	}
	return subnet, nil
}

// DeleteSubnet deletes a subnet.
func (a *MemoryIPAMManagerAdapter) DeleteSubnet(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.subnets, id)
	delete(a.nextIP, id)
	return nil
}

// AllocateIP allocates an IP from a subnet.
func (a *MemoryIPAMManagerAdapter) AllocateIP(
	_ context.Context,
	subnetID, containerID string,
) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	subnet, ok := a.subnets[subnetID]
	if !ok {
		return "", vpcerrors.ErrSubnetNotFound
	}

	if subnet.AvailableIPs <= 0 {
		return "", vpcerrors.ErrIPExhausted
	}

	_, ipNet, _ := net.ParseCIDR(subnet.CIDR)
	offset := a.nextIP[subnetID]

	ip := make(net.IP, len(ipNet.IP))
	copy(ip, ipNet.IP)
	ip[len(ip)-1] += byte(offset)

	a.nextIP[subnetID]++
	subnet.AvailableIPs--

	a.allocs[containerID] = &domain.IPAllocation{
		IP:          ip.String(),
		SubnetID:    subnetID,
		ContainerID: containerID,
	}

	return ip.String(), nil
}

// ReleaseIP releases an allocated IP.
func (a *MemoryIPAMManagerAdapter) ReleaseIP(_ context.Context, ipStr string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for containerID, alloc := range a.allocs {
		if alloc.IP == ipStr {
			delete(a.allocs, containerID)
			if subnet, ok := a.subnets[alloc.SubnetID]; ok {
				subnet.AvailableIPs++
			}
			break
		}
	}
	return nil
}

// GetIPAllocation retrieves the IP allocation for a container.
func (a *MemoryIPAMManagerAdapter) GetIPAllocation(_ context.Context, containerID string) (*domain.IPAllocation, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	alloc, ok := a.allocs[containerID]
	if !ok {
		return nil, vpcerrors.ErrAllocationNotFound
	}
	return alloc, nil
}

// GetSubnetCapacity returns the capacity of a subnet.
func (a *MemoryIPAMManagerAdapter) GetSubnetCapacity(_ context.Context, subnetID string) (available, total int, err error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	subnet, ok := a.subnets[subnetID]
	if !ok {
		return 0, 0, vpcerrors.ErrSubnetNotFound
	}
	return subnet.AvailableIPs, subnet.TotalIPs, nil
}

// Ensure MemoryIPAMManagerAdapter implements IPAMManager.
var _ outbound.IPAMManager = (*MemoryIPAMManagerAdapter)(nil)
