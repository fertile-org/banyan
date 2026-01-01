package usecases

import (
	"context"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
)

// VPCCoordinator is the main coordinator that delegates to specialized use cases.
type VPCCoordinator struct {
	provisioning *NetworkProvisioningUseCase
	networking   *ContainerNetworkingUseCase
	security     *SecurityPolicyUseCase
	logger       *slog.Logger
}

// NewVPCCoordinator creates a new VPCCoordinator.
func NewVPCCoordinator(
	networkMgr outbound.NetworkManager,
	ipamMgr outbound.IPAMManager,
	securityMgr outbound.SecurityManager,
	dnsMgr outbound.DNSManager,
	store outbound.ContainerNetworkStore,
	logger *slog.Logger,
) *VPCCoordinator {
	if logger == nil {
		logger = slog.Default()
	}
	return &VPCCoordinator{
		provisioning: NewNetworkProvisioningUseCase(networkMgr, ipamMgr, securityMgr, logger),
		networking:   NewContainerNetworkingUseCase(networkMgr, ipamMgr, securityMgr, dnsMgr, store, logger),
		security:     NewSecurityPolicyUseCase(securityMgr, store, logger),
		logger:       logger,
	}
}

// Network Provisioning

// ProvisionNetwork creates a VPC with subnets and security groups.
func (c *VPCCoordinator) ProvisionNetwork(ctx context.Context, spec domain.NetworkProvisionSpec) (*domain.NetworkInfo, error) {
	return c.provisioning.ProvisionNetwork(ctx, spec)
}

// DeleteNetwork deletes a VPC and its associated resources.
func (c *VPCCoordinator) DeleteNetwork(ctx context.Context, vpcID string) error {
	return c.provisioning.DeleteNetwork(ctx, vpcID)
}

// GetNetworkStatus returns the status of a VPC.
func (c *VPCCoordinator) GetNetworkStatus(ctx context.Context, vpcID string) (*domain.NetworkInfo, error) {
	return c.provisioning.GetNetworkStatus(ctx, vpcID)
}

// Container Networking

// AllocateContainerNetwork allocates network resources for a container.
func (c *VPCCoordinator) AllocateContainerNetwork(ctx context.Context, req domain.ContainerNetworkRequest) (*domain.ContainerNetwork, error) {
	return c.networking.AllocateContainerNetwork(ctx, req)
}

// ReleaseContainerNetwork releases network resources for a container.
func (c *VPCCoordinator) ReleaseContainerNetwork(ctx context.Context, containerID string) error {
	return c.networking.ReleaseContainerNetwork(ctx, containerID)
}

// UpdateContainerNetwork updates network configuration for a container.
func (c *VPCCoordinator) UpdateContainerNetwork(ctx context.Context, containerID string, updates domain.ContainerNetworkUpdate) error {
	return c.networking.UpdateContainerNetwork(ctx, containerID, updates)
}

// GetContainerNetwork returns the network allocation for a container.
func (c *VPCCoordinator) GetContainerNetwork(ctx context.Context, containerID string) (*domain.ContainerNetwork, error) {
	return c.networking.GetContainerNetwork(ctx, containerID)
}

// ListContainersByNetwork lists all containers in a VPC.
func (c *VPCCoordinator) ListContainersByNetwork(ctx context.Context, vpcID string) ([]string, error) {
	return c.networking.ListContainersByNetwork(ctx, vpcID)
}

// Security

// ApplySecurityPolicy applies a network policy.
func (c *VPCCoordinator) ApplySecurityPolicy(ctx context.Context, policy domain.NetworkPolicy) error {
	return c.security.ApplySecurityPolicy(ctx, policy)
}

// DeleteSecurityPolicy removes a network policy.
func (c *VPCCoordinator) DeleteSecurityPolicy(ctx context.Context, name, namespace string) error {
	return c.security.DeleteSecurityPolicy(ctx, name, namespace)
}

// CreateSecurityGroup creates a new security group.
func (c *VPCCoordinator) CreateSecurityGroup(ctx context.Context, vpcID string, spec domain.SecurityGroupSpec) (*domain.SecurityGroup, error) {
	return c.security.CreateSecurityGroup(ctx, vpcID, spec)
}

// DeleteSecurityGroup deletes a security group.
func (c *VPCCoordinator) DeleteSecurityGroup(ctx context.Context, groupID string) error {
	return c.security.DeleteSecurityGroup(ctx, groupID)
}

// DNS

// RegisterServiceDNS registers a service name with an IP for DNS resolution.
func (c *VPCCoordinator) RegisterServiceDNS(ctx context.Context, serviceName, namespace string, ip string) error {
	return c.networking.RegisterServiceDNS(ctx, serviceName, namespace, ip)
}

// UnregisterServiceDNS removes a service name from DNS.
func (c *VPCCoordinator) UnregisterServiceDNS(ctx context.Context, serviceName, namespace string) error {
	return c.networking.UnregisterServiceDNS(ctx, serviceName, namespace)
}

// Ensure VPCCoordinator implements VPCCoordinatorService.
var _ inbound.VPCCoordinatorService = (*VPCCoordinator)(nil)
