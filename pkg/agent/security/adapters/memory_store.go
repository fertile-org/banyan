package adapters

import (
	"context"
	"sync"

	"github.com/fertile-org/banyan/pkg/agent/security/domain"
	"github.com/fertile-org/banyan/pkg/agent/security/ports/outbound"
)

// MemoryRuleStore implements RuleStore with in-memory storage.
type MemoryRuleStore struct {
	rules map[string]*domain.SecurityRule // key: networkID:ruleID
	mu    sync.RWMutex
}

// NewMemoryRuleStore creates a new MemoryRuleStore.
func NewMemoryRuleStore() *MemoryRuleStore {
	return &MemoryRuleStore{
		rules: make(map[string]*domain.SecurityRule),
	}
}

func (s *MemoryRuleStore) key(networkID domain.NetworkID, ruleID domain.RuleID) string {
	return string(networkID) + ":" + string(ruleID)
}

// SaveRule saves a security rule.
func (s *MemoryRuleStore) SaveRule(_ context.Context, rule *domain.SecurityRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.rules[s.key(rule.NetworkID, rule.ID)] = rule
	return nil
}

// GetRule retrieves a security rule by ID.
func (s *MemoryRuleStore) GetRule(_ context.Context, networkID domain.NetworkID, ruleID domain.RuleID) (*domain.SecurityRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rule, ok := s.rules[s.key(networkID, ruleID)]
	if !ok {
		return nil, domain.ErrRuleNotFound
	}
	return rule, nil
}

// DeleteRule deletes a security rule.
func (s *MemoryRuleStore) DeleteRule(_ context.Context, networkID domain.NetworkID, ruleID domain.RuleID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.rules, s.key(networkID, ruleID))
	return nil
}

// ListRules lists all rules for a network.
func (s *MemoryRuleStore) ListRules(_ context.Context, networkID domain.NetworkID) ([]*domain.SecurityRule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*domain.SecurityRule
	for _, rule := range s.rules {
		if rule.NetworkID == networkID {
			result = append(result, rule)
		}
	}
	return result, nil
}

// Ensure MemoryRuleStore implements outbound.RuleStore
var _ outbound.RuleStore = (*MemoryRuleStore)(nil)
