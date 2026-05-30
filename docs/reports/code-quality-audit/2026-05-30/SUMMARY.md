# Code Quality Audit Report — 2026-05-30

## Scope

Full audit of the Banyan codebase. Focus on doc-code integrity, integration paths, dead code, code smells, and consistency.

## Methodology

Systematic review against Banyan component map and quality checklist.
Parallel audit agents covering: Doc-Code Integrity, Integration, Dead Code, Code Smells, Test Quality.

Go tooling: `go build` (verified all 3 binaries compile), workspace structure verified.

## Prior Audit Comparison (2026-03-27)

| Status | Count | Details |
|--------|-------|---------|
| **Previously Fixed** | 22 | All critical and high fixes from 2026-03-06 remain in place |
| **Still Open** | 2 | God files (grpc_server.go grew from ~1500 to 2128 lines), Agent/Node naming inconsistency |
| **New Issues Found** | 5 | See findings below |

## Findings Summary

| Severity | DOC | INT | DEAD | SMELL | TEST | CONS | BUILD | Total |
|----------|-----|-----|------|-------|------|------|-------|-------|
| Critical |  0  |  1  |  0   |   0   |  0   |  0   |   0   |   1   |
| High     |  1  |  0  |  1   |   0   |  0   |  0   |   0   |   2   |
| Medium   |  0  |  0  |  0   |   2   |  0   |  1   |   0   |   3   |
| Low      |  0  |  0  |  0   |   0   |  0   |  0   |   0   |   0   |
| Info     |  0  |  0  |  1   |   0   |  0   |  1   |   0   |   2   |
| **Total**|**1**|**1**|**2** | **2** |**0** |**2** | **0** | **8** |

## Critical Findings

| ID | Title | Category |
|----|-------|----------|
| [CRIT-INT-001](findings/CRIT-INT-001-vpc-initialize-not-called.md) | vpc.InitializeNetwork() never called in engine/agent startup | INT |

## High Findings

| ID | Title | Category |
|----|-------|----------|
| [HIGH-DOC-001](findings/HIGH-DOC-001-status-command-missing.md) | `status` command documented in CLI reference but missing from implementation | DOC |
| [HIGH-DEAD-001](findings/HIGH-DEAD-001-types-api-unused.md) | pkg/types/api.go types unused in production | DEAD |

## Medium Findings

| ID | Title | Category |
|----|-------|----------|
| [MED-SMELL-001](findings/MED-SMELL-001-agents-nodes-inconsistency.md) | Agent/Node naming inconsistency across codebase | CONS |
| [MED-SMELL-002](findings/MED-SMELL-002-rpc-handler-documentation.md) | Auth RPC handlers lack dedicated tests | SMELL |
| [MED-SMELL-003](findings/MED-SMELL-003-web-handler-rpcs-lack-tests.md) | Web dashboard RPC handlers lack direct tests | SMELL |

## Low Findings

None

## Informational Findings

| ID | Title | Category |
|----|-------|----------|
| [INFO-DEAD-001](findings/INFO-DEAD-001-god-file-grpc-server.md) | grpc_server.go at ~2000+ lines (2128 reported Mar 2026, still large) | DEAD |
| [INFO-CONS-001](findings/INFO-CONS-001-previous-audit-items.md) | Previous audit items still open: god file, naming inconsistency | CONS |

## Top Recommendations

1. **Integrate VPC initialization** (CRIT-INT-001) — `vpc.InitializeNetwork()` is only called in integration tests. The engine uses `overlay.NewSubnetAllocator()` directly instead. Decide if the old function is deprecated or needs to be wired in.
2. **Add `status` command** (HIGH-DOC-001) — CLI reference documents `banyan-cli status` but it doesn't exist. The `engine` command provides similar functionality, but docs are inaccurate.
3. **Remove unused api.go types** (HIGH-DEAD-001) — `DeployRequest`, `DeployResponse`, `DownRequest`, `DownResponse`, `StatusResponse`, `DeploymentStatus`, `InfoResponse`, `HealthResponse`, `ErrorResponse` are defined but never used in production code.
4. **Add tests for auth handlers** (MED-SMELL-002) — Login, ListUsers, CreateUser, DeleteUser, ChangePassword have no dedicated tests.
5. **Add tests for web handlers** (MED-SMELL-003) — GetClusterOverview, ListAgents, ListDeployments, GetDeployment, GetServiceLogs, StreamAgentLogs only tested indirectly via TestGetDashboardData.

## Packages Reviewed

| Package | Status | Notes |
|---------|--------|-------|
| pkg/engine | Reviewed | grpc_server.go is large but all handlers present |
| pkg/agent | Reviewed | Clean implementation, good coverage |
| pkg/types | Reviewed | api.go has unused types |
| pkg/rpc | Reviewed | All RPCs have handlers |
| pkg/storage | Reviewed | Clean, factory pattern |
| pkg/vpc | Reviewed | overlay used, root package has unused InitializeNetwork |
| pkg/vpc/overlay | Reviewed | Well tested, proper WireGuard integration |
| pkg/metrics | Reviewed | Clean |
| pkg/proxy | Reviewed | Clean |
| pkg/logging | Reviewed | Clean |
| cmd/banyan-engine | Reviewed | Proper startup path |
| cmd/banyan-agent | Reviewed | Proper startup path |
| cmd/banyan-cli | Reviewed | Missing status command |

## Doc Pages Verified

| Doc Page | Status | Notes |
|----------|--------|-------|
| index.mdx (homepage) | PASS | All 9 feature cards verified |
| roadmap.md | PASS | M1-M12 marked Done, verified |
| reference/cli.md | FAIL | Documents `status` command that doesn't exist |
| reference/manifest.md | PASS | All fields exist |
| getting-started/quickstart.md | PASS | Commands and workflow accurate |
| getting-started/installation.md | PASS | Install process accurate |
| guides/authentication.md | PASS | WireGuard tunnel claims verified |
| guides/secrets.md | PASS | AES-256-GCM claims verified |
| guides/auto-scaling.md | PASS | Autoscale implementation verified |
| guides/high-availability.md | PASS | Multi-engine claims verified |

## Go Build Verification

All three binaries compile successfully:
- `banyan-engine` ✓
- `banyan-agent` ✓
- `banyan-cli` ✓

## gRPC Handler Coverage

**33/33 RPCs have handlers (100% coverage)**

| Service | RPCs | Covered |
|---------|------|---------|
| EngineService | 31 | 100% |
| AgentService | 1 | 100% |