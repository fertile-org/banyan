# [MED-001] Services Bind to 0.0.0.0 by Default

**Status**: FIXED (2026-03-07) — All services now bind to control tunnel IPs or `127.0.0.1`. Engine gRPC, OCI registry, and metrics bind to `10.200.0.1` (or `127.0.0.1` in insecure mode). Agent gRPC server binds to the agent's tunnel IP (derived from `TunnelIPFromPublicKey`).
**Severity**: Medium
**Responsibility**: Default Issue
**Component**: Engine, Agent, Metrics
**File(s)**:
- `pkg/engine/grpc_server.go:64` (engine gRPC — `":"+opts.Port`)
- `pkg/agent/grpc_server.go:26` (agent gRPC — `":"+port`)
- `pkg/engine/engine.go:215` (Prometheus metrics — `":"+port`)
- `pkg/engine/engine.go:766` (OCI registry — `":"+port`)

## Description

All network services bind to `0.0.0.0` (all interfaces), making them accessible from any network including public-facing interfaces. This applies to:

1. **Engine gRPC** (port 50051) — full control plane API
2. **Agent gRPC** (dynamic port) — log streaming
3. **Prometheus metrics** (port 9090) — cluster metrics, agent names, deployment data
4. **OCI registry** (port 5000) — container images

When combined with WireGuard, the intended access path is through the tunnel interface. Binding to all interfaces exposes these services beyond the tunnel.

## Impact

Services are reachable from networks they shouldn't be on (public interfaces, adjacent VPCs). The metrics endpoint is unauthenticated and reveals cluster topology.

## Recommendation

Default to binding on the WireGuard tunnel interface when configured, or `127.0.0.1` when not. Allow override via `--bind-address` flag.

## Secure Default Consideration

- **Default**: Bind to WireGuard tunnel IP or localhost
- **Override**: `--bind-address 0.0.0.0` with warning
