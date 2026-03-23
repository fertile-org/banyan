# Milestone 9: Auto-Scaling & Workload Rebalancing — Implementation Plan

## Overview

Combine auto-scaling and workload rebalancing into a single milestone. Both features share the same prerequisites: per-container metrics, incremental task management, and graceful container drain. Auto-scaling answers "how many replicas?" and rebalancing answers "which agent runs them?"

## Current State

- **Metrics**: Host-level CPU/memory collected every 15s via heartbeat. **No per-container metrics.**
- **Scaling**: Requires full blue-green redeployment. No incremental task add/remove. No `scale` command.
- **Drain**: Proxy removed instantly, container killed. No graceful drain period.
- **Container stats**: `nerdctl stats --no-stream` available but not used.
- **Rebalancing**: None. Containers stay on their initially scheduled agent forever.

## Desired End State

```yaml
name: my-app

services:
  api:
    image: myapp/api
    deploy:
      replicas: 2
      autoscale:
        min: 2
        max: 10
        target_cpu: 70        # percent
        cooldown: 60s
    ports:
      - "8080:8080"

  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data
    deploy:
      placement:
        node: db-*            # pinned — won't be rebalanced
```

```bash
# Manual scaling
banyan-cli scale my-app api=5

# Auto-scaling happens automatically based on target_cpu
# Rebalancing happens automatically when agents are imbalanced
```

### Verification
- Per-container CPU/memory visible in `banyan-cli container` and Prometheus
- `banyan-cli scale` adds/removes replicas without full redeploy
- Auto-scale: deploy with `target_cpu: 70`, generate load, verify replicas increase
- Rebalance: overload one agent, verify containers migrate to less-loaded agents
- Graceful drain: stop a container, verify no dropped requests during transition

## What We're NOT Doing

- **Custom metrics** (HTTP request rate, queue depth) — only CPU/memory for v1
- **Scale-to-zero** — min replicas must be >= 1
- **Predictive scaling** — reactive only (based on current metrics)
- **Cross-deployment rebalancing** — only within a single deployment
- **Stateful container migration** — skip containers with volumes
- **Vertical scaling** (changing CPU/memory limits) — horizontal only

## Implementation Approach

Build shared prerequisites first (metrics, incremental tasks, drain), then add scaling and rebalancing as decision layers on top.

```
Phase 1: Per-container metrics  ← foundation
Phase 2: Incremental tasks      ← foundation
Phase 3: Graceful drain         ← foundation
Phase 4: Auto-scale rules       ← uses 1+2+3
Phase 5: Workload rebalancing   ← uses 1+2+3
Phase 6: Tests & documentation
```

---

## Phase 1: Per-Container Metrics

### Overview
Agent collects per-container CPU and memory usage via `nerdctl stats --no-stream` and reports it alongside container health. Engine stores metrics on TaskRecord for auto-scaling decisions.

### Changes Required

#### 1. Container metrics collection
**File**: `pkg/agent/agent.go`

Add to `checkContainerHealth()` — collect metrics alongside status:

```go
// After getting container status, also get resource usage
type ContainerMetrics struct {
    CPUPercent  float64
    MemoryUsed uint64
    MemoryLimit uint64
}

// collectContainerMetrics runs "nerdctl stats --no-stream --format json"
// for all tracked containers in a single call.
var containerMetricsCollector = collectContainerMetrics

func collectContainerMetrics(ctx context.Context, names []string) map[string]ContainerMetrics {
    // nerdctl stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}'
    // Parse output into per-container metrics
}
```

#### 2. Proto: Add metrics to ContainerStatus
**File**: `pkg/rpc/proto/banyan/v1/engine.proto`

```proto
message ContainerStatus {
    string container_name = 1;
    string status = 2;
    string ip = 3;
    string health_status = 4;
    // NEW:
    double cpu_percent = 5;         // 0.0-100.0
    uint64 memory_used_bytes = 6;
    uint64 memory_limit_bytes = 7;
}
```

#### 3. Engine: Store metrics on TaskRecord
**File**: `pkg/engine/grpc_server.go` — `ReportContainerHealth()` handler

Store `cpu_percent` and `memory_used_bytes` on the TaskRecord alongside existing status/IP/health fields.

