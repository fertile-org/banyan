package outbound

import (
	"context"
	"net"
)

// CNIProvider interfaces with the CNI runtime for network operations.
type CNIProvider interface {
	// AddToNetwork adds a container to a network using CNI.
	AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error

	// RemoveFromNetwork removes a container from a network using CNI.
	RemoveFromNetwork(ctx context.Context, containerID, networkID string) error

	// GetPluginStatus returns the status of the CNI plugin.
	GetPluginStatus(ctx context.Context) (*PluginStatus, error)
}

// PluginStatus represents the status of a CNI plugin.
type PluginStatus struct {
	Name    string
	Version string
	Message string
	Ready   bool
}
