package engine

import (
	"context"
	"testing"

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
