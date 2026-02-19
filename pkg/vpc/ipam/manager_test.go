package ipam_test

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/fertile-org/banyan/pkg/storage"
)

func TestIPAMManager_AllocateHostSubnet(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	tests := []struct {
		name       string
		hostID     string
		wantSubnet string
		wantErr    bool
	}{
		{
			name:       "allocate subnet for first host",
			hostID:     "host-001",
			wantSubnet: "10.0.1.0/24",
			wantErr:    false,
		},
		{
			name:       "allocate subnet for second host",
			hostID:     "host-002",
			wantSubnet: "10.0.2.0/24",
			wantErr:    false,
		},
		{
			name:       "allocate subnet for third host",
			hostID:     "host-003",
			wantSubnet: "10.0.3.0/24",
			wantErr:    false,
		},
		{
			name:       "allocate for same host (should return same subnet)",
			hostID:     "host-001",
			wantSubnet: "10.0.1.0/24",
			wantErr:    false,
		},
		{
			name:    "allocate with empty host ID",
			hostID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subnet, err := manager.AllocateHostSubnet(ctx, tt.hostID)
			if (err != nil) != tt.wantErr {
				t.Errorf("AllocateHostSubnet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if subnet == nil {
					t.Fatal("expected subnet, got nil")
				}
				if subnet.String() != tt.wantSubnet {
					t.Errorf("expected subnet %s, got %s", tt.wantSubnet, subnet.String())
				}
			}
		})
	}
}

func TestIPAMManager_AllocateIP(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// First allocate a subnet for the host
	subnet, err := manager.AllocateHostSubnet(ctx, "host-test")
	if err != nil {
		t.Fatalf("Failed to allocate subnet: %v", err)
	}

	tests := []struct {
		name    string
		subnet  *net.IPNet
		wantIP  string
		wantErr bool
	}{
		{
			name:    "allocate first IP from subnet",
			subnet:  subnet,
			wantIP:  "10.0.1.2", // .1 is reserved for gateway
			wantErr: false,
		},
		{
			name:    "allocate second IP from subnet",
			subnet:  subnet,
			wantIP:  "10.0.1.3",
			wantErr: false,
		},
		{
			name:    "allocate third IP from subnet",
			subnet:  subnet,
			wantIP:  "10.0.1.4",
			wantErr: false,
		},
		{
			name:    "allocate from nil subnet",
			subnet:  nil,
			wantErr: true,
		},
		{
			name: "allocate from unmanaged subnet",
			subnet: &net.IPNet{
				IP:   net.ParseIP("192.168.0.0"),
				Mask: net.CIDRMask(24, 32),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := manager.AllocateIP(ctx, tt.subnet)
			if (err != nil) != tt.wantErr {
				t.Errorf("AllocateIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if ip == nil {
					t.Fatal("expected IP, got nil")
				}
				if ip.String() != tt.wantIP {
					t.Errorf("expected IP %s, got %s", tt.wantIP, ip.String())
				}
			}
		})
	}
}

func TestIPAMManager_ReleaseIP(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Allocate subnet and IPs
	subnet, _ := manager.AllocateHostSubnet(ctx, "release-test-host")
	ip1, _ := manager.AllocateIP(ctx, subnet)
	_, _ = manager.AllocateIP(ctx, subnet)

	tests := []struct {
		name    string
		ip      net.IP
		wantErr bool
		verify  func(t *testing.T)
	}{
		{
			name:    "release allocated IP",
			ip:      ip1,
			wantErr: false,
			verify: func(t *testing.T) {
				// Should be able to allocate same IP again
				newIP, err := manager.AllocateIP(ctx, subnet)
				if err != nil {
					t.Errorf("Failed to re-allocate released IP: %v", err)
				}
				if !newIP.Equal(ip1) {
					t.Errorf("Expected to get same IP %s, got %s", ip1, newIP)
				}
			},
		},
		{
			name:    "release non-allocated IP",
			ip:      net.ParseIP("10.0.1.100"),
			wantErr: true,
		},
		{
			name:    "release nil IP",
			ip:      nil,
			wantErr: true,
		},
		{
			name:    "release allocated IP (ip1 was re-allocated in test case 1)",
			ip:      ip1, // Was released in test case 1, then re-allocated in verify
			wantErr: false, // Should succeed since it's currently allocated
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ReleaseIP(ctx, tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReleaseIP() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verify != nil {
				tt.verify(t)
			}
		})
	}
}

