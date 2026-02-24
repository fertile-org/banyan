package security

import (
	"context"
	"fmt"
	"testing"
)

func TestIptablesExecutor_DryRun(t *testing.T) {
	executor := NewIptablesExecutor(true)

	ctx := context.Background()

	// All operations should succeed in dry-run mode
	if err := executor.CreateChain(ctx, "filter", "TEST-CHAIN"); err != nil {
		t.Errorf("CreateChain failed in dry-run mode: %v", err)
	}

	if err := executor.FlushChain(ctx, "filter", "TEST-CHAIN"); err != nil {
		t.Errorf("FlushChain failed in dry-run mode: %v", err)
	}

	rule := IptablesRule{
		Table: "filter",
		Chain: "TEST-CHAIN",
		Args:  []string{"-s", "10.0.1.0/24", "-j", "ACCEPT"},
	}
	if err := executor.ApplyRule(ctx, rule); err != nil {
		t.Errorf("ApplyRule failed in dry-run mode: %v", err)
	}

	if err := executor.AddDefaultDeny(ctx, "filter", "TEST-CHAIN"); err != nil {
		t.Errorf("AddDefaultDeny failed in dry-run mode: %v", err)
	}

	if err := executor.JumpToChain(ctx, "filter", "FORWARD", "TEST-CHAIN"); err != nil {
		t.Errorf("JumpToChain failed in dry-run mode: %v", err)
	}

	if err := executor.DeleteChain(ctx, "filter", "TEST-CHAIN"); err != nil {
		t.Errorf("DeleteChain failed in dry-run mode: %v", err)
	}
}

