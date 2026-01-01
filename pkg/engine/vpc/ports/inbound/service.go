package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
)

// VPCCoordinatorService defines the VPC coordination operations.
type VPCCoordinatorService interface {
	// Network Provisioning
	ProvisionNetwork(ctx context.Context, spec domain.NetworkProvisionSpec) (*domain.NetworkInfo, error)
	DeleteNetwork(ctx context.Context, vpcID string) error
	GetNetworkStatus(ctx context.Context, vpcID string) (*domain.NetworkInfo, error)

	// Container Networking
	AllocateContainerNetwork(ctx context.Context, req domain.ContainerNetworkRequest) (*domain.ContainerNetwork, error)
	ReleaseContainerNetwork(ctx context.Context, containerID string) error
	UpdateContainerNetwork(ctx context.Context, containerID string, updates domain.ContainerNetworkUpdate) error

	// Security
	ApplySecurityPolicy(ctx context.Context, policy domain.NetworkPolicy) error
	DeleteSecurityPolicy(ctx context.Context, name, namespace string) error
	CreateSecurityGroup(ctx context.Context, vpcID string, spec domain.SecurityGroupSpec) (*domain.SecurityGroup, error)
	DeleteSecurityGroup(ctx context.Context, groupID string) error

	// Query
	GetContainerNetwork(ctx context.Context, containerID string) (*domain.ContainerNetwork, error)
	ListContainersByNetwork(ctx context.Context, vpcID string) ([]string, error)

	// DNS Registration
	RegisterServiceDNS(ctx context.Context, serviceName, namespace string, ip string) error
	UnregisterServiceDNS(ctx context.Context, serviceName, namespace string) error
}
