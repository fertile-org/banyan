package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
)

// SecurityService is the inbound port for security operations.
// It provides the interface for managing security rules and policies.
type SecurityService interface {
	// AddRule adds a security rule.
	AddRule(ctx context.Context, rule *domain.SecurityRule) error

	// RemoveRule removes a security rule by ID.
	RemoveRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error

	// GetRule gets a security rule by ID.
	GetRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error)

	// ListRules lists all security rules for a network.
	ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error)

	// ApplyPolicy applies security rules to the system (iptables).
	ApplyPolicy(ctx context.Context, networkID domain.NetworkID) error

	// GetPolicyStatus gets the current status of a network's security policy.
	GetPolicyStatus(ctx context.Context, networkID domain.NetworkID) (*domain.PolicyStatus, error)
}
