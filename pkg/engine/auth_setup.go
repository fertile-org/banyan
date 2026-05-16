// auth_setup.go wires the pkg/engine/auth package into the running engine:
// it generates/loads the JWT signing key, consumes the init-time admin
// bootstrap file, and assembles the auth.AuthDeps bundle the gRPC and
// Connect-go servers need to enforce authentication.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"

	"github.com/fertile-org/banyan/pkg/engine/auth"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// jwtKeySize is the byte length of the HMAC-SHA256 signing key.
const jwtKeySize = 32

// jwtKeyRecord is the etcd-stored wrapper for the JWT signing key.
type jwtKeyRecord struct {
	Key []byte `json:"key"`
}

// loadOrCreateJWTKey returns the engine's JWT signing key, generating and
// persisting a new one in etcd at types.KeyAuthJWTKey on first start.
func loadOrCreateJWTKey(ctx context.Context, store storage.StateStore) ([]byte, error) {
	var rec jwtKeyRecord
	if err := store.Get(ctx, types.KeyAuthJWTKey, &rec); err == nil && len(rec.Key) == jwtKeySize {
		return rec.Key, nil
	}

	key := make([]byte, jwtKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate JWT signing key: %w", err)
	}
	if err := store.Save(ctx, types.KeyAuthJWTKey, &jwtKeyRecord{Key: key}); err != nil {
		return nil, fmt.Errorf("failed to persist JWT signing key: %w", err)
	}
	return key, nil
}

// bootstrapRecord mirrors the JSON written by `banyan-engine init` to
// auth-bootstrap.json. The password is already bcrypt-hashed by init.
type bootstrapRecord struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
}

// consumeAuthBootstrap reads the init-time admin seed file and creates the
// first admin user if no users exist yet. The file is always removed after
// processing so it is consumed exactly once. A missing file is a no-op.
func consumeAuthBootstrap(ctx context.Context, users auth.UserStore, bootstrapPath string) error {
	data, err := os.ReadFile(bootstrapPath)
	if err != nil {
		return nil // no bootstrap file — nothing to do
	}

	var rec bootstrapRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("invalid auth bootstrap file %s: %w", bootstrapPath, err)
	}

	existing, err := users.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to check existing users: %w", err)
	}
	if len(existing) > 0 {
		// Users already exist — the bootstrap seed is stale. Remove it.
		_ = os.Remove(bootstrapPath)
		return nil
	}

	if err := users.Create(ctx, &auth.User{
		Username:     rec.Username,
		PasswordHash: rec.PasswordHash,
		Role:         rec.Role,
		CreatedBy:    "init",
	}); err != nil {
		return fmt.Errorf("failed to create bootstrap admin user: %w", err)
	}
	_ = os.Remove(bootstrapPath)
	return nil
}
