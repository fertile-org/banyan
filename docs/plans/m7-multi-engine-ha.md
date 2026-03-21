# Milestone 7: Multi-Engine HA — Implementation Plan

## Overview

Add multi-engine high availability to Banyan. Multiple engine processes share state via etcd, with leader-based scheduling and active-active RPC handling. Single-engine mode (with or without managed etcd) remains the zero-config default. Replace the in-memory OCI registry with a managed Distribution (Docker Registry v2) subprocess for persistent image storage.

## Current State Analysis

- Engine is fundamentally single-process: no engine ID, no self-registration, no leader election
- All scheduling and VPC state lives in-memory (`SubnetAllocator`, `PeerTracker`)
- etcd is used as simple KV store — no CAS, no transactions, no distributed locks
- `SaveWithTTL`, `Watch`, `KeepAlive` exist in `pkg/storage/etcd.go` but are **unused**
- Agents and CLI connect to a single engine endpoint
- Registry is in-memory only (`google/go-containerregistry`), not persistent, not shareable
- `ManagedEtcd bool` and `EtcdEndpoints []string` already exist in config
- Install script (`install.sh`) pins etcd, nerdctl, containerd, buildkit, CNI — no registry binary yet

### Key Discoveries
- `pkg/storage/etcd.go:179-211` — `SaveWithTTL()` ready for engine registration
- `pkg/storage/etcd.go:285-291` — `Watch()` ready for cross-engine sync
- `pkg/storage/etcd.go:302-357` — `KeepAlive()` ready for engine liveness
- `pkg/engine/engine.go:266-279` — `engineLoop()` runs every 3s, must be leader-gated
- `pkg/engine/grpc_server.go:644-715` — `Deploy()` creates PENDING record, scheduling is async
- `pkg/engine/engine.go:450-509` — `schedulePendingDeployment()` is the critical section
- `pkg/vpc/overlay/allocator.go:43-61` — `Allocate()` uses in-memory map only
- `pkg/vpc/overlay/peers.go:21-25` — `Update()` uses in-memory map only
- `pkg/agent/agent.go:747-836` — reconnection already has exponential backoff
- `cmd/banyan-cli/cmd/client.go:47-70` — `NewAutoEngineClient()` hardcodes single endpoint
- `pkg/engine/engine.go:801-826` — `startRegistry()` uses `google/go-containerregistry` in-memory handler
- `cmd/banyan-engine/cmd/engine.go:544-611` — managed etcd pattern (subprocess start/stop/health check)

## Desired End State

Both etcd and registry follow the same **managed vs user-provided** pattern. Multi-engine is an explicit opt-in that requires both to be user-provided.

### Configuration Matrix

| Component | Single-engine default | User can override | Multi-engine |
|-----------|----------------------|-------------------|-------------|
| **etcd** | Managed (embedded process) | External etcd cluster | External **required** |
| **Registry** | Managed (Distribution subprocess) | External registry | External **required** |
| **Scheduling** | Direct (no locking) | — | Leader election + locks |
| **Connectivity** | Single endpoint | — | Multiple endpoints |

### Config examples

```yaml
# Default single-engine (zero config)
engine:
  managed_etcd: true         # default
  managed_registry: true     # new default

# Single-engine with external etcd (user's choice)
engine:
  managed_etcd: false
  store_address: "http://etcd.internal:2379"
  managed_registry: true     # still managed

# Single-engine with external registry (user's choice)
engine:
  managed_etcd: true
  managed_registry: false
  external_registry_url: "registry.example.com:5000"

# Multi-engine (both external required)
engine:
  multi_engine: true
  managed_etcd: false        # required when multi_engine=true
  store_address: "http://etcd.internal:2379"
  managed_registry: false    # required when multi_engine=true
  external_registry_url: "registry.example.com:5000"
```

### Verification
- All existing E2E tests pass unchanged in single-engine mode
- Managed registry: images persist across engine restart
- New E2E test: two engines, deploy/down/status work correctly
- Leader failover: kill leader, new leader elected within 15s, scheduling resumes
- Agent failover: kill connected engine, agent reconnects to another within 60s

## What We're NOT Doing

- Active-active scheduling (too complex, leader-based is sufficient)
- Automatic engine discovery via etcd (agents/CLI use static endpoint list)
- Registry replication between engines (require external registry instead)
- Distributed rate limiting (per-engine is acceptable)
- Distributed event buffer (per-engine is acceptable)
- CLI context/profile switching (single config file with multiple endpoints)
- Load balancing between engines (simple failover is sufficient for v1)
- Hosting Harbor (too heavy — Distribution registry v2 is sufficient as managed option)

## Implementation Approach

**Managed services pattern**: etcd and registry both follow "managed by default, user-provided as option." This is Banyan's philosophy — sensible defaults over configuration.

**Leader-based coordination**: One engine is the scheduling leader (runs `engineLoop`). All engines handle RPCs. Deploy/Down RPCs just write to etcd (PENDING/STOPPING status) — the leader picks up the work.

**Incremental phases**: Each phase is independently testable and doesn't break single-engine mode.

| Phase | What | Risk | Dependencies |
|-------|------|------|-------------|
| 1 | Persistent managed registry (Distribution) | Low | None |
| 2 | Engine identity & self-registration | Low | None |
| 3 | Storage CAS & distributed locks | Medium | None |
| 4 | etcd-backed SubnetAllocator & PeerTracker | Medium | Phase 3 |
| 5 | Leader election & scheduling coordination | **High** | Phases 2, 3 |
| 6 | Agent multi-endpoint support | Low | None |
| 7 | CLI multi-endpoint support | Low | None |

