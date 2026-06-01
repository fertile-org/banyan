package auth

import (
	"context"
	"errors"
	"testing"
)

func TestRoleAuthorizer_AdminAllowsEverything(t *testing.T) {
	a := NewRoleAuthorizer()
	ctx := context.Background()

	resources := []string{"deployment", "status", "logs", "secret", "user", "container"}
	actions := []string{"create", "read", "update", "delete", "scale", "update-self"}

	for _, r := range resources {
		for _, act := range actions {
			if err := a.Authorize(ctx, RoleAdmin, r, act); err != nil {
				t.Errorf("admin should be allowed %s/%s, got: %v", r, act, err)
			}
		}
	}
}

func TestRoleAuthorizer_DeployerPermissions(t *testing.T) {
	a := NewRoleAuthorizer()
	ctx := context.Background()

	allowed := []struct{ resource, action string }{
		{"deployment", "create"},
		{"deployment", "read"},
		{"deployment", "update"},
		{"deployment", "delete"},
		{"deployment", "scale"},
		{"container", "read"},
		{"logs", "read"},
		{"status", "read"},
		{"secret", "read"},
		{"user", "update-self"},
	}
	for _, tt := range allowed {
		if err := a.Authorize(ctx, RoleDeployer, tt.resource, tt.action); err != nil {
			t.Errorf("deployer should be allowed %s/%s, got: %v", tt.resource, tt.action, err)
		}
	}

	denied := []struct{ resource, action string }{
		{"secret", "create"},
		{"secret", "delete"},
		{"user", "create"},
		{"user", "delete"},
		{"user", "update"},
	}
	for _, tt := range denied {
		if err := a.Authorize(ctx, RoleDeployer, tt.resource, tt.action); err == nil {
			t.Errorf("deployer should be denied %s/%s", tt.resource, tt.action)
		}
	}
}

func TestRoleAuthorizer_ViewerPermissions(t *testing.T) {
	a := NewRoleAuthorizer()
	ctx := context.Background()

	allowed := []struct{ resource, action string }{
		{"deployment", "read"},
		{"container", "read"},
		{"logs", "read"},
		{"status", "read"},
		{"user", "update-self"},
	}
	for _, tt := range allowed {
		if err := a.Authorize(ctx, RoleViewer, tt.resource, tt.action); err != nil {
			t.Errorf("viewer should be allowed %s/%s, got: %v", tt.resource, tt.action, err)
		}
	}

	denied := []struct{ resource, action string }{
		{"deployment", "create"},
		{"deployment", "delete"},
		{"deployment", "scale"},
		{"secret", "read"},
		{"secret", "create"},
		{"user", "create"},
	}
	for _, tt := range denied {
		if err := a.Authorize(ctx, RoleViewer, tt.resource, tt.action); err == nil {
			t.Errorf("viewer should be denied %s/%s", tt.resource, tt.action)
		}
	}
}

func TestRoleAuthorizer_UnknownRole(t *testing.T) {
	a := NewRoleAuthorizer()
	err := a.Authorize(context.Background(), "wizard", "deployment", "create")
	if err == nil {
		t.Fatal("unknown role should be denied")
	}
}

func TestRoleAuthorizer_UnknownResource(t *testing.T) {
	a := NewRoleAuthorizer()
	// Even admin allows unknown resources (wildcard)
	if err := a.Authorize(context.Background(), RoleAdmin, "spaceship", "launch"); err != nil {
		t.Errorf("admin wildcard should allow any resource: %v", err)
	}
	// Deployer does not
	if err := a.Authorize(context.Background(), RoleDeployer, "spaceship", "launch"); err == nil {
		t.Error("deployer should not have access to unknown resources")
	}
}

func TestPermissionDeniedError_Format(t *testing.T) {
	a := NewRoleAuthorizer()
	err := a.Authorize(context.Background(), RoleViewer, "deployment", "create")
	if err == nil {
		t.Fatal("expected error")
	}

	var pde *PermissionDeniedError
	if !errors.As(err, &pde) {
		t.Fatalf("expected PermissionDeniedError, got %T", err)
	}
	if pde.RequiredRole != RoleDeployer {
		t.Errorf("required role = %q, want %q", pde.RequiredRole, RoleDeployer)
	}
	if pde.Resource != "deployment" {
		t.Errorf("resource = %q, want 'deployment'", pde.Resource)
	}
	if pde.Action != "create" {
		t.Errorf("action = %q, want 'create'", pde.Action)
	}

	msg := err.Error()
	expected := "permission denied: deployment/create requires the 'deployer' role"
	if msg != expected {
		t.Errorf("error message = %q, want %q", msg, expected)
	}
}

func TestPermissionDeniedError_SecretCreate(t *testing.T) {
	a := NewRoleAuthorizer()
	err := a.Authorize(context.Background(), RoleDeployer, "secret", "create")

	var pde *PermissionDeniedError
	if !errors.As(err, &pde) {
		t.Fatalf("expected PermissionDeniedError, got %T", err)
	}
	if pde.RequiredRole != RoleAdmin {
		t.Errorf("required role = %q, want %q", pde.RequiredRole, RoleAdmin)
	}
}

func TestRoleAuthorizer_ImplementsInterface(t *testing.T) {
	var _ Authorizer = (*RoleAuthorizer)(nil)
}
