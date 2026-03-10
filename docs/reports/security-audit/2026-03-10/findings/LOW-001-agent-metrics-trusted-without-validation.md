# [LOW-001] Agent-reported metrics trusted without validation

**Status**: FIXED
**Severity**: Low
**Responsibility**: Mitigation Gap
**Component**: Engine — Heartbeat handler, Scheduler
**File(s)**: `pkg/engine/grpc_server.go:385-390`, `pkg/types/helpers.go:139-143`

## Description

The engine stores `SystemMetrics` from agent heartbeats directly onto `NodeRecord` without any validation or bounds checking. A compromised agent can report arbitrary values for `MemoryTotalBytes`, `MemoryUsedBytes`, `CPUCores`, and `CPUUsageRatio`.

```go
// grpc_server.go — Heartbeat handler
if req.SystemMetrics != nil {
    node.MemoryTotalBytes = req.SystemMetrics.MemoryTotalBytes
    node.MemoryUsedBytes = req.SystemMetrics.MemoryUsedBytes
    node.CPUCores = req.SystemMetrics.CpuCores
    node.CPUUsageRatio = req.SystemMetrics.CpuUsageRatio
}
```

The scheduler then uses these values to decide where to place containers:

```go
// helpers.go — pickAgentByResources
avail := int64(agent.MemoryTotalBytes) - int64(agent.MemoryUsedBytes) - int64(batchMemory[agent.Name])
```

## Impact

- **Who**: A compromised or rogue agent (threat actor #2 in threat model)
- **What**: Scheduling manipulation — the agent can attract all workloads (report huge available memory) or avoid all workloads (report zero memory)
- **Blast radius**: Scheduling decisions for the entire cluster

However, this is **low severity** because:
1. Agents are authenticated via WireGuard tunnel — only registered agents can send heartbeats
2. A compromised agent already has greater capabilities (container execution, host access, task result manipulation)
3. The worst outcome is suboptimal scheduling, not data exposure or unauthorized access
4. The `ValidateClusterCapacity` check uses `MemoryTotalBytes` — a rogue agent inflating this could cause deployments to pass capacity checks when they shouldn't fit

## Evidence

In `pickAgentByResources`, the `int64()` cast on `MemoryTotalBytes` wraps negative if the value exceeds `math.MaxInt64` (~9.2 EB). A malicious agent reporting `MemoryTotalBytes = math.MaxUint64` would cause:

```
int64(math.MaxUint64) = -1
avail = -1 - int64(used) - int64(batch) → very negative
```

This would make the agent **never** selected, effectively opting out of workloads.

Conversely, reporting `MemoryTotalBytes = math.MaxInt64` and `MemoryUsedBytes = 0` would make the agent always selected, concentrating all workloads on one node.

## Recommendation

Add basic sanity checks on agent-reported metrics before storing:

```go
if req.SystemMetrics != nil {
    // Sanity: reject unreasonable values (e.g., > 64 TB)
    const maxReasonableMemory = 64 * 1024 * 1024 * 1024 * 1024 // 64 TB
    if req.SystemMetrics.MemoryTotalBytes > 0 && req.SystemMetrics.MemoryTotalBytes < maxReasonableMemory {
        node.MemoryTotalBytes = req.SystemMetrics.MemoryTotalBytes
        node.MemoryUsedBytes = min(req.SystemMetrics.MemoryUsedBytes, req.SystemMetrics.MemoryTotalBytes)
    }
    if req.SystemMetrics.CpuCores > 0 && req.SystemMetrics.CpuCores <= 1024 {
        node.CPUCores = req.SystemMetrics.CpuCores
    }
    if req.SystemMetrics.CpuUsageRatio >= 0 && req.SystemMetrics.CpuUsageRatio <= 1.0 {
        node.CPUUsageRatio = req.SystemMetrics.CpuUsageRatio
    }
}
```

This doesn't prevent a compromised agent from lying within bounds, but prevents integer overflow in the scheduler and filters obviously bogus values.
