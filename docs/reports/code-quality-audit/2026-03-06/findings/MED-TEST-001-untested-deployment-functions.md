# [MED-TEST-001] Engine Deployment Functions Lack Direct Unit Tests

**Status**: ACKNOWLEDGED — Functions are methods on engineGRPCServer requiring full server setup. Covered by existing integration tests. Recommend extracting pure logic into standalone functions when these methods are next modified.
**Severity**: Medium
**Category**: TEST
**Component**: pkg/engine
**File(s)**: `pkg/engine/grpc_server.go`

## Description

Several critical deployment orchestration functions are only tested indirectly through high-level integration tests:

- `prepareForRedeploy()` — Blue-green preparation
- `deployServices()` — Service deployment orchestration
- `findRunningDeploymentByName()` — Deployment lookup
- `getRunningServiceNames()` — Service filtering
- `teardownDeploymentServices()` — Selective teardown
- `collectServiceBackends()` — Load balancer backend collection (61 lines)

## Evidence

- `grpc_server_test.go` has Deploy/Down tests but they test the full RPC path
- No tests isolate individual helper functions
- Edge cases (concurrent deployments, partial failures, empty service lists) are not explicitly tested

## Impact

- Blue-green transition edge cases may have untested failure modes
- If integration tests pass for wrong reasons, underlying logic isn't validated
- Difficult to diagnose failures in specific helper functions

## Recommendation

Add targeted unit tests for each function with:
- Happy path
- Error/edge cases (no running deployment, partial service match, concurrent access)
- Table-driven tests with at least 3 cases each
