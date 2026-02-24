package proxy

import (
	"fmt"
	"strings"
	"sync"
)

// mockIPTables tracks iptables chains and rules in memory for testing.
type mockIPTables struct {
	mu     sync.Mutex
	chains map[string]map[string][]string // table -> chain -> rules
	errors map[string]error               // method -> error (for error injection)
}

func newMockIPTables() *mockIPTables {
	return &mockIPTables{
		chains: make(map[string]map[string][]string),
		errors: make(map[string]error),
	}
}

func (m *mockIPTables) setError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = err
}

func (m *mockIPTables) getError(method string) error {
	if err, ok := m.errors[method]; ok {
		return err
	}
	return nil
}

func (m *mockIPTables) ensureTable(table string) {
	if _, ok := m.chains[table]; !ok {
		m.chains[table] = make(map[string][]string)
	}
}

func (m *mockIPTables) NewChain(table, chain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("NewChain"); err != nil {
		return err
	}
	m.ensureTable(table)
	if _, exists := m.chains[table][chain]; exists {
		return fmt.Errorf("chain %s already exists in table %s", chain, table)
	}
	m.chains[table][chain] = nil
	return nil
}

func (m *mockIPTables) ClearChain(table, chain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("ClearChain"); err != nil {
		return err
	}
	m.ensureTable(table)
	m.chains[table][chain] = nil
	return nil
}

func (m *mockIPTables) DeleteChain(table, chain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("DeleteChain"); err != nil {
		return err
	}
	m.ensureTable(table)
	delete(m.chains[table], chain)
	return nil
}

func (m *mockIPTables) Append(table, chain string, rulespec ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("Append"); err != nil {
		return err
	}
	m.ensureTable(table)
	rule := strings.Join(rulespec, " ")
	m.chains[table][chain] = append(m.chains[table][chain], rule)
	return nil
}

func (m *mockIPTables) Insert(table, chain string, pos int, rulespec ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("Insert"); err != nil {
		return err
	}
	m.ensureTable(table)
	rule := strings.Join(rulespec, " ")
	rules := m.chains[table][chain]
	idx := pos - 1 // iptables positions are 1-based
	if idx < 0 {
		idx = 0
	}
	if idx >= len(rules) {
		m.chains[table][chain] = append(rules, rule)
	} else {
		m.chains[table][chain] = append(rules[:idx], append([]string{rule}, rules[idx:]...)...)
	}
	return nil
}

func (m *mockIPTables) Delete(table, chain string, rulespec ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("Delete"); err != nil {
		return err
	}
	m.ensureTable(table)
	rule := strings.Join(rulespec, " ")
	rules := m.chains[table][chain]
	for i, r := range rules {
		if r == rule {
			m.chains[table][chain] = append(rules[:i], rules[i+1:]...)
			return nil
		}
	}
	return nil // No error if rule doesn't exist (matches real iptables -D behavior when not found)
}

func (m *mockIPTables) Exists(table, chain string, rulespec ...string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("Exists"); err != nil {
		return false, err
	}
	m.ensureTable(table)
	rule := strings.Join(rulespec, " ")
	for _, r := range m.chains[table][chain] {
		if r == rule {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockIPTables) List(table, chain string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("List"); err != nil {
		return nil, err
	}
	m.ensureTable(table)
	rules := m.chains[table][chain]
	result := make([]string, len(rules))
	copy(result, rules)
	return result, nil
}

func (m *mockIPTables) ListChains(table string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.getError("ListChains"); err != nil {
		return nil, err
	}
	m.ensureTable(table)
	var chains []string
	for chain := range m.chains[table] {
		chains = append(chains, chain)
	}
	return chains, nil
}

// getRules returns the rules for a chain in a table (for test assertions).
func (m *mockIPTables) getRules(table, chain string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureTable(table)
	rules := m.chains[table][chain]
	result := make([]string, len(rules))
	copy(result, rules)
	return result
}

// chainExists checks if a chain exists in a table (for test assertions).
func (m *mockIPTables) chainExists(table, chain string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureTable(table)
	_, exists := m.chains[table][chain]
	return exists
}