Phases 1, 2, 3, 6, 7 have no interdependencies and could be parallelized. Phase 4 requires Phase 3. Phase 5 requires Phases 2 and 3.

---

## Phase 1: Persistent Managed Registry (Distribution)

### Overview
Replace the in-memory `google/go-containerregistry` registry with a managed [Distribution](https://github.com/distribution/distribution) (Docker Registry v2) subprocess. Same pattern as managed etcd: start as subprocess, store data on disk, health check before proceeding. Images survive engine restart.

This phase benefits **all** users (single-engine included) and has no multi-engine dependency.

### Changes Required

#### 1. Install script — Add Distribution registry binary
**File**: `install.sh`

Add version constant and download function:
```bash
REGISTRY_VERSION="2.8.3"

install_registry() {
    if command -v registry &>/dev/null; then
        info "Distribution registry already installed."
        return 0
    fi

    info "Installing Distribution registry v${REGISTRY_VERSION}..."
    local url="https://github.com/distribution/distribution/releases/download/v${REGISTRY_VERSION}/registry_${REGISTRY_VERSION}_linux_${ARCH}.tar.gz"
    download_and_extract "$url" /usr/local/bin/registry
    chmod +x /usr/local/bin/registry
    info "Distribution registry v${REGISTRY_VERSION} installed."
}
```

Call `install_registry` in the engine install path.

#### 2. EngineConfig — Add managed registry fields
**File**: `pkg/types/config.go`

Add to `EngineConfig` struct:
```go
ManagedRegistry     bool   `yaml:"managed_registry,omitempty"`
ExternalRegistryURL string `yaml:"external_registry_url,omitempty"`
```

#### 3. Engine Options — Add registry config
**File**: `pkg/engine/engine.go`

Add to `Options` struct:
```go
ManagedRegistry     bool
ExternalRegistryURL string
```

#### 4. Managed registry subprocess — Same pattern as managed etcd
**File**: `cmd/banyan-engine/cmd/engine.go`

Add constants, start/stop/health functions mirroring `startManagedEtcd`/`stopManagedEtcd`:

```go
const managedRegistryAddr = "127.0.0.1:5000"

// startManagedRegistry starts a Distribution registry subprocess.
// Data is stored at {dataDir}/registry/ for persistence across restarts.
func startManagedRegistry(dataDir, bindAddr, port string) (*exec.Cmd, error) {
    registryDataDir := filepath.Join(dataDir, "registry")
    if err := os.MkdirAll(registryDataDir, 0o700); err != nil {
        return nil, fmt.Errorf("create registry data dir: %w", err)
    }

    // Write minimal Distribution config
    configPath := filepath.Join(dataDir, "registry-config.yml")
    configContent := fmt.Sprintf(`version: 0.1
log:
  level: warn
storage:
  filesystem:
    rootdirectory: %s
  delete:
    enabled: true
http:
  addr: %s:%s
  headers:
    X-Content-Type-Options: [nosniff]
`, registryDataDir, bindAddr, port)

    if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
        return nil, fmt.Errorf("write registry config: %w", err)
    }

    cmd := exec.Command("registry", "serve", configPath)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start registry: %w", err)
    }

    // Wait for registry to become healthy
    if err := waitForRegistry(bindAddr+":"+port, 10*time.Second); err != nil {
        _ = cmd.Process.Kill()
        return nil, fmt.Errorf("registry did not become healthy: %w", err)
    }

    return cmd, nil
}

func waitForRegistry(addr string, timeout time.Duration) error {
    healthURL := "http://" + addr + "/v2/"
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        resp, err := http.Get(healthURL) //nolint:gosec // managed registry on localhost
        if err == nil {
            resp.Body.Close()
            if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
                return nil
            }
        }
        time.Sleep(200 * time.Millisecond)
    }
    return fmt.Errorf("timeout waiting for registry at %s", addr)
}

// stopManagedRegistry gracefully stops the managed registry process.
// Same pattern as stopManagedEtcd.
func stopManagedRegistry(cmd *exec.Cmd) {
    if cmd == nil || cmd.Process == nil {
        return
    }
    _ = cmd.Process.Signal(syscall.SIGTERM)
    done := make(chan error, 1)
    go func() { done <- cmd.Wait() }()
    select {
    case <-done:
    case <-time.After(5 * time.Second):
        _ = cmd.Process.Kill()
        <-done
    }
    logging.Info("Managed registry stopped")
}
```

#### 5. Engine start — Registry mode selection
**File**: `cmd/banyan-engine/cmd/engine.go`

In `runEngineStart()`, after etcd setup but before creating Engine:

```go
var registryCmd *exec.Cmd
var externalRegistryURL string

if cfg.Engine.ExternalRegistryURL != "" {
    // User-provided registry
    externalRegistryURL = cfg.Engine.ExternalRegistryURL
} else if cfg.Engine.ManagedRegistry || cfg.Engine.ManagedEtcd /* backward compat: old configs have no ManagedRegistry field */ {
    // Start managed Distribution registry
    registryBindAddr := "127.0.0.1"
    if controlTunnelActive {
        registryBindAddr = types.ControlTunnelEngineIP
    }
    registryCmd, err = startManagedRegistry(engineDataDir, registryBindAddr, engineRegistryPort)
    if err != nil {
        return fmt.Errorf("failed to start managed registry: %w", err)
    }
    defer stopManagedRegistry(registryCmd)
}
```

#### 6. Engine Run() — Remove in-memory registry, accept external URL
**File**: `pkg/engine/engine.go`

Replace the `startRegistry()` call block (lines 134-162) with:

```go
if e.opts.ExternalRegistryURL != "" {
    // External or managed-subprocess registry — URL already known
    e.registryURL = e.opts.ExternalRegistryURL
    e.logger().Info("Using registry", "url", e.registryURL)
} else {
    // Fallback: start in-memory registry (backward compat for dev/test)
    registryBindAddr := "127.0.0.1"
    if e.opts.ControlTunnelActive {
        registryBindAddr = types.ControlTunnelEngineIP
    }
    e.logger().Info("Starting in-memory OCI registry", "bind", registryBindAddr, "port", e.opts.RegistryPort)
    registryListener, startErr := startRegistry(ctx, registryBindAddr, e.opts.RegistryPort)
    if startErr != nil {
        return fmt.Errorf("failed to start registry: %w", startErr)
    }
    _ = registryListener
    registryHost := registryBindAddr
    if registryHost == "127.0.0.1" || registryHost == "0.0.0.0" {
        engineIP, ipErr := DetermineEngineIP()
        if ipErr != nil {
            return fmt.Errorf("failed to determine engine IP: %w", ipErr)
        }
        registryHost = engineIP
    }
    e.registryURL = fmt.Sprintf("%s:%s", registryHost, e.opts.RegistryPort)
}

if saveErr := e.store.Save(ctx, types.KeyRegistry, e.registryURL); saveErr != nil {
    return fmt.Errorf("failed to save registry URL: %w", saveErr)
}
e.logger().Info("Registry URL saved to store", "url", e.registryURL)
```

The managed Distribution subprocess is started by `cmd/banyan-engine/` before `engine.New()`, and its URL is passed via `Options.ExternalRegistryURL`. From Engine's perspective, it's just an external URL. This keeps engine.go clean — it doesn't know or care whether the registry is managed or external.

#### 7. Init wizard — Registry choice
**File**: `cmd/banyan-engine/cmd/engine.go`

In `runEngineInit()`, after etcd setup:

```
Registry configuration:
  1) Managed registry (recommended - stores images locally, persistent across restarts)
  2) External registry (provide your own Docker/Harbor/etc. registry)

Choice [1]:
```

If managed: set `ManagedRegistry = true`
If external: prompt for URL, set `ManagedRegistry = false`, `ExternalRegistryURL = url`

#### 8. Remove google/go-containerregistry dependency (future cleanup)
After confirming managed Distribution works in production, the in-memory `startRegistry()` function and `google/go-containerregistry` import can be removed. For now, keep as fallback for dev/test environments where Distribution binary isn't installed.

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/engine/...` — registry URL selection tests (external URL, fallback to in-memory)
- [x] `go test ./pkg/types/...` — new config fields serialize correctly
- [x] `golangci-lint run ./...` — no lint errors
- [ ] Existing E2E tests pass (backward compat: in-memory fallback when no Distribution binary)

#### Manual Verification:
- [ ] `banyan-engine init` prompts for registry choice, saves to config
- [ ] `banyan-engine start` starts Distribution subprocess (check `ps aux | grep registry`)
- [ ] Push image via `banyan-cli up` with `build:` directive — image stored on disk
- [ ] Restart engine — pushed images still available (persistent!)
- [ ] `ls /var/lib/banyan/registry/` shows blob data
- [ ] Agent pulls image successfully from managed registry

**Implementation Note**: This phase benefits all users immediately (persistent images). No multi-engine dependency. Pause for manual verification before proceeding.

---

## Phase 2: Engine Identity & Self-Registration

### Overview
Give each engine a unique identity and register it in etcd with a TTL lease. This is the foundation for leader election (Phase 5) and engine discovery. No behavioral change for single-engine mode.

### Changes Required

#### 1. EngineConfig — Add fields
**File**: `pkg/types/config.go`

Add to `EngineConfig` struct:
```go
EngineID    string `yaml:"engine_id,omitempty"`
MultiEngine bool   `yaml:"multi_engine,omitempty"`
```

#### 2. EngineRecord — New type
**File**: `pkg/types/records.go`

Add new record type and key constant:
```go
const KeyEngines = "engines/"

type EngineRecord struct {
    ID          string    `json:"id"`
    Status      string    `json:"status"`
    GRPCAddr    string    `json:"grpc_addr"`
    RegistryURL string    `json:"registry_url,omitempty"`
    StartedAt   time.Time `json:"started_at"`
    LastSeen    time.Time `json:"last_seen"`
}
```

#### 3. Engine Options — Add EngineID and MultiEngine
**File**: `pkg/engine/engine.go`

Add to `Options` struct:
```go
EngineID    string
MultiEngine bool
```

Store on Engine struct:
```go
type Engine struct {
    // existing fields...
    engineID    string
    multiEngine bool
}
```

Set in `New()`:
```go
engineID := opts.EngineID
if engineID == "" {
    engineID = generateEngineID() // hostname-<4 random hex chars>
}
```

#### 4. Engine self-registration in Run()
**File**: `pkg/engine/engine.go`

After gRPC server starts, register engine in etcd using `KeepAlive`:
```go
if etcdStore, ok := e.store.(*storage.EtcdStore); ok {
    engineRecord := types.EngineRecord{
        ID:          e.engineID,
        Status:      "ready",
        GRPCAddr:    grpcListenAddr,
        RegistryURL: e.registryURL,
        StartedAt:   time.Now(),
        LastSeen:    time.Now(),
    }
    data, _ := json.Marshal(engineRecord)
    if err := etcdStore.KeepAlive(ctx, types.KeyEngines+e.engineID, string(data), 15*time.Second); err != nil {
        e.logger().Warn("Failed to register engine in etcd", "error", err)
    }
}
```
Engine record auto-expires if engine crashes (15s TTL, renewed every 10s by KeepAlive).

#### 5. Init wizard — Generate engine ID
**File**: `cmd/banyan-engine/cmd/engine.go`

In `runEngineInit()`, after WireGuard keypair generation:
- Generate engine ID: `hostname-<4 random hex chars>` (e.g., `prod-web-1-a3f2`)
- Display to user, allow override
- Save to config

Add multi-engine prompt:
```
Enable multi-engine mode? (requires external etcd and external registry) [y/N]:
```
If yes:
- Set `MultiEngine = true`
- Validate: error if managed etcd or managed registry selected
- Must have external etcd + external registry configured

#### 6. Start command — Validate multi-engine prerequisites
**File**: `cmd/banyan-engine/cmd/engine.go`

In `runEngineStart()`, after config load:
```go
if cfg.Engine.MultiEngine {
    if cfg.Engine.ManagedEtcd {
        return fmt.Errorf("multi-engine mode requires external etcd (set managed_etcd: false and provide store_address)")
    }
    if cfg.Engine.ManagedRegistry || cfg.Engine.ExternalRegistryURL == "" {
        return fmt.Errorf("multi-engine mode requires external registry (set managed_registry: false and provide external_registry_url)")
    }
}
```

Pass `EngineID` and `MultiEngine` to `engine.New()`.

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/types/...` — new types compile and serialize correctly
- [x] `go test ./pkg/engine/...` — existing tests pass, new engine registration test
- [x] `go test ./cmd/banyan-engine/...` — multi-engine prerequisite validation tests
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] `banyan-engine init` generates engine ID and displays it
- [ ] `banyan-engine start` registers engine in etcd (check with `etcdctl get /banyan/engines/ --prefix`)
- [ ] Engine record auto-expires after engine stops (wait 15s, verify key gone)
- [ ] Single-engine mode works exactly as before
- [ ] Multi-engine mode rejects managed etcd and managed registry

