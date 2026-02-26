# Milestone 4: CLI Dashboard & Prometheus Metrics

## Overview

A live terminal dashboard for cluster monitoring built with charmbracelet/bubbletea, backed by Prometheus-native metrics throughout the entire stack.

**Two deliverables in one milestone:**

1. **Prometheus-native metrics** — Engine and agents expose all metrics in Prometheus format. Teams plug their existing Prometheus + Grafana stack in, zero custom exporters needed.
2. **CLI dashboard** — `banyan-cli dashboard` opens an interactive TUI that displays the same metrics in real-time.

### Why Prometheus-native matters

Kubernetes requires three separate exporters to get comparable observability:
- `kube-state-metrics` for cluster state
- `node-exporter` for system metrics
- `cAdvisor` for container metrics

Banyan: **one scrape target** (the engine), everything included. Point Prometheus at `<engine>:<api_port>/metrics` and get full cluster observability.

This is a genuine competitive advantage — no dashboard to install, no exporters to configure, no Helm charts to deploy. It just works.

---

## UI Design

### Screen 1: Overview (default)

The "is everything OK?" screen. An engineer opens this and knows in 2 seconds.

```
╭─ Banyan Dashboard ────────────────────────────────────────── ↻ 3s ─╮
│                                                                      │
│  ╭─ Engine ────────────────────╮  ╭─ Cluster ──────────────────────╮│
│  │ ● Running    2d 14h 23m    │  │ Agents       3/3 ●●●           ││
│  │ CPU  ████░░░░░░ 42%        │  │ Deployments  2 running         ││
│  │ Mem  ██░░░░░░░░ 18%        │  │ Containers   8/8 healthy       ││
│  │ Disk █████░░░░░ 52%        │  │ Tasks        12 ok · 0 failed  ││
│  ╰─────────────────────────────╯  ╰────────────────────────────────╯│
│                                                                      │
│  ╭─ Agents ──────────────────────────────────────────────────────╮  │
│  │   Name       Status  Tags          CPU        Mem      Cntrs │  │
│  │ ▸ worker-1   ● up    gpu, east    ████░ 42%  ██░ 28%    4   │  │
│  │   worker-2   ● up    west         ██░░░ 23%  ███ 35%    3   │  │
│  │   worker-3   ● up    east         █░░░░ 11%  █░░ 15%    1   │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ Deployments ─────────────────────────────────────────────────╮  │
│  │   Name       Status     Healthy  Services  Tags      Age     │  │
│  │ ▸ web-app    ● running  3/3      2         east      2d 5h   │  │
│  │   api-svc    ● running  2/2      1         west      1d 3h   │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ Recent Events ───────────────────────────────────────────────╮  │
│  │ 12:34:56  ✓ web-app-web-2 started on worker-3               │  │
│  │ 12:34:50  ↻ Deployment web-app redeployed (blue-green)      │  │
│  │ 12:30:12  + worker-3 connected (tags: east)                 │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  1 Overview  2 Agents  3 Deploys  4 Containers    / Search  ? Help  │
╰──────────────────────────────────────────────────────────────────────╯
```

### Screen 2: Agent Detail

Drill into a specific agent. Press Enter on an agent row, or press `2`.

```
╭─ Banyan ▸ worker-1 ──────────────────────────────────────── ↻ 3s ─╮
│                                                                      │
│  ╭─ Info ────────────────────────────────────────────────────────╮  │
│  │ Status     ● ready           Last Seen    2s ago              │  │
│  │ Tags       gpu, us-east      Created      5d 12h ago         │  │
│  │ Subnet     10.0.45.0/24      Address      192.168.1.10       │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ Resources ───────────────────────────────────────────────────╮  │
│  │ CPU    ████████████░░░░░░░░░░░░░░░░░░░░ 42%     4 cores     │  │
│  │ Memory ██████████░░░░░░░░░░░░░░░░░░░░░░ 28%     2.2/8.0 GB  │  │
│  │ Disk   ████████████████░░░░░░░░░░░░░░░░ 52%     41/80 GB    │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ Containers (4) ──────────────────────────────────────────────╮  │
│  │   Container          Service   Status     IP          Ports  │  │
│  │ ▸ web-app-web-0      web       ● running  10.0.45.2   :80   │  │
│  │   web-app-web-1      web       ● running  10.0.45.3   :80   │  │
│  │   web-app-db-0       db        ● running  10.0.45.4   :5432 │  │
│  │   api-svc-api-0      api       ● running  10.0.45.5   :8080 │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  Esc Back   ↑↓ Navigate   Enter Detail   r Refresh                  │
╰──────────────────────────────────────────────────────────────────────╯
```

