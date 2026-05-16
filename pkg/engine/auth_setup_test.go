package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fertile-org/banyan/pkg/engine/auth"
	"github.com/fertile-org/banyan/pkg/storage"
)

func TestLoadOrCreateJWTKey_GeneratesOnFirstCall(t *testing.T) {
	store := storage.NewMemoryStore()
	key, err := loadOrCreateJWTKey(context.Background(), store)
	if err != nil {
		t.Fatalf("loadOrCreateJWTKey: %v", err)
	}
	if len(key) != jwtKeySize {
		t.Errorf("key length = %d, want %d", len(key), jwtKeySize)
	}
}

func TestLoadOrCreateJWTKey_StableAcrossCalls(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	key1, err := loadOrCreateJWTKey(ctx, store)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	key2, err := loadOrCreateJWTKey(ctx, store)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(key1) != string(key2) {
		t.Error("key should be stable across calls — second call regenerated it")
	}
}

func writeBootstrap(t *testing.T, dir string) string {
	t.Helper()
	// password_hash is a real bcrypt hash of "password123"
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	path := filepath.Join(dir, "auth-bootstrap.json")
	data := []byte(`{"username":"admin","password_hash":"` + hash + `","role":"admin"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	return path
}

func TestConsumeAuthBootstrap_CreatesAdminWhenNoUsers(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	path := writeBootstrap(t, t.TempDir())

	if err := consumeAuthBootstrap(context.Background(), users, path); err != nil {
		t.Fatalf("consumeAuthBootstrap: %v", err)
	}

	u, err := users.Get(context.Background(), "admin")
	if err != nil {
		t.Fatalf("admin user not created: %v", err)
	}
	if u.Role != auth.RoleAdmin {
		t.Errorf("admin role = %q, want %q", u.Role, auth.RoleAdmin)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("bootstrap file should be deleted after consumption")
	}
}

func TestConsumeAuthBootstrap_SkipsWhenUsersExist(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	ctx := context.Background()

	// Pre-existing user
	hash, _ := auth.HashPassword("existing")
	if err := users.Create(ctx, &auth.User{
		Username: "existing-admin", PasswordHash: hash, Role: auth.RoleAdmin,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	path := writeBootstrap(t, t.TempDir())
	if err := consumeAuthBootstrap(ctx, users, path); err != nil {
		t.Fatalf("consumeAuthBootstrap: %v", err)
	}

	// admin from bootstrap must NOT have been created
	if _, err := users.Get(ctx, "admin"); err == nil {
		t.Error("bootstrap admin should not be created when users already exist")
	}
	// stale bootstrap file should still be removed
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("stale bootstrap file should be deleted")
	}
}

func TestConsumeAuthBootstrap_NoFileIsNoOp(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	missing := filepath.Join(t.TempDir(), "auth-bootstrap.json")

	if err := consumeAuthBootstrap(context.Background(), users, missing); err != nil {
		t.Errorf("missing bootstrap file should be a no-op, got: %v", err)
	}
}
