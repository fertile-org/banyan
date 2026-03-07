# [LOW-DEAD-001] Deprecated Proto Fields Never Used

**Status**: FIXED (2026-03-06) — Added [deprecated = true] proto3 option to all 3 fields
**Severity**: Low
**Category**: DEAD
**Component**: pkg/rpc/proto
**File(s)**: `pkg/rpc/proto/banyan/v1/engine.proto:50,53,83`

## Description

Three proto fields are marked deprecated in comments but are never read or written in Go code:
- `RegisterResponse.store_endpoints`
- `RegisterResponse.overlay_type`
- `VPCPeer.vtep_mac`

## Evidence

- Fields exist in proto definition with deprecation comments
- Grep for `StoreEndpoints`, `OverlayType`, `VtepMac` across Go code returns zero results
- Fields are wire-compatible no-ops

## Impact

- Confuses new developers reading proto definitions
- Minor proto message size overhead (empty fields still take tag bytes)

## Recommendation

Mark fields with proto3 `[deprecated = true]` option and add a comment noting when they can be removed (next major version).