#### 4. TaskRecord: Add metrics fields
**File**: `pkg/types/records.go`

```go
type TaskRecord struct {
    // ...existing fields...
    CPUPercent      float64 `json:"cpu_percent,omitempty"`
    MemoryUsedBytes uint64  `json:"memory_used_bytes,omitempty"`
    MemoryLimitBytes uint64 `json:"memory_limit_bytes,omitempty"`
}
```

#### 5. Prometheus: Per-container metrics
**File**: `pkg/metrics/registry.go`

```go
banyan_container_cpu_percent{deployment="...", service="...", container="...", agent="..."}
banyan_container_memory_used_bytes{deployment="...", service="...", container="...", agent="..."}
```

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/agent/...` — container metrics collection (mock nerdctl stats)
- [ ] `go test ./pkg/engine/...` — metrics stored on TaskRecord
- [ ] `go test ./pkg/types/...` — new fields serialize correctly
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] `banyan-cli container` shows CPU/memory per container
- [ ] Prometheus endpoint shows per-container metrics

---

## Phase 2: Incremental Task Operations

### Overview
Add the ability to add or remove individual replicas from a running deployment without blue-green redeployment. New `Scale` RPC and `banyan-cli scale` command.

### Changes Required

#### 1. Scale RPC
**File**: `pkg/rpc/proto/banyan/v1/engine.proto`

```proto
message ScaleRequest {
    string name = 1;                      // deployment name
    map<string, int32> replicas = 2;      // service → target replica count
    repeated string tags = 3;
}

message ScaleResponse {
    string deployment_id = 1;
    map<string, int32> previous = 2;      // service → old count
    map<string, int32> current = 3;       // service → new count
}

service EngineService {
    // ...existing RPCs...
    rpc Scale(ScaleRequest) returns (ScaleResponse);
}
```

#### 2. Engine: Scale handler
**File**: `pkg/engine/grpc_server.go`

```go
func (s *engineGRPCServer) Scale(ctx, req) (*ScaleResponse, error) {
    // 1. Find running deployment by name + tags
    // 2. For each service in req.replicas:
    //    a. current = count existing completed create_and_start tasks
    //    b. target = req.replicas[service]
    //    c. If target > current: create (target - current) new tasks
    //    d. If target < current: create (current - target) stop tasks for excess replicas
    // 3. Update deployment.Services[service].Replicas = target
    // 4. Return previous and current counts
}
```

Key design: Scale modifies the RUNNING deployment in-place. No new deployment ID. No blue-green. Tasks are added/removed individually.

#### 3. CLI: banyan-cli scale command
**File**: `cmd/banyan-cli/cmd/scale.go` (new file)

```bash
banyan-cli scale my-app api=5 web=3
banyan-cli scale my-app api=5 --tags env:prod
```

#### 4. Agent: Handle new tasks on running deployment
No agent changes needed — agents already poll for pending tasks and execute them. New create_and_start tasks will be picked up. Stop tasks will also be picked up.

#### 5. CLI client: Scale RPC method
**File**: `cmd/banyan-cli/cmd/client.go`

Add `Scale()` method to EngineClient.

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/engine/...` — Scale RPC: scale up, scale down, no-op, invalid service
- [ ] `go test ./cmd/banyan-cli/...` — scale command parsing
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] `banyan-cli scale my-app api=3` scales from 2 to 3 replicas (new container appears)
- [ ] `banyan-cli scale my-app api=1` scales from 3 to 1 (2 containers removed)
- [ ] Scaled deployment stays in RUNNING status (no DEPLOYING transition)
- [ ] New replicas get DNS/proxy registration within 15s

---

## Phase 3: Graceful Drain

### Overview
When stopping a container (scale down, rebalance, or redeploy), remove it from the load balancer first, wait for in-flight requests to complete, then stop the container.

### Changes Required

#### 1. Drain period on stop tasks
**File**: `pkg/agent/agent.go`

Before executing a `stop_and_remove` task:

