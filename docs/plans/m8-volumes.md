# Milestone 8: Volume Support — Implementation Plan

## Overview

Add Docker Compose-compatible volume support to Banyan. Users can declare volumes in `banyan.yaml` using the same syntax as Docker Compose — named volumes, bind mounts, tmpfs, and NFS. This is the most-requested missing feature for real-world deployments (databases, config files, logs).

## Current State

- **Zero volume code** exists in the codebase. The word "volume" doesn't appear in any Go source file.
- The manifest parser (`pkg/types/manifest.go`) supports Docker Compose fields: `image`, `build`, `ports`, `environment`, `deploy`, `healthcheck`, `depends_on`, `restart`, `entrypoint`, `command`, `env_file`.
- `buildNerdctlRunArgs()` in `pkg/agent/agent.go` generates nerdctl CLI flags from TaskRecord fields. No `-v` or `--mount` flags are generated.
- nerdctl supports `-v` (named volumes, bind mounts, read-only) and `--mount` (tmpfs, structured options). **nerdctl does NOT support NFS volume drivers** — Banyan must mount NFS on the host.

## Desired End State

Users write the same volume syntax as Docker Compose:

```yaml
name: my-app

services:
  db:
    image: postgres:15
    volumes:
      - db-data:/var/lib/postgresql/data         # named volume
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql:ro  # bind mount
    deploy:
      placement:
        node: db-*

  api:
    image: myapp/api
    volumes:
      - uploads:/app/uploads                     # shared NFS volume
      - type: tmpfs
        target: /tmp
        tmpfs:
          size: 512m

volumes:
  db-data:                                        # local named volume
  uploads:
    driver: local
    driver_opts:
      type: nfs
      o: "addr=nfs.internal,vers=4,soft"
      device: ":/exports/uploads"
```

### Verification
- `go test ./pkg/types/...` — volume parsing tests (short syntax, long syntax, top-level volumes)
- `go test ./pkg/agent/...` — buildNerdctlRunArgs generates correct `-v`/`--mount` flags
- `go test ./pkg/engine/...` — proto conversion round-trips volumes correctly
- E2E test: deploy a service with a named volume, verify data persists across `down` + `up`
- E2E test: deploy with a bind mount, verify file is accessible in container

## What We're NOT Doing

- **Volume lifecycle management** (`banyan-cli volume create/rm/ls`) — not needed for v1, nerdctl manages volumes
- **Volume backup/restore** — out of scope
- **Distributed/replicated volumes** (Longhorn, JuiceFS) — future milestone
- **`volumes_from`** — deprecated Docker Compose feature, nerdctl doesn't support it
- **Volume labels** — low priority, can add later
- **External volumes** (`external: true`) — low priority, can add later
- **Bind propagation** (`bind.propagation`) — niche, can add later

## Implementation Approach

The volume data flows through the same pipeline as every other manifest field:

```
banyan.yaml → ManifestService → proto ManifestService → ServiceRecord → TaskRecord → nerdctl args
```

Each phase adds volumes to one layer of this pipeline.

---

## Phase 1: Types & Parsing

### Overview
Add volume types to the manifest parser and internal records. Support both Docker Compose short syntax (`source:target:mode`) and long syntax (`type`, `source`, `target`, `read_only`). Add top-level `volumes` map.

### Changes Required

#### 1. Volume types
**File**: `pkg/types/manifest.go`

Add new types:

```go
// VolumeMount represents a service-level volume mount.
// Supports both Docker Compose short syntax (string) and long syntax (struct).
type VolumeMount struct {
    Type     string    `yaml:"type,omitempty"`      // "volume", "bind", "tmpfs"
    Source   string    `yaml:"source,omitempty"`     // volume name or host path
    Target   string    `yaml:"target"`               // container path (required)
    ReadOnly bool      `yaml:"read_only,omitempty"`
    Tmpfs    *TmpfsOpt `yaml:"tmpfs,omitempty"`
}

type TmpfsOpt struct {
    Size string `yaml:"size,omitempty"` // e.g., "512m", "1g"
}

// VolumeConfig represents a top-level named volume declaration.
type VolumeConfig struct {
    Driver     string            `yaml:"driver,omitempty"`
    DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
    External   bool              `yaml:"external,omitempty"`
    Name       string            `yaml:"name,omitempty"`
}

// VolumeMounts supports both string and struct forms (like DependsOnConfig).
type VolumeMounts []VolumeMount

func (v *VolumeMounts) UnmarshalYAML(value *yaml.Node) error {
    // Parse each item: string → short syntax, mapping → long syntax
}
```

