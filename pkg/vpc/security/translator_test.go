package security

import (
	"context"
	"net"
	"testing"

	"github.com/fertile-org/banyan/pkg/vpc"
)

// mockServiceResolver is a mock implementation of ServiceResolver for testing
type mockServiceResolver struct {
	services map[string][]net.IP
}

func newMockServiceResolver() *mockServiceResolver {
	return &mockServiceResolver{
		services: make(map[string][]net.IP),
	}
}

func (m *mockServiceResolver) AddService(name string, ips ...string) {
	ipList := make([]net.IP, len(ips))
	for i, ipStr := range ips {
		ipList[i] = net.ParseIP(ipStr)
	}
	m.services[name] = ipList
}

func (m *mockServiceResolver) ResolveServiceIPs(ctx context.Context, serviceName string) ([]net.IP, error) {
	ips, ok := m.services[serviceName]
	if !ok {
		return nil, nil
	}
	return ips, nil
}

func TestTranslator_TranslateRule_ServiceToService(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("web", "10.0.1.10", "10.0.1.11")
	resolver.AddService("database", "10.0.2.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "database",
		From:        "service:web",
		ToPort:      "5432",
		Protocol:    "tcp",
		Action:      "allow",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	// Should generate 2 rules (2 web IPs × 1 database IP)
	if len(rules) != 2 {
		t.Fatalf("Expected 2 iptables rules, got %d", len(rules))
	}

	// Verify first rule (web IP 10.0.1.10 → database IP 10.0.2.10)
	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "10.0.1.10",
			"-d", "10.0.2.10",
			"-p", "tcp",
			"--dport", "5432",
			"-j", "ACCEPT",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule 0 mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}

	// Verify second rule (web IP 10.0.1.11 → database IP 10.0.2.10)
	expected.Args[1] = "10.0.1.11"
	if !compareIptablesRules(rules[1], expected) {
		t.Errorf("Rule 1 mismatch.\nGot:  %+v\nWant: %+v", rules[1], expected)
	}
}

func TestTranslator_TranslateRule_InternetIngress(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("web", "10.0.1.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "internet",
		ToPort:      "443",
		Protocol:    "tcp",
		Action:      "allow",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	// Should generate 1 rule (internet → 1 web IP)
	if len(rules) != 1 {
		t.Fatalf("Expected 1 iptables rule, got %d", len(rules))
	}

	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "0.0.0.0/0",
			"-d", "10.0.1.10",
			"-p", "tcp",
			"--dport", "443",
			"-j", "ACCEPT",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}
}

func TestTranslator_TranslateRule_CIDRToService(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("database", "10.0.2.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "database",
		From:        "cidr:10.0.1.0/24",
		ToPort:      "5432",
		Protocol:    "tcp",
		Action:      "allow",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 iptables rule, got %d", len(rules))
	}

	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "10.0.1.0/24",
			"-d", "10.0.2.10",
			"-p", "tcp",
			"--dport", "5432",
			"-j", "ACCEPT",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}
}

func TestTranslator_TranslateRule_DenyAction(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("web", "10.0.1.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "internet",
		ToPort:      "22",
		Protocol:    "tcp",
		Action:      "deny",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 iptables rule, got %d", len(rules))
	}

	// Verify DROP action for deny
	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "0.0.0.0/0",
			"-d", "10.0.1.10",
			"-p", "tcp",
			"--dport", "22",
			"-j", "DROP",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}
}

func TestTranslator_TranslateRule_NoPort(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("web", "10.0.1.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "internet",
		Protocol:    "icmp",
		Action:      "allow",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 iptables rule, got %d", len(rules))
	}

	// ICMP rule should not have --dport
	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "0.0.0.0/0",
			"-d", "10.0.1.10",
			"-p", "icmp",
			"-j", "ACCEPT",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}
}

func TestTranslator_TranslateRule_AllProtocols(t *testing.T) {
	resolver := newMockServiceResolver()
	resolver.AddService("web", "10.0.1.10")

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "cidr:10.0.2.0/24",
		Protocol:    "all",
		Action:      "allow",
	}

	rules, err := translator.TranslateRule(context.Background(), rule)
	if err != nil {
		t.Fatalf("TranslateRule failed: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("Expected 1 iptables rule, got %d", len(rules))
	}

	// "all" protocol should not have -p flag
	expected := IptablesRule{
		Table: "filter",
		Chain: "FORWARD",
		Args: []string{
			"-s", "10.0.2.0/24",
			"-d", "10.0.1.10",
			"-j", "ACCEPT",
		},
	}

	if !compareIptablesRules(rules[0], expected) {
		t.Errorf("Rule mismatch.\nGot:  %+v\nWant: %+v", rules[0], expected)
	}
}

func TestTranslator_ParseSource_InvalidFormats(t *testing.T) {
	translator := NewTranslator(nil)

	testCases := []struct {
		name   string
		source string
	}{
		{"empty string", ""},
		{"invalid format", "invalid"},
		{"empty service name", "service:"},
		{"empty CIDR", "cidr:"},
		{"invalid CIDR", "cidr:invalid"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translator.parseSource(context.Background(), tc.source)
			if err == nil {
				t.Errorf("Expected error for source %q, got nil", tc.source)
			}
		})
	}
}

func TestTranslator_ParseDestination_InvalidFormats(t *testing.T) {
	translator := NewTranslator(nil)

	testCases := []struct {
		name        string
		to          string
		serviceName string
	}{
		{"both empty", "", ""},
		{"empty service with service: prefix", "service:", ""},
		{"empty CIDR", "cidr:", ""},
		{"invalid CIDR", "cidr:invalid", ""},
		{"invalid format", "invalid", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := translator.parseDestination(context.Background(), tc.to, tc.serviceName)
			if err == nil {
				t.Errorf("Expected error for to=%q serviceName=%q, got nil", tc.to, tc.serviceName)
			}
		})
	}
}

func TestTranslator_TranslateRule_NilRule(t *testing.T) {
	translator := NewTranslator(nil)

	_, err := translator.TranslateRule(context.Background(), nil)
	if err == nil {
		t.Error("Expected error for nil rule, got nil")
	}
}

func TestTranslator_TranslateRule_NoServiceResolver(t *testing.T) {
	translator := NewTranslator(nil)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "service:api",
		ToPort:      "80",
		Protocol:    "tcp",
		Action:      "allow",
	}

	_, err := translator.TranslateRule(context.Background(), rule)
	if err == nil {
		t.Error("Expected error when service resolver is nil, got nil")
	}
}

func TestTranslator_TranslateRule_ServiceNotFound(t *testing.T) {
	resolver := newMockServiceResolver()
	// Don't add any services

	translator := NewTranslator(resolver)

	rule := &vpc.SecurityRule{
		ServiceName: "web",
		From:        "service:nonexistent",
		ToPort:      "80",
		Protocol:    "tcp",
		Action:      "allow",
	}

	_, err := translator.TranslateRule(context.Background(), rule)
	if err == nil {
		t.Error("Expected error for nonexistent service, got nil")
	}
}

// compareIptablesRules compares two IptablesRule structs for equality
func compareIptablesRules(a, b IptablesRule) bool {
	if a.Table != b.Table || a.Chain != b.Chain {
		return false
	}

	if len(a.Args) != len(b.Args) {
		return false
	}

	for i := range a.Args {
		if a.Args[i] != b.Args[i] {
			return false
		}
	}

	return true
}
