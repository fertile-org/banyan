# Control Tunnel /32 Routing Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **NOTE FOR THIS EXECUTION:** The user asked **not to commit**. Do every step except the `git commit` steps — leave changes in the working tree for review.

**Goal:** Stop Banyan control-tunnel interfaces from claiming a broad `10.200.0.0/16` route (which conflicts when two roles share a host); use a `/32` interface address plus an explicit per-peer `/32` route instead.

**Architecture:** The control tunnel keeps one WireGuard interface per role. We add a device-scope route operation (`AddDeviceRoute`), change the control-tunnel interface address mask from `/16` to `/32`, and have `AddControlPeer` install a `/32` route to each peer's tunnel IP (matching the `allowed_ips` it already sets).

**Tech Stack:** Go (multi-module workspace via `go.work`), WireGuard, `ip` (iproute2).

> **Module note:** The affected package lives in the `pkg/vpc` module. Run its tests from inside that module: `cd pkg/vpc && go test ./overlay/...`. Do NOT run `go test ./...` from the repo root.

---

## File Structure

| File | Responsibility |
|------|----------------|
| `pkg/vpc/overlay/cni.go` | `LinkOperations` interface — add `AddDeviceRoute`. |
| `pkg/vpc/overlay/link_ops.go` | `ExecLinkOps` — implement `AddDeviceRoute` via `ip route`. |
| `pkg/vpc/overlay/mock_test.go` | `mockLinkOps` — record `AddDeviceRoute` calls for tests. |
| `pkg/vpc/overlay/control_tunnel.go` | `/16`→`/32` interface mask; `AddControlPeer` takes `linkOps` and adds the per-peer route; update `AddControlPeerExec`. |
| `pkg/vpc/overlay/control_tunnel_test.go` | Update existing tests for `/32` mask and the new `AddControlPeer` signature/route. |

---

## Task 1: Add a device-scope route operation

`AddRoute` requires a gateway (`ip route ... via <gw> ... onlink`) and cannot
express a plain device route. Add a new `LinkOperations` method for routes that
are directly on a device (no gateway). This is interface + exec plumbing; its
behavior is exercised through the mock in Task 3.

**Files:**
- Modify: `pkg/vpc/overlay/cni.go`
- Modify: `pkg/vpc/overlay/link_ops.go`
- Modify: `pkg/vpc/overlay/mock_test.go`

- [ ] **Step 1: Add the method to the `LinkOperations` interface**

In `pkg/vpc/overlay/cni.go`, the interface currently reads:

```go
type LinkOperations interface {
	CreateBridge(name string) error
	SetLinkUp(name string) error
	SetLinkAddress(name string, mac net.HardwareAddr) error
	AddAddress(name string, addr *net.IPNet) error
	AddRoute(dst net.IPNet, gw net.IP, dev string) error
	DeleteLink(name string) error
	LinkExists(name string) (bool, error)
}
```

Add `AddDeviceRoute` right after `AddRoute`:

```go
type LinkOperations interface {
	CreateBridge(name string) error
	SetLinkUp(name string) error
	SetLinkAddress(name string, mac net.HardwareAddr) error
	AddAddress(name string, addr *net.IPNet) error
	AddRoute(dst net.IPNet, gw net.IP, dev string) error
	AddDeviceRoute(dst net.IPNet, dev string) error
	DeleteLink(name string) error
	LinkExists(name string) (bool, error)
}
```

- [ ] **Step 2: Implement it in `ExecLinkOps`**

In `pkg/vpc/overlay/link_ops.go`, add this method directly after the existing
`AddRoute` method (after line 36):

```go
// AddDeviceRoute adds a route to dst reachable directly on dev, with no gateway.
// Used for per-peer control-tunnel routes (e.g. 10.200.108.133/32 dev wg-ctl-eng).
func (e *ExecLinkOps) AddDeviceRoute(dst net.IPNet, dev string) error {
	return runCmd("ip", "route", "replace", dst.String(), "dev", dev)
}
```

- [ ] **Step 3: Add it to the test mock**

In `pkg/vpc/overlay/mock_test.go`, add this method directly after the existing
`AddRoute` mock (after line 43):

```go
func (m *mockLinkOps) AddDeviceRoute(dst net.IPNet, dev string) error {
	m.record("AddDeviceRoute", dst.String(), dev)
	return nil
}
```

- [ ] **Step 4: Verify the package compiles and existing tests still pass**