---

## Phase 3: Storage Layer — CAS & Distributed Locks

### Overview
Extend the `StateStore` interface with compare-and-swap and distributed lock primitives. These are the building blocks for safe concurrent access in Phase 5.

### Changes Required

#### 1. Extend StateStore interface
**File**: `pkg/storage/interface.go`

Add new interfaces (StateStore itself unchanged for backward compat):
```go
// CASStore extends StateStore with compare-and-swap for multi-engine coordination.
type CASStore interface {
    StateStore

    // GetWithRevision returns the value and its etcd ModRevision.
    GetWithRevision(ctx context.Context, key string, value interface{}) (revision int64, err error)

    // SaveIfRevision performs a compare-and-swap: saves only if the key's
    // current ModRevision matches expectedRevision.
    SaveIfRevision(ctx context.Context, key string, value interface{}, expectedRevision int64) error
}

// LockStore extends StateStore with distributed locking.
type LockStore interface {
    StateStore

    // Lock acquires a distributed lock on the given key.
    // Returns an unlock function. The lock auto-expires after ttl.
    Lock(ctx context.Context, key string, ttl time.Duration) (unlock func(), err error)
}

var ErrRevisionMismatch = errors.New("revision mismatch: key was modified concurrently")
```

#### 2. Implement CAS in EtcdStore
**File**: `pkg/storage/etcd.go`

