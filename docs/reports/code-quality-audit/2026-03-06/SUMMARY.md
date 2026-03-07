# Code Quality Audit Report — 2026-03-06

## Scope

Full audit of the Banyan codebase (`feat/code-audit` branch, commit `dac0cbe`).

## Methodology

Systematic review against Banyan component map and quality checklist.
Go tooling: `go vet`, `go build`, `go mod tidy`, `golangci-lint` results.
Serena MCP server used for codebase exploration and symbol analysis.
4 parallel audit agents covering: Doc-Code Integrity, Integration/Dead Code, Code Smells/Consistency, Test Quality.

## Remediation Summary

| Status | Count | Details |
|--------|-------|---------|
| **FIXED** | 12 | Code/doc changes applied, tests pass |
| **ACKNOWLEDGED** | 4 | Valid findings, deferred due to risk/scope |
| **DEFERRED** | 0 | |
| **Total** | **16** | |

## Findings Summary

| Severity | DOC | INT | DEAD | SMELL | TEST | CONS | BUILD | Total |
|----------|-----|-----|------|-------|------|------|-------|-------|
| Critical |  1  |     |      |       |      |      |       |  **1**  |
| High     |  1  |  1  |      |   3   |  2   |      |       |  **7**  |
| Medium   |     |     |      |   1   |  2   |  2   |  2    |  **7**  |
| Low      |     |     |  1   |       |      |      |       |  **1**  |
| **Total**| **2** | **1** | **1** | **4** | **4** | **2** | **2** | **16** |

## Critical Findings

| ID | Title | Status |
|----|-------|--------|
| [CRIT-DOC-001](findings/CRIT-DOC-001-per-service-deploy-strategy-contradiction.md) | Per-service deployment strategy contradiction | **FIXED** |

## High Findings

| ID | Title | Status |
|----|-------|--------|
| [HIGH-DOC-001](findings/HIGH-DOC-001-env-file-missing-from-manifest-reference.md) | `env_file` missing from manifest reference | **FIXED** |
| [HIGH-INT-001](findings/HIGH-INT-001-unused-package-shared-domain.md) | `pkg/shared/domain` dead code | **FIXED** (deleted) |
| [HIGH-SMELL-001](findings/HIGH-SMELL-001-ignored-errors-network-operations.md) | Ignored errors on network ops | **FIXED** (now logged) |
| [HIGH-SMELL-002](findings/HIGH-SMELL-002-inconsistent-error-wrapping.md) | Inconsistent `%v` vs `%w` wrapping | **FIXED** |
| [HIGH-SMELL-003](findings/HIGH-SMELL-003-function-with-7-parameters.md) | `Register()` 7 parameters | **FIXED** (RegisterRequest struct) |
| [HIGH-TEST-001](findings/HIGH-TEST-001-untested-log-provider.md) | `log_provider.go` untested | **FIXED** (tests added) |
| [HIGH-TEST-002](findings/HIGH-TEST-002-flaky-time-sleep-tests.md) | `time.Sleep` in tests | ACKNOWLEDGED |

## Medium Findings

| ID | Title | Status |
|----|-------|--------|
| [MED-SMELL-001](findings/MED-SMELL-001-time-sleep-in-production.md) | `time.Sleep` in production code | **FIXED** (polling loop) |
| [MED-CONS-001](findings/MED-CONS-001-naming-inconsistency-node-vs-agent.md) | NodeName vs AgentName | **FIXED** |
| [MED-CONS-002](findings/MED-CONS-002-logging-pattern-inconsistency.md) | Logging pattern inconsistency | ACKNOWLEDGED |
| [MED-BUILD-001](findings/MED-BUILD-001-go-mod-tidy-diff.md) | `go mod tidy` diff | **FIXED** |
| [MED-BUILD-002](findings/MED-BUILD-002-golangci-lint-findings.md) | golangci-lint findings | **FIXED** (errcheck, gosec, gofmt) |
| [MED-TEST-001](findings/MED-TEST-001-untested-deployment-functions.md) | Deployment functions undertested | ACKNOWLEDGED |
| [MED-TEST-002](findings/MED-TEST-002-skipped-tests-without-ci-alternative.md) | Skipped infra-dependent tests | ACKNOWLEDGED |

## Low Findings

| ID | Title | Status |
|----|-------|--------|
| [LOW-DEAD-001](findings/LOW-DEAD-001-deprecated-proto-fields.md) | Deprecated proto fields | **FIXED** (proper annotation) |

