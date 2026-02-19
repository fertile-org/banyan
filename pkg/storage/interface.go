package storage

import "context"

// StateStore provides persistence for VPC state
type StateStore interface {
	// Save stores a value by key
	Save(ctx context.Context, key string, value interface{}) error

	// Get retrieves a value by key
	Get(ctx context.Context, key string, value interface{}) error

	// Delete removes a value by key
	Delete(ctx context.Context, key string) error

	// List returns all keys with a given prefix
	List(ctx context.Context, prefix string) ([]string, error)

	// Close releases any resources held by the store
	Close() error
}
