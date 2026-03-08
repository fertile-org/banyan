# [HIGH-001] Public Key Auth Is Identity Assertion, Not Cryptographic Proof

**Status**: FIXED (2026-03-07) — Custom public key auth layer (`PublicKeyCredentials`, `PublicKeyValidator`, metadata interceptors) removed entirely. Authentication now relies on WireGuard's cryptographic handshake at the tunnel layer. Only peers with whitelisted WireGuard private keys can establish a tunnel connection to the engine. The TCP fallback path is removed — engine requires WireGuard keys or explicit `--allow-insecure` flag (which binds to localhost only).
**Severity**: High
**Responsibility**: Platform Issue
**Component**: Authentication
**File(s)**:
- `pkg/rpc/auth.go` (old: `PublicKeyCredentials` — now removed)
- `pkg/engine/grpc_server.go` (old: `PublicKeyValidator` interceptors — now removed)

## Description

The `PublicKeyCredentials` type sent the WireGuard public key as plaintext gRPC metadata (`x-banyan-public-key`). The server validated it by checking if the key existed in a whitelist map. There was no challenge-response, no signature verification, and no proof that the caller possessed the corresponding private key.

## Impact

- **Who**: Any attacker who intercepted or learned an agent's public key (public by nature in WireGuard)
- **What they gained**: Impersonate that agent — register, heartbeat, poll tasks, report results
- **Blast radius**: Per-agent — attacker gained control of one agent's identity

## Resolution

The entire custom auth layer was removed. WireGuard's Noise protocol handshake provides cryptographic proof of private key possession at the tunnel level. The engine now:
1. Requires WireGuard keys to start (or explicit `--allow-insecure` for dev, which binds to localhost)
2. Binds gRPC to the WireGuard tunnel IP (`10.200.0.1`), making it unreachable without a valid tunnel
3. Identifies agents by their tunnel IP (reverse-mapped from `TunnelIPFromPublicKey`)
