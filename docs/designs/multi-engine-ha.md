# Multi-Engine High Availability Design

**Status:** Design Phase | **Effort:** Medium | **Priority:** High

## Overview

Enable multiple active engine nodes to share workload, eliminating the single point of failure in the current architecture. Engines coordinate through a shared etcd cluster, allowing any engine to handle CLI requests and schedule tasks.

## Current State

**Single Engine Architecture:**
```
CLI → Engine (single) → Agents
      ↓
    etcd/BadgerDB
    Embedded Registry
```

**Limitations:**
- Single engine is a SPOF
- BadgerDB (default) is local-only
- Embedded registry is instance-local
- No coordination for concurrent operations

## Target Design

**Active-Active Multi-Engine:**
```
                    ┌──────────────┐
                    │     etcd     │
                    │  (clustered) │
                    └──────┬───────┘
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
  ┌────▼────┐         ┌────▼────┐         ┌────▼────┐
  │ Engine 1│         │ Engine 2│         │ Engine 3│
  │+Registry│         │+Registry│         │+Registry│
  └────┬────┘         └────┬────┘         └────┬────┘
       │                   │                   │
       └───────────────────┼───────────────────┘
                           │
                   ┌───────▼───────┐
                   │  Load Balancer │
                   │   (optional)   │
                   └───────┬───────┘
                           │
       ┌───────────────────┼───────────────────┐
       │                   │                   │
  ┌────▼─────┐       ┌────▼─────┐       ┌────▼─────┐
  │   CLI    │       │  Agent 1 │       │  Agent 2 │
  └──────────┘       └──────────┘       └──────────┘
```

**Key Properties:**
- ✅ Any engine can handle CLI requests
- ✅ Any engine can schedule tasks
- ✅ All state coordination via etcd
- ✅ Distributed registry with index lookup
- ✅ No single point of failure

## Critical Coordination Points

### 1. Task Scheduling (Compare-And-Swap)

**Problem:** Two engines might schedule the same task.

**Solution:** etcd Compare-And-Swap (CAS) transactions.

```
Engine A sees: need 3 replicas of web
Engine B sees: need 3 replicas of web

Both read /tasks/pool and attempt CAS:
- Engine A: CAS(task1, "engine-a") → SUCCESS
- Engine B: CAS(task1, "engine-b") → FAIL (already claimed)
- Engine B: tries task2 instead
```

**etcd Transaction Example:**
```go
txn := client.Txn(ctx).
    If(modRev("/tasks/pool/task1") == expectedRev).
    Then(op.Put("/tasks/pool/task1", "engine-a")).
    Else()
response, _ := txn.Commit()
if !response.Succeeded {
    // Task claimed by another engine, try next
}
```

### 2. Registry Index

**Problem:** Agent needs to know which engine has the image.

**Solution:** Registry index in etcd.

**etcd Structure:**
```
/registry/images/
  ├── sha256:abc123... → {"engine": "engine-1:5000", "path": "/var/lib/registry/abc123"}
  ├── sha256:def456... → {"engine": "engine-2:5000", "path": "/var/lib/registry/def456"}
```

**Agent Pull Flow:**
```
1. Agent receives task: pull web:sha256:abc123
2. Agent queries etcd: GET /registry/images/sha256:abc123
3. Response: {"engine": "http://engine-1:5000"}
4. Agent pulls directly from Engine 1's registry
```

**No LB needed** for registries — agents connect directly to the specific engine.

### 3. Deployment Updates (Optimistic Locking)

**Problem:** Concurrent deploy requests can race.

**Solution:** Version/timestamp on deployments.

```
/deploys/my-app:
  {
    "version": 5,
    "manifest": {...},
    "status": "running"
  }

Update request:
  CAS(version=5, newVersion=6)
  If version ≠ 5, reject with "stale update" error
```

### 4. Session Management

**Problem:** Agent reconnection to different engine loses session.

**Solution:** Move sessions from `sync.Map` to etcd.

**Current:** `sync.Map[sessionToken] -> Session`
**Multi-Engine:** `/sessions/{token} -> {agentID, expiry, ...}`

## Implementation Phases

### Phase 1: Storage & Prerequisites (Low)

**Changes:**
- Require etcd for multi-engine mode (BadgerDB = single-engine only)
- Document etcd cluster setup (3-node recommendation)
- Add `--multi-engine` flag to `banyan-engine start`

**Acceptance:**
- [ ] Engine validates etcd connectivity in multi-engine mode
- [ ] Documentation for etcd cluster setup
- [ ] CI tests with etcd (already exists)

### Phase 2: Task Claiming (Medium)