func TestIptablesExecutor_CreateChain_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name  string
		table string
		chain string
	}{
		{"empty table", "", "TEST-CHAIN"},
		{"empty chain", "filter", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.CreateChain(ctx, tc.table, tc.chain)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIptablesExecutor_FlushChain_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name  string
		table string
		chain string
	}{
		{"empty table", "", "TEST-CHAIN"},
		{"empty chain", "filter", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.FlushChain(ctx, tc.table, tc.chain)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIptablesExecutor_DeleteChain_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name  string
		table string
		chain string
	}{
		{"empty table", "", "TEST-CHAIN"},
		{"empty chain", "filter", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.DeleteChain(ctx, tc.table, tc.chain)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIptablesExecutor_ApplyRule_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name string
		rule IptablesRule
	}{
		{
			"empty table",
			IptablesRule{
				Table: "",
				Chain: "TEST-CHAIN",
				Args:  []string{"-j", "ACCEPT"},
			},
		},
		{
			"empty chain",
			IptablesRule{
				Table: "filter",
				Chain: "",
				Args:  []string{"-j", "ACCEPT"},
			},
		},
		{
			"empty args",
			IptablesRule{
				Table: "filter",
				Chain: "TEST-CHAIN",
				Args:  []string{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.ApplyRule(ctx, tc.rule)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIptablesExecutor_AddDefaultDeny_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name  string
		table string
		chain string
	}{
		{"empty table", "", "TEST-CHAIN"},
		{"empty chain", "filter", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.AddDefaultDeny(ctx, tc.table, tc.chain)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIptablesExecutor_JumpToChain_Validation(t *testing.T) {
	executor := NewIptablesExecutor(true)
	ctx := context.Background()

	testCases := []struct {
		name      string
		table     string
		fromChain string
		toChain   string
	}{
		{"empty table", "", "FORWARD", "TEST-CHAIN"},
		{"empty fromChain", "filter", "", "TEST-CHAIN"},
		{"empty toChain", "filter", "FORWARD", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := executor.JumpToChain(ctx, tc.table, tc.fromChain, tc.toChain)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
		})
	}
}

// mockIPTables is a mock implementation of the IPTables interface for testing.
type mockIPTables struct {
	chains      map[string][]string // table -> chain names
	rules       map[string][]string // "table/chain" -> rule strings
	newChainErr error
	clearErr    error
	deleteErr   error
	appendErr   error
	listErr     error
}

func newMockIPTables() *mockIPTables {
	return &mockIPTables{
		chains: make(map[string][]string),
		rules:  make(map[string][]string),
	}
}

func (m *mockIPTables) NewChain(table, chain string) error {
	if m.newChainErr != nil {
		return m.newChainErr
	}
	m.chains[table] = append(m.chains[table], chain)
	return nil
}

func (m *mockIPTables) ClearChain(table, chain string) error {
	if m.clearErr != nil {
		return m.clearErr
	}
	key := table + "/" + chain
	m.rules[key] = nil
	return nil
}

func (m *mockIPTables) DeleteChain(table, chain string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	chains := m.chains[table]
	for i, c := range chains {
		if c == chain {
			m.chains[table] = append(chains[:i], chains[i+1:]...)
			break
		}
	}
	return nil
}

func (m *mockIPTables) Append(table, chain string, rulespec ...string) error {
	if m.appendErr != nil {
		return m.appendErr
	}
	key := table + "/" + chain
	m.rules[key] = append(m.rules[key], fmt.Sprintf("%v", rulespec))
	return nil
}

func (m *mockIPTables) ListChains(table string) ([]string, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.chains[table], nil
}

func TestIptablesExecutor_WithMock_CreateChain(t *testing.T) {
	mock := newMockIPTables()
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	t.Run("creates new chain", func(t *testing.T) {
		err := executor.CreateChain(ctx, "filter", "BANYAN-test")
		if err != nil {
			t.Fatalf("CreateChain failed: %v", err)
		}
		if len(mock.chains["filter"]) != 1 || mock.chains["filter"][0] != "BANYAN-test" {
			t.Errorf("expected chain BANYAN-test, got %v", mock.chains["filter"])
		}
	})

	t.Run("skips existing chain", func(t *testing.T) {
		// Chain already exists from previous test
		err := executor.CreateChain(ctx, "filter", "BANYAN-test")
		if err != nil {
			t.Fatalf("CreateChain failed: %v", err)
		}
		// Should still have only 1 chain
		if len(mock.chains["filter"]) != 1 {
			t.Errorf("expected 1 chain, got %d", len(mock.chains["filter"]))
		}
	})

	t.Run("handles ListChains error", func(t *testing.T) {
		errMock := newMockIPTables()
		errMock.listErr = fmt.Errorf("iptables not available")
		errExecutor := NewIptablesExecutorWithIPTables(errMock)

		err := errExecutor.CreateChain(ctx, "filter", "TEST")
		if err == nil {
			t.Fatal("expected error when ListChains fails")
		}
	})

	t.Run("handles NewChain error", func(t *testing.T) {
		errMock := newMockIPTables()
		errMock.newChainErr = fmt.Errorf("permission denied")
		errExecutor := NewIptablesExecutorWithIPTables(errMock)

		err := errExecutor.CreateChain(ctx, "filter", "TEST")
		if err == nil {
			t.Fatal("expected error when NewChain fails")
		}
	})
}

func TestIptablesExecutor_WithMock_FlushChain(t *testing.T) {
	mock := newMockIPTables()
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	// Add some rules
	key := "filter/BANYAN-test"
	mock.rules[key] = []string{"rule1", "rule2"}

	err := executor.FlushChain(ctx, "filter", "BANYAN-test")
	if err != nil {
		t.Fatalf("FlushChain failed: %v", err)
	}
	if len(mock.rules[key]) != 0 {
		t.Errorf("expected empty rules after flush, got %v", mock.rules[key])
	}
}

func TestIptablesExecutor_WithMock_FlushChain_Error(t *testing.T) {
	mock := newMockIPTables()
	mock.clearErr = fmt.Errorf("permission denied")
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	err := executor.FlushChain(ctx, "filter", "BANYAN-test")
	if err == nil {
		t.Fatal("expected error when ClearChain fails")
	}
}

func TestIptablesExecutor_WithMock_DeleteChain(t *testing.T) {
	mock := newMockIPTables()
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	// Create chain first
	mock.chains["filter"] = []string{"BANYAN-test"}

	err := executor.DeleteChain(ctx, "filter", "BANYAN-test")
	if err != nil {
		t.Fatalf("DeleteChain failed: %v", err)
	}
	if len(mock.chains["filter"]) != 0 {
		t.Errorf("expected chain to be deleted, got %v", mock.chains["filter"])
	}
}

func TestIptablesExecutor_WithMock_DeleteChain_ClearError(t *testing.T) {
	mock := newMockIPTables()
	mock.clearErr = fmt.Errorf("flush failed")
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	err := executor.DeleteChain(ctx, "filter", "BANYAN-test")
	if err == nil {
		t.Fatal("expected error when ClearChain fails during delete")
	}
}

func TestIptablesExecutor_WithMock_DeleteChain_DeleteError(t *testing.T) {
	mock := newMockIPTables()
	mock.deleteErr = fmt.Errorf("delete failed")
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	err := executor.DeleteChain(ctx, "filter", "BANYAN-test")
	if err == nil {
		t.Fatal("expected error when DeleteChain fails")
	}
}

func TestIptablesExecutor_WithMock_ApplyRule(t *testing.T) {
	mock := newMockIPTables()
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	rule := IptablesRule{
		Table: "filter",
		Chain: "BANYAN-test",
		Args:  []string{"-s", "10.0.1.0/24", "-j", "ACCEPT"},
	}

	err := executor.ApplyRule(ctx, rule)
	if err != nil {
		t.Fatalf("ApplyRule failed: %v", err)
	}

	key := "filter/BANYAN-test"
	if len(mock.rules[key]) != 1 {
		t.Errorf("expected 1 rule, got %d", len(mock.rules[key]))
	}
}

func TestIptablesExecutor_WithMock_ApplyRule_Error(t *testing.T) {
	mock := newMockIPTables()
	mock.appendErr = fmt.Errorf("iptables busy")
	executor := NewIptablesExecutorWithIPTables(mock)
	ctx := context.Background()

	rule := IptablesRule{
		Table: "filter",
		Chain: "BANYAN-test",
		Args:  []string{"-j", "DROP"},
	}

	err := executor.ApplyRule(ctx, rule)
	if err == nil {
		t.Fatal("expected error when Append fails")
	}
}

func TestNewIptablesExecutorWithIPTables(t *testing.T) {
	mock := newMockIPTables()
	executor := NewIptablesExecutorWithIPTables(mock)

	if executor.ipt != mock {
		t.Error("expected mock IPTables to be set")
	}
	if executor.dryRun {
		t.Error("expected dryRun to be false")
	}
}