Short syntax parsing (`source:target:mode`):
- `/host/path:/container/path` → bind mount (absolute path starts with `/`)
- `./relative:/container/path` → bind mount (starts with `./` or `../`)
- `name:/container/path` → named volume
- `name:/container/path:ro` → named volume, read-only

#### 2. Add to manifest structs
**File**: `pkg/types/manifest.go`

```go
type BanyanManifest struct {
    Services map[string]ManifestService `yaml:"services"`
    Volumes  map[string]VolumeConfig    `yaml:"volumes,omitempty"` // NEW
    // ...existing fields...
}

type ManifestService struct {
    Volumes VolumeMounts `yaml:"volumes,omitempty"` // NEW
    // ...existing fields...
}
```

#### 3. Add to records
**File**: `pkg/types/records.go`

```go
type ServiceRecord struct {
    Volumes []VolumeMount `json:"volumes,omitempty"` // NEW
    // ...existing fields...
}

type TaskRecord struct {
    Volumes []VolumeMount `json:"volumes,omitempty"` // NEW
    // ...existing fields...
}
```

#### 4. Pass through helpers
**File**: `pkg/types/helpers.go`

In `BuildServiceRecords()`: copy `svc.Volumes` → `ServiceRecord.Volumes`
In `BuildTasksForDeployment()`: copy `svc.Volumes` → `TaskRecord.Volumes`

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/types/...` — short syntax parsing: `name:/path`, `/host:/path`, `./rel:/path:ro`
- [ ] `go test ./pkg/types/...` — long syntax parsing: `type: bind`, `type: volume`, `type: tmpfs`
- [ ] `go test ./pkg/types/...` — top-level volumes with driver/driver_opts
- [ ] `go test ./pkg/types/...` — round-trip: YAML → struct → JSON → struct
- [ ] `golangci-lint run ./pkg/types/...` — no lint errors

---

## Phase 2: Proto & Conversion

### Overview
Add volume fields to proto definitions and update manifestToProto/protoToManifest conversion functions.

### Changes Required

#### 1. Proto definitions
**File**: `pkg/rpc/proto/banyan/v1/engine.proto`

```proto
message VolumeMount {
    string type = 1;        // "volume", "bind", "tmpfs"
    string source = 2;      // volume name or host path
    string target = 3;      // container path
    bool read_only = 4;
    TmpfsOpt tmpfs = 5;
}

message TmpfsOpt {
    string size = 1;
}

message VolumeConfig {
    string driver = 1;
    map<string, string> driver_opts = 2;
    bool external = 3;
    string name = 4;
}

message ManifestService {
    // ...existing fields 1-10...
    repeated VolumeMount volumes = 11;    // NEW
}

message Manifest {
    // ...existing fields 1-3...
    map<string, VolumeConfig> volumes = 4; // NEW
}