### Screen 3: Deployment Detail

Drill into a deployment. Press Enter on a deployment row, or press `3`.

```
╭─ Banyan ▸ web-app ───────────────────────────────────────── ↻ 3s ─╮
│                                                                      │
│  ╭─ Deployment ──────────────────────────────────────────────────╮  │
│  │ Status     ● running          Strategy   blue-green           │  │
│  │ Created    2d 5h ago          Tags       us-east              │  │
│  │ Healthy    3/3                Updated    5m ago               │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ web  nginx:latest  ×2 replicas ──────────────────────────────╮  │
│  │   Container          Agent      Status     IP          Ports │  │
│  │   web-app-web-0      worker-1   ● running  10.0.45.2   :80  │  │
│  │   web-app-web-1      worker-2   ● running  10.0.46.2   :80  │  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  ╭─ db  postgres:15  ×1 replica ─────────────────────────────────╮  │
│  │   Container          Agent      Status     IP          Ports │  │
│  │   web-app-db-0       worker-1   ● running  10.0.45.4   :5432│  │
│  ╰───────────────────────────────────────────────────────────────╯  │
│                                                                      │
│  Esc Back   ↑↓ Navigate   Enter Container Detail                    │
╰──────────────────────────────────────────────────────────────────────╯
```

### Screen 4: All Containers

Flat list of every container across all agents. Press `4`.

```
╭─ Banyan ▸ Containers ────────────────────────────────────── ↻ 3s ─╮
│                                                                      │
│  Total: 8   Healthy: 8/8   Filter: all                              │
│                                                                      │
│  ╭──────────────────────────────────────────────────────────────╮   │
│  │   Container           Agent     Service  Status     IP       │   │
│  │ ▸ api-svc-api-0       worker-1  api      ● running  .45.5   │   │
│  │   api-svc-api-1       worker-2  api      ● running  .46.5   │   │
│  │   api-svc-redis-0     worker-2  redis    ● running  .46.6   │   │
│  │   api-svc-worker-0    worker-3  worker   ● running  .47.3   │   │
│  │   web-app-db-0        worker-1  db       ● running  .45.4   │   │
│  │   web-app-web-0       worker-1  web      ● running  .45.2   │   │
│  │   web-app-web-1       worker-2  web      ● running  .46.2   │   │
│  │   web-app-web-2       worker-3  web      ● running  .47.2   │   │
│  ╰──────────────────────────────────────────────────────────────╯   │
│                                                                      │
│  Esc Back   ↑↓ Navigate   Enter Detail   / Filter   l Logs         │
╰──────────────────────────────────────────────────────────────────────╯
```

### Visual Design System

**Status colors** (consistent everywhere):

