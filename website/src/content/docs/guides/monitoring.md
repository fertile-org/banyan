---
title: See What's Running
description: Monitor your cluster with the built-in terminal dashboard and Prometheus metrics.
sidebar:
  order: 4
---

Monitor your entire Banyan cluster — engine health, agents, deployments, and containers — from the terminal or with your existing Prometheus setup.

:::note
You need a working `banyan-cli` connection to the engine before using the dashboard. If you haven't set this up yet, follow the [Quickstart](/getting-started/quickstart/) or [Secure Your Cluster](/guides/authentication/) guide first.
:::

## Terminal dashboard

Open the dashboard:

```bash
banyan-cli dashboard
```

<div class="not-content" style="margin: 1.5rem 0; text-align: center;">
  <video autoplay loop muted playsinline style="max-width: 100%; border-radius: 8px; border: 1px solid var(--sl-color-gray-5); box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12);">
    <source src="/dashboard/demo-dashboard.mp4" type="video/mp4" />
  </video>
</div>

The dashboard connects to your engine and shows live cluster state, updating every 5 seconds. Everything runs in your terminal — no browser, no Grafana, no configuration files.

### Views

Switch views by pressing the corresponding number key:

| Key | View | What it shows |
|-----|------|---------------|
| `1` | Overview | Engine health, cluster summary, agents, deployments, and recent events — all on one screen |
| `2` | Agents | All connected agents with CPU, memory, disk usage, and container count |
| `3` | Deploys | Deployments grouped by name, with health status and version history |
| `4` | Containers | Every container across the cluster — status, image, agent, and replica info |
| `5` | Engine | Detailed engine metrics with CPU, memory, and disk progress bars |

### Navigation

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `Enter` | Drill into agent or deployment details |
| `Esc` | Go back |
| `p` | Open command palette |
| `r` | Refresh data |
| `?` | Show keyboard shortcuts |
| `q` | Quit |

Lists scroll automatically when they're longer than the screen — navigate to the bottom and the view follows, like htop.

### Refresh interval

The default refresh is 5 seconds. For a slower refresh:

```bash
banyan-cli dashboard --refresh 30s
```

---

## Prometheus metrics

The engine exposes a `/metrics` endpoint in Prometheus format. When the engine starts, it prints:

```
Prometheus metrics available at :9090/metrics
```

### Add Banyan to your Prometheus config

Add a scrape target to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: banyan
    static_configs:
      - targets: ["your-engine-host:9090"]  # Replace with your engine's address
```

Reload Prometheus, and metrics start flowing.

### Available metrics

All metrics use the `banyan_` prefix.

**Engine**

| Metric | Type | Description |
|--------|------|-------------|
| `banyan_engine_uptime_seconds` | Gauge | Seconds since engine started |
| `banyan_engine_cpu_usage_ratio` | Gauge | Engine host CPU usage (0.0–1.0) |
| `banyan_engine_memory_used_bytes` | Gauge | Engine host memory in use |
| `banyan_engine_memory_total_bytes` | Gauge | Engine host total memory |
| `banyan_engine_disk_used_bytes` | Gauge | Engine host disk usage |
| `banyan_engine_disk_total_bytes` | Gauge | Engine host total disk |

**Cluster**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `banyan_cluster_agents_total` | Gauge | | Total registered agents |
| `banyan_cluster_agents_connected` | Gauge | | Currently connected agents |
| `banyan_cluster_deployments_total` | Gauge | `status` | Deployments by status (running, stopped, etc.) |
| `banyan_cluster_containers_total` | Gauge | | Total containers across all agents |
| `banyan_cluster_containers_healthy` | Gauge | | Currently healthy containers |
| `banyan_cluster_tasks_total` | Gauge | `status` | Tasks by status |

**Per-agent** (labeled by `agent`)

| Metric | Type | Description |
|--------|------|-------------|
| `banyan_agent_cpu_usage_ratio` | Gauge | Agent CPU usage (0.0–1.0) |
| `banyan_agent_memory_used_bytes` | Gauge | Agent memory in use |
| `banyan_agent_memory_total_bytes` | Gauge | Agent total memory |
| `banyan_agent_disk_used_bytes` | Gauge | Agent disk usage |
| `banyan_agent_disk_total_bytes` | Gauge | Agent total disk |
| `banyan_agent_containers_total` | Gauge | Containers on this agent |
| `banyan_agent_info` | Gauge | Agent metadata (status, subnet) — value is always 1 |

**Per-deployment** (labeled by `deployment`)

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `banyan_deployment_replicas_desired` | Gauge | `service` | Desired replica count per service |
| `banyan_deployment_replicas_healthy` | Gauge | `service` | Healthy replica count per service |
| `banyan_deployment_info` | Gauge | `status`, `strategy` | Deployment metadata — value is always 1 |

**Events**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `banyan_events_total` | Counter | `type` | Cumulative event count by type |

### Example queries

Check if all agents are connected:

```promql
banyan_cluster_agents_connected == banyan_cluster_agents_total
```

Find unhealthy deployments:

```promql
banyan_deployment_replicas_healthy < banyan_deployment_replicas_desired
```

Memory usage per agent as a percentage:

```promql
banyan_agent_memory_used_bytes / banyan_agent_memory_total_bytes * 100
```

Alert when an agent goes down:

```yaml
# Example Prometheus alert rule
groups:
  - name: banyan
    rules:
      - alert: BanyanAgentDown
        expr: banyan_cluster_agents_connected < banyan_cluster_agents_total
        for: 1m
        labels:
          severity: warning
        annotations:
          summary: "Banyan agent disconnected"
```

### Custom metrics port

The engine serves metrics on port `9090` by default. To change it, set the `metrics_port` field in your engine config at `/etc/banyan/banyan.yaml`:

```yaml
engine:
  metrics_port: "8080"
```

---

## What's next

- [CLI Reference — dashboard](/reference/cli/#dashboard) for all flags and keyboard shortcuts
- [Redeployment](/guides/redeployment/) to update running apps with zero downtime
- [Roadmap](/roadmap/) for upcoming features like per-container metrics and resource-aware scheduling
