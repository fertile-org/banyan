package outbound

import (
	"context"
	"net"
)

// IPAllocator handles IP address allocation from the IPAM system.
type IPAllocator interface {
	// AllocateIP allocates an IP address from a subnet for a container.
	AllocateIP(ctx context.Context, subnet *net.IPNet, containerID string) (net.IP, error)

	// ReleaseIP releases an allocated IP address.
	ReleaseIP(ctx context.Context, ip net.IP) error

	// GetSubnet gets the subnet for a network.
	GetSubnet(ctx context.Context, networkID string) (*net.IPNet, error)
}
