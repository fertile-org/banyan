# Research: Multi-Engine HA (Milestone 7) — Codebase Analysis

**Date**: 2026-03-13T22:38:00+07:00
**Git Commit**: 60a0a8e
**Branch**: feat/code-coverage

## Research Question

Analyze the codebase to prepare for Multi-Engine HA implementation. Both single-engine (managed etcd) and multi-engine (user-provided etcd) modes must work. Identify all components needing changes, coordination points, race conditions, and architectural decisions.

---

## Summary

The codebase is fundamentally **single-engine**. There are no distributed coordination primitives, no leader election, and all critical scheduling/networking state lives in-memory on a single engine process. Migrating to multi-engine requires changes across 6 major subsystems: engine lifecycle, gRPC server state, etcd storage, scheduling, agent connectivity, and CLI connectivity.

The key insight: **most of the hard work is etcd coordination**. The storage layer already uses etcd but treats it as a simple KV store — no CAS, no Watch, no distributed locks. Multi-engine requires upgrading the storage patterns to use etcd transactions and watches.

---

## 1. Engine Lifecycle & Configuration

### Current State

**`pkg/engine/engine.go`** — Engine struct (L42-52):
```go
type Engine struct {
    config    types.EngineConfig
    store     storage.StateStore
    rpcServer *engineGRPCServer
    logger    *slog.Logger
    cancel    context.CancelFunc
}
```

**`cmd/banyan-engine/cmd/engine.go`** — Managed etcd start (L544-572):
- Engine starts embedded etcd as a child process (`etcd --data-dir ... --listen-client-urls http://127.0.0.1:2379`)
- Waits for etcd to be ready, then connects
- Kills etcd on engine shutdown

**`pkg/types/config.go`** — EngineConfig (L42-58):
```go
type EngineConfig struct {
    DataDir          string
    ListenAddr       string
    EtcdEndpoints    []string
    ManagedEtcd      bool
    RegistryAddr     string
    RegistryDataDir  string
    WGPrivateKey     string
    WGPublicKey      string
    // ... TLS fields
}
```

