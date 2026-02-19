package storage

import (
	"testing"
)

func TestNewStore_UnsupportedBackend(t *testing.T) {
	_, err := NewStore("mongodb", "localhost:27017", "/banyan")
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

func TestNewStore_EtcdBackend(t *testing.T) {
	// Without a running etcd server, this should fail to connect
	_, err := NewStore("etcd", "localhost:19998", "/banyan")
	if err == nil {
		t.Fatal("expected error connecting to non-existent etcd")
	}
}
