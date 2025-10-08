package network_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fertile/banyan/pkg/vpc"
	"github.com/fertile/banyan/pkg/vpc/network"
	"github.com/fertile/banyan/pkg/vpc/storage"
)

func TestNetworkManager_CreateNetwork(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager := network.NewManager(store)

	tests := []struct {
		name    string
		config  vpc.NetworkConfig
		wantErr bool
		checks  func(t *testing.T, net *vpc.Network)
	}{
		{
			name: "create network with default values",
			config: vpc.NetworkConfig{
				Name: "test-network",
			},
			wantErr: false,
			checks: func(t *testing.T, net *vpc.Network) {
				if net == nil {
					t.Fatal("expected network, got nil")
				}
				if net.Name != "test-network" {
					t.Errorf("expected name test-network, got %s", net.Name)
				}
				if net.CIDR != "10.0.0.0/16" {
					t.Errorf("expected default CIDR 10.0.0.0/16, got %s", net.CIDR)
				}
				if net.DNSSuffix != ".internal" {
					t.Errorf("expected default DNS suffix .internal, got %s", net.DNSSuffix)
				}
				if net.Status != "active" {
					t.Errorf("expected status active, got %s", net.Status)
				}
			},
		},
		{
			name: "create network with custom CIDR",
			config: vpc.NetworkConfig{
				Name: "custom-network",
				CIDR: "172.16.0.0/12",
			},
			wantErr: false,
			checks: func(t *testing.T, net *vpc.Network) {
				if net == nil {
					t.Fatal("expected network, got nil")
				}
				if net.CIDR != "172.16.0.0/12" {
					t.Errorf("expected CIDR 172.16.0.0/12, got %s", net.CIDR)
				}
			},
		},
		{
			name: "create network with invalid CIDR",
			config: vpc.NetworkConfig{
				Name: "invalid-network",
				CIDR: "invalid-cidr",
			},
			wantErr: true,
		},
		{
			name: "create network with duplicate name",
			config: vpc.NetworkConfig{
				Name: "test-network", // Same as first test
			},
			wantErr: true, // Should fail on duplicate
		},
		{
			name: "create network with custom DNS suffix",
			config: vpc.NetworkConfig{
				Name:      "dns-network",
				DNSSuffix: ".prod",
			},
			wantErr: false,
			checks: func(t *testing.T, net *vpc.Network) {
				if net == nil {
					t.Fatal("expected network, got nil")
				}
				if net.DNSSuffix != ".prod" {
					t.Errorf("expected DNS suffix .prod, got %s", net.DNSSuffix)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			net, err := manager.CreateNetwork(ctx, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateNetwork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, net)
			}
		})
	}
}

func TestNetworkManager_GetNetwork(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager := network.NewManager(store)

	// First create a network
	created, err := manager.CreateNetwork(ctx, vpc.NetworkConfig{
		Name: "get-test-network",
	})
	if err != nil {
		t.Fatalf("Failed to create test network: %v", err)
	}

	tests := []struct {
		name      string
		networkID string
		wantErr   bool
		checks    func(t *testing.T, net *vpc.Network)
	}{
		{
			name:      "get existing network",
			networkID: created.ID,
			wantErr:   false,
			checks: func(t *testing.T, net *vpc.Network) {
				if net == nil {
					t.Fatal("expected network, got nil")
				}
				if net.ID != created.ID {
					t.Errorf("expected ID %s, got %s", created.ID, net.ID)
				}
				if net.Name != "get-test-network" {
					t.Errorf("expected name get-test-network, got %s", net.Name)
				}
			},
		},
		{
			name:      "get non-existent network",
			networkID: "non-existent-id",
			wantErr:   true,
		},
		{
			name:      "get with empty ID",
			networkID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			net, err := manager.GetNetwork(ctx, tt.networkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNetwork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, net)
			}
		})
	}
}

func TestNetworkManager_DeleteNetwork(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager := network.NewManager(store)

	// Create networks for testing
	network1, _ := manager.CreateNetwork(ctx, vpc.NetworkConfig{Name: "delete-test-1"})
	_, _ = manager.CreateNetwork(ctx, vpc.NetworkConfig{Name: "delete-test-2"})

	tests := []struct {
		name      string
		networkID string
		wantErr   bool
		verify    func(t *testing.T)
	}{
		{
			name:      "delete existing network",
			networkID: network1.ID,
			wantErr:   false,
			verify: func(t *testing.T) {
				// Verify network is deleted
				_, err := manager.GetNetwork(ctx, network1.ID)
				if err == nil {
					t.Error("expected error when getting deleted network")
				}
			},
		},
		{
			name:      "delete non-existent network",
			networkID: "non-existent",
			wantErr:   true,
		},
		{
			name:      "delete already deleted network",
			networkID: network1.ID, // Already deleted above
			wantErr:   true,
		},
		{
			name:      "delete with empty ID",
			networkID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.DeleteNetwork(ctx, tt.networkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteNetwork() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.verify != nil {
				tt.verify(t)
			}
		})
	}
}

func TestNetworkManager_ListNetworks(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager := network.NewManager(store)

	// Create multiple networks
	manager.CreateNetwork(ctx, vpc.NetworkConfig{Name: "list-test-1"})
	manager.CreateNetwork(ctx, vpc.NetworkConfig{Name: "list-test-2"})
	manager.CreateNetwork(ctx, vpc.NetworkConfig{Name: "list-test-3"})

	networks, err := manager.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}

	if len(networks) < 3 {
		t.Errorf("expected at least 3 networks, got %d", len(networks))
	}

	// Check that our test networks are in the list
	found := make(map[string]bool)
	for _, net := range networks {
		if net.Name == "list-test-1" || net.Name == "list-test-2" || net.Name == "list-test-3" {
			found[net.Name] = true
		}
	}

	if len(found) != 3 {
		t.Errorf("expected to find all 3 test networks, found %d", len(found))
	}
}

func TestNetworkManager_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	manager := network.NewManager(store)

	// Test concurrent network creation
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			_, err := manager.CreateNetwork(ctx, vpc.NetworkConfig{
				Name: fmt.Sprintf("concurrent-test-%d", index),
			})
			if err != nil {
				t.Errorf("concurrent creation failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all networks were created
	networks, err := manager.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks() error = %v", err)
	}

	concurrentCount := 0
	for _, net := range networks {
		if strings.HasPrefix(net.Name, "concurrent-test-") {
			concurrentCount++
		}
	}

	if concurrentCount != 10 {
		t.Errorf("expected 10 concurrent networks, got %d", concurrentCount)
	}
}
