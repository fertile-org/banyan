package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryStore_SaveAndGet(t *testing.T) {
	store := NewMemoryStore()
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

	t.Run("overwrite existing key", func(t *testing.T) {
		if err := store.Save(ctx, "overwrite", "first"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		if err := store.Save(ctx, "overwrite", "second"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		var got string
		if err := store.Get(ctx, "overwrite", &got); err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got != "second" {
			t.Errorf("expected 'second', got %q", got)
		}
	})
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	t.Run("delete existing key", func(t *testing.T) {
		store.Save(ctx, "del-key", "value")
		if err := store.Delete(ctx, "del-key"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		var got string
		if err := store.Get(ctx, "del-key", &got); err == nil {
			t.Fatal("expected error after delete")
		}
	})

	t.Run("delete nonexistent key does not error", func(t *testing.T) {
		if err := store.Delete(ctx, "nonexistent"); err != nil {
			t.Fatalf("Delete of nonexistent key should not error: %v", err)
		}
	})
}

func TestMemoryStore_List(t *testing.T) {
	store := NewMemoryStore()
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

func TestMemoryStore_Close(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestMemoryStore_FilePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")
	ctx := context.Background()

	t.Run("save persists to file", func(t *testing.T) {
		store, err := NewMemoryStoreWithFile(filePath)
		if err != nil {
			t.Fatalf("NewMemoryStoreWithFile failed: %v", err)
		}

		if err := store.Save(ctx, "persist-key", "persist-value"); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Fatal("expected file to exist after Save")
		}
	})

	t.Run("reload from file", func(t *testing.T) {
		store, err := NewMemoryStoreWithFile(filePath)
		if err != nil {
			t.Fatalf("NewMemoryStoreWithFile failed: %v", err)
		}

		var got string
		if err := store.Get(ctx, "persist-key", &got); err != nil {
			t.Fatalf("Get failed after reload: %v", err)
		}
		if got != "persist-value" {
			t.Errorf("expected 'persist-value', got %q", got)
		}
	})

	t.Run("delete persists to file", func(t *testing.T) {
		store, err := NewMemoryStoreWithFile(filePath)
		if err != nil {
			t.Fatalf("NewMemoryStoreWithFile failed: %v", err)
		}

		if err := store.Delete(ctx, "persist-key"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		// Reload and verify key is gone
		store2, err := NewMemoryStoreWithFile(filePath)
		if err != nil {
			t.Fatalf("NewMemoryStoreWithFile failed: %v", err)
		}

		var got string
		if err := store2.Get(ctx, "persist-key", &got); err == nil {
			t.Fatal("expected error after delete and reload")
		}
	})
}

func TestNewStore_Factory(t *testing.T) {
	t.Run("unsupported backend", func(t *testing.T) {
		_, err := NewStore("unsupported", "addr", "/banyan")
		if err == nil {
			t.Fatal("expected error for unsupported backend")
		}
	})

	t.Run("badger backend rejected", func(t *testing.T) {
		_, err := NewStore("badger", t.TempDir(), "/banyan")
		if err == nil {
			t.Fatal("expected error for badger backend (no longer supported)")
		}
	})
}

func TestMemoryStore_PersistError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test persist error as root")
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")
	ctx := context.Background()

	store, err := NewMemoryStoreWithFile(filePath)
	if err != nil {
		t.Fatalf("NewMemoryStoreWithFile failed: %v", err)
	}

	// Save initial data
	if err := store.Save(ctx, "key1", "value1"); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Make directory read-only so persist fails on subsequent writes
	os.Chmod(tmpDir, 0o555)
	t.Cleanup(func() { os.Chmod(tmpDir, 0o755) })

	// Remove the existing file so persist has to write a new one
	// Actually the file already exists, but let's try making it unwritable
	os.Chmod(filePath, 0o444)
	t.Cleanup(func() { os.Chmod(filePath, 0o644) })

	err = store.Save(ctx, "key2", "value2")
	if err == nil {
		t.Fatal("expected error when persist fails")
	}

	// Also test Delete persist failure
	err = store.Delete(ctx, "key1")
	if err == nil {
		t.Fatal("expected error when persist fails on delete")
	}
}

func TestMemoryStore_GetUnmarshalError(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Directly inject invalid JSON bytes into the store's data map
	store.mu.Lock()
	store.data["bad-key"] = []byte("not valid json {{{")
	store.mu.Unlock()

	var got string
	err := store.Get(ctx, "bad-key", &got)
	if err == nil {
		t.Fatal("expected unmarshal error for corrupt data")
	}
}

func TestMemoryStore_SaveMarshalError(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// A channel cannot be JSON-marshalled
	err := store.Save(ctx, "bad-key", make(chan int))
	if err == nil {
		t.Fatal("expected marshal error for unsupported type")
	}
}

func TestMemoryStore_LoadReadError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test read error as root")
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "state.json")

	// Create valid file
	os.WriteFile(filePath, []byte(`{"k":"v"}`), 0644)

	// Make it unreadable
	os.Chmod(filePath, 0o000)
	t.Cleanup(func() { os.Chmod(filePath, 0o644) })

	_, err := NewMemoryStoreWithFile(filePath)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}
}

func TestMemoryStore_PersistMkdirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test mkdir error as root")
	}

	// Use a path under /dev/null which cannot be a directory
	store := &MemoryStore{
		data:     map[string][]byte{"k": []byte(`"v"`)},
		filePath: "/dev/null/subdir/state.json",
	}

	err := store.Save(context.Background(), "key", "value")
	if err == nil {
		t.Fatal("expected error when MkdirAll fails")
	}
}

func TestNewMemoryStoreWithFile(t *testing.T) {
	t.Run("nonexistent file creates empty store", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "new.json")

		store, err := NewMemoryStoreWithFile(filePath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		keys, _ := store.List(context.Background(), "")
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "bad.json")

		os.WriteFile(filePath, []byte("not json"), 0644)

		_, err := NewMemoryStoreWithFile(filePath)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}
