package ipam

import (
	"context"
	"net"

	"github.com/fertile/banyan/pkg/vpc"
)

// Manager implements vpc.IPAMManager interface
type Manager struct {
	// Will add fields during implementation phase
}

// NewManager creates a new IPAM manager
func NewManager() *Manager {
	return &Manager{}
}

// AllocateHostSubnet allocates a /24 subnet for a host
func (m *Manager) AllocateHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
	// TDD: Implementation will be added after test review
	return nil, nil
}

// AllocateIP allocates an IP from a subnet
func (m *Manager) AllocateIP(ctx context.Context, subnet *net.IPNet) (net.IP, error) {
	// TDD: Implementation will be added after test review
	return nil, nil
}

// ReleaseIP releases an allocated IP
func (m *Manager) ReleaseIP(ctx context.Context, ip net.IP) error {
	// TDD: Implementation will be added after test review
	return nil
}

// RenewLease renews the lease for a host's subnet
func (m *Manager) RenewLease(ctx context.Context, hostID string) error {
	// TDD: Implementation will be added after test review
	return nil
}

// GetHostSubnet returns the subnet allocated to a host
func (m *Manager) GetHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
	// TDD: Implementation will be added after test review
	return nil, nil
}

// Ensure Manager implements IPAMManager interface
var _ vpc.IPAMManager = (*Manager)(nil)
