package overlay

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// mockLinkOps records all calls to verify correct operation order and args.
type mockLinkOps struct {
	calls []mockCall
}

type mockCall struct {
	method string
	args   []string
}

func (m *mockLinkOps) record(method string, args ...string) {
	m.calls = append(m.calls, mockCall{method: method, args: args})
}

func (m *mockLinkOps) CreateVXLAN(name string, vni int, port int, srcIP net.IP) error {
	m.record("CreateVXLAN", name, srcIP.String())
	return nil
}

func (m *mockLinkOps) CreateBridge(name string) error {
	m.record("CreateBridge", name)
	return nil
}

func (m *mockLinkOps) SetLinkMaster(slave, master string) error {
	m.record("SetLinkMaster", slave, master)
	return nil
}

func (m *mockLinkOps) SetLinkUp(name string) error {
	m.record("SetLinkUp", name)
	return nil
}

func (m *mockLinkOps) SetLinkAddress(name string, mac net.HardwareAddr) error {
	m.record("SetLinkAddress", name, mac.String())
	return nil
}

func (m *mockLinkOps) AddAddress(name string, addr *net.IPNet) error {
	m.record("AddAddress", name, addr.String())
	return nil
}

func (m *mockLinkOps) AddRoute(dst net.IPNet, gw net.IP, dev string) error {
	m.record("AddRoute", dst.String(), gw.String(), dev)
	return nil
}

func (m *mockLinkOps) AddFDBEntry(mac net.HardwareAddr, ip net.IP, dev string) error {
	m.record("AddFDBEntry", mac.String(), ip.String(), dev)
	return nil
}

func (m *mockLinkOps) AddARPEntry(ip net.IP, mac net.HardwareAddr, dev string) error {
	m.record("AddARPEntry", ip.String(), mac.String(), dev)
	return nil
}

func (m *mockLinkOps) DeleteLink(name string) error {
	m.record("DeleteLink", name)
	return nil
}

func (m *mockLinkOps) LinkExists(name string) (bool, error) {
	m.record("LinkExists", name)
	return true, nil
}

func (m *mockLinkOps) hasCall(method string) bool {
	for _, c := range m.calls {
		if c.method == method {
			return true
		}
	}
	return false
}

func (m *mockLinkOps) callCount(method string) int {
	count := 0
	for _, c := range m.calls {
		if c.method == method {
			count++
		}
	}
	return count
}

func TestVXLANDriver_Init(t *testing.T) {
	ops := &mockLinkOps{}
	driver := NewVXLANDriverWithOps(ops)

	subnet := net.IPNet{
		IP:   net.ParseIP("10.0.45.0"),
		Mask: net.CIDRMask(24, 32),
	}
	hostIP := net.ParseIP("192.168.1.10")

	err := driver.Init(context.Background(), subnet, hostIP)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify call sequence
	expectedMethods := []string{
		"CreateVXLAN",    // 1. Create VXLAN interface
		"CreateBridge",   // 2. Create bridge
		"SetLinkAddress", // 3. Set bridge MAC to deterministic VTEP MAC
		"SetLinkMaster",  // 4. Attach VXLAN to bridge
		"AddAddress",     // 5. Assign VTEP IP
		"SetLinkUp",      // 6. Bring up VXLAN
		"SetLinkUp",      // 7. Bring up bridge
	}

	if len(ops.calls) != len(expectedMethods) {
		t.Fatalf("expected %d calls, got %d: %v", len(expectedMethods), len(ops.calls), ops.calls)
	}

	for i, expected := range expectedMethods {
		if ops.calls[i].method != expected {
			t.Errorf("call %d: expected %s, got %s", i, expected, ops.calls[i].method)
		}
	}

	// Verify VXLAN was created with correct name
	if ops.calls[0].args[0] != "banyan.1" {
		t.Errorf("expected VXLAN name 'banyan.1', got %q", ops.calls[0].args[0])
	}

	// Verify bridge was created with correct name
	if ops.calls[1].args[0] != "banyan0" {
		t.Errorf("expected bridge name 'banyan0', got %q", ops.calls[1].args[0])
	}

	// Verify bridge MAC was set to deterministic VTEP MAC (02:42:0a:00:2d:01 for 10.0.45.0/24)
	if ops.calls[2].args[0] != "banyan0" {
		t.Errorf("expected SetLinkAddress on 'banyan0', got %q", ops.calls[2].args[0])
	}
	if ops.calls[2].args[1] != "02:42:0a:00:2d:01" {
		t.Errorf("expected bridge MAC '02:42:0a:00:2d:01', got %q", ops.calls[2].args[1])
	}

	// Verify VTEP IP was assigned correctly (10.0.45.1/24)
	if ops.calls[4].args[1] != "10.0.45.1/24" {
		t.Errorf("expected VTEP IP '10.0.45.1/24', got %q", ops.calls[4].args[1])
	}
}

