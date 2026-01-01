package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
)

// Mock implementations

type mockSecurityManager struct {
	addRuleFunc    func(ctx context.Context, rule *domain.SecurityRule) error
	removeRuleFunc func(ctx context.Context, ruleID domain.RuleID) error
	listRulesFunc  func(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error)
	applyFunc      func(ctx context.Context, networkID domain.NetworkID) error
}

func (m *mockSecurityManager) AddRule(ctx context.Context, rule *domain.SecurityRule) error {
	if m.addRuleFunc != nil {
		return m.addRuleFunc(ctx, rule)
	}
	return nil
}

func (m *mockSecurityManager) RemoveRule(ctx context.Context, ruleID domain.RuleID) error {
	if m.removeRuleFunc != nil {
		return m.removeRuleFunc(ctx, ruleID)
	}
	return nil
}

func (m *mockSecurityManager) ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error) {
	if m.listRulesFunc != nil {
		return m.listRulesFunc(ctx, networkID)
	}
	return nil, nil
}

func (m *mockSecurityManager) ApplyRules(ctx context.Context, networkID domain.NetworkID) error {
	if m.applyFunc != nil {
		return m.applyFunc(ctx, networkID)
	}
	return nil
}

type mockRuleStore struct {
	rules      map[string]*domain.SecurityRule
	saveFunc   func(ctx context.Context, rule *domain.SecurityRule) error
	getFunc    func(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error)
	deleteFunc func(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error
	listFunc   func(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error)
}

func newMockRuleStore() *mockRuleStore {
	return &mockRuleStore{
		rules: make(map[string]*domain.SecurityRule),
	}
}

func (m *mockRuleStore) key(networkID domain.NetworkID, ruleID domain.RuleID) string {
	return string(networkID) + ":" + string(ruleID)
}

func (m *mockRuleStore) SaveRule(ctx context.Context, rule *domain.SecurityRule) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, rule)
	}
	m.rules[m.key(rule.NetworkID, rule.ID)] = rule
	return nil
}

func (m *mockRuleStore) GetRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, networkID, ruleID)
	}
	rule, ok := m.rules[m.key(networkID, ruleID)]
	if !ok {
		return nil, domain.ErrRuleNotFound
	}
	return rule, nil
}

func (m *mockRuleStore) DeleteRule(ctx context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, networkID, ruleID)
	}
	delete(m.rules, m.key(networkID, ruleID))
	return nil
}

func (m *mockRuleStore) ListRules(ctx context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, networkID)
	}
	var result []*domain.SecurityRule
	for _, rule := range m.rules {
		if rule.NetworkID == networkID {
			result = append(result, rule)
		}
	}
	return result, nil
}

// Tests

func TestAddRule_Success(t *testing.T) {
	securityManager := &mockSecurityManager{}
	ruleStore := newMockRuleStore()

	uc := NewSecurityUseCase(securityManager, ruleStore)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
		Direction: domain.DirectionIngress,
		Action:    domain.ActionAllow,
		From:      "cidr:10.0.0.0/24",
		To:        "service:web",
		ToPort:    "80",
		Protocol:  domain.ProtocolTCP,
	}

	err := uc.AddRule(context.Background(), rule)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify rule was saved
	saved, err := ruleStore.GetRule(context.Background(), "network1", "rule1")
	if err != nil {
		t.Fatalf("expected rule to be saved, got error %v", err)
	}
	if saved.ID != "rule1" {
		t.Errorf("expected rule1, got %s", saved.ID)
	}
}

func TestAddRule_InvalidRule(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	err := uc.AddRule(context.Background(), nil)

	if !errors.Is(err, domain.ErrInvalidRule) {
		t.Errorf("expected ErrInvalidRule, got %v", err)
	}
}

func TestAddRule_InvalidNetworkID(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "",
		Direction: domain.DirectionIngress,
		Action:    domain.ActionAllow,
	}

	err := uc.AddRule(context.Background(), rule)

	if !errors.Is(err, domain.ErrInvalidNetworkID) {
		t.Errorf("expected ErrInvalidNetworkID, got %v", err)
	}
}

func TestAddRule_InvalidDirection(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
		Direction: "invalid",
		Action:    domain.ActionAllow,
	}

	err := uc.AddRule(context.Background(), rule)

	if !errors.Is(err, domain.ErrInvalidDirection) {
		t.Errorf("expected ErrInvalidDirection, got %v", err)
	}
}

func TestAddRule_InvalidAction(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
		Direction: domain.DirectionIngress,
		Action:    "invalid",
	}

	err := uc.AddRule(context.Background(), rule)

	if !errors.Is(err, domain.ErrInvalidAction) {
		t.Errorf("expected ErrInvalidAction, got %v", err)
	}
}

func TestAddRule_AlreadyExists(t *testing.T) {
	securityManager := &mockSecurityManager{}
	ruleStore := newMockRuleStore()

	// Pre-populate rule
	ruleStore.rules["network1:rule1"] = &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
	}

	uc := NewSecurityUseCase(securityManager, ruleStore)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
		Direction: domain.DirectionIngress,
		Action:    domain.ActionAllow,
	}

	err := uc.AddRule(context.Background(), rule)

	if !errors.Is(err, domain.ErrRuleAlreadyExists) {
		t.Errorf("expected ErrRuleAlreadyExists, got %v", err)
	}
}

