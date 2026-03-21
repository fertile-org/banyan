package engine

import (
	"context"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// etcdPeerTracker tracks VPC overlay peers in etcd for multi-engine coordination.
// Single-engine mode uses the in-memory overlay.PeerTracker instead.
type etcdPeerTracker struct {
	store storage.StateStore
}

const peerKeyPrefix = "vpc/peers/"

// newEtcdPeerTracker creates an etcd-backed peer tracker.
func newEtcdPeerTracker(store storage.StateStore) *etcdPeerTracker {
	return &etcdPeerTracker{store: store}
}

// Update adds or updates a peer in etcd.
func (t *etcdPeerTracker) Update(ctx context.Context, agentName string, peer overlay.Peer) { //nolint:gocritic // matches PeerTrackerInterface
	_ = t.store.Save(ctx, peerKeyPrefix+agentName, &peer)
}

// Remove deletes a peer from etcd.
func (t *etcdPeerTracker) Remove(ctx context.Context, agentName string) {
	_ = t.store.Delete(ctx, peerKeyPrefix+agentName)
}

// GetPeersExcluding returns all peers except the one with the given agent name.
func (t *etcdPeerTracker) GetPeersExcluding(ctx context.Context, agentName string) []overlay.Peer {
	keys, err := t.store.List(ctx, peerKeyPrefix)
	if err != nil {
		return nil
	}

	var peers []overlay.Peer
	for _, key := range keys {
		name := key[len(peerKeyPrefix):]
		if name == agentName {
			continue
		}
		var peer overlay.Peer
		if err := t.store.Get(ctx, key, &peer); err != nil {
			continue
		}
		peers = append(peers, peer)
	}
	return peers
}