Extend `etcdKV` interface with `Txn`:
```go
type etcdKV interface {
    // existing methods...
    Txn(ctx context.Context) clientv3.Txn
}
```

Implement `GetWithRevision`:
```go
func (s *EtcdStore) GetWithRevision(ctx context.Context, key string, value interface{}) (int64, error) {
    fullKey := s.prefix + key
    resp, err := s.client.Get(ctx, fullKey)
    if err != nil {
        return 0, fmt.Errorf("etcd get: %w", err)
    }
    if len(resp.Kvs) == 0 {
        return 0, fmt.Errorf("key not found: %s", key)
    }
    if err := json.Unmarshal(resp.Kvs[0].Value, value); err != nil {
        return 0, fmt.Errorf("unmarshal: %w", err)
    }
    return resp.Kvs[0].ModRevision, nil
}
```

Implement `SaveIfRevision`:
```go
func (s *EtcdStore) SaveIfRevision(ctx context.Context, key string, value interface{}, expectedRevision int64) error {
    fullKey := s.prefix + key
    data, err := json.Marshal(value)
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    txnResp, err := s.client.Txn(ctx).
        If(clientv3.Compare(clientv3.ModRevision(fullKey), "=", expectedRevision)).
        Then(clientv3.OpPut(fullKey, string(data))).
        Commit()
    if err != nil {
        return fmt.Errorf("etcd txn: %w", err)
    }
    if !txnResp.Succeeded {
        return storage.ErrRevisionMismatch
    }
    return nil
}
```

#### 3. Implement distributed lock in EtcdStore
**File**: `pkg/storage/etcd.go`

