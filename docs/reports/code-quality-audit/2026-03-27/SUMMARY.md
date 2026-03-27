# Code Quality Audit Report — 2026-03-27

## Scope

Full audit of the Banyan codebase (`feat/audit-20260327` branch). Focus on new features since 2026-03-06 audit: Auto-Scaling (M9), Secrets Management (M11), and security audit fixes.

## Methodology

Systematic review against Banyan component map and quality checklist.
4 parallel audit agents covering: Doc-Code Integrity, Dead Code & Integration, Code Smells & Consistency, Previous Findings.

## Prior Audit Comparison (2026-03-06)

| Status | Count | Details |
|--------|-------|---------|
| **Previously Fixed** | 22 | All critical and high fixes from 2026-03-06 remain in place |
| **Still Open** | 2 | God files (grpc_server.go grew from ~1500 to 2128 lines), Agent/Node naming inconsistency |

## Findings Summary

| Severity | DOC | INT | DEAD | SMELL | TEST | CONS | BUILD | Total |
|----------|-----|-----|------|-------|------|------|-------|-------|
| Critical |  0  |  0  |  0   |   1   |  0   |  0   |   0   |   1   |
| High     |  0  |  0  |  0   |   2   |  0   |  0   |   0   |   2   |
| Medium   |  0  |  0  |  0   |   0   |  0   |  1   |   0   |   1   |
| Low      |  0  |  0  |  1   |   0   |  0   |  0   |   0   |   1   |
| Info     |  0  |  0  |  0   |   1   |  0   |  0   |   0   |   1   |
| **Total**|**0**|**0**|**1** | **4** |**0** |**1** | **0** | **6** |

## Critical Findings

| ID | Title | Category |
|----|-------|----------|
| [CRIT-SMELL-001](findings/CRIT-SMELL-001-rebalance-cooldown-race.md) | Race condition: rebalanceMigrationCooldown map unprotected | SMELL |

## High Findings

| ID | Title | Category |
|----|-------|----------|
| [HIGH-SMELL-001](findings/HIGH-SMELL-001-autoscale-swallowed-errors.md) | Autoscale silently ignores store.Save errors for critical operations | SMELL |
| [HIGH-SMELL-002](findings/HIGH-SMELL-002-duplicated-magic-timeouts.md) | Agent staleness timeout (60s) duplicated across 3 files | SMELL |

## Medium Findings

| ID | Title | Category |
|----|-------|----------|
| [MED-CONS-001](findings/MED-CONS-001-agent-vs-node-naming.md) | "Agent" vs "Node" naming inconsistency between RPC and storage layers | CONS |

## Low Findings

| ID | Title | Category |
|----|-------|----------|
| [LOW-DEAD-001](findings/LOW-DEAD-001-stale-temp-file.md) | Leftover temp file: agent.go.tmp.14377.1773508998865 | DEAD |

## Informational

| ID | Title | Category |
|----|-------|----------|
| [INFO-SMELL-001](findings/INFO-SMELL-001-god-file-grpc-server.md) | grpc_server.go at 2,128 lines — growing but still manageable | SMELL |

## Top Recommendations

1. **Fix race condition** (CRIT-SMELL-001) — Add `sync.Mutex` to protect `rebalanceMigrationCooldown` map in autoscale.go
2. **Log autoscale save errors** (HIGH-SMELL-001) — Replace `_ = e.store.Save(...)` with error logging in autoscale.go
3. **Extract timeout constants** (HIGH-SMELL-002) — Define `AgentStalenessThreshold`, `DeploymentLockTimeout` as package constants
4. **Delete temp file** (LOW-DEAD-001) — Remove `cmd/banyan-agent/cmd/agent.go.tmp.*`

## Doc-Code Integrity

**All documentation claims verified.** No mismatches found.

| Doc Page | Status |
|----------|--------|
| index.mdx (homepage) | PASS — all 9 feature cards verified |
| roadmap.md | PASS — M1-M9, M11 marked Done, all verified |
| reference/manifest.md | PASS — all fields exist in ManifestService |
| reference/cli.md | PASS — all 13 commands registered |
| guides/secrets.md | PASS — AES-256-GCM, --reveal, no-persist claims all verified |
| guides/auto-scaling.md | PASS — evaluateAutoscale, scaleService both integrated |
| blog/from-one-server-to-many.md | PASS — comparison table and roadmap updated |

## Packages Reviewed

| Package | Status | Notes |
|---------|--------|-------|
| pkg/engine | Reviewed | grpc_server.go is large but all handlers present |
| pkg/engine (secrets.go) | Reviewed | All exports used, encryption correct |
| pkg/engine (autoscale.go) | Reviewed | Race condition found, swallowed errors |
| pkg/agent | Reviewed | Clean, secrets env-file approach correct |
| pkg/types | Reviewed | All fields parsed and used |
| pkg/rpc | Reviewed | All 17 RPCs have handlers |
| pkg/vpc/dns | Reviewed | Default bind fixed to 127.0.0.1 |
| pkg/storage | Reviewed | TLS config correct |
| cmd/banyan-engine | Reviewed | Init wizard handles secrets key correctly |
| cmd/banyan-cli | Reviewed | All commands registered |
