---
title: Auto-Scaling
description: Scale services automatically based on CPU metrics, or manually with a single command.
sidebar:
  order: 5
---

Scale services up when demand increases and back down when it drops — defined in your manifest, handled by the engine.

## Manual scaling

Adjust replicas on a running deployment without redeploying:

```bash
banyan-cli scale my-app api=5
```

```
  api: 2 → 5 replicas (scaling up)
```

New containers are distributed across available agents using the same resource-aware scheduling as initial deployment. Scale-down removes containers gracefully — proxy removal, DNS cleanup, drain period, then stop.

Scale multiple services at once:

```bash
banyan-cli scale my-app api=5 web=3
```

No blue-green. No new deployment ID. Containers are added or removed individually from the running deployment.

## Automatic scaling

Add `deploy.autoscale` to your manifest and the engine adjusts replicas based on CPU usage:

```yaml
name: my-app

services:
  api:
    image: myapp/api:latest
    deploy:
      replicas: 2               # initial count
      autoscale:
        min: 2                  # never go below 2
        max: 10                 # never go above 10
        target_cpu: 70          # target average CPU %
        cooldown: 30s           # min time between scale events
      stop_grace_period: 5s     # drain time before stopping
    ports:
      - "8080:8080"
```

Deploy this manifest with `banyan-cli up -f banyan.yaml`. Banyan starts 2 replicas and manages the count from there.

### What happens under the hood

1. **Metrics collection** — Each agent runs `nerdctl stats` every 10 seconds and reports per-container CPU and memory to the engine.

2. **Autoscale evaluation** — Every 30 seconds, the engine checks each service with `deploy.autoscale` configured:
   - If average CPU across all replicas > `target_cpu` → add 1 replica (up to `max`)
   - If average CPU < `target_cpu / 2` → remove 1 replica (down to `min`)

3. **Cooldown** — After any scale event, no further scaling happens for the `cooldown` period. This prevents rapid flapping when CPU fluctuates near the threshold.

4. **Graceful drain** — When removing a container, the engine follows a sequence: remove from the load balancer, remove DNS records, wait `stop_grace_period`, then stop the container.

### Scale timing

From a load spike to a new container being ready:

| Step | Typical time |
|------|-------------|
| Agent detects CPU increase | ~10s (health check interval) |
| Engine evaluates autoscale | ~30s (evaluation interval) |
| New container starts | ~5-15s (image pull + startup) |
| **Total** | **~45-55s** |

Scale-down is more conservative. The hysteresis threshold (`target_cpu / 2`) means CPU must drop significantly before replicas are removed. Combined with the cooldown, this prevents premature scale-down after a brief traffic spike.

## Combining autoscale with other features

### Placement constraints

Autoscale respects `deploy.placement.node`. New replicas are only scheduled on agents matching the glob pattern:

```yaml
services:
  api:
    deploy:
      placement:
        node: compute-*         # only on compute nodes
      autoscale:
        min: 1
        max: 5
        target_cpu: 70
```

### Resource limits

Pair autoscale with `deploy.resources` so the scheduler places new replicas on agents with enough capacity:

```yaml
services:
  api:
    deploy:
      replicas: 2
      resources:
        limits:
          memory: 512m
          cpus: "1"
      autoscale:
        min: 2
        max: 8
        target_cpu: 70
```

### Stateful services

Auto-scaling works best with stateless services. For databases and other stateful containers, use manual scaling instead:

```bash
# Scale Redis to 3 replicas (manual, intentional)
banyan-cli scale my-app redis=3
```

:::caution
Avoid auto-scaling services with volumes. Each new replica gets its own local volume — data is not shared. For shared state, use NFS volumes or an external database.
:::

## Workload rebalancing

Separate from auto-scaling, the engine monitors agent-level resource usage and rebalances containers when agents become unevenly loaded.

**When it triggers**: An agent's CPU or memory exceeds 95%.

**What it does**: Migrates one stateless container from the overloaded agent to the least loaded agent.

**What it won't do**:
- Move containers with volumes (stateful)
- Move containers with placement constraints (pinned)
- Move a container to an agent that would exceed 70% load after migration
- Move a container that was migrated less than 10 minutes ago
- Move more than one container per agent per cycle

Rebalancing is automatic and requires no configuration. It handles situations like an agent restarting empty while others carry its former workload.

## Monitoring auto-scale events

Watch the engine logs for scaling activity:

```
engine: Autoscale: adjusting replicas  deployment=my-app  service=api  from=2  to=3  avg_cpu=85.0%  target_cpu=70
```

Check current replica counts:

```bash
banyan-cli deployment my-app
```

Auto-scale events also appear in the terminal dashboard (`banyan-cli dashboard`) and the cluster events (`banyan-cli events`).