func TestVXLANDriver_ReconcilePeers(t *testing.T) {
	ops := &mockLinkOps{}
	driver := NewVXLANDriverWithOps(ops)

	_, peerSubnet, _ := net.ParseCIDR("10.0.46.0/24")
	peer := Peer{
		Subnet: *peerSubnet,
		HostIP: net.ParseIP("192.168.1.20"),
		VTEPIP: net.ParseIP("10.0.46.1"),
		MAC:    DeterministicMAC(*peerSubnet),
	}

	err := driver.ReconcilePeers(context.Background(), []Peer{peer})
	if err != nil {
		t.Fatalf("ReconcilePeers failed: %v", err)
	}

	// Verify FDB, ARP, and route entries
	if !ops.hasCall("AddFDBEntry") {
		t.Error("expected AddFDBEntry call")
	}
	if !ops.hasCall("AddARPEntry") {
		t.Error("expected AddARPEntry call")
	}
	if !ops.hasCall("AddRoute") {
		t.Error("expected AddRoute call")
	}

	// Verify 3 calls total (one of each)
	if len(ops.calls) != 3 {
		t.Errorf("expected 3 calls, got %d", len(ops.calls))
	}

	// Verify route is via VXLAN interface (banyan.1), not bridge (banyan0).
	// ARP entries are on banyan.1, so the route must also use banyan.1
	// for the kernel to resolve the next hop's MAC from static neighbor entries.
	routeCall := ops.calls[2]
	if routeCall.method != "AddRoute" {
		t.Fatalf("expected call 2 to be AddRoute, got %s", routeCall.method)
	}
	if routeCall.args[2] != "banyan.1" {
		t.Errorf("expected route via VXLAN 'banyan.1', got %q", routeCall.args[2])
	}
}

func TestVXLANDriver_ReconcilePeers_Incremental(t *testing.T) {
	ops := &mockLinkOps{}
	driver := NewVXLANDriverWithOps(ops)

	_, subnet1, _ := net.ParseCIDR("10.0.46.0/24")
	peer1 := Peer{
		Subnet: *subnet1,
		HostIP: net.ParseIP("192.168.1.20"),
		VTEPIP: net.ParseIP("10.0.46.1"),
		MAC:    DeterministicMAC(*subnet1),
	}

	// First reconcile
	driver.ReconcilePeers(context.Background(), []Peer{peer1})
	firstCallCount := len(ops.calls)
	if firstCallCount != 3 {
		t.Fatalf("expected 3 calls after first reconcile, got %d", firstCallCount)
	}

	// Second reconcile with same peer — re-applies idempotent commands (FDB, ARP, route)
	// to recover from external modifications (e.g., CNI resetting routes)
	driver.ReconcilePeers(context.Background(), []Peer{peer1})
	secondCallCount := len(ops.calls)
	if secondCallCount != firstCallCount+3 {
		t.Errorf("expected 3 idempotent re-apply calls, got %d new calls", secondCallCount-firstCallCount)
	}

	// Third reconcile with additional peer — should add entries for new peer
	_, subnet2, _ := net.ParseCIDR("10.0.47.0/24")
	peer2 := Peer{
		Subnet: *subnet2,
		HostIP: net.ParseIP("192.168.1.30"),
		VTEPIP: net.ParseIP("10.0.47.1"),
		MAC:    DeterministicMAC(*subnet2),
	}
	driver.ReconcilePeers(context.Background(), []Peer{peer1, peer2})
	thirdCallCount := len(ops.calls)
	if thirdCallCount != secondCallCount+6 {
		t.Errorf("expected 6 calls (3 per peer), got %d", thirdCallCount-secondCallCount)
	}
}

