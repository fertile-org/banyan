# [INFO-001] Capacity validation compares against total memory, not available memory

**Severity**: Informational
**Responsibility**: User Responsibility
**Component**: Scheduler — Capacity validation
**File(s)**: `pkg/types/helpers.go:198-231`

## Description

`ValidateClusterCapacity` compares deployment resource requests against `totalMemory` (sum of all agents' `MemoryTotalBytes`) rather than available memory (total minus currently used). This means a deployment can pass the capacity check even when most cluster memory is already consumed by the OS and other workloads.

```go
if requestedMemory > totalMemory {
    return fmt.Errorf(...)
}
```

## Impact

A deployment that passes capacity validation may still fail at scheduling time if no individual agent has enough available memory. This is a reliability concern, not a security issue. The scheduler (`pickAgentByResources`) does account for used memory when picking agents, so containers still go to the best available node — but the overall deployment won't be rejected early if the cluster is nearly full.

## Recommendation

This is a design tradeoff — comparing against total memory is simpler and avoids rejecting deployments when memory usage is temporarily high. No security action needed. Consider adding available-memory validation as a future improvement for better user experience (clearer "not enough resources" errors).