Run: `cd pkg/vpc && go build ./overlay/... && go vet ./overlay/... && go test ./overlay/...`
Expected: build/vet clean; all existing overlay tests PASS (no behavior changed yet).

- [ ] **Step 5: Commit** _(skip for this execution — see note at top)_

```bash
git add pkg/vpc/overlay/cni.go pkg/vpc/overlay/link_ops.go pkg/vpc/overlay/mock_test.go
git commit -m "feat(overlay): add AddDeviceRoute for gateway-less device routes"
```

---

## Task 2: Use a /32 mask for the control-tunnel interface

**Files:**
- Modify: `pkg/vpc/overlay/control_tunnel.go`
- Test: `pkg/vpc/overlay/control_tunnel_test.go`

- [ ] **Step 1: Update the existing tests to expect /32**

In `pkg/vpc/overlay/control_tunnel_test.go`, change the two assertions that
currently expect a `/16` address.

In `TestSetupControlTunnel` (around line 51), change:

```go
	if linkOps.calls[2].args[1] != "10.200.0.1/16" {
		t.Errorf("expected IP '10.200.0.1/16', got %q", linkOps.calls[2].args[1])
	}
```

to:

```go
	if linkOps.calls[2].args[1] != "10.200.0.1/32" {
		t.Errorf("expected IP '10.200.0.1/32', got %q", linkOps.calls[2].args[1])
	}
```

In `TestSetupControlTunnel_AgentIP` (around line 73-76), change:

```go
	// Verify IP assignment uses /16 mask (index 2: after LinkExists + DeleteLink)
	if linkOps.calls[2].args[1] != "10.200.173.42/16" {
		t.Errorf("expected IP '10.200.173.42/16', got %q", linkOps.calls[2].args[1])
	}
```

to:

```go
	// Verify IP assignment uses /32 mask (index 2: after LinkExists + DeleteLink)
	if linkOps.calls[2].args[1] != "10.200.173.42/32" {
		t.Errorf("expected IP '10.200.173.42/32', got %q", linkOps.calls[2].args[1])
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd pkg/vpc && go test ./overlay/ -run TestSetupControlTunnel -v`
Expected: FAIL — both tests report `expected IP '.../32', got '.../16'` (code still assigns /16).

- [ ] **Step 3: Change the interface mask to /32**

In `pkg/vpc/overlay/control_tunnel.go`, the constant block currently reads:

```go
const (
	controlTunnelCIDR = 16
	controlKeepalive  = 25
)
```

Replace it with (the `/16` constant is no longer used):

```go
const controlKeepalive = 25
```

Then in `SetupControlTunnel`, change the address assignment (around lines 38-42)
from:

```go
	// Assign tunnel IP with /16 mask
	addr := &net.IPNet{
		IP:   myIP,
		Mask: net.CIDRMask(controlTunnelCIDR, 32),
	}
```

to:

```go
	// Assign tunnel IP with a /32 mask: the interface owns only its own IP and
	// does NOT install a broad 10.200.0.0/16 connected route. Routes to peers are
	// added per-peer in AddControlPeer. This lets multiple control tunnels
	// (e.g. engine + co-located CLI) coexist on one host without route conflicts.
	addr := &net.IPNet{
		IP:   myIP,
		Mask: net.CIDRMask(32, 32),
	}
```

Also update the function doc comment (line 16) from "assigned myIP with a /16
mask" to "assigned myIP with a /32 mask".

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd pkg/vpc && go test ./overlay/ -run TestSetupControlTunnel -v`
Expected: PASS (both `TestSetupControlTunnel` and `TestSetupControlTunnel_AgentIP`).

- [ ] **Step 5: Commit** _(skip for this execution — see note at top)_

```bash
git add pkg/vpc/overlay/control_tunnel.go pkg/vpc/overlay/control_tunnel_test.go
git commit -m "fix(overlay): use /32 mask for control-tunnel interface"
```

---

## Task 3: Add a per-peer /32 route in AddControlPeer

**Files:**
- Modify: `pkg/vpc/overlay/control_tunnel.go`
- Test: `pkg/vpc/overlay/control_tunnel_test.go`

- [ ] **Step 1: Update the existing AddControlPeer tests for the new signature + route**

In `pkg/vpc/overlay/control_tunnel_test.go`, replace the whole
`TestAddControlPeer` function (lines 79-101) with:

```go
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
```

Replace the whole `TestAddControlPeer_NoEndpoint` function (lines 103-119) with:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd pkg/vpc && go test ./overlay/ -run TestAddControlPeer -v`
Expected: FAIL — compile error: not enough arguments in call to `AddControlPeer`
(the signature does not yet take `linkOps`).

