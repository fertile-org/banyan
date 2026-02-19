package storage

import (
	"context"
	"testing"
	"time"
)

// TestEtcdStore tests basic Save/Get/Delete/List operations
// NOTE: This test requires a running etcd server
// Run with: etcd &  # Start etcd in background
//           go test -v ./pkg/vpc/storage -run TestEtcdStore
func TestEtcdStore(t *testing.T) {
	// Skip if no etcd available
	store, err := NewEtcdStore([]string{"http://localhost:2379"}, "/banyan/test/")
	if err != nil {
		t.Skipf("Skipping etcd tests - no etcd server available: %v", err)
		return
	}
	defer store.Close()

	ctx := context.Background()

	// Clean up before test
	keys, _ := store.List(ctx, "")
	for _, key := range keys {
		store.Delete(ctx, key)
	}

	// Test Save and Get
	t.Run("SaveAndGet", func(t *testing.T) {
		type TestData struct {
			Name  string
			Value int
		}

		data := TestData{Name: "test", Value: 42}
		err := store.Save(ctx, "test/key1", &data)
		if err != nil {
			t.Fatalf("Failed to save: %v", err)
		}

		var retrieved TestData
		err = store.Get(ctx, "test/key1", &retrieved)
		if err != nil {
			t.Fatalf("Failed to get: %v", err)
		}

		if retrieved.Name != data.Name || retrieved.Value != data.Value {
			t.Errorf("Retrieved data mismatch: got %+v, want %+v", retrieved, data)
		}
	})

	// Test Get non-existent key
	t.Run("GetNonExistent", func(t *testing.T) {
		var data map[string]string
		err := store.Get(ctx, "nonexistent", &data)
		if err == nil {
			t.Error("Expected error for non-existent key, got nil")
		}
	})

	// Test Delete
	t.Run("Delete", func(t *testing.T) {
		data := map[string]string{"foo": "bar"}
		err := store.Save(ctx, "test/key2", &data)
		if err != nil {
			t.Fatalf("Failed to save: %v", err)
		}

		err = store.Delete(ctx, "test/key2")
		if err != nil {
			t.Fatalf("Failed to delete: %v", err)
		}

		var retrieved map[string]string
		err = store.Get(ctx, "test/key2", &retrieved)
		if err == nil {
			t.Error("Expected error after delete, got nil")
		}
	})

	// Test List
	t.Run("List", func(t *testing.T) {
		// Save multiple keys
		for i := 0; i < 5; i++ {
			key := "test/items/" + string(rune('a'+i))
			data := map[string]int{"value": i}
			err := store.Save(ctx, key, &data)
			if err != nil {
				t.Fatalf("Failed to save key %s: %v", key, err)
			}
		}

		// List with prefix
		keys, err := store.List(ctx, "test/items/")
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}

		if len(keys) != 5 {
			t.Errorf("Expected 5 keys, got %d: %v", len(keys), keys)
		}

		// Cleanup
		for _, key := range keys {
			store.Delete(ctx, key)
		}
	})

	// Test SaveWithTTL
	t.Run("SaveWithTTL", func(t *testing.T) {
		data := map[string]string{"expires": "soon"}
		err := store.SaveWithTTL(ctx, "test/ttl-key", &data, 2*time.Second)
		if err != nil {
			t.Fatalf("Failed to save with TTL: %v", err)
		}

		// Verify it exists
		var retrieved map[string]string
		err = store.Get(ctx, "test/ttl-key", &retrieved)
		if err != nil {
			t.Fatalf("Failed to get TTL key: %v", err)
		}

		if retrieved["expires"] != "soon" {
			t.Errorf("Retrieved data mismatch: got %v", retrieved)
		}

		// Wait for expiration
		time.Sleep(3 * time.Second)

		// Verify it expired
		err = store.Get(ctx, "test/ttl-key", &retrieved)
		if err == nil {
			t.Error("Expected key to expire after TTL, but it still exists")
		}
	})

	// Test empty key errors
	t.Run("EmptyKey", func(t *testing.T) {
		data := map[string]string{"test": "data"}

		err := store.Save(ctx, "", &data)
		if err == nil {
			t.Error("Expected error for empty key in Save")
		}

		err = store.Get(ctx, "", &data)
		if err == nil {
			t.Error("Expected error for empty key in Get")
		}

		err = store.Delete(ctx, "")
		if err == nil {
			t.Error("Expected error for empty key in Delete")
		}
	})

	// Test nil value errors
	t.Run("NilValue", func(t *testing.T) {
		err := store.Save(ctx, "test/nil", nil)
		if err == nil {
			t.Error("Expected error for nil value in Save")
		}

		err = store.Get(ctx, "test/key1", nil)
		if err == nil {
			t.Error("Expected error for nil value in Get")
		}
	})
}

// TestEtcdStorePrefix tests that the prefix is correctly applied
func TestEtcdStorePrefix(t *testing.T) {
	store, err := NewEtcdStore([]string{"http://localhost:2379"}, "/prefix/test/")
	if err != nil {
		t.Skipf("Skipping etcd tests - no etcd server available: %v", err)
		return
	}
	defer store.Close()

	if store.prefix != "/prefix/test/" {
		t.Errorf("Expected prefix '/prefix/test/', got '%s'", store.prefix)
	}

	// Test with prefix without trailing slash
	store2, err := NewEtcdStore([]string{"http://localhost:2379"}, "/another/prefix")
	if err != nil {
		t.Skipf("Skipping etcd tests - no etcd server available: %v", err)
		return
	}
	defer store2.Close()

	if store2.prefix != "/another/prefix/" {
		t.Errorf("Expected prefix to have trailing slash, got '%s'", store2.prefix)
	}

	// Test with empty prefix (should use default)
	store3, err := NewEtcdStore([]string{"http://localhost:2379"}, "")
	if err != nil {
		t.Skipf("Skipping etcd tests - no etcd server available: %v", err)
		return
	}
	defer store3.Close()

	if store3.prefix != "/banyan/vpc/" {
		t.Errorf("Expected default prefix '/banyan/vpc/', got '%s'", store3.prefix)
	}
}

// TestEtcdStoreNoEndpoints tests error when no endpoints provided
func TestEtcdStoreNoEndpoints(t *testing.T) {
	_, err := NewEtcdStore([]string{}, "/test/")
	if err == nil {
		t.Error("Expected error when creating store with no endpoints")
	}
}