message TaskRecord {
    // ...existing fields 1-19...
    repeated VolumeMount volumes = 20;    // NEW
}
```

Regenerate: `protoc --go_out=... --go-grpc_out=... engine.proto`

#### 2. CLI → Engine conversion
**File**: `cmd/banyan-cli/cmd/client.go`

In `manifestToProto()`: convert `types.VolumeMount` → `banyanpb.VolumeMount`

#### 3. Engine → Types conversion
**File**: `pkg/engine/grpc_server.go`

In `protoToManifest()`: convert `banyanpb.VolumeMount` → `types.VolumeMount`

#### 4. PollTasks handler
**File**: `pkg/engine/grpc_server.go`

In `PollTasks()`: include volumes when converting TaskRecord → proto.

#### 5. Agent task conversion
**File**: `pkg/agent/agent.go`

In `pbTaskToLocal()`: convert proto volumes → types.VolumeMount.

### Success Criteria

#### Automated Verification:
- [ ] Proto compiles cleanly
- [ ] `go test ./cmd/banyan-cli/...` — manifestToProto round-trip with volumes
- [ ] `go test ./pkg/engine/...` — protoToManifest round-trip with volumes
- [ ] `golangci-lint run ./...` — no lint errors

---

## Phase 3: Container Creation

### Overview
Generate nerdctl `-v` and `--mount` flags from volume definitions in buildNerdctlRunArgs. This is where volumes actually take effect.

### Changes Required

#### 1. Resolve top-level volume configs
**File**: `pkg/types/manifest.go` or new `pkg/types/volumes.go`

```go
// ResolveVolumes merges service-level volume mounts with top-level volume
// declarations. Named volumes inherit driver/driver_opts from the top-level
// volumes map. Relative bind mount paths are resolved against basePath.
func ResolveVolumes(mounts []VolumeMount, topLevel map[string]VolumeConfig, basePath string) []VolumeMount
```

- Named volume references look up top-level config (driver, driver_opts)
- Relative paths (`./config`) are resolved to absolute paths against the manifest directory
- Top-level volumes with `driver_opts.type: nfs` are tagged for agent-side NFS mounting

#### 2. Generate nerdctl flags
**File**: `pkg/agent/agent.go`

In `buildNerdctlRunArgs()`, after existing flags:

```go
// Volumes
for _, vol := range task.Volumes {
    switch vol.Type {
    case "bind", "":
        // -v /host/path:/container/path[:ro]
        flag := vol.Source + ":" + vol.Target
        if vol.ReadOnly {
            flag += ":ro"
        }
        args = append(args, "-v", flag)

    case "volume":
        // -v name:/container/path[:ro]
        flag := vol.Source + ":" + vol.Target
        if vol.ReadOnly {
            flag += ":ro"
        }
        args = append(args, "-v", flag)

    case "tmpfs":
        // --mount type=tmpfs,target=/path[,tmpfs-size=512m]
        mount := "type=tmpfs,target=" + vol.Target
        if vol.Tmpfs != nil && vol.Tmpfs.Size != "" {
            mount += ",tmpfs-size=" + vol.Tmpfs.Size
        }
        args = append(args, "--mount", mount)
    }
}
```

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/agent/...` — buildNerdctlRunArgs with named volume
- [ ] `go test ./pkg/agent/...` — buildNerdctlRunArgs with bind mount (absolute, relative)
- [ ] `go test ./pkg/agent/...` — buildNerdctlRunArgs with read-only mount
- [ ] `go test ./pkg/agent/...` — buildNerdctlRunArgs with tmpfs
- [ ] `go test ./pkg/agent/...` — buildNerdctlRunArgs with multiple volumes

#### Manual Verification:
- [ ] Deploy postgres with named volume, insert data, `banyan-cli down`, re-deploy, data persists
- [ ] Deploy nginx with bind-mounted config file, verify config is readable
- [ ] Deploy with `:ro` mount, verify container can't write

---

## Phase 4: NFS Agent Mount

### Overview
Since nerdctl doesn't support NFS volume drivers, the agent must mount NFS shares on the host before starting containers. This phase adds that capability.

### Changes Required

#### 1. NFS mount management on agent
**File**: `pkg/agent/nfs_mounts.go` (new file)

```go
// NFSMountManager handles mounting/unmounting NFS shares on the agent host.
type NFSMountManager struct {
    mountBase string // e.g., /var/lib/banyan/nfs-mounts
}

// EnsureMount mounts an NFS share if not already mounted.
// Returns the local mount path.
func (m *NFSMountManager) EnsureMount(ctx context.Context, vol types.VolumeMount) (string, error) {
    // 1. Parse driver_opts: type=nfs, o=addr=...,vers=4, device=:/path
    // 2. Compute mount path: /var/lib/banyan/nfs-mounts/{hash}
    // 3. Check if already mounted (mountpoint -q)
    // 4. If not: mkdir -p && mount -t nfs -o {opts} {addr}:{device} {mountPath}
    // 5. Return mountPath
}
```

