# [CRIT-DOC-001] Per-Service Deployment Strategy Contradiction

**Status**: FIXED (2026-03-06)
**Severity**: Critical
**Category**: DOC
**Component**: website/docs, pkg/engine
**File(s)**: `website/src/content/docs/guides/redeployment.md:74`, `website/src/content/docs/reference/cli.md:237`, `pkg/engine/grpc_server.go`

## Description

Three sources in the project give contradictory information about which deployment strategy per-service deploys use:

1. **Redeployment guide** says: "Per-service deploys use a **recreate** strategy (stop old, then start new) instead of blue-green."
2. **CLI reference** says: "Per-service deploys use the same blue-green strategy as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy."
3. **Code** says: `deployServices` handles per-service redeployment using the blue-green strategy.

## Evidence

- `website/src/content/docs/guides/redeployment.md` line 74 claims **recreate** strategy
- `website/src/content/docs/reference/cli.md` line 237 claims **blue-green** strategy
- `pkg/engine/grpc_server.go` function `deployServices()` implements **blue-green** strategy

The code and CLI reference agree (blue-green). The redeployment guide is wrong.

## Impact

- **User impact**: A user reading the redeployment guide will expect downtime during per-service deploys (recreate = stop-then-start). They may build deployment scripts or communicate deployment windows to stakeholders based on this incorrect assumption.
- **Trust impact**: When the behavior doesn't match the docs, users lose confidence in all documentation.
- **Internal contradiction**: Two official docs disagree with each other, making the project look careless.

## Recommendation

Update `website/src/content/docs/guides/redeployment.md` line 74 to match the CLI reference and code:

> "Per-service deploys use the **same blue-green strategy** as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy."
