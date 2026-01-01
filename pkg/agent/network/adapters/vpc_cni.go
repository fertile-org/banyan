package adapters

import (
	"context"
	"net"

	"github.com/fertile-org/banyan/pkg/agent/network/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// VPCCNIAdapter wraps the VPC module's CNI runtime to implement the network module's CNIProvider interface.
type VPCCNIAdapter struct {
	cniRuntime vpc.CNIRuntime
	pluginName string
}

// NewVPCCNIAdapter creates a new VPCCNIAdapter.
func NewVPCCNIAdapter(cniRuntime vpc.CNIRuntime, pluginName string) *VPCCNIAdapter {
	return &VPCCNIAdapter{
		cniRuntime: cniRuntime,
		pluginName: pluginName,
	}
}

// AddToNetwork adds a container to a network using CNI.
func (a *VPCCNIAdapter) AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error {
	return a.cniRuntime.AddToNetwork(ctx, containerID, networkID, ip)
}

// RemoveFromNetwork removes a container from a network using CNI.
func (a *VPCCNIAdapter) RemoveFromNetwork(ctx context.Context, containerID, networkID string) error {
	return a.cniRuntime.RemoveFromNetwork(ctx, containerID, networkID)
}

// GetPluginStatus returns the status of the CNI plugin.
func (a *VPCCNIAdapter) GetPluginStatus(ctx context.Context) (*outbound.PluginStatus, error) {
	vpcStatus, err := a.cniRuntime.GetPluginStatus(ctx, a.pluginName)
	if err != nil {
		return nil, err
	}

	return &outbound.PluginStatus{
		Name:    vpcStatus.Name,
		Version: vpcStatus.Version,
		Ready:   vpcStatus.Status == "active",
		Message: vpcStatus.Error,
	}, nil
}

// Ensure VPCCNIAdapter implements outbound.CNIProvider
var _ outbound.CNIProvider = (*VPCCNIAdapter)(nil)
