package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dgraph-io/badger/v4"
)

// BadgerStore implements StateStore using BadgerDB as an embedded key-value store.
type BadgerStore struct {
	db     *badger.DB
	prefix string
}

// NewBadgerStore creates a new BadgerStore that persists data to the given directory.
func NewBadgerStore(dataDir, prefix string) (*BadgerStore, error) {
	if dataDir == "" {
		dataDir = "/var/lib/banyan/store"
	}

	if prefix == "" {
		prefix = "/banyan/"
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create badger data directory: %w", err)
	}

	opts := badger.DefaultOptions(dataDir)
	opts.Logger = nil // suppress verbose badger logs

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open badger database at %s: %w", dataDir, err)
	}

	return &BadgerStore{
		db:     db,
		prefix: prefix,
	}, nil
}

// NewBadgerStoreWithDB creates a BadgerStore with an existing DB instance (useful for testing).
func NewBadgerStoreWithDB(db *badger.DB, prefix string) *BadgerStore {
	if prefix == "" {
		prefix = "/banyan/"
	}

	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &BadgerStore{
		db:     db,
		prefix: prefix,
	}
}

func (s *BadgerStore) Save(_ context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	fullKey := s.prefix + key
	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte(fullKey), data); err != nil {
			return fmt.Errorf("failed to save to badger: %w", err)
		}
		return nil
	})
}

func (s *BadgerStore) Get(_ context.Context, key string, value interface{}) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	if value == nil {
		return fmt.Errorf("value cannot be nil")
	}

	fullKey := s.prefix + key
	return s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fullKey))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return fmt.Errorf("key not found: %s", key)
			}
			return fmt.Errorf("failed to get from badger: %w", err)
		}

		data, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("failed to read value from badger: %w", err)
		}

		if err := json.Unmarshal(data, value); err != nil {
			return fmt.Errorf("failed to unmarshal value: %w", err)
		}

		return nil
	})
}

func (s *BadgerStore) Delete(_ context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	fullKey := s.prefix + key
	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete([]byte(fullKey)); err != nil {
			return fmt.Errorf("failed to delete from badger: %w", err)
		}
		return nil
	})
}

func (s *BadgerStore) List(_ context.Context, prefix string) ([]string, error) {
	fullPrefix := s.prefix + prefix

	var keys []string
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte(fullPrefix)
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte(fullPrefix)); it.Valid(); it.Next() {
			item := it.Item()
			key := string(item.Key())
			keys = append(keys, strings.TrimPrefix(key, s.prefix))
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}

	return keys, nil
}

func (s *BadgerStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
