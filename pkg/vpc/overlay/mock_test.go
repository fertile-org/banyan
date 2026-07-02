package overlay

import "net"

// mockLinkOps records all calls to verify correct operation order and args.
type mockLinkOps struct {
	calls     []mockCall
	existsFor map[string]bool // override LinkExists per name; nil means all return true
}

type mockCall struct {
	method string
	args   []string
}

func (m *mockLinkOps) record(method string, args ...string) {
	m.calls = append(m.calls, mockCall{method: method, args: args})
}

func (m *mockLinkOps) CreateBridge(name string) error {
	m.record("CreateBridge", name)
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

func (m *mockLinkOps) AddDeviceRoute(dst net.IPNet, dev string) error {
	m.record("AddDeviceRoute", dst.String(), dev)
	return nil
}

func (m *mockLinkOps) DeleteLink(name string) error {
	m.record("DeleteLink", name)
	return nil
}

func (m *mockLinkOps) LinkExists(name string) (bool, error) {
	m.record("LinkExists", name)
	if m.existsFor != nil {
		return m.existsFor[name], nil
	}
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
