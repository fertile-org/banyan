# Docker Compose Compatibility: Healthcheck

## Overview

Add `healthcheck` support to Banyan. This is a two-phase feature:

- **Phase 1**: Pass healthcheck config to nerdctl so containers run health checks locally. Report health status through existing agent → engine pipeline. Update deployment status to reflect container health.
- **Phase 2**: Make `depends_on` health-aware so services can wait for dependencies to be healthy before starting.

**Goal:** A compose file with `healthcheck:` and `depends_on: condition: service_healthy` works on Banyan.

---

## Compose Syntax

```yaml
services:
  db:
    image: postgres:15
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "postgres"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 30s

  api:
    image: my-api:latest
    depends_on:
      db:
        condition: service_healthy
```

---

## Phase 1: Healthcheck Passthrough + Status Reporting

### 1. Manifest (`pkg/types/manifest.go`)

Add `Healthcheck` to `ManifestService`:

```go
Healthcheck *ManifestHealthcheck `yaml:"healthcheck,omitempty"`
```

New type:

```go
ManifestHealthcheck struct {
    Test        []string `yaml:"test,omitempty"`
    Interval    string   `yaml:"interval,omitempty"`    // e.g., "10s"
    Timeout     string   `yaml:"timeout,omitempty"`     // e.g., "5s"
    Retries     int      `yaml:"retries,omitempty"`
    StartPeriod string   `yaml:"start_period,omitempty"` // e.g., "30s"
    Disable     bool     `yaml:"disable,omitempty"`
}
```

**YAML unmarshaling note:** Docker Compose supports two `test` formats:
- List form: `test: ["CMD", "curl", "-f", "http://localhost"]`
- String form: `test: curl -f http://localhost` (implicitly `CMD-SHELL`)

Need a custom unmarshaler or accept both via `interface{}` check, same pattern as `command`.

### 2. Proto (`pkg/rpc/banyanpb/banyan.proto`)

```proto
message ManifestHealthcheck {
  repeated string test = 1;
  string interval = 2;
  string timeout = 3;
  int32 retries = 4;
  string start_period = 5;
  bool disable = 6;
}

// In ManifestService:
ManifestHealthcheck healthcheck = <next>;

// In Task:
ManifestHealthcheck healthcheck = <next>;
```

### 3. Agent: Pass to nerdctl

In `buildNerdctlRunArgs()`:

```go
if task.Healthcheck != nil && !task.Healthcheck.Disable {
    hc := task.Healthcheck
    if len(hc.Test) > 0 {
        // CMD format: --health-cmd "pg_isready -U postgres"
        // CMD-SHELL format: --health-cmd "sh -c 'curl -f http://localhost'"
        switch hc.Test[0] {
        case "CMD":
            args = append(args, "--health-cmd", strings.Join(hc.Test[1:], " "))
        case "CMD-SHELL":
            args = append(args, "--health-cmd", hc.Test[1])
        case "NONE":
            args = append(args, "--no-healthcheck")
        default:
            // Treat as CMD-SHELL (string form from compose)
            args = append(args, "--health-cmd", strings.Join(hc.Test, " "))
        }
    }
    if hc.Interval != "" {
        args = append(args, "--health-interval", hc.Interval)
    }
    if hc.Timeout != "" {
        args = append(args, "--health-timeout", hc.Timeout)
    }
    if hc.Retries > 0 {
        args = append(args, "--health-retries", strconv.Itoa(hc.Retries))
    }
    if hc.StartPeriod != "" {
        args = append(args, "--health-start-period", hc.StartPeriod)
    }
}
```

### 4. Agent: Report health status

Currently the agent reports container status via `ContainerStatus` in health checks to the engine. Extend this to include healthcheck state.

**How nerdctl/containerd reports health:**

`nerdctl inspect <container>` returns a `State.Health.Status` field with values:
- `starting` — within start_period
- `healthy` — healthcheck passing
- `unhealthy` — healthcheck failing (exceeded retries)
- (absent) — no healthcheck configured

In the agent's health check loop (the function that inspects containers and reports status to the engine):

```go
// After getting container info from nerdctl inspect
if inspectResult.State.Health != nil {
    containerStatus.HealthStatus = inspectResult.State.Health.Status
}
```

**Proto change:** Add `health_status` field to `ContainerStatus`:

```proto
message ContainerStatus {
  // existing fields...
  string health_status = <next>;  // "", "starting", "healthy", "unhealthy"
}
```

### 5. Engine: Track and surface health status

**Store health status:** When the engine receives health reports from agents, store `HealthStatus` on `TaskRecord`:

```go
taskRecord.HealthStatus = report.HealthStatus
```

**Deployment health:** The engine currently considers a deployment "running" when all containers have status "running". With healthchecks:

- If ANY service in the deployment has a `healthcheck` configured:
  - Container is "healthy" only when `health_status == "healthy"` (not just "running")
  - Container is "starting" when `health_status == "starting"` (within start_period)
- If a service has NO healthcheck: existing behavior (running = healthy)

**Blue-green teardown:** Currently triggers when new deployment reaches `StatusRunning`. With healthchecks, delay teardown until all containers with healthchecks report `healthy`. This prevents tearing down old containers while new ones are still in `starting` state.

### 6. CLI: Display health status

Update status display commands (`deployment`, `container`) to show health status:

```
NAME                      STATUS       HEALTH      AGENT           IMAGE
my-app-db-0               running      healthy     worker-1        postgres:15
my-app-api-0              running      starting    worker-2        my-api:latest
```

### 7. Tests

- Manifest parsing (list form, string form, disable)
- `buildNerdctlRunArgs` with healthcheck flags
- Agent health report includes health_status
- Engine stores and surfaces health_status
- Blue-green teardown waits for healthy
- Proto round-trip

