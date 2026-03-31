---
title: Roadmap
description: What's next for Banyan.
sidebar:
  order: 99
---

## Milestone 1 — Core Orchestration (MVP)

Status: **Done**

Run containers across multiple servers using a familiar YAML manifest.

- Parse banyan.yaml manifest (Docker Compose-compatible syntax)
- Engine control plane with etcd-based state
- Agent nodes with containerd/nerdctl container management
- Round-robin scheduling across agents
- CLI for engine, agent, and deploy workflows
- VPC networking layer (IPAM, DNS, CNI)
- E2E test infrastructure

---

## Milestone 2 — Service Observability

Status: **Done**

See what's running, check container health, stream logs, and stop deployments — all from the CLI.

- Agent monitors container health after deployment (running, exited, restarting)
- Agent reports per-container status back to Engine via gRPC
- `banyan-cli deployment` and `banyan-cli container` show per-service and per-container status
- `banyan-cli logs` streams container logs from agents (via engine gRPC proxy)
- Detect and surface failed containers (e.g., exited immediately after start)
- `banyan-cli down` command to stop and remove all containers for a deployment

---

## Milestone 3 — Security

Status: **Done**

Secure all inter-component communication with WireGuard-based authentication and encryption.

- All inter-component communication uses gRPC with public key authentication
- Each component generates an X25519 keypair during `init`
- Agent/CLI → Engine: public key in gRPC metadata, validated against whitelist
- Engine → Agent: session token authentication for log streaming
- Config file at `/etc/banyan/banyan.yaml` with sections: `engine`, `agent`, `cli`
- `init` commands for engine, agent, and CLI prompt for credentials and connection info
- Three separate binaries: `banyan-engine`, `banyan-agent`, `banyan-cli`

See [Authentication](/guides/authentication/) for details.

---

## Milestone 3.5 — Agent Tags for Environment Isolation

Status: **Done**

Optional tags on agents and deployments for environment isolation (e.g. staging vs production on shared infrastructure).

- Agent tags configured in `/etc/banyan/banyan.yaml` and sent via Register/Heartbeat RPCs
- `--tags` flag on `banyan-cli up` and `banyan-cli down` for deployment tag matching
- Tag matching rules: both untagged = match, one side tagged = no match, intersection = match
- Same app name with different tags can coexist as independent deployments
- Engine scheduling filters agents by tag match before assigning tasks

---

## Milestone 3.6 — Networking

Status: **Done**

Built-in overlay networking and cross-host load balancing without external dependencies.

- WireGuard overlay managed by Engine via abstract `OverlayDriver` interface
- Per-agent /24 subnet allocation from VPC CIDR via `SubnetAllocator`
- Peer discovery via heartbeat RPC (15s convergence)
- iptables DNAT proxy on each agent for port forwarding to container backends
- Cross-host load balancing: every agent aware of all service backends cluster-wide, probability-based DNAT rules distribute traffic across all replicas regardless of which agent they run on
- Service DNS: agent-local DNS server on bridge gateway IP resolves `<service>.<app-name>.internal` to container IPs (e.g., `db.my-app.internal`). Short names (e.g., `db`) also work when there's no conflict across deployments.

---

## Milestone 4 — Blue-Green Redeployment

Status: **Done**

Update running applications with zero downtime.

- **Blue-green strategy**: New containers start alongside old ones; old are torn down only after new deployment is healthy
- **Automatic rollback on failure**: If the new deployment fails, old containers keep running — no downtime
- **Per-service deployment**: Redeploy only specific services with `banyan-cli up -f banyan.yaml web api`
- **Dependency validation**: Per-service deploys validate `depends_on` — dependencies must be running or included
- **`deploy` → `up` rename**: The deploy command is now `banyan-cli up` (with `deploy` kept as an alias)
- **Per-service `down`**: Stop specific services with `banyan-cli down --name my-app web db`

