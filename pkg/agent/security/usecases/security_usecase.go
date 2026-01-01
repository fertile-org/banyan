package usecases

import (
	"context"
	"fmt"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
	"github.com/fertile-org/banyan/pkg/agent/security/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/security/ports/outbound"
)

// SecurityUseCase implements the SecurityService interface.
type SecurityUseCase struct {
	securityManager outbound.SecurityManager
	ruleStore       outbound.RuleStore
}

// NewSecurityUseCase creates a new SecurityUseCase.
func NewSecurityUseCase(
	securityManager outbound.SecurityManager,
	ruleStore outbound.RuleStore,
) *SecurityUseCase {
	return &SecurityUseCase{
		securityManager: securityManager,
		ruleStore:       ruleStore,
	}
}

// AddRule adds a security rule.
func (uc *SecurityUseCase) AddRule(ctx context.Context, rule *domain.SecurityRule) error {
	if err := uc.validateRule(rule); err != nil {
		return err
	}

	// Check if rule already exists
	existing, err := uc.ruleStore.GetRule(ctx, rule.NetworkID, rule.ID)
	if err == nil && existing != nil {
		return domain.ErrRuleAlreadyExists
	}

	// Save to store
	if err := uc.ruleStore.SaveRule(ctx, rule); err != nil {
		return fmt.Errorf("failed to save rule: %w", err)
	}

	// Add to backend
	if err := uc.securityManager.AddRule(ctx, rule); err != nil {
		// Rollback store on failure
		_ = uc.ruleStore.DeleteRule(ctx, rule.NetworkID, rule.ID)
		return fmt.Errorf("failed to add rule to backend: %w", err)
	}

	return nil
}

// RemoveRule removes a security rule by ID.
func (uc *SecurityUseCase) RemoveRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error {
	if networkID == "" {
		return domain.ErrInvalidNetworkID
	}

	// Check if rule exists
	_, err := uc.ruleStore.GetRule(ctx, networkID, ruleID)
	if err != nil {
		return domain.ErrRuleNotFound
	}

	// Remove from backend
	if err := uc.securityManager.RemoveRule(ctx, ruleID); err != nil {
		return fmt.Errorf("failed to remove rule from backend: %w", err)
	}

	// Delete from store
	if err := uc.ruleStore.DeleteRule(ctx, networkID, ruleID); err != nil {
		return fmt.Errorf("failed to delete rule from store: %w", err)
	}

	return nil
}

// GetRule gets a security rule by ID.
func (uc *SecurityUseCase) GetRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error) {
	if networkID == "" {
		return nil, domain.ErrInvalidNetworkID
	}

	rule, err := uc.ruleStore.GetRule(ctx, networkID, ruleID)
	if err != nil {
		return nil, domain.ErrRuleNotFound
	}

	return rule, nil
}

// ListRules lists all security rules for a network.
func (uc *SecurityUseCase) ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error) {
	if networkID == "" {
		return nil, domain.ErrInvalidNetworkID
	}

	return uc.ruleStore.ListRules(ctx, networkID)
}

// ApplyPolicy applies security rules to the system (iptables).
func (uc *SecurityUseCase) ApplyPolicy(ctx context.Context, networkID domain.NetworkID) error {
	if networkID == "" {
		return domain.ErrInvalidNetworkID
	}

	if err := uc.securityManager.ApplyRules(ctx, networkID); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrApplyFailed, err)
	}

	return nil
}

// GetPolicyStatus gets the current status of a network's security policy.
func (uc *SecurityUseCase) GetPolicyStatus(ctx context.Context, networkID domain.NetworkID) (*domain.PolicyStatus, error) {
	if networkID == "" {
		return nil, domain.ErrInvalidNetworkID
	}

	rules, err := uc.ruleStore.ListRules(ctx, networkID)
	if err != nil {
		return &domain.PolicyStatus{
			NetworkID: networkID,
			Applied:   false,
			Error:     err.Error(),
		}, nil
	}

	return &domain.PolicyStatus{
		NetworkID: networkID,
		RuleCount: len(rules),
		Applied:   true,
	}, nil
}

// validateRule validates a security rule.
func (uc *SecurityUseCase) validateRule(rule *domain.SecurityRule) error {
	if rule == nil {
		return domain.ErrInvalidRule
	}

	if rule.NetworkID == "" {
		return domain.ErrInvalidNetworkID
	}

	if rule.Direction != domain.DirectionIngress && rule.Direction != domain.DirectionEgress {
		return domain.ErrInvalidDirection
	}

	if rule.Action != domain.ActionAllow && rule.Action != domain.ActionDeny {
		return domain.ErrInvalidAction
	}

	return nil
}

// Ensure SecurityUseCase implements inbound.SecurityService
var _ inbound.SecurityService = (*SecurityUseCase)(nil)
