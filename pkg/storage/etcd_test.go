package storage

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// mockEtcdKV is an in-memory mock of etcdKV for unit tests.
type mockEtcdKV struct {
	mu      sync.RWMutex
	data    map[string]string
	leaseID atomic.Int64
}

func newMockEtcdKV() *mockEtcdKV {
	return &mockEtcdKV{data: make(map[string]string)}
}

func (m *mockEtcdKV) Put(_ context.Context, key, val string, _ ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return &clientv3.PutResponse{}, nil
}

func (m *mockEtcdKV) Get(_ context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var kvs []*mvccpb.KeyValue

	if len(opts) > 0 {
		// Treat as prefix scan (WithPrefix is the only option used in List)
		for k, v := range m.data {
			if strings.HasPrefix(k, key) {
				kvs = append(kvs, &mvccpb.KeyValue{
					Key:   []byte(k),
					Value: []byte(v),
				})
			}
		}
		// Sort for deterministic order
		sort.Slice(kvs, func(i, j int) bool {
			return string(kvs[i].Key) < string(kvs[j].Key)
		})
	} else {
		// Exact key lookup
		if val, ok := m.data[key]; ok {
			kvs = append(kvs, &mvccpb.KeyValue{
				Key:   []byte(key),
				Value: []byte(val),
			})
		}
	}

	return &clientv3.GetResponse{Kvs: kvs}, nil
}

func (m *mockEtcdKV) Delete(_ context.Context, key string, _ ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return &clientv3.DeleteResponse{}, nil
}

func (m *mockEtcdKV) Grant(_ context.Context, _ int64) (*clientv3.LeaseGrantResponse, error) {
	id := m.leaseID.Add(1)
	return &clientv3.LeaseGrantResponse{ID: clientv3.LeaseID(id)}, nil
}

func (m *mockEtcdKV) KeepAliveOnce(_ context.Context, _ clientv3.LeaseID) (*clientv3.LeaseKeepAliveResponse, error) {
	return &clientv3.LeaseKeepAliveResponse{}, nil
}

func (m *mockEtcdKV) Watch(_ context.Context, _ string, _ ...clientv3.OpOption) clientv3.WatchChan {
	return nil
}

func (m *mockEtcdKV) Close() error {
	return nil
}

// newMockEtcdStore creates an EtcdStore backed by the in-memory mock.
func newMockEtcdStore() *EtcdStore {
	return NewEtcdStoreWithClient(newMockEtcdKV(), "/banyan/test/")
}

