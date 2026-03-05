# Docker Compose Compatibility: Quick Wins

## Overview

Add `restart`, `entrypoint`, and resource limits (`mem_limit`/`cpus`) to the Banyan manifest. These are the most commonly used Docker Compose fields that Banyan doesn't support yet, and all follow the same implementation pattern: parse from YAML, pass through proto, forward as nerdctl flags on the agent.

**Goal:** A user with a typical `docker-compose.yml` that uses `restart`, `entrypoint`, and resource limits can deploy it on Banyan without removing those fields.

---

## Feature 1: `restart`

Nearly every production compose file has `restart: unless-stopped`. Without this, a crashed container stays dead until the next redeployment.

### Compose syntax

```yaml
services:
  web:
    image: nginx:alpine
    restart: unless-stopped   # or: no, always, on-failure, on-failure:3
```

### Implementation

**1. Manifest (`pkg/types/manifest.go`)**

Add `Restart` field to `ManifestService`:

```go
Restart string `yaml:"restart,omitempty"`
```

Valid values: `no` (default), `always`, `unless-stopped`, `on-failure`, `on-failure:N`.

**2. Proto (`pkg/rpc/banyanpb/banyan.proto`)**

Add field to `ManifestService` message:

```proto
string restart = <next_field_number>;
```

**3. CLI → Engine (`cmd/banyan-cli/cmd/client.go`)**

In `manifestToProto()`, pass `svc.Restart` to the proto `ManifestService`.

**4. Engine → Agent**

The engine already passes the full manifest service to agents via task records. The `Restart` field flows through `TaskRecord` and into the proto `Task` message. Add `restart` field to the `Task` proto message.

**5. Agent (`pkg/agent/`)**

In `buildNerdctlRunArgs()`, add:

```go
if task.Restart != "" {
    args = append(args, "--restart", task.Restart)
}
```

nerdctl supports: `no`, `always`, `unless-stopped`, `on-failure[:max-retries]`.

**6. Validation**

In manifest validation (if exists), validate restart is one of the allowed values. Otherwise, nerdctl will reject invalid values at runtime.

**7. Tests**

- `pkg/types/`: manifest parsing test with `restart` field
- `cmd/banyan-cli/cmd/`: `manifestToProto` includes restart
- `pkg/agent/`: `buildNerdctlRunArgs` produces correct `--restart` flag
- Proto round-trip test

**8. Docs**

- Update `website/src/content/docs/reference/manifest.md`: add `restart` to the Service fields table and structure example

---

## Feature 2: `entrypoint`

Overrides the container's `ENTRYPOINT`. Common for customizing startup behavior without building a new image.

### Compose syntax

```yaml
services:
  db:
    image: postgres:15
    entrypoint: ["docker-entrypoint.sh", "--config", "/etc/postgres.conf"]
    # or string form:
    # entrypoint: /custom-entrypoint.sh
```

### Implementation

**1. Manifest (`pkg/types/manifest.go`)**

Add `Entrypoint` field to `ManifestService`. Use the same `Command` pattern (supports both string and list):

```go
Entrypoint []string `yaml:"entrypoint,omitempty"`
```

Note: Docker Compose supports both `entrypoint: "string"` and `entrypoint: ["list"]`. The YAML `[]string` type with a custom unmarshaler (or accepting both) handles this. Check if `Command` already handles the string-vs-list case — if so, use the same approach.

**2. Proto**

Add to `ManifestService` and `Task`:

```proto
repeated string entrypoint = <next_field_number>;
```

**3. CLI → Engine**

In `manifestToProto()`, pass `svc.Entrypoint`.

**4. Agent**

In `buildNerdctlRunArgs()`:

```go
if len(task.Entrypoint) > 0 {
    args = append(args, "--entrypoint", strings.Join(task.Entrypoint, " "))
}
```

Note: nerdctl `--entrypoint` takes a single string. If the entrypoint has args, they become the command. Check nerdctl behavior — may need to pass entrypoint as first arg and rest as command args.

Actually, nerdctl follows Docker behavior:
- `--entrypoint` sets only the entrypoint binary
- Additional args after the image name become CMD

So for `entrypoint: ["sh", "-c", "echo hello"]`:
```
nerdctl run --entrypoint sh <image> -c "echo hello"
```

Implementation:
```go
if len(task.Entrypoint) > 0 {
    args = append(args, "--entrypoint", task.Entrypoint[0])
    // remaining entrypoint args prepend to command
    extraArgs = task.Entrypoint[1:]
}
```

