# [MED-BUILD-002] go vet: IPv6-Incompatible Address Format

**Severity**: Medium
**Category**: BUILD
**Component**: pkg/vpc/debug
**File(s)**: `pkg/vpc/debug/manager.go:415`

## Description

`go vet` reports an IPv6-incompatible address format string passed to `net.Dial`.

## Evidence

```
debug/manager.go:415:25: address format "%s:%d" does not work with IPv6
(passed to net.Dial at L416)
```

The format `"%s:%d"` produces `host:port` which is ambiguous for IPv6 addresses. Should use `net.JoinHostPort()` instead.

## Impact

- **Functional**: Debug connectivity checks will fail for IPv6 addresses
- **Build**: `go vet` failure in CI pipelines

## Recommendation

Replace `fmt.Sprintf("%s:%d", host, port)` with `net.JoinHostPort(host, strconv.Itoa(port))`.
