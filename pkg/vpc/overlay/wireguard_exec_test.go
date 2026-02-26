package overlay

import "testing"

// mockWireGuardOps records all calls to verify correct operation sequences.
type mockWireGuardOps struct {
	calls []mockCall
}

func (m *mockWireGuardOps) record(method string, args ...string) {
	m.calls = append(m.calls, mockCall{method: method, args: args})
}

func (m *mockWireGuardOps) CreateInterface(name string) error {
	m.record("CreateInterface", name)
	return nil
}

func (m *mockWireGuardOps) ConfigureInterface(name, privateKeyB64 string, port int) error {
	m.record("ConfigureInterface", name)
	return nil
}

func (m *mockWireGuardOps) AddPeer(iface, pubKey, endpoint string, allowedIPs []string, keepalive int) error {
	m.record("AddPeer", iface, pubKey, endpoint)
	return nil
}

func (m *mockWireGuardOps) RemovePeer(iface, pubKey string) error {
	m.record("RemovePeer", iface, pubKey)
	return nil
}

func (m *mockWireGuardOps) hasCall(method string) bool {
	for _, c := range m.calls {
		if c.method == method {
			return true
		}
	}
	return false
}

func (m *mockWireGuardOps) callCount(method string) int {
	count := 0
	for _, c := range m.calls {
		if c.method == method {
			count++
		}
	}
	return count
}

func TestMockWireGuardOps_Interface(t *testing.T) {
	// Verify our mock implements the interface
	var _ WireGuardOps = (*mockWireGuardOps)(nil)

	ops := &mockWireGuardOps{}

	ops.CreateInterface("banyan-wg")
	ops.ConfigureInterface("banyan-wg", "base64key", 51820)
	ops.AddPeer("banyan-wg", "pubkey", "192.168.1.1:51820", []string{"10.0.45.0/24"}, 25)
	ops.RemovePeer("banyan-wg", "pubkey")

	if len(ops.calls) != 4 {
		t.Fatalf("expected 4 calls, got %d", len(ops.calls))
	}

	if ops.calls[0].method != "CreateInterface" {
		t.Errorf("expected CreateInterface, got %s", ops.calls[0].method)
	}
	if ops.calls[0].args[0] != "banyan-wg" {
		t.Errorf("expected banyan-wg, got %s", ops.calls[0].args[0])
	}

	if ops.calls[2].method != "AddPeer" {
		t.Errorf("expected AddPeer, got %s", ops.calls[2].method)
	}
	if ops.calls[2].args[1] != "pubkey" {
		t.Errorf("expected pubkey, got %s", ops.calls[2].args[1])
	}
}

func TestExecWireGuardOps_Interface(t *testing.T) {
	// Verify ExecWireGuardOps implements the interface
	var _ WireGuardOps = (*ExecWireGuardOps)(nil)
}
