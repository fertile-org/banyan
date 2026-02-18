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

## Milestone 4 — Metrics Collection

Collect and store resource metrics from every node and container.

- Agent-side metric collection: CPU, memory, disk usage
- Container-level metrics: per-container CPU, memory, restart count
- Request throughput metrics per service
- Metric storage in etcd (or lightweight time-series store)
- Metric retrieval API for other components to consume

---

## Milestone 5 — Health-Based Scheduling and Resource Requests

Smarter task distribution based on node resources instead of simple round-robin.

- Agent reports node resource usage (CPU, memory, disk) to Engine via etcd
- Engine selects the node with the most available resources when scheduling new tasks
- Resource requests in banyan.yaml: services can declare CPU and memory requirements (e.g. `cpus: 2`, `memory: 4g`)
- Engine validates that target node has sufficient resources before assigning a task
- Engine rejects deployments that exceed total cluster capacity

---

## Milestone 6 — Auto-Scaling and Redeployment

Scale services based on metrics and support zero-downtime updates.

- **Auto-scaling**: Define scaling rules in the manifest (min/max replicas, target thresholds)
- **Auto-scaling**: Engine evaluates metrics against rules and adjusts replica count
- **Auto-scaling**: Graceful scale-down (drain before stopping)
- **Redeployment**: Rolling update when service image or config changes
- **Redeployment**: Health check between rollout steps
- **Redeployment**: Automatic rollback on failure

---

## Milestone 7 — Monitoring Dashboard and CLI

Give operators visibility into the cluster through a web UI and CLI commands.

- **CLI**: Live cluster status with per-node resource usage
- **CLI**: Per-service metrics (replicas, throughput, error rate)
- **CLI**: Container log streaming
- **Dashboard**: Web UI for cluster overview
- **Dashboard**: Deployment history and status
- **Dashboard**: Real-time metrics and graphs

---

## Milestone 8 — Advanced Security

Stronger authentication model for production environments.

- Private key authentication for agent-to-engine connections
- Private key authentication for CLI-to-engine and CLI-to-agent
- Key generation and distribution tooling
- Certificate rotation support

---

## Milestone 9 — Advanced Metrics and Dashboard Enhancements

Deeper observability and richer operational tooling.

- Custom application metrics (user-defined)
- Alerting rules and notifications
- Historical trends and capacity planning views
- Multi-cluster dashboard support
- Metric export to external systems (Prometheus, Grafana)
