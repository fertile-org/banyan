package overlay

import (
	"net"
	"testing"
)

func TestSetupControlTunnel(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	myIP := net.ParseIP("10.200.0.1")

	err := SetupControlTunnel(wgOps, linkOps, "wg-ctl-eng", "privkey-base64", myIP, 51821)
	if err != nil {
		t.Fatalf("SetupControlTunnel failed: %v", err)
	}

	// Verify WireGuard operations
	if wgOps.callCount("CreateInterface") != 1 {
		t.Errorf("expected 1 CreateInterface call, got %d", wgOps.callCount("CreateInterface"))
	}
	if wgOps.calls[0].args[0] != "wg-ctl-eng" {
		t.Errorf("expected interface 'wg-ctl-eng', got %q", wgOps.calls[0].args[0])
	}

	if wgOps.callCount("ConfigureInterface") != 1 {
		t.Errorf("expected 1 ConfigureInterface call, got %d", wgOps.callCount("ConfigureInterface"))
	}
	if wgOps.calls[1].args[0] != "wg-ctl-eng" {
		t.Errorf("expected ConfigureInterface on 'wg-ctl-eng', got %q", wgOps.calls[1].args[0])
	}

	// Verify link operations sequence: LinkExists, DeleteLink (cleanup old), AddAddress, SetLinkUp
	if len(linkOps.calls) != 4 {
		t.Fatalf("expected 4 link calls, got %d: %v", len(linkOps.calls), linkOps.calls)
	}

	if linkOps.calls[0].method != "LinkExists" {
		t.Errorf("call 0: expected LinkExists, got %s", linkOps.calls[0].method)
	}
	if linkOps.calls[1].method != "DeleteLink" {
		t.Errorf("call 1: expected DeleteLink, got %s", linkOps.calls[1].method)
	}

	if linkOps.calls[2].method != "AddAddress" {
		t.Errorf("call 2: expected AddAddress, got %s", linkOps.calls[2].method)
	}
	if linkOps.calls[2].args[0] != "wg-ctl-eng" {
		t.Errorf("expected AddAddress on 'wg-ctl-eng', got %q", linkOps.calls[2].args[0])
	}
	if linkOps.calls[2].args[1] != "10.200.0.1/32" {
		t.Errorf("expected IP '10.200.0.1/32', got %q", linkOps.calls[2].args[1])
	}

	if linkOps.calls[3].method != "SetLinkUp" {
		t.Errorf("call 3: expected SetLinkUp, got %s", linkOps.calls[3].method)
	}
	if linkOps.calls[3].args[0] != "wg-ctl-eng" {
		t.Errorf("expected SetLinkUp on 'wg-ctl-eng', got %q", linkOps.calls[3].args[0])
	}
}

func TestSetupControlTunnel_AgentIP(t *testing.T) {
	linkOps := &mockLinkOps{}
	wgOps := &mockWireGuardOps{}
	myIP := net.ParseIP("10.200.173.42")

	err := SetupControlTunnel(wgOps, linkOps, "wg-ctl-agt", "agent-privkey", myIP, 0)
	if err != nil {
		t.Fatalf("SetupControlTunnel failed: %v", err)
	}

	// Verify IP assignment uses /32 mask (index 2: after LinkExists + DeleteLink)
	if linkOps.calls[2].args[1] != "10.200.173.42/32" {
		t.Errorf("expected IP '10.200.173.42/32', got %q", linkOps.calls[2].args[1])
	}
}

