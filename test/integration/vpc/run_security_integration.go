//go:build ignore

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

// MockServiceResolver for integration tests
type MockSecurityServiceResolver struct {
	services map[string][]net.IP
}

func NewMockSecurityServiceResolver() *MockSecurityServiceResolver {
	return &MockSecurityServiceResolver{
		services: map[string][]net.IP{
			"web": {
				net.ParseIP("10.0.1.10"),
				net.ParseIP("10.0.1.11"),
			},
			"api": {
				net.ParseIP("10.0.2.10"),
			},
			"database": {
				net.ParseIP("10.0.3.10"),
			},
		},
	}
}

func (m *MockSecurityServiceResolver) ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error) {
	if ips, ok := m.services[serviceName]; ok {
		return ips, nil
	}
	return nil, fmt.Errorf("service %q not found", serviceName)
}

// Helper function to check if iptables chain exists
func iptablesChainExists(table, chain string) bool {
	cmd := exec.Command("iptables-legacy", "-t", table, "-L", chain, "-n")
	err := cmd.Run()
	return err == nil
}

// Helper function to list iptables rules in a chain
func listIptablesRules(table, chain string) (string, error) {
	cmd := exec.Command("iptables-legacy", "-t", table, "-L", chain, "-n", "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to list rules: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

// cleanupIptables removes test chains and rules
func cleanupIptables(p *helpers.Printer, networkID string) {
	chainName := fmt.Sprintf("BANYAN-%s", networkID)

	// Remove jump rule from FORWARD chain (ignore errors)
	exec.Command("iptables-legacy", "-t", "filter", "-D", "FORWARD", "-j", chainName).Run()

	// Flush and delete custom chain (ignore errors)
	exec.Command("iptables-legacy", "-t", "filter", "-F", chainName).Run()
	exec.Command("iptables-legacy", "-t", "filter", "-X", chainName).Run()

	p.Info(fmt.Sprintf("Cleaned up iptables chain: %s", chainName))
}

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Check if running as root
	if os.Geteuid() != 0 {
		p.Error("This test requires root privileges")
		p.Info("Please run with: sudo go run test/integration/vpc/security_integration_test.go")
		os.Exit(1)
	}

	exitCode := runSecurityTests(ctx, p)
	if exitCode == 0 {
		p.Result(true, "All security integration tests passed!")
	} else {
		p.Result(false, fmt.Sprintf("Security integration tests failed with exit code %d", exitCode))
	}
	os.Exit(exitCode)
}

func runSecurityTests(ctx context.Context, p *helpers.Printer) int {
	p.Title("Testing Security Manager Integration with iptables")

	resolver := NewMockSecurityServiceResolver()
	manager := security.NewManager(resolver, false) // Not dry-run, actual iptables

	// Test 1: Basic Rule Application
	p.Step("Test 1: Basic Rule Application")
	if !testBasicRuleApplication(ctx, p, manager) {
		return 1
	}

	// Test 2: Service-to-Service Rules
	p.Step("Test 2: Service-to-Service Rules")
	if !testServiceToService(ctx, p, manager) {
		return 1
	}

	// Test 3: Egress Rules
	p.Step("Test 3: Egress Rules")
	if !testEgressRules(ctx, p, manager) {
		return 1
	}

	// Test 4: Multi-Tier Application
	p.Step("Test 4: Multi-Tier Application Security")
	if !testMultiTierSecurity(ctx, p, manager) {
		return 1
	}

	// Test 5: Rule Updates and Cleanup
	p.Step("Test 5: Rule Updates and Cleanup")
	if !testRuleUpdates(ctx, p, manager) {
		return 1
	}

	return 0
}

func testBasicRuleApplication(ctx context.Context, p *helpers.Printer, manager vpc.SecurityManager) bool {
	networkID := "net001" // Short name to avoid chain name length limit
	defer cleanupIptables(p, networkID)

	p.Info("Adding rule: allow internet to web service on port 443")

	rule := &vpc.SecurityRule{
		ID:          "rule-001",
		NetworkID:   networkID,
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "443",
		Protocol:    "tcp",
	}

	if err := manager.AddRule(ctx, rule); err != nil {
		p.Error(fmt.Sprintf("Failed to add rule: %v", err))
		return false
	}
	p.Success("Rule added successfully")

	p.Info("Applying rules to iptables...")
	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules: %v", err))
		return false
	}
	p.Success("Rules applied successfully")

	// Verify chain was created
	chainName := fmt.Sprintf("BANYAN-%s", networkID)
	if !iptablesChainExists("filter", chainName) {
		p.Error(fmt.Sprintf("Chain %s was not created", chainName))
		return false
	}
	p.Success(fmt.Sprintf("Chain %s created successfully", chainName))

	// List and verify rules
	rules, err := listIptablesRules("filter", chainName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list rules: %v", err))
		return false
	}

	p.Code(rules)

	// Verify the rule contains expected elements
	if !strings.Contains(rules, "tcp") {
		p.Error("Expected TCP protocol in rules")
		return false
	}
	if !strings.Contains(rules, "dpt:443") {
		p.Error("Expected destination port 443 in rules")
		return false
	}
	if !strings.Contains(rules, "ACCEPT") {
		p.Error("Expected ACCEPT action in rules")
		return false
	}

	p.Success("✓ Basic rule application test passed")
	return true
}

