package overlay

import (
	"context"
	"testing"
)

func TestNewSubnetAllocator(t *testing.T) {
	t.Run("valid CIDR", func(t *testing.T) {
		a, err := NewSubnetAllocator("10.0.0.0/16")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if a == nil {
			t.Fatal("expected non-nil allocator")
		}
	})

	t.Run("invalid CIDR", func(t *testing.T) {
		_, err := NewSubnetAllocator("not-a-cidr")
		if err == nil {
			t.Fatal("expected error for invalid CIDR")
		}
	})

	t.Run("CIDR too small", func(t *testing.T) {
		_, err := NewSubnetAllocator("10.0.0.0/24")
		if err == nil {
			t.Fatal("expected error for /24 CIDR")
		}
	})

	t.Run("CIDR /25 too small", func(t *testing.T) {
		_, err := NewSubnetAllocator("10.0.0.0/25")
		if err == nil {
			t.Fatal("expected error for /25 CIDR")
		}
	})
}

func TestAllocate_FirstAgent(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	subnet, err := a.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	if subnet.String() != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", subnet.String())
	}
}

func TestAllocate_MultipleAgents(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	s1, err := a.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Allocate worker-1 failed: %v", err)
	}

	s2, err := a.Allocate(ctx, "worker-2")
	if err != nil {
		t.Fatalf("Allocate worker-2 failed: %v", err)
	}

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

func TestAllocate_Idempotent(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	s1, err := a.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("first Allocate failed: %v", err)
	}

	s2, err := a.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("second Allocate failed: %v", err)
	}

	if s1.String() != s2.String() {
		t.Errorf("expected same subnet, got %s and %s", s1.String(), s2.String())
	}
}

func TestAllocate_Release(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	s1, _ := a.Allocate(ctx, "worker-1")
	a.Release(ctx, "worker-1")

	// The released subnet should be available again
	s2, err := a.Allocate(ctx, "worker-2")
	if err != nil {
		t.Fatalf("Allocate after release failed: %v", err)
	}

	if s1.String() != s2.String() {
		t.Errorf("expected released subnet to be reused: got %s and %s", s1.String(), s2.String())
	}
}

func TestAllocate_ReleaseNonexistent(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic
	a.Release(context.Background(), "nonexistent")
}

func TestAllocate_Exhaustion(t *testing.T) {
	// /23 has only 2 /24 blocks
	a, err := NewSubnetAllocator("10.0.0.0/23")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	_, err = a.Allocate(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Allocate worker-1 failed: %v", err)
	}

	_, err = a.Allocate(ctx, "worker-2")
	if err != nil {
		t.Fatalf("Allocate worker-2 failed: %v", err)
	}

	_, err = a.Allocate(ctx, "worker-3")
	if err == nil {
		t.Fatal("expected error on exhaustion")
	}
}

func TestGetAll(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	a.Allocate(ctx, "worker-1")
	a.Allocate(ctx, "worker-2")

	all := a.GetAll(ctx)
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

func TestGetAll_Empty(t *testing.T) {
	a, err := NewSubnetAllocator("10.0.0.0/16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	all := a.GetAll(context.Background())
	if len(all) != 0 {
		t.Errorf("expected 0 allocations, got %d", len(all))
	}
}