func TestAddControlPeer(t *testing.T) {
	wgOps := &mockWireGuardOps{}
	linkOps := &mockLinkOps{}
	tunnelIP := net.ParseIP("10.200.0.1")

	err := AddControlPeer(wgOps, linkOps, "wg-ctl-eng", "peer-pubkey-base64", "192.168.1.100:51821", tunnelIP)
	if err != nil {
		t.Fatalf("AddControlPeer failed: %v", err)
	}

	if wgOps.callCount("AddPeer") != 1 {
		t.Fatalf("expected 1 AddPeer call, got %d", wgOps.callCount("AddPeer"))
	}
	call := wgOps.calls[0]
	if call.args[0] != "wg-ctl-eng" {
		t.Errorf("expected iface 'wg-ctl-eng', got %q", call.args[0])
	}
	if call.args[1] != "peer-pubkey-base64" {
		t.Errorf("expected pubkey 'peer-pubkey-base64', got %q", call.args[1])
	}
	if call.args[2] != "192.168.1.100:51821" {
		t.Errorf("expected endpoint '192.168.1.100:51821', got %q", call.args[2])
	}

	// A /32 device route to the peer's tunnel IP must be added.
	if linkOps.callCount("AddDeviceRoute") != 1 {
		t.Fatalf("expected 1 AddDeviceRoute call, got %d", linkOps.callCount("AddDeviceRoute"))
	}
	var routeCall mockCall
	for _, c := range linkOps.calls {
		if c.method == "AddDeviceRoute" {
			routeCall = c
		}
	}
	if routeCall.args[0] != "10.200.0.1/32" {
		t.Errorf("expected route '10.200.0.1/32', got %q", routeCall.args[0])
	}
	if routeCall.args[1] != "wg-ctl-eng" {
		t.Errorf("expected route dev 'wg-ctl-eng', got %q", routeCall.args[1])
	}
}

func TestAddControlPeer_NoEndpoint(t *testing.T) {
	wgOps := &mockWireGuardOps{}
	linkOps := &mockLinkOps{}
	tunnelIP := net.ParseIP("10.200.42.5")

	err := AddControlPeer(wgOps, linkOps, "wg-ctl-agt", "agent-pubkey", "", tunnelIP)
	if err != nil {
		t.Fatalf("AddControlPeer failed: %v", err)
	}

	if wgOps.callCount("AddPeer") != 1 {
		t.Fatalf("expected 1 AddPeer call, got %d", wgOps.callCount("AddPeer"))
	}
	// Empty endpoint is valid (engine learns from incoming packets)
	if wgOps.calls[0].args[2] != "" {
		t.Errorf("expected empty endpoint, got %q", wgOps.calls[0].args[2])
	}

	// Route to the peer's tunnel IP is added regardless of endpoint.
	if linkOps.callCount("AddDeviceRoute") != 1 {
		t.Fatalf("expected 1 AddDeviceRoute call, got %d", linkOps.callCount("AddDeviceRoute"))
	}
	var routeCall mockCall
	for _, c := range linkOps.calls {
		if c.method == "AddDeviceRoute" {
			routeCall = c
		}
	}
	if routeCall.args[0] != "10.200.42.5/32" {
		t.Errorf("expected route '10.200.42.5/32', got %q", routeCall.args[0])
	}
	if routeCall.args[1] != "wg-ctl-agt" {
		t.Errorf("expected route dev 'wg-ctl-agt', got %q", routeCall.args[1])
	}
}

func TestCleanupControlTunnel(t *testing.T) {
	t.Run("interface exists", func(t *testing.T) {
		linkOps := &mockLinkOps{}

		err := CleanupControlTunnel(linkOps, "wg-ctl-eng")
		if err != nil {
			t.Fatalf("CleanupControlTunnel failed: %v", err)
		}

		if linkOps.callCount("LinkExists") != 1 {
			t.Errorf("expected 1 LinkExists call, got %d", linkOps.callCount("LinkExists"))
		}
		if linkOps.callCount("DeleteLink") != 1 {
			t.Errorf("expected 1 DeleteLink call, got %d", linkOps.callCount("DeleteLink"))
		}
		if linkOps.calls[1].args[0] != "wg-ctl-eng" {
			t.Errorf("expected DeleteLink on 'wg-ctl-eng', got %q", linkOps.calls[1].args[0])
		}
	})

	t.Run("interface does not exist", func(t *testing.T) {
		linkOps := &mockLinkOpsNoExist{}

		err := CleanupControlTunnel(linkOps, "wg-ctl-eng")
		if err != nil {
			t.Fatalf("CleanupControlTunnel failed: %v", err)
		}
		// Should not attempt to delete
		if linkOps.deleteCount > 0 {
			t.Errorf("expected 0 DeleteLink calls, got %d", linkOps.deleteCount)
		}
	})
}

// mockLinkOpsNoExist is a minimal LinkOperations mock where LinkExists returns false.
type mockLinkOpsNoExist struct {
	mockLinkOps
	deleteCount int
}

func (m *mockLinkOpsNoExist) LinkExists(name string) (bool, error) {
	return false, nil
}

func (m *mockLinkOpsNoExist) DeleteLink(name string) error {
	m.deleteCount++
	return nil
}
