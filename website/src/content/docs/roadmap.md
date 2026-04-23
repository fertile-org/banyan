---
title: Roadmap
description: What's next for Banyan.
sidebar:
  order: 99
---

Banyan's core orchestration platform is feature-complete. Twelve milestones have shipped — from container scheduling to self-healing deployments. Here's where things stand and what's coming next.

## What's Done

The following features are shipped and production-ready:

| Area | Features |
|---|---|
| **Orchestration** | Docker Compose-compatible YAML manifests, multi-agent scheduling, resource-aware placement, blue-green redeployment |
| **Networking** | WireGuard overlay, cross-host load balancing, service DNS, IPAM, port forwarding |
| **High Availability** | Active-active multi-engine, agent failover, CLI failover, distributed locking |
| **Storage** | Named volumes, bind mounts, tmpfs, NFS shared volumes, placement pinning |
| **Security** | X25519 public key auth, WireGuard encryption, AES-256-GCM secrets management, agent tags for environment isolation |
| **Scaling** | Auto-scaling (CPU-based), manual `banyan-cli scale`, workload rebalancing with anti-flap safeguards |
| **Self-Healing** | Desired-state reconciliation (10s loop), restart policies, agent failure rescheduling, engine restart recovery |
| **Observability** | Terminal dashboard (TUI), web dashboard, container logs, per-container CPU/memory metrics, deployment health status |
| **Operations** | Systemd services, env_file support, CLI secret management, per-service deploy/down |

**12 milestones shipped.** See [Roadmap v1 (Archive)](/roadmap-v1/) for the full milestone-by-milestone history.

---

## What's Next

These are the remaining milestones — advanced features that round out the platform for production team use.

### M13 — Advanced Security

Attribute-based access control (ABAC) and certificate rotation.

### M14 — Advanced Networking

Health-check-based routing, session affinity, network policies, VPC peering, and L7 ingress.

### M15 — Rootless CLI

Remove the `sudo` requirement from all CLI commands — userspace WireGuard, user-space config directory.

### M16 — Dashboard: Manifest Editor & Container Exec

In-browser YAML editor with one-click deploy, WebSocket terminal into containers, and TUI/web parity policy.

---

## Contributing

Banyan is open source. [Open an issue](https://github.com/fertile-org/banyan/issues) or start a discussion if there's something you'd like to see on the roadmap.
