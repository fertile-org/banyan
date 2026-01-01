package adapters

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
	"github.com/fertile-org/banyan/pkg/agent/security/ports/outbound"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// VPCSecurityAdapter wraps the VPC module's SecurityManager to implement the security module's SecurityManager interface.
type VPCSecurityAdapter struct {
	vpcSecurityManager vpc.SecurityManager
}

// NewVPCSecurityAdapter creates a new VPCSecurityAdapter.
func NewVPCSecurityAdapter(vpcSecurityManager vpc.SecurityManager) *VPCSecurityAdapter {
	return &VPCSecurityAdapter{
		vpcSecurityManager: vpcSecurityManager,
	}
}

// AddRule adds a security rule to the backend.
func (a *VPCSecurityAdapter) AddRule(ctx context.Context, rule *domain.SecurityRule) error {
	vpcRule := a.toVPCRule(rule)
	return a.vpcSecurityManager.AddRule(ctx, vpcRule)
}

// RemoveRule removes a security rule from the backend.
func (a *VPCSecurityAdapter) RemoveRule(ctx context.Context, ruleID domain.RuleID) error {
	return a.vpcSecurityManager.RemoveRule(ctx, string(ruleID))
}

// ListRules lists all security rules for a network.
func (a *VPCSecurityAdapter) ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error) {
	vpcRules, err := a.vpcSecurityManager.ListRules(ctx, string(networkID))
	if err != nil {
		return nil, err
	}

	rules := make([]*domain.SecurityRule, len(vpcRules))
	for i, vpcRule := range vpcRules {
		rules[i] = a.fromVPCRule(vpcRule)
	}

	return rules, nil
}

// ApplyRules applies rules to the system (iptables).
func (a *VPCSecurityAdapter) ApplyRules(ctx context.Context, networkID domain.NetworkID) error {
	return a.vpcSecurityManager.ApplyRules(ctx, string(networkID))
}

// toVPCRule converts a domain rule to a VPC rule.
func (a *VPCSecurityAdapter) toVPCRule(rule *domain.SecurityRule) *vpc.SecurityRule {
	return &vpc.SecurityRule{
		ID:          string(rule.ID),
		NetworkID:   string(rule.NetworkID),
		ServiceName: rule.ServiceName,
		Direction:   string(rule.Direction),
		Action:      string(rule.Action),
		From:        rule.From,
		To:          rule.To,
		ToPort:      rule.ToPort,
		Protocol:    string(rule.Protocol),
	}
}

// fromVPCRule converts a VPC rule to a domain rule.
func (a *VPCSecurityAdapter) fromVPCRule(vpcRule *vpc.SecurityRule) *domain.SecurityRule {
	return &domain.SecurityRule{
		ID:          domain.RuleID(vpcRule.ID),
		NetworkID:   domain.NetworkID(vpcRule.NetworkID),
		ServiceName: vpcRule.ServiceName,
		Direction:   domain.Direction(vpcRule.Direction),
		Action:      domain.Action(vpcRule.Action),
		From:        vpcRule.From,
		To:          vpcRule.To,
		ToPort:      vpcRule.ToPort,
		Protocol:    domain.Protocol(vpcRule.Protocol),
	}
}

// Ensure VPCSecurityAdapter implements outbound.SecurityManager
var _ outbound.SecurityManager = (*VPCSecurityAdapter)(nil)