| State | Color | Symbol |
|---|---|---|
| Running / Healthy / Connected | Green (#42) | `●` |
| Pending / Deploying | Yellow (#214) | `●` |
| Failed / Error / Disconnected | Red (#196) | `●` |
| Stopped / Inactive | Gray (#241) | `●` |

**Resource bars** (adaptive color by severity):

```
  0-50%  → Green   ████░░░░░░
 50-80%  → Yellow  ████████░░
 80-100% → Red     █████████░
```

**Keyboard navigation**:

| Key | Action |
|---|---|
| `1` `2` `3` `4` | Switch views: Overview / Agents / Deploys / Containers |
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `Enter` | Drill into selected item |
| `Esc` | Back to parent view |
| `q` | Quit dashboard |
| `/` | Filter/search (future) |
| `r` | Force refresh |
| `?` | Toggle help overlay |
| `Tab` | Cycle focus between panels (overview only) |

---

## Metrics Architecture — Prometheus Native

### Core Principle

Every metric in Banyan is defined once using `prometheus/client_golang`. The same metrics are:
1. Exposed at `/metrics` for external Prometheus scraping
2. Read by the gRPC `GetDashboardData` handler for the CLI dashboard
3. Available for future web dashboard via the same `/metrics` endpoint

No custom metrics formats. No translation layers. Prometheus IS the metrics backbone.

### Metric Catalog

All metrics follow Prometheus naming conventions: `banyan_<component>_<metric>_<unit>`.

#### Engine System Metrics

Exposed on the engine's `/metrics` HTTP endpoint.

```
# HELP banyan_engine_info Engine metadata.
# TYPE banyan_engine_info gauge
banyan_engine_info{version="0.1.0"} 1

# HELP banyan_engine_uptime_seconds Seconds since engine started.
# TYPE banyan_engine_uptime_seconds gauge
banyan_engine_uptime_seconds 123456

# HELP banyan_engine_cpu_usage_ratio Engine host CPU usage (0.0–1.0).
# TYPE banyan_engine_cpu_usage_ratio gauge
banyan_engine_cpu_usage_ratio 0.42

# HELP banyan_engine_memory_used_bytes Engine host memory in use.
# TYPE banyan_engine_memory_used_bytes gauge
banyan_engine_memory_used_bytes 3221225472

# HELP banyan_engine_memory_total_bytes Engine host total memory.
# TYPE banyan_engine_memory_total_bytes gauge
banyan_engine_memory_total_bytes 8589934592

# HELP banyan_engine_disk_used_bytes Engine host disk usage.
# TYPE banyan_engine_disk_used_bytes gauge
banyan_engine_disk_used_bytes 42949672960

# HELP banyan_engine_disk_total_bytes Engine host total disk.
# TYPE banyan_engine_disk_total_bytes gauge
banyan_engine_disk_total_bytes 85899345920

# HELP banyan_engine_cpu_cores Engine host CPU core count.
# TYPE banyan_engine_cpu_cores gauge
banyan_engine_cpu_cores 4
```

#### Cluster Metrics (on engine)

Aggregated from all agents, updated on every heartbeat/health report.

```
# HELP banyan_cluster_agents_total Total registered agents.
# TYPE banyan_cluster_agents_total gauge
banyan_cluster_agents_total 3

# HELP banyan_cluster_agents_connected Currently connected agents.
# TYPE banyan_cluster_agents_connected gauge
banyan_cluster_agents_connected 3

# HELP banyan_cluster_deployments_total Total deployments.
# TYPE banyan_cluster_deployments_total gauge
banyan_cluster_deployments_total{status="running"} 2
banyan_cluster_deployments_total{status="stopped"} 1

# HELP banyan_cluster_containers_total Total containers.
# TYPE banyan_cluster_containers_total gauge
banyan_cluster_containers_total 8

# HELP banyan_cluster_containers_healthy Currently healthy containers.
# TYPE banyan_cluster_containers_healthy gauge
banyan_cluster_containers_healthy 8

# HELP banyan_cluster_tasks_total Total tasks by status.
# TYPE banyan_cluster_tasks_total gauge
banyan_cluster_tasks_total{status="completed"} 12
banyan_cluster_tasks_total{status="failed"} 0
banyan_cluster_tasks_total{status="pending"} 0
```

#### Per-Agent Metrics (on engine, labeled by agent)

Agents report these in heartbeat. Engine aggregates with agent label.

```
# HELP banyan_agent_cpu_usage_ratio Agent CPU usage (0.0–1.0).
# TYPE banyan_agent_cpu_usage_ratio gauge
banyan_agent_cpu_usage_ratio{agent="worker-1"} 0.42
banyan_agent_cpu_usage_ratio{agent="worker-2"} 0.23
banyan_agent_cpu_usage_ratio{agent="worker-3"} 0.11

# HELP banyan_agent_memory_used_bytes Agent memory in use.
# TYPE banyan_agent_memory_used_bytes gauge
banyan_agent_memory_used_bytes{agent="worker-1"} 2362232012
banyan_agent_memory_used_bytes{agent="worker-2"} 3006477107

# HELP banyan_agent_memory_total_bytes Agent total memory.
# TYPE banyan_agent_memory_total_bytes gauge
banyan_agent_memory_total_bytes{agent="worker-1"} 8589934592
banyan_agent_memory_total_bytes{agent="worker-2"} 8589934592

# HELP banyan_agent_disk_used_bytes Agent disk usage.
# TYPE banyan_agent_disk_used_bytes gauge
banyan_agent_disk_used_bytes{agent="worker-1"} 42949672960

# HELP banyan_agent_disk_total_bytes Agent total disk.
# TYPE banyan_agent_disk_total_bytes gauge
banyan_agent_disk_total_bytes{agent="worker-1"} 85899345920

# HELP banyan_agent_cpu_cores Agent CPU core count.
# TYPE banyan_agent_cpu_cores gauge
banyan_agent_cpu_cores{agent="worker-1"} 4

# HELP banyan_agent_containers_total Containers on this agent.
# TYPE banyan_agent_containers_total gauge
banyan_agent_containers_total{agent="worker-1"} 4
banyan_agent_containers_total{agent="worker-2"} 3

# HELP banyan_agent_info Agent metadata.
# TYPE banyan_agent_info gauge
banyan_agent_info{agent="worker-1",status="ready",subnet="10.0.45.0/24"} 1
```

#### Per-Deployment Metrics (on engine)

```
# HELP banyan_deployment_info Deployment metadata.
# TYPE banyan_deployment_info gauge
banyan_deployment_info{deployment="web-app",status="running",strategy="blue-green"} 1

# HELP banyan_deployment_replicas_desired Desired replica count per service.
# TYPE banyan_deployment_replicas_desired gauge
banyan_deployment_replicas_desired{deployment="web-app",service="web"} 2
banyan_deployment_replicas_desired{deployment="web-app",service="db"} 1

# HELP banyan_deployment_replicas_healthy Healthy replica count per service.
# TYPE banyan_deployment_replicas_healthy gauge
banyan_deployment_replicas_healthy{deployment="web-app",service="web"} 2
banyan_deployment_replicas_healthy{deployment="web-app",service="db"} 1
```

#### Event Counters (on engine)

```
# HELP banyan_events_total Cumulative event count by type.
# TYPE banyan_events_total counter
banyan_events_total{type="agent.connected"} 5
banyan_events_total{type="agent.disconnected"} 2
banyan_events_total{type="deployment.started"} 8
banyan_events_total{type="deployment.running"} 7
banyan_events_total{type="deployment.failed"} 1
banyan_events_total{type="task.completed"} 24
banyan_events_total{type="task.failed"} 1
```

#### Future: Per-Container Metrics (on agent)

Deferred to Phase 5+. Would come from nerdctl stats or cgroup reading.

```
# HELP banyan_container_cpu_usage_ratio Container CPU usage (0.0–1.0).
# TYPE banyan_container_cpu_usage_ratio gauge
banyan_container_cpu_usage_ratio{container="web-app-web-0",service="web",agent="worker-1"} 0.15

# HELP banyan_container_memory_used_bytes Container memory in use.
# TYPE banyan_container_memory_used_bytes gauge
banyan_container_memory_used_bytes{container="web-app-web-0",service="web",agent="worker-1"} 134217728
```

### Data Flow

```
                      ┌──────────────┐
                      │  Prometheus  │
                      │  (external)  │
                      └──────┬───────┘
                             │ scrapes /metrics
                             ▼
┌─────────┐  Heartbeat    ┌──────────┐  /metrics (HTTP)
│ Agent 1  │──────────────▸│          │◂──────────────── Prometheus
│ Agent 2  │──────────────▸│  Engine  │
│ Agent 3  │──────────────▸│          │◂──────────────── CLI Dashboard
└─────────┘  + metrics    └──────────┘  GetDashboardData (gRPC)
                             │
                  ┌──────────┴──────────┐
                  │ prometheus/client_golang │
                  │ registry (single source │
                  │ of truth for all metrics)│
                  └─────────────────────────┘
```

1. **Agents** collect system metrics locally (CPU/RAM/disk via `/proc` + `syscall`)
2. **Agents** send metrics snapshot in `HeartbeatRequest.system_metrics`
3. **Engine** stores agent metrics in prometheus registry (labeled by agent name)
4. **Engine** collects its own system metrics and cluster-level aggregates
5. **Engine** exposes `/metrics` HTTP endpoint via `promhttp.Handler()`
6. **CLI Dashboard** calls `GetDashboardData` gRPC → engine reads from same prometheus registry
7. **External Prometheus** scrapes engine's `/metrics` for alerting, dashboards, long-term storage

### Metrics Package: `pkg/metrics/`

New Go module at `pkg/metrics/`.

```
pkg/metrics/
├── go.mod                  # depends on prometheus/client_golang
├── collector.go            # SystemCollector — CPU, memory, disk from /proc
├── collector_test.go       # unit tests with fixtures
├── registry.go             # Prometheus metric definitions + engine registry
├── registry_test.go        # unit tests
└── types.go                # SystemMetrics struct (for heartbeat transport)
```

**Key types:**

```go
// SystemMetrics is the transport format for heartbeat reporting.
// Values mirror what prometheus gauges hold.
type SystemMetrics struct {
    CPUUsageRatio    float64 // 0.0-1.0
    MemoryUsedBytes  uint64
    MemoryTotalBytes uint64
    DiskUsedBytes    uint64
    DiskTotalBytes   uint64
    CPUCores         uint32
}

// SystemCollector reads system metrics from /proc and syscall.
// Thread-safe. CPU measurement uses delta between samples.
type SystemCollector struct { ... }

func NewSystemCollector() *SystemCollector
func (c *SystemCollector) Collect() SystemMetrics

// EngineMetricsRegistry holds all prometheus metrics for the engine.
// Provides methods to update metrics from heartbeats and internal state.
type EngineMetricsRegistry struct {
    registry *prometheus.Registry
    // engine system metrics
    // per-agent metrics (labeled)
    // cluster aggregate metrics
    // deployment metrics
    // event counters
}

func NewEngineMetricsRegistry() *EngineMetricsRegistry
func (r *EngineMetricsRegistry) Handler() http.Handler  // promhttp
func (r *EngineMetricsRegistry) UpdateAgentMetrics(agentName string, m SystemMetrics)
func (r *EngineMetricsRegistry) UpdateClusterState(agents, deployments, tasks ...)
func (r *EngineMetricsRegistry) IncrementEvent(eventType string)
```

**CPU collection approach:**

CPU usage requires two `/proc/stat` samples to compute delta. The `SystemCollector` stores the previous sample and computes CPU usage as the delta since last call. Since heartbeats happen every 15 seconds, this naturally provides a 15-second CPU average. The first call after creation returns 0 (no previous sample).

**No background goroutine needed** — the heartbeat interval (15s) is the natural sampling interval.

---

## Proto Changes

### New message: SystemMetrics

```proto
message SystemMetrics {
  double cpu_usage_ratio = 1;       // 0.0-1.0
  uint64 memory_used_bytes = 2;
  uint64 memory_total_bytes = 3;
  uint64 disk_used_bytes = 4;
  uint64 disk_total_bytes = 5;
  uint32 cpu_cores = 6;
}
```

### Modified: HeartbeatRequest (add field 4)

```proto
message HeartbeatRequest {
  string agent_name = 1;
  string session_token = 2;
  repeated string tags = 3;
  SystemMetrics system_metrics = 4;  // NEW
}
```

### New RPC: GetDashboardData

```proto
service EngineService {
  // ... existing RPCs ...
  rpc GetDashboardData(GetDashboardDataRequest) returns (GetDashboardDataResponse);
}

message GetDashboardDataRequest {}

message GetDashboardDataResponse {
  EngineStatus engine = 1;
  repeated AgentDetail agents = 2;
  repeated DeploymentInfo deployments = 3;  // reuse existing message
  ClusterSummary summary = 4;
  repeated ClusterEvent recent_events = 5;
}

message EngineStatus {
  string status = 1;
  int64 started_at_unix = 2;
  SystemMetrics system_metrics = 3;
  string version = 4;
}

message AgentDetail {
  string name = 1;
  string status = 2;
  string api_address = 3;
  int64 last_seen_unix = 4;
  int64 created_at_unix = 5;
  repeated string tags = 6;
  SystemMetrics system_metrics = 7;
  string vpc_subnet = 8;
  int32 container_count = 9;
}

message ClusterSummary {
  int32 total_agents = 1;
  int32 connected_agents = 2;
  int32 total_deployments = 3;
  int32 running_deployments = 4;
  int32 total_containers = 5;
  int32 healthy_containers = 6;
  map<string, int32> tasks_by_status = 7;
}

message ClusterEvent {
  int64 timestamp_unix = 1;
  string type = 2;       // "agent.connected", "deployment.started", etc.
  string message = 3;
  string severity = 4;   // "info", "warning", "error"
}
```

---

## Package Structure

### New packages

```
pkg/metrics/                    # Prometheus metrics + system collection
├── go.mod                      # prometheus/client_golang dependency
├── collector.go                # SystemCollector (CPU/RAM/disk from /proc)
├── collector_test.go
├── registry.go                 # EngineMetricsRegistry (prom registry + update methods)
├── registry_test.go
└── types.go                    # SystemMetrics transport struct

pkg/dashboard/                  # TUI dashboard (bubbletea)
├── go.mod                      # bubbletea, bubbles, lipgloss
├── model.go                    # Main DashboardModel — top-level tea.Model
├── model_test.go
├── overview.go                 # Overview view (engine + cluster + agents + deploys + events)
├── overview_test.go
├── agents.go                   # Agent detail view
├── agents_test.go
├── deployments.go              # Deployment detail view
├── deployments_test.go
├── containers.go               # All containers view
├── containers_test.go
├── styles.go                   # Shared lipgloss styles, progress bar rendering
├── keys.go                     # Key bindings (tea.KeyMap)
├── fetch.go                    # Data fetcher (gRPC client wrapper + polling)
├── fetch_test.go
└── types.go                    # View-layer data types (decoupled from proto)
```

### Modified packages

```
pkg/rpc/proto/.../engine.proto  # New messages + GetDashboardData RPC
pkg/agent/agent.go              # Create SystemCollector, send metrics in heartbeat
pkg/engine/engine.go            # Store startedAt, create EngineMetricsRegistry
pkg/engine/grpc_server.go       # Implement GetDashboardData, store agent metrics, emit events
pkg/engine/events.go            # New file — EventBuffer ring buffer
cmd/banyan-cli/cmd/dashboard.go # New 'dashboard' command
cmd/banyan-engine/cmd/engine.go # Start /metrics HTTP endpoint
```

---

## Event System

Simple in-memory ring buffer on the engine. No persistence — monitoring events don't need to survive restarts.

```go
// pkg/engine/events.go
type Event struct {
    Timestamp time.Time
    Type      string   // "agent.connected", "deployment.started", etc.
    Message   string
    Severity  string   // "info", "warning", "error"
}

type EventBuffer struct {
    events []Event
    mu     sync.RWMutex
    size   int  // max entries, default 100
}

func NewEventBuffer(size int) *EventBuffer
func (b *EventBuffer) Add(e Event)
func (b *EventBuffer) Recent(n int) []Event  // most recent n events
```

**Events emitted at:**

| Location | Event Type | Severity | Example Message |
|---|---|---|---|
| `Register()` success | `agent.connected` | info | "worker-1 connected (tags: gpu, east)" |
| Heartbeat timeout | `agent.disconnected` | warning | "worker-2 disconnected (last seen 45s ago)" |
| `Deploy()` called | `deployment.started` | info | "Deployment web-app started (blue-green)" |
| Deployment reaches running | `deployment.running` | info | "Deployment web-app is running (3/3 healthy)" |
| Deployment fails | `deployment.failed` | error | "Deployment web-app failed: image pull error" |
| Task completed | `task.completed` | info | "web-app-web-0 started on worker-1" |
| Task failed | `task.failed` | error | "web-app-web-2 failed on worker-3: exit code 1" |
| `Down()` called | `deployment.stopped` | info | "Deployment web-app stopped" |

Each event also increments its corresponding `banyan_events_total{type="..."}` Prometheus counter.

---

## Implementation Phases

### Phase 1: Metrics Foundation

**Goal**: Agents collect and report system metrics. Engine stores them. Prometheus `/metrics` endpoint.

| Step | File | Change |
|---|---|---|
| 1.1 | `pkg/metrics/` | New module: `SystemCollector`, `SystemMetrics`, `EngineMetricsRegistry` |
| 1.2 | `pkg/metrics/collector.go` | CPU from `/proc/stat`, memory from `/proc/meminfo`, disk from `syscall.Statfs` |
| 1.3 | `pkg/metrics/registry.go` | Define all prometheus metrics, update methods, `promhttp.Handler()` |
| 1.4 | `pkg/rpc/proto/.../engine.proto` | Add `SystemMetrics` message, add field to `HeartbeatRequest` |
| 1.5 | `pkg/agent/agent.go` | Create `SystemCollector` in `Run()`, include metrics in heartbeat |
| 1.6 | `pkg/engine/grpc_server.go` | On heartbeat: extract metrics, update `EngineMetricsRegistry` |
| 1.7 | `pkg/engine/engine.go` | Create `EngineMetricsRegistry`, self-collect engine system metrics |
| 1.8 | `cmd/banyan-engine/cmd/engine.go` | Start HTTP server for `/metrics` on API port |
| 1.9 | Tests | Unit tests for collector (with /proc fixtures) and registry |

**Deliverable**: `curl <engine>:<api_port>/metrics` returns full Prometheus metrics.

### Phase 2: Dashboard Data RPC + Events

**Goal**: New gRPC endpoint returns everything the dashboard needs. Event system.

| Step | File | Change |
|---|---|---|
| 2.1 | `pkg/rpc/proto/.../engine.proto` | Add `GetDashboardData` RPC + response messages |
| 2.2 | `pkg/engine/events.go` | New file: `EventBuffer` ring buffer |
| 2.3 | `pkg/engine/engine.go` | Store `startedAt`, create `EventBuffer`, pass to gRPC server |
| 2.4 | `pkg/engine/grpc_server.go` | Implement `GetDashboardData()`, emit events at lifecycle points |
| 2.5 | `pkg/types/records.go` | Add `SystemMetrics` field to `NodeRecord` |
| 2.6 | CLI client | Add `DashboardData()` method |
| 2.7 | Tests | gRPC handler tests, event buffer tests |

**Deliverable**: `GetDashboardData` returns engine status, agent details with metrics, deployments, cluster summary, recent events.

### Phase 3: Dashboard TUI — Overview

**Goal**: Working `banyan-cli dashboard` with overview screen.

| Step | File | Change |
|---|---|---|
| 3.1 | `pkg/dashboard/` | New module with bubbletea, bubbles, lipgloss deps |
| 3.2 | `pkg/dashboard/styles.go` | Color palette, progress bars, status indicators |
| 3.3 | `pkg/dashboard/keys.go` | Key bindings |
| 3.4 | `pkg/dashboard/types.go` | View-layer types (decoupled from proto) |
| 3.5 | `pkg/dashboard/fetch.go` | Data fetcher with polling ticker |
| 3.6 | `pkg/dashboard/overview.go` | Engine card, cluster card, agents table, deploys table, events |
| 3.7 | `pkg/dashboard/model.go` | Main model, view switching, tick-based refresh |
| 3.8 | `cmd/banyan-cli/cmd/dashboard.go` | `banyan-cli dashboard` command with `--refresh` flag |
| 3.9 | Tests | View rendering tests (golden file or snapshot) |

**Deliverable**: `banyan-cli dashboard` shows live overview with auto-refresh.

### Phase 4: Detail Views

**Goal**: Agent, deployment, and container detail screens with navigation.

| Step | File | Change |
|---|---|---|
| 4.1 | `pkg/dashboard/agents.go` | Agent detail: info card, resource bars, container list |
| 4.2 | `pkg/dashboard/deployments.go` | Deployment detail: info card, services with replicas |
| 4.3 | `pkg/dashboard/containers.go` | All containers: flat table, sorting |
| 4.4 | `pkg/dashboard/model.go` | View switching, back navigation, drill-down from tables |
| 4.5 | Tests | Navigation tests, rendering tests |

**Deliverable**: Full 4-screen dashboard with keyboard navigation.

### Phase 5: Polish

**Goal**: Production-quality TUI experience.

| Step | File | Change |
|---|---|---|
| 5.1 | Responsive layout | Adapt to terminal width (compact mode < 100 cols) |
| 5.2 | Help overlay | `?` shows key binding cheat sheet |
| 5.3 | Error recovery | Engine unreachable → show status, auto-retry |
| 5.4 | Loading states | Spinner on initial fetch |
| 5.5 | Filter/search | `/` to filter agents, deployments, containers |

### Future (post-M4)

| Feature | Description |
|---|---|
| Container CPU/RAM | Per-container metrics from nerdctl stats or cgroup |
| Agent `/metrics` endpoint | Direct scraping of individual agents |
| Request throughput | L7 proxy integration or Prometheus service metrics |
| Log panel | Integrated log streaming in dashboard |
| Historical sparklines | Requires time-series storage |
| Web dashboard | Separate frontend consuming same `/metrics` + gRPC |
| Grafana dashboards | Pre-built .json dashboards for Grafana |
| AlertManager rules | Pre-built alert rules for common scenarios |

---

## Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Metrics library | `prometheus/client_golang` | Native Prometheus format. No translation layer. Industry standard. |
| Metrics endpoint location | Engine `/metrics` HTTP | Single scrape target for whole cluster. Agents report via heartbeat. Simpler Prometheus config. |
| Polling vs streaming (dashboard) | Polling (3s default) | Data changes every 3-15s anyway. Simpler. Streaming can be added later. |
| Separate RPC vs enhance GetStatus | New `GetDashboardData` RPC | `GetStatus` stays lightweight for scripts. Dashboard needs metrics, events, summaries. |
| System metrics collection | Pure Go (`/proc` + `syscall`) | No external deps for collection. Only `prometheus/client_golang` for exposition. Linux is the target. |
| Container CPU/RAM | Deferred to post-M4 | Requires nerdctl stats or cgroup. Significant complexity. System metrics deliver 80% of value first. |
| Event storage | In-memory ring buffer | Monitoring events don't need persistence. Simple. Zero deps. |
| Dashboard package | `pkg/dashboard/` | Testable, decoupled from CLI. CLI command is just the entry point. |
| CLI command | `banyan-cli dashboard` (alias: `dash`) | Clear, discoverable. `status` stays as one-shot for scripts. |

---

## Data Requirements by Screen

Summary of what each screen needs and where it comes from:

| UI Element | Source | Available Today | New |
|---|---|---|---|
| Engine status | `Health` RPC | Yes | - |
| Engine uptime | `engine.startedAt` | No | Phase 2 |
| Engine CPU/RAM/Disk | `SystemCollector` on engine | No | Phase 1 |
| Connected agents count | Node records | Yes (calculate) | Aggregated in Phase 2 |
| Agent list + status + tags | `GetStatus` | Yes | - |
| Agent CPU/RAM/Disk | Heartbeat `SystemMetrics` | No | Phase 1 |
| Agent VPC subnet | `PeerTracker` | Available, not in status | Phase 2 |
| Agent container count | Task records | Yes (calculate) | Aggregated in Phase 2 |
| Deployment list + status | `GetStatus` | Yes | - |
| Deployment services + replicas | `GetStatus` | Yes | - |
| Task/container details | `GetStatus` | Yes | - |
| Container IP/ports | Task records | Yes | - |
| Recent events | `EventBuffer` | No | Phase 2 |
| Cluster summary | Aggregation | No (client-side) | Phase 2 (server-side) |
| All Prometheus metrics | `EngineMetricsRegistry` | No | Phase 1 |
