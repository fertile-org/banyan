package storage

import (
	"context"
	"errors"
	"time"
)

// ErrRevisionMismatch is returned when a CAS operation fails because
// the key was modified concurrently.
var ErrRevisionMismatch = errors.New("revision mismatch: key was modified concurrently")

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

// CASStore extends StateStore with compare-and-swap for multi-engine coordination.
type CASStore interface {
	StateStore

	// GetWithRevision returns the value and its etcd ModRevision.
	GetWithRevision(ctx context.Context, key string, value interface{}) (revision int64, err error)

	// SaveIfRevision performs a compare-and-swap: saves only if the key's
	// current ModRevision matches expectedRevision.
	// Returns ErrRevisionMismatch if the key was modified concurrently.
	SaveIfRevision(ctx context.Context, key string, value interface{}, expectedRevision int64) error
}

// LockStore extends StateStore with distributed locking.
type LockStore interface {
	StateStore

	// Lock acquires a distributed lock on the given key.
	// Returns an unlock function. The lock auto-expires after ttl.
	Lock(ctx context.Context, key string, ttl time.Duration) (unlock func(), err error)
}
