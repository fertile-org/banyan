package adapters

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	containerdomain "github.com/fertile-org/banyan/pkg/agent/container/domain"
	containerinbound "github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	healthdomain "github.com/fertile-org/banyan/pkg/agent/health/domain"
	healthinbound "github.com/fertile-org/banyan/pkg/agent/health/ports/inbound"
	networkdomain "github.com/fertile-org/banyan/pkg/agent/network/domain"
	networkinbound "github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"
	securitydomain "github.com/fertile-org/banyan/pkg/agent/security/domain"
	securityinbound "github.com/fertile-org/banyan/pkg/agent/security/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
)

// ContainerServiceAdapter adapts the container service to the task executor's interface.
type ContainerServiceAdapter struct {
	service containerinbound.ContainerService
}

// NewContainerServiceAdapter creates a new container service adapter.
func NewContainerServiceAdapter(service containerinbound.ContainerService) *ContainerServiceAdapter {
	return &ContainerServiceAdapter{service: service}
}

// CreateContainer creates a new container.
func (a *ContainerServiceAdapter) CreateContainer(ctx context.Context, spec *outbound.ContainerSpec) (*outbound.ContainerInfo, error) {
	// Parse image reference
	imageRef, err := containerdomain.ParseImageRef(spec.Image)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %q: %w", spec.Image, err)
	}

	// Convert environment map to EnvVar slice
	var envVars []containerdomain.EnvVar
	for k, v := range spec.Environment {
		envVars = append(envVars, containerdomain.EnvVar{Name: k, Value: v})
	}

	// Convert to container service spec
	containerSpec := &containerinbound.ContainerSpec{
		Name: spec.Name,
		Config: containerdomain.ContainerConfig{
			Image:   imageRef,
			Command: spec.Command,
			Env:     envVars,
			Labels:  spec.Labels,
		},
	}

	info, err := a.service.CreateContainer(ctx, containerSpec)
	if err != nil {
		return nil, err
	}

	return &outbound.ContainerInfo{
		ID:     string(info.ID),
		Name:   info.Name,
		State:  string(info.State),
		Status: fmt.Sprintf("Health: %s", info.Health),
	}, nil
}

// StartContainer starts a container.
func (a *ContainerServiceAdapter) StartContainer(ctx context.Context, containerID string) error {
	return a.service.StartContainer(ctx, containerdomain.ContainerID(containerID))
}

// StopContainer stops a container.
func (a *ContainerServiceAdapter) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	return a.service.StopContainer(ctx, containerdomain.ContainerID(containerID), timeout)
}

// RemoveContainer removes a container.
func (a *ContainerServiceAdapter) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	return a.service.RemoveContainer(ctx, containerdomain.ContainerID(containerID), force)
}

// RestartContainer restarts a container.
func (a *ContainerServiceAdapter) RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	return a.service.RestartContainer(ctx, containerdomain.ContainerID(containerID), timeout)
}

// GetContainer gets container info.
func (a *ContainerServiceAdapter) GetContainer(ctx context.Context, containerID string) (*outbound.ContainerInfo, error) {
	info, err := a.service.GetContainer(ctx, containerdomain.ContainerID(containerID))
	if err != nil {
		return nil, err
	}

	return &outbound.ContainerInfo{
		ID:     string(info.ID),
		Name:   info.Name,
		State:  string(info.State),
		Status: fmt.Sprintf("Health: %s", info.Health),
	}, nil
}

// Ensure ContainerServiceAdapter implements outbound.ContainerService
var _ outbound.ContainerService = (*ContainerServiceAdapter)(nil)

// NetworkServiceAdapter adapts the network service to the task executor's interface.
type NetworkServiceAdapter struct {
	service networkinbound.NetworkService
}

// NewNetworkServiceAdapter creates a new network service adapter.
func NewNetworkServiceAdapter(service networkinbound.NetworkService) *NetworkServiceAdapter {
	return &NetworkServiceAdapter{service: service}
}

// ConnectContainer connects a container to a network.
func (a *NetworkServiceAdapter) ConnectContainer(ctx context.Context, containerID, networkID, ipAddress string) error {
	var ip net.IP
	if ipAddress != "" {
		ip = net.ParseIP(ipAddress)
	}

	opts := &networkinbound.ConnectOptions{
		IPAddress: ip,
	}
	_, err := a.service.ConnectContainer(
		ctx,
		networkdomain.ContainerID(containerID),
		networkdomain.NetworkID(networkID),
		opts,
	)
	return err
}

// DisconnectContainer disconnects a container from a network.
func (a *NetworkServiceAdapter) DisconnectContainer(ctx context.Context, containerID, networkID string) error {
	return a.service.DisconnectContainer(
		ctx,
		networkdomain.ContainerID(containerID),
		networkdomain.NetworkID(networkID),
	)
}