func testServiceToService(ctx context.Context, p *helpers.Printer, manager vpc.SecurityManager) bool {
	networkID := "net002"
	defer cleanupIptables(p, networkID)

	p.Info("Adding rule: allow API service to access database on port 5432")

	rule := &vpc.SecurityRule{
		ID:          "rule-002",
		NetworkID:   networkID,
		ServiceName: "database",
		Direction:   "ingress",
		Action:      "allow",
		From:        "service:api",
		ToPort:      "5432",
		Protocol:    "tcp",
	}

	if err := manager.AddRule(ctx, rule); err != nil {
		p.Error(fmt.Sprintf("Failed to add rule: %v", err))
		return false
	}

	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules: %v", err))
		return false
	}

	chainName := fmt.Sprintf("BANYAN-%s", networkID)
	rules, err := listIptablesRules("filter", chainName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list rules: %v", err))
		return false
	}

	p.Code(rules)

	// Verify rule includes source IP (from API service)
	if !strings.Contains(rules, "10.0.2.10") {
		p.Error("Expected source IP 10.0.2.10 (API service) in rules")
		return false
	}

	// Verify destination port
	if !strings.Contains(rules, "dpt:5432") {
		p.Error("Expected destination port 5432 in rules")
		return false
	}

	p.Success("✓ Service-to-service rule test passed")
	return true
}

func testEgressRules(ctx context.Context, p *helpers.Printer, manager vpc.SecurityManager) bool {
	networkID := "net003"
	defer cleanupIptables(p, networkID)

	p.Info("Adding egress rule: web service can connect to API on port 8080")

	rule := &vpc.SecurityRule{
		ID:          "rule-003",
		NetworkID:   networkID,
		ServiceName: "web",
		Direction:   "egress",
		Action:      "allow",
		To:          "service:api",
		ToPort:      "8080",
		Protocol:    "tcp",
	}

	if err := manager.AddRule(ctx, rule); err != nil {
		p.Error(fmt.Sprintf("Failed to add rule: %v", err))
		return false
	}

	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules: %v", err))
		return false
	}

	chainName := fmt.Sprintf("BANYAN-%s", networkID)
	rules, err := listIptablesRules("filter", chainName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list rules: %v", err))
		return false
	}

	p.Code(rules)

	// For egress rule, source should be web service IPs
	if !strings.Contains(rules, "10.0.1.10") && !strings.Contains(rules, "10.0.1.11") {
		p.Error("Expected source IP from web service in rules")
		return false
	}

	// Destination should be API service
	if !strings.Contains(rules, "10.0.2.10") {
		p.Error("Expected destination IP 10.0.2.10 (API service) in rules")
		return false
	}

	p.Success("✓ Egress rule test passed")
	return true
}

