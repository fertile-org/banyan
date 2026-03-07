# [HIGH-TEST-002] Flaky Tests Using time.Sleep

**Status**: FIXED (2026-03-07) — Replaced 10 `time.Sleep` calls with proper synchronization: `waitForDNSReady` helper (4 DNS tests), `waitForTCPReady` helper (1 agent test), removed unnecessary sleeps (5 tests). 1 intentional mock delay left as-is.
**Severity**: High
**Category**: TEST
**Component**: pkg/engine, pkg/agent, pkg/storage, pkg/vpc
**File(s)**: `pkg/engine/grpc_server_test.go:2254`, `pkg/agent/grpc_server_test.go:213,227,233,264`, `pkg/storage/etcd_test.go`, `pkg/vpc/dns/server_test.go`

## Description

Multiple test files use `time.Sleep()` for synchronization instead of proper Go concurrency primitives (channels, WaitGroups, condition variables). These are inherently flaky.

## Evidence

```go
// pkg/agent/grpc_server_test.go:213
time.Sleep(100 * time.Millisecond)

// pkg/engine/grpc_server_test.go:2254
time.Sleep(50 * time.Millisecond)
```

Sleep durations range from 50ms to 200ms across test files.

## Impact

- Tests are timing-dependent and will fail on slow CI machines
- Wastes test execution time (sleeping when the condition may already be met)
- Creates intermittent failures that erode trust in the test suite

## Recommendation

Replace `time.Sleep` with:
- `sync.WaitGroup` for "wait until N things complete"
- Channels for "wait until this event happens"
- `require.Eventually` (testify) for "wait until condition is true with timeout"
