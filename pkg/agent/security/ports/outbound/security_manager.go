package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
)

// SecurityManager interfaces with the underlying security system (VPC security module).
type SecurityManager interface {
	// AddRule adds a security rule to the backend.
	AddRule(ctx context.Context, rule *domain.SecurityRule) error

	// RemoveRule removes a security rule from the backend.
	RemoveRule(ctx context.Context, ruleID domain.RuleID) error

	// ListRules lists all security rules for a network.
	ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error)

	// ApplyRules applies rules to the system (iptables).
	ApplyRules(ctx context.Context, networkID domain.NetworkID) error
}

// RuleStore persists security rule information.
type RuleStore interface {
	// SaveRule saves a security rule.
	SaveRule(ctx context.Context, rule *domain.SecurityRule) error

	// GetRule retrieves a security rule by ID.
	GetRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error)

	// DeleteRule deletes a security rule.
	DeleteRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error

	// ListRules lists all rules for a network.
	ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error)
}
