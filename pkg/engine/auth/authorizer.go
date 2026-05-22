package auth

import (
	"context"
	"fmt"
)

// RoleAuthorizer implements Authorizer using the static role→permission map
// defined in roles.go. It checks whether the subject's role grants access
// to the requested resource+action pair.
type RoleAuthorizer struct{}

// NewRoleAuthorizer creates a new RoleAuthorizer.
func NewRoleAuthorizer() *RoleAuthorizer {
	return &RoleAuthorizer{}
}

// Authorize checks if the subject (role name) is allowed to perform the
// given action on the given resource. Returns nil if allowed, or an error
// with the required role name for helpful CLI error messages.
func (a *RoleAuthorizer) Authorize(_ context.Context, subject, resource, action string) error {
	perms, ok := rolePermissions[subject]
	if !ok {
		return &PermissionDeniedError{
			Resource:     resource,
			Action:       action,
			RequiredRole: RequiredRoleForPermission(resource, action),
		}
	}

	for _, p := range perms {
		if (p.Resource == "*" || p.Resource == resource) &&
			(p.Action == "*" || p.Action == action) {
			return nil
		}
	}

	return &PermissionDeniedError{
		Resource:     resource,
		Action:       action,
		RequiredRole: RequiredRoleForPermission(resource, action),
	}
}

// PermissionDeniedError carries enough context for the CLI to show
// an actionable error message: "Need the 'deployer' role."
type PermissionDeniedError struct {
	Resource     string
	Action       string
	RequiredRole string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf(
		"permission denied: %s/%s requires the '%s' role",
		e.Resource, e.Action, e.RequiredRole,
	)
}
