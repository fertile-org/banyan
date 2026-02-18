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

## Milestone 2 — Basic Security

Secure communication between CLI, Engine, and Agents.

- Username/password authentication for agent-to-engine connections
- Username/password authentication for CLI-to-engine commands
- Username/password authentication for CLI-to-agent commands
- Credential configuration via CLI flags and config file

---

## Milestone 3 — Metrics Collection

Collect and store resource metrics from every node and container.

- Agent-side metric collection: CPU, memory, disk usage
- Container-level metrics: per-container CPU, memory, restart count
- Request throughput metrics per service
- Metric storage in etcd (or lightweight time-series store)
- Metric retrieval API for other components to consume

---

## Milestone 4 — Auto-Scaling and Redeployment

Scale services based on metrics and support zero-downtime updates.

- **Auto-scaling**: Define scaling rules in the manifest (min/max replicas, target thresholds)
- **Auto-scaling**: Engine evaluates metrics against rules and adjusts replica count
- **Auto-scaling**: Graceful scale-down (drain before stopping)
- **Redeployment**: Rolling update when service image or config changes
- **Redeployment**: Health check between rollout steps
- **Redeployment**: Automatic rollback on failure

---

## Milestone 5 — Monitoring Dashboard and CLI

Give operators visibility into the cluster through a web UI and CLI commands.

- **CLI**: Live cluster status with per-node resource usage
- **CLI**: Per-service metrics (replicas, throughput, error rate)
- **CLI**: Container log streaming
- **Dashboard**: Web UI for cluster overview
- **Dashboard**: Deployment history and status
- **Dashboard**: Real-time metrics and graphs

---

## Milestone 6 — Advanced Security

Stronger authentication model for production environments.

- Private key authentication for agent-to-engine connections
- Private key authentication for CLI-to-engine and CLI-to-agent
- Key generation and distribution tooling
- Certificate rotation support

---

## Milestone 7 — Advanced Metrics and Dashboard Enhancements

Deeper observability and richer operational tooling.

- Custom application metrics (user-defined)
- Alerting rules and notifications
- Historical trends and capacity planning views
- Multi-cluster dashboard support
- Metric export to external systems (Prometheus, Grafana)
