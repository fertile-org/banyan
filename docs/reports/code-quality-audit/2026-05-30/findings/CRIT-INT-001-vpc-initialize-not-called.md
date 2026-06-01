# [CRIT-INT-001] vpc.InitializeNetwork() never called in engine/agent startup

**Severity**: Critical
**Category**: INT
**Component**: pkg/vpc, cmd/banyan-engine, cmd/banyan-agent
**File(s)**: `pkg/vpc/network.go:21`, `pkg/engine/engine.go:185-216`

## Description

The component map states that `vpc.InitializeNetwork()` must be called in the engine startup path. This function exists in `pkg/vpc/network.go:21` but is **never called** in any production code path — only in integration tests.

The engine instead initializes VPC overlay networking directly via `overlay.NewSubnetAllocator()` and `overlay.NewPeerTracker()` in `pkg/engine/engine.go:185-216`.

## Evidence

**vpc.InitializeNetwork() exists but is unused:**

```go
// pkg/vpc/network.go:21
func InitializeNetwork(ctx context.Context, store storage.SchedulerStore, netConfig *NetworkConfig) error {
    // Writes Flannel configuration to etcd...
}
```

**Search for actual calls:**
```bash
$ grep -r "InitializeNetwork" /home/work/freelancer/banyan/cmd/ /home/work/freelancer/banyan/pkg/engine/ /home/work/freelancer/banyan/pkg/agent/
# No production calls found

$ grep -r "InitializeNetwork" /home/work/freelancer/banyan/test/integration/
test/integration/vpc/run_multi_host_integration.go:390:  err = vpc.InitializeNetwork(ctx, store, netConfig)
# Only called in integration test script
```

**Engine uses overlay package directly:**

```go
// pkg/engine/engine.go:185-216
var allocator overlay.SubnetAllocatorInterface
var peerTracker overlay.PeerTrackerInterface
if e.opts.VPCCIDR != "" {
    if e.multiEngine {
        allocator, allocErr = newEtcdSubnetAllocator(e.opts.VPCCIDR, e.store, lockStore)
        peerTracker = newEtcdPeerTracker(e.store)
    } else {
        allocator, allocErr = overlay.NewSubnetAllocator(e.opts.VPCCIDR)
        peerTracker = overlay.NewPeerTracker()
    }
}
```

The engine creates its own subnet allocator and peer tracker using `pkg/vpc/overlay` directly — it does not call `vpc.InitializeNetwork()`.

## Impact

**User impact**: Containers on different agents may not be able to communicate across hosts if the VPC initialization is incomplete. The `InitializeNetwork()` function configures Flannel-based networking which may be required for cross-host container communication in certain configurations.

**Developer impact**: The component map is misleading — it claims `vpc.InitializeNetwork()` is called during engine startup, but the actual initialization happens through a different code path (`overlay.NewSubnetAllocator()`). This discrepancy could cause confusion during debugging or when adding new VPC features.

**System impact**: If the old Flannel initialization approach is needed for compatibility but isn't being called, cross-host networking may be broken in configurations that expect it.

## Recommendation

1. **Determine intent**: Is `vpc.InitializeNetwork()` deprecated in favor of direct overlay initialization? If so, update the component map and remove the dead code.
2. **If the function is still needed**: Wire it into the engine startup path in `pkg/engine/engine.go` before the overlay allocator is created.
3. **Document the change**: If the VPC initialization approach changed, update the component map to reflect the actual wiring (`overlay.NewSubnetAllocator()` vs `vpc.InitializeNetwork()`).