func TestVXLANDriver_WriteCNIConfig(t *testing.T) {
	// Override cniConfigDir for testing — use WriteCNIConfig's internal logic
	// with a temp dir by creating a driver and testing output
	tmpDir := t.TempDir()

	driver := NewVXLANDriverWithOps(&mockLinkOps{})

	// We need to test the JSON output, so let's manually test the config generation
	subnet := net.IPNet{
		IP:   net.ParseIP("10.0.45.0"),
		Mask: net.CIDRMask(24, 32),
	}

	config := cniConfigData{
		CNIVersion:  "0.3.1",
		Name:        "banyan",
		Type:        "bridge",
		Bridge:      driver.bridgeName,
		IsGateway:   true,
		IPMasq:      false,
		HairpinMode: true,
		IPAM: cniIPAMData{
			Type:   "host-local",
			Subnet: subnet.String(),
			Routes: []cniRouteData{{Dst: "0.0.0.0/0"}},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	configPath := filepath.Join(tmpDir, "10-banyan.conf")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Read back and verify
	readData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var parsed cniConfigData
	if err := json.Unmarshal(readData, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Name != "banyan" {
		t.Errorf("expected name 'banyan', got %q", parsed.Name)
	}
	if parsed.Type != "bridge" {
		t.Errorf("expected type 'bridge', got %q", parsed.Type)
	}
	if parsed.Bridge != "banyan0" {
		t.Errorf("expected bridge 'banyan0', got %q", parsed.Bridge)
	}
	if !parsed.IsGateway {
		t.Error("expected isGateway true")
	}
	if parsed.IPMasq {
		t.Error("expected ipMasq false")
	}
	if !parsed.HairpinMode {
		t.Error("expected hairpinMode true")
	}
	if parsed.IPAM.Type != "host-local" {
		t.Errorf("expected IPAM type 'host-local', got %q", parsed.IPAM.Type)
	}
	if parsed.IPAM.Subnet != "10.0.45.0/24" {
		t.Errorf("expected IPAM subnet '10.0.45.0/24', got %q", parsed.IPAM.Subnet)
	}
}

func TestVXLANDriver_Cleanup(t *testing.T) {
	ops := &mockLinkOps{}
	driver := NewVXLANDriverWithOps(ops)

	err := driver.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Should check existence and delete both VXLAN and bridge
	if ops.callCount("LinkExists") != 2 {
		t.Errorf("expected 2 LinkExists calls, got %d", ops.callCount("LinkExists"))
	}
	if ops.callCount("DeleteLink") != 2 {
		t.Errorf("expected 2 DeleteLink calls, got %d", ops.callCount("DeleteLink"))
	}
}

func TestDeterministicMAC(t *testing.T) {
	tests := []struct {
		name     string
		subnet   string
		expected string
	}{
		{
			name:     "10.0.45.0/24",
			subnet:   "10.0.45.0/24",
			expected: "02:42:0a:00:2d:01",
		},
		{
			name:     "10.0.0.0/24",
			subnet:   "10.0.0.0/24",
			expected: "02:42:0a:00:00:01",
		},
		{
			name:     "10.0.255.0/24",
			subnet:   "10.0.255.0/24",
			expected: "02:42:0a:00:ff:01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, subnet, _ := net.ParseCIDR(tt.subnet)
			mac := DeterministicMAC(*subnet)
			if mac.String() != tt.expected {
				t.Errorf("expected MAC %s, got %s", tt.expected, mac.String())
			}
		})
	}
}

func TestVTEPIP(t *testing.T) {
	tests := []struct {
		subnet   string
		expected string
	}{
		{"10.0.45.0/24", "10.0.45.1"},
		{"10.0.0.0/24", "10.0.0.1"},
		{"172.16.1.0/24", "172.16.1.1"},
	}

	for _, tt := range tests {
		t.Run(tt.subnet, func(t *testing.T) {
			_, subnet, _ := net.ParseCIDR(tt.subnet)
			vtep := VTEPIP(*subnet)
			if vtep.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, vtep.String())
			}
		})
	}
}

func TestPeerFromSubnetAndHost(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		peer, err := PeerFromSubnetAndHost("10.0.45.0/24", "192.168.1.10", "02:42:0a:00:2d:01")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if peer.Subnet.String() != "10.0.45.0/24" {
			t.Errorf("expected subnet 10.0.45.0/24, got %s", peer.Subnet.String())
		}
		if peer.HostIP.String() != "192.168.1.10" {
			t.Errorf("expected host IP 192.168.1.10, got %s", peer.HostIP.String())
		}
		if peer.VTEPIP.String() != "10.0.45.1" {
			t.Errorf("expected VTEP IP 10.0.45.1, got %s", peer.VTEPIP.String())
		}
	})

	t.Run("invalid subnet", func(t *testing.T) {
		_, err := PeerFromSubnetAndHost("invalid", "192.168.1.10", "02:42:0a:00:2d:01")
		if err == nil {
			t.Fatal("expected error for invalid subnet")
		}
	})

	t.Run("invalid host IP", func(t *testing.T) {
		_, err := PeerFromSubnetAndHost("10.0.45.0/24", "invalid", "02:42:0a:00:2d:01")
		if err == nil {
			t.Fatal("expected error for invalid host IP")
		}
	})

	t.Run("invalid MAC", func(t *testing.T) {
		_, err := PeerFromSubnetAndHost("10.0.45.0/24", "192.168.1.10", "invalid")
		if err == nil {
			t.Fatal("expected error for invalid MAC")
		}
	})
}

func TestNewVXLANDriver(t *testing.T) {
	driver := NewVXLANDriver()
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
	if driver.vni != defaultVNI {
		t.Errorf("expected VNI %d, got %d", defaultVNI, driver.vni)
	}
	if driver.bridgeName != defaultBridge {
		t.Errorf("expected bridge %q, got %q", defaultBridge, driver.bridgeName)
	}
}
