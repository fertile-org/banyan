package storage

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedisStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := NewRedisStoreWithClient(client, "/banyan")

	return store, mr
}

func TestRedisStore_SaveAndGet(t *testing.T) {
	store, _ := newTestRedisStore(t)
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

func TestRedisStore_Delete(t *testing.T) {
	store, _ := newTestRedisStore(t)
	ctx := context.Background()

	store.Save(ctx, "del-key", "value")

	if err := store.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	var got string
	if err := store.Get(ctx, "del-key", &got); err == nil {
		t.Fatal("expected error after delete")
	}

	// Deleting non-existent key should not error
	if err := store.Delete(ctx, "nonexistent"); err != nil {
		t.Fatalf("Delete of nonexistent key should not error: %v", err)
	}

	// Empty key returns error
	if err := store.Delete(ctx, ""); err == nil {
		t.Fatal("expected error for empty key on Delete")
	}
}

func TestRedisStore_List(t *testing.T) {
	store, _ := newTestRedisStore(t)
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

func TestRedisStore_Close(t *testing.T) {
	store, _ := newTestRedisStore(t)

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestRedisStore_PrefixHandling(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Run("empty prefix gets default", func(t *testing.T) {
		store := NewRedisStoreWithClient(client, "")
		if store.prefix != "/banyan/" {
			t.Errorf("expected default prefix '/banyan/', got %q", store.prefix)
		}
	})

	t.Run("prefix without trailing slash gets one", func(t *testing.T) {
		store := NewRedisStoreWithClient(client, "/test")
		if store.prefix != "/test/" {
			t.Errorf("expected '/test/', got %q", store.prefix)
		}
	})
}
