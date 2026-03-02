// Package overlay provides the abstract overlay networking interface and implementations
// for cross-host container communication in Banyan.
package overlay

import (
	"context"
	"fmt"
	"net"
)

// Peer represents a remote agent in the overlay network.
type Peer struct {
	Subnet    net.IPNet // allocated /24 subnet for this agent
	HostIP    net.IP    // agent's public/reachable IP
	VTEPIP    net.IP    // VTEP tunnel endpoint IP (first IP in subnet)
	PublicKey string    // WireGuard public key (base64)
}

// OverlayDriver manages the data-plane for overlay networking on an agent.
type OverlayDriver interface {
	// Init creates the overlay interface and bridge for the allocated subnet.
	Init(ctx context.Context, subnet net.IPNet, hostIP net.IP) error
	// ReconcilePeers updates routing/forwarding tables for cross-host communication.
	ReconcilePeers(ctx context.Context, peers []Peer) error
	// WriteCNIConfig writes the CNI configuration for nerdctl to use.
	WriteCNIConfig(subnet net.IPNet) error
	// Cleanup tears down overlay interfaces on shutdown.
	Cleanup(ctx context.Context) error
}

// DeterministicMAC generates a deterministic MAC address from a subnet.
// Format: 02:42:0a:00:<third-octet>:01 (locally administered, unicast).
// Used for the bridge MAC so incoming decapsulated frames are recognized.
func DeterministicMAC(subnet net.IPNet) net.HardwareAddr {
	ip := subnet.IP.To4()
	if ip == nil {
		// Fallback for non-IPv4
		return net.HardwareAddr{0x02, 0x42, 0x0a, 0x00, 0x00, 0x01}
	}
	return net.HardwareAddr{0x02, 0x42, ip[0], ip[1], ip[2], 0x01}
}

// VTEPIP returns the first usable IP in a subnet (gateway IP).
// For a /24 subnet like 10.0.45.0/24, this returns 10.0.45.1.
func VTEPIP(subnet net.IPNet) net.IP {
	ip := make(net.IP, len(subnet.IP))
	copy(ip, subnet.IP)
	ip4 := ip.To4()
	if ip4 == nil {
		return ip
	}
	ip4[3] = 1
	return ip4
}

// PeerFromProto constructs a Peer from proto-level strings (subnet, host IP, and WireGuard public key).
func PeerFromProto(subnetStr, hostIPStr, publicKey string) (Peer, error) {
	_, subnet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		return Peer{}, fmt.Errorf("invalid subnet %q: %w", subnetStr, err)
	}

	hostIP := net.ParseIP(hostIPStr)
	if hostIP == nil {
		return Peer{}, fmt.Errorf("invalid host IP %q", hostIPStr)
	}

	if publicKey == "" {
		return Peer{}, fmt.Errorf("empty public key")
	}

	return Peer{
		Subnet:    *subnet,
		HostIP:    hostIP,
		VTEPIP:    VTEPIP(*subnet),
		PublicKey: publicKey,
	}, nil
}