Use etcd concurrency package:
```go
import "go.etcd.io/etcd/client/v3/concurrency"

func (s *EtcdStore) Lock(ctx context.Context, key string, ttl time.Duration) (func(), error) {
    session, err := concurrency.NewSession(s.rawClient, concurrency.WithTTL(int(ttl.Seconds())))
    if err != nil {
        return nil, fmt.Errorf("create session: %w", err)
    }
    mutex := concurrency.NewMutex(session, s.prefix+key)
    if err := mutex.Lock(ctx); err != nil {
        session.Close()
        return nil, fmt.Errorf("acquire lock: %w", err)
    }
    return func() {
        mutex.Unlock(context.Background())
        session.Close()
    }, nil
}
```

Need to expose `rawClient` on EtcdStore (the underlying `*clientv3.Client`):
```go
func (s *EtcdStore) Client() *clientv3.Client {
    return s.rawClient
}
```

#### 4. Single-engine fallback
Multi-engine code paths use type assertions:
```go
if casStore, ok := e.store.(storage.CASStore); ok && e.multiEngine {
    // use CAS path
} else {
    // use simple path (existing behavior)
}
```

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/storage/...` — CAS tests: save-if-revision success, revision mismatch, concurrent writers
- [ ] `go test ./pkg/storage/...` — Lock tests: acquire, release, auto-expire, contention
- [ ] `golangci-lint run ./...` — no lint errors
- [x] Existing tests pass unchanged (interface is backward-compatible)

#### Manual Verification:
- [ ] Single-engine mode: no behavioral change
- [ ] etcdctl: verify CAS transaction actually uses etcd Txn API

**Implementation Note**: Pause for manual confirmation before proceeding.

---

## Phase 4: etcd-backed SubnetAllocator & PeerTracker

### Overview
Move VPC subnet allocation and peer tracking from in-memory to etcd-backed. In single-engine mode, keep the current in-memory implementation. In multi-engine mode, use etcd for coordination.

### Changes Required

#### 1. Interface abstraction
**File**: `pkg/vpc/overlay/allocator.go` and `peers.go`

Extract interfaces so engine code doesn't care about implementation:
```go
type SubnetAllocatorInterface interface {
    Allocate(ctx context.Context, agentName string) (*net.IPNet, error)
    Release(ctx context.Context, agentName string)
    GetAll(ctx context.Context) map[string]*net.IPNet
}

type PeerTrackerInterface interface {
    Update(ctx context.Context, agentName string, peer Peer) error
    Remove(ctx context.Context, agentName string) error
    GetPeersExcluding(ctx context.Context, agentName string) []Peer
}
```

Update existing `SubnetAllocator` and `PeerTracker` to implement these interfaces (add `ctx` parameter, return errors where needed). Update all callers in `pkg/engine/grpc_server.go`.

#### 2. etcd-backed SubnetAllocator
**File**: `pkg/vpc/overlay/etcd_allocator.go` (new file)

```go
type EtcdSubnetAllocator struct {
    vpcCIDR   *net.IPNet
    subnetLen int
    store     storage.CASStore
    lockStore storage.LockStore
}

// Key schema: /banyan/vpc/subnets/<agent-name> → subnet CIDR string
const subnetKeyPrefix = "vpc/subnets/"
```

`Allocate()` uses distributed lock:
```go
func (a *EtcdSubnetAllocator) Allocate(ctx context.Context, agentName string) (*net.IPNet, error) {
    // Idempotent check
    var existing string
    if _, err := a.store.GetWithRevision(ctx, subnetKeyPrefix+agentName, &existing); err == nil {
        _, subnet, _ := net.ParseCIDR(existing)
        return subnet, nil
    }

    // Lock, list used subnets, find available, save
    unlock, err := a.lockStore.Lock(ctx, "locks/subnet-allocator", 10*time.Second)
    if err != nil {
        return nil, fmt.Errorf("acquire subnet lock: %w", err)
    }
    defer unlock()

    used := a.listUsedSubnets(ctx)
    subnet := findAvailableSubnet(a.vpcCIDR, a.subnetLen, used)
    if subnet == nil {
        return nil, fmt.Errorf("no available subnets in %s", a.vpcCIDR)
    }

    if err := a.store.Save(ctx, subnetKeyPrefix+agentName, subnet.String()); err != nil {
        return nil, fmt.Errorf("save subnet allocation: %w", err)
    }
    return subnet, nil
}
```

#### 3. etcd-backed PeerTracker
**File**: `pkg/vpc/overlay/etcd_peers.go` (new file)

```go
type EtcdPeerTracker struct {
    store storage.StateStore
}

const peerKeyPrefix = "vpc/peers/"
```

Methods — simple etcd CRUD, no locking needed:
- `Update(ctx, agentName, peer)` → `store.Save(ctx, peerKeyPrefix+agentName, peer)`
- `Remove(ctx, agentName)` → `store.Delete(ctx, peerKeyPrefix+agentName)`
- `GetPeersExcluding(ctx, agentName)` → `store.List(ctx, peerKeyPrefix)`, filter locally

#### 4. Engine initialization — choose implementation
**File**: `pkg/engine/engine.go`

In `Run()`:
```go
if e.multiEngine {
    casStore := e.store.(storage.CASStore)
    lockStore := e.store.(storage.LockStore)
    allocator = overlay.NewEtcdSubnetAllocator(e.opts.VPCCIDR, casStore, lockStore)
    peerTracker = overlay.NewEtcdPeerTracker(e.store)
} else {
    allocator, allocErr = overlay.NewSubnetAllocator(e.opts.VPCCIDR)
    peerTracker = overlay.NewPeerTracker()
}
```

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/vpc/overlay/...` — etcd allocator tests (allocate, release, idempotent, concurrent from two goroutines)
- [x] `go test ./pkg/vpc/overlay/...` — etcd peer tracker tests (update, remove, get excluding)
- [x] `go test ./pkg/engine/...` — existing tests pass with in-memory implementation (interface change is backward compat)
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] Single-engine: VPC networking works exactly as before
- [ ] Verify etcd keys: `etcdctl get /banyan/vpc/subnets/ --prefix`
- [ ] Verify etcd keys: `etcdctl get /banyan/vpc/peers/ --prefix`

