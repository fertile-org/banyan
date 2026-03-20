package overlay

import (
	"context"
	"encoding/binary"
	"fmt"
	"maps"
	"net"
	"sync"
)

// SubnetAllocatorInterface is the interface for subnet allocation.
// Both in-memory (SubnetAllocator) and etcd-backed implementations satisfy this.
type SubnetAllocatorInterface interface {
	Allocate(ctx context.Context, agentName string) (*net.IPNet, error)
	Release(ctx context.Context, agentName string)
	GetAll(ctx context.Context) map[string]*net.IPNet
}

// SubnetAllocator assigns /24 subnets to agents from a VPC CIDR.
type SubnetAllocator struct {
	vpcCIDR   *net.IPNet
	subnetLen int                   // prefix length for allocated subnets (default 24)
	allocated map[string]*net.IPNet // agentName → subnet
	used      map[string]bool       // subnet string → true (for fast conflict check)
	mu        sync.Mutex
}

// NewSubnetAllocator creates a new allocator for the given VPC CIDR.
// The VPC CIDR must be smaller than /24 (e.g., /16, /20).
func NewSubnetAllocator(vpcCIDR string) (*SubnetAllocator, error) {
	_, cidr, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid VPC CIDR %q: %w", vpcCIDR, err)
	}

	ones, _ := cidr.Mask.Size()
	if ones >= 24 {
		return nil, fmt.Errorf("VPC CIDR %q is too small (must be larger than /24)", vpcCIDR)
	}

	return &SubnetAllocator{
		vpcCIDR:   cidr,
		subnetLen: 24,
		allocated: make(map[string]*net.IPNet),
		used:      make(map[string]bool),
	}, nil
}

// Allocate assigns a /24 subnet to the given agent. Idempotent — returns the
// same subnet if the agent already has one.
func (a *SubnetAllocator) Allocate(_ context.Context, agentName string) (*net.IPNet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Idempotent: return existing allocation
	if subnet, ok := a.allocated[agentName]; ok {
		return subnet, nil
	}

	// Iterate through /24 blocks in the VPC CIDR
	subnet, err := a.findAvailableSubnet()
	if err != nil {
		return nil, err
	}

	a.allocated[agentName] = subnet
	a.used[subnet.String()] = true
	return subnet, nil
}

// Release frees the subnet allocated to the given agent.
func (a *SubnetAllocator) Release(_ context.Context, agentName string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if subnet, ok := a.allocated[agentName]; ok {
		delete(a.used, subnet.String())
		delete(a.allocated, agentName)
	}
}

// GetAll returns a copy of all current allocations.
func (a *SubnetAllocator) GetAll(_ context.Context) map[string]*net.IPNet {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make(map[string]*net.IPNet, len(a.allocated))
	maps.Copy(result, a.allocated)
	return result
}

// findAvailableSubnet iterates through /24 blocks in the VPC CIDR and returns
// the first unallocated one.
func (a *SubnetAllocator) findAvailableSubnet() (*net.IPNet, error) {
	baseIP := a.vpcCIDR.IP.To4()
	if baseIP == nil {
		return nil, fmt.Errorf("only IPv4 CIDRs are supported")
	}

	// Convert base IP to uint32 for arithmetic
	base := binary.BigEndian.Uint32(baseIP)
	ones, bits := a.vpcCIDR.Mask.Size()

	// Number of /24 blocks in this CIDR
	totalBlocks := 1 << uint(a.subnetLen-ones)
	_ = bits // 32 for IPv4

	for i := 0; i < totalBlocks; i++ {
		// Calculate the start IP of this /24 block
		blockIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(blockIP, base+uint32(i)<<8) //nolint:gosec // i is bounded by totalBlocks

		subnet := &net.IPNet{
			IP:   blockIP,
			Mask: net.CIDRMask(a.subnetLen, 32),
		}

		if !a.used[subnet.String()] {
			return subnet, nil
		}
	}

	return nil, fmt.Errorf("no available /24 subnets in VPC CIDR %s (%d blocks exhausted)", a.vpcCIDR, totalBlocks)
}
