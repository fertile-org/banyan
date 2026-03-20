package overlay

import (
	"context"
	"sync"
)

// PeerTrackerInterface is the interface for peer tracking.
// Both in-memory (PeerTracker) and etcd-backed implementations satisfy this.
type PeerTrackerInterface interface {
	Update(ctx context.Context, agentName string, peer Peer)
	Remove(ctx context.Context, agentName string)
	GetPeersExcluding(ctx context.Context, agentName string) []Peer
}

// PeerTracker maintains the set of active agents and their network info.
type PeerTracker struct {
	peers map[string]Peer // agentName → Peer
	mu    sync.RWMutex
}

// NewPeerTracker creates a new PeerTracker.
func NewPeerTracker() *PeerTracker {
	return &PeerTracker{
		peers: make(map[string]Peer),
	}
}

// Update adds or updates a peer in the tracker.
func (t *PeerTracker) Update(_ context.Context, agentName string, peer Peer) { //nolint:gocritic // matches PeerTrackerInterface

	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[agentName] = peer
}

// Remove deletes a peer from the tracker.
func (t *PeerTracker) Remove(_ context.Context, agentName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, agentName)
}

// GetPeersExcluding returns all peers except the one with the given agent name.
func (t *PeerTracker) GetPeersExcluding(_ context.Context, agentName string) []Peer {
	t.mu.RLock()
	defer t.mu.RUnlock()

	peers := make([]Peer, 0, len(t.peers))
	for name, peer := range t.peers {
		if name != agentName {
			peers = append(peers, peer)
		}
	}
	return peers
}
