package network

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/fertile/banyan/pkg/vpc"
	"github.com/fertile/banyan/pkg/vpc/storage"
	"github.com/google/uuid"
)

type Manager struct {
	store storage.StateStore
}

func NewManager(store storage.StateStore) *Manager {
	return &Manager{
		store: store,
	}
}

// allocateVxlanID finds the next available VNI starting from 100
// VNI range: 100-16777215 (avoiding lower VNIs that might be reserved)
func (m *Manager) allocateVxlanID(existingNetworks []*vpc.Network) int {
	maxVNI := 99 // Start from 100
	for _, net := range existingNetworks {
		if net.VxlanID > maxVNI {
			maxVNI = net.VxlanID
		}
	}
	return maxVNI + 1
}

func (m *Manager) CreateNetwork(ctx context.Context, config vpc.NetworkConfig) (*vpc.Network, error) {
	// Apply defaults
	if config.CIDR == "" {
		config.CIDR = "10.0.0.0/16"
	}
	if config.DNSSuffix == "" {
		config.DNSSuffix = ".internal"
	}
	if config.Driver == "" {
		config.Driver = "flannel"
	}

	// Validate CIDR format
	if _, _, err := net.ParseCIDR(config.CIDR); err != nil {
		return nil, fmt.Errorf("invalid CIDR format: %w", err)
	}

	// Get existing networks for validation and VNI allocation
	networks, err := m.ListNetworks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicates: %w", err)
	}

	// Check for duplicate names
	for _, net := range networks {
		if net.Name == config.Name {
			return nil, fmt.Errorf("network with name %s already exists", config.Name)
		}
	}

	// Auto-allocate VxlanID if not specified
	if config.VxlanID == 0 {
		config.VxlanID = m.allocateVxlanID(networks)
	} else {
		// Validate user-specified VxlanID for collision
		for _, net := range networks {
			if net.VxlanID == config.VxlanID {
				return nil, fmt.Errorf("VxlanID %d already in use by network %s", config.VxlanID, net.Name)
			}
		}
	}

	// Create network object
	network := &vpc.Network{
		ID:        uuid.New().String(),
		Name:      config.Name,
		CIDR:      config.CIDR,
		VxlanID:   config.VxlanID,
		DNSSuffix: config.DNSSuffix,
		CreatedAt: time.Now(),
		Status:    "active",
	}

	// Save to store
	key := fmt.Sprintf("networks/%s", network.ID)
	if err := m.store.Save(ctx, key, network); err != nil {
		return nil, fmt.Errorf("failed to save network: %w", err)
	}

	return network, nil
}

func (m *Manager) DeleteNetwork(ctx context.Context, networkID string) error {
	key := fmt.Sprintf("networks/%s", networkID)

	// Check if network exists
	var network vpc.Network
	if err := m.store.Get(ctx, key, &network); err != nil {
		return fmt.Errorf("network not found: %w", err)
	}

	// Delete network
	if err := m.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}

	return nil
}

func (m *Manager) GetNetwork(ctx context.Context, networkID string) (*vpc.Network, error) {
	key := fmt.Sprintf("networks/%s", networkID)

	var network vpc.Network
	if err := m.store.Get(ctx, key, &network); err != nil {
		return nil, fmt.Errorf("network not found: %w", err)
	}

	return &network, nil
}

func (m *Manager) ListNetworks(ctx context.Context) ([]*vpc.Network, error) {
	keys, err := m.store.List(ctx, "networks/")
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	networks := make([]*vpc.Network, 0, len(keys))
	for _, key := range keys {
		var network vpc.Network
		if err := m.store.Get(ctx, key, &network); err != nil {
			continue // Skip invalid entries
		}
		networks = append(networks, &network)
	}

	return networks, nil
}

var _ vpc.NetworkManager = (*Manager)(nil)