func testMultiTierSecurity(ctx context.Context, p *helpers.Printer, manager vpc.SecurityManager) bool {
	networkID := "net004"
	defer cleanupIptables(p, networkID)

	p.Info("Setting up multi-tier security: Web → API → Database")

	rules := []*vpc.SecurityRule{
		{
			ID:          "mt-001",
			NetworkID:   networkID,
			ServiceName: "web",
			Direction:   "ingress",
			Action:      "allow",
			From:        "internet",
			ToPort:      "443",
			Protocol:    "tcp",
		},
		{
			ID:          "mt-002",
			NetworkID:   networkID,
			ServiceName: "web",
			Direction:   "egress",
			Action:      "allow",
			To:          "service:api",
			ToPort:      "8080",
			Protocol:    "tcp",
		},
		{
			ID:          "mt-003",
			NetworkID:   networkID,
			ServiceName: "api",
			Direction:   "ingress",
			Action:      "allow",
			From:        "service:web",
			ToPort:      "8080",
			Protocol:    "tcp",
		},
		{
			ID:          "mt-004",
			NetworkID:   networkID,
			ServiceName: "api",
			Direction:   "egress",
			Action:      "allow",
			To:          "service:database",
			ToPort:      "5432",
			Protocol:    "tcp",
		},
		{
			ID:          "mt-005",
			NetworkID:   networkID,
			ServiceName: "database",
			Direction:   "ingress",
			Action:      "allow",
			From:        "service:api",
			ToPort:      "5432",
			Protocol:    "tcp",
		},
	}

	// Add all rules
	for _, rule := range rules {
		if err := manager.AddRule(ctx, rule); err != nil {
			p.Error(fmt.Sprintf("Failed to add rule %s: %v", rule.ID, err))
			return false
		}
	}
	p.Success(fmt.Sprintf("Added %d rules successfully", len(rules)))

	// Apply all rules
	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules: %v", err))
		return false
	}

	chainName := fmt.Sprintf("BANYAN-%s", networkID)
	rulesList, err := listIptablesRules("filter", chainName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list rules: %v", err))
		return false
	}

	p.Code(rulesList)

	// Verify we have ACCEPT rules
	if !strings.Contains(rulesList, "ACCEPT") {
		p.Error("Expected ACCEPT rules in chain")
		return false
	}

	// Verify service IPs are present
	if !strings.Contains(rulesList, "10.0.1") && !strings.Contains(rulesList, "10.0.2") {
		p.Error("Expected service IPs in rules")
		return false
	}

	p.Success("✓ Multi-tier security test passed")
	return true
}

func testRuleUpdates(ctx context.Context, p *helpers.Printer, manager vpc.SecurityManager) bool {
	networkID := "net005"
	defer cleanupIptables(p, networkID)

	p.Info("Testing rule updates and cleanup")

	// Add initial rule
	rule1 := &vpc.SecurityRule{
		ID:          "upd-001",
		NetworkID:   networkID,
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "80",
		Protocol:    "tcp",
	}

	if err := manager.AddRule(ctx, rule1); err != nil {
		p.Error(fmt.Sprintf("Failed to add rule: %v", err))
		return false
	}

	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules first time: %v", err))
		return false
	}
	p.Success("Initial rule applied")

	// Add another rule
	rule2 := &vpc.SecurityRule{
		ID:          "upd-002",
		NetworkID:   networkID,
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "443",
		Protocol:    "tcp",
	}

	if err := manager.AddRule(ctx, rule2); err != nil {
		p.Error(fmt.Sprintf("Failed to add second rule: %v", err))
		return false
	}

	// Apply again (should flush and recreate)
	if err := manager.ApplyRules(ctx, networkID); err != nil {
		p.Error(fmt.Sprintf("Failed to apply rules second time: %v", err))
		return false
	}
	p.Success("Rules updated successfully")

	chainName := fmt.Sprintf("BANYAN-%s", networkID)
	updatedRules, err := listIptablesRules("filter", chainName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list updated rules: %v", err))
		return false
	}

	p.Code(updatedRules)

	// Verify both port 80 and 443 are present
	if !strings.Contains(updatedRules, "dpt:80") {
		p.Error("Expected port 80 in updated rules")
		return false
	}
	if !strings.Contains(updatedRules, "dpt:443") {
		p.Error("Expected port 443 in updated rules")
		return false
	}

	p.Success("✓ Rule update and cleanup test passed")
	return true
}
