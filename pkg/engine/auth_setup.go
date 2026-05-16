// auth_setup.go wires the pkg/engine/auth package into the running engine:
// it generates/loads the JWT signing key, consumes the init-time admin
// bootstrap file, and assembles the auth.AuthDeps bundle the gRPC and
// Connect-go servers need to enforce authentication.
package engine

import (
	"context"
	"crypto/rand"
	"fmt"

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
