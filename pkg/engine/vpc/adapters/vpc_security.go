package adapters

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/vpc/domain"
	vpcerrors "github.com/fertile-org/banyan/pkg/engine/vpc/errors"
	"github.com/fertile-org/banyan/pkg/engine/vpc/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/security"
)

// noopServiceResolver is a minimal ServiceResolver implementation.
type noopServiceResolver struct{}

// ResolveServiceIPs returns an empty list (no-op implementation).
func (r *noopServiceResolver) ResolveServiceIPs(_ context.Context, _ string) ([]net.IP, error) {
	return nil, nil
}

// VPCSecurityManagerAdapter wraps the pkg/vpc SecurityManager.
type VPCSecurityManagerAdapter struct {
	manager         *security.Manager
	securityGroups  map[string]*domain.SecurityGroup
	networkPolicies map[string]*domain.NetworkPolicy // key: namespace/name
	mu              sync.RWMutex
}

// NewVPCSecurityManagerAdapter creates a new VPCSecurityManagerAdapter.
func NewVPCSecurityManagerAdapter() *VPCSecurityManagerAdapter {
	return &VPCSecurityManagerAdapter{
		manager:         security.NewManager(&noopServiceResolver{}, true), // dryRun=true for now
		securityGroups:  make(map[string]*domain.SecurityGroup),
		networkPolicies: make(map[string]*domain.NetworkPolicy),
	}
}

// CreateSecurityGroup creates a new security group.
func (a *VPCSecurityManagerAdapter) CreateSecurityGroup(
	ctx context.Context,
	vpcID string,
	spec domain.SecurityGroupSpec,
) (*domain.SecurityGroup, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	sgID := fmt.Sprintf("%s-sg-%s", vpcID, spec.Name)

	sg := &domain.SecurityGroup{
		ID:        sgID,
		Name:      spec.Name,
		VPCID:     vpcID,
		Rules:     spec.Rules,
		CreatedAt: time.Now(),
	}

	// Convert and add rules to the underlying manager
	for _, rule := range spec.Rules {
		vpcRule := a.convertToVPCRule(vpcID, rule)
		if err := a.manager.AddRule(ctx, vpcRule); err != nil {
			return nil, err
		}
	}

	a.securityGroups[sgID] = sg
	return sg, nil
}

// GetSecurityGroup retrieves a security group by ID.
func (a *VPCSecurityManagerAdapter) GetSecurityGroup(_ context.Context, id string) (*domain.SecurityGroup, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sg, ok := a.securityGroups[id]
	if !ok {
		return nil, vpcerrors.ErrSecurityGroupNotFound
	}
	return sg, nil
}

// UpdateSecurityGroup updates security group rules.
func (a *VPCSecurityManagerAdapter) UpdateSecurityGroup(
	ctx context.Context,
	id string,
	rules []domain.SecurityRule,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	sg, ok := a.securityGroups[id]
	if !ok {
		return vpcerrors.ErrSecurityGroupNotFound
	}

	sg.Rules = rules

	// Re-apply rules
	if err := a.manager.ApplyRules(ctx, sg.VPCID); err != nil {
		return err
	}

	return nil
}

// DeleteSecurityGroup deletes a security group.
func (a *VPCSecurityManagerAdapter) DeleteSecurityGroup(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.securityGroups, id)
	return nil
}

// ApplyNetworkPolicy applies a network policy.
func (a *VPCSecurityManagerAdapter) ApplyNetworkPolicy(
	ctx context.Context,
	policy domain.NetworkPolicy,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)
	a.networkPolicies[key] = &policy

	// Convert network policy to security rules and apply
	// This is a simplified implementation
	return nil
}

// DeleteNetworkPolicy removes a network policy.
func (a *VPCSecurityManagerAdapter) DeleteNetworkPolicy(
	_ context.Context,
	name, namespace string,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	delete(a.networkPolicies, key)
	return nil
}