**Implementation Note**: Pause here for manual verification.

---

## Phase 5: Leader Election & Scheduling Coordination

### Overview
Implement leader election so only one engine runs the scheduling loop. All engines can handle RPCs. Deploy/Down just write to etcd; the leader picks up the work.

This is the most critical phase — it's where multi-engine actually starts working.

### Changes Required

#### 1. Leader election using etcd
**File**: `pkg/engine/leader.go` (new file)

```go
type LeaderElection struct {
    client   *clientv3.Client
    isLeader atomic.Bool
    engineID string
    logger   *logging.Logger
}

func NewLeaderElection(client *clientv3.Client, engineID string) *LeaderElection { ... }

func (le *LeaderElection) Run(ctx context.Context) error {
    for {
        session, err := concurrency.NewSession(le.client, concurrency.WithTTL(15))
        if err != nil {
            le.logger.Error("Failed to create election session", "error", err)
            select {
            case <-time.After(5 * time.Second):
                continue
            case <-ctx.Done():
                return ctx.Err()
            }
        }

        election := concurrency.NewElection(session, "/banyan/leader/")

        // Campaign blocks until we become leader or ctx is cancelled
        if err := election.Campaign(ctx, le.engineID); err != nil {
            session.Close()
            if ctx.Err() != nil {
                return ctx.Err()
            }
            continue
        }

        le.isLeader.Store(true)
        le.logger.Info("Became scheduling leader", "engine_id", le.engineID)

        // Block until session expires (leadership lost) or ctx cancelled
        select {
        case <-session.Done():
            le.isLeader.Store(false)
            le.logger.Warn("Lost leadership, re-campaigning", "engine_id", le.engineID)
            // Loop back to re-campaign
        case <-ctx.Done():
            le.isLeader.Store(false)
            election.Resign(context.Background())
            session.Close()
            return ctx.Err()
        }
    }
}

func (le *LeaderElection) IsLeader() bool { return le.isLeader.Load() }

func (le *LeaderElection) Resign() {
    le.isLeader.Store(false)
    // Session close handles resignation
}
```

#### 2. Add leader field to Engine
**File**: `pkg/engine/engine.go`

```go
type Engine struct {
    // existing fields...
    leader *LeaderElection
}
```

#### 3. Gate engineLoop on leadership
**File**: `pkg/engine/engine.go`

In `engineLoop()`:
```go
func (e *Engine) engineLoop(ctx context.Context) {
    ticker := time.NewTicker(3 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Single-engine: always run. Multi-engine: only if leader.
            if !e.multiEngine || e.leader.IsLeader() {
                e.processDeployments(ctx)
            }
            e.updateMetrics(ctx)
        }
    }
}
```

#### 4. Distributed lock per deployment during scheduling
**File**: `pkg/engine/engine.go`

In `schedulePendingDeployment()`, add deployment-level lock:
```go
func (e *Engine) schedulePendingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
    if e.multiEngine {
        lockStore := e.store.(storage.LockStore)
        unlock, err := lockStore.Lock(ctx, "locks/deploy/"+deployment.ID, 30*time.Second)
        if err != nil {
            return // another engine is handling this, or etcd issue
        }
        defer unlock()

        // Re-read deployment after acquiring lock (may have been scheduled already)
        var fresh types.DeploymentRecord
        if err := e.store.Get(ctx, types.KeyDeployments+deployment.ID, &fresh); err != nil {
            return
        }
        if fresh.Status != types.StatusPending {
            return // already scheduled by another engine
        }
        deployment = &fresh
    }

    // ... existing scheduling logic unchanged ...
}
```

Apply the same pattern to `checkDeployingDeployment()` and `checkStoppingDeployment()` — lock per deployment before status transitions.

#### 5. CAS for deployment status transitions
**File**: `pkg/engine/engine.go`

Add helper for safe status transitions:
```go
func (e *Engine) saveDeploymentStatus(ctx context.Context, key string, deployment *types.DeploymentRecord, revision int64) error {
    if e.multiEngine && revision > 0 {
        casStore := e.store.(storage.CASStore)
        return casStore.SaveIfRevision(ctx, key, deployment, revision)
    }
    return e.store.Save(ctx, key, deployment)
}
```

Update `schedulePendingDeployment`, `checkDeployingDeployment`, `checkStoppingDeployment`, `blueGreenTeardownOld` to use this helper.

#### 6. Engine Run() — start leader election
**File**: `pkg/engine/engine.go`

In `Run()`, after gRPC server starts:
```go
if e.multiEngine {
    etcdStore := e.store.(*storage.EtcdStore)
    e.leader = NewLeaderElection(etcdStore.Client(), e.engineID)
    go e.leader.Run(ctx)
}
```

