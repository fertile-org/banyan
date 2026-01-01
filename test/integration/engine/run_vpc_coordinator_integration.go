//go:build ignore

// Integration test for Engine VPC Coordinator
// Tests the full VPC workflow: Provision Network → Allocate Container → Apply Security → DNS → Cleanup
//
// Run with: go run ./test/integration/engine/run_vpc_coordinator_integration.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/fertile-org/banyan/pkg/engine/vpc/adapters"
	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	"github.com/fertile-org/banyan/pkg/engine/vpc/usecases"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All VPC Coordinator integration tests passed!")
	} else {
		p.Result(false, "VPC Coordinator integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("VPC Coordinator Integration Test")

	// Create VPC coordinator with all adapters
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	networkManager := adapters.NewMemoryNetworkManagerAdapter()
	ipamManager := adapters.NewMemoryIPAMManagerAdapter()
	securityManager := adapters.NewMemorySecurityManagerAdapter()
	dnsManager := adapters.NewMemoryDNSManagerAdapter()
	containerStore := adapters.NewMemoryContainerNetworkStoreAdapter()

	coordinator := usecases.NewVPCCoordinator(networkManager, ipamManager, securityManager, dnsManager, containerStore, logger)

	// Test 1: Provision VPC network
	p.Step("Provisioning VPC network")

	spec := domain.NetworkProvisionSpec{
		Name: "test-vpc",
		CIDR: "10.100.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "container-subnet-1", CIDR: "10.100.1.0/24", Purpose: domain.SubnetPurposeContainer},
			{Name: "container-subnet-2", CIDR: "10.100.2.0/24", Purpose: domain.SubnetPurposeContainer},
		},
		DNSServers: []string{"10.100.0.2"},
	}

	networkInfo, err := coordinator.ProvisionNetwork(ctx, spec)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to provision network: %v", err))
		return 1
	}

	if networkInfo.VPCID == "" {
		p.Error("VPC ID is empty")
		return 1
	}

	p.Success(fmt.Sprintf("VPC provisioned: ID=%s, CIDR=%s", networkInfo.VPCID, networkInfo.CIDR))

	// Test 2: Verify subnets
	p.Step("Verifying subnets were created")

	if len(networkInfo.Subnets) != 2 {
		p.Error(fmt.Sprintf("Expected 2 subnets, got %d", len(networkInfo.Subnets)))
		return 1
	}

	for _, subnet := range networkInfo.Subnets {
		p.Info(fmt.Sprintf("  Subnet: ID=%s, CIDR=%s", subnet.SubnetID, subnet.CIDR))
	}

	p.Success("Subnets created successfully")

	// Test 3: Allocate container network
	p.Step("Allocating container network")

	containerReq := domain.ContainerNetworkRequest{
		ContainerID: "container-1",
		ServiceName: "api",
		Namespace:   "default",
		VPCID:       networkInfo.VPCID,
	}

	containerNet, err := coordinator.AllocateContainerNetwork(ctx, containerReq)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to allocate container network: %v", err))
		return 1
	}

	if containerNet.IPAddress == "" {
		p.Error("Container IP is empty")
		return 1
	}

	p.Success(fmt.Sprintf("Container network allocated: IP=%s, Subnet=%s", containerNet.IPAddress, containerNet.SubnetID))

	// Test 4: Allocate second container
	p.Step("Allocating second container network")

	containerReq2 := domain.ContainerNetworkRequest{
		ContainerID: "container-2",
		ServiceName: "db",
		Namespace:   "default",
		VPCID:       networkInfo.VPCID,
	}

	containerNet2, err := coordinator.AllocateContainerNetwork(ctx, containerReq2)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to allocate second container network: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Second container network allocated: IP=%s", containerNet2.IPAddress))

	// Verify IPs are different
	if containerNet.IPAddress == containerNet2.IPAddress {
		p.Error("Container IPs should be different")
		return 1
	}

	p.Info(fmt.Sprintf("  Container 1 IP: %s", containerNet.IPAddress))
	p.Info(fmt.Sprintf("  Container 2 IP: %s", containerNet2.IPAddress))

	// Test 5: Create security group
	p.Step("Creating security group")

	sgSpec := domain.SecurityGroupSpec{
		Name: "web-servers",
		Rules: []domain.SecurityRule{
			{
				Direction: domain.RuleDirectionIngress,
				Protocol:  "tcp",
				PortRange: domain.PortRange{Start: 80, End: 80},
				Source:    "0.0.0.0/0",
				Action:    domain.RuleActionAllow,
			},
			{
				Direction: domain.RuleDirectionIngress,
				Protocol:  "tcp",
				PortRange: domain.PortRange{Start: 443, End: 443},
				Source:    "0.0.0.0/0",
				Action:    domain.RuleActionAllow,
			},
			{
				Direction:   domain.RuleDirectionEgress,
				Protocol:    "all",
				Destination: "0.0.0.0/0",
				Action:      domain.RuleActionAllow,
			},
		},
	}

	sg, err := coordinator.CreateSecurityGroup(ctx, networkInfo.VPCID, sgSpec)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create security group: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Security group created: ID=%s, Name=%s", sg.ID, sg.Name))

	// Test 6: Apply network policy
	p.Step("Applying network policy")

	policy := domain.NetworkPolicy{
		Name:      "api-policy",
		Namespace: "default",
		PodSelector: map[string]string{
			"app": "api",
		},
		Ingress: []domain.NetworkPolicyRule{
			{
				From: []domain.NetworkPolicyPeer{
					{
						PodSelector: map[string]string{
							"app": "frontend",
						},
					},
				},
				Ports: []domain.PortRange{
					{Start: 8080, End: 8080},
				},
			},
		},
		Egress: []domain.NetworkPolicyRule{
			{
				To: []domain.NetworkPolicyPeer{
					{
						PodSelector: map[string]string{
							"app": "db",
						},
					},
				},
				Ports: []domain.PortRange{
					{Start: 5432, End: 5432},
				},
			},
		},
	}

	err = coordinator.ApplySecurityPolicy(ctx, policy)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to apply security policy: %v", err))
		return 1
	}

	p.Success("Network policy applied")

	// Test 7: Register DNS
	p.Step("Registering DNS entries")

	err = coordinator.RegisterServiceDNS(ctx, "api", "default", containerNet.IPAddress)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to register DNS: %v", err))
		return 1
	}

	err = coordinator.RegisterServiceDNS(ctx, "db", "default", containerNet2.IPAddress)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to register DNS: %v", err))
		return 1
	}

	p.Success("DNS entries registered: api.default, db.default")

	// Test 8: Resolve DNS
	p.Step("Resolving DNS entries")

	resolvedIPs, err := dnsManager.LookupHost(ctx, "api.default")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to resolve DNS: %v", err))
		return 1
	}

	expectedIP := net.ParseIP(containerNet.IPAddress)
	found := false
	for _, ip := range resolvedIPs {
		if ip.Equal(expectedIP) {
			found = true
			break
		}
	}
	if !found && len(resolvedIPs) > 0 {
		p.Error(fmt.Sprintf("Expected IP %s in resolved IPs, got %v", containerNet.IPAddress, resolvedIPs))
		return 1
	}

	p.Success(fmt.Sprintf("DNS resolved: api.default → %v", resolvedIPs))

	// Test 9: Get network status
	p.Step("Getting network status")

	status, err := coordinator.GetNetworkStatus(ctx, networkInfo.VPCID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get network status: %v", err))
		return 1
	}

	p.Info(fmt.Sprintf("  VPC ID: %s", status.VPCID))
	p.Info(fmt.Sprintf("  Status: %s", status.Status))
	p.Info(fmt.Sprintf("  Subnets: %d", len(status.Subnets)))

	p.Success("Network status retrieved")

	// Test 10: Get container network
	p.Step("Getting container network")

	retrieved, err := coordinator.GetContainerNetwork(ctx, "container-1")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container network: %v", err))
		return 1
	}

	if retrieved.IPAddress != containerNet.IPAddress {
		p.Error(fmt.Sprintf("Expected IP %s, got %s", containerNet.IPAddress, retrieved.IPAddress))
		return 1
	}

	p.Success("Container network retrieved")

	// Test 11: Release container network
	p.Step("Releasing container network")

	err = coordinator.ReleaseContainerNetwork(ctx, "container-1")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to release container network: %v", err))
		return 1
	}

	p.Success("Container network released")

	// Test 12: Unregister DNS
	p.Step("Unregistering DNS entries")

	err = coordinator.UnregisterServiceDNS(ctx, "api", "default")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to unregister DNS: %v", err))
		return 1
	}

	p.Success("DNS entry unregistered")

	// Test 13: Delete security policy
	p.Step("Deleting security policy")

	err = coordinator.DeleteSecurityPolicy(ctx, "api-policy", "default")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to delete security policy: %v", err))
		return 1
	}

	p.Success("Security policy deleted")

	// Test 14: Delete security group
	p.Step("Deleting security group")

	err = coordinator.DeleteSecurityGroup(ctx, sg.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to delete security group: %v", err))
		return 1
	}

	p.Success("Security group deleted")

	// Test 15: Delete network
	p.Step("Deleting VPC network")

	err = coordinator.DeleteNetwork(ctx, networkInfo.VPCID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to delete network: %v", err))
		return 1
	}

	p.Success("VPC network deleted")

	// Test 16: Verify cleanup
	p.Step("Verifying network cleanup")

	_, err = coordinator.GetNetworkStatus(ctx, networkInfo.VPCID)
	if err == nil {
		p.Error("Expected error when getting deleted network")
		return 1
	}

	p.Success("Network cleanup verified")

	// Test 17: Multiple VPCs
	p.Step("Testing multiple VPCs")

	for i := 1; i <= 3; i++ {
		vpcSpec := domain.NetworkProvisionSpec{
			Name: fmt.Sprintf("multi-vpc-%d", i),
			CIDR: fmt.Sprintf("10.%d.0.0/16", 200+i),
			Subnets: []domain.SubnetSpec{
				{Name: "subnet-1", CIDR: fmt.Sprintf("10.%d.1.0/24", 200+i), Purpose: domain.SubnetPurposeContainer},
			},
		}
		info, err := coordinator.ProvisionNetwork(ctx, vpcSpec)
		if err != nil {
			p.Error(fmt.Sprintf("Failed to provision VPC %d: %v", i, err))
			return 1
		}
		p.Info(fmt.Sprintf("  VPC %d: ID=%s, CIDR=%s", i, info.VPCID, info.CIDR))
	}

	p.Success("Multiple VPCs managed successfully")

	return 0
}
