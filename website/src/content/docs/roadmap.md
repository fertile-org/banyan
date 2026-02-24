---
title: Roadmap
description: What's next for Banyan.
sidebar:
  order: 99
---

## Milestone 1 — Core Orchestration (MVP)

Status: **Done**

Deploy containers across multiple servers using a familiar YAML manifest.

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

Per-container health status, logs, and visibility from the CLI.

- Agent monitors container health after deployment (running, exited, restarting)
- Agent reports per-container status back to Engine via gRPC
- `banyan-cli status` shows per-service and per-container status (not just aggregate)
- CLI command to stream container logs from agents (via engine gRPC proxy)
- Detect and surface failed containers (e.g. exited immediately after start)
- `banyan-cli down` command to stop and remove all containers for a deployment

---

## Milestone 3 — Basic Security

Status: **Done**

Secure gRPC communication between CLI, Engine, and Agents.

- All inter-component communication uses gRPC with password authentication
- Agent → Engine: password in gRPC metadata on every call
- CLI → Engine: password in gRPC metadata on every call
- Engine → Agent: session token authentication for log streaming
- Config file at `/etc/banyan/banyan.yaml` with sections: `security`, `engine`, `agent`, `cli`
- `init` commands for engine, agent, and CLI prompt for credentials and connection info
- Three separate binaries: `banyan-engine`, `banyan-agent`, `banyan-cli`

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

## Milestone 3.6 — Basic Networking

Status: **Done**

Built-in overlay networking and cross-host load balancing without external dependencies.

- Built-in VXLAN overlay managed by Engine (replaced Flannel) with deterministic VTEP MACs
- Per-agent /24 subnet allocation from VPC CIDR via `SubnetAllocator`
- Peer discovery via heartbeat RPC (15s convergence)
- iptables DNAT proxy on each agent for port forwarding to container backends
- Cross-host load balancing: every agent aware of all service backends cluster-wide, probability-based DNAT rules distribute traffic across all replicas regardless of which agent they run on

---

## Milestone 4 — Metrics Collection

Collect and expose resource metrics from every node and container in Prometheus-compatible format.

