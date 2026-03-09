# [MED-BUILD-003] golangci-lint Findings Summary

**Severity**: Medium
**Category**: BUILD
**Component**: Multiple packages
**File(s)**: See details below

## Description

`golangci-lint` reports multiple issues across the codebase. Grouped by category:

### Unchecked Errors (errcheck) — 5 findings
| File | Line | Issue |
|------|------|-------|
| `pkg/agent/agent.go` | 205 | `a.dnsServer.Stop()` return value not checked |
| `pkg/agent/agent.go` | 253 | `a.proxy.RemoveBackend()` return value not checked |
| `pkg/engine/events.go` | 136 | `s.file.Write()` return value not checked |
| `pkg/engine/events.go` | 206 | `f.Write()` return value not checked |
| `pkg/storage/etcd.go` | 348 | `s.client.Put()` return value not checked |

### Security Issues (gosec) — 5 findings
| File | Line | Issue |
|------|------|-------|
| `pkg/agent/vpc_networking.go` | 395 | WriteFile with 0644 permissions (should be 0600) |
| `pkg/proxy/proxy.go` | 26 | WriteFile with 0644 permissions |
| `pkg/storage/memory.go` | 68 | WriteFile with 0644 permissions |
| `pkg/vpc/overlay/wireguard.go` | 171 | WriteFile with 0644 permissions |
| `pkg/vpc/cni/runtime.go` | 94 | Subprocess with potential tainted input |

### Formatting Issues (gofmt/goimports) — 8 findings
Multiple files have formatting issues: `pkg/agent/agent.go`, `pkg/engine/engine.go`, `pkg/logging/logger.go`, `pkg/proxy/iptables_noop.go`, `pkg/vpc/overlay/allocator.go`, `pkg/vpc/overlay/wireguard.go`, `pkg/vpc/cni/runtime.go`, `pkg/vpc/debug/manager.go`, `pkg/vpc/dns/manager.go`

### Struct Alignment (govet/fieldalignment) — 18 findings
Multiple structs across the codebase have suboptimal field ordering for memory alignment.

### Code Style (gocritic/gosimple) — 8 findings
- `rangeValCopy`: 3 instances of copying 136-byte structs in range loops
- `unlambda`: 2 unnecessary wrapper lambdas in vpc_networking.go
- `paramTypeCombine`: 3 instances of combinable parameter types
- `sprintfQuotedString`: 1 use of `"%s"` instead of `%q`
- `hugeParam`: 2 instances of passing large structs by value

### Unused/Dead Code — 2 findings
| File | Line | Issue |
|------|------|-------|
| `pkg/proxy/proxy.go` | 193 | `cleanupStaleChains()` always returns nil |
| `pkg/proxy/proxy.go` | 345 | Commented-out code detected |

## Impact

- **Security**: File permissions too permissive (should be 0600 for sensitive configs)
- **Performance**: Struct copying in hot loops, suboptimal memory alignment
- **Maintenance**: Formatting inconsistencies, unnecessary wrappers
- **Correctness**: Unchecked errors could cause silent data loss

## Recommendation

1. **Immediate**: Fix errcheck findings (unchecked errors on Write/Stop/Put)
2. **Immediate**: Fix gosec file permissions to 0600
3. **Soon**: Run `gofmt -w` on all files with formatting issues
4. **Later**: Address fieldalignment and gocritic suggestions
