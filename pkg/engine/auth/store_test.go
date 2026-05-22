package auth

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/storage"
)

func newTestStore() *EtcdUserStore {
	return NewEtcdUserStore(storage.NewMemoryStore())
}

func createTestUser(t *testing.T, s *EtcdUserStore, username, role, createdBy string) {
	t.Helper()
	hash, err := HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	err = s.Create(context.Background(), &User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		CreatedBy:    createdBy,
	})
	if err != nil {
		t.Fatalf("Create(%s): %v", username, err)
	}
}

func TestCreate_HappyPath(t *testing.T) {
	s := newTestStore()
	hash, _ := HashPassword("secret")
	err := s.Create(context.Background(), &User{
		Username:     "alice",
		PasswordHash: hash,
		Role:         RoleAdmin,
		CreatedBy:    "system",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	user, err := s.Get(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("username = %q, want 'alice'", user.Username)
	}
	if user.Role != RoleAdmin {
		t.Errorf("role = %q, want %q", user.Role, RoleAdmin)
	}
	if user.CreatedBy != "system" {
		t.Errorf("created_by = %q, want 'system'", user.CreatedBy)
	}
	if user.CreatedAt.IsZero() {
		t.Error("created_at should not be zero")
	}
}

func TestCreate_DuplicateUsername(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "alice", RoleAdmin, "system")

	hash, _ := HashPassword("other")
	err := s.Create(context.Background(), &User{
		Username:     "alice",
		PasswordHash: hash,
		Role:         RoleViewer,
	})
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestCreate_InvalidRole(t *testing.T) {
	s := newTestStore()
	hash, _ := HashPassword("pw")
	err := s.Create(context.Background(), &User{
		Username:     "bob",
		PasswordHash: hash,
		Role:         "superadmin",
	})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestCreate_InvalidUsername(t *testing.T) {
	s := newTestStore()
	hash, _ := HashPassword("pw")

	tests := []struct {
		name     string
		username string
	}{
		{"empty", ""},
		{"spaces", "alice bob"},
		{"slash", "alice/bob"},
		{"backslash", "alice\\bob"},
		{"too long", string(make([]byte, 65))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Create(context.Background(), &User{
				Username:     tt.username,
				PasswordHash: hash,
				Role:         RoleViewer,
			})
			if err == nil {
				t.Errorf("expected error for username %q", tt.username)
			}
		})
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newTestStore()
	_, err := s.Get(context.Background(), "nobody")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestUpdate_ChangeRole(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "admin2", RoleAdmin, "system")

	err := s.Update(context.Background(), &User{
		Username: "admin1",
		Role:     RoleDeployer,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	user, _ := s.Get(context.Background(), "admin1")
	if user.Role != RoleDeployer {
		t.Errorf("role = %q, want %q", user.Role, RoleDeployer)
	}
}

func TestUpdate_ChangePassword(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "alice", RoleViewer, "admin")

	newHash, _ := HashPassword("newpassword")
	err := s.Update(context.Background(), &User{
		Username:     "alice",
		PasswordHash: newHash,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	user, _ := s.Get(context.Background(), "alice")
	if !CheckPassword(user.PasswordHash, "newpassword") {
		t.Error("password should have been updated")
	}
}

func TestUpdate_DisableUser(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "bob", RoleDeployer, "admin1")

	err := s.Update(context.Background(), &User{
		Username: "bob",
		Disabled: true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	user, _ := s.Get(context.Background(), "bob")
	if !user.Disabled {
		t.Error("user should be disabled")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s := newTestStore()
	err := s.Update(context.Background(), &User{
		Username: "ghost",
		Role:     RoleViewer,
	})
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestUpdate_InvalidRole(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "alice", RoleViewer, "system")

	err := s.Update(context.Background(), &User{
		Username: "alice",
		Role:     "wizard",
	})
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

// --- Last-admin guard tests ---

func TestUpdate_LastAdminDemote_Blocked(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "solo-admin", RoleAdmin, "system")

	err := s.Update(context.Background(), &User{
		Username: "solo-admin",
		Role:     RoleViewer,
	})
	if err == nil {
		t.Fatal("expected error when demoting the last admin")
	}
}

func TestUpdate_LastAdminDisable_Blocked(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "solo-admin", RoleAdmin, "system")

	err := s.Update(context.Background(), &User{
		Username: "solo-admin",
		Disabled: true,
	})
	if err == nil {
		t.Fatal("expected error when disabling the last admin")
	}
}

func TestUpdate_DemoteAdminWithOtherAdmin_Allowed(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "admin2", RoleAdmin, "system")

	err := s.Update(context.Background(), &User{
		Username: "admin1",
		Role:     RoleViewer,
	})
	if err != nil {
		t.Fatalf("should allow demote when another admin exists: %v", err)
	}
}

func TestDelete_HappyPath(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "bob", RoleDeployer, "admin1")

	err := s.Delete(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = s.Get(context.Background(), "bob")
	if err == nil {
		t.Fatal("expected user to be deleted")
	}
}

func TestDelete_NotFound(t *testing.T) {
	s := newTestStore()
	err := s.Delete(context.Background(), "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestDelete_LastAdmin_Blocked(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "solo-admin", RoleAdmin, "system")

	err := s.Delete(context.Background(), "solo-admin")
	if err == nil {
		t.Fatal("expected error when deleting the last admin")
	}

	// Verify user still exists
	user, err := s.Get(context.Background(), "solo-admin")
	if err != nil {
		t.Fatalf("user should still exist: %v", err)
	}
	if user.Username != "solo-admin" {
		t.Error("wrong user returned")
	}
}

func TestDelete_AdminWithOtherAdmin_Allowed(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "admin2", RoleAdmin, "system")

	err := s.Delete(context.Background(), "admin1")
	if err != nil {
		t.Fatalf("should allow delete when another admin exists: %v", err)
	}
}

func TestDelete_DisabledAdminDoesNotCount(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "admin1", RoleAdmin, "system")
	createTestUser(t, s, "admin2", RoleAdmin, "system")

	// Disable admin2 first
	_ = s.Update(context.Background(), &User{
		Username: "admin2",
		Disabled: true,
	})

	// Now admin1 is the last *active* admin — delete should be blocked
	err := s.Delete(context.Background(), "admin1")
	if err == nil {
		t.Fatal("expected error: admin2 is disabled so admin1 is the last active admin")
	}
}

func TestList_Empty(t *testing.T) {
	s := newTestStore()
	users, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestList_MultipleUsers(t *testing.T) {
	s := newTestStore()
	createTestUser(t, s, "alice", RoleAdmin, "system")
	createTestUser(t, s, "bob", RoleDeployer, "alice")
	createTestUser(t, s, "charlie", RoleViewer, "alice")

	users, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(users))
	}

	// Verify password hashes are cleared
	for _, u := range users {
		if u.PasswordHash != "" {
			t.Errorf("password hash should be cleared for user %q", u.Username)
		}
	}
}

// --- Password hashing tests ---

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("mysecret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if hash == "mysecret" {
		t.Fatal("hash should not equal plaintext")
	}
	if !CheckPassword(hash, "mysecret") {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword(hash, "wrongpassword") {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestCheckPassword_EmptyHash(t *testing.T) {
	if CheckPassword("", "anything") {
		t.Error("empty hash should never match")
	}
}

// --- Username validation tests ---

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		username string
		wantErr  bool
	}{
		{"alice", false},
		{"bob-123", false},
		{"admin_user", false},
		{"user.name", false},
		{"a", false},
		{"", true},
		{"alice bob", true},
		{"user/name", true},
		{"user\\name", true},
		{string(make([]byte, 65)), true},
	}
	for _, tt := range tests {
		err := validateUsername(tt.username)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateUsername(%q) err = %v, wantErr = %v", tt.username, err, tt.wantErr)
		}
	}
}