// GetContainerNetwork gets the network configuration for a container.
func (a *NetworkServiceAdapter) GetContainerNetwork(ctx context.Context, containerID string) (*outbound.ContainerNetworkInfo, error) {
	endpoints, err := a.service.ListEndpoints(ctx, networkdomain.ContainerID(containerID))
	if err != nil {
		return nil, err
	}

	if len(endpoints) == 0 {
		return nil, nil
	}

	// Return the first endpoint's info
	ep := endpoints[0]
	return &outbound.ContainerNetworkInfo{
		ContainerID: containerID,
		NetworkID:   string(ep.NetworkID),
		IPAddress:   ep.IPAddress.String(),
		Gateway:     ep.Gateway.String(),
		MacAddress:  ep.MacAddress,
	}, nil
}

// Ensure NetworkServiceAdapter implements outbound.NetworkService
var _ outbound.NetworkService = (*NetworkServiceAdapter)(nil)

// SecurityServiceAdapter adapts the security service to the task executor's interface.
type SecurityServiceAdapter struct {
	service securityinbound.SecurityService
}

// NewSecurityServiceAdapter creates a new security service adapter.
func NewSecurityServiceAdapter(service securityinbound.SecurityService) *SecurityServiceAdapter {
	return &SecurityServiceAdapter{service: service}
}

// ApplyPolicy applies a security policy.
func (a *SecurityServiceAdapter) ApplyPolicy(ctx context.Context, policy *outbound.SecurityPolicy) error {
	// First add the rules
	for i, rule := range policy.Rules {
		// Generate a unique rule ID
		ruleID := fmt.Sprintf("%s-%s-%d", policy.ID, rule.Direction, i)

		// Build ToPort from Port or PortRange
		toPort := rule.PortRange
		if toPort == "" && rule.Port > 0 {
			toPort = strconv.Itoa(rule.Port)
		}

		domainRule := &securitydomain.SecurityRule{
			ID:        securitydomain.RuleID(ruleID),
			NetworkID: securitydomain.NetworkID(policy.ID),
			Direction: securitydomain.Direction(rule.Direction),
			Protocol:  securitydomain.Protocol(rule.Protocol),
			ToPort:    toPort,
			From:      rule.Source,
			Action:    securitydomain.Action(rule.Action),
		}
		if err := a.service.AddRule(ctx, domainRule); err != nil {
			return err
		}
	}

	// Then apply the policy
	return a.service.ApplyPolicy(ctx, securitydomain.NetworkID(policy.ID))
}

// RemovePolicy removes a security policy.
func (a *SecurityServiceAdapter) RemovePolicy(ctx context.Context, policyID string) error {
	// List all rules for this network and remove them
	rules, err := a.service.ListRules(ctx, securitydomain.NetworkID(policyID))
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if err := a.service.RemoveRule(ctx, securitydomain.NetworkID(policyID), rule.ID); err != nil {
			return err
		}
	}

	return nil
}

// GetPolicyStatus gets the status of a security policy.
func (a *SecurityServiceAdapter) GetPolicyStatus(ctx context.Context, policyID string) (*outbound.PolicyStatus, error) {
	status, err := a.service.GetPolicyStatus(ctx, securitydomain.NetworkID(policyID))
	if err != nil {
		return nil, err
	}

	return &outbound.PolicyStatus{
		PolicyID: policyID,
		Applied:  status.Applied,
		Error:    status.Error,
	}, nil
}

// Ensure SecurityServiceAdapter implements outbound.SecurityService
var _ outbound.SecurityService = (*SecurityServiceAdapter)(nil)

// HealthServiceAdapter adapts the health service to the task executor's interface.
type HealthServiceAdapter struct {
	service healthinbound.HealthService
}

// NewHealthServiceAdapter creates a new health service adapter.
func NewHealthServiceAdapter(service healthinbound.HealthService) *HealthServiceAdapter {
	return &HealthServiceAdapter{service: service}
}

// RunHealthCheck runs a health check for a container.
func (a *HealthServiceAdapter) RunHealthCheck(ctx context.Context, containerID, checkType, target string, timeout time.Duration) (*outbound.HealthCheckResult, error) {
	// First register a temporary check
	checkID := healthdomain.CheckID("temp-" + containerID)
	check := &healthdomain.HealthCheck{
		ID:          checkID,
		ContainerID: healthdomain.ContainerID(containerID),
		Type:        healthdomain.CheckType(checkType),
		Target:      target,
		Timeout:     timeout,
	}

	if err := a.service.RegisterCheck(ctx, check); err != nil {
		return nil, err
	}

	// Run the check
	result, err := a.service.RunCheck(ctx, healthdomain.ContainerID(containerID), checkID)
	if err != nil {
		// Cleanup
		_ = a.service.UnregisterCheck(ctx, healthdomain.ContainerID(containerID), checkID)
		return nil, err
	}

	// Cleanup
	_ = a.service.UnregisterCheck(ctx, healthdomain.ContainerID(containerID), checkID)

	return &outbound.HealthCheckResult{
		Healthy:  result.Status == healthdomain.HealthStatusHealthy,
		Message:  result.Output,
		Duration: result.Duration,
	}, nil
}

// Ensure HealthServiceAdapter implements outbound.HealthService
var _ outbound.HealthService = (*HealthServiceAdapter)(nil)
