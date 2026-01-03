//go:build ignore

// Network Provisioning Integration Test
//
// This test verifies network provisioning in Engine-Agent communication.
// Scenario: Deployment includes network config → VPC Coordinator provisions →
// Container gets network allocation → Security rules applied → DNS registered
//
// Usage:
//
//	go run ./test/integration/integration/run_network_provisioning_integration.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Engine VPC components
	vpcadapters "github.com/fertile-org/banyan/pkg/engine/vpc/adapters"
	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcuc "github.com/fertile-org/banyan/pkg/engine/vpc/usecases"

	// Engine Registry components
	regadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	regdomain "github.com/fertile-org/banyan/pkg/engine/registry/domain"
	reguc "github.com/fertile-org/banyan/pkg/engine/registry/usecases"

	// Engine State components
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	statedomain "github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateuc "github.com/fertile-org/banyan/pkg/engine/state/usecases"

	"github.com/fertile-org/banyan/test/integration/helpers"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)
	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Network Provisioning Integration Test")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// ======================================================================
	// PART 1: Setup Engine VPC Coordinator
	// ======================================================================

	p.Step("1. Setting up Engine VPC Coordinator")

	networkManager := vpcadapters.NewMemoryNetworkManagerAdapter()
	ipamManager := vpcadapters.NewMemoryIPAMManagerAdapter()
	securityManager := vpcadapters.NewMemorySecurityManagerAdapter()
	dnsManager := vpcadapters.NewMemoryDNSManagerAdapter()
	containerStore := vpcadapters.NewMemoryContainerNetworkStoreAdapter()

	vpcCoordinator := vpcuc.NewVPCCoordinator(networkManager, ipamManager, securityManager, dnsManager, containerStore, logger)
	if vpcCoordinator == nil {
		p.Error("Failed to create VPC Coordinator")
		return 1
	}
	p.Success("Engine VPC Coordinator initialized")

	// ======================================================================
	// PART 2: Setup Agent Registry
	// ======================================================================

	p.Step("2. Registering network-capable agents")

	agentRepo := regadapters.NewMemoryAgentRepository()
	eventPublisher := regadapters.NewMemoryEventPublisher()
	registry := reguc.NewRegistryUseCase(agentRepo, eventPublisher, logger)

	// Register 2 agents with network capability
	agent1, err := registry.RegisterAgent(ctx, &regdomain.RegisterAgentRequest{
		Hostname: "network-node-1",
		Address:  "192.168.1.10:9090",
		Capabilities: []regdomain.Capability{
			{Type: regdomain.CapabilityContainerRuntime, Version: "1.0"},
			{Type: regdomain.CapabilityNetworkNode, Version: "1.0"},
		},
		Resources: regdomain.Resources{
			CPUCores:      4,
			MemoryMB:      8192,
			CPUAvailable:  3.5,
			MemoryFreeMB:  6000,
			MaxContainers: 20,
		},
		Labels:  map[string]string{"role": "network"},
		Version: "1.0.0",
	})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to register agent1: %v", err))
		return 1
	}

	agent2, err := registry.RegisterAgent(ctx, &regdomain.RegisterAgentRequest{
		Hostname: "network-node-2",
		Address:  "192.168.1.11:9090",
		Capabilities: []regdomain.Capability{
			{Type: regdomain.CapabilityContainerRuntime, Version: "1.0"},
			{Type: regdomain.CapabilityNetworkNode, Version: "1.0"},
		},
		Resources: regdomain.Resources{
			CPUCores:      4,
			MemoryMB:      8192,
			CPUAvailable:  3.0,
			MemoryFreeMB:  5500,
			MaxContainers: 20,
		},
		Labels:  map[string]string{"role": "network"},
		Version: "1.0.0",
	})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to register agent2: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Registered 2 network-capable agents: %s, %s", agent1.ID, agent2.ID))

	// ======================================================================
	// PART 3: Setup State Manager
	// ======================================================================

	p.Step("3. Setting up Engine State Manager")
	stateRepo := stateadapters.NewMemoryStateRepository()
	stateUseCase := stateuc.NewStateUseCase(stateRepo)
	if stateUseCase == nil {
		p.Error("Failed to create State UseCase")
		return 1
	}
	p.Success("Engine State Manager initialized")

	// ======================================================================
	// PART 4: Provision VPC Network
	// ======================================================================

	p.Step("4. Provisioning VPC network for deployment")

	vpcSpec := domain.NetworkProvisionSpec{
		Name: "app-vpc",
		CIDR: "10.50.0.0/16",
		Subnets: []domain.SubnetSpec{
			{Name: "container-subnet", CIDR: "10.50.1.0/24", Purpose: domain.SubnetPurposeContainer},
			{Name: "service-subnet", CIDR: "10.50.2.0/24", Purpose: domain.SubnetPurposeService},
		},
		DNSServers: []string{"10.50.0.2"},
	}

	networkInfo, err := vpcCoordinator.ProvisionNetwork(ctx, vpcSpec)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to provision network: %v", err))
		return 1
	}

	p.Info(fmt.Sprintf("  VPC ID: %s", networkInfo.VPCID))
	p.Info(fmt.Sprintf("  CIDR: %s", networkInfo.CIDR))
	p.Info(fmt.Sprintf("  Subnets: %d", len(networkInfo.Subnets)))
	p.Success("VPC network provisioned")

	// ======================================================================
	// PART 5: Set Desired State with Network Config
	// ======================================================================

	p.Step("5. Setting desired state with network configuration")

	deploymentID := "network-app-deployment"
	desiredState := &statedomain.DesiredState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceDesiredState{
			"frontend": {
				Name:      "frontend",
				Image:     "nginx:latest",
				Replicas:  2,
				NetworkID: networkInfo.VPCID,
				Ports: []statedomain.PortMapping{
					{ContainerPort: 80, HostPort: 80, Protocol: "tcp"},
				},
				AgentIDs: []string{string(agent1.ID)},
			},
			"backend": {
				Name:      "backend",
				Image:     "api:v1",
				Replicas:  2,
				NetworkID: networkInfo.VPCID,
				Ports: []statedomain.PortMapping{
					{ContainerPort: 8080, HostPort: 8080, Protocol: "tcp"},
				},
				AgentIDs: []string{string(agent2.ID)},
			},
		},
		UpdatedAt: time.Now(),
	}

	if err := stateUseCase.SetDesiredState(ctx, desiredState); err != nil {
		p.Error(fmt.Sprintf("Failed to set desired state: %v", err))
		return 1
	}
	p.Success("Desired state set with network configuration")

	// ======================================================================
	// PART 6: Allocate Container Networks
	// ======================================================================

	p.Step("6. Allocating container networks")

	containerIDs := []string{"frontend-001", "frontend-002", "backend-001", "backend-002"}
	serviceMap := map[string]string{
		"frontend-001": "frontend",
		"frontend-002": "frontend",
		"backend-001":  "backend",
		"backend-002":  "backend",
	}

	allocatedNetworks := make(map[string]*domain.ContainerNetwork)
	for _, containerID := range containerIDs {
		req := domain.ContainerNetworkRequest{
			ContainerID: containerID,
			ServiceName: serviceMap[containerID],
			Namespace:   "default",
			VPCID:       networkInfo.VPCID,
		}

		net, err := vpcCoordinator.AllocateContainerNetwork(ctx, req)
		if err != nil {
			p.Error(fmt.Sprintf("Failed to allocate network for %s: %v", containerID, err))
			return 1
		}
		allocatedNetworks[containerID] = net
		p.Info(fmt.Sprintf("  %s: IP=%s, Subnet=%s", containerID, net.IPAddress, net.SubnetID))
	}
	p.Success(fmt.Sprintf("Allocated networks for %d containers", len(allocatedNetworks)))

	// Verify unique IPs
	ips := make(map[string]bool)
	for containerID, net := range allocatedNetworks {
		if ips[net.IPAddress] {
			p.Error(fmt.Sprintf("Duplicate IP allocation: %s", net.IPAddress))
			return 1
		}
		ips[net.IPAddress] = true
		_ = containerID
	}
	p.Success("Verified all container IPs are unique")

	// ======================================================================
	// PART 7: Create Security Groups
	// ======================================================================

	p.Step("7. Creating security groups")

	frontendSG, err := vpcCoordinator.CreateSecurityGroup(ctx, networkInfo.VPCID, domain.SecurityGroupSpec{
		Name: "frontend-sg",
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
		},
	})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create frontend security group: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  Frontend SG: %s", frontendSG.ID))

	backendSG, err := vpcCoordinator.CreateSecurityGroup(ctx, networkInfo.VPCID, domain.SecurityGroupSpec{
		Name: "backend-sg",
		Rules: []domain.SecurityRule{
			{
				Direction: domain.RuleDirectionIngress,
				Protocol:  "tcp",
				PortRange: domain.PortRange{Start: 8080, End: 8080},
				Source:    "10.50.1.0/24", // Only from frontend subnet
				Action:    domain.RuleActionAllow,
			},
		},
	})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create backend security group: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  Backend SG: %s", backendSG.ID))
	p.Success("Security groups created")

	// ======================================================================
	// PART 8: Apply Network Policy
	// ======================================================================

	p.Step("8. Applying network policy")

	policy := domain.NetworkPolicy{
		Name:      "frontend-to-backend",
		Namespace: "default",
		PodSelector: map[string]string{
			"service": "frontend",
		},
		Egress: []domain.NetworkPolicyRule{
			{
				To: []domain.NetworkPolicyPeer{
					{
						PodSelector: map[string]string{
							"service": "backend",
						},
					},
				},
				Ports: []domain.PortRange{
					{Start: 8080, End: 8080},
				},
			},
		},
	}

	err = vpcCoordinator.ApplySecurityPolicy(ctx, policy)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to apply network policy: %v", err))
		return 1
	}
	p.Success("Network policy applied: frontend → backend on port 8080")

	// ======================================================================
	// PART 9: Register DNS Entries
	// ======================================================================

	p.Step("9. Registering DNS entries for service discovery")

	// Register each container with DNS
	for containerID, net := range allocatedNetworks {
		serviceName := serviceMap[containerID]
		err = vpcCoordinator.RegisterServiceDNS(ctx, serviceName, "default", net.IPAddress)
		if err != nil {
			p.Error(fmt.Sprintf("Failed to register DNS for %s: %v", containerID, err))
			return 1
		}
	}
	p.Success("DNS entries registered for all containers")

	// ======================================================================
	// PART 10: Resolve DNS
	// ======================================================================

	p.Step("10. Verifying DNS resolution")

	frontendIPs, err := dnsManager.LookupHost(ctx, "frontend.default")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to resolve frontend.default: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  frontend.default → %v", frontendIPs))

	backendIPs, err := dnsManager.LookupHost(ctx, "backend.default")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to resolve backend.default: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  backend.default → %v", backendIPs))
	p.Success("DNS resolution verified")

	// ======================================================================
	// PART 11: Set Actual State
	// ======================================================================

	p.Step("11. Setting actual state with network info")

	var frontendInstances, backendInstances []statedomain.InstanceState
	for containerID, net := range allocatedNetworks {
		instance := statedomain.InstanceState{
			ContainerID: containerID,
			AgentID:     string(agent1.ID),
			IP:          net.IPAddress,
			Status:      statedomain.ContainerRunning,
			Health:      statedomain.HealthHealthy,
			StartedAt:   time.Now(),
		}
		if serviceMap[containerID] == "frontend" {
			frontendInstances = append(frontendInstances, instance)
		} else {
			instance.AgentID = string(agent2.ID)
			backendInstances = append(backendInstances, instance)
		}
	}

	actualState := &statedomain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceActualState{
			"frontend": {
				Name:      "frontend",
				Instances: frontendInstances,
			},
			"backend": {
				Name:      "backend",
				Instances: backendInstances,
			},
		},
		CollectedAt: time.Now(),
	}

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}
	p.Success("Actual state set with network information")

	// ======================================================================
	// PART 12: Verify No Drift
	// ======================================================================

	p.Step("12. Verifying no drift")

	drift, err := stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}
	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Unexpected drift: %d drifts", len(drift.Drifts)))
		return 1
	}
	p.Success("No drift detected - network state matches desired")

	// ======================================================================
	// PART 13: Get Network Status
	// ======================================================================

	p.Step("13. Getting network status")

	status, err := vpcCoordinator.GetNetworkStatus(ctx, networkInfo.VPCID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get network status: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  VPC Status: %s", status.Status))
	p.Info(fmt.Sprintf("  Subnets: %d", len(status.Subnets)))
	p.Success("Network status retrieved")

	// ======================================================================
	// PART 14: Cleanup - Release Networks
	// ======================================================================

	p.Step("14. Releasing container networks")

	for containerID := range allocatedNetworks {
		if err := vpcCoordinator.ReleaseContainerNetwork(ctx, containerID); err != nil {
			p.Error(fmt.Sprintf("Failed to release network for %s: %v", containerID, err))
			return 1
		}
	}
	p.Success("All container networks released")

	// ======================================================================
	// PART 15: Cleanup - Unregister DNS
	// ======================================================================

	p.Step("15. Unregistering DNS entries")

	for _, service := range []string{"frontend", "backend"} {
		if err := vpcCoordinator.UnregisterServiceDNS(ctx, service, "default"); err != nil {
			p.Error(fmt.Sprintf("Failed to unregister DNS for %s: %v", service, err))
			return 1
		}
	}
	p.Success("DNS entries unregistered")

	// ======================================================================
	// PART 16: Cleanup - Delete Security
	// ======================================================================

	p.Step("16. Deleting security resources")

	if err := vpcCoordinator.DeleteSecurityPolicy(ctx, "frontend-to-backend", "default"); err != nil {
		p.Error(fmt.Sprintf("Failed to delete network policy: %v", err))
		return 1
	}
	if err := vpcCoordinator.DeleteSecurityGroup(ctx, frontendSG.ID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete frontend SG: %v", err))
		return 1
	}
	if err := vpcCoordinator.DeleteSecurityGroup(ctx, backendSG.ID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete backend SG: %v", err))
		return 1
	}
	p.Success("Security resources deleted")

	// ======================================================================
	// PART 17: Cleanup - Delete VPC
	// ======================================================================

	p.Step("17. Deleting VPC network")

	if err := vpcCoordinator.DeleteNetwork(ctx, networkInfo.VPCID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete VPC: %v", err))
		return 1
	}
	p.Success("VPC network deleted")

	// ======================================================================
	// PART 18: Cleanup - Deregister Agents
	// ======================================================================

	p.Step("18. Deregistering agents")

	if err := registry.DeregisterAgent(ctx, agent1.ID); err != nil {
		p.Error(fmt.Sprintf("Failed to deregister agent1: %v", err))
		return 1
	}
	if err := registry.DeregisterAgent(ctx, agent2.ID); err != nil {
		p.Error(fmt.Sprintf("Failed to deregister agent2: %v", err))
		return 1
	}
	p.Success("Agents deregistered")

	// ======================================================================
	// PART 19: Cleanup - Delete State
	// ======================================================================

	p.Step("19. Deleting deployment state")

	if err := stateUseCase.DeleteDesiredState(ctx, deploymentID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete desired state: %v", err))
		return 1
	}
	p.Success("Deployment state deleted")

	p.Result(true, "All network provisioning integration tests passed!")
	return 0
}
