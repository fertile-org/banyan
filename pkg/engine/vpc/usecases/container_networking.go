package usecases

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcerrors "github.com/fertile-org/banyan/pkg/engine/vpc/errors"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
)

// ContainerNetworkingUseCase handles container network allocation.
type ContainerNetworkingUseCase struct {
	networkMgr  outbound.NetworkManager
	ipamMgr     outbound.IPAMManager
	securityMgr outbound.SecurityManager
	dnsMgr      outbound.DNSManager
	store       outbound.ContainerNetworkStore
	logger      *slog.Logger
}

// NewContainerNetworkingUseCase creates a new ContainerNetworkingUseCase.
func NewContainerNetworkingUseCase(
	networkMgr outbound.NetworkManager,
	ipamMgr outbound.IPAMManager,
	securityMgr outbound.SecurityManager,
	dnsMgr outbound.DNSManager,
	store outbound.ContainerNetworkStore,
	logger *slog.Logger,
) *ContainerNetworkingUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContainerNetworkingUseCase{
		networkMgr:  networkMgr,
		ipamMgr:     ipamMgr,
		securityMgr: securityMgr,
		dnsMgr:      dnsMgr,
		store:       store,
		logger:      logger,
	}
}

// AllocateContainerNetwork allocates network resources for a container.
func (uc *ContainerNetworkingUseCase) AllocateContainerNetwork(
	ctx context.Context,
	req domain.ContainerNetworkRequest,
) (*domain.ContainerNetwork, error) {
	// Validate VPC exists
	vpc, err := uc.networkMgr.GetVPC(ctx, req.VPCID)
	if err != nil {
		return nil, fmt.Errorf("VPC not found: %w", err)
	}

	// Select subnet (auto-select if not specified)
	subnetID := req.SubnetID
	if subnetID == "" {
		subnetID, err = uc.selectSubnet(ctx, vpc, domain.SubnetPurposeContainer)
		if err != nil {
			return nil, fmt.Errorf("failed to select subnet: %w", err)
		}
	}

	// Get subnet details
	subnet, err := uc.ipamMgr.GetSubnet(ctx, subnetID)
	if err != nil {
		return nil, fmt.Errorf("subnet not found: %w", err)
	}

	// Allocate IP
	ip, err := uc.ipamMgr.AllocateIP(ctx, subnetID, req.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}

	// Generate MAC address
	mac := uc.generateMAC(req.ContainerID)

	// Build container network info
	containerNet := &domain.ContainerNetwork{
		ContainerID:    req.ContainerID,
		IPAddress:      ip,
		Gateway:        subnet.Gateway,
		SubnetCIDR:     subnet.CIDR,
		MAC:            mac,
		DNS:            []string{"10.96.0.10"}, // Default cluster DNS
		SecurityGroups: req.SecurityGroups,
		VPCID:          req.VPCID,
		SubnetID:       subnetID,
	}

	// Persist allocation
	if err := uc.store.Save(ctx, containerNet); err != nil {
		// Rollback IP allocation
		_ = uc.ipamMgr.ReleaseIP(ctx, ip)
		return nil, fmt.Errorf("failed to save allocation: %w", err)
	}

	// Register DNS if service name provided
	if req.ServiceName != "" && uc.dnsMgr != nil {
		hostname := req.ServiceName
		if req.Namespace != "" {
			hostname = fmt.Sprintf("%s.%s", req.ServiceName, req.Namespace)
		}
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil {
			if err := uc.dnsMgr.RegisterHost(ctx, hostname, parsedIP); err != nil {
				uc.logger.Warn("failed to register DNS", "hostname", hostname, "error", err)
			}
		}
	}

	uc.logger.Info("container network allocated",
		"container_id", req.ContainerID,
		"ip", ip,
		"subnet_id", subnetID,
	)

	return containerNet, nil
}

