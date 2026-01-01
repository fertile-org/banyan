package outbound

import (
	"context"
	"net"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
)

// NetworkManager handles VPC and subnet operations.
// This interface wraps the pkg/vpc NetworkManager.
type NetworkManager interface {
	CreateVPC(ctx context.Context, name, cidr string, metadata map[string]string) (*domain.VPC, error)
	GetVPC(ctx context.Context, id string) (*domain.VPC, error)
	DeleteVPC(ctx context.Context, id string) error
	ListVPCs(ctx context.Context) ([]*domain.VPC, error)
	AddSubnetToVPC(vpcID string, subnet domain.Subnet) error
}

// IPAMManager handles IP address allocation.
// This interface wraps the pkg/vpc IPAMManager.
type IPAMManager interface {
	// Subnet operations
	CreateSubnet(ctx context.Context, vpcID, cidr string, purpose domain.SubnetPurpose) (*domain.Subnet, error)
	GetSubnet(ctx context.Context, id string) (*domain.Subnet, error)
	DeleteSubnet(ctx context.Context, id string) error

	// IP allocation
	AllocateIP(ctx context.Context, subnetID, containerID string) (string, error)
	ReleaseIP(ctx context.Context, ip string) error
	GetIPAllocation(ctx context.Context, containerID string) (*domain.IPAllocation, error)

	// Capacity
	GetSubnetCapacity(ctx context.Context, subnetID string) (available, total int, err error)
}

// SecurityManager handles security groups and policies.
// This interface wraps the pkg/vpc SecurityManager.
type SecurityManager interface {
	CreateSecurityGroup(ctx context.Context, vpcID string, spec domain.SecurityGroupSpec) (*domain.SecurityGroup, error)
	GetSecurityGroup(ctx context.Context, id string) (*domain.SecurityGroup, error)
	UpdateSecurityGroup(ctx context.Context, id string, rules []domain.SecurityRule) error
	DeleteSecurityGroup(ctx context.Context, id string) error

	ApplyNetworkPolicy(ctx context.Context, policy domain.NetworkPolicy) error
	DeleteNetworkPolicy(ctx context.Context, name, namespace string) error
}

// DNSManager handles DNS registration for service discovery.
// This interface wraps the pkg/vpc DNSManager.
type DNSManager interface {
	RegisterHost(ctx context.Context, hostname string, ip net.IP) error
	UnregisterHost(ctx context.Context, hostname string) error
	LookupHost(ctx context.Context, hostname string) ([]net.IP, error)
	UpdateHealth(ctx context.Context, hostname string, healthy bool) error
}

// ContainerNetworkStore tracks container network allocations.
type ContainerNetworkStore interface {
	Save(ctx context.Context, network *domain.ContainerNetwork) error
	Get(ctx context.Context, containerID string) (*domain.ContainerNetwork, error)
	Delete(ctx context.Context, containerID string) error
	FindByVPC(ctx context.Context, vpcID string) ([]domain.ContainerNetwork, error)
}
