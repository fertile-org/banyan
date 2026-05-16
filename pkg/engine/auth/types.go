package auth

import (
	"context"
	"time"
)

// Role constants for Banyan authorization.
const (
	RoleAdmin    = "admin"
	RoleDeployer = "deployer"
	RoleViewer   = "viewer"
)

// ValidRoles is the set of valid role strings.
var ValidRoles = map[string]bool{
	RoleAdmin:    true,
	RoleDeployer: true,
	RoleViewer:   true,
}

// User represents an authenticated Banyan user stored in etcd.
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
	Disabled     bool      `json:"disabled"`
}

// Authorizer checks whether a subject is allowed to perform an action on a resource.
// Implementations return nil to allow, or a descriptive error to deny.
//
//	err := authorizer.Authorize(ctx, "alice", "deployment", "create")
//	if err != nil { /* denied */ }
type Authorizer interface {
	Authorize(ctx context.Context, subject, resource, action string) error
}

// UserStore manages user accounts in the backing store.
type UserStore interface {
	Create(ctx context.Context, user *User) error
	Get(ctx context.Context, username string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, username string) error
	List(ctx context.Context) ([]*User, error)
}

// ContextKey is the type for context keys used by the auth package.
type ContextKey string

const (
	// ContextKeyUsername is the context key for the authenticated username.
	ContextKeyUsername ContextKey = "auth_username"
	// ContextKeyRole is the context key for the authenticated user's role.
	ContextKeyRole ContextKey = "auth_role"
)

// UsernameFromContext extracts the authenticated username from the context.
func UsernameFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyUsername).(string)
	return v
}

// RoleFromContext extracts the authenticated user's role from the context.
func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ContextKeyRole).(string)
	return v
}

// WithIdentity returns a new context with the authenticated user's identity.
func WithIdentity(ctx context.Context, username, role string) context.Context {
	ctx = context.WithValue(ctx, ContextKeyUsername, username)
	ctx = context.WithValue(ctx, ContextKeyRole, role)
	return ctx
}
