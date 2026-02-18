# Milestone 2 — Service Observability: Design & Implementation Plan

## Problem

After deploying, there's no way to know if containers are actually healthy. The agent marks tasks "completed" the moment `nerdctl run` succeeds, but never checks again. A container that crashes immediately (like postgres without env vars) shows as "running" in the deployment status. There's also no way to tear down a deployment.

## Design Decisions

### 1. Container health stored in TaskRecord (not a new key)

The TaskRecord already tracks which container was created, on which agent. We add two fields:

```go
ContainerStatus    string    // "running", "exited", "paused", "not_found"
ContainerCheckedAt time.Time // last health check timestamp
```

**Why not a new etcd key pattern?** The task record already has the container name, agent ID, and image. Adding health to it avoids a second lookup path and keeps the etcd key structure flat. If we need to separate concerns later, we extract — but not now.

### 2. Agent polls container health (not push-based)

A new goroutine in the agent checks containers every 10 seconds via `nerdctl inspect`. This matches the existing polling pattern (task poll: 2s, heartbeat: 15s).

**Why not etcd watch?** The codebase uses polling everywhere. Consistency over optimization.

### 3. `engine status` always shows per-container detail

No `--brief` / `--verbose` flags. The summary line shows a healthy/total count. Below it, each container is listed with its node and status. The whole point of this milestone is visibility — hiding it behind a flag defeats the purpose.

### 4. `down` reuses the task mechanism

The `down` command creates `stop_and_remove` tasks targeting the same agents that ran the containers. The engine monitors stop tasks the same way it monitors start tasks. No new communication channels needed.

### 5. Logs are local only

`banyan-cli logs` runs `nerdctl logs` on the current machine. If the container isn't local, it tells you which node has it. Remote log streaming requires an agent API layer — that's future work, not milestone 2.

## Implementation Phases

### Phase 1: Container Health Monitoring

**types.go** — Add fields and constants:

```go
const (
    statusStopping = "stopping"
    statusStopped  = "stopped"
)

const (
    taskTypeStopAndRemove = "stop_and_remove"
)

// Add to TaskRecord:
ContainerStatus    string    `json:"container_status,omitempty"`
ContainerCheckedAt time.Time `json:"container_checked_at,omitempty"`
```

**agent.go** — New `containerHealthLoop` goroutine (10s interval):

```
For each completed create_and_start task on this node:
  Run: nerdctl inspect --format '{{.State.Status}}' <container-name>
  If success: update ContainerStatus (e.g. "running", "exited")
  If container not found: set ContainerStatus = "not_found"
  Update ContainerCheckedAt = now
  Save task back to etcd
```

Starts alongside the existing `agentLoop` and `agentHeartbeat` goroutines.

**helpers.go** — New function:

```go
func getContainerStatus(ctx context.Context, containerName string) string
```

### Phase 2: Enhanced Status Display

**engine.go** — Rewrite `runEngineStatus` deployment section:

```
Deployments: 1
  - my-app (status: running, containers: 5/6 healthy)
    web:
      my-app-web-0 on worker-1: running
      my-app-web-1 on worker-2: running
    api:
      my-app-api-0 on worker-1: running
      my-app-api-1 on worker-2: running
      my-app-api-2 on worker-1: running
    db:
      my-app-db-0 on worker-1: exited
```

Logic:
1. For each deployment, list tasks across all agents
2. Group tasks by service name
3. Count healthy (ContainerStatus == "running") vs total
4. Print summary line with healthy/total
5. Print each container with node and status

**helpers.go** — New function:

```go
func groupTasksByService(tasks []TaskRecord) map[string][]TaskRecord
```

### Phase 3: `banyan-cli down`

**New command**: `banyan-cli down --name <app-name> [services...]` or `banyan-cli down -f banyan.yaml [services...]`

Supports per-service teardown, like Docker Compose:

```bash
banyan-cli down --name my-app           # stops ALL services
banyan-cli down --name my-app web db    # stops only web and db
banyan-cli down -f banyan.yaml web      # stops only web (reads name from file)
```

**Flow** (in deploy.go or new down section):

1. Find deployment by name in etcd (scan deployments/ for matching Name)
2. Read all tasks for this deployment across all agents
3. If service names provided as positional args:
   - Validate each name exists in the deployment's Services map
   - Filter tasks to only those matching the requested ServiceName(s)
