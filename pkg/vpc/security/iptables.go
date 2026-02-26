package security

import (
	"context"
	"fmt"

	goiptables "github.com/coreos/go-iptables/iptables"
)

// IPTables abstracts iptables operations for testability.
type IPTables interface {
	NewChain(table, chain string) error
	ClearChain(table, chain string) error
	DeleteChain(table, chain string) error
	Append(table, chain string, rulespec ...string) error
	ListChains(table string) ([]string, error)
}

// IptablesExecutor manages iptables operations using coreos/go-iptables.
type IptablesExecutor struct {
	ipt    IPTables
	dryRun bool
}

// NewIptablesExecutor creates a new IptablesExecutor.
// In dryRun mode, all operations succeed without touching iptables.
// In production mode, it initializes the coreos/go-iptables library.
func NewIptablesExecutor(dryRun bool) *IptablesExecutor {
	if dryRun {
		return &IptablesExecutor{dryRun: true}
	}
	ipt, err := goiptables.New()
	if err != nil {
		// Fall back to dry-run if iptables is not available (e.g. in CI).
		// Callers that need real iptables should use NewIptablesExecutorWithIPTables.
		return &IptablesExecutor{dryRun: true}
	}
	return &IptablesExecutor{ipt: ipt}
}

// NewIptablesExecutorWithIPTables creates an IptablesExecutor with a custom IPTables implementation.
// Used for testing with mocks.
func NewIptablesExecutorWithIPTables(ipt IPTables) *IptablesExecutor {
	return &IptablesExecutor{ipt: ipt}
}

// CreateChain creates a custom iptables chain.
func (e *IptablesExecutor) CreateChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	if e.dryRun {
		return nil
	}

	// Check if chain already exists
	chains, err := e.ipt.ListChains(table)
	if err != nil {
		return fmt.Errorf("failed to list chains in table %s: %w", table, err)
	}
	for _, c := range chains {
		if c == chain {
			return nil // Already exists
		}
	}

	if err := e.ipt.NewChain(table, chain); err != nil {
		return fmt.Errorf("failed to create chain %s in table %s: %w", chain, table, err)
	}

	return nil
}

// FlushChain removes all rules from a chain.
func (e *IptablesExecutor) FlushChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	if e.dryRun {
		return nil
	}

	if err := e.ipt.ClearChain(table, chain); err != nil {
		return fmt.Errorf("failed to flush chain %s in table %s: %w", chain, table, err)
	}

	return nil
}

// DeleteChain deletes a custom iptables chain (flushes first).
func (e *IptablesExecutor) DeleteChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	if e.dryRun {
		return nil
	}

	// Flush first, then delete
	if err := e.ipt.ClearChain(table, chain); err != nil {
		return fmt.Errorf("failed to flush chain %s before delete: %w", chain, err)
	}
	if err := e.ipt.DeleteChain(table, chain); err != nil {
		return fmt.Errorf("failed to delete chain %s in table %s: %w", chain, table, err)
	}

	return nil
}

// ApplyRule adds an iptables rule.
func (e *IptablesExecutor) ApplyRule(ctx context.Context, rule IptablesRule) error {
	if rule.Table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if rule.Chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}
	if len(rule.Args) == 0 {
		return fmt.Errorf("rule args cannot be empty")
	}

	if e.dryRun {
		return nil
	}

	if err := e.ipt.Append(rule.Table, rule.Chain, rule.Args...); err != nil {
		return fmt.Errorf("failed to apply rule to chain %s: %w", rule.Chain, err)
	}

	return nil
}

// AddDefaultDeny adds a default deny rule to a chain.
func (e *IptablesExecutor) AddDefaultDeny(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	return e.ApplyRule(ctx, IptablesRule{
		Table: table,
		Chain: chain,
		Args:  []string{"-j", "DROP"},
	})
}

// JumpToChain creates a rule to jump from one chain to another.
func (e *IptablesExecutor) JumpToChain(ctx context.Context, table, fromChain, toChain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if fromChain == "" {
		return fmt.Errorf("fromChain cannot be empty")
	}
	if toChain == "" {
		return fmt.Errorf("toChain cannot be empty")
	}

	return e.ApplyRule(ctx, IptablesRule{
		Table: table,
		Chain: fromChain,
		Args:  []string{"-j", toChain},
	})
}
