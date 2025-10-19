package security

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// IptablesExecutor manages iptables operations
type IptablesExecutor struct {
	dryRun bool // For testing without actual changes
}

// NewIptablesExecutor creates a new IptablesExecutor
func NewIptablesExecutor(dryRun bool) *IptablesExecutor {
	return &IptablesExecutor{
		dryRun: dryRun,
	}
}

// CreateChain creates a custom iptables chain
func (e *IptablesExecutor) CreateChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	// Check if chain already exists
	exists, err := e.chainExists(ctx, table, chain)
	if err != nil {
		return fmt.Errorf("failed to check if chain exists: %w", err)
	}

	if exists {
		return nil // Chain already exists, nothing to do
	}

	args := []string{"-t", table, "-N", chain}

	if e.dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "iptables-legacy", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create chain %s in table %s: %w (stderr: %s)", chain, table, err, stderr.String())
	}

	return nil
}

// FlushChain removes all rules from a chain
func (e *IptablesExecutor) FlushChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	args := []string{"-t", table, "-F", chain}

	if e.dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "iptables-legacy", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to flush chain %s in table %s: %w (stderr: %s)", chain, table, err, stderr.String())
	}

	return nil
}

// DeleteChain deletes a custom iptables chain
func (e *IptablesExecutor) DeleteChain(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	// First flush the chain
	if err := e.FlushChain(ctx, table, chain); err != nil {
		return err
	}

	args := []string{"-t", table, "-X", chain}

	if e.dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "iptables-legacy", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete chain %s in table %s: %w (stderr: %s)", chain, table, err, stderr.String())
	}

	return nil
}

// ApplyRule adds an iptables rule
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

	// Build command: iptables -t <table> -A <chain> <args>
	args := []string{"-t", rule.Table, "-A", rule.Chain}
	args = append(args, rule.Args...)

	if e.dryRun {
		return nil
	}

	cmd := exec.CommandContext(ctx, "iptables-legacy", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply rule to chain %s: %w (stderr: %s)", rule.Chain, err, stderr.String())
	}

	return nil
}

// AddDefaultDeny adds a default deny rule to a chain
func (e *IptablesExecutor) AddDefaultDeny(ctx context.Context, table, chain string) error {
	if table == "" {
		return fmt.Errorf("table cannot be empty")
	}
	if chain == "" {
		return fmt.Errorf("chain cannot be empty")
	}

	rule := IptablesRule{
		Table: table,
		Chain: chain,
		Args:  []string{"-j", "DROP"},
	}

	return e.ApplyRule(ctx, rule)
}

// chainExists checks if a chain exists in the given table
func (e *IptablesExecutor) chainExists(ctx context.Context, table, chain string) (bool, error) {
	if e.dryRun {
		return false, nil
	}

	args := []string{"-t", table, "-L", chain, "-n"}
	cmd := exec.CommandContext(ctx, "iptables-legacy", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If the chain doesn't exist, iptables returns an error
		if strings.Contains(stderr.String(), "No chain/target/match by that name") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check chain existence: %w (stderr: %s)", err, stderr.String())
	}

	return true, nil
}

// JumpToChain creates a rule to jump from one chain to another
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

	rule := IptablesRule{
		Table: table,
		Chain: fromChain,
		Args:  []string{"-j", toChain},
	}

	return e.ApplyRule(ctx, rule)
}
