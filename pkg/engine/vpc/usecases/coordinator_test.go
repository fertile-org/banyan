package usecases

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/engine/vpc/adapters"
	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
)

func TestVPCCoordinator_ProvisionNetwork(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Test provisioning a network
	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.0.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "containers", CIDR: "10.0.1.0/24", Purpose: domain.SubnetPurposeContainer},
		},
	}

	info, err := coordinator.ProvisionNetwork(ctx, spec)
	if err != nil {
		t.Fatalf("ProvisionNetwork failed: %v", err)
	}

	if info.VPCID == "" {
		t.Error("Expected VPC ID to be set")
	}
	if info.Name != "test-vpc" {
		t.Errorf("Expected name 'test-vpc', got '%s'", info.Name)
	}
	if info.Status != domain.VPCStatusActive {
		t.Errorf("Expected status Active, got '%s'", info.Status)
	}
	if len(info.Subnets) != 1 {
		t.Errorf("Expected 1 subnet, got %d", len(info.Subnets))
	}
}

func TestVPCCoordinator_AllocateContainerNetwork(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// First provision a network
	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.0.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "containers", CIDR: "10.0.1.0/24", Purpose: domain.SubnetPurposeContainer},
		},
	}

	info, err := coordinator.ProvisionNetwork(ctx, spec)
	if err != nil {
		t.Fatalf("ProvisionNetwork failed: %v", err)
	}

	// Add subnet to VPC for selection
	err = networkMgr.AddSubnetToVPC(info.VPCID, domain.Subnet{
		ID:           info.Subnets[0].SubnetID,
		VPCID:        info.VPCID,
		CIDR:         "10.0.1.0/24",
		Purpose:      domain.SubnetPurposeContainer,
		AvailableIPs: 254,
		TotalIPs:     254,
	})
	if err != nil {
		t.Fatalf("AddSubnetToVPC failed: %v", err)
	}

	// Allocate container network
	req := domain.ContainerNetworkRequest{
		ContainerID: "container-1",
		ServiceName: "api",
		Namespace:   "default",
		VPCID:       info.VPCID,
		SubnetID:    info.Subnets[0].SubnetID,
	}

	containerNet, err := coordinator.AllocateContainerNetwork(ctx, req)
	if err != nil {
		t.Fatalf("AllocateContainerNetwork failed: %v", err)
	}

	if containerNet.ContainerID != "container-1" {
		t.Errorf("Expected container ID 'container-1', got '%s'", containerNet.ContainerID)
	}
	if containerNet.IPAddress == "" {
		t.Error("Expected IP address to be set")
	}
	if containerNet.VPCID != info.VPCID {
		t.Errorf("Expected VPC ID '%s', got '%s'", info.VPCID, containerNet.VPCID)
	}
	if containerNet.MAC == "" {
		t.Error("Expected MAC address to be set")
	}
}

func TestVPCCoordinator_ReleaseContainerNetwork(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Provision network
	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.0.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "containers", CIDR: "10.0.1.0/24", Purpose: domain.SubnetPurposeContainer},
		},
	}

	info, _ := coordinator.ProvisionNetwork(ctx, spec)
	_ = networkMgr.AddSubnetToVPC(info.VPCID, domain.Subnet{
		ID:           info.Subnets[0].SubnetID,
		VPCID:        info.VPCID,
		CIDR:         "10.0.1.0/24",
		Purpose:      domain.SubnetPurposeContainer,
		AvailableIPs: 254,
		TotalIPs:     254,
	})

	// Allocate
	req := domain.ContainerNetworkRequest{
		ContainerID: "container-1",
		VPCID:       info.VPCID,
		SubnetID:    info.Subnets[0].SubnetID,
	}
	_, _ = coordinator.AllocateContainerNetwork(ctx, req)

	// Release
	err := coordinator.ReleaseContainerNetwork(ctx, "container-1")
	if err != nil {
		t.Fatalf("ReleaseContainerNetwork failed: %v", err)
	}

	// Verify released
	_, err = coordinator.GetContainerNetwork(ctx, "container-1")
	if err == nil {
		t.Error("Expected error when getting released container network")
	}
}

