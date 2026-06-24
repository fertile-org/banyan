# VPC CIDR Auto-Detection & Configuration — Design Spec

**Date:** 2026-06-18
**Status:** Approved (pending spec review)

## Problem

After initializing an agent and linking it to the engine, checking cluster status
fails with a `VPC CIDR conflict` error. Root cause analysis of the codebase:

- The default VPC CIDR is hardcoded to `10.0.0.0/16` (`cmd/banyan-engine/cmd/engine.go:121,131`).
- On startup, `checkCIDRConflict()` (`pkg/engine/engine.go:1067`) compares the VPC CIDR
  against every host network interface (excluding Banyan-managed interfaces). If they
  overlap, the engine refuses to start with `VPC CIDR conflict` (`engine.go:189-190`).
- Cloud VMs (e.g. Oracle Cloud Infrastructure) very often place the host's private
  subnet inside `10.0.0.0/16`, which collides with the default.

Two underlying gaps:

1. **The `init` wizard never prompts for the VPC CIDR.** It only exposes a `--vpc-cidr`
   flag. A normal interactive init silently uses the default and only surfaces the
   conflict later, at `start`.
2. **The VPC CIDR is never persisted.** `EngineConfig` has no VPC CIDR field, and
   `start` reads the value purely from the `--vpc-cidr` flag (default `10.0.0.0/16`).
   Even passing `--vpc-cidr` to `init` is lost — `start` always reverts to the default.

## Goal

Proactively detect a conflict-free private CIDR, let the user see and configure it
during `init`, and persist the choice so `start` uses it. Improve the conflict error
message to suggest a free range.

## Non-Goals (YAGNI)

- No changes to per-agent IPAM / subnet allocation logic.
- No web UI.
- No silent, automatic CIDR changes on a running engine.

## Design

### 1. Persist VPC CIDR in config

Add a field to `EngineConfig` (`pkg/types/config.go`):

```go
VPCCIDR string `yaml:"vpc_cidr,omitempty"`
```

- `init` writes the chosen range to `cfg.Engine.VPCCIDR`.
- `start` resolves the VPC CIDR with this precedence:
  1. `--vpc-cidr` flag, only when explicitly set by the user
     (detected via `cmd.Flags().Changed("vpc-cidr")`, not by comparing to the default).
  2. `cfg.Engine.VPCCIDR` from the config file.
  3. Default `10.0.0.0/16`.

### 2. Free-CIDR detection helper

In `pkg/engine`, add pure, independently testable functions:

```go
// Candidate ranges, tried in order (10.x then 172.x).
var candidateCIDRs = []string{
    "10.10.0.0/16", "10.20.0.0/16", "10.30.0.0/16",
    "10.50.0.0/16", "10.100.0.0/16",
    "172.20.0.0/16", "172.30.0.0/16",
}

// suggestFreeCIDR returns the first candidate that does not overlap any host
// interface, or "" if all candidates conflict.
func suggestFreeCIDR() (string, error)
```

- Reuses the existing `checkCIDRConflict()` / `listInterfaceAddrs()` mechanism (compare
  against host interfaces, skip Banyan-managed interfaces). Refactor only to allow
  checking each candidate; **do not change the existing check behavior**.
- The pool deliberately excludes `10.0.0.0/16` (common cloud collision) and
  `10.200.0.0/16` (reserved for the control tunnel, see `pkg/types/config.go:16`).

Selection logic:
1. Iterate `candidateCIDRs`, return the first non-conflicting one.
2. If none are free, return `""`; callers handle this (wizard falls back to manual
   entry, non-interactive returns an error).

### 3. Wizard `init` + non-interactive behavior

**Interactive** (always prompt, pre-filled with a detected safe range):
Add a step in `runEngineInit`, after WireGuard keypair generation:

1. Call `suggestFreeCIDR()` to get the suggested range.
2. Show a `huh` input with:
   - Title: "VPC CIDR (internal network range for containers)".
   - Default value: the suggested range (e.g. `10.10.0.0/16`). If `suggestFreeCIDR`
     returns `""`, leave it empty with a note that no free range was found and the user
     must enter one manually.
   - Help text: list which host interfaces occupy which ranges, so the user understands
     why `10.0.0.0/16` is avoided.
3. Validate on input: parse the CIDR and run `checkCIDRConflict()` on the entered value.
   On conflict, show an inline error and require re-entry (never accept a conflicting
   range).
4. Save to `cfg.Engine.VPCCIDR`.

**Non-interactive** (`init --non-interactive`):
- If `--vpc-cidr` is provided: use it, validate it; on conflict return a clear error
  (including a suggested free range).
- If not provided: call `suggestFreeCIDR()`; if a free range exists, use it; otherwise
  return an error asking the user to specify `--vpc-cidr`.

### 4. Improved conflict error at `start`

When `checkCIDRConflict` fails at start, augment the message with a suggestion (reusing
`suggestFreeCIDR()`):

```
VPC CIDR 10.0.0.0/16 overlaps with host interface ens3 (10.0.0.5/24)
  → Suggested free range: 10.10.0.0/16
  → Fix: re-run `sudo banyan-engine init` to choose a range,
         or start with --vpc-cidr 10.10.0.0/16
```

## Testing

- Unit test `suggestFreeCIDR()`: mock `listInterfaceAddrsFunc` (already a swappable var)
  to cover: first candidate free; first few occupied; all occupied (returns `""`);
  Banyan-managed interfaces ignored.
- Unit test resolve precedence: flag > config > default.
- Config round-trip: `VPCCIDR` serializes/deserializes correctly in YAML.
- Keep the existing `TestCheckCIDRConflict` (behavior unchanged).

## Affected Files

| File | Change |
|------|--------|
| `pkg/types/config.go` | Add `VPCCIDR` field to `EngineConfig` |
| `pkg/engine/engine.go` | Add `suggestFreeCIDR()` + `candidateCIDRs`; refactor conflict check for reuse; improve error message |
| `cmd/banyan-engine/cmd/engine.go` | Add VPC CIDR step to `init` wizard; non-interactive handling; resolve precedence at `start` |
| `pkg/engine/engine_test.go` | Tests for `suggestFreeCIDR` and precedence |
