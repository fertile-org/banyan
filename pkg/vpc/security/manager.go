package security

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/fertile-org/banyan/pkg/vpc"
)

// Manager implements vpc.SecurityManager interface
type Manager struct {
	translator *Translator
	executor   *IptablesExecutor
	rules      map[string]*vpc.SecurityRule // ruleID -> rule
	mu         sync.RWMutex
}

// NewManager creates a new security manager
func NewManager(resolver ServiceResolver, dryRun bool) *Manager {
	return &Manager{
		translator: NewTranslator(resolver),
		executor:   NewIptablesExecutor(dryRun),
		rules:      make(map[string]*vpc.SecurityRule),
	}
}

// AddRule adds a security rule
func (m *Manager) AddRule(ctx context.Context, rule *vpc.SecurityRule) error {
	if rule == nil {
		return fmt.Errorf("rule cannot be nil")
	}

	// Validate rule
	if err := m.validateRule(rule); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate rule ID
	if _, exists := m.rules[rule.ID]; exists {
		return fmt.Errorf("rule with ID %q already exists", rule.ID)
	}

	// Store rule
	m.rules[rule.ID] = rule

	return nil
}

// RemoveRule removes a security rule
func (m *Manager) RemoveRule(ctx context.Context, ruleID string) error {
	if ruleID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if rule exists
	if _, exists := m.rules[ruleID]; !exists {
		return fmt.Errorf("rule with ID %q not found", ruleID)
	}

	// Remove rule
	delete(m.rules, ruleID)

	return nil
}

// ListRules lists all rules for a network
func (m *Manager) ListRules(ctx context.Context, networkID string) ([]*vpc.SecurityRule, error) {
	if networkID == "" {
		return nil, fmt.Errorf("network ID cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var rules []*vpc.SecurityRule
	for _, rule := range m.rules {
		if rule.NetworkID == networkID {
			rules = append(rules, rule)
		}
	}

	return rules, nil
}

// ApplyRules applies rules to the system (iptables)
func (m *Manager) ApplyRules(ctx context.Context, networkID string) error {
	if networkID == "" {
		return fmt.Errorf("network ID cannot be empty")
	}

	// Get rules for this network
	rules, err := m.ListRules(ctx, networkID)
	if err != nil {
		return err
	}

	// Create custom chain for this network
	chainName := fmt.Sprintf("BANYAN-%s", networkID)

	// Create chain (idempotent - won't fail if chain exists)
	if err := m.executor.CreateChain(ctx, "filter", chainName); err != nil {
		return fmt.Errorf("failed to create chain %s: %w", chainName, err)
	}

	// Flush existing rules in chain
	if err := m.executor.FlushChain(ctx, "filter", chainName); err != nil {
		return fmt.Errorf("failed to flush chain %s: %w", chainName, err)
	}

	// Translate and apply each rule
	for _, rule := range rules {
		iptablesRules, err := m.translator.TranslateRule(ctx, rule)
		if err != nil {
			return fmt.Errorf("failed to translate rule %s: %w", rule.ID, err)
		}

		// Apply each translated iptables rule
		for _, iptablesRule := range iptablesRules {
			// Override chain to use our custom chain
			iptablesRule.Chain = chainName

			if err := m.executor.ApplyRule(ctx, iptablesRule); err != nil {
				return fmt.Errorf("failed to apply rule %s: %w", rule.ID, err)
			}
		}
	}

	// Add default deny at the end of the chain (if there are rules)
	if len(rules) > 0 {
		if err := m.executor.AddDefaultDeny(ctx, "filter", chainName); err != nil {
			return fmt.Errorf("failed to add default deny: %w", err)
		}
	}

	// Create jump from FORWARD chain to our custom chain
	// This connects container-to-container traffic to our security rules
	if err := m.executor.JumpToChain(ctx, "filter", "FORWARD", chainName); err != nil {
		return fmt.Errorf("failed to create jump to chain %s: %w", chainName, err)
	}

	return nil
}

// validateRule validates a security rule
func (m *Manager) validateRule(rule *vpc.SecurityRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}

	if rule.NetworkID == "" {
		return fmt.Errorf("network ID cannot be empty")
	}

	// For ingress rules (or default), From must be set
	// For egress rules, To must be set
	if rule.Direction == "egress" {
		if rule.To == "" {
			return fmt.Errorf("to field cannot be empty for egress rules")
		}
		// Validate To format for egress
		if !strings.HasPrefix(rule.To, "service:") &&
			!strings.HasPrefix(rule.To, "cidr:") &&
			rule.To != "internet" {
			return fmt.Errorf("invalid to format %q (use 'internet', 'service:name', or 'cidr:x.x.x.x/y')", rule.To)
		}
		// Validate CIDR if present
		if strings.HasPrefix(rule.To, "cidr:") {
			cidr := strings.TrimPrefix(rule.To, "cidr:")
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid CIDR in to field: %w", err)
			}
		}
	} else {
		// Ingress or default direction
		if rule.From == "" {
			return fmt.Errorf("from field cannot be empty")
		}
		// Validate From format
		if !strings.HasPrefix(rule.From, "service:") &&
			!strings.HasPrefix(rule.From, "cidr:") &&
			rule.From != "internet" {
			return fmt.Errorf("invalid from format %q (use 'internet', 'service:name', or 'cidr:x.x.x.x/y')", rule.From)
		}
		// Validate CIDR if present
		if strings.HasPrefix(rule.From, "cidr:") {
			cidr := strings.TrimPrefix(rule.From, "cidr:")
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return fmt.Errorf("invalid CIDR in from field: %w", err)
			}
		}
	}

	// Validate port format if specified
	if rule.ToPort != "" {
		// Port should be a number or range (e.g., "80" or "8000-8100")
		// For now, we accept any non-empty string (more validation can be added)
		if !isValidPort(rule.ToPort) {
			return fmt.Errorf("invalid port format %q", rule.ToPort)
		}
	}

	// Validate action
	if rule.Action != "allow" && rule.Action != "deny" {
		return fmt.Errorf("action must be 'allow' or 'deny', got %q", rule.Action)
	}

	// Validate direction
	if rule.Direction != "ingress" && rule.Direction != "egress" && rule.Direction != "" {
		return fmt.Errorf("direction must be 'ingress', 'egress', or empty, got %q", rule.Direction)
	}

	return nil
}

// isValidPort checks if a port string is valid
// Accepts: "80", "8000-8100", etc.
func isValidPort(port string) bool {
	if port == "" {
		return false
	}

	// Check if it's a port range (e.g., "8000-8100")
	if strings.Contains(port, "-") {
		parts := strings.Split(port, "-")
		if len(parts) != 2 {
			return false
		}
		// Validate both parts
		var start, end int
		if _, err := fmt.Sscanf(parts[0], "%d", &start); err != nil {
			return false
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &end); err != nil {
			return false
		}
		// Check port range validity
		if start < 1 || start > 65535 || end < 1 || end > 65535 || start > end {
			return false
		}
		return true
	}

	// Single port number
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		return false
	}
	// Valid port range is 1-65535
	return portNum >= 1 && portNum <= 65535
}

// Ensure Manager implements SecurityManager interface
var _ vpc.SecurityManager = (*Manager)(nil)