```go
func (a *Agent) drainAndStop(ctx context.Context, task *types.TaskRecord) error {
    // 1. Remove from proxy (no new traffic)
    if a.proxy != nil {
        a.proxy.RemoveBackend(task.ContainerName)
    }

    // 2. Remove from DNS
    if a.dnsManager != nil {
        // unregister this container's DNS entries
    }

    // 3. Wait drain period (default 5s, configurable via manifest)
    drainPeriod := 5 * time.Second
    select {
    case <-time.After(drainPeriod):
    case <-ctx.Done():
    }

    // 4. Stop container
    return commandRunner(ctx, "nerdctl", "rm", "-f", task.ContainerName)
}
```

#### 2. Manifest: drain period config
**File**: `pkg/types/manifest.go`

```go
type ManifestDeploy struct {
    // ...existing fields...
    StopGracePeriod string `yaml:"stop_grace_period,omitempty"` // Docker Compose compatible
}
```

This matches Docker Compose's `stop_grace_period` field.

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/agent/...` — drain removes proxy before stopping container
- [ ] `go test ./pkg/types/...` — stop_grace_period parses correctly

#### Manual Verification:
- [ ] Scale down: existing connections complete before container stops
- [ ] No dropped requests during blue-green redeployment

---

## Phase 4: Auto-Scale Rules

### Overview
Engine evaluates per-container metrics against scaling rules defined in the manifest. Scales up when over threshold, scales down when under.

### Changes Required

#### 1. Manifest: autoscale config
**File**: `pkg/types/manifest.go`

```go
type ManifestDeploy struct {
    // ...existing fields...
    Autoscale *ManifestAutoscale `yaml:"autoscale,omitempty"`
}

type ManifestAutoscale struct {
    Min       int    `yaml:"min"`           // minimum replicas (default: deploy.replicas)
    Max       int    `yaml:"max"`           // maximum replicas
    TargetCPU int    `yaml:"target_cpu"`    // target CPU percentage (e.g., 70)
    Cooldown  string `yaml:"cooldown"`      // min time between scale events (e.g., "60s")
}
```

#### 2. ServiceRecord and TaskRecord: carry autoscale config
**Files**: `pkg/types/records.go`, `pkg/types/helpers.go`

Pass autoscale config through the pipeline so the engine can evaluate it.

#### 3. Proto: autoscale fields
**File**: `pkg/rpc/proto/banyan/v1/engine.proto`

Add autoscale to ManifestDeploy and ManifestService proto messages.

#### 4. Engine: auto-scale evaluation loop
**File**: `pkg/engine/engine.go`

Add `evaluateAutoscale()` to the engine loop (runs every 30s):

```go
func (e *Engine) evaluateAutoscale(ctx context.Context) {
    // For each RUNNING deployment with autoscale config:
    //   1. Collect per-container CPU metrics from TaskRecords
    //   2. Calculate average CPU across replicas of each service
    //   3. If avg > target_cpu and replicas < max: scale up by 1
    //   4. If avg < target_cpu * 0.5 and replicas > min: scale down by 1
    //   5. Respect cooldown period (don't scale again within cooldown)
    //   6. Use the Scale() handler internally to add/remove tasks
}
```

Scaling algorithm (simple reactive):
- **Scale up**: avg CPU > `target_cpu` → add 1 replica (up to `max`)
- **Scale down**: avg CPU < `target_cpu * 0.5` → remove 1 replica (down to `min`)
- **Cooldown**: minimum `cooldown` duration between consecutive scale events per service
- **Hysteresis**: scale-down threshold is half of scale-up threshold (prevents flapping)

#### 5. Engine: track last scale event
**File**: `pkg/types/records.go`

```go
type ServiceRecord struct {
    // ...existing fields...
    LastScaleAt time.Time `json:"last_scale_at,omitempty"`
}
```

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/types/...` — autoscale config parsing
- [ ] `go test ./pkg/engine/...` — scale up when CPU > target, scale down when CPU < target/2
- [ ] `go test ./pkg/engine/...` — cooldown period respected
- [ ] `go test ./pkg/engine/...` — min/max bounds enforced
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] Deploy with `target_cpu: 70`, generate CPU load, verify replicas increase
- [ ] Stop load, verify replicas decrease after cooldown
- [ ] Replicas stay within min/max bounds

---

## Phase 5: Workload Rebalancing

### Overview
Engine detects when agents are unevenly loaded and migrates stateless containers from overloaded agents to underloaded ones.

### Changes Required

#### 1. Engine: rebalance evaluation
**File**: `pkg/engine/engine.go`