func TestVPCCoordinator_SecurityOperations(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Create a security group
	sgSpec := domain.SecurityGroupSpec{
		Name: "web-servers",
		Rules: []domain.SecurityRule{
			{
				Direction: domain.RuleDirectionIngress,
				Protocol:  "tcp",
				PortRange: domain.PortRange{Start: 80, End: 80},
				Action:    domain.RuleActionAllow,
			},
		},
	}

	sg, err := coordinator.CreateSecurityGroup(ctx, "vpc-1", sgSpec)
	if err != nil {
		t.Fatalf("CreateSecurityGroup failed: %v", err)
	}

	if sg.Name != "web-servers" {
		t.Errorf("Expected name 'web-servers', got '%s'", sg.Name)
	}
	if len(sg.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(sg.Rules))
	}

	// Delete security group
	err = coordinator.DeleteSecurityGroup(ctx, sg.ID)
	if err != nil {
		t.Fatalf("DeleteSecurityGroup failed: %v", err)
	}
}

func TestVPCCoordinator_NetworkPolicy(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Apply a network policy
	policy := domain.NetworkPolicy{
		Name:        "api-policy",
		Namespace:   "default",
		PodSelector: map[string]string{"app": "api"},
		Ingress: []domain.NetworkPolicyRule{
			{
				From: []domain.NetworkPolicyPeer{
					{PodSelector: map[string]string{"app": "frontend"}},
				},
				Ports: []domain.PortRange{{Start: 8080, End: 8080}},
			},
		},
	}

	err := coordinator.ApplySecurityPolicy(ctx, policy)
	if err != nil {
		t.Fatalf("ApplySecurityPolicy failed: %v", err)
	}

	// Delete policy
	err = coordinator.DeleteSecurityPolicy(ctx, "api-policy", "default")
	if err != nil {
		t.Fatalf("DeleteSecurityPolicy failed: %v", err)
	}
}

func TestVPCCoordinator_GetNetworkStatus(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Provision network
	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.0.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "containers", CIDR: "10.0.1.0/24", Purpose: domain.SubnetPurposeContainer},
		},
	}

	info, _ := coordinator.ProvisionNetwork(ctx, spec)

	// Get status
	status, err := coordinator.GetNetworkStatus(ctx, info.VPCID)
	if err != nil {
		t.Fatalf("GetNetworkStatus failed: %v", err)
	}

	if status.VPCID != info.VPCID {
		t.Errorf("Expected VPC ID '%s', got '%s'", info.VPCID, status.VPCID)
	}
	if status.Status != domain.VPCStatusActive {
		t.Errorf("Expected status Active, got '%s'", status.Status)
	}
}

func TestVPCCoordinator_DeleteNetwork(t *testing.T) {
	networkMgr := adapters.NewMemoryNetworkManagerAdapter()
	ipamMgr := adapters.NewMemoryIPAMManagerAdapter()
	securityMgr := adapters.NewMemorySecurityManagerAdapter()
	dnsMgr := adapters.NewMemoryDNSManagerAdapter()
	store := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := NewVPCCoordinator(networkMgr, ipamMgr, securityMgr, dnsMgr, store, nil)

	ctx := context.Background()

	// Provision network
	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.0.0.0/16",
	}

	info, _ := coordinator.ProvisionNetwork(ctx, spec)

	// Delete network
	err := coordinator.DeleteNetwork(ctx, info.VPCID)
	if err != nil {
		t.Fatalf("DeleteNetwork failed: %v", err)
	}

	// Verify deleted
	_, err = coordinator.GetNetworkStatus(ctx, info.VPCID)
	if err == nil {
		t.Error("Expected error when getting deleted network")
	}
}
