package storage

import "fmt"

// NewStore creates a StateStore for the given backend type.
// Supported backends: "badger", "redis", "etcd".
func NewStore(backend, address, prefix string) (StateStore, error) {
	switch backend {
	case "badger":
		return NewBadgerStore(address, prefix)
	case "redis":
		return NewRedisStore(address, prefix)
	case "etcd":
		return NewEtcdStore([]string{address}, prefix)
	default:
		return nil, fmt.Errorf("unsupported store backend: %q (supported: badger, redis, etcd)", backend)
	}
}