- [ ] **Step 3: Update AddControlPeer and its Exec wrapper**

In `pkg/vpc/overlay/control_tunnel.go`, replace the whole `AddControlPeer`
function (lines 55-63) with:

```go
// AddControlPeer adds a peer to the specified control tunnel interface and
// installs a /32 route to the peer's tunnel IP via that interface.
// Endpoint may be empty (engine learns agent endpoints from incoming packets).
func AddControlPeer(wgOps WireGuardOps, linkOps LinkOperations, iface, pubKey, endpoint string, tunnelIP net.IP) error {
	allowedIPs := []string{tunnelIP.String() + "/32"}
	if err := wgOps.AddPeer(iface, pubKey, endpoint, allowedIPs, controlKeepalive); err != nil {
		return fmt.Errorf("add control peer %s: %w", pubKey[:8], err)
	}

	route := net.IPNet{IP: tunnelIP, Mask: net.CIDRMask(32, 32)}
	if err := linkOps.AddDeviceRoute(route, iface); err != nil {
		return fmt.Errorf("add control peer route %s: %w", tunnelIP, err)
	}
	return nil
}
```

Then replace the whole `AddControlPeerExec` wrapper (lines 85-88) with:

```go
// AddControlPeerExec is a convenience wrapper using exec-based operations.
func AddControlPeerExec(iface, pubKey, endpoint string, tunnelIP net.IP) error {
	return AddControlPeer(&ExecWireGuardOps{}, &ExecLinkOps{}, iface, pubKey, endpoint, tunnelIP)
}
```

(The wrapper's external signature is unchanged, so the engine, agent, and CLI
callers — which all use `AddControlPeerExec` / `addControlPeerFn` — need no edits.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd pkg/vpc && go test ./overlay/ -run TestAddControlPeer -v`
Expected: PASS (`TestAddControlPeer` and `TestAddControlPeer_NoEndpoint`).

- [ ] **Step 5: Run the full overlay test suite**

Run: `cd pkg/vpc && go test ./overlay/...`
Expected: PASS (no regressions).

- [ ] **Step 6: Verify the dependent CLI modules still build**

The control-tunnel callers live in separate modules. Confirm the unchanged
`AddControlPeerExec` signature didn't break them:

Run:
```bash
cd cmd/banyan-engine && go build ./... && \
cd ../banyan-agent && go build ./... && \
cd ../banyan-cli && go build ./...
```
Expected: all three build with no errors.

- [ ] **Step 7: Commit** _(skip for this execution — see note at top)_

```bash
git add pkg/vpc/overlay/control_tunnel.go pkg/vpc/overlay/control_tunnel_test.go
git commit -m "fix(overlay): add per-peer /32 route for control tunnel peers"
```

---

## Self-Review Checklist

1. **Spec coverage:**
   - [ ] New device-scope route op → Task 1
   - [ ] /32 interface mask → Task 2
   - [ ] Per-peer /32 route in `AddControlPeer` (+ `linkOps` param, Exec wrapper) → Task 3
   - [ ] Callers unchanged (engine/agent/CLI use `AddControlPeerExec`) → Task 3 Step 6 verifies
   - [ ] Mock updated → Task 1 Step 3
   - [ ] Tests for /32 mask + per-peer route → Task 2 / Task 3

2. **Placeholder scan:**
   - [ ] No "TBD"/"TODO"/"handle edge cases" — every code step shows full code.

3. **Type/name consistency:**
   - [ ] `AddDeviceRoute(dst net.IPNet, dev string) error` identical in interface (Task 1.1), `ExecLinkOps` (1.2), mock (1.3), and call site (Task 3.3).
   - [ ] `AddControlPeer(wgOps, linkOps, iface, pubKey, endpoint, tunnelIP)` signature consistent between tests (Task 3.1) and implementation (Task 3.3).
   - [ ] `controlKeepalive` retained; `controlTunnelCIDR` removed (Task 2.3) and not referenced elsewhere.

4. **Build & tests:**
   - [ ] `cd pkg/vpc && go test ./overlay/...` passes
   - [ ] `cd cmd/banyan-engine && go build ./...` (and agent, cli) succeed

5. **Migration note (no code):**
   - [ ] After deploying the new binary and restarting services, the control
         interface is recreated with /32; the staging manual workarounds
         (`ip route replace 10.200.108.133/32 ...` and the `ExecStartPost`
         drop-in) can be removed.
