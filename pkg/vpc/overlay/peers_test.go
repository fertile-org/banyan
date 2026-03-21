package overlay

import (
	"context"
	"net"
	"testing"
)

func TestPeerTracker_Update(t *testing.T) {
	tracker := NewPeerTracker()
	ctx := context.Background()

	peer := Peer{
		Subnet: net.IPNet{IP: net.ParseIP("10.0.1.0"), Mask: net.CIDRMask(24, 32)},
		HostIP: net.ParseIP("192.168.1.10"),
	}
	tracker.Update(ctx, "worker-1", peer)

	peers := tracker.GetPeersExcluding(ctx, "other")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].HostIP.String() != "192.168.1.10" {
		t.Errorf("expected 192.168.1.10, got %s", peers[0].HostIP.String())
	}
}

func TestPeerTracker_UpdateOverwrite(t *testing.T) {
	tracker := NewPeerTracker()
	ctx := context.Background()

	peer1 := Peer{HostIP: net.ParseIP("192.168.1.10")}
	peer2 := Peer{HostIP: net.ParseIP("192.168.1.20")}

	tracker.Update(ctx, "worker-1", peer1)
	tracker.Update(ctx, "worker-1", peer2) // overwrite

	peers := tracker.GetPeersExcluding(ctx, "other")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer after overwrite, got %d", len(peers))
	}
	if peers[0].HostIP.String() != "192.168.1.20" {
		t.Errorf("expected updated IP 192.168.1.20, got %s", peers[0].HostIP.String())
	}
}

func TestPeerTracker_Remove(t *testing.T) {
	tracker := NewPeerTracker()
	ctx := context.Background()

	tracker.Update(ctx, "worker-1", Peer{HostIP: net.ParseIP("192.168.1.10")})
	tracker.Update(ctx, "worker-2", Peer{HostIP: net.ParseIP("192.168.1.20")})
	tracker.Remove(ctx, "worker-1")

	peers := tracker.GetPeersExcluding(ctx, "other")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer after remove, got %d", len(peers))
	}
	if peers[0].HostIP.String() != "192.168.1.20" {
		t.Errorf("expected worker-2 IP, got %s", peers[0].HostIP.String())
	}
}

func TestPeerTracker_RemoveNonexistent(t *testing.T) {
	tracker := NewPeerTracker()
	// Should not panic
	tracker.Remove(context.Background(), "nonexistent")
}

func TestPeerTracker_GetPeersExcluding(t *testing.T) {
	tracker := NewPeerTracker()
	ctx := context.Background()

	tracker.Update(ctx, "worker-1", Peer{HostIP: net.ParseIP("192.168.1.10")})
	tracker.Update(ctx, "worker-2", Peer{HostIP: net.ParseIP("192.168.1.20")})
	tracker.Update(ctx, "worker-3", Peer{HostIP: net.ParseIP("192.168.1.30")})

	peers := tracker.GetPeersExcluding(ctx, "worker-2")
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers (excluding worker-2), got %d", len(peers))
	}

	// Check that worker-2 is excluded
	for _, p := range peers {
		if p.HostIP.String() == "192.168.1.20" {
			t.Error("expected worker-2 to be excluded")
		}
	}
}

func TestPeerTracker_Empty(t *testing.T) {
	tracker := NewPeerTracker()
	peers := tracker.GetPeersExcluding(context.Background(), "anyone")
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestPeerTracker_GetPeersExcluding_NoMatch(t *testing.T) {
	tracker := NewPeerTracker()
	ctx := context.Background()
	tracker.Update(ctx, "worker-1", Peer{HostIP: net.ParseIP("192.168.1.10")})

	// Excluding a non-existent agent returns all peers
	peers := tracker.GetPeersExcluding(ctx, "nonexistent")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
}
