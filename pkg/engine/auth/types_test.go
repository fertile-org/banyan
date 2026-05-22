package auth

import (
	"context"
	"testing"
)

func TestValidRoles(t *testing.T) {
	for _, role := range []string{RoleAdmin, RoleDeployer, RoleViewer} {
		if !ValidRoles[role] {
			t.Errorf("expected %q to be a valid role", role)
		}
	}
	if ValidRoles["superadmin"] {
		t.Error("expected 'superadmin' to be invalid")
	}
	if ValidRoles[""] {
		t.Error("expected empty string to be invalid")
	}
}

func TestContextIdentity(t *testing.T) {
	ctx := context.Background()

	// Empty context returns empty strings
	if got := UsernameFromContext(ctx); got != "" {
		t.Errorf("expected empty username from bare context, got %q", got)
	}
	if got := RoleFromContext(ctx); got != "" {
		t.Errorf("expected empty role from bare context, got %q", got)
	}

	// WithIdentity sets both values
	ctx = WithIdentity(ctx, "alice", RoleDeployer)
	if got := UsernameFromContext(ctx); got != "alice" {
		t.Errorf("expected username 'alice', got %q", got)
	}
	if got := RoleFromContext(ctx); got != RoleDeployer {
		t.Errorf("expected role %q, got %q", RoleDeployer, got)
	}
}

func TestWithIdentityOverwrite(t *testing.T) {
	ctx := WithIdentity(context.Background(), "alice", RoleAdmin)
	ctx = WithIdentity(ctx, "bob", RoleViewer)

	if got := UsernameFromContext(ctx); got != "bob" {
		t.Errorf("expected overwritten username 'bob', got %q", got)
	}
	if got := RoleFromContext(ctx); got != RoleViewer {
		t.Errorf("expected overwritten role %q, got %q", RoleViewer, got)
	}
}
