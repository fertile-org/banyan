package storage

import "fmt"

// NewStoreWithOptions creates an etcd StateStore with full connection options.
func NewStoreWithOptions(opts *EtcdOptions) (StateStore, error) {
	return NewEtcdStore(opts)
}

// NewStore creates a StateStore for the given backend type.
// Supported backends: "etcd".
func NewStore(backend, address, prefix string) (StateStore, error) {
	if backend != "etcd" {
		return nil, fmt.Errorf("unsupported store backend: %q (only etcd is supported)", backend)
	}
	return NewStoreWithOptions(&EtcdOptions{
		Endpoints: []string{address},
		Prefix:    prefix,
	})
}
