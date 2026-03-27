# INFO-002: ResolveSecrets error message lists missing secret names

**Severity**: Informational
**Responsibility**: Observation
**Component**: Engine — Secrets Manager
**File(s)**: `pkg/engine/secrets.go:207`

## Description

When `ResolveSecrets()` fails, the error lists the missing secret names: `"secrets not found: DB_PASSWORD, API_KEY"`. This is logged by the PollTasks handler and could appear in engine logs.

## Impact

Secret names (not values) visible in engine logs. Names are not themselves sensitive, but listing them reveals what secrets exist in the cluster.

## Recommendation

Acceptable for v1 — the information helps debugging. In a future RBAC implementation, consider whether agents should see which secrets they're missing access to.
