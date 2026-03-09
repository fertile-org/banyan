# [HIGH-SMELL-001] Ignored Errors on Critical Network Operations

**Status**: FIXED (2026-03-06) — Errors are now logged instead of silently ignored
**Severity**: High
**Category**: SMELL
**Component**: pkg/vpc/overlay
**File(s)**: `pkg/vpc/overlay/control_tunnel.go:18-19`, `pkg/vpc/overlay/wireguard.go:66-67`

## Description

Error return values from `LinkExists()` and `DeleteLink()` are silently ignored during network interface setup. These are critical infrastructure operations where failure could leave stale interfaces or mask permission errors.

## Evidence

`pkg/vpc/overlay/control_tunnel.go:18-19`:
```go
if exists, _ := linkOps.LinkExists(iface); exists {
    _ = linkOps.DeleteLink(iface)
}
```

`pkg/vpc/overlay/wireguard.go:66-67`:
```go
if exists, _ := d.linkOps.LinkExists(d.wgName); exists {
    _ = d.linkOps.DeleteLink(d.wgName)
}
```

## Impact

- If `LinkExists` fails (e.g., due to permissions), the code assumes the link doesn't exist and may try to create a duplicate
- If `DeleteLink` fails, a stale interface remains but the code continues creating a new one, potentially causing conflicts
- Both are "best effort cleanup" but should at minimum log the error

## Recommendation

Log errors instead of ignoring:
```go
if exists, err := linkOps.LinkExists(iface); err != nil {
    log.Warn("failed to check interface existence", "iface", iface, "error", err)
} else if exists {
    if err := linkOps.DeleteLink(iface); err != nil {
        log.Warn("failed to delete stale interface", "iface", iface, "error", err)
    }
}
```
