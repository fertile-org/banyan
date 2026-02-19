package storage

import (
	"context"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

func newTestBadgerStore(t *testing.T) *BadgerStore {
	t.Helper()

	opts := badger.DefaultOptions(t.TempDir())
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open badger: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return NewBadgerStoreWithDB(db, "/banyan")
}

func TestBadgerStore_SaveAndGet(t *testing.T) {
	store := newTestBadgerStore(t)
	ctx := context.Background()

	t.Run("save and get string", func(t *testing.T) {
		if err := store.Save(ctx, "key1", "hello"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		var got string
		if err := store.Get(ctx, "key1", &got); err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got != "hello" {
			t.Errorf("expected 'hello', got %q", got)
		}
	})

	t.Run("save and get struct", func(t *testing.T) {
		type testData struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		}
		input := testData{Name: "test", Count: 42}

		if err := store.Save(ctx, "key2", input); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		var got testData
		if err := store.Get(ctx, "key2", &got); err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.Name != "test" || got.Count != 42 {
			t.Errorf("expected {test, 42}, got %+v", got)
		}
	})

	t.Run("get nonexistent key", func(t *testing.T) {
		var got string
		err := store.Get(ctx, "nonexistent", &got)
		if err == nil {
			t.Fatal("expected error for nonexistent key")
		}
	})

	t.Run("empty key returns error", func(t *testing.T) {
		if err := store.Save(ctx, "", "val"); err == nil {
			t.Fatal("expected error for empty key on Save")
		}
		var v string
		if err := store.Get(ctx, "", &v); err == nil {
			t.Fatal("expected error for empty key on Get")
		}
	})

	t.Run("nil value returns error", func(t *testing.T) {
		if err := store.Save(ctx, "k", nil); err == nil {
			t.Fatal("expected error for nil value on Save")
		}
		if err := store.Get(ctx, "k", nil); err == nil {
			t.Fatal("expected error for nil value on Get")
		}
	})
}

func TestBadgerStore_Delete(t *testing.T) {
	store := newTestBadgerStore(t)
	ctx := context.Background()

	store.Save(ctx, "del-key", "value")

	if err := store.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	var got string
	if err := store.Get(ctx, "del-key", &got); err == nil {
		t.Fatal("expected error after delete")
	}

	// Deleting non-existent key should not error (badger ignores missing keys on delete)
	if err := store.Delete(ctx, "nonexistent"); err != nil {
		t.Fatalf("Delete of nonexistent key should not error: %v", err)
	}

	// Empty key returns error
	if err := store.Delete(ctx, ""); err == nil {
		t.Fatal("expected error for empty key on Delete")
	}
}

func TestBadgerStore_List(t *testing.T) {
	store := newTestBadgerStore(t)
	ctx := context.Background()

	store.Save(ctx, "nodes/worker-1", "a")
	store.Save(ctx, "nodes/worker-2", "b")
	store.Save(ctx, "tasks/worker-1/t1", "c")

	t.Run("list by prefix", func(t *testing.T) {
		keys, err := store.List(ctx, "nodes/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
		}
	})

	t.Run("list tasks prefix", func(t *testing.T) {
		keys, err := store.List(ctx, "tasks/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("expected 1 key, got %d: %v", len(keys), keys)
		}
		if keys[0] != "tasks/worker-1/t1" {
			t.Errorf("expected 'tasks/worker-1/t1', got %q", keys[0])
		}
	})

	t.Run("list nonexistent prefix returns empty", func(t *testing.T) {
		keys, err := store.List(ctx, "nothing/")
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})
}

func TestBadgerStore_Close(t *testing.T) {
	opts := badger.DefaultOptions(t.TempDir())
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open badger: %v", err)
	}

	store := NewBadgerStoreWithDB(db, "/banyan")
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestBadgerStore_PrefixHandling(t *testing.T) {
	opts := badger.DefaultOptions(t.TempDir())
	opts.Logger = nil
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("failed to open badger: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	t.Run("empty prefix gets default", func(t *testing.T) {
		store := NewBadgerStoreWithDB(db, "")
		if store.prefix != "/banyan/" {
			t.Errorf("expected default prefix '/banyan/', got %q", store.prefix)
		}
	})

	t.Run("prefix without trailing slash gets one", func(t *testing.T) {
		store := NewBadgerStoreWithDB(db, "/test")
		if store.prefix != "/test/" {
			t.Errorf("expected '/test/', got %q", store.prefix)
		}
	})
}
