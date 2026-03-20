---
title: High Availability
description: Run multiple engines so your cluster keeps working when a server goes down.
sidebar:
  order: 3
---

By default, Banyan runs one engine that manages everything. That's the right choice for most teams — no extra setup, no moving parts.

When you need the cluster to survive an engine failure, add a second engine. Both engines are active: they handle CLI commands, agent heartbeats, and schedule deployments. If one goes down, the other continues immediately. No election delay, no manual failover.

## What changes

| | Single engine (default) | Multiple engines |
|---|---|---|
| **etcd** | Managed by Banyan | You provide an etcd cluster |
| **Registry** | Managed by Banyan | You provide an OCI registry |
| **Scheduling** | Direct | Coordinated via per-deployment locks in etcd |
| **Agent config** | One engine address | List of engine addresses |
| **Failover** | None (single point) | Instant — other engines continue |
| **Setup effort** | Zero | You run etcd and a registry separately |

The trade-off is real: HA means running your own etcd and registry. For many teams, a single engine with a good backup strategy is enough. HA is for when downtime of the control plane is not acceptable.

## Prerequisites

Before setting up multiple engines, you need:

- **An etcd cluster** — at least 3 nodes for etcd's own HA. A single etcd node works for testing but defeats the purpose.
- **An OCI registry** — any Docker-compatible registry (Harbor, Docker Registry, GitLab Container Registry, etc.). All engines push/pull from the same registry.

:::note
Banyan's managed etcd and managed registry are single-process — they can't be shared between engines. That's why HA mode requires external services.
:::

## 1. Initialize the engines

On Engine 1 (`192.168.1.10`):

```bash
sudo banyan-engine init
```

The wizard asks for the deployment mode:

```
Deployment mode:
  > Single engine — zero config, everything managed for you (recommended)
    Multi-engine HA — 2+ engines for high availability (requires your own etcd and registry)
```

Choose **Multi-engine HA**. The wizard then asks for:

- **External etcd endpoint** — your etcd cluster address (e.g., `http://etcd.internal:2379`)
- **External registry URL** — your OCI registry (e.g., `registry.internal:5000`)
- **Etcd connection security** — None, password, TLS, or mTLS

```bash
sudo systemctl enable --now banyan-engine
```

Repeat on Engine 2 (`192.168.1.20`) with the same etcd and registry addresses.

Both engines generate their own WireGuard keypairs during init. Copy each engine's public key — agents need it.

## 2. Configure agents with multiple engines

On each agent, run `banyan-agent init`. The wizard asks for the engine host and port as usual. After that:

```
Add additional engine endpoints for high availability?
  For single-engine setups, choose No

  > Yes / No
```

Choose **Yes**. The wizard prompts for each engine's address and WireGuard public key:

```
Engine #2 — address:
  host:port (or leave empty to finish)

  > 192.168.1.20:50051

Engine #2 — WireGuard public key:
  Displayed during 'banyan-engine init' on that server

  > ZwlhPtN5dWw4TRSOkrUhjwC4w1jtABOnFUgVEdHImi8=
```

The primary engine (from the host/port and WG key you entered earlier) is automatically included as engine #1. Each engine has its own WireGuard key — the agent sets up encrypted tunnels to all of them. Add as many engines as you need — leave the address empty to finish.

The agent connects to the first available engine. If that engine goes down, the agent reconnects to the next one within seconds.

```bash
sudo systemctl enable --now banyan-agent
```

## 3. Configure the CLI

On your deploy machine, run `banyan-cli init`. Same pattern — after the engine host/port, it asks:

```
Add additional engine endpoints for high availability?
  > Yes / No
```

If yes, add each engine's address and WireGuard public key one at a time — same flow as the agent wizard. The CLI sets up tunnels to all engines and connects to the first one that responds.

## 4. Verify

Check that both engines are registered:

```bash
banyan-cli engine
```

```
Engine
==================================================
  Status:    running
  Uptime:    5m
  ...

Cluster Summary
--------------------------------------------------
  Agents:       2/2 connected
  ...
```

Both engines see the same cluster state because they share etcd. You can run `banyan-cli` commands against either engine — the result is the same.

## How it works

All engines are identical. There's no leader, no primary, no standby. Every engine:

- Handles CLI commands (deploy, status, down)
- Accepts agent registrations and heartbeats
- Runs the scheduling loop to assign tasks to agents
- Monitors deployment progress

Per-deployment distributed locks in etcd prevent two engines from scheduling the same deployment twice. The engine that receives a Deploy command schedules it immediately. Other engines skip it when they see it's already been handled.

```
CLI runs "banyan-cli up"
  → connects to Engine 2 (first healthy endpoint)
  → Engine 2 writes deployment to etcd, schedules tasks immediately
  → Engine 1's loop sees the deployment is already scheduled — skips it
  → agents poll either engine for tasks — same data from etcd
```

## When an engine goes down

If Engine 1 crashes:

- **Agents connected to Engine 1** detect heartbeat failures and reconnect to Engine 2 within seconds
- **Engine 2 continues scheduling** — it was already active
- **CLI commands** automatically try the next endpoint
- **Engine 1's registration in etcd expires** after 15 seconds (TTL-based)

When Engine 1 comes back, it re-registers in etcd and starts processing work again. No manual intervention needed.

## Config reference

Engine config (`/etc/banyan/banyan.yaml`):

```yaml
engine:
  multi_engine: true
  managed_etcd: false
  store_address: "http://etcd.internal:2379"
  managed_registry: false
  external_registry_url: "registry.internal:5000"
```

Agent config:

```yaml
agent:
  engines:
    - address: "192.168.1.10:50051"
      wg_public_key: "engine-1-public-key-here"
    - address: "192.168.1.20:50051"
      wg_public_key: "engine-2-public-key-here"
```

CLI config:

```yaml
cli:
  engines:
    - address: "192.168.1.10:50051"
      wg_public_key: "engine-1-public-key-here"
    - address: "192.168.1.20:50051"
      wg_public_key: "engine-2-public-key-here"
```

:::caution
All engines must point to the same etcd cluster and the same registry. Engines don't replicate state between themselves — etcd is the single source of truth.
:::

## Going back to single-engine

If you decide HA isn't worth the operational overhead:

1. Remove `multi_engine: true` from the engine config and switch back to `managed_etcd: true` and `managed_registry: true`
2. Remove the `engines:` list from agent and CLI configs
3. Restart all components

You're back to zero-config single-engine mode.