- **Prometheus-compatible metrics**: Expose `/metrics` endpoint with Prometheus format (no custom metric format)
- Agent-side metric collection: CPU, memory, disk usage per container
- Container-level metrics: per-container CPU%, memory usage, restart count
- Node-level metrics: total CPU, memory, disk usage per agent
- Service-level metrics: request throughput, error rate per service
- **CLI monitoring interface**: Terminal UI dashboard (use https://github.com/charmbracelet/bubbletea) (similar to `htop`/`pm2`) showing live metrics directly in CLI
- Metric storage in etcd for short-term retention
- Metric retrieval API for other components to consume

---

## Milestone 5 — Health-Based Scheduling and Resource Requests

Smarter task distribution based on node resources instead of simple round-robin.

- Agent reports node resource usage (CPU, memory, disk) to Engine via etcd
- Engine selects the node with the most available resources when scheduling new tasks
- Resource requests in banyan.yaml: services can declare CPU and memory requirements (e.g. `cpus: 2`, `memory: 4g`)
- **Default resource requests**: Services without explicit resource requirements get sensible defaults (512MB RAM, 1 CPU) — configurable via engine flags
- Engine validates that target node has sufficient resources before assigning a task
- Engine rejects deployments that exceed total cluster capacity

---

## Milestone 6 — Multi-Engine High Availability

Multiple active engine nodes share workload for high availability and horizontal scaling.

- **Active-active engines**: Any engine can handle CLI requests and schedule tasks
- **etcd coordination**: Task claiming via Compare-And-Swap to prevent duplication
- **Distributed registry**: Index-based lookup so agents pull images from the correct engine
- **Optimistic locking**: Concurrent deployment updates are serialized
- **Session state in etcd**: Agents can reconnect to any engine
- **Client load balancing**: CLI connects to any available engine

See [Multi-Engine HA Design](https://github.com/fertile-org/banyan/tree/main/docs/designs/multi-engine-ha.md) for detailed architecture.

---

## Milestone 7 — Auto-Scaling and Redeployment

Scale services based on metrics and support zero-downtime updates.

- **Auto-scaling**: Define scaling rules in the manifest (min/max replicas, target thresholds)
- **Auto-scaling**: Engine evaluates metrics against rules and adjusts replica count
- **Auto-scaling**: Graceful scale-down (drain before stopping)
- ~~**Redeployment**: Rolling update when service image or config changes~~ → **Done**: Blue-green redeployment via `banyan-cli up` (full and per-service)
- ~~**Redeployment**: Health check between rollout steps~~ → **Done**: New deployment must reach `running` before old is torn down
- ~~**Port management**: Built-in TCP proxy on agents~~ → **Done**: Agents proxy host ports to containers, enabling port sharing and zero-downtime per-service deploys
- **Redeployment**: Automatic rollback on failure

---

## Milestone 8 — Monitoring Dashboard

Web-based dashboard for cluster visualization and monitoring.

- **Dashboard**: Cluster overview with all nodes and services
- **Dashboard**: Per-node resource usage graphs (CPU, memory, disk)
- **Dashboard**: Per-service metrics (replicas, throughput, error rate)
- **Dashboard**: Deployment history and status timeline
- **Dashboard**: Real-time metrics and live updates
- **Dashboard**: Container log viewer with filtering

Note: CLI monitoring interface (terminal UI) is delivered in Milestone 4.

---

## Milestone 9 — Advanced Security

Stronger authentication model for production environments.

- Private key authentication for agent-to-engine connections
- Private key authentication for CLI-to-engine and CLI-to-agent
- Key generation and distribution tooling
- Certificate rotation support

---

## Milestone 10 — Dynamic Workload Rebalancing

Automatically redistribute services across nodes based on actual resource usage and node capacity.

- **Resource monitoring**: Engine tracks actual CPU/memory usage per container (from metrics collected in Milestone 4)
- **Capacity detection**: Identify over-utilized nodes (>80% resources) and under-utilized nodes
- **Service migration**: Gracefully move containers from crowded nodes to nodes with available capacity
- **Migration strategy**: Drain-and-restart for stateless services (stop on source, start on destination)
- **Stateful handling**: Exclude databases and stateful services from auto-migration (manual rebalancing only)
- **Threshold configuration**: Configurable triggers (e.g., migrate when node >90% full or container is resource-starved)
- **Safety checks**: Verify destination node has sufficient capacity before migration
- **Rollback support**: Revert failed migrations back to original node

This milestone enables the cluster to self-optimize: services needing more resources are automatically moved to nodes where they can thrive.

---

## Milestone 11 — Advanced Metrics and Dashboard Enhancements

Deeper observability and richer operational tooling.

- Custom application metrics (user-defined)
- Alerting rules and notifications
- Historical trends and capacity planning views
- Multi-cluster dashboard support
- Metric export to external systems (Prometheus, Grafana)

---

## Milestone 12 — Advanced Networking

Service discovery, traffic policies, and encrypted communication across the cluster.

- **Service DNS**: Containers resolve service names to IPs (e.g. `curl http://api:8080` from any container). Options: `/etc/hosts` injection via CNI or lightweight DNS server on the bridge
- **Health-check-based routing**: Only route to healthy containers — filter backends by `container_status` before including in HeartbeatResponse
- **Session affinity**: Optional sticky sessions per service using iptables `recent` module or connection tracking (`session_affinity: true` in banyan.yaml)
- **Network policies**: Control which services can communicate — iptables rules on each agent to filter traffic between service subnets (service-level allow/deny in banyan.yaml)
- **WireGuard overlay driver**: Alternative to VXLAN via the existing `OverlayDriver` interface — better performance and built-in encryption, no separate mTLS needed
- **Ingress / L7 routing**: HTTP path/host-based routing via a lightweight reverse proxy (Caddy or Envoy) auto-configured from service definitions
- **mTLS between services**: Encrypted service-to-service communication — WireGuard approach (transparent at network layer) or sidecar proxy pattern
- **Multi-tenant network isolation**: Separate VPC CIDRs per deployment or tag group with different VNIs
