package engine

import (
	"context"
	"net"
	"testing"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

func TestEtcdPeerTracker_Update(t *testing.T) {
	store := storage.NewMemoryStore()
	tracker := newEtcdPeerTracker(store)
	ctx := context.Background()

	peer := overlay.Peer{
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

func TestEtcdPeerTracker_Remove(t *testing.T) {
	store := storage.NewMemoryStore()
	tracker := newEtcdPeerTracker(store)
	ctx := context.Background()

	tracker.Update(ctx, "worker-1", overlay.Peer{HostIP: net.ParseIP("192.168.1.10")})
	tracker.Update(ctx, "worker-2", overlay.Peer{HostIP: net.ParseIP("192.168.1.20")})
	tracker.Remove(ctx, "worker-1")

	peers := tracker.GetPeersExcluding(ctx, "other")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer after remove, got %d", len(peers))
	}
	if peers[0].HostIP.String() != "192.168.1.20" {
		t.Errorf("expected worker-2 IP, got %s", peers[0].HostIP.String())
	}
}

func TestEtcdPeerTracker_GetPeersExcluding(t *testing.T) {
	store := storage.NewMemoryStore()
	tracker := newEtcdPeerTracker(store)
	ctx := context.Background()

	tracker.Update(ctx, "worker-1", overlay.Peer{HostIP: net.ParseIP("192.168.1.10")})
	tracker.Update(ctx, "worker-2", overlay.Peer{HostIP: net.ParseIP("192.168.1.20")})
	tracker.Update(ctx, "worker-3", overlay.Peer{HostIP: net.ParseIP("192.168.1.30")})

	peers := tracker.GetPeersExcluding(ctx, "worker-2")
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers (excluding worker-2), got %d", len(peers))
	}
	for _, p := range peers {
		if p.HostIP.String() == "192.168.1.20" {
			t.Error("expected worker-2 to be excluded")
		}
	}
}

func TestEtcdPeerTracker_Empty(t *testing.T) {
	store := storage.NewMemoryStore()
	tracker := newEtcdPeerTracker(store)

	peers := tracker.GetPeersExcluding(context.Background(), "anyone")
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestEtcdPeerTracker_Overwrite(t *testing.T) {
	store := storage.NewMemoryStore()
	tracker := newEtcdPeerTracker(store)
	ctx := context.Background()

	tracker.Update(ctx, "worker-1", overlay.Peer{HostIP: net.ParseIP("192.168.1.10")})
	tracker.Update(ctx, "worker-1", overlay.Peer{HostIP: net.ParseIP("192.168.1.20")})

	peers := tracker.GetPeersExcluding(ctx, "other")
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].HostIP.String() != "192.168.1.20" {
		t.Errorf("expected updated IP 192.168.1.20, got %s", peers[0].HostIP.String())
	}
}