## Changes Made

### Documentation
1. Fixed redeployment guide: "recreate" → "blue-green" for per-service deploys
2. Added `env_file` section to manifest reference (field table, structure, examples, merge order)
3. Added `env_file` row to Docker Compose comparison table

### Dead Code Removal
4. Removed `pkg/shared/domain/` (5 source files, 4 test files, 0 imports)
5. Removed `./pkg/shared/domain` from `go.work`

### Code Quality
6. Fixed 4 errcheck violations: DNS server stop, proxy backend removal, event WAL writes, etcd KeepAlive put
7. Fixed gosec WriteFile: `pkg/storage/memory.go` 0644→0600; added nolint comments for intentional sysctl/CNI permissions
8. Applied `goimports -w` across all packages
9. Replaced silently ignored errors (`_ =`) with logged errors in `pkg/vpc/overlay/` (control_tunnel.go, wireguard.go)
10. Standardized error wrapping: `%v` → `%w` in `pkg/agent/agent.go`
11. Refactored `EngineClient.Register()` from 7 params → `RegisterRequest` struct
12. Replaced `time.Sleep(200ms)` with process exit polling loop in `vpc_networking.go`
13. Added `[deprecated = true]` proto3 option to 3 deprecated fields

### Tests
14. Created `pkg/agent/log_provider_test.go` with tests for `cmdReadCloser` (read, close, kill, already-exited)

### Build Health
15. Ran `go mod tidy` in `pkg/agent/`

## Verification

All 16 modules build and pass tests:
```
pkg/agent         ok  3.6s
pkg/engine        ok  6.1s
pkg/logging       ok  0.0s
pkg/metrics       ok  0.0s
pkg/proxy         ok  0.0s
pkg/rpc           ok  0.0s
pkg/storage       ok  6.2s
pkg/types         ok  0.0s
pkg/vpc           ok  (3 sub-packages)
pkg/vpc/overlay   ok  0.0s
cmd/banyan-agent  ok  0.0s
cmd/banyan-cli    ok  21.1s
cmd/banyan-engine ok  4.0s
```

## Packages Reviewed

| Package | Status | Post-Audit |
|---------|--------|------------|
| `pkg/engine` | Integrated, tested | errcheck fixed, gofmt applied |
| `pkg/agent` | Integrated, tested | Register refactored, log_provider tested, go.mod tidied |
| `pkg/rpc` | Integrated, tested | Clean |
| `pkg/types` | Integrated, tested | Clean |
| `pkg/storage` | Integrated, tested | errcheck fixed, gosec fixed |
| `pkg/vpc` | Integrated, tested | goimports applied |
| `pkg/vpc/overlay` | Integrated, tested | Ignored errors now logged |
| `pkg/logging` | Integrated, tested | gofmt applied |
| `pkg/metrics` | Integrated, tested | Clean |
| `pkg/proxy` | Integrated, tested | gosec nolint documented |
| `pkg/shared/domain` | **DELETED** | Was dead code |
| `cmd/banyan-engine` | Compiles, tested | Clean |
| `cmd/banyan-agent` | Compiles, tested | Clean |
| `cmd/banyan-cli` | Compiles, tested | Clean |

## Doc Pages Verified

| Doc Page | Status | Post-Audit |
|----------|--------|------------|
| `index.mdx` | Pass | No changes needed |
| `README.md` | Pass | No changes needed |
| `getting-started/quickstart.md` | Pass | No changes needed |
| `getting-started/installation.md` | Pass | No changes needed |
| `reference/cli.md` | Pass | No changes needed |
| `reference/manifest.md` | **Pass** | `env_file` field added |
| `roadmap.md` | Pass | No changes needed |
| `guides/redeployment.md` | **Pass** | Strategy claim fixed |
| `guides/authentication.md` | Pass | No changes needed |
| `guides/multi-node.md` | Pass | No changes needed |

## Build Health (Post-Remediation)

| Check | Result |
|-------|--------|
| `go build ./...` | All modules compile |
| `go vet ./...` | 1 remaining: IPv6 format in `pkg/vpc/debug/manager.go:415` |
| `go mod tidy` | All modules clean |
| `golangci-lint` | errcheck, gosec, gofmt fixed; fieldalignment and style warnings remain |
| License | Apache 2.0 confirmed |
| Tests | All pass |

---

**Audit Date**: 2026-03-06
**Branch**: `feat/code-audit`
**Status**: Complete — 16 findings: 12 fixed, 4 acknowledged
