package engine

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
)

func TestEtcdSubnetAllocator_Allocate(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}

	alloc, err := newEtcdSubnetAllocator("10.0.0.0/16", store, lockStore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	subnet, err := alloc.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if subnet.String() != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", subnet.String())
	}
}

func TestEtcdSubnetAllocator_Idempotent(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}
	alloc, _ := newEtcdSubnetAllocator("10.0.0.0/16", store, lockStore)
	ctx := context.Background()

	s1, _ := alloc.Allocate(ctx, "worker-1")
	s2, _ := alloc.Allocate(ctx, "worker-1")

	if s1.String() != s2.String() {
		t.Errorf("expected same subnet, got %s and %s", s1.String(), s2.String())
	}
}

func TestEtcdSubnetAllocator_MultipleAgents(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}
	alloc, _ := newEtcdSubnetAllocator("10.0.0.0/16", store, lockStore)
	ctx := context.Background()

	s1, _ := alloc.Allocate(ctx, "worker-1")
	s2, _ := alloc.Allocate(ctx, "worker-2")

	if s1.String() == s2.String() {
		t.Errorf("expected different subnets, both got %s", s1.String())
	}
	if s1.String() != "10.0.0.0/24" {
		t.Errorf("worker-1 expected 10.0.0.0/24, got %s", s1.String())
	}
	if s2.String() != "10.0.1.0/24" {
		t.Errorf("worker-2 expected 10.0.1.0/24, got %s", s2.String())
	}
}

func TestEtcdSubnetAllocator_Release(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}
	alloc, _ := newEtcdSubnetAllocator("10.0.0.0/16", store, lockStore)
	ctx := context.Background()

	s1, _ := alloc.Allocate(ctx, "worker-1")
	alloc.Release(ctx, "worker-1")

	s2, err := alloc.Allocate(ctx, "worker-2")
	if err != nil {
		t.Fatalf("Allocate after release failed: %v", err)
	}
	if s1.String() != s2.String() {
		t.Errorf("expected released subnet to be reused: got %s and %s", s1.String(), s2.String())
	}
}

func TestEtcdSubnetAllocator_GetAll(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}
	alloc, _ := newEtcdSubnetAllocator("10.0.0.0/16", store, lockStore)
	ctx := context.Background()

	alloc.Allocate(ctx, "worker-1")
	alloc.Allocate(ctx, "worker-2")

	all := alloc.GetAll(ctx)
	if len(all) != 2 {
		t.Fatalf("expected 2 allocations, got %d", len(all))
	}
	if _, ok := all["worker-1"]; !ok {
		t.Error("expected worker-1 in allocations")
	}
	if _, ok := all["worker-2"]; !ok {
		t.Error("expected worker-2 in allocations")
	}
}

func TestEtcdSubnetAllocator_Exhaustion(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}
	alloc, _ := newEtcdSubnetAllocator("10.0.0.0/23", store, lockStore)
	ctx := context.Background()

	alloc.Allocate(ctx, "worker-1")
	alloc.Allocate(ctx, "worker-2")
	_, err := alloc.Allocate(ctx, "worker-3")
	if err == nil {
		t.Fatal("expected error on exhaustion")
	}
}

func TestEtcdSubnetAllocator_InvalidCIDR(t *testing.T) {
	store := storage.NewMemoryStore()
	lockStore := &mockLockStore{StateStore: store}

	_, err := newEtcdSubnetAllocator("not-a-cidr", store, lockStore)
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}

	_, err = newEtcdSubnetAllocator("10.0.0.0/24", store, lockStore)
	if err == nil {
		t.Fatal("expected error for /24 CIDR")
	}
}

// mockLockStore provides a simple in-memory lock for testing.
type mockLockStore struct {
	storage.StateStore
}

func (m *mockLockStore) Lock(_ context.Context, _ string, _ time.Duration) (func(), error) {
	return func() {}, nil
}
