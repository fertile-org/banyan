package security

import (
	"context"
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