#### 7. Graceful shutdown — resign leadership
**File**: `pkg/engine/engine.go`

In `Close()`:
```go
if e.leader != nil {
    e.leader.Resign()
}
```

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/engine/...` — leader election tests (become leader, resign, re-elect)
- [ ] `go test ./pkg/engine/...` — scheduling with lock tests (two mock engines, only one schedules)
- [ ] `go test ./pkg/engine/...` — CAS status transition tests (conflict detection)
- [ ] `golangci-lint run ./...` — no lint errors
- [x] Existing tests pass (single-engine code paths unchanged)

#### Manual Verification:
- [ ] Start two engines with same etcd — only one runs scheduling
- [ ] Kill leader — second engine becomes leader within 15s
- [ ] Deploy with two engines running — no duplicate tasks created
- [ ] `etcdctl get /banyan/leader/ --prefix` shows current leader
- [ ] Blue-green deployment works correctly with two engines

**Implementation Note**: This is the most critical phase. Pause for thorough manual verification before proceeding.

---

## Phase 6: Agent Multi-Endpoint Support

### Overview
Agents can be configured with multiple engine endpoints and failover to the next on disconnect.

### Changes Required

#### 1. AgentConfig — Add endpoints list
**File**: `pkg/types/config.go`

Add to `AgentConfig`:
```go
EngineEndpoints []string `yaml:"engine_endpoints,omitempty"`
```

Backward compat: if `EngineEndpoints` is empty, fall back to `EngineHost:EnginePort`.

#### 2. Agent EngineClient — Multi-endpoint
**File**: `pkg/agent/engine_client.go`

Add endpoint list and failover:
```go
type EngineClient struct {
    endpoints  []string
    currentIdx int
    conn       *grpc.ClientConn
    client     banyanpb.EngineServiceClient
    mu         sync.Mutex
}

func NewEngineClient(endpoints []string) (*EngineClient, error) {
    ec := &EngineClient{endpoints: endpoints}
    if err := ec.connectTo(0); err != nil {
        return nil, err
    }
    return ec, nil
}

func (ec *EngineClient) connectTo(idx int) error {
    ec.mu.Lock()
    defer ec.mu.Unlock()
    if ec.conn != nil {
        ec.conn.Close()
    }
    conn, err := grpc.NewClient(ec.endpoints[idx], grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        return err
    }
    ec.conn = conn
    ec.client = banyanpb.NewEngineServiceClient(conn)
    ec.currentIdx = idx
    return nil
}

func (ec *EngineClient) Failover() error {
    next := (ec.currentIdx + 1) % len(ec.endpoints)
    return ec.connectTo(next)
}
```

#### 3. Agent reconnection — Try all endpoints
**File**: `pkg/agent/agent.go`

In `reconnect()`, cycle through endpoints:
```go
func (a *Agent) reconnect(ctx context.Context) {
    for attempt := 0; ; attempt++ {
        for i := 0; i < len(a.client.endpoints); i++ {
            if err := a.client.Failover(); err != nil {
                continue
            }
            if err := a.client.Health(ctx); err != nil {
                continue
            }
            if err := a.register(ctx); err != nil {
                continue
            }
            return // success
        }
        backoff := min(reconnectBackoffInitial*(1<<attempt), reconnectBackoffMax)
        select {
        case <-time.After(backoff):
        case <-ctx.Done():
            return
        }
    }
}
```

#### 4. Agent init — Prompt for additional endpoints
**File**: `cmd/banyan-agent/cmd/agent.go`

After primary engine host/port:
```
Additional engine endpoints for HA (comma-separated, or leave empty):
```

#### 5. Agent start — Build endpoint list
**File**: `cmd/banyan-agent/cmd/agent.go`

Build endpoints from config, apply WireGuard tunnel override:
```go
var endpoints []string
if len(cfg.Agent.EngineEndpoints) > 0 {
    endpoints = cfg.Agent.EngineEndpoints
} else {
    endpoints = []string{primaryEndpoint}
}