// ReleaseContainerNetwork releases network resources for a container.
func (uc *ContainerNetworkingUseCase) ReleaseContainerNetwork(
	ctx context.Context,
	containerID string,
) error {
	// Get current allocation
	allocation, err := uc.store.Get(ctx, containerID)
	if err != nil {
		return fmt.Errorf("allocation not found: %w", err)
	}

	// Release IP
	if err := uc.ipamMgr.ReleaseIP(ctx, allocation.IPAddress); err != nil {
		uc.logger.Warn("failed to release IP", "ip", allocation.IPAddress, "error", err)
	}

	// Delete allocation record
	if err := uc.store.Delete(ctx, containerID); err != nil {
		return fmt.Errorf("failed to delete allocation: %w", err)
	}

	uc.logger.Info("container network released",
		"container_id", containerID,
		"ip", allocation.IPAddress,
	)

	return nil
}

// UpdateContainerNetwork updates network configuration for a container.
func (uc *ContainerNetworkingUseCase) UpdateContainerNetwork(
	ctx context.Context,
	containerID string,
	updates domain.ContainerNetworkUpdate,
) error {
	allocation, err := uc.store.Get(ctx, containerID)
	if err != nil {
		return fmt.Errorf("allocation not found: %w", err)
	}

	// Update security groups
	if len(updates.SecurityGroups) > 0 {
		allocation.SecurityGroups = updates.SecurityGroups
	}

	if err := uc.store.Save(ctx, allocation); err != nil {
		return fmt.Errorf("failed to update allocation: %w", err)
	}

	uc.logger.Info("container network updated",
		"container_id", containerID,
		"security_groups", updates.SecurityGroups,
	)

	return nil
}

// GetContainerNetwork returns the network allocation for a container.
func (uc *ContainerNetworkingUseCase) GetContainerNetwork(
	ctx context.Context,
	containerID string,
) (*domain.ContainerNetwork, error) {
	return uc.store.Get(ctx, containerID)
}

// ListContainersByNetwork lists all containers in a VPC.
func (uc *ContainerNetworkingUseCase) ListContainersByNetwork(
	ctx context.Context,
	vpcID string,
) ([]string, error) {
	networks, err := uc.store.FindByVPC(ctx, vpcID)
	if err != nil {
		return nil, err
	}

	containerIDs := make([]string, len(networks))
	for i, net := range networks {
		containerIDs[i] = net.ContainerID
	}
	return containerIDs, nil
}

// RegisterServiceDNS registers a service name with an IP for DNS resolution.
func (uc *ContainerNetworkingUseCase) RegisterServiceDNS(
	ctx context.Context,
	serviceName, namespace string,
	ip string,
) error {
	if uc.dnsMgr == nil {
		return nil
	}

	hostname := serviceName
	if namespace != "" {
		hostname = fmt.Sprintf("%s.%s", serviceName, namespace)
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address: %s", ip)
	}

	return uc.dnsMgr.RegisterHost(ctx, hostname, parsedIP)
}

// UnregisterServiceDNS removes a service name from DNS.
func (uc *ContainerNetworkingUseCase) UnregisterServiceDNS(
	ctx context.Context,
	serviceName, namespace string,
) error {
	if uc.dnsMgr == nil {
		return nil
	}

	hostname := serviceName
	if namespace != "" {
		hostname = fmt.Sprintf("%s.%s", serviceName, namespace)
	}

	return uc.dnsMgr.UnregisterHost(ctx, hostname)
}

// selectSubnet finds the best subnet for container allocation.
func (uc *ContainerNetworkingUseCase) selectSubnet(
	ctx context.Context,
	vpc *domain.VPC,
	purpose domain.SubnetPurpose,
) (string, error) {
	// Find subnet with matching purpose and available capacity
	var bestSubnetID string
	maxAvailable := 0

	for _, subnet := range vpc.Subnets {
		if subnet.Purpose != purpose {
			continue
		}

		available, _, err := uc.ipamMgr.GetSubnetCapacity(ctx, subnet.ID)
		if err != nil {
			continue
		}

		if available > maxAvailable {
			maxAvailable = available
			bestSubnetID = subnet.ID
		}
	}

	if bestSubnetID == "" {
		return "", vpcerrors.ErrNoAvailableSubnet
	}

	return bestSubnetID, nil
}

// generateMAC generates a deterministic MAC address from container ID.
func (uc *ContainerNetworkingUseCase) generateMAC(containerID string) string {
	hash := sha256.Sum256([]byte(containerID))
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x",
		hash[0], hash[1], hash[2], hash[3], hash[4])
}
