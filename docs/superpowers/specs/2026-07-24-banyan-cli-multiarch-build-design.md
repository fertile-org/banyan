# banyan-cli multi-arch build — design

**Date:** 2026-07-24
**Status:** Approved design, pending spec review
**Author:** Pham Dang Huy (with Claude)

## Problem

`banyan-cli up` builds container images on the machine that runs the CLI and
pushes them to the engine registry ("Option B" build flow). The build path is
architecture-blind: `buildImage` (`cmd/banyan-cli/cmd/deploy.go:314`) runs
`nerdctl build -t <img> <ctx>` with no `--platform`, so images always inherit
the build host's architecture.

This breaks real deployments: the build box is an amd64 WSL2 machine, but the
production agent `banyan-agent-1` (Oracle Cloud Ampere) is **aarch64/arm64**.
amd64 images land on the arm64 agent and silently stay in `restarting`. The
cluster has no way to know an agent's architecture — none of `RegisterRequest`
(`engine.proto:81`), `HeartbeatRequest` (:110), `SystemMetrics` (:70), or
`AgentInfo` (:323) carries an arch field.

The current workaround for the takahiro-dev deploy is a hand-run
`deploy/takahiro-dev/build-arm64.sh` that cross-builds via QEMU and switches
the manifest from `build:` to pinned `image:` refs. It works but is manual,
easy to forget on redeploys, and lives outside the CLI.

## Goal

Make `banyan-cli` build images for the architecture(s) the target agents
actually run, so `build:` services work on a mixed-arch cluster without manual
scripts. Cross-arch builds use QEMU emulation on the machine that runs the CLI
(team decision — no remote builder).

## Non-goals

- **Remote build on the agent.** Considered and rejected in favor of local QEMU
  emulation. May be revisited later; the builder logic is kept behind a single
  build function so a remote implementation could slot in without touching the
  deploy flow.
- **MySQL 5.7 → 8.0 migration** for takahiro-dev. That is an operational task
  (`mysqldump` → `scp` → import), not part of this CLI feature.
- **Arch-aware scheduler changes.** Not needed — a single multi-arch OCI image
  index resolves the right layers at pull time on the agent (see Approach).
- **New multi-arch E2E test.** Requires a real mixed-arch cluster; out of scope.
  Coverage is unit + proto round-trip; the gap is stated explicitly below.

## Approach

Architecture becomes an agent-declared attribute that flows up to the engine;
the CLI reads it and builds the right platform(s). No layer has to guess.

```
agent (runtime.GOARCH)
   │  Register  (new `arch` field)
   ▼
engine  → NodeRecord.Arch (etcd)  → ListAgents / GetInfo expose arch
   ▲
   │  GetInfo / ListAgents  (CLI asks before building)
CLI `up`
   │  compute per-service platform set
   ▼
nerdctl build --platform=<set>  →  push --all-platforms  →  registry (1 tag = OCI index)
   ▼
agent pulls its own arch (containerd selects from the index)
```

### Per-service platform resolution (CLI)

For each service with a `build:` config, the CLI picks the target platform set
in this precedence order:

1. **Explicit `platform:` on the service** → use it verbatim (manual override,
   wins over everything). *(Phase 1)*
2. **`placement.node: X`** → the arch of node X.
3. **`deploy` with tags** → union of the arch of all agents matching the tags.
4. **No placement constraint** → union of **all** cluster agent arches (the
   image must run wherever the scheduler places it).
5. **Engine unreachable, or matched agents report no arch (old agents)** →
   **build host arch + a warning**. This preserves offline `--dry-run`.

### Why a single multi-arch index (not per-arch tags)

`nerdctl build --platform=linux/amd64,linux/arm64` + `nerdctl push
--all-platforms` produces one tag holding an OCI image index. containerd on the
agent automatically pulls the matching arch. Confirmed available on the
installed toolchain (nerdctl 2.1.3 exposes `build --platform strings` and `push
--all-platforms`; BuildKit 0.19). The engine, reconciler, and scheduler keep
deploying a single image reference unchanged — no scheduler work, and it stays
correct even for external `image:` refs that are already multi-arch.

### Deploy-flow reordering

