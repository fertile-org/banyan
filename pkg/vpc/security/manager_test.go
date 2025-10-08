package security_test

import (
	"context"
	"testing"

	"github.com/fertile/banyan/pkg/vpc"
	"github.com/fertile/banyan/pkg/vpc/security"
)

func TestSecurityManager_AddRule(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	tests := []struct {
		name    string
		rule    vpc.SecurityRule
		wantErr bool
	}{
		{
			name: "add allow rule for service",
			rule: vpc.SecurityRule{
				ID:          "rule-001",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "service:api",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add allow rule from CIDR",
			rule: vpc.SecurityRule{
				ID:          "rule-002",
				NetworkID:   "network-001",
				ServiceName: "database",
				Direction:   "ingress",
				Action:      "allow",
				From:        "cidr:10.0.1.0/24",
				ToPort:      "5432",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add allow rule from internet",
			rule: vpc.SecurityRule{
				ID:          "rule-003",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "443",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add deny rule (explicit)",
			rule: vpc.SecurityRule{
				ID:          "rule-004",
				NetworkID:   "network-001",
				ServiceName: "database",
				Direction:   "ingress",
				Action:      "deny",
				From:        "internet",
				ToPort:      "5432",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add egress rule",
			rule: vpc.SecurityRule{
				ID:          "rule-005",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "egress",
				Action:      "allow",
				To:          "internet",
				ToPort:      "443",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add rule with invalid from format",
			rule: vpc.SecurityRule{
				ID:          "rule-006",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "invalid-format",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
		{
			name: "add rule with invalid CIDR",
			rule: vpc.SecurityRule{
				ID:          "rule-007",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "cidr:invalid",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
		{
			name: "add rule with empty ID",
			rule: vpc.SecurityRule{
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
		{
			name: "add rule with empty network ID",
			rule: vpc.SecurityRule{
				ID:          "rule-008",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
		{
			name: "add rule with invalid port",
			rule: vpc.SecurityRule{
				ID:          "rule-009",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "99999",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
		{
			name: "add rule with port range",
			rule: vpc.SecurityRule{
				ID:          "rule-010",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "service:api",
				ToPort:      "8000-8100",
				Protocol:    "tcp",
			},
			wantErr: false,
		},
		{
			name: "add duplicate rule ID",
			rule: vpc.SecurityRule{
				ID:          "rule-001", // Same as first rule
				NetworkID:   "network-001",
				ServiceName: "api",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "3000",
				Protocol:    "tcp",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.AddRule(ctx, &tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityManager_RemoveRule(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// First add some rules
	rule1 := &vpc.SecurityRule{
		ID:          "remove-test-001",
		NetworkID:   "network-001",
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "80",
		Protocol:    "tcp",
	}
	manager.AddRule(ctx, rule1)

	tests := []struct {
		name    string
		ruleID  string
		wantErr bool
	}{
		{
			name:    "remove existing rule",
			ruleID:  "remove-test-001",
			wantErr: false,
		},
		{
			name:    "remove non-existent rule",
			ruleID:  "non-existent",
			wantErr: true,
		},
		{
			name:    "remove with empty ID",
			ruleID:  "",
			wantErr: true,
		},
		{
			name:    "remove already removed rule",
			ruleID:  "remove-test-001", // Already removed above
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RemoveRule(ctx, tt.ruleID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveRule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityManager_ListRules(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// Add rules for different networks
	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "list-test-001",
		NetworkID:   "network-001",
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "80",
		Protocol:    "tcp",
	})

	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "list-test-002",
		NetworkID:   "network-001",
		ServiceName: "database",
		Direction:   "ingress",
		Action:      "allow",
		From:        "service:web",
		ToPort:      "5432",
		Protocol:    "tcp",
	})

	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "list-test-003",
		NetworkID:   "network-002",
		ServiceName: "api",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "3000",
		Protocol:    "tcp",
	})

	tests := []struct {
		name       string
		networkID  string
		wantCount  int
		wantErr    bool
		checkRules func(t *testing.T, rules []*vpc.SecurityRule)
	}{
		{
			name:      "list rules for network with multiple rules",
			networkID: "network-001",
			wantCount: 2,
			wantErr:   false,
			checkRules: func(t *testing.T, rules []*vpc.SecurityRule) {
				if len(rules) != 2 {
					t.Errorf("expected 2 rules, got %d", len(rules))
				}
				// Check that both rules belong to network-001
				for _, rule := range rules {
					if rule.NetworkID != "network-001" {
						t.Errorf("expected network-001, got %s", rule.NetworkID)
					}
				}
			},
		},
		{
			name:      "list rules for network with one rule",
			networkID: "network-002",
			wantCount: 1,
			wantErr:   false,
			checkRules: func(t *testing.T, rules []*vpc.SecurityRule) {
				if len(rules) != 1 {
					t.Errorf("expected 1 rule, got %d", len(rules))
				}
				if rules[0].ServiceName != "api" {
					t.Errorf("expected api service, got %s", rules[0].ServiceName)
				}
			},
		},
		{
			name:      "list rules for non-existent network",
			networkID: "network-999",
			wantCount: 0,
			wantErr:   false,
			checkRules: func(t *testing.T, rules []*vpc.SecurityRule) {
				if len(rules) != 0 {
					t.Errorf("expected 0 rules, got %d", len(rules))
				}
			},
		},
		{
			name:      "list rules with empty network ID",
			networkID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := manager.ListRules(ctx, tt.networkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListRules() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checkRules != nil {
				tt.checkRules(t, rules)
			}
		})
	}
}

func TestSecurityManager_ApplyRules(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// Setup rules for a network
	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "apply-test-001",
		NetworkID:   "network-apply",
		ServiceName: "web",
		Direction:   "ingress",
		Action:      "allow",
		From:        "internet",
		ToPort:      "80",
		Protocol:    "tcp",
	})

	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "apply-test-002",
		NetworkID:   "network-apply",
		ServiceName: "database",
		Direction:   "ingress",
		Action:      "allow",
		From:        "service:web",
		ToPort:      "5432",
		Protocol:    "tcp",
	})

	tests := []struct {
		name      string
		networkID string
		wantErr   bool
	}{
		{
			name:      "apply rules for existing network",
			networkID: "network-apply",
			wantErr:   false,
		},
		{
			name:      "apply rules for non-existent network",
			networkID: "network-non-existent",
			wantErr:   false, // Should succeed even with no rules
		},
		{
			name:      "apply rules with empty network ID",
			networkID: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ApplyRules(ctx, tt.networkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyRules() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecurityManager_DenyByDefault(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// Create a network with only one allow rule
	manager.AddRule(ctx, &vpc.SecurityRule{
		ID:          "deny-test-001",
		NetworkID:   "network-secure",
		ServiceName: "database",
		Direction:   "ingress",
		Action:      "allow",
		From:        "service:web",
		ToPort:      "5432",
		Protocol:    "tcp",
	})

	// Apply rules to generate iptables
	err := manager.ApplyRules(ctx, "network-secure")
	if err != nil {
		t.Fatalf("Failed to apply rules: %v", err)
	}

	// In the real implementation, this test would verify that:
	// 1. Default policy is DROP for all chains
	// 2. Only explicitly allowed traffic is permitted
	// 3. The database service can only be accessed from web service
	// 4. All other traffic to database is dropped

	// This is a placeholder test that will be expanded during implementation
	t.Log("Deny by default behavior will be verified in implementation")
}

func TestSecurityManager_RuleTranslation(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// Test that rules are correctly translated to iptables commands
	testCases := []struct {
		name             string
		rule             vpc.SecurityRule
		expectedContains []string // Strings that should be in iptables rules
	}{
		{
			name: "service to service rule",
			rule: vpc.SecurityRule{
				ID:          "trans-001",
				NetworkID:   "network-001",
				ServiceName: "database",
				Direction:   "ingress",
				Action:      "allow",
				From:        "service:web",
				ToPort:      "5432",
				Protocol:    "tcp",
			},
			expectedContains: []string{
				"ACCEPT",
				"tcp",
				"--dport 5432",
				// DNS-based service resolution will add specific source
			},
		},
		{
			name: "CIDR rule",
			rule: vpc.SecurityRule{
				ID:          "trans-002",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "cidr:10.0.1.0/24",
				ToPort:      "80",
				Protocol:    "tcp",
			},
			expectedContains: []string{
				"ACCEPT",
				"-s 10.0.1.0/24",
				"--dport 80",
				"tcp",
			},
		},
		{
			name: "internet rule",
			rule: vpc.SecurityRule{
				ID:          "trans-003",
				NetworkID:   "network-001",
				ServiceName: "web",
				Direction:   "ingress",
				Action:      "allow",
				From:        "internet",
				ToPort:      "443",
				Protocol:    "tcp",
			},
			expectedContains: []string{
				"ACCEPT",
				"--dport 443",
				"tcp",
				// Should allow from any source (0.0.0.0/0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.AddRule(ctx, &tc.rule)
			if err != nil {
				t.Fatalf("Failed to add rule: %v", err)
			}

			// This test will verify the actual iptables translation
			// during implementation phase
			t.Logf("Rule translation for %s will be verified in implementation", tc.name)
		})
	}
}

func TestSecurityManager_ComplexScenarios(t *testing.T) {
	ctx := context.Background()
	manager := security.NewManager()

	// Scenario: Multi-tier application with proper isolation
	// Web tier -> API tier -> Database tier
	// Only web is exposed to internet

	rules := []*vpc.SecurityRule{
		// Web tier rules
		{
			ID:          "complex-001",
			NetworkID:   "prod-network",
			ServiceName: "web",
			Direction:   "ingress",
			Action:      "allow",
			From:        "internet",
			ToPort:      "443",
			Protocol:    "tcp",
		},
		{
			ID:          "complex-002",
			NetworkID:   "prod-network",
			ServiceName: "web",
			Direction:   "egress",
			Action:      "allow",
			To:          "service:api",
			ToPort:      "8080",
			Protocol:    "tcp",
		},
		// API tier rules
		{
			ID:          "complex-003",
			NetworkID:   "prod-network",
			ServiceName: "api",
			Direction:   "ingress",
			Action:      "allow",
			From:        "service:web",
			ToPort:      "8080",
			Protocol:    "tcp",
		},
		{
			ID:          "complex-004",
			NetworkID:   "prod-network",
			ServiceName: "api",
			Direction:   "egress",
			Action:      "allow",
			To:          "service:database",
			ToPort:      "5432",
			Protocol:    "tcp",
		},
		// Database tier rules
		{
			ID:          "complex-005",
			NetworkID:   "prod-network",
			ServiceName: "database",
			Direction:   "ingress",
			Action:      "allow",
			From:        "service:api",
			ToPort:      "5432",
			Protocol:    "tcp",
		},
		// Explicitly deny database from internet (redundant but explicit)
		{
			ID:          "complex-006",
			NetworkID:   "prod-network",
			ServiceName: "database",
			Direction:   "ingress",
			Action:      "deny",
			From:        "internet",
			ToPort:      "5432",
			Protocol:    "tcp",
		},
	}

	// Add all rules
	for _, rule := range rules {
		err := manager.AddRule(ctx, rule)
		if err != nil {
			t.Errorf("Failed to add rule %s: %v", rule.ID, err)
		}
	}

	// Verify rules are correctly stored
	prodRules, err := manager.ListRules(ctx, "prod-network")
	if err != nil {
		t.Fatalf("Failed to list rules: %v", err)
	}

	if len(prodRules) != 6 {
		t.Errorf("Expected 6 rules, got %d", len(prodRules))
	}

	// Apply rules
	err = manager.ApplyRules(ctx, "prod-network")
	if err != nil {
		t.Fatalf("Failed to apply rules: %v", err)
	}

	t.Log("Complex multi-tier scenario test completed")
}