Add `evaluateRebalance()` to the engine loop (runs every 60s):

```go
func (e *Engine) evaluateRebalance(ctx context.Context) {
    // 1. Get all agents with metrics (CPU usage)
    // 2. Calculate average cluster CPU
    // 3. Find overloaded agents (CPU > 85%)
    // 4. Find underloaded agents (CPU < 40%)
    // 5. For each overloaded agent:
    //    a. Find stateless containers (no volumes, no placement constraint)
    //    b. Pick the container using the least CPU
    //    c. Create stop task on overloaded agent
    //    d. Create start task on underloaded agent
    //    e. Update TaskRecord.AgentID
    // 6. Limit: max 1 migration per cycle per agent (prevent thundering herd)
}
```

#### 2. Detect stateless containers
A container is "migratable" if:
- Service has no `deploy.placement.node` (not pinned)
- Service has no `volumes` (data would be lost)
- Container is `running` and `completed` status

#### 3. Migration execution
Migration = stop on old agent + start on new agent:

```go
func (e *Engine) migrateTask(ctx context.Context, task *TaskRecord, fromAgent, toAgent string) error {
    // 1. Create stop task on fromAgent (with graceful drain)
    // 2. Create start task on toAgent (same image, env, ports, etc.)
    // 3. The start task gets a new replica index suffix
    // 4. Update deployment's task count tracking
}
```

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/engine/...` — rebalance triggers when agent CPU > 85%
- [ ] `go test ./pkg/engine/...` — skip containers with volumes or placement
- [ ] `go test ./pkg/engine/...` — max 1 migration per cycle
- [ ] `golangci-lint run ./...` — no lint errors

#### Manual Verification:
- [ ] Overload one agent with CPU stress, verify container migrates to another
- [ ] Containers with volumes are NOT migrated
- [ ] Containers with placement constraints are NOT migrated

---

## Phase 6: Tests & Documentation

### Unit Tests
- Container metrics collection (mock nerdctl stats output)
- Scale RPC: up, down, bounds, invalid service
- Graceful drain: proxy removal before stop
- Auto-scale evaluation: CPU thresholds, cooldown, min/max
- Rebalance evaluation: imbalance detection, stateless filter, migration limit

### E2E Tests
- Deploy with auto-scale config, verify replicas adjust under load
- `banyan-cli scale` command: scale up and down
- Rebalance: verify container moves when agent is overloaded

### Documentation
- Manifest reference: `deploy.autoscale`, `deploy.stop_grace_period`
- CLI reference: `banyan-cli scale`
- Guide: "Auto-Scaling" with examples
- Roadmap: Mark Milestone 9 as Done

### Success Criteria

#### Automated Verification:
- [ ] All unit tests pass with > 80% coverage on new code
- [ ] E2E tests pass
- [ ] `golangci-lint run ./...` — no lint errors

---

## Performance Considerations

- **nerdctl stats**: ~50ms per call for all containers. Called every 10s (health check interval). Acceptable.
- **Auto-scale evaluation**: Runs every 30s. Reads TaskRecords from etcd (already cached by orchestration loop). Negligible overhead.
- **Rebalance evaluation**: Runs every 60s. Same data source. Negligible.
- **Graceful drain**: Default 5s wait. Adds 5s to scale-down and blue-green transitions.
- **Migration overhead**: Stop + start takes ~10s (image already pulled). Acceptable for 60s evaluation cycle.

## Migration Notes

- **No data migration** — new fields are additive with `omitempty`
- **Proto backward-compatible** — new fields are additive (proto3 defaults)
- **Existing deployments**: No auto-scaling until manifest is updated with `autoscale` config
- **No rebalancing for pinned services**: `deploy.placement.node` prevents migration

## References

- Research: per-container metrics via nerdctl stats, cgroup v2 alternative
- Current metrics system: `pkg/metrics/collector.go`, `pkg/agent/agent.go:checkContainerHealth()`
- Current task lifecycle: `pkg/engine/engine.go:processDeployments()`, `pkg/types/helpers.go:BuildTasksForDeployment()`
- Current proxy: `pkg/proxy/proxy.go`, `pkg/agent/agent.go:setupProxyForContainer()`
- Docker Compose: `stop_grace_period` field compatibility