**5. Tests**

- Manifest parsing (string form, list form)
- `manifestToProto` includes entrypoint
- `buildNerdctlRunArgs` produces correct `--entrypoint` flag + extra args
- Proto round-trip test

**6. Docs**

- Update manifest reference: add `entrypoint` to Service fields table

---

## Feature 3: Resource Limits (`mem_limit` / `cpus`)

Prevents containers from consuming unbounded resources. Acts as a cgroup safety net even without resource-aware scheduling.

### Compose syntax

Compose supports two forms. We support the `deploy.resources` form (Compose v3+):

```yaml
services:
  api:
    image: my-api:latest
    deploy:
      resources:
        limits:
          memory: 512m
          cpus: "0.5"
        reservations:
          memory: 256m
          cpus: "0.25"
```

### Implementation

**1. Manifest (`pkg/types/manifest.go`)**

Add to `ManifestDeploy`:

```go
Resources *ManifestResources `yaml:"resources,omitempty"`
```

New types:

```go
ManifestResources struct {
    Limits       *ResourceSpec `yaml:"limits,omitempty"`
    Reservations *ResourceSpec `yaml:"reservations,omitempty"`
}

ResourceSpec struct {
    Memory string `yaml:"memory,omitempty"`  // e.g., "512m", "1g"
    CPUs   string `yaml:"cpus,omitempty"`    // e.g., "0.5", "2"
}
```

**2. Proto**

Add to `ManifestDeploy`:

```proto
message ManifestResources {
  ResourceSpec limits = 1;
  ResourceSpec reservations = 2;
}

message ResourceSpec {
  string memory = 1;
  string cpus = 2;
}

// In ManifestDeploy:
ManifestResources resources = <next_field_number>;
```

Add corresponding fields to the `Task` message so the agent receives them.

**3. CLI → Engine**

In `manifestToProto()`, pass resources through.

**4. Agent**

In `buildNerdctlRunArgs()`:

```go
if task.MemoryLimit != "" {
    args = append(args, "--memory", task.MemoryLimit)
}
if task.CPULimit != "" {
    args = append(args, "--cpus", task.CPULimit)
}
if task.MemoryReservation != "" {
    args = append(args, "--memory-reservation", task.MemoryReservation)
}
// Note: nerdctl doesn't have --cpus-reservation, CPU reservations
// map to --cpu-shares in Docker. Skip for Phase 1.
```

nerdctl flags: `--memory` (limit), `--cpus` (limit), `--memory-reservation` (soft limit).

**5. Phase 2 (future, not in this plan)**

Resource-aware scheduling: engine checks agent available resources before placing containers. Requires tracking resource usage per agent and bin-packing logic. Not needed now — cgroup limits alone prevent runaway containers.

**6. Tests**

- Manifest parsing with resources
- `manifestToProto` includes resources
- `buildNerdctlRunArgs` produces correct `--memory`, `--cpus` flags
- Proto round-trip test

**7. Docs**

- Update manifest reference: add `deploy.resources` to fields table

---

## Implementation Order

All three features touch the same files in the same way. Implement together in one pass:

1. **Proto changes** — add all new fields to `banyan.proto`, regenerate
2. **Manifest types** — add all new fields to `manifest.go`
3. **CLI client** — update `manifestToProto()` once for all fields
4. **Engine** — pass through to task records (all fields at once)
5. **Agent** — update `buildNerdctlRunArgs()` once for all flags
6. **Tests** — for each layer
7. **Docs** — update manifest reference once with all new fields

## Files Modified

| File | Change |
|------|--------|
| `pkg/types/manifest.go` | Add `Restart`, `Entrypoint`, `ManifestResources`, `ResourceSpec` |
| `pkg/types/manifest_test.go` | Parsing tests for new fields |
| `pkg/rpc/banyanpb/banyan.proto` | Add fields to `ManifestService`, `ManifestDeploy`, `Task` |
| `cmd/banyan-cli/cmd/client.go` | Update `manifestToProto()` |
| `cmd/banyan-cli/cmd/client_test.go` | Test proto conversion |
| `pkg/engine/grpc_server.go` | Pass new fields to task records |
| `pkg/engine/grpc_server_test.go` | Test task creation with new fields |
| `pkg/agent/agent.go` or nerdctl builder | Update `buildNerdctlRunArgs()` |
| `pkg/agent/agent_test.go` | Test nerdctl arg generation |
| `website/src/content/docs/reference/manifest.md` | Document new fields |