See [Redeployment](/guides/redeployment/) for details.

---

## Milestone 4.6 — Live Terminal Dashboard

Status: **Done**

Monitor the entire cluster from your terminal — no browser, no Grafana, no setup.

- **`banyan-cli dashboard`**: Live terminal UI built with [Bubbletea](https://github.com/charmbracelet/bubbletea) showing real-time cluster state
- **Overview screen**: Engine health (CPU, memory, disk), cluster summary, agent table, deployment table, and recent events — all on one screen
- **Agent and deployment drill-down**: Select any agent or deployment to see detailed metrics, container status, service breakdown, and resource usage
- **Container list**: Flat view of every container across the cluster with status, image, agent, and replica info
- **Command palette**: Press `p` to fuzzy-search and jump between views
- **Keyboard navigation**: htop-style scrolling, vim keys (`j`/`k`), number keys to switch views, `Enter` to drill in, `Esc` to go back
- **Floating overlays**: Help and command palette float over the dashboard without hiding the underlying view
- Auto-refresh with configurable interval (`--refresh` flag, default 5s)

See [CLI Reference — dashboard](/reference/cli/#dashboard) for details.

---

## Milestone 5 — Production Readiness

Status: **Done**

Deploy with confidence: environment files for configuration, systemd services for reliability.

- **`env_file` support**: Reference `.env` files in the manifest (`env_file: .env` or `env_file: [.env, .env.local]`), matching Docker Compose syntax
- **Variable loading**: Parse key-value pairs from `.env` files and inject as container environment variables at deploy time
- **File distribution**: CLI bundles referenced `.env` files with the manifest so agents can resolve them on any node
- **Systemd service files**: Install script creates `banyan-engine.service` and `banyan-agent.service` for `systemctl enable --now` lifecycle management — auto-start on boot, restart on crash

---

## Milestone 6 — Resource-Aware Scheduling

Status: **Done**

Smarter task distribution based on node resources instead of simple round-robin.

- **Agent resource reporting**: Agents report CPU, memory, and disk usage to the engine via heartbeat (stored on NodeRecord in etcd)
- **Resource-aware scheduling**: Engine selects the agent with the most available memory when assigning tasks, tracking batch allocations to prevent piling tasks on one node
- **Resource requests in manifest**: Services can declare CPU and memory requirements via `deploy.resources` (e.g., `memory: 512m`, `cpus: "0.5"`)
- **Default resource requests**: Services without explicit requirements default to 512MB RAM and 1 CPU core for scheduling purposes
- **Cluster capacity validation**: Engine rejects deployments whose total resource requests exceed total cluster capacity
- **Graceful fallback**: When agents haven't reported metrics yet (e.g., first heartbeat pending), scheduling falls back to round-robin

---

## Milestone 7 — Multi-Engine High Availability

Status: **Done**

Run multiple engines for high availability. All engines are active — no leader, no standby. If one goes down, the others continue instantly.

- **Active-active scheduling**: All engines handle RPCs and run the scheduling loop. Per-deployment distributed locks in etcd prevent duplicate work.
- **Instant scheduling**: Deploy commands trigger scheduling immediately on the receiving engine, instead of waiting for a polling loop.
- **Managed registry**: Persistent OCI image storage via Distribution (Docker Registry v2) subprocess. Images survive engine restarts.
- **Agent multi-endpoint failover**: Agents configured with multiple engine addresses reconnect to the next available engine within seconds.
- **CLI multi-endpoint failover**: CLI tries each configured engine endpoint with a health check, connects to the first one that responds.
- **External etcd + registry required**: HA mode requires user-provided etcd cluster and OCI registry (managed services are single-process and can't be shared).
- **Zero-config single-engine preserved**: Default single-engine mode is unchanged — no new configuration needed for existing users.

See [High Availability](/guides/high-availability/) for setup guide.

---

## Milestone 8 — Volumes

Status: **Done**

Persistent storage for containers — named volumes, bind mounts, tmpfs, and NFS shared volumes. Same syntax as Docker Compose.

- **Named volumes**: Persistent local storage managed by the container engine. Data survives container restarts.
- **Bind mounts**: Mount host directories or files into containers. Absolute paths or relative to `/var/lib/banyan/data/` on each agent.
- **tmpfs**: In-memory temporary storage with optional size limits.
- **NFS shared volumes**: Declare NFS in the manifest, Banyan mounts it on each agent automatically. Multiple replicas on different agents share the same data.
- **Read-only mounts**: Append `:ro` or set `read_only: true` to prevent container writes.
- **Placement + volumes**: Pin stateful services to specific agents with `deploy.placement.node` to ensure data locality.

See [Manifest Reference — Volumes](/reference/manifest/#volumes) for syntax and examples.

---

## Milestone 9 — Auto-Scaling & Workload Rebalancing

Status: **Done**

Automatic horizontal scaling based on CPU metrics, manual scaling via CLI, and workload rebalancing across agents.

- **`banyan-cli scale`**: Adjust replica counts on a running deployment without redeploying. Containers are added or removed individually — no blue-green, no new deployment ID.
- **Auto-scaling rules in manifest**: Define `deploy.autoscale` with `min`, `max`, `target_cpu`, and `cooldown`. Engine evaluates CPU metrics every 30 seconds and adjusts replicas automatically.
- **Per-container metrics**: Agents collect CPU and memory usage per container via `nerdctl stats` and report to the engine in health checks.
- **Graceful scale-down**: Removing containers follows a drain sequence — remove from proxy, remove DNS, wait grace period, then stop. No dropped requests.
- **Workload rebalancing**: Engine detects overloaded agents (CPU or memory > 95%) and migrates stateless containers to underloaded agents. Five safeguards prevent infinite migration: per-container cooldown (10 min), high threshold (95%), target validation, minimum imbalance (30%), and max one migration per agent per cycle.

See [Auto-Scaling](/guides/auto-scaling/) for the guide and [Manifest Reference — Autoscale](/reference/manifest/#auto-scaling) for syntax.

---

## Milestone 10 — Web Monitoring Dashboard

Web-based dashboard for cluster visualization and monitoring.

- Cluster overview with all nodes and services
- Per-node resource usage graphs (CPU, memory, disk)
- Per-service metrics (replicas, throughput, error rate)
- Deployment history and status timeline
- Real-time metrics and live updates
- Container log viewer with filtering

The terminal dashboard (`banyan-cli dashboard`) is already available — see [Milestone 4.6](#milestone-46--live-terminal-dashboard). This milestone adds a web-based UI for teams that prefer browser-based monitoring.

---

## Milestone 11 — Secrets Management

Status: **Done**

Manage sensitive configuration — database passwords, API keys, tokens — without plaintext in manifests or source control.

- **`banyan-cli secret` commands**: `create`, `list`, `get` (with `--reveal`), `delete` for managing encrypted secrets
- **AES-256-GCM encryption**: Secrets encrypted at rest in etcd with a 256-bit key stored on the engine (`/etc/banyan/keys/secrets.key`)
- **Manifest `secrets:` field**: Reference secrets by name — injected as environment variables into containers at runtime
- **Just-in-time resolution**: Secret values never stored in task records. Decrypted only during PollTasks (in-memory), transmitted over WireGuard
- **Deploy-time validation**: Deploying with a missing secret fails immediately with an actionable error
- **Delete protection**: Secrets referenced by running deployments cannot be deleted

See [Secrets](/guides/secrets/) for the guide and [Manifest Reference — Secrets](/reference/manifest/#secrets) for syntax.

---

## Milestone 12 — Advanced Security

Authorization and certificate lifecycle management.

- Attribute-based access control (ABAC) for CLI commands and API actions — define roles and permissions in a config file, enforce in engine gRPC handlers
- Certificate rotation support

---

## Milestone 13 — Advanced Networking

Service discovery, traffic policies, and encrypted communication across the cluster.

- **Health-check-based routing**: Only route to healthy containers — health status is already tracked via `healthcheck:` in the manifest; next step is filtering backends by health status in HeartbeatResponse
- **Session affinity**: Optional sticky sessions per service using iptables `recent` module or connection tracking (`session_affinity: true` in banyan.yaml)
- **Network policies**: Control which services can communicate — iptables rules on each agent to filter traffic between service subnets (service-level allow/deny in banyan.yaml)
- **VPC peering**: Allow explicit cross-deployment communication — deployments are isolated by default (per-deployment iptables chains); VPC peering lets users define exceptions so specific services in one deployment can reach services in another (e.g., a shared database deployment)
- **Ingress / L7 routing**: HTTP path/host-based routing via a lightweight reverse proxy (Caddy or Envoy) auto-configured from service definitions

---

## Milestone 14 — Self-Healing Deployments

Redesign the deployment lifecycle so Banyan keeps user workloads running through any failure — container crashes, agent deaths, engine restarts, or full cluster outages. Replace the current case-by-case patches with a systematic desired-state reconciliation engine.

### Core architecture

- **Desired state model**: The deployment record IS the desired state. A "running" deployment with 3 web replicas and 1 db replica means Banyan must ensure exactly that is running at all times, not just at deploy time.
- **Actual state tracking**: Each agent reports its full container inventory (name, status, IP, metrics) in every health report. The engine maintains an authoritative map of actual state per agent.
- **Reconciliation loop**: A dedicated engine loop (every 10s, configurable) compares desired vs actual state for every running deployment and generates repair tasks. This is the single source of all recovery — not scattered across health reports, registration, and dashboard queries.

### Failure recovery matrix

Every failure scenario has a defined detection method, recovery action, and timing guarantee:

- **Container crashes on healthy agent**: Detected via health report (container status → exited). Recovery: create new `create_and_start` task on same agent. Respects `restart:` policy from manifest (always, on-failure, no). Timing: within one health check interval (~10s).
- **Agent restarts, containers gone**: Detected via health report with 0 containers (agent alive, nothing running). Recovery: re-create all tasks for that agent's share of every running deployment. Timing: within one health check interval (~10s).
- **Single agent dies**: Detected via staleness (no heartbeat for >60s). Recovery: reschedule the dead agent's tasks to other available agents using resource-aware scheduling. Grace period before rescheduling (configurable, default 2 min) to avoid flapping if agent is just rebooting. Timing: grace period + scheduling cycle.
- **Agent comes back after rescheduling**: Re-registers, receives cleanup tasks for old containers. No duplicate work — tasks already rescheduled elsewhere are not re-assigned.
- **Engine restarts**: On startup, engine runs a full reconciliation pass before accepting new deploys. Compares all "running" deployment records against last-known task states. Generates repair tasks for any drift found. Timing: immediate on startup.
- **All agents die**: All deployments marked as degraded (not stopped — desired state preserved). Tasks are queued. As agents come back and register, engine immediately schedules pending repair tasks. First agent gets work within seconds of registering.
- **Network partition**: Agent can't reach engine but containers are fine. Engine sees agent as stale, but uses an extended grace period (configurable, default 5 min) before rescheduling — partitions are usually transient. If agent reconnects, it sends a health report and the engine sees containers are still running. No unnecessary rescheduling.
- **Partial deployment failure**: Some tasks succeed, some fail. Engine retries failed tasks (up to configurable limit, default 3). If retry limit exceeded, deployment marked as degraded with clear error. Successful containers keep running — no full rollback for partial failures.

### Restart policy support

Respect Docker Compose `restart:` field in the manifest:
- `restart: always` (default) — always restart crashed containers
- `restart: on-failure` — restart only on non-zero exit code
- `restart: no` — never restart; mark as exited and leave it

### Rescheduling safeguards

Prevent cascading failures and flapping:
- **Grace period**: Don't reschedule immediately when an agent goes stale. Wait for it to come back (default 2 min).
- **Anti-flapping**: Per-container cooldown (5 min) prevents the same container from being rescheduled repeatedly between agents.
- **Capacity check**: Don't reschedule if remaining agents lack capacity. Mark deployment as degraded with actionable error instead.
- **Stateful pinning**: Services with `deploy.placement.node` or local volume mounts are NOT rescheduled to other agents — they wait for their agent to come back.

### Observability

- **Deployment health status**: New field on DeploymentRecord — `healthy`, `degraded`, `recovering`, `stopped`. Dashboard and CLI show this prominently.
- **Reconciliation events**: Every drift detection and repair action emits a cluster event (visible in dashboard and `banyan-cli events`).
- **Reconciliation metrics**: Prometheus counters for drift detections, repair tasks created, rescheduling events, and failed recoveries.

### Agent lifecycle cleanup

- **Graceful shutdown**: Agent traps SIGTERM/SIGINT and cleans up all resources — VXLAN interface, WireGuard tunnel, CNI config, DNS server, iptables rules. No stale interfaces left behind.
- **Stale interface recovery on startup**: Before initializing networking, agent detects and removes leftover interfaces (banyan0, vxlan1, wg-control) from a previous unclean shutdown. Startup always succeeds regardless of prior state.

### Critical design constraint: container name is NOT a unique identifier

Container names like `my-app-api-0` are reused across deployments. The same app deployed three times creates three tasks all named `my-app-api-0`. Any logic that matches by container name alone will incorrectly update/affect tasks from old deployments. This applies to:
- Health report processing (status updates, IP assignment, metrics)
- Unreported container detection (marking containers as exited)
- Cleanup task creation (stop_and_remove)
- Log streaming (which container to stream from)
- StopTask routing (which task to stop)

**All operations must scope by deployment ID** (or use the task ID directly). Container name is only unique within a single deployment, never across deployments. The reconciliation engine must enforce this invariant everywhere.

### What this replaces

This milestone consolidates and replaces all current ad-hoc recovery logic:
- Health report unreported-container detection (current quick fix)
- Stale agent reconciliation in engine loop (current quick fix)
- Dashboard auto-reconcile for superseded deployments
- Agent-side `restoreActiveContainers()` on Register

---

## Milestone 15 — Rootless CLI

Remove the sudo requirement from `banyan-cli`. Engine and agent need root (they manage containers, networking, and system services) but the CLI is a user tool — it should work without elevated privileges.

- **User-space config**: Move CLI config from `/etc/banyan/banyan.yaml` (root-owned) to `~/.config/banyan/config.yaml` (user-owned). `banyan-cli init` writes to user dir. Engine/agent config stays in `/etc/banyan/`.
- **Userspace WireGuard**: Replace kernel WireGuard (`wg-ctl-cli` interface, requires root) with a userspace implementation (e.g., `wireguard-go` or embedded Go WireGuard via `golang.zx2c4.com/wireguard`). No kernel interface, no root needed. The tunnel runs in-process for the duration of the CLI command.
- **No sudo for any CLI command**: `banyan-cli up`, `dashboard`, `logs`, `scale`, `down`, `secret` — all work as a normal user.
- **Migration path**: `banyan-cli init` detects existing `/etc/banyan/` config and offers to migrate CLI section to `~/.config/banyan/`. Existing root-based setups keep working.
- **Key storage**: CLI private key moves to `~/.config/banyan/keys/cli.key` with `0600` permissions (user-owned, not root-owned).
- **`banyan-cli login`**: No longer needs sudo — sets up userspace WireGuard tunnel in the background or per-command.