### Key Observations
- `ManagedEtcd bool` already exists — single vs external etcd is a config distinction
- `EtcdEndpoints []string` already supports multiple endpoints
- Engine startup is sequential: etcd → store → gRPC server → orchestration loop
- No concept of engine identity (no engine ID, no engine name)
- No heartbeat from engine to anything (engines don't announce themselves)

### What Needs to Change
- Engine needs a unique ID (for leader election, lease ownership)
- Multi-engine mode: skip managed etcd, connect to user-provided etcd cluster
- Engine needs to register itself in etcd (other engines/CLI need to discover it)
- Graceful handoff on engine shutdown (release leases, drain work)

---

## 2. gRPC Server — In-Memory State

### Current State

**`pkg/engine/grpc_server.go`** — engineGRPCServer struct (L65-80):
```go
type engineGRPCServer struct {
    store           storage.StateStore
    allocator       *overlay.SubnetAllocator    // VPC subnet allocation
    peerTracker     *overlay.PeerTracker         // VPC peer tracking
    limiter         *rate.Limiter                // rate limiting
    metricsRegistry *metrics.Registry            // Prometheus metrics
    events          *EventBuffer                 // recent events
    logger          *slog.Logger
    // ...
}
```

### State Classification

| State | Type | Multi-Engine Impact |
|-------|------|-------------------|
| `allocator` (SubnetAllocator) | **Must coordinate** | Two engines allocating subnets = overlap → IP conflicts |
| `peerTracker` (PeerTracker) | **Must coordinate** | Each engine only knows peers of agents connected to it |
| `limiter` | Per-engine OK | Rate limiting is per-engine (acceptable) |
| `metricsRegistry` | Per-engine OK | Metrics are per-engine |
| `events` | Per-engine OK | Events are per-engine (can be merged in CLI) |

### RPC Handler State Access Patterns

| RPC | etcd Reads | etcd Writes | In-Memory State | Coordination Risk |
|-----|-----------|-------------|-----------------|-------------------|
| `Deploy` | deployments | deployment, tasks | allocator | **HIGH** — two engines deploying same service |
| `Down` | deployment, tasks | deployment, tasks | peerTracker | **HIGH** — concurrent deploy+down |
| `Register` | nodes | node | allocator, peerTracker | **MEDIUM** — agent registers on two engines |
| `Heartbeat` | node, tasks | node, tasks | peerTracker, events | **LOW** — agent heartbeats to one engine |
| `Status` | deployments, nodes | — | — | None (read-only) |
| `Logs` | tasks, nodes | — | — | None (read-only) |

### What Needs to Change
- `SubnetAllocator` must be backed by etcd (atomic allocation)
- `PeerTracker` must be backed by etcd (all engines see all peers)
- Deploy/Down RPCs need distributed locking per deployment

---

## 3. etcd Storage Layer

### Current State

**`pkg/storage/interface.go`** — StateStore interface:
```go
type StateStore interface {
    Save(ctx context.Context, key, value string) error
    Get(ctx context.Context, key string) (string, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) (map[string]string, error)
    Close() error
}
```

**`pkg/storage/etcd.go`** — Additional methods exist but are **unused**:
- `SaveWithTTL(ctx, key, value, ttl)` — save with lease (defined but never called)
- `Watch(ctx, prefix)` — watch for changes (defined but never called)
- `KeepAlive(ctx, key, value, ttl)` — create lease + keep alive (defined but never called)

### Key Schema
```
/banyan/deployments/<deployment-id>   → DeploymentRecord JSON
/banyan/nodes/<agent-name>            → NodeRecord JSON
/banyan/tasks/<agent-name>/<task-id>  → TaskRecord JSON
/banyan/registry/<image-ref>          → RegistryRecord JSON
```

### Critical Gap: No CAS Operations
Every write is a blind `Put`. No compare-and-swap, no transactions. Example from `grpc_server.go`:
```go
node, _ := store.Get(key)       // read
node.Status = "running"          // modify
store.Save(key, marshal(node))   // write (overwrites any concurrent change!)
```

### What Needs to Change
- Add `SaveIfRevision(ctx, key, value, expectedRevision)` — CAS operation
- Add `Transaction(ctx, ops)` — atomic multi-key operations
- Use `Watch` to keep in-memory caches in sync across engines
- Use `SaveWithTTL` for engine registration (auto-cleanup on crash)
- Add `KeepAlive` for engine liveness

---

## 4. Scheduling & Orchestration

### Current State

**`pkg/engine/engine.go`** — engineLoop() (L266-279):
- Runs every 5 seconds
- Scans all deployments, finds those in `StatusPending`
- Calls `schedulePendingDeployment()` which calls `BuildTasksForDeployment()`

**`pkg/types/helpers.go`** — BuildTasksForDeployment():
- Reads all agents from store
- Picks agents using `pickAgentByResources()` (most available memory)
- Creates TaskRecords and saves them

### Race Conditions (Multi-Engine)

| ID | Scenario | Current Behavior | Multi-Engine Risk |
|----|----------|-----------------|-------------------|
| RC-1 | Two engines run engineLoop simultaneously | N/A (single engine) | Both schedule the same pending deployment → duplicate tasks |
| RC-2 | Deploy + Down on same service concurrently | Sequential (single engine) | One engine deploys while another tears down → inconsistent state |
| RC-3 | Two agents heartbeat to different engines | N/A | Both engines update the same NodeRecord → last-write-wins |
| RC-4 | Subnet allocation on two engines | N/A | Both allocate the same /24 → IP conflicts |
| RC-5 | Blue-green: old teardown while new deploying | Sequential | New engine sees old as "running", old engine tears down new → data loss |
| RC-6 | Two engines pick same agent for scheduling | N/A | Over-commitment of agent resources |

### What Needs to Change
- Deployment scheduling: acquire etcd lock per deployment before scheduling
- engineLoop: only the leader runs scheduling (or each engine handles subset)
- Blue-green transitions: atomic state change with CAS
- Agent resource accounting: use etcd transactions to prevent over-commit

---

## 5. Agent-Engine Communication

### Current State

**`pkg/agent/engine_client.go`** — Single endpoint:
```go
type EngineClient struct {
    conn   *grpc.ClientConn
    client rpc.EngineServiceClient
}
```

**`pkg/agent/agent.go`** — Reconnection (L750-836):
- Agent connects to single engine endpoint
- On disconnect: exponential backoff (1s → 60s)
- Re-registers on reconnect

### What Needs to Change
- Agent needs multiple engine endpoints (or DNS-based discovery)
- On connect, agent picks one engine (round-robin or random)
- On disconnect, try next engine (failover)
- Agent must re-register on failover (new engine doesn't know it)
- Heartbeat goes to connected engine only (engine shares state via etcd)

---

## 6. CLI-Engine Communication

### Current State

**`cmd/banyan-cli/cmd/client.go`** — NewAutoEngineClient() (L47-70):
- Checks WireGuard tunnel → connects to `10.200.0.1`
- Falls back to `CLIConfig.EngineAddr` (single endpoint)

**`cmd/banyan-cli/cmd/init.go`** — CLI init:
- Prompts for single engine address
- Stores in config file

### What Needs to Change
- CLI config needs multiple engine endpoints
- CLI tries endpoints in order (or uses DNS)
- WireGuard tunnel: `10.200.0.1` is engine-specific — multi-engine needs multiple tunnel peers or a VIP
- CLI init: prompt for comma-separated endpoints or DNS name

---

## 7. Registry

### Current State

**`pkg/engine/engine.go:801-826`** — Registry is in-memory only:
- Uses `google/go-containerregistry` with in-memory blob store
- Image layers stored in engine process memory
- Not persisted to disk, not stored in etcd (too large)
- Lost on every engine restart

### Decision: Managed Distribution Registry

Replace in-memory registry with managed [Distribution](https://github.com/distribution/distribution) (Docker Registry v2) subprocess. Same pattern as managed etcd:

| | etcd | Registry |
|---|---|---|
| **Managed default** | Embedded etcd process | Distribution subprocess |
| **User-provided** | External etcd cluster | External registry (Harbor, Docker Hub, etc.) |
| **Multi-engine** | External required | External required |
| **Data dir** | `/var/lib/banyan/etcd/` | `/var/lib/banyan/registry/` |

- Single-engine: managed registry (persistent, zero setup)
- Multi-engine: external registry required (managed is per-machine, not shareable)

---

## 8. Design Decisions Required

### D1: Leader vs Active-Active
- **Leader-based**: One engine runs scheduling loop, others proxy. Simpler, fewer race conditions.
- **Active-active**: All engines can schedule. Better availability, more complex coordination.
- **Recommendation**: Leader-based for scheduling, active-active for read RPCs (Status, Logs).

### D2: Engine Discovery
- **Static config**: Agents/CLI configured with list of engine endpoints.
- **DNS-based**: Single DNS name resolves to all engines.
- **etcd-based**: Engines register in etcd, agents discover from etcd (chicken-and-egg problem).
- **Recommendation**: Static config (simplest, matches user expectation).

### D3: Registry Strategy — DECIDED
- **Decision**: Managed Distribution (Docker Registry v2) subprocess by default, same pattern as managed etcd
- Single-engine: managed registry on local disk (persistent, zero setup)
- User can bring their own registry (Harbor, Docker Hub, etc.)
- Multi-engine: external registry required

### D4: WireGuard Tunnel
- **Current**: All traffic tunneled to single engine IP `10.200.0.1`.
- **Multi-engine**: Each engine gets a tunnel IP, agents/CLI connect to any.
- **Alternative**: External load balancer in front of engines.
- **Recommendation**: Multiple tunnel peers, connect to first available.

### D5: Subnet Allocation
- **Current**: In-memory `SubnetAllocator` tracks used subnets.
- **Multi-engine**: Must be in etcd to prevent overlap.
- **Recommendation**: Store allocated subnets in etcd (`/banyan/subnets/<agent-name>` → CIDR).

---

## 9. Mode Matrix — Three Valid Configurations

Multi-engine is an **explicit user choice** (not inferred from etcd config). Users can provide external etcd even with a single engine.

| Configuration | etcd | Registry | Scheduling | Agent/CLI Config |
|--------------|------|----------|------------|-----------------|
| **Single engine + managed etcd** (default) | Managed (embedded process) | Built-in (in-memory) | Direct (no locking) | Single endpoint |
| **Single engine + external etcd** | User-provided | Built-in (in-memory) | Direct (no locking) | Single endpoint |
| **Multi-engine** (requires external etcd) | User-provided (required) | External required | Leader election + locks | Multiple endpoints |

### Mode activation
- **`ManagedEtcd`**: controls whether engine starts embedded etcd. Independent of engine count.
- **Multi-engine mode**: explicit opt-in (e.g., `--multi-engine` flag or config field). When enabled, external etcd is **required** (error if `ManagedEtcd=true`), external registry is **required**, and distributed coordination (leader election, CAS) is activated.

**Key principle**: Multi-engine features must be **additive**. Single-engine mode (with or without managed etcd) should see no behavioral change.

---

## 10. Existing Infrastructure to Leverage

These exist in the codebase but are currently unused — they were designed for this:

1. **`SaveWithTTL`** — engine registration with auto-expiry
2. **`Watch`** — cross-engine state synchronization
3. **`KeepAlive`** — engine liveness heartbeat
4. **`EtcdEndpoints []string`** — already supports multiple etcd nodes
5. **`ManagedEtcd bool`** — already distinguishes managed vs external

---

## Code References

- `pkg/engine/engine.go:42-52` — Engine struct
- `pkg/engine/engine.go:107-207` — Engine.Run() startup
- `pkg/engine/engine.go:266-279` — engineLoop (scheduling)
- `pkg/engine/grpc_server.go:65-80` — engineGRPCServer in-memory state
- `pkg/storage/etcd.go:179-211` — SaveWithTTL (unused)
- `pkg/storage/etcd.go:285-291` — Watch (unused)
- `pkg/storage/etcd.go:302-357` — KeepAlive (unused)
- `pkg/storage/interface.go` — StateStore interface
- `pkg/types/config.go:42-58` — EngineConfig
- `pkg/types/helpers.go:139-143` — pickAgentByResources
- `pkg/vpc/overlay/allocator.go` — SubnetAllocator (in-memory)
- `pkg/vpc/overlay/peers.go` — PeerTracker (in-memory)
- `pkg/agent/engine_client.go` — Single endpoint gRPC client
- `cmd/banyan-engine/cmd/engine.go:544-572` — Managed etcd startup
- `cmd/banyan-cli/cmd/client.go:47-70` — NewAutoEngineClient
