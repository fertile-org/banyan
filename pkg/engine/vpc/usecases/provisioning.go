package usecases

import (
	"context"
	"log/slog"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
)

// NetworkProvisioningUseCase handles VPC and subnet creation.
type NetworkProvisioningUseCase struct {
	networkMgr  outbound.NetworkManager
	ipamMgr     outbound.IPAMManager
	securityMgr outbound.SecurityManager
	logger      *slog.Logger
}

// NewNetworkProvisioningUseCase creates a new NetworkProvisioningUseCase.
func NewNetworkProvisioningUseCase(
	networkMgr outbound.NetworkManager,
	ipamMgr outbound.IPAMManager,
	securityMgr outbound.SecurityManager,
	logger *slog.Logger,
) *NetworkProvisioningUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &NetworkProvisioningUseCase{
		networkMgr:  networkMgr,
		ipamMgr:     ipamMgr,
		securityMgr: securityMgr,
		logger:      logger,
	}
}

// ProvisionNetwork creates a VPC with subnets and security groups.
func (uc *NetworkProvisioningUseCase) ProvisionNetwork(
	ctx context.Context,
	spec domain.NetworkProvisionSpec,
) (*domain.NetworkInfo, error) {
	// Step 1: Create VPC
	vpc, err := uc.networkMgr.CreateVPC(ctx, spec.Name, spec.CIDR, spec.Metadata)
	if err != nil {
		return nil, err
	}

	uc.logger.Info("VPC created", "vpc_id", vpc.ID, "cidr", spec.CIDR)

	// Step 2: Create subnets
	var subnets []domain.SubnetInfo
	for _, subnetSpec := range spec.Subnets {
		subnet, err := uc.ipamMgr.CreateSubnet(ctx, vpc.ID, subnetSpec.CIDR, subnetSpec.Purpose)
		if err != nil {
			// Rollback: delete VPC
			_ = uc.networkMgr.DeleteVPC(ctx, vpc.ID)
			return nil, err
		}

		// Add subnet reference to VPC for container network allocation
		if err := uc.networkMgr.AddSubnetToVPC(vpc.ID, *subnet); err != nil {
			uc.logger.Warn("failed to add subnet to VPC", "subnet_id", subnet.ID, "error", err)
		}

		subnets = append(subnets, domain.SubnetInfo{
			SubnetID:     subnet.ID,
			CIDR:         subnet.CIDR,
			Purpose:      subnet.Purpose,
			AvailableIPs: subnet.AvailableIPs,
			AllocatedIPs: 0,
		})
	}

	// Step 3: Create security groups
	var securityGroups []string
	for _, sgSpec := range spec.SecurityGroups {
		sg, err := uc.securityMgr.CreateSecurityGroup(ctx, vpc.ID, sgSpec)
		if err != nil {
			uc.logger.Warn("failed to create security group", "name", sgSpec.Name, "error", err)
			continue
		}
		securityGroups = append(securityGroups, sg.ID)
	}

	// Create default security group if none specified
	if len(securityGroups) == 0 {
		defaultSG, err := uc.securityMgr.CreateSecurityGroup(ctx, vpc.ID, domain.SecurityGroupSpec{
			Name: "default",
			Rules: []domain.SecurityRule{
				{Direction: domain.RuleDirectionEgress, Protocol: "all", Action: domain.RuleActionAllow},
			},
		})
		if err == nil && defaultSG != nil {
			securityGroups = append(securityGroups, defaultSG.ID)
		}
	}

	return &domain.NetworkInfo{
		VPCID:          vpc.ID,
		Name:           vpc.Name,
		CIDR:           vpc.CIDR,
		Status:         domain.VPCStatusActive,
		Subnets:        subnets,
		SecurityGroups: securityGroups,
	}, nil
}

// DeleteNetwork deletes a VPC and its associated resources.
func (uc *NetworkProvisioningUseCase) DeleteNetwork(
	ctx context.Context,
	vpcID string,
) error {
	// Get VPC to find associated resources
	vpc, err := uc.networkMgr.GetVPC(ctx, vpcID)
	if err != nil {
		return err
	}

	// Delete subnets first
	for _, subnet := range vpc.Subnets {
		if err := uc.ipamMgr.DeleteSubnet(ctx, subnet.ID); err != nil {
			uc.logger.Warn("failed to delete subnet", "subnet_id", subnet.ID, "error", err)
		}
	}

	// Delete VPC
	if err := uc.networkMgr.DeleteVPC(ctx, vpcID); err != nil {
		return err
	}

	uc.logger.Info("VPC deleted", "vpc_id", vpcID)
	return nil
}

// GetNetworkStatus returns the status of a VPC.
func (uc *NetworkProvisioningUseCase) GetNetworkStatus(
	ctx context.Context,
	vpcID string,
) (*domain.NetworkInfo, error) {
	vpc, err := uc.networkMgr.GetVPC(ctx, vpcID)
	if err != nil {
		return nil, err
	}

	var subnets []domain.SubnetInfo
	var totalAvailable, totalAllocated int

	for _, subnet := range vpc.Subnets {
		available, total, err := uc.ipamMgr.GetSubnetCapacity(ctx, subnet.ID)
		if err != nil {
			continue
		}
		allocated := total - available
		subnets = append(subnets, domain.SubnetInfo{
			SubnetID:     subnet.ID,
			CIDR:         subnet.CIDR,
			Purpose:      subnet.Purpose,
			AvailableIPs: available,
			AllocatedIPs: allocated,
		})
		totalAvailable += available
		totalAllocated += allocated
	}

	return &domain.NetworkInfo{
		VPCID:        vpc.ID,
		Name:         vpc.Name,
		CIDR:         vpc.CIDR,
		Status:       vpc.Status,
		Subnets:      subnets,
		AvailableIPs: totalAvailable,
		AllocatedIPs: totalAllocated,
	}, nil
}