Today `runDeploy` builds at `deploy.go:147`, *before* the engine client exists
(created ~`:180`). Phase 2 needs the agent arch map before building, so the flow
is reordered:

```
parse manifest → connect engine (if configured) → GetInfo/ListAgents
  → build arch map → resolve per-service platforms → build --platform
  → push --all-platforms → deploy
```

`--dry-run` and the engine-offline branch still build, using the host-arch
fallback (rule 5), so offline validation keeps working.

## Phasing

### Phase 1 — manual declaration + preflight (unblocks takahiro-dev now)

No proto/engine changes. Small diff.

| File | Change |
|---|---|
| `pkg/types/manifest.go` | Add `Platform string \`yaml:"platform,omitempty"\`` to `ManifestService`. Empty = current behavior. |
| `cmd/banyan-cli/cmd/deploy.go` | `buildImageArgs`/`buildImage` (108, 314) take a `platform` arg; when set, insert `--platform=<p>`. Add a global `--platform` flag on `up` to override all build services. |
| `cmd/banyan-cli/cmd/deploy.go` | `pushImage`: when a service was built for a non-host platform, push with `--all-platforms` so the index is pushed. |
| `cmd/banyan-cli/cmd/preflight.go` (new) | Before a cross-arch build, check `/proc/sys/fs/binfmt_misc/qemu-<arch>`. If missing, error with clear instructions (run `install-deps.sh`, or `nerdctl run --privileged --rm tonistiigi/binfmt --install <arch>`). |
| `install-deps.sh` | Add `install_qemu_binfmt()` (installs `qemu-user-static` + `binfmt-support`, or runs `tonistiigi/binfmt`). Invoke it for `ROLE=cli`/`all`. Replaces the "install buildx" idea — the stack is buildkit, not Docker, so buildx does not apply; QEMU binfmt is what is actually missing. |
| `docs/` | Document `platform:` in the manifest and the QEMU requirement. |

Result: takahiro-dev drops the manual `build-arm64.sh`, returns to `build:` in
`banyan.yaml` with `platform: linux/arm64` on the four app services, and
`banyan-cli up` cross-builds automatically.

### Phase 2 — auto-detect arch from the cluster

| File | Change |
|---|---|
| `pkg/rpc/proto/banyan/v1/engine.proto` | Add `string arch = 7;` to `RegisterRequest` (81) and `string arch = 5;` to `AgentInfo` (323). Not on Heartbeat — arch is static, only needed at Register. Regenerate `.pb.go`. |
| `pkg/agent/engine_client.go` | `RegisterRequest` struct (101) + call site (110): set `Arch: runtime.GOARCH`. |
| `pkg/types/records.go` | `NodeRecord` (134): add `Arch string \`json:"arch,omitempty"\``. |
| `pkg/engine/grpc_handlers_agent.go` | Register handler (~46): store `node.Arch = req.Arch`. |
| `pkg/engine/grpc_handlers_cli.go` | `ListAgents`/`GetInfo` (232, 309): return `Arch` in `AgentInfo`. |
| `cmd/banyan-cli/cmd/deploy.go` | Reorder `runDeploy` (connect → GetInfo/ListAgents → node→arch map → apply the resolution rules → build). Host-arch fallback + warning on offline / unknown arch. |

`platform:` from Phase 1 becomes the manual override in the precedence rules —
not discarded.

### Compatibility (degrade gracefully — decided)

Old agents send `arch=""`. The CLI treats such a node as unknown: if the
service has no explicit `platform:`, it builds host-arch and warns — "agent X
did not report arch; building for <host>; upgrade the agent or set `platform:`".
Nobody breaks on a version-skewed upgrade (this includes the live takahiro-dev
cluster).

## Testing

- **Unit:** `buildImageArgs` emits the correct `--platform`; platform-set
  resolution table (has placement / has tags / no constraint / offline / mixed
  arch); preflight detects missing binfmt.
- **Proto round-trip:** agent sets arch → engine stores → `ListAgents` returns
  it.
- **No new E2E** — a mixed-arch cluster is not available in CI. This is an
  explicit coverage gap; manual verification is the takahiro-dev deploy itself.

## Open items

None blocking. The MySQL 5.7→8.0 data migration for takahiro-dev is tracked
separately as an operational step, not part of this feature.