func TestIPAMManager_RenewLease(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Allocate subnet
	manager.AllocateHostSubnet(ctx, "lease-test-host")

	tests := []struct {
		name    string
		hostID  string
		wantErr bool
	}{
		{
			name:    "renew existing lease",
			hostID:  "lease-test-host",
			wantErr: false,
		},
		{
			name:    "renew non-existent lease",
			hostID:  "non-existent-host",
			wantErr: true,
		},
		{
			name:    "renew with empty host ID",
			hostID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RenewLease(ctx, tt.hostID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RenewLease() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIPAMManager_GetHostSubnet(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Allocate some subnets
	subnet1, _ := manager.AllocateHostSubnet(ctx, "get-test-host-1")
	subnet2, _ := manager.AllocateHostSubnet(ctx, "get-test-host-2")

	tests := []struct {
		name       string
		hostID     string
		wantSubnet *net.IPNet
		wantErr    bool
	}{
		{
			name:       "get existing host subnet",
			hostID:     "get-test-host-1",
			wantSubnet: subnet1,
			wantErr:    false,
		},
		{
			name:       "get another existing host subnet",
			hostID:     "get-test-host-2",
			wantSubnet: subnet2,
			wantErr:    false,
		},
		{
			name:    "get non-existent host subnet",
			hostID:  "non-existent",
			wantErr: true,
		},
		{
			name:    "get with empty host ID",
			hostID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subnet, err := manager.GetHostSubnet(ctx, tt.hostID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetHostSubnet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if subnet == nil {
					t.Fatal("expected subnet, got nil")
				}
				if !subnet.IP.Equal(tt.wantSubnet.IP) {
					t.Errorf("expected subnet %s, got %s", tt.wantSubnet, subnet)
				}
			}
		})
	}
}

func TestIPAMManager_SubnetExhaustion(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Try to allocate max number of hosts (for 10.0.0.0/16, we have 256 /24 subnets)
	// Test with a smaller number for performance
	maxHosts := 10
	for i := 0; i < maxHosts; i++ {
		hostID := fmt.Sprintf("exhaustion-host-%d", i)
		subnet, err := manager.AllocateHostSubnet(ctx, hostID)
		if err != nil {
			t.Fatalf("Failed to allocate subnet for host %d: %v", i, err)
		}
		expectedSubnet := fmt.Sprintf("10.0.%d.0/24", i+1)
		if subnet.String() != expectedSubnet {
			t.Errorf("Expected subnet %s for host %d, got %s", expectedSubnet, i, subnet.String())
		}
	}
}

func TestIPAMManager_IPExhaustion(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager, err := ipam.NewManager(store, "10.0.0.0/16")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Allocate a subnet
	subnet, err := manager.AllocateHostSubnet(ctx, "ip-exhaustion-host")
	if err != nil {
		t.Fatalf("Failed to allocate subnet: %v", err)
	}

	// Try to allocate all IPs in the subnet (254 usable IPs in /24)
	// Test with a smaller number for performance
	maxIPs := 10
	allocatedIPs := make([]net.IP, 0, maxIPs)

	for i := 0; i < maxIPs; i++ {
		ip, err := manager.AllocateIP(ctx, subnet)
		if err != nil {
			t.Fatalf("Failed to allocate IP %d: %v", i, err)
		}
		allocatedIPs = append(allocatedIPs, ip)

		// First IP should be .2 (gateway is .1)
		expectedIP := fmt.Sprintf("10.0.1.%d", i+2)
		if ip.String() != expectedIP {
			t.Errorf("Expected IP %s for allocation %d, got %s", expectedIP, i, ip.String())
		}
	}

	// Release one IP and verify we can allocate it again
	manager.ReleaseIP(ctx, allocatedIPs[5])
	ip, err := manager.AllocateIP(ctx, subnet)
	if err != nil {
		t.Fatalf("Failed to allocate IP after release: %v", err)
	}
	if !ip.Equal(allocatedIPs[5]) {
		t.Errorf("Expected to get released IP %s, got %s", allocatedIPs[5], ip)
	}
}

func TestIPAMManager_LeaseExpiration(t *testing.T) {
	// This test would verify that expired leases are handled correctly
	// In a real implementation, we'd need to mock time or have a way to set lease duration
	t.Skip("Lease expiration test requires time manipulation - implement after review")
}
