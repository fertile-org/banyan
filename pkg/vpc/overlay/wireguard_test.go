package overlay

import (
	"context"
	"net"
	"testing"
)

func TestWireGuardDriver_Init(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	driver := NewWireGuardDriverWithOps(linkOps, wgOps, "privkey-base64", "pubkey-base64")

	subnet := net.IPNet{
		IP:   net.ParseIP("10.0.45.0"),
		Mask: net.CIDRMask(24, 32),
	}
	hostIP := net.ParseIP("192.168.1.10")

	err := driver.Init(context.Background(), subnet, hostIP)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify WireGuard operations
	if wgOps.callCount("CreateInterface") != 1 {
		t.Errorf("expected 1 CreateInterface call, got %d", wgOps.callCount("CreateInterface"))
	}
	if wgOps.calls[0].args[0] != "banyan-wg" {
		t.Errorf("expected WG interface 'banyan-wg', got %q", wgOps.calls[0].args[0])
	}

	if wgOps.callCount("ConfigureInterface") != 1 {
		t.Errorf("expected 1 ConfigureInterface call, got %d", wgOps.callCount("ConfigureInterface"))
	}

	// Verify link operations sequence (includes cleanup of leftover interfaces)
	expectedLinkMethods := []string{
		"LinkExists",     // 0. Check WG exists (cleanup)
		"DeleteLink",     // 1. Delete old WG (mock returns exists=true)
		"LinkExists",     // 2. Check bridge exists (cleanup)
		"DeleteLink",     // 3. Delete old bridge
		"CreateBridge",   // 4. Create bridge
		"SetLinkAddress", // 5. Set bridge MAC
		"AddAddress",     // 6. Assign VTEP IP
		"SetLinkUp",      // 7. Bring up WireGuard
		"SetLinkUp",      // 8. Bring up bridge
	}

	if len(linkOps.calls) != len(expectedLinkMethods) {
		t.Fatalf("expected %d link calls, got %d: %v", len(expectedLinkMethods), len(linkOps.calls), linkOps.calls)
	}

	for i, expected := range expectedLinkMethods {
		if linkOps.calls[i].method != expected {
			t.Errorf("link call %d: expected %s, got %s", i, expected, linkOps.calls[i].method)
		}
	}

	// Verify bridge name (after cleanup calls)
	if linkOps.calls[4].args[0] != "banyan0" {
		t.Errorf("expected bridge name 'banyan0', got %q", linkOps.calls[4].args[0])
	}

	// Verify bridge MAC was set to deterministic VTEP MAC
	if linkOps.calls[5].args[1] != "02:42:0a:00:2d:01" {
		t.Errorf("expected bridge MAC '02:42:0a:00:2d:01', got %q", linkOps.calls[5].args[1])
	}

	// Verify VTEP IP was assigned correctly
	if linkOps.calls[6].args[1] != "10.0.45.1/24" {
		t.Errorf("expected VTEP IP '10.0.45.1/24', got %q", linkOps.calls[6].args[1])
	}
}

func TestWireGuardDriver_ReconcilePeers(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	driver := NewWireGuardDriverWithOps(linkOps, wgOps, "privkey", "pubkey")

	_, peerSubnet, _ := net.ParseCIDR("10.0.46.0/24")
	peer := Peer{
		Subnet:    *peerSubnet,
		HostIP:    net.ParseIP("192.168.1.20"),
		VTEPIP:    net.ParseIP("10.0.46.1"),
		PublicKey: "peer-pubkey-base64",
	}

	err := driver.ReconcilePeers(context.Background(), []Peer{peer})
	if err != nil {
		t.Fatalf("ReconcilePeers failed: %v", err)
	}

	// Verify WireGuard peer was added
	if wgOps.callCount("AddPeer") != 1 {
		t.Fatalf("expected 1 AddPeer call, got %d", wgOps.callCount("AddPeer"))
	}
	if wgOps.calls[0].args[0] != "banyan-wg" {
		t.Errorf("expected iface 'banyan-wg', got %q", wgOps.calls[0].args[0])
	}
	if wgOps.calls[0].args[1] != "peer-pubkey-base64" {
		t.Errorf("expected pubkey 'peer-pubkey-base64', got %q", wgOps.calls[0].args[1])
	}
	if wgOps.calls[0].args[2] != "192.168.1.20:51820" {
		t.Errorf("expected endpoint '192.168.1.20:51820', got %q", wgOps.calls[0].args[2])
	}

	// Verify route was added
	if linkOps.callCount("AddRoute") != 1 {
		t.Fatalf("expected 1 AddRoute call, got %d", linkOps.callCount("AddRoute"))
	}
	// Route should be via WireGuard interface
	if linkOps.calls[0].args[2] != "banyan-wg" {
		t.Errorf("expected route via 'banyan-wg', got %q", linkOps.calls[0].args[2])
	}

	// No FDB or ARP entries (WireGuard is L3, unlike VXLAN)
	if linkOps.hasCall("AddFDBEntry") {
		t.Error("WireGuard should not add FDB entries")
	}
	if linkOps.hasCall("AddARPEntry") {
		t.Error("WireGuard should not add ARP entries")
	}
}

