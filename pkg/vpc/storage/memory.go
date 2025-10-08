package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// MemoryStore implements StateStore with in-memory storage and optional file persistence
type MemoryStore struct {
	mu       sync.RWMutex
	data     map[string][]byte
	filePath string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string][]byte),
	}
}

// NewMemoryStoreWithFile creates a store that persists to a file
func NewMemoryStoreWithFile(filePath string) (*MemoryStore, error) {
	store := &MemoryStore{
		data:     make(map[string][]byte),
		filePath: filePath,
	}

	// Load existing data if file exists
	if _, err := os.Stat(filePath); err == nil {
		if err := store.load(); err != nil {
			return nil, fmt.Errorf("failed to load state: %w", err)
		}
	}

	return store, nil
}

func (m *MemoryStore) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &m.data)
}

func (m *MemoryStore) persist() error {
	if m.filePath == "" {
		return nil // No persistence configured
	}

	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(m.data)
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}

func (m *MemoryStore) Save(ctx context.Context, key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	m.data[key] = data

	// Persist to file if configured
	if err := m.persist(); err != nil {
		return fmt.Errorf("failed to persist: %w", err)
	}

	return nil
}

func (m *MemoryStore) Get(ctx context.Context, key string, value interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, ok := m.data[key]
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}

	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

func (m *MemoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)

	// Persist to file if configured
	if err := m.persist(); err != nil {
		return fmt.Errorf("failed to persist: %w", err)
	}

	return nil
}

func (m *MemoryStore) List(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}

	return keys, nil
}