#### 2. Pre-process volumes before container start
**File**: `pkg/agent/agent.go`

In `executeCreateAndStart()`, before `buildNerdctlRunArgs()`:

```go
// Resolve NFS volumes to local mount paths
resolvedVolumes := make([]types.VolumeMount, len(task.Volumes))
copy(resolvedVolumes, task.Volumes)
for i, vol := range resolvedVolumes {
    if isNFSVolume(vol) {
        localPath, err := nfsMounter.EnsureMount(ctx, vol)
        if err != nil {
            return nil, fmt.Errorf("NFS mount failed for %s: %w", vol.Target, err)
        }
        // Convert to bind mount for nerdctl
        resolvedVolumes[i] = types.VolumeMount{
            Type:     "bind",
            Source:   localPath,
            Target:   vol.Target,
            ReadOnly: vol.ReadOnly,
        }
    }
}
```

#### 3. Install NFS client in install scripts
**Files**: `install-deps.sh`

```bash
install_nfs_client() {
    if command -v mount.nfs &>/dev/null; then
        info "NFS client already installed."
        return
    fi
    case "$OS" in
        ubuntu|debian|...) $PKG_INSTALL nfs-common ;;
        *)                 $PKG_INSTALL nfs-utils ;;
    esac
}
```

Add `install_nfs_client` to agent install path.

### Success Criteria

#### Automated Verification:
- [ ] `go test ./pkg/agent/...` — NFS mount resolution (mock mount command)
- [ ] `go test ./pkg/agent/...` — NFS volume converted to bind mount for nerdctl

#### Manual Verification:
- [ ] Deploy with NFS volume, verify container can read/write to NFS share
- [ ] Multiple containers sharing the same NFS volume work correctly

---

## Phase 5: Tests & E2E

### Unit Tests
- Volume YAML parsing (short syntax, long syntax, edge cases)
- Volume resolution (named → top-level lookup, relative path resolution)
- buildNerdctlRunArgs for all volume types
- Proto round-trip (manifestToProto ↔ protoToManifest with volumes)
- NFS mount management (mock commands)

### E2E Tests
Add to `test/e2e/examples/`:
- `banyan-volumes.yaml` — manifest with named volume + bind mount
- Test: deploy, write data, down, re-deploy, verify data persists (named volume)
- Test: deploy with bind mount, verify file accessible in container

### Success Criteria

#### Automated Verification:
- [ ] All unit tests pass with > 80% coverage on new code
- [ ] E2E tests pass: volume persistence across down + up
- [ ] E2E tests pass: bind mount file accessibility
- [ ] `golangci-lint run ./...` — no lint errors

---

## Phase 6: Documentation

### Manifest Reference
**File**: `website/src/content/docs/reference/manifest.md`

Add `volumes` section with examples for named volumes, bind mounts, tmpfs, NFS.

### Roadmap
Update M8 status to Done.

### Limitations
Document clearly:
- Named volumes are local to each agent (not shared)
- NFS requires NFS server (user-provided)
- `external: true` not yet supported
- Volume labels not yet supported

---

## Performance Considerations

- **No overhead for non-volume deployments** — if `Volumes` is empty, nothing happens
- **NFS mount latency** — `mount -t nfs` can take 1-5s, happens once per unique NFS share per agent
- **Volume parsing** — negligible (YAML string splitting)

## Migration Notes

- **No data migration** — volumes are a new additive feature
- **Config backward-compatible** — `volumes` field has `omitempty`, existing manifests work unchanged
- **Proto backward-compatible** — new fields are additive (proto3 default values)
- **No breaking changes** — strictly additive to every struct and proto message

## References

- Docker Compose volume spec: https://docs.docker.com/reference/compose-file/volumes/
- nerdctl volume support: https://github.com/containerd/nerdctl/blob/main/docs/command-reference.md
- nerdctl NFS limitation: https://github.com/containerd/nerdctl/issues/694
