package overlay

import (
	"sync"
)

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
func (t *PeerTracker) Update(agentName string, peer Peer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.peers[agentName] = peer
}

// Remove deletes a peer from the tracker.
func (t *PeerTracker) Remove(agentName string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.peers, agentName)
}

// GetPeersExcluding returns all peers except the one with the given agent name.
func (t *PeerTracker) GetPeersExcluding(agentName string) []Peer {
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