func TestAddRule_BackendFailed(t *testing.T) {
	securityManager := &mockSecurityManager{
		addRuleFunc: func(ctx context.Context, rule *domain.SecurityRule) error {
			return errors.New("backend error")
		},
	}
	ruleStore := newMockRuleStore()

	uc := NewSecurityUseCase(securityManager, ruleStore)

	rule := &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
		Direction: domain.DirectionIngress,
		Action:    domain.ActionAllow,
	}

	err := uc.AddRule(context.Background(), rule)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify rule was rolled back
	_, err = ruleStore.GetRule(context.Background(), "network1", "rule1")
	if !errors.Is(err, domain.ErrRuleNotFound) {
		t.Error("expected rule to be rolled back from store")
	}
}

func TestRemoveRule_Success(t *testing.T) {
	securityManager := &mockSecurityManager{}
	ruleStore := newMockRuleStore()

	// Pre-populate rule
	ruleStore.rules["network1:rule1"] = &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
	}

	uc := NewSecurityUseCase(securityManager, ruleStore)

	err := uc.RemoveRule(context.Background(), "network1", "rule1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify rule was deleted
	_, err = ruleStore.GetRule(context.Background(), "network1", "rule1")
	if !errors.Is(err, domain.ErrRuleNotFound) {
		t.Error("expected rule to be deleted")
	}
}

func TestRemoveRule_NotFound(t *testing.T) {
	securityManager := &mockSecurityManager{}
	ruleStore := newMockRuleStore()

	uc := NewSecurityUseCase(securityManager, ruleStore)

	err := uc.RemoveRule(context.Background(), "network1", "nonexistent")

	if !errors.Is(err, domain.ErrRuleNotFound) {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

func TestRemoveRule_InvalidNetworkID(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	err := uc.RemoveRule(context.Background(), "", "rule1")

	if !errors.Is(err, domain.ErrInvalidNetworkID) {
		t.Errorf("expected ErrInvalidNetworkID, got %v", err)
	}
}

func TestGetRule_Success(t *testing.T) {
	ruleStore := newMockRuleStore()
	ruleStore.rules["network1:rule1"] = &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
	}

	uc := NewSecurityUseCase(nil, ruleStore)

	rule, err := uc.GetRule(context.Background(), "network1", "rule1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if rule.ID != "rule1" {
		t.Errorf("expected rule1, got %s", rule.ID)
	}
}

func TestGetRule_NotFound(t *testing.T) {
	ruleStore := newMockRuleStore()

	uc := NewSecurityUseCase(nil, ruleStore)

	_, err := uc.GetRule(context.Background(), "network1", "nonexistent")

	if !errors.Is(err, domain.ErrRuleNotFound) {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

func TestListRules_Success(t *testing.T) {
	ruleStore := newMockRuleStore()
	ruleStore.rules["network1:rule1"] = &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
	}
	ruleStore.rules["network1:rule2"] = &domain.SecurityRule{
		ID:        "rule2",
		NetworkID: "network1",
	}

	uc := NewSecurityUseCase(nil, ruleStore)

	rules, err := uc.ListRules(context.Background(), "network1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
}

func TestListRules_InvalidNetworkID(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	_, err := uc.ListRules(context.Background(), "")

	if !errors.Is(err, domain.ErrInvalidNetworkID) {
		t.Errorf("expected ErrInvalidNetworkID, got %v", err)
	}
}

func TestApplyPolicy_Success(t *testing.T) {
	applied := false
	securityManager := &mockSecurityManager{
		applyFunc: func(ctx context.Context, networkID domain.NetworkID) error {
			applied = true
			return nil
		},
	}

	uc := NewSecurityUseCase(securityManager, nil)

	err := uc.ApplyPolicy(context.Background(), "network1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !applied {
		t.Error("expected rules to be applied")
	}
}

func TestApplyPolicy_Failed(t *testing.T) {
	securityManager := &mockSecurityManager{
		applyFunc: func(ctx context.Context, networkID domain.NetworkID) error {
			return errors.New("iptables error")
		},
	}

	uc := NewSecurityUseCase(securityManager, nil)

	err := uc.ApplyPolicy(context.Background(), "network1")

	if !errors.Is(err, domain.ErrApplyFailed) {
		t.Errorf("expected ErrApplyFailed, got %v", err)
	}
}

func TestGetPolicyStatus_Success(t *testing.T) {
	ruleStore := newMockRuleStore()
	ruleStore.rules["network1:rule1"] = &domain.SecurityRule{
		ID:        "rule1",
		NetworkID: "network1",
	}
	ruleStore.rules["network1:rule2"] = &domain.SecurityRule{
		ID:        "rule2",
		NetworkID: "network1",
	}

	uc := NewSecurityUseCase(nil, ruleStore)

	status, err := uc.GetPolicyStatus(context.Background(), "network1")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.NetworkID != "network1" {
		t.Errorf("expected network1, got %s", status.NetworkID)
	}

	if status.RuleCount != 2 {
		t.Errorf("expected 2 rules, got %d", status.RuleCount)
	}

	if !status.Applied {
		t.Error("expected applied to be true")
	}
}

func TestGetPolicyStatus_InvalidNetworkID(t *testing.T) {
	uc := NewSecurityUseCase(nil, nil)

	_, err := uc.GetPolicyStatus(context.Background(), "")

	if !errors.Is(err, domain.ErrInvalidNetworkID) {
		t.Errorf("expected ErrInvalidNetworkID, got %v", err)
	}
}
