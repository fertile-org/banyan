# [HIGH-DEAD-001] pkg/types/api.go types unused in production

**Severity**: High
**Category**: DEAD
**Component**: pkg/types
**File(s)**: `pkg/types/api.go:1-53`

## Description

The file `pkg/types/api.go` defines several types that appear to be API request/response structures but are never used in any production code:

- `DeployRequest`
- `DeployResponse`
- `DownRequest`
- `DownResponse`
- `StatusResponse`
- `DeploymentStatus`
- `InfoResponse`
- `HealthResponse`
- `ErrorResponse`

## Evidence

**Search for usage of api.go types:**

```bash
$ grep -r "DeployRequest\|DeployResponse\|DownRequest\|DownResponse\|StatusResponse\|DeploymentStatus\|InfoResponse\|HealthResponse\|ErrorResponse" /home/work/freelancer/banyan/pkg/ /home/work/freelancer/banyan/cmd/ --include="*.go" | grep -v "_test.go" | grep -v "^pkg/types/api.go"
# No production usage found
```

The types are only defined in `api.go` but never referenced by any production code. They appear to be vestigial code from an older API implementation.

## Impact

**Developer impact**: These types add cognitive load — anyone reading the codebase might expect them to be used somewhere. They create confusion about the actual API structure.

**Maintenance impact**: Dead types that are never tested or used can become outdated and potentially misleading if the API changes but these types don't.

## Recommendation

1. **If these types are truly dead**: Delete `pkg/types/api.go` entirely. They serve no purpose.
2. **If they were intended for a future REST API**: Move them to a separate file or add a comment explaining they are for a planned API. Better yet, don't define types for features that don't exist yet.

The simplest fix is to delete the file since the actual gRPC API uses proto-generated types in `pkg/rpc/banyanpb/`, not these Go struct types.