**Changes:**
- Implement CAS-based task claiming in scheduler
- Add retry logic when CAS fails
- Update task pool structure in etcd

**etcd Changes:**
```
/tasks/pool/
  ├── task-1 → {"status": "pending", "owner": "", "lease": 0}
  ├── task-2 → {"status": "claimed", "owner": "engine-1", "lease": 12345}

On claim:
  txn.If(
    modRev("/tasks/pool/task-1") == rev,
    owner == ""
  ).Then(
    Put("/tasks/pool/task-1", {"status": "claimed", "owner": "engine-a"})
  )
```

**Acceptance:**
- [ ] Two engines can schedule concurrently without duplicate tasks
- [ ] Failed claims are retried with different tasks
- [ ] Unit tests for CAS operations

### Phase 3: Registry Index (Medium)

**Changes:**
- Add registry index writer on image push
- Add registry index lookup on task assignment
- Include image location in task payload

**Image Push Flow:**
```
1. Build completes on Engine A
2. Push to local registry
3. Write to etcd: PUT /registry/images/sha256:abc123 -> {"engine": "engine-a:5000"}
4. Task includes: {"image": "web:sha256:abc123", "registry": "http://engine-a:5000"}
```

**Agent Pull Flow:**
```
1. Agent receives task with registry URL
2. Agent pulls: http://engine-a:5000/v2/web/manifests/sha256:abc123
3. Fallback: if registry unavailable, request reschedule
```

**Acceptance:**
- [ ] Registry index is updated on every build
- [ ] Agents pull images from correct engine registry
- [ ] Index cleanup on deployment deletion

### Phase 4: Session State (Low)

**Changes:**
- Move session storage from `sync.Map` to etcd
- Add session TTL and refresh logic
- Update session validation

**etcd Structure:**
```
/sessions/{sessionToken} -> {
  "agentID": "agent-1",
  "createdAt": 1234567890,
  "expiresAt": 1234567890 + 3600,
  "lastHeartbeat": 1234567890
}
```

**Acceptance:**
- [ ] Agents can reconnect to any engine with valid session
- [ ] Expired sessions are cleaned up
- [ ] Session token validation works across engines

### Phase 5: Deployment Locking (Medium)

**Changes:**
- Add version field to deployments
- Implement optimistic locking for updates
- Return error on concurrent modification

**Acceptance:**
- [ ] Concurrent deploy requests are serialized
- [ ] Stale updates return clear error message
- [ ] CLI can retry on conflict

### Phase 6: Client Load Balancing (Low)

**Changes:**
- CLI supports multiple engine endpoints
- Add simple round-robin or random selection
- Optional: external LB configuration

**Config Example:**
```yaml
cli:
  engines:
    - http://engine-1:9090
    - http://engine-2:9090
    - http://engine-3:9090
```

**Acceptance:**
- [ ] CLI connects to available engine if one fails
- [ ] Requests are distributed across engines
- [ ] Fallback works transparently

### Phase 7: Image Cleanup (Low)

**Changes:**
- Add TTL or reference counting for registry entries
- Background cleanup job for unused images
- Configurable retention policy

**Acceptance:**
- [ ] Unused images are cleaned up
- [ ] Active images are preserved
- [ ] Cleanup doesn't break running deployments

## Open Questions

| Question | Options | Recommendation |
|----------|---------|----------------|
| **Registry storage** | External registry vs. distributed | Start with distributed (simpler for users) |
| **Load balancer** | Built-in CLI LB vs. external LB | Optional external LB (HAProxy/Nginx) |
| **Session backend** | etcd vs. Redis | etcd (already required) |
| **Image cleanup** | TTL vs. reference counting | TTL-based (simpler) |
| **Engine discovery** | Static config vs. service discovery | Static config initially |

## Migration Path

**Single-Engine → Multi-Engine:**

1. **Backup etcd data** (if using BadgerDB, migrate to etcd first)
2. **Add second engine** with `--multi-engine` flag
3. **Verify task claiming** works (deploy with multiple replicas)
4. **Enable CLI load balancing** (configure multiple endpoints)
5. **Add third engine** for full HA

**Rollback:** Remove multi-engine engines, restart single engine.

## Success Criteria

- [ ] 3+ engines can run concurrently without conflicts
- [ ] Engine failure doesn't stop new deployments
- [ ] Agents pull images from correct registry
- [ ] CLI can operate with any engine available
- [ ] No task duplication across engines
- [ ] Session continuity across engine reconnection

## References

- Current storage abstraction: `pkg/storage/interface.go`
- etcd concurrency primitives: https://etcd.io/docs/latest/learning/api/#concurrency
- VPC etcd requirements: Flannel requires etcd for networking