func TestEtcdStore_SaveAndGet(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	t.Run("String", func(t *testing.T) {
		err := store.Save(ctx, "key1", "hello")
		if err != nil {
			t.Fatalf("Save: %v", err)
		}

		var got string
		err = store.Get(ctx, "key1", &got)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("Struct", func(t *testing.T) {
		type Item struct {
			Name  string
			Value int
		}

		want := Item{Name: "test", Value: 42}
		if err := store.Save(ctx, "struct1", &want); err != nil {
			t.Fatalf("Save: %v", err)
		}

		var got Item
		if err := store.Get(ctx, "struct1", &got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("NonexistentKey", func(t *testing.T) {
		var data map[string]string
		err := store.Get(ctx, "nonexistent", &data)
		if err == nil {
			t.Error("expected error for nonexistent key")
		}
	})

	t.Run("Overwrite", func(t *testing.T) {
		if err := store.Save(ctx, "over", "first"); err != nil {
			t.Fatalf("Save first: %v", err)
		}
		if err := store.Save(ctx, "over", "second"); err != nil {
			t.Fatalf("Save second: %v", err)
		}

		var got string
		if err := store.Get(ctx, "over", &got); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got != "second" {
			t.Errorf("got %q, want %q", got, "second")
		}
	})
}

func TestEtcdStore_Delete(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	t.Run("ExistingKey", func(t *testing.T) {
		if err := store.Save(ctx, "delme", "value"); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if err := store.Delete(ctx, "delme"); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		var got string
		if err := store.Get(ctx, "delme", &got); err == nil {
			t.Error("expected error after delete")
		}
	})

	t.Run("NonexistentKey", func(t *testing.T) {
		// Delete of nonexistent key should not error (etcd behavior)
		if err := store.Delete(ctx, "nope"); err != nil {
			t.Fatalf("Delete nonexistent: %v", err)
		}
	})

	t.Run("EmptyKey", func(t *testing.T) {
		err := store.Delete(ctx, "")
		if err == nil {
			t.Error("expected error for empty key")
		}
	})
}

func TestEtcdStore_List(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	t.Run("ByPrefix", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			key := "items/" + string(rune('a'+i))
			if err := store.Save(ctx, key, i); err != nil {
				t.Fatalf("Save %s: %v", key, err)
			}
		}

		keys, err := store.List(ctx, "items/")
		if err != nil {
			t.Fatalf("List: %v", err)
		}

		if len(keys) != 3 {
			t.Fatalf("got %d keys, want 3: %v", len(keys), keys)
		}

		sort.Strings(keys)
		want := []string{"items/a", "items/b", "items/c"}
		for i, k := range keys {
			if k != want[i] {
				t.Errorf("key[%d] = %q, want %q", i, k, want[i])
			}
		}
	})

	t.Run("NonexistentPrefix", func(t *testing.T) {
		keys, err := store.List(ctx, "nope/")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})
}

func TestEtcdStore_SaveWithTTL(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	// Mock doesn't actually expire keys, but the code path exercises Grant + Put with lease
	err := store.SaveWithTTL(ctx, "ttl-key", "expires", 5*time.Second)
	if err != nil {
		t.Fatalf("SaveWithTTL: %v", err)
	}

	var got string
	if err := store.Get(ctx, "ttl-key", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "expires" {
		t.Errorf("got %q, want %q", got, "expires")
	}
}

func TestEtcdStore_EmptyKey(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	if err := store.Save(ctx, "", "value"); err == nil {
		t.Error("Save: expected error for empty key")
	}

	var v string
	if err := store.Get(ctx, "", &v); err == nil {
		t.Error("Get: expected error for empty key")
	}

	if err := store.Delete(ctx, ""); err == nil {
		t.Error("Delete: expected error for empty key")
	}

	if err := store.SaveWithTTL(ctx, "", "value", time.Second); err == nil {
		t.Error("SaveWithTTL: expected error for empty key")
	}
}

func TestEtcdStore_NilValue(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	if err := store.Save(ctx, "k", nil); err == nil {
		t.Error("Save: expected error for nil value")
	}

	if err := store.Get(ctx, "k", nil); err == nil {
		t.Error("Get: expected error for nil value")
	}

	if err := store.SaveWithTTL(ctx, "k", nil, time.Second); err == nil {
		t.Error("SaveWithTTL: expected error for nil value")
	}
}

func TestEtcdStore_MarshalError(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	// Channels cannot be marshaled to JSON
	ch := make(chan int)
	err := store.Save(ctx, "bad", ch)
	if err == nil {
		t.Error("expected marshal error")
	}

	err = store.SaveWithTTL(ctx, "bad", ch, time.Second)
	if err == nil {
		t.Error("expected marshal error for SaveWithTTL")
	}
}

func TestEtcdStore_UnmarshalError(t *testing.T) {
	// Inject invalid JSON directly into the mock
	mock := newMockEtcdKV()
	store := NewEtcdStoreWithClient(mock, "/banyan/test/")

	mock.mu.Lock()
	mock.data["/banyan/test/bad"] = "not-valid-json{"
	mock.mu.Unlock()

	var got map[string]string
	err := store.Get(context.Background(), "bad", &got)
	if err == nil {
		t.Error("expected unmarshal error")
	}
}

func TestEtcdStore_Close(t *testing.T) {
	t.Run("NonNilClient", func(t *testing.T) {
		store := newMockEtcdStore()
		if err := store.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	})

	t.Run("NilClient", func(t *testing.T) {
		store := &EtcdStore{client: nil, prefix: "/test/"}
		if err := store.Close(); err != nil {
			t.Fatalf("Close nil: %v", err)
		}
	})
}

func TestNewEtcdStoreWithClient_PrefixHandling(t *testing.T) {
	mock := newMockEtcdKV()

	t.Run("EmptyPrefix", func(t *testing.T) {
		s := NewEtcdStoreWithClient(mock, "")
		if s.prefix != "/banyan/vpc/" {
			t.Errorf("got prefix %q, want %q", s.prefix, "/banyan/vpc/")
		}
	})

	t.Run("NoTrailingSlash", func(t *testing.T) {
		s := NewEtcdStoreWithClient(mock, "/custom")
		if s.prefix != "/custom/" {
			t.Errorf("got prefix %q, want %q", s.prefix, "/custom/")
		}
	})

	t.Run("WithTrailingSlash", func(t *testing.T) {
		s := NewEtcdStoreWithClient(mock, "/custom/")
		if s.prefix != "/custom/" {
			t.Errorf("got prefix %q, want %q", s.prefix, "/custom/")
		}
	})
}

func TestEtcdStore_KeepAlive(t *testing.T) {
	store := newMockEtcdStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := store.KeepAlive(ctx, "alive", "heartbeat", 10*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("KeepAlive: %v", err)
	}

	// Verify the key was stored
	var got string
	if err := store.Get(ctx, "alive", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "heartbeat" {
		t.Errorf("got %q, want %q", got, "heartbeat")
	}

	// Let the keepalive goroutine tick at least once
	time.Sleep(200 * time.Millisecond)
	cancel()
}

func TestEtcdStore_Watch(t *testing.T) {
	store := newMockEtcdStore()
	ctx := context.Background()

	// Watch returns nil channel from mock - just verify no panic
	ch := store.Watch(ctx, "prefix/")
	if ch != nil {
		t.Error("expected nil watch channel from mock")
	}
}

// TestEtcdStoreNoEndpoints tests error when no endpoints provided
func TestEtcdStoreNoEndpoints(t *testing.T) {
	_, err := NewEtcdStore([]string{}, "/test/")
	if err == nil {
		t.Error("Expected error when creating store with no endpoints")
	}
}

// --- Real etcd tests (skipped in CI) ---

func TestEtcdStore_RealServer(t *testing.T) {
	store, err := NewEtcdStore([]string{"http://localhost:2379"}, "/banyan/test/")
	if err != nil {
		t.Skipf("Skipping: no etcd server available: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Clean up
	keys, _ := store.List(ctx, "")
	for _, key := range keys {
		store.Delete(ctx, key)
	}

	t.Run("SaveAndGet", func(t *testing.T) {
		type TestData struct {
			Name  string
			Value int
		}

		data := TestData{Name: "test", Value: 42}
		if err := store.Save(ctx, "test/key1", &data); err != nil {
			t.Fatalf("Save: %v", err)
		}

		var retrieved TestData
		if err := store.Get(ctx, "test/key1", &retrieved); err != nil {
			t.Fatalf("Get: %v", err)
		}

		if retrieved != data {
			t.Errorf("got %+v, want %+v", retrieved, data)
		}
	})
}
