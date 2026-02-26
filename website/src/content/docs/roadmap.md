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
- Agent workers with containerd/nerdctl container management
- Round-robin scheduling across workers
- CLI for engine, agent, and deploy workflows
- VPC networking layer (IPAM, DNS, CNI)
- E2E test infrastructure

---

## Milestone 2 — Service Observability

Status: **Done**

See what's running, check container health, stream logs, and stop deployments — all from the CLI.

- Agent monitors container health after deployment (running, exited, restarting)
- Agent reports per-container status back to Engine via gRPC
- `banyan-cli status` shows per-service and per-container status
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

- WireGuard overlay (default) with VXLAN fallback, both managed by Engine via abstract `OverlayDriver` interface
- Built-in VXLAN overlay managed by Engine with deterministic VTEP MACs
- Per-agent /24 subnet allocation from VPC CIDR via `SubnetAllocator`
- Peer discovery via heartbeat RPC (15s convergence)
- iptables DNAT proxy on each agent for port forwarding to container backends
- Cross-host load balancing: every agent aware of all service backends cluster-wide, probability-based DNAT rules distribute traffic across all replicas regardless of which agent they run on
- Service DNS: agent-local DNS server on bridge gateway IP resolves `<service>.internal` to container IPs, with `--dns-search internal` enabling short names (e.g., `ping db` from any container)

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

## Milestone 4.5 — Rootless Mode

Run Banyan without root. Today every component needs `sudo` — the engine to manage etcd and WireGuard, the agent to run containerd and configure networking, even the CLI to write config during `init`. This milestone removes that requirement so Banyan works on shared servers, locked-down environments, and developer machines without elevated privileges.

- **Rootless containerd support**: Agent detects rootless containerd and adapts nerdctl commands accordingly (user socket, unprivileged namespace)
- **User-space networking**: Replace kernel WireGuard with wireguard-go and iptables DNAT with a Go TCP proxy — no kernel modules, no sysctl writes, no `/proc` access
- **User-scoped config and data**: Config in `~/.config/banyan/` and data in `~/.local/share/banyan/` instead of `/etc/banyan/` and `/var/lib/banyan/` — no root needed for `init`
- **Unprivileged ports only**: All default ports already above 1024 (gRPC 50051, registry 5000); services needing 80/443 can use port mapping from a higher port
- **Graceful fallback**: When running as root, Banyan uses the faster kernel-mode networking (WireGuard, iptables). Without root, it transparently falls back to user-space equivalents. Same manifest, same commands — just without `sudo`

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

## Milestone 5 — Metrics Collection

Collect and expose resource metrics from every node and container in Prometheus-compatible format.

- **Prometheus-compatible metrics**: Expose `/metrics` endpoint in Prometheus format
- Agent-side metric collection: CPU, memory, disk usage per container
- Container-level metrics: per-container CPU%, memory usage, restart count
- Node-level metrics: total CPU, memory, disk usage per agent
- Service-level metrics: request throughput, error rate per service
- Metric storage in etcd for short-term retention
- Metric retrieval API for other components to consume

---

## Milestone 6 — Health-Based Scheduling and Resource Requests

Smarter task distribution based on node resources instead of simple round-robin.

- Agent reports node resource usage (CPU, memory, disk) to Engine via etcd
- Engine selects the node with the most available resources when scheduling
- Resource requests in banyan.yaml: services can declare CPU and memory requirements (e.g., `cpus: 2`, `memory: 4g`)
- **Default resource requests**: Services without explicit requirements get sensible defaults (512MB RAM, 1 CPU)
- Engine validates that target node has sufficient resources before assigning a task
- Engine rejects deployments that exceed total cluster capacity

---

## Milestone 7 — Multi-Engine High Availability

Multiple active engine nodes share workload for high availability and horizontal scaling.

- **Active-active engines**: Any engine can handle CLI requests and schedule tasks
- **etcd coordination**: Task claiming via Compare-And-Swap to prevent duplication
- **Distributed registry**: Index-based lookup so agents pull images from the correct engine
- **Optimistic locking**: Concurrent deployment updates are serialized
- **Session state in etcd**: Agents can reconnect to any engine
- **Client load balancing**: CLI connects to any available engine

---

## Milestone 8 — Auto-Scaling

Scale services based on metrics and support automatic rollback.

- Define scaling rules in the manifest (min/max replicas, target thresholds)
- Engine evaluates metrics against rules and adjusts replica count
- Graceful scale-down (drain before stopping)
- Automatic rollback on deployment failure

---

## Milestone 9 — Web Monitoring Dashboard

Web-based dashboard for cluster visualization and monitoring.

- Cluster overview with all nodes and services
- Per-node resource usage graphs (CPU, memory, disk)
- Per-service metrics (replicas, throughput, error rate)
- Deployment history and status timeline
- Real-time metrics and live updates
- Container log viewer with filtering

The terminal dashboard (`banyan-cli dashboard`) is already available — see [Milestone 4.6](#milestone-46--live-terminal-dashboard). This milestone adds a web-based UI for teams that prefer browser-based monitoring.

---

## Milestone 10 — Advanced Security

Stronger authentication model for production environments.

- ~~Private key authentication for agent-to-engine connections~~ → **Done**: X25519 public key whitelist authentication
- ~~Private key authentication for CLI-to-engine and CLI-to-agent~~ → **Done**: CLI uses same public key auth
- ~~Key generation and distribution tooling~~ → **Done**: `init` commands generate keypairs, admin copies public keys to engine
- ~~Encrypted control plane~~ → **Done**: WireGuard control tunnel (`wg-control`) encrypts all gRPC traffic between engine, agents, and CLI (port 51821/UDP)
- Certificate rotation support

---

## Milestone 11 — Dynamic Workload Rebalancing

Automatically redistribute services across nodes based on actual resource usage and node capacity.

- Engine tracks actual CPU/memory usage per container (from metrics collected in Milestone 5)
- Identify over-utilized and under-utilized nodes
- Gracefully move containers from crowded nodes to nodes with available capacity
- Drain-and-restart for stateless services; manual rebalancing only for stateful services
- Configurable triggers (e.g., migrate when node >90% full)
- Safety checks: verify destination has capacity before migration
- Rollback support for failed migrations

---

## Milestone 12 — Advanced Networking

Service discovery, traffic policies, and encrypted communication across the cluster.

- **Health-check-based routing**: Only route to healthy containers — filter backends by `container_status` before including in HeartbeatResponse
- **Session affinity**: Optional sticky sessions per service using iptables `recent` module or connection tracking (`session_affinity: true` in banyan.yaml)
- **Network policies**: Control which services can communicate — iptables rules on each agent to filter traffic between service subnets (service-level allow/deny in banyan.yaml)
- ~~**WireGuard overlay driver**: Alternative to VXLAN via the existing `OverlayDriver` interface~~ → **Done**: WireGuard is the default overlay, VXLAN kept as fallback
- **Ingress / L7 routing**: HTTP path/host-based routing via a lightweight reverse proxy (Caddy or Envoy) auto-configured from service definitions
- **mTLS between services**: Encrypted service-to-service communication — WireGuard approach (transparent at network layer) or sidecar proxy pattern
- **Multi-tenant network isolation**: Separate VPC CIDRs per deployment or tag group with different VNIs