// convertToVPCRule converts a domain security rule to a VPC security rule.
func (a *VPCSecurityManagerAdapter) convertToVPCRule(networkID string, rule domain.SecurityRule) *vpc.SecurityRule {
	direction := "ingress"
	if rule.Direction == domain.RuleDirectionEgress {
		direction = "egress"
	}

	action := "allow"
	if rule.Action == domain.RuleActionDeny {
		action = "deny"
	}

	// Convert port range to string
	portStr := ""
	if rule.PortRange.Start > 0 {
		if rule.PortRange.End > 0 && rule.PortRange.End != rule.PortRange.Start {
			portStr = strconv.Itoa(rule.PortRange.Start) + "-" + strconv.Itoa(rule.PortRange.End)
		} else {
			portStr = strconv.Itoa(rule.PortRange.Start)
		}
	}

	return &vpc.SecurityRule{
		NetworkID: networkID,
		Direction: direction,
		Action:    action,
		Protocol:  rule.Protocol,
		From:      rule.Source,
		To:        rule.Destination,
		ToPort:    portStr,
	}
}

// Ensure VPCSecurityManagerAdapter implements SecurityManager.
var _ outbound.SecurityManager = (*VPCSecurityManagerAdapter)(nil)

// MemorySecurityManagerAdapter is an in-memory implementation for testing.
type MemorySecurityManagerAdapter struct {
	securityGroups  map[string]*domain.SecurityGroup
	networkPolicies map[string]*domain.NetworkPolicy
	mu              sync.RWMutex
	nextID          int
}

// NewMemorySecurityManagerAdapter creates a new in-memory security manager.
func NewMemorySecurityManagerAdapter() *MemorySecurityManagerAdapter {
	return &MemorySecurityManagerAdapter{
		securityGroups:  make(map[string]*domain.SecurityGroup),
		networkPolicies: make(map[string]*domain.NetworkPolicy),
	}
}

// CreateSecurityGroup creates a new security group in memory.
func (a *MemorySecurityManagerAdapter) CreateSecurityGroup(
	_ context.Context,
	vpcID string,
	spec domain.SecurityGroupSpec,
) (*domain.SecurityGroup, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.nextID++
	sgID := fmt.Sprintf("sg-%d", a.nextID)

	sg := &domain.SecurityGroup{
		ID:        sgID,
		Name:      spec.Name,
		VPCID:     vpcID,
		Rules:     spec.Rules,
		CreatedAt: time.Now(),
	}

	a.securityGroups[sgID] = sg
	return sg, nil
}

// GetSecurityGroup retrieves a security group by ID.
func (a *MemorySecurityManagerAdapter) GetSecurityGroup(_ context.Context, id string) (*domain.SecurityGroup, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sg, ok := a.securityGroups[id]
	if !ok {
		return nil, vpcerrors.ErrSecurityGroupNotFound
	}
	return sg, nil
}

// UpdateSecurityGroup updates security group rules.
func (a *MemorySecurityManagerAdapter) UpdateSecurityGroup(
	_ context.Context,
	id string,
	rules []domain.SecurityRule,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	sg, ok := a.securityGroups[id]
	if !ok {
		return vpcerrors.ErrSecurityGroupNotFound
	}

	sg.Rules = rules
	return nil
}

// DeleteSecurityGroup deletes a security group.
func (a *MemorySecurityManagerAdapter) DeleteSecurityGroup(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.securityGroups, id)
	return nil
}

// ApplyNetworkPolicy applies a network policy.
func (a *MemorySecurityManagerAdapter) ApplyNetworkPolicy(
	_ context.Context,
	policy domain.NetworkPolicy,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%s/%s", policy.Namespace, policy.Name)
	a.networkPolicies[key] = &policy
	return nil
}

// DeleteNetworkPolicy removes a network policy.
func (a *MemorySecurityManagerAdapter) DeleteNetworkPolicy(
	_ context.Context,
	name, namespace string,
) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	delete(a.networkPolicies, key)
	return nil
}

// Ensure MemorySecurityManagerAdapter implements SecurityManager.
var _ outbound.SecurityManager = (*MemorySecurityManagerAdapter)(nil)
