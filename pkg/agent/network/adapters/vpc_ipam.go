package adapters

import (
	"context"
	"net"

	"github.com/fertile-org/banyan/pkg/agent/network/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// VPCIPAMAdapter wraps the VPC module's IPAM manager to implement the network module's IPAllocator interface.
type VPCIPAMAdapter struct {
	ipamManager vpc.IPAMManager
	hostID      string
}

// NewVPCIPAMAdapter creates a new VPCIPAMAdapter.
func NewVPCIPAMAdapter(ipamManager vpc.IPAMManager, hostID string) *VPCIPAMAdapter {
	return &VPCIPAMAdapter{
		ipamManager: ipamManager,
		hostID:      hostID,
	}
}

// AllocateIP allocates an IP address from a subnet for a container.
// The containerID is used for tracking but not directly passed to VPC IPAM.
func (a *VPCIPAMAdapter) AllocateIP(ctx context.Context, subnet *net.IPNet, _ string) (net.IP, error) {
	return a.ipamManager.AllocateIP(ctx, subnet)
}

// ReleaseIP releases an allocated IP address.
func (a *VPCIPAMAdapter) ReleaseIP(ctx context.Context, ip net.IP) error {
	return a.ipamManager.ReleaseIP(ctx, ip)
}

// GetSubnet gets the subnet for a network.
// For VPC networks, this returns the host's allocated subnet.
func (a *VPCIPAMAdapter) GetSubnet(ctx context.Context, _ string) (*net.IPNet, error) {
	return a.ipamManager.GetHostSubnet(ctx, a.hostID)
}

// Ensure VPCIPAMAdapter implements outbound.IPAllocator
var _ outbound.IPAllocator = (*VPCIPAMAdapter)(nil)
