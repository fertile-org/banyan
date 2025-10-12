package ipam

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/fertile/banyan/pkg/vpc"
	"github.com/fertile/banyan/pkg/vpc/storage"
)

type Manager struct {
	store      storage.StateStore
	vpcCIDR    *net.IPNet
	mu         sync.RWMutex
	nextSubnet int // Next /24 subnet to allocate (e.g., 1 for 10.0.1.0/24)
}

func NewManager(store storage.StateStore, vpcCIDR string) (*Manager, error) {
	_, cidr, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return nil, fmt.Errorf("invalid VPC CIDR: %w", err)
	}

	m := &Manager{
		store:      store,
		vpcCIDR:    cidr,
		nextSubnet: 1, // Start from .1.0/24
	}

	// Calculate nextSubnet from existing leases
	if err := m.calculateNextSubnet(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to calculate next subnet: %w", err)
	}

	return m, nil
}

func (m *Manager) calculateNextSubnet(ctx context.Context) error {
	// Get all existing leases
	keys, err := m.store.List(ctx, "ipam/leases/")
	if err != nil {
		return err
	}

	maxSubnet := 0
	for _, key := range keys {
		var lease vpc.SubnetLease
		if err := m.store.Get(ctx, key, &lease); err != nil {
			continue
		}

		// Extract the third octet from the subnet IP (e.g., 10.0.1.0 -> 1)
		if lease.Subnet != nil {
			ip := lease.Subnet.IP.To4()
			if ip != nil {
				subnetNum := int(ip[2])
				if subnetNum > maxSubnet {
					maxSubnet = subnetNum
				}
			}
		}
	}

	m.nextSubnet = maxSubnet + 1
	return nil
}

func (m *Manager) AllocateHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate hostID
	if hostID == "" {
		return nil, fmt.Errorf("hostID cannot be empty")
	}

	// Check if host already has a subnet
	key := fmt.Sprintf("ipam/leases/%s", hostID)
	var lease vpc.SubnetLease
	if err := m.store.Get(ctx, key, &lease); err == nil {
		return lease.Subnet, nil // Already allocated
	}

	// Allocate new /24 subnet
	ip := m.vpcCIDR.IP
	subnet := &net.IPNet{
		IP:   net.IPv4(ip[0], ip[1], byte(m.nextSubnet), 0),
		Mask: net.CIDRMask(24, 32),
	}

	// Create lease
	lease = vpc.SubnetLease{
		HostID:    hostID,
		Subnet:    subnet,
		LeaseTime: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Save lease
	if err := m.store.Save(ctx, key, &lease); err != nil {
		return nil, fmt.Errorf("failed to save lease: %w", err)
	}

	m.nextSubnet++
	return subnet, nil
}

func (m *Manager) AllocateIP(ctx context.Context, subnet *net.IPNet) (net.IP, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if subnet == nil {
		return nil, fmt.Errorf("subnet cannot be nil")
	}

	// Validate subnet is within VPC CIDR
	if !m.vpcCIDR.Contains(subnet.IP) {
		return nil, fmt.Errorf("subnet %s is not within VPC CIDR %s", subnet.String(), m.vpcCIDR.String())
	}

	// Get allocated IPs for this subnet
	prefix := fmt.Sprintf("ipam/ips/%s", subnet.String())
	allocatedKeys, err := m.store.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list allocated IPs: %w", err)
	}

	// Track allocated IPs
	allocated := make(map[string]bool)
	for _, key := range allocatedKeys {
		allocated[key] = true
	}

	// Find next available IP (start from .2, .1 is gateway)
	// For /24 subnet like 10.0.1.0/24, IPs range from 10.0.1.0 to 10.0.1.255
	baseIP := subnet.IP.To4()
	if baseIP == nil {
		return nil, fmt.Errorf("only IPv4 is supported")
	}

	// Iterate through all possible IPs in the subnet (skip .0 and .1)
	for i := 2; i < 255; i++ {
		ip := net.IPv4(baseIP[0], baseIP[1], baseIP[2], byte(i))

		ipKey := fmt.Sprintf("ipam/ips/%s/%s", subnet.String(), ip.String())
		if !allocated[ipKey] {
			// Allocate this IP
			if err := m.store.Save(ctx, ipKey, ip.String()); err != nil {
				return nil, fmt.Errorf("failed to save IP: %w", err)
			}
			return ip, nil
		}
	}

	return nil, fmt.Errorf("no available IPs in subnet")
}

func (m *Manager) ReleaseIP(ctx context.Context, ip net.IP) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ip == nil {
		return fmt.Errorf("IP cannot be nil")
	}

	// Find which subnet this IP belongs to
	keys, err := m.store.List(ctx, "ipam/ips/")
	if err != nil {
		return fmt.Errorf("failed to list IPs: %w", err)
	}

	// Delete IP allocation
	found := false
	for _, key := range keys {
		if strings.HasSuffix(key, "/"+ip.String()) {
			if err := m.store.Delete(ctx, key); err != nil {
				return fmt.Errorf("failed to delete IP: %w", err)
			}
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("IP not found: %s", ip.String())
	}

	return nil
}

func (m *Manager) RenewLease(ctx context.Context, hostID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("ipam/leases/%s", hostID)
	var lease vpc.SubnetLease
	if err := m.store.Get(ctx, key, &lease); err != nil {
		return fmt.Errorf("lease not found: %w", err)
	}

	// Renew lease
	lease.ExpiresAt = time.Now().Add(24 * time.Hour)
	return m.store.Save(ctx, key, &lease)
}

func (m *Manager) GetHostSubnet(ctx context.Context, hostID string) (*net.IPNet, error) {
	key := fmt.Sprintf("ipam/leases/%s", hostID)
	var lease vpc.SubnetLease
	if err := m.store.Get(ctx, key, &lease); err != nil {
		return nil, fmt.Errorf("lease not found: %w", err)
	}

	return lease.Subnet, nil
}

// Ensure Manager implements IPAMManager interface
var _ vpc.IPAMManager = (*Manager)(nil)
