# [HIGH-SMELL-001] Ignored Errors on Critical Network Operations

**Severity**: High
**Category**: SMELL
**Component**: pkg/vpc/overlay
**File(s)**: `pkg/vpc/overlay/control_tunnel.go:18-19`, `pkg/vpc/overlay/wireguard.go:66-67`

## Description

Critical network setup operations silently ignore errors from `LinkExists()` and `DeleteLink()` calls. If these operations fail (permissions, kernel module issues), the code continues as if everything is fine.

## Evidence

**control_tunnel.go:18-19:**
```go
if exists, _ := linkOps.LinkExists(iface); exists {
    _ = linkOps.DeleteLink(iface)
}
```

**wireguard.go:66-67:**
```go
if exists, _ := d.linkOps.LinkExists(d.wgName); exists {
    _ = d.linkOps.DeleteLink(d.wgName)
}
```

Both patterns ignore the error from `LinkExists` (could mask permission issues) and `DeleteLink` (could leave stale interfaces).

## Impact

- **System impact**: Stale network interfaces could persist, causing routing conflicts
- **Debug impact**: Silent failures make network issues extremely hard to diagnose
- **Security impact**: Stale WireGuard interfaces could maintain unintended tunnels

## Recommendation

Log errors even if they don't prevent continuation:
```go
if exists, err := linkOps.LinkExists(iface); err != nil {
    log.Warn("Failed to check interface existence", "iface", iface, "error", err)
} else if exists {
    if err := linkOps.DeleteLink(iface); err != nil {
        log.Warn("Failed to delete existing interface", "iface", iface, "error", err)
    }
}
```
