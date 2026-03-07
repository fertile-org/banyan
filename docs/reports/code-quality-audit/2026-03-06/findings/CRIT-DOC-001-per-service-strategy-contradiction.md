# [CRIT-DOC-001] Per-Service Deployment Strategy Documentation Contradiction

**Severity**: Critical
**Category**: DOC
**Component**: website/docs, pkg/engine
**File(s)**: `website/src/content/docs/guides/redeployment.md:74`, `website/src/content/docs/reference/cli.md:237`, `pkg/engine/grpc_server.go`

## Description

Three documentation sources contradict each other about which deployment strategy is used for per-service deploys:

1. **Redeployment guide** (line 74): Claims per-service deploys use a **recreate** strategy (stop old, then start new)
2. **CLI reference** (line 237): Claims per-service deploys use the **same blue-green strategy** as full deploys
3. **Code** (`grpc_server.go` — `deployServices()`): Implements **blue-green strategy**

The redeployment guide actively lies to users.

## Evidence

**redeployment.md line 74:**
> "Per-service deploys use a **recreate** strategy (stop old, then start new) instead of blue-green."

**cli.md line 237:**
> "Per-service deploys use the same blue-green strategy as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy."

**grpc_server.go — deployServices():**
> Comment: "deployServices handles per-service redeployment using the blue-green strategy."

## Impact

- **User impact**: Users reading the redeployment guide expect downtime during per-service deploys. They may avoid per-service deploys for production services, not knowing blue-green is actually available.
- **Trust impact**: When users discover the contradiction, they lose trust in all documentation.
- **Operational impact**: Users planning maintenance windows based on incorrect strategy information.

## Recommendation

Update `website/src/content/docs/guides/redeployment.md` line 74 to match the code and CLI reference:
> "Per-service deploys use the **same blue-green strategy** as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy."
