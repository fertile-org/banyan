package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

const bcryptCost = 12

// EtcdUserStore implements UserStore using a storage.StateStore backend (etcd or memory).
type EtcdUserStore struct {
	store storage.StateStore
}

// NewEtcdUserStore creates a new user store backed by the given StateStore.
func NewEtcdUserStore(store storage.StateStore) *EtcdUserStore {
	return &EtcdUserStore{store: store}
}

// Create adds a new user. Returns error if username already exists or role is invalid.
func (s *EtcdUserStore) Create(ctx context.Context, user *User) error {
	if err := validateUsername(user.Username); err != nil {
		return err
	}
	if !ValidRoles[user.Role] {
		return fmt.Errorf("invalid role %q: must be one of admin, deployer, viewer", user.Role)
	}

	key := types.KeyAuthUsers + user.Username
	var existing User
	if err := s.store.Get(ctx, key, &existing); err == nil {
		return fmt.Errorf("user %q already exists", user.Username)
	}

	user.CreatedAt = time.Now()
	return s.store.Save(ctx, key, user)
}

// Get retrieves a user by username. Returns error if not found.
func (s *EtcdUserStore) Get(ctx context.Context, username string) (*User, error) {
	key := types.KeyAuthUsers + username
	var user User
	if err := s.store.Get(ctx, key, &user); err != nil {
		return nil, fmt.Errorf("user %q not found", username)
	}
	return &user, nil
}

// Update modifies an existing user. Enforces the last-admin guard:
// refuses to disable or demote the last active admin.
func (s *EtcdUserStore) Update(ctx context.Context, user *User) error {
	if user.Role != "" && !ValidRoles[user.Role] {
		return fmt.Errorf("invalid role %q: must be one of admin, deployer, viewer", user.Role)
	}

	key := types.KeyAuthUsers + user.Username
	var existing User
	if err := s.store.Get(ctx, key, &existing); err != nil {
		return fmt.Errorf("user %q not found", user.Username)
	}

	// Last-admin guard: if this user is currently an active admin,
	// check whether the update would remove them as admin.
	losingAdmin := existing.Role == RoleAdmin && !existing.Disabled &&
		(user.Disabled || (user.Role != "" && user.Role != RoleAdmin))
	if losingAdmin {
		if err := s.ensureOtherAdminExists(ctx, user.Username); err != nil {
			return err
		}
	}

	// Apply changes (preserve fields the caller didn't set)
	if user.Role != "" {
		existing.Role = user.Role
	}
	if user.PasswordHash != "" {
		existing.PasswordHash = user.PasswordHash
	}
	existing.Disabled = user.Disabled

	return s.store.Save(ctx, key, &existing)
}

// Delete removes a user. Enforces the last-admin guard:
// refuses to delete the last active admin.
func (s *EtcdUserStore) Delete(ctx context.Context, username string) error {
	key := types.KeyAuthUsers + username
	var existing User
	if err := s.store.Get(ctx, key, &existing); err != nil {
		return fmt.Errorf("user %q not found", username)
	}

	if existing.Role == RoleAdmin && !existing.Disabled {
		if err := s.ensureOtherAdminExists(ctx, username); err != nil {
			return err
		}
	}

	return s.store.Delete(ctx, key)
}

// List returns all users. Password hashes are cleared from the returned records.
func (s *EtcdUserStore) List(ctx context.Context) ([]*User, error) {
	keys, err := s.store.List(ctx, types.KeyAuthUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	var users []*User
	for _, k := range keys {
		var user User
		if err := s.store.Get(ctx, k, &user); err != nil {
			continue
		}
		user.PasswordHash = "" // never expose hashes
		users = append(users, &user)
	}
	return users, nil
}

// ensureOtherAdminExists checks that at least one other active admin
// exists besides the given username. Returns error if this is the last admin.
func (s *EtcdUserStore) ensureOtherAdminExists(ctx context.Context, excludeUsername string) error {
	keys, err := s.store.List(ctx, types.KeyAuthUsers)
	if err != nil {
		return fmt.Errorf("failed to check admin count: %w", err)
	}
	for _, k := range keys {
		var u User
		if err := s.store.Get(ctx, k, &u); err != nil {
			continue
		}
		if u.Username != excludeUsername && u.Role == RoleAdmin && !u.Disabled {
			return nil // another active admin exists
		}
	}
	return fmt.Errorf("cannot remove the last admin: at least one active admin must exist")
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword verifies a plaintext password against a bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// validateUsername checks that a username is reasonable.
func validateUsername(username string) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	if len(username) > 64 {
		return fmt.Errorf("username must be 64 characters or fewer")
	}
	if strings.ContainsAny(username, " /\\") {
		return fmt.Errorf("username cannot contain spaces or slashes")
	}
	return nil
}