4. For each completed `create_and_start` task (filtered):
   - Create a `stop_and_remove` task on the same agent
   - Task ID: `<original-task-id>-stop`
   - Container name: same as original
5. Deployment status:
   - **All services stopped** → mark deployment status = "stopping" (Engine transitions to "stopped" when all stop tasks complete)
   - **Some services stopped** → deployment stays "running" (health checks from Phase 1 will update stopped containers to "not_found")
6. Wait for all stop tasks to complete (same polling as deploy)
7. Print result showing which services were stopped

**agent.go** — New handler in `executeTask`:

```go
case taskTypeStopAndRemove:
    return executeStopAndRemove(ctx, task)
```

`executeStopAndRemove`:
```
Run: nerdctl rm -f <container-name>
Return success (ignore "not found" errors — container may already be gone)
```

**engine.go** — Add to `processDeployments`:

```go
case statusStopping:
    checkStoppingDeployment(ctx, store, key, &deployment)
```

`checkStoppingDeployment` follows the same pattern as `checkDeployingDeployment`:
- Count stop_and_remove tasks for this deployment
- When all complete → mark deployment "stopped"

### Phase 4: Container Logs (with remote streaming)

**New command**: `banyan-cli logs <container-name>`

**Architecture** — Adapter pattern for log providers:

```go
// LogProvider retrieves container logs.
type LogProvider interface {
    StreamLogs(ctx context.Context, containerName string, opts LogOptions) (io.ReadCloser, error)
}

type LogOptions struct {
    Follow bool
    Tail   int
}
```

**Default implementation**: `NerdctlLogProvider`
- Runs `nerdctl logs` with appropriate flags
- Returns stdout as `io.ReadCloser`
- No storage — pure streaming

**Future adapters** (not milestone 2):
- WAL adapter: agent writes logs to files, serves from disk
- Loki adapter: agent pushes to Loki, queries via LogQL

**Agent HTTP server** — New goroutine started in `runAgentStart`:
- Listens on `--api-port` (default: `9090`)
- Single endpoint: `GET /v1/logs/{container}?follow=true&tail=100`
- Uses `LogProvider` to get log stream, writes as chunked HTTP response
- Minimal — just this one endpoint for now

**NodeRecord** — Add API address:

```go
APIAddress string `json:"api_address,omitempty"` // e.g. "192.168.1.10:9090"
```

Agent auto-detects hostname + port for the default. User can override with `--api-address`.

**CLI `logs` command flow**:
1. Query etcd tasks to find which agent has the container
2. If container is on this node → stream directly via `LogProvider`
3. If container is remote → HTTP GET to agent's `APIAddress`
4. Pipe output to stdout

**Flags:**
- `--follow` / `-f`: follow log output
- `--tail <n>`: number of lines from end
- `--etcd`: etcd endpoint (to look up which agent has the container)

### Phase 5: Tests

**helpers_test.go** — New tests:
- `TestGroupTasksByService` — grouping logic
- `TestGetContainerStatus` — mock nerdctl output parsing

**Existing test updates:**
- Update `TestDetermineDeploymentStatus` if logic changes
- Test the stop task creation logic

## File Changes Summary

| File | Changes |
|------|---------|
| `types.go` | Add ContainerStatus/ContainerCheckedAt to TaskRecord, new status/task type constants, LogProvider interface, LogOptions, APIAddress on NodeRecord |
| `agent.go` | Add containerHealthLoop goroutine, executeStopAndRemove handler, HTTP server for log streaming |
| `engine.go` | Enhanced status display, add statusStopping handler in processDeployments |
| `deploy.go` | Add `down` command with per-service support |
| `logs.go` | New `logs` command — local and remote streaming |
| `helpers.go` | Add getContainerStatus, groupTasksByService |
| `log_provider.go` | NerdctlLogProvider implementation |
| `helpers_test.go` | Tests for new helpers |

## What This Does NOT Include

- Log storage adapters (WAL, Loki — future milestones, the LogProvider interface is ready for them)
- Container restart on failure (needs restart policy — future milestone)
- Health check endpoints (needs custom health check config — future milestone)
- Task cleanup / garbage collection
- Agent API authentication (milestone 3 — basic security)

These are intentionally deferred. Each builds on the foundation this milestone creates.