if controlTunnelActive {
    for i, ep := range endpoints {
        _, port, _ := net.SplitHostPort(ep)
        endpoints[i] = types.ControlTunnelEngineIP + ":" + port
    }
}
```

### Success Criteria

#### Automated Verification:
- [x] `go test ./pkg/agent/...` — multi-endpoint client tests (failover, reconnect cycle)
- [ ] `go test ./cmd/banyan-agent/...` — endpoint list building tests
- [ ] `golangci-lint run ./...` — no lint errors
- [x] Existing tests pass (single endpoint = `[]string{endpoint}`)

#### Manual Verification:
- [ ] Agent with single endpoint: works exactly as before
- [ ] Agent with two endpoints: connects to first, fails over to second when first dies
- [ ] Agent re-registers on new engine after failover
- [ ] Heartbeat resumes on new engine

**Implementation Note**: Pause for manual verification.

---

## Phase 7: CLI Multi-Endpoint Support

### Overview
CLI can be configured with multiple engine endpoints for failover.

### Changes Required

#### 1. CLIConfig — Add endpoints list
**File**: `pkg/types/config.go`

Add to `CLIConfig`:
```go
EngineEndpoints    []string `yaml:"engine_endpoints,omitempty"`
EngineWGPublicKeys []string `yaml:"engine_wg_public_keys,omitempty"` // multi-engine WG keys
```

#### 2. NewAutoEngineClient — Try multiple endpoints
**File**: `cmd/banyan-cli/cmd/client.go`

```go
func NewAutoEngineClient(engineAddr string) (*EngineClient, error) {
    cfg, _ := types.LoadConfig(configPath)

    // Build endpoint list
    var endpoints []string
    if len(cfg.CLI.EngineEndpoints) > 0 {
        endpoints = cfg.CLI.EngineEndpoints
    } else {
        port := "50051"
        if cfg.CLI.EnginePort != "" {
            port = cfg.CLI.EnginePort
        }
        host := cfg.CLI.EngineHost
        if host == "" {
            host = "localhost"
        }
        endpoints = []string{host + ":" + port}
    }

    // WireGuard tunnel: replace hosts with tunnel IPs
    if cfg.CLI.EngineWGPublicKey != "" && controlTunnelExistsFn(types.ControlIfaceCLI) {
        for i, ep := range endpoints {
            _, port, _ := net.SplitHostPort(ep)
            endpoints[i] = controlTunnelEngineIP + ":" + port
        }
    }

    // Try each endpoint with health check
    var lastErr error
    for _, ep := range endpoints {
        client, err := NewEngineClient(ep)
        if err != nil {
            lastErr = err
            continue
        }
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
        err = client.Health(ctx)
        cancel()
        if err != nil {
            client.Close()
            lastErr = err
            continue
        }
        return client, nil
    }
    return nil, fmt.Errorf("failed to connect to any engine: %w", lastErr)
}
```

#### 3. CLI init — Prompt for multiple endpoints
**File**: `cmd/banyan-cli/cmd/init.go`

After engine host/port:
```
Additional engine endpoints for HA (comma-separated, or leave empty):
```

#### 4. WireGuard tunnel — Multiple engine peers
**File**: `cmd/banyan-cli/cmd/init.go`

For multi-engine, each engine has its own WireGuard public key. CLI tunnel needs multiple peers:
```go
for _, enginePubKey := range cfg.CLI.EngineWGPublicKeys {
    tunnelIP := types.TunnelIPFromPublicKey(enginePubKey)
    overlay.AddControlPeerExec(types.ControlIfaceCLI, enginePubKey, tunnelIP, engineEndpoint)
}
```

### Success Criteria

#### Automated Verification:
- [x] `go test ./cmd/banyan-cli/...` — multi-endpoint client tests
- [ ] `golangci-lint run ./...` — no lint errors
- [x] Existing tests pass (single endpoint backward compat)

#### Manual Verification:
- [ ] CLI with single endpoint: works exactly as before
- [ ] CLI with two endpoints: command succeeds when first engine is down
- [ ] `banyan-cli status` shows correct cluster state from either engine

---

## Testing Strategy

### Unit Tests
- **Storage**: CAS (success, conflict, concurrent), Lock (acquire, release, auto-expire, contention)
- **Leader election**: Campaign, resign, re-elect, session expiry
- **SubnetAllocator (etcd)**: Allocate, release, concurrent allocation, idempotent
- **PeerTracker (etcd)**: Update, remove, get excluding, empty
- **Agent client**: Multi-endpoint constructor, failover, reconnect cycle
- **CLI client**: Multi-endpoint resolution, health check failover
- **Registry**: Managed subprocess start/stop, health check, config generation

### Integration Tests (E2E)
- **Registry persistence**: Push image, restart engine, pull image still works
- **Two engines**: Deploy, status, down all work correctly
- **Leader failover**: Kill leader, verify new leader within 15s, scheduling resumes
- **Agent failover**: Kill agent's engine, verify reconnection to other engine
- **Split brain prevention**: Two engines, verify no duplicate tasks
- **Registry in multi-engine**: Both engines use same external registry, push+pull works

### Backward Compatibility
- All existing E2E tests must pass unchanged in single-engine mode
- No config migration needed (new fields are optional with `omitempty`)
- Old configs without `managed_registry` field: default to in-memory fallback (backward compat)

## Performance Considerations

- **Leader election**: Uses etcd session with 15s TTL. Failover takes ~15s (session expiry + re-election)
- **CAS overhead**: One extra etcd round-trip per status transition. Negligible for scheduling (every 3s)
- **Distributed locks**: 10-30s TTL. Lock contention rare (only during concurrent deploys of same app)
- **etcd-backed allocator/peers**: ~1ms per operation. Acceptable for register/heartbeat RPCs
- **Managed registry**: Distribution registry adds ~30MB memory, <10ms per pull request

## Migration Notes

- **No data migration**: New etcd keys are additive. Existing keys unchanged.
- **Config backward-compatible**: All new YAML fields have `omitempty`. Existing configs work without changes.
- **Registry migration**: Old in-memory images are lost on upgrade (users re-deploy once). New managed registry persists.
- **Rolling upgrade path**: Upgrade engines one at a time. Single-engine behavior preserved until multi-engine explicitly enabled.
- **Rollback**: Disable multi-engine in config, restart. Falls back to single-engine behavior.
- **Install script**: Must run `install.sh` again to get the Distribution registry binary.

## References

- Research document: `docs/plans/m7-multi-engine-ha-research.md`
- Distribution registry: `github.com/distribution/distribution` v2.8.3
- etcd concurrency package: `go.etcd.io/etcd/client/v3/concurrency`
- etcd leader election: `concurrency.Election` with campaign/resign
- etcd distributed lock: `concurrency.Mutex` with session TTL
- Managed etcd pattern: `cmd/banyan-engine/cmd/engine.go:544-611`
