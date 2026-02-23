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

See what's running, check container health, stream logs, and stop deployments — all from the CLI.

- Agent monitors container health after deployment (running, exited, restarting)
- Agent reports per-container status back to Engine via gRPC
- `banyan-cli status` shows per-service and per-container status
- `banyan-cli logs` streams container logs from agents (via engine gRPC proxy)
- Detect and surface failed containers (e.g., exited immediately after start)
- `banyan-cli down` command to stop and remove all containers for a deployment

---

## Milestone 3 — Basic Security

Status: **Done**

Secure gRPC communication between CLI, Engine, and Agents with password-based token exchange.

- All inter-component communication uses gRPC with token authentication
- Password used once at init, exchanged for a long-lived auth token
- Token stored as SHA-256 hash in etcd — compromise doesn't leak usable tokens
- CLI tokens expire after 30 days; agent tokens don't expire
- Config file at `/etc/banyan/banyan.yaml` with sections: `security`, `engine`, `agent`, `cli`
- `init` and `auth` commands for engine, agent, and CLI
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

## Milestone 5 — Metrics Collection

Collect and expose resource metrics from every node and container in Prometheus-compatible format.

- **Prometheus-compatible metrics**: Expose `/metrics` endpoint in Prometheus format
- Agent-side metric collection: CPU, memory, disk usage per container
- Container-level metrics: per-container CPU%, memory usage, restart count
- Node-level metrics: total CPU, memory, disk usage per agent
- Service-level metrics: request throughput, error rate per service
- **Terminal monitoring dashboard**: `banyan-cli monitor` — a live terminal UI (built with [Bubbletea](https://github.com/charmbracelet/bubbletea)) showing real-time cluster metrics
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

## Milestone 9 — Monitoring Dashboard

Web-based dashboard for cluster visualization and monitoring.

- Cluster overview with all nodes and services
- Per-node resource usage graphs (CPU, memory, disk)
- Per-service metrics (replicas, throughput, error rate)
- Deployment history and status timeline
- Real-time metrics and live updates
- Container log viewer with filtering

The terminal monitoring dashboard (`banyan-cli monitor`) is delivered in Milestone 5.

---

## Milestone 10 — Advanced Security

Stronger authentication for production environments.

- Private key authentication for agent-to-engine connections
- Private key authentication for CLI-to-engine and CLI-to-agent
- Key generation and distribution tooling
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

## Milestone 12 — Advanced Metrics and Dashboard Enhancements

Deeper observability and richer operational tooling.

- Custom application metrics (user-defined)
- Alerting rules and notifications
- Historical trends and capacity planning views
- Multi-cluster dashboard support
- Metric export to external systems (Prometheus, Grafana)
