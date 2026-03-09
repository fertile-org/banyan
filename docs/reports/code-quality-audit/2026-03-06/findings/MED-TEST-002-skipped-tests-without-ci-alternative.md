# [MED-TEST-002] Infrastructure-Dependent Tests Skipped Without CI Alternative

**Status**: ACKNOWLEDGED — Tests require CNI plugins, non-loopback interfaces, and DNS port binding. These are intentionally skipped in constrained environments. Mock-based alternatives already exist for most logic; the skipped tests validate real system integration.
**Severity**: Medium
**Category**: TEST
**Component**: pkg/agent, pkg/types, pkg/vpc
**File(s)**: `pkg/agent/vpc_networking_test.go:62,650`, `pkg/types/config_test.go`, `pkg/vpc/ipam/manager_test.go`, `pkg/vpc/dns/manager_test.go`

## Description

Multiple tests use `t.Skip()` when environmental conditions aren't met (CNI plugins missing, non-loopback interfaces unavailable, DNS port binding). These tests never run in standard CI environments.

## Evidence

```go
// pkg/agent/vpc_networking_test.go:62
t.Skip("all CNI plugins present on this machine")

// pkg/agent/vpc_networking_test.go:650
// detectHostIP CI skip
```

## Impact

- Core networking tests don't run in CI, creating a gap between local dev testing and CI
- VPC networking was the previous source of a critical integration failure
- No automated validation of networking paths in containerized environments

## Recommendation

1. Create mock-based unit tests that run everywhere for logic validation
2. Tag infrastructure tests with `//go:build integration` and run them in CI with proper setup
3. Document which CI runner configuration is needed for full test coverage
