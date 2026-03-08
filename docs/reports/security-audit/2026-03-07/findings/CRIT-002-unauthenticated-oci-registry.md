# [CRIT-002] OCI Registry Unauthenticated and Unencrypted

**Status**: PARTIALLY FIXED (2026-03-07) — Registry now binds to WireGuard control tunnel IP (`10.200.0.1`) when keys are configured, or `127.0.0.1` in insecure mode. No longer listens on `0.0.0.0`. Remaining: no registry-level auth, no TLS, `--insecure-registry` still used.
**Severity**: Critical
**Responsibility**: Platform Issue
**Component**: Engine — Embedded OCI Registry
**File(s)**:
- `pkg/engine/engine.go:762-786` (registry setup — no auth, no TLS)
- `pkg/engine/engine.go:766` (binds to `0.0.0.0`)
- `pkg/agent/agent.go:355` (agent pulls with `--insecure-registry`)
- `cmd/banyan-cli/cmd/deploy.go:379` (CLI pushes with `--insecure-registry`)

## Description

The embedded OCI registry is a plain HTTP server with no authentication:

1. **No authentication**: `registry.New()` is called with no auth middleware. Anyone can push and pull images.
2. **No TLS**: The HTTP server has no TLS configuration. Image content travels in plaintext.
3. **Binds to all interfaces**: `net.Listen("tcp", ":"+port)` binds to `0.0.0.0`, making the registry reachable from any network.
4. **Insecure flag**: Both CLI push and agent pull use `--insecure-registry`, which disables TLS verification.

## Impact

- **Who**: Any host that can reach the engine on port 5000 (default)
- **What they gain**:
  - **Push malicious images**: An attacker can overwrite any image tag (e.g., `my-app:latest`) with a trojaned version. Agents will pull and execute it on the next deployment.
  - **Pull proprietary images**: All built images (application source code compiled into containers) are readable by anyone.
  - **MITM image content**: Since transport is unencrypted, an attacker between CLI/agent and engine can modify image layers in transit.
- **Blast radius**: All nodes in the cluster. Every agent that pulls a poisoned image runs attacker-controlled code.

## Evidence

```go
// pkg/engine/engine.go:770-771 — no TLS, no auth
registryHandler := registry.New()
registryServer := &http.Server{Handler: registryHandler}

// pkg/engine/engine.go:766 — binds to all interfaces
lis, err := net.Listen("tcp", ":"+port)

// pkg/agent/agent.go:355 — insecure pull
commandRunner(ctx, "nerdctl", "pull", "--insecure-registry", task.Image)
```

## Recommendation

1. **Bind to WireGuard tunnel interface** (not `0.0.0.0`) so only authenticated agents can reach it
2. **Add basic auth** using a token derived from the WireGuard public key or a shared registry secret
3. **Add TLS** with auto-generated certificates
4. **Remove `--insecure-registry`** from agent pull and CLI push once TLS is in place
5. **Verify image digests** — pin images by digest, not just tag
