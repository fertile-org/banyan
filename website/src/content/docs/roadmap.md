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

## Milestone 9 — Auto-Scaling

Scale services based on metrics and support automatic rollback.

- Define scaling rules in the manifest (min/max replicas, target thresholds)
- Engine evaluates metrics against rules and adjusts replica count
- Graceful scale-down (drain before stopping)
- Automatic rollback on deployment failure

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

## Milestone 11 — Advanced Security

Authorizaton, secrets management, and certificate rotation for secure cluster operations.

- attribute-based access control (ABAC) for CLI commands and API actions — define roles and permissions in a config file, enforce in engine gRPC handlers
- secrets management: encrypt secrets at rest in etcd, inject into containers as environment variables or files, with CLI commands to create and manage secrets
- Certificate rotation support

---

## Milestone 12 — Dynamic Workload Rebalancing

Automatically redistribute services across nodes based on actual resource usage and node capacity.

- Engine tracks actual CPU/memory usage per container (from agent health reports)
- Identify over-utilized and under-utilized nodes
- Gracefully move containers from crowded nodes to nodes with available capacity
- Drain-and-restart for stateless services; manual rebalancing only for stateful services
- Configurable triggers (e.g., migrate when node >90% full)
- Safety checks: verify destination has capacity before migration
- Rollback support for failed migrations

---

## Milestone 13 — Advanced Networking

Service discovery, traffic policies, and encrypted communication across the cluster.

- **Health-check-based routing**: Only route to healthy containers — health status is already tracked via `healthcheck:` in the manifest; next step is filtering backends by health status in HeartbeatResponse
- **Session affinity**: Optional sticky sessions per service using iptables `recent` module or connection tracking (`session_affinity: true` in banyan.yaml)
- **Network policies**: Control which services can communicate — iptables rules on each agent to filter traffic between service subnets (service-level allow/deny in banyan.yaml)
- **VPC peering**: Allow explicit cross-deployment communication — deployments are isolated by default (per-deployment iptables chains); VPC peering lets users define exceptions so specific services in one deployment can reach services in another (e.g., a shared database deployment)
- **Ingress / L7 routing**: HTTP path/host-based routing via a lightweight reverse proxy (Caddy or Envoy) auto-configured from service definitions
