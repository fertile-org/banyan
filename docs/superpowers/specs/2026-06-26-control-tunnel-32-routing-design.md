# Control Tunnel /32 Routing — Design Spec

**Date:** 2026-06-26
**Status:** Approved (pending spec review)

## Problem

Each Banyan role (engine, agent, CLI) runs its own WireGuard control-tunnel
interface (`wg-ctl-eng`, `wg-ctl-agt`, `wg-ctl-cli`) with its own keypair and a
deterministic tunnel IP in `10.200.0.0/16` (`pkg/types.ControlTunnelCIDR`).

`SetupControlTunnel` assigns the interface address with a **/16 mask**
(`pkg/vpc/overlay/control_tunnel.go:38-42`). That makes the kernel install a
connected route `10.200.0.0/16 dev <iface>` for every control-tunnel interface.

When two roles are co-located on one host — e.g. running `banyan-cli` on the
engine server — that host ends up with two interfaces (`wg-ctl-eng` and
`wg-ctl-cli`) **both claiming the same `10.200.0.0/16`**. The two overlapping
connected routes are ambiguous: replies to a third party's tunnel IP (e.g. an
agent at `10.200.108.133`) can be sent out the wrong interface, which has no
matching peer, and are dropped.

Observed on staging (2026-06-26, Oracle Linux 9 / arm64 / OCI): WireGuard
handshake succeeded, the agent's gRPC SYN reached the engine, but no SYN-ACK
returned (it left via the wrong tunnel), so the agent was stuck at
"Waiting for engine gRPC to be ready". Strict `rp_filter` (effective max of
`all`=0 and per-iface=1) compounded it. Manual workarounds on the server:
`sysctl net.ipv4.conf.all.rp_filter=2` and
`ip route replace 10.200.108.133/32 dev wg-ctl-eng`.

See `[[banyan-control-tunnel-routing]]` (memory) for the field notes.

## Goal

Make co-locating multiple control-tunnel roles on one host work correctly by
removing the broad `/16` connected route. Use a `/32` interface address plus an
explicit per-peer `/32` route, so every tunnel IP resolves to exactly one route.

This preserves the existing architecture (one WG identity/interface per role) —
it only changes how routes are scoped.

## Non-Goals (YAGNI)

- No dynamic peer removal / per-peer route cleanup. Control peers are only added
  at setup time; `SetupControlTunnel` deletes and recreates the interface on each
  start, and routes bound to a device are removed automatically when the device
  is deleted. (Confirmed: no `RemoveControlPeer` caller exists.)
- No change to the data-plane overlay (`wg-ctl` data tunnel / CNI). Only the
  control tunnel is affected.
- No change to `ControlTunnelCIDR` (`10.200.0.0/16` is still the address space the
  tunnel IPs are drawn from; only the interface *mask* changes).

## Design

### 1. New device-scope route op

`AddRoute` (`pkg/vpc/overlay/link_ops.go:30`) requires a gateway
(`ip route ... via <gw> ... onlink`) and cannot express a plain device route, so
add a new method to the `LinkOperations` interface (`pkg/vpc/overlay/cni.go:12`):

```go
// AddDeviceRoute adds a route to dst reachable directly on dev (no gateway).
AddDeviceRoute(dst net.IPNet, dev string) error
```

`ExecLinkOps` implementation (`link_ops.go`):

```go
func (e *ExecLinkOps) AddDeviceRoute(dst net.IPNet, dev string) error {
	return runCmd("ip", "route", "replace", dst.String(), "dev", dev)
}
```

(The existing `AddRoute` is left unchanged — the data-plane still uses its
`via gw onlink` form.)

### 2. /32 interface address

In `SetupControlTunnel` (`control_tunnel.go`), change the interface address mask
from `/16` to `/32` so the interface no longer installs a broad connected route:

```go
addr := &net.IPNet{
	IP:   myIP,
	Mask: net.CIDRMask(32, 32),
}
```

Update the `controlTunnelCIDR = 16` constant/comment accordingly (it is only used
for this mask).

### 3. Per-peer /32 route in AddControlPeer

`AddControlPeer` already sets `allowed_ips = tunnelIP/32`. Add the matching OS
route. Its signature gains a `LinkOperations`:

```go
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

### 4. Wrapper keeps external signature

`AddControlPeerExec` passes `&ExecLinkOps{}` so the three callers
(engine `cmd/banyan-engine/cmd/engine.go:881`, agent
`cmd/banyan-agent/cmd/agent.go:479`, CLI `cmd/banyan-cli/cmd/init.go:29` via
`addControlPeerFn`) do **not** change:

```go
func AddControlPeerExec(iface, pubKey, endpoint string, tunnelIP net.IP) error {
	return AddControlPeer(&ExecWireGuardOps{}, &ExecLinkOps{}, iface, pubKey, endpoint, tunnelIP)
}
```

### 5. Migration

No manual migration. `SetupControlTunnel` already deletes and recreates the
interface on every start, so after the new binary is deployed and the
service restarts, the interface is recreated with the `/32` address and per-peer
routes. The manual staging workarounds (the `10.200.108.133/32` route and the
`ExecStartPost` drop-in) become unnecessary and can be removed.

## Testing

- `control_tunnel_test.go`:
  - `SetupControlTunnel` assigns the interface address with a `/32` mask
    (assert the `*net.IPNet` passed to the mock `AddAddress` has mask `/32`).
  - `AddControlPeer` calls `AddDeviceRoute` with `tunnelIP/32` and the correct
    iface, in addition to `AddPeer`.
- Update the mock `LinkOperations` (`pkg/vpc/overlay/mock_test.go`) to implement
  `AddDeviceRoute` and record its calls.
- Existing data-plane tests using `AddRoute` are unaffected.

## Affected Files

| File | Change |
|------|--------|
| `pkg/vpc/overlay/cni.go` | Add `AddDeviceRoute` to `LinkOperations` interface |
| `pkg/vpc/overlay/link_ops.go` | Implement `AddDeviceRoute` in `ExecLinkOps` |
| `pkg/vpc/overlay/control_tunnel.go` | `/16`→`/32` mask; `AddControlPeer` adds per-peer route + takes `linkOps`; update `AddControlPeerExec` |
| `pkg/vpc/overlay/mock_test.go` | Mock implements/records `AddDeviceRoute` |
| `pkg/vpc/overlay/control_tunnel_test.go` | Tests for /32 mask and per-peer route |