func TestWireGuardDriver_ReconcilePeers_SkipsEmptyPublicKey(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	driver := NewWireGuardDriverWithOps(linkOps, wgOps, "privkey", "pubkey")

	_, peerSubnet, _ := net.ParseCIDR("10.0.46.0/24")
	peer := Peer{
		Subnet:    *peerSubnet,
		HostIP:    net.ParseIP("192.168.1.20"),
		VTEPIP:    net.ParseIP("10.0.46.1"),
		PublicKey: "", // empty — should be skipped
	}

	err := driver.ReconcilePeers(context.Background(), []Peer{peer})
	if err != nil {
		t.Fatalf("ReconcilePeers failed: %v", err)
	}

	if wgOps.callCount("AddPeer") != 0 {
		t.Errorf("expected 0 AddPeer calls for empty public key, got %d", wgOps.callCount("AddPeer"))
	}
}

func TestWireGuardDriver_ReconcilePeers_MultiplePeers(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	driver := NewWireGuardDriverWithOps(linkOps, wgOps, "privkey", "pubkey")

	_, subnet1, _ := net.ParseCIDR("10.0.46.0/24")
	_, subnet2, _ := net.ParseCIDR("10.0.47.0/24")

	peers := []Peer{
		{Subnet: *subnet1, HostIP: net.ParseIP("192.168.1.20"), VTEPIP: net.ParseIP("10.0.46.1"), PublicKey: "key1"},
		{Subnet: *subnet2, HostIP: net.ParseIP("192.168.1.30"), VTEPIP: net.ParseIP("10.0.47.1"), PublicKey: "key2"},
	}

	err := driver.ReconcilePeers(context.Background(), peers)
	if err != nil {
		t.Fatalf("ReconcilePeers failed: %v", err)
	}

	if wgOps.callCount("AddPeer") != 2 {
		t.Errorf("expected 2 AddPeer calls, got %d", wgOps.callCount("AddPeer"))
	}
	if linkOps.callCount("AddRoute") != 2 {
		t.Errorf("expected 2 AddRoute calls, got %d", linkOps.callCount("AddRoute"))
	}
}

func TestWireGuardDriver_Cleanup(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	driver := NewWireGuardDriverWithOps(linkOps, wgOps, "privkey", "pubkey")

	err := driver.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if linkOps.callCount("LinkExists") != 2 {
		t.Errorf("expected 2 LinkExists calls, got %d", linkOps.callCount("LinkExists"))
	}
	if linkOps.callCount("DeleteLink") != 2 {
		t.Errorf("expected 2 DeleteLink calls, got %d", linkOps.callCount("DeleteLink"))
	}
}

func TestNewWireGuardDriver(t *testing.T) {
	driver := NewWireGuardDriver("privkey", "pubkey")
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
	if driver.bridgeName != defaultBridge {
		t.Errorf("expected bridge %q, got %q", defaultBridge, driver.bridgeName)
	}
	if driver.wgName != defaultWGName {
		t.Errorf("expected WG name %q, got %q", defaultWGName, driver.wgName)
	}
	if driver.privateKey != "privkey" {
		t.Errorf("expected private key 'privkey', got %q", driver.privateKey)
	}
	if driver.publicKey != "pubkey" {
		t.Errorf("expected public key 'pubkey', got %q", driver.publicKey)
	}
}

func TestPeerFromSubnetAndHostWG(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		peer, err := PeerFromSubnetAndHostWG("10.0.45.0/24", "192.168.1.10", "pubkey-base64")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if peer.Subnet.String() != "10.0.45.0/24" {
			t.Errorf("expected subnet 10.0.45.0/24, got %s", peer.Subnet.String())
		}
		if peer.HostIP.String() != "192.168.1.10" {
			t.Errorf("expected host IP 192.168.1.10, got %s", peer.HostIP.String())
		}
		if peer.PublicKey != "pubkey-base64" {
			t.Errorf("expected public key 'pubkey-base64', got %q", peer.PublicKey)
		}
		if peer.VTEPIP.String() != "10.0.45.1" {
			t.Errorf("expected VTEP IP 10.0.45.1, got %s", peer.VTEPIP.String())
		}
	})

	t.Run("invalid subnet", func(t *testing.T) {
		_, err := PeerFromSubnetAndHostWG("invalid", "192.168.1.10", "key")
		if err == nil {
			t.Fatal("expected error for invalid subnet")
		}
	})

	t.Run("invalid host IP", func(t *testing.T) {
		_, err := PeerFromSubnetAndHostWG("10.0.45.0/24", "invalid", "key")
		if err == nil {
			t.Fatal("expected error for invalid host IP")
		}
	})

	t.Run("empty public key", func(t *testing.T) {
		_, err := PeerFromSubnetAndHostWG("10.0.45.0/24", "192.168.1.10", "")
		if err == nil {
			t.Fatal("expected error for empty public key")
		}
	})
}

// Verify WireGuardDriver satisfies OverlayDriver interface
var _ OverlayDriver = (*WireGuardDriver)(nil)