---

## Phase 2: `depends_on` with `condition: service_healthy`

### Current state

`depends_on` is currently a simple list:

```yaml
depends_on:
  - db
```

Compose also supports the long form with conditions:

```yaml
depends_on:
  db:
    condition: service_healthy
  redis:
    condition: service_started
```

### 1. Manifest changes

`DependsOn` currently is `[]string`. Need to support both forms:

```go
// DependsOn supports both short form (list of strings) and long form (map with conditions)
DependsOn DependsOnConfig `yaml:"depends_on,omitempty"`
```

New types:

```go
type DependsOnConfig map[string]DependsOnEntry

type DependsOnEntry struct {
    Condition string `yaml:"condition,omitempty"` // "service_started" (default) or "service_healthy"
}
```

Need a custom YAML unmarshaler for `DependsOnConfig` to handle both:
- `depends_on: ["db", "redis"]` → map with default `service_started` condition
- `depends_on: {db: {condition: service_healthy}}` → map with explicit condition

### 2. Engine: Health-aware task scheduling

Currently the engine dispatches all tasks at once (respecting `depends_on` ordering for per-service deploys). With `condition: service_healthy`:

**For full deploys:** The engine schedules services in dependency order. When a dependency has `condition: service_healthy`, the engine waits for that dependency's containers to report `health_status == "healthy"` before scheduling the dependent service.

Implementation in the orchestration loop:

```go
// Before scheduling a service's tasks:
for depName, dep := range service.DependsOn {
    if dep.Condition == "service_healthy" {
        // Wait until all containers of depName have health_status == "healthy"
        if !allContainersHealthy(deployment, depName) {
            // Skip this service for now, retry in next orchestration tick
            continue
        }
    }
    // For "service_started" (default): existing behavior — check containers are running
}
```

**Timeout:** Add a configurable timeout for health-aware waits (default: 5 minutes). If a dependency doesn't become healthy within the timeout, mark the deployment as failed.

**For per-service deploys:** Same validation as today, but additionally check that dependencies with `condition: service_healthy` are actually healthy (not just running).

### 3. Proto changes

Update `DependsOn` in the proto from `repeated string` to a map or repeated message:

```proto
message DependsOnEntry {
  string condition = 1;  // "service_started" or "service_healthy"
}

// In ManifestService:
map<string, DependsOnEntry> depends_on = <next>;
```

This is a breaking proto change for `depends_on`. Handle backward compatibility:
- Keep the old `repeated string depends_on` field (deprecated)
- Add new `map<string, DependsOnEntry> depends_on_v2` field
- Engine reads `depends_on_v2` if present, falls back to `depends_on`
- CLI always writes `depends_on_v2`

### 4. Tests

- Manifest parsing (short form backward compat, long form with conditions)
- Engine scheduling: waits for healthy before dispatching dependent tasks
- Engine scheduling: timeout when dependency never becomes healthy
- Per-service deploy validation with health conditions
- Proto backward compatibility (old clients with string depends_on)

---

## Implementation Order

### Phase 1 (do first, ~1-2 days)

1. Proto: add healthcheck message + health_status to ContainerStatus
2. Manifest: add Healthcheck struct
3. CLI: pass healthcheck through manifestToProto
4. Engine: pass healthcheck to task records, store health_status from reports
5. Agent: buildNerdctlRunArgs with healthcheck flags
6. Agent: report health_status from nerdctl inspect
7. Engine: blue-green teardown waits for healthy
8. CLI: display health_status in deployment/container views
9. Tests + docs

### Phase 2 (after Phase 1, ~1-2 days)

1. Manifest: DependsOnConfig with custom unmarshaler (backward compat)
2. Proto: depends_on_v2 map field
3. Engine: health-aware task scheduling with timeout
4. Engine: per-service deploy validation with conditions
5. Tests + docs

## Files Modified

| File | Phase | Change |
|------|-------|--------|
| `pkg/types/manifest.go` | 1+2 | Add `Healthcheck`, update `DependsOn` type |
| `pkg/types/manifest_test.go` | 1+2 | Parsing tests |
| `pkg/rpc/banyanpb/banyan.proto` | 1+2 | Healthcheck message, health_status, depends_on_v2 |
| `cmd/banyan-cli/cmd/client.go` | 1+2 | Update `manifestToProto()` |
| `cmd/banyan-cli/cmd/client_test.go` | 1+2 | Proto conversion tests |
| `pkg/engine/grpc_server.go` | 1+2 | Store health_status, health-aware scheduling |
| `pkg/engine/grpc_server_test.go` | 1+2 | Tests |
| `pkg/agent/agent.go` | 1 | Report health_status from inspect |
| `pkg/agent/agent_test.go` | 1 | Tests |
| `pkg/agent/` (nerdctl builder) | 1 | Healthcheck nerdctl flags |
| `cmd/banyan-cli/cmd/deployment_cmd.go` | 1 | Display health column |
| `cmd/banyan-cli/cmd/container_cmd.go` | 1 | Display health column |
| `website/src/content/docs/reference/manifest.md` | 1+2 | Document healthcheck + depends_on conditions |

## Risk: nerdctl inspect health status

Need to verify that `nerdctl inspect` actually returns health status for containerd-managed containers. Docker does this via the daemon's built-in health monitor. containerd may require a separate health check mechanism or a shim. If nerdctl doesn't support `--health-cmd` natively, we may need to implement health checking in the agent itself (poll with exec).

**Mitigation:** Test `nerdctl run --health-cmd "true" --health-interval 5s nginx:alpine && nerdctl inspect <id>` on a real system before starting implementation. If unsupported, fall back to agent-side health polling using `nerdctl exec`.
