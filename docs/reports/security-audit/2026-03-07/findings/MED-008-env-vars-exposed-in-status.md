# [MED-008] Environment Variables Exposed in Status/Dashboard RPCs

**Severity**: Medium
**Status**: FIXED
**Responsibility**: Platform Issue
**Component**: Engine gRPC Server
**File(s)**:
- `pkg/engine/grpc_server.go:663` (GetStatus — service environment)
- `pkg/engine/grpc_server.go:683` (GetStatus — task environment)
- `pkg/engine/grpc_server.go:1424,1443` (GetDashboardData — same exposure)

## Description

Environment variables from deployment manifests (which commonly contain secrets like `DATABASE_URL`, `API_KEY`, `SECRET_KEY`) are returned verbatim in GetStatus and GetDashboardData responses.

Any authenticated CLI user can read all environment variables for all deployments. Combined with CRIT-003 (no-auth default), these are accessible to unauthenticated users.

## Recommendation

1. Omit environment variables from status/dashboard responses by default
2. Add a `--show-env` flag to CLI commands that need them
3. Consider masking values: `DATABASE_PASSWORD=****`

## Fix

Removed the `Environment` field from `ServiceInfo` and `TaskInfo` in both `GetStatus` and `GetDashboardData` responses in `pkg/engine/grpc_server.go`. Environment variables are no longer returned in any status or dashboard RPC response.
