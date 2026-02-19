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
	t.Run("close open client", func(t *testing.T) {
		store, _ := newTestRedisStore(t)
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	})

	t.Run("close nil client", func(t *testing.T) {
		store := &RedisStore{client: nil, prefix: "/banyan/"}
		if err := store.Close(); err != nil {
			t.Fatalf("Close with nil client should not error: %v", err)
		}
	})
}

func TestRedisStore_Overwrite(t *testing.T) {
	store, _ := newTestRedisStore(t)
	ctx := context.Background()

	store.Save(ctx, "key", "first")
	store.Save(ctx, "key", "second")

	var got string
	if err := store.Get(ctx, "key", &got); err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

func TestNewRedisStore_DefaultAddress(t *testing.T) {
	// This test verifies the constructor behavior with miniredis
	mr := miniredis.RunT(t)
	store, err := NewRedisStore(mr.Addr(), "")
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	defer store.Close()

	if store.prefix != "/banyan/" {
		t.Errorf("expected default prefix '/banyan/', got %q", store.prefix)
	}
}

func TestNewRedisStore_PrefixNoSlash(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := NewRedisStore(mr.Addr(), "/custom")
	if err != nil {
		t.Fatalf("NewRedisStore failed: %v", err)
	}
	defer store.Close()

	if store.prefix != "/custom/" {
		t.Errorf("expected '/custom/', got %q", store.prefix)
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

func TestRedisStore_SaveMarshalError(t *testing.T) {
	store, _ := newTestRedisStore(t)
	ctx := context.Background()

	// A channel cannot be JSON-marshalled
	err := store.Save(ctx, "bad-key", make(chan int))
	if err == nil {
		t.Fatal("expected marshal error for unsupported type")
	}
}

func TestRedisStore_GetUnmarshalError(t *testing.T) {
	store, mr := newTestRedisStore(t)
	ctx := context.Background()

	// Write invalid JSON directly to miniredis
	mr.Set(store.prefix+"bad-json", "not valid json {{{")

	var got string
	err := store.Get(ctx, "bad-json", &got)
	if err == nil {
		t.Fatal("expected unmarshal error for corrupt data")
	}
}

func TestRedisStore_ServerDown(t *testing.T) {
	store, mr := newTestRedisStore(t)
	ctx := context.Background()

	// Save some data first
	store.Save(ctx, "key1", "value1")

	// Close miniredis to simulate server failure
	mr.Close()

	t.Run("save fails", func(t *testing.T) {
		err := store.Save(ctx, "key2", "value2")
		if err == nil {
			t.Fatal("expected error when redis is down")
		}
	})

	t.Run("get fails", func(t *testing.T) {
		var got string
		err := store.Get(ctx, "key1", &got)
		if err == nil {
			t.Fatal("expected error when redis is down")
		}
	})

	t.Run("delete fails", func(t *testing.T) {
		err := store.Delete(ctx, "key1")
		if err == nil {
			t.Fatal("expected error when redis is down")
		}
	})

	t.Run("list fails", func(t *testing.T) {
		_, err := store.List(ctx, "")
		if err == nil {
			t.Fatal("expected error when redis is down")
		}
	})
}
