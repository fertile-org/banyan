# [MED-BUILD-002] golangci-lint Reports Multiple Issues

**Severity**: Medium
**Category**: BUILD
**Component**: Multiple packages
**File(s)**: Various (see below)

## Description

`golangci-lint run ./...` reports issues across multiple packages. While many are style (fieldalignment, gofmt), some are substantive (errcheck, gosec).

## Evidence

### Unchecked Errors (errcheck)
- `pkg/agent/agent.go:205` — `a.dnsServer.Stop()` return not checked
- `pkg/agent/agent.go:253` — `a.proxy.RemoveBackend()` return not checked
- `pkg/engine/events.go:136,206` — `s.file.Write()` / `f.Write()` returns not checked
- `pkg/storage/etcd.go:348` — `s.client.Put()` return not checked

### Security Issues (gosec)
- `pkg/agent/vpc_networking.go:395` — WriteFile with 0o644 (should be 0o600 or less)
- `pkg/proxy/proxy.go:26` — WriteFile with 0o644
- `pkg/storage/memory.go:68` — WriteFile with 0644
- `pkg/vpc/overlay/wireguard.go:171` — WriteFile with 0o644
- `pkg/vpc/cni/runtime.go:94` — Subprocess with potential tainted input (G204)
- `pkg/vpc/overlay/allocator.go:97` — Integer overflow conversion (G115)

### Formatting Issues (gofmt/goimports)
- `pkg/agent/agent.go:50` — Not properly formatted
- `pkg/agent/vpc_networking.go:14` — Import not formatted
- `pkg/engine/engine.go:24` — Not properly formatted
- `pkg/logging/logger.go:14` — Not properly formatted
- `pkg/proxy/iptables_noop.go:10` — Not properly formatted
- `pkg/vpc/overlay/allocator.go:14` — Not properly formatted
- `pkg/vpc/overlay/wireguard.go:15` — Not properly formatted
- `pkg/vpc/cni/runtime.go:12`, `debug/manager.go:11`, `dns/manager.go:10` — Not properly formatted

### Static Analysis (govet)
- `pkg/vpc/debug/manager.go:415` — IPv6 format issue (`%s:%d` doesn't work with IPv6)
- Multiple `fieldalignment` warnings across all packages
- `pkg/vpc/overlay/wireguard_keys.go:17` — Variable shadowing

### Code Improvements (gocritic, gosimple)
- `pkg/agent/vpc_networking.go:25-26` — Unnecessary lambda wrappers (unlambda)
- `pkg/engine/events.go:102,129` — Should use type conversion (S1016)
- `pkg/engine/engine.go:340`, `grpc_server.go:635,1367` — Range value copies 136 bytes
- `pkg/proxy/proxy.go:345` — Commented-out code detected
- `pkg/proxy/proxy.go:193` — Result always nil (unparam)

## Impact

- **errcheck**: Silently ignored errors could mask failures in DNS cleanup, proxy cleanup, event logging, and etcd writes
- **gosec**: Overly permissive file permissions and potential command injection
- **gofmt**: Code formatting inconsistency
- **govet**: IPv6 incompatibility in debug manager; variable shadowing could cause bugs

## Recommendation

1. Fix all errcheck issues (log or handle error returns)
2. Fix gosec WriteFile permissions to 0o600
3. Run `gofmt -w` across all packages
4. Fix the IPv6 format issue in debug manager
5. Address variable shadowing in wireguard_keys.go
