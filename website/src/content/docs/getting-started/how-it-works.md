---
title: How Banyan Works
description: The mental model behind Banyan — three concepts that explain everything.
sidebar:
  order: 0.5
---

> **Audience note:** This page is philosophy-first. If you skip it and go straight to the [quickstart](/getting-started/quickstart/), Banyan will feel like a collection of commands. With these mental models, every command and behavior you encounter will make intuitive sense.

---

## 1. The Gap Between One Server and Many

Docker Compose is a process supervisor with a YAML syntax. It works beautifully on one machine — but that's exactly where its contract ends.

The moment you need a second server, you face problems Compose was never designed to solve:

| Problem | What it means |
|---------|--------------|
| **Where does this run?** | You have 3 services and 2 servers. Who decides what goes where? |
| **What if a server dies?** | Containers on it are gone. Who restarts them somewhere else? |
| **How do services find each other?** | Container IPs change across servers. How does your web app always find the database? |
| **How do I update without downtime?** | You want v2 to replace v1 while traffic keeps flowing. |
| **How do secrets get to containers?** | Database passwords shouldn't live in your manifest or source control. |

Kubernetes solves all of these. But it does so with an architecture designed for platform teams managing hundreds of services across thousands of nodes. If you're 5 engineers running 10 services on 3 servers, most of that machinery is overhead.

Banyan solves the same problems with a smaller, opinionated design built for that scenario.

---

## 2. Three Concepts: Engine, Agent, Manifest

Everything in Banyan is one of three things. This is the whole mental model.

```
                    ┌─────────────────────────────┐
 You ──────────────▶│          Engine              │
  (manifest)        │   schedules, stores state,   │
                    │   monitors health             │
                    └──────────┬──────────┬─────────┘
                               │          │
                          ┌────▼───┐ ┌────▼───┐
                          │ Agent  │ │ Agent  │
                          │ web-1  │ │ web-2  │
                          │ ┌───┐  │ │ ┌───┐  │
                          │ │ C │  │ │ │ C │  │
                          │ └───┘  │ │ └───┘  │
                          └────────┘ └────────┘
```

**Engine** — the brain. One process (or several, for high availability) that holds the desired state of your cluster. It decides which containers run where, tracks their health, and repairs drift. It never runs your application containers.

**Agent** — the hands. One per server. Each agent polls the engine for work, starts and stops containers using containerd, and reports back what's running. Agents don't make decisions — they execute.

**Manifest** — the contract. A YAML file that describes what you want running. Same `services:` block as Docker Compose, plus `deploy.replicas` when you want more than one copy. The manifest is the desired state. You give it to the engine. Banyan makes it real.

That's it. When you run `banyan-cli up -f banyan.yaml`, you're handing the engine a manifest. The engine turns it into tasks, assigns them to agents, and monitors the result. When things break, the engine fixes them — the same way you would, but every 10 seconds instead of whenever you notice.

---

## 3. Desired State, Not Commands

This is the single most important idea in Banyan. Everything else follows from it.

You do not tell Banyan "start 3 nginx containers." You tell it "the desired state is: 3 nginx replicas exist." Banyan then continuously compares that against what's actually running, and acts to close the gap.

```
Desired State (your manifest, stored in the engine)
        │
        │  "3 replicas of web should be running"
        ▼
  ┌──────────────┐
  │ Reconciler   │  ◄── observes actual state every 10s
  └──────────────┘
        │
        │  observes: "only 2 replicas running — one crashed"
        │  action:   "restart the crashed one"
        ▼
  Actual State (what agents report)
```

This loop — **observe, diff, act** — runs continuously. It is called the **reconciliation loop**.

**Why this matters:**

- **Self-healing.** A container crashes? The reconciler notices the gap between desired (3 replicas) and actual (2 replicas), and creates a new one. You don't intervene.
- **Agent dies? Same loop.** The engine notices the agent stopped sending heartbeats, waits a grace period (in case it's rebooting), then reschedules its containers to other agents. Your manifest didn't change. The engine converges back to it.
- **Engine restarts? Still the same loop.** On startup, the engine runs a full reconciliation pass — compares every "running" deployment against what agents report, and repairs any drift it finds.
- **You can always re-deploy.** If something looks wrong, run `banyan-cli up -f banyan.yaml` again. The engine diffs desired against actual and only changes what needs changing.

**The corollary:** Banyan doesn't guarantee anything happens right now. It guarantees **eventual convergence** toward your desired state, within seconds for container crashes, within minutes for server failures.

---

## 4. How the Engine Tracks State

The engine stores all cluster state in etcd, an embedded key-value store that runs as part of the engine process. (For high availability, you run an external etcd cluster — but the default single-engine setup manages it for you.)

```
What lives in the engine's store         What does NOT
─────────────────────────────────        ──────────────────────
Deployment records (desired state)       Container images
Task records (assigned work)             Application logs
Agent records (who's connected)          Your source code
Secrets (encrypted at rest)              Container filesystem
Cluster events                           Runtime metrics history
```

Every component reads and writes state through the engine's gRPC API. Agents don't access etcd directly. The CLI doesn't access etcd directly. The engine is the only door to the data.

This means:
- **The engine is the single source of truth.** If the engine says 3 replicas should be running, that's the truth.
- **Agents are stateless.** If an agent crashes and restarts, it registers with the engine, gets its task list, and picks up where it left off. No local state to lose.
- **The CLI is a thin client.** It sends commands to the engine and displays results. It doesn't store cluster state.

---

## 5. How a Deployment Flows

When you run `banyan-cli up -f banyan.yaml`, here's what happens step by step:

```
1. CLI reads banyan.yaml
   │
2. CLI sends manifest to Engine via gRPC
   │
3. Engine creates a DeploymentRecord (desired state)
   │
4. Engine creates TaskRecords — one per container
   │  Assigns each to an agent based on resource availability
   │
5. Agents poll for new tasks
   │  Each agent pulls its assigned tasks from the engine
   │
6. Agents start containers via containerd
   │  Container images pulled from Banyan's built-in registry
   │
7. Agents report container status back to engine
   │  Every 10 seconds: running, IPs, CPU, memory
   │
8. Engine marks deployment as "running"
   │  All tasks completed successfully
   │
9. Reconciler takes over
   Monitors continuously. If anything drifts from desired
   state — container crash, agent failure — it acts.
```

Steps 1-8 happen once, during deployment. Step 9 runs forever.

---

## 6. Failure Recovery — How the Reconciler Thinks

The reconciler isn't one big function. It's three focused controllers that run in sequence every 10 seconds:

**AgentReconciler** runs first. It checks: is every agent still sending heartbeats? If an agent has been silent for longer than the grace period (2 minutes for recently-added agents, 5 minutes for long-running ones), it marks that agent's containers as gone and reschedules them to healthy agents.

**ContainerReconciler** runs second. For every running deployment, it compares desired replicas against actual running containers. If a container crashed on a healthy agent, it restarts it — respecting the service's `restart:` policy:

| Policy | Behavior |
|--------|----------|
| `always` (default) | Restart on any exit, with exponential backoff |
| `on-failure` | Restart only on non-zero exit code |
| `on-failure:3` | Restart on failure, but give up after 3 attempts |
| `unless-stopped` | Restart unless you explicitly ran `banyan-cli down` |
| `no` | Never restart. The container exited, leave it. |

**DeploymentReconciler** runs third. It looks at the results of the first two and computes a health status for each deployment: `healthy`, `recovering`, `degraded`, or `stopped`. This status is what you see in `banyan-cli status` and the web dashboard.

The order matters. Agent failures are detected first, then container failures on healthy agents, then the overall health picture is updated. Each controller sees the results of the one before it.

---

## 7. Networking — How Containers Find Each Other

When two containers need to talk, even across different servers, Banyan handles the networking automatically.

Each agent gets a subnet (like `10.0.1.0/24`). Containers on that agent get IPs from that subnet. Traffic between agents flows through an encrypted WireGuard tunnel — no extra configuration.

```
Agent 1 (10.0.1.0/24)          Agent 2 (10.0.2.0/24)
┌─────────────────────┐        ┌─────────────────────┐
│ web (10.0.1.2)      │        │ api (10.0.2.2)      │
│       │              │        │       ▲              │
│       │   bridge     │        │       │   bridge     │
│       └──banyan0─────┼──WG────┼───────banyan0────────│
└─────────────────────┘        └─────────────────────┘
```

**Service DNS**: Containers can reach each other by service name. If your manifest has a service called `db`, any other container can connect to `db` (or `db.my-app.internal`). The agent runs a local DNS server that resolves service names to container IPs.

**Load balancing**: If `web` has 3 replicas spread across 2 agents, traffic to `web` is distributed across all 3 replicas automatically using iptables DNAT rules. Every agent knows about every backend — traffic goes directly to the right container, regardless of which agent it's on.

---

## 8. What Banyan Is Not

Equally important is knowing where Banyan's scope ends:

| Assumption | Reality |
|------------|---------|
| "Banyan manages my application" | Banyan manages containers. It doesn't know what your app does, whether its responses are correct, or whether your business logic works. |
| "Banyan replaces my CI/CD" | Banyan runs containers. Building images and pushing them to the registry is your job (or your CI's). |
| "Banyan handles L7 routing" | Banyan provides L4 (TCP) load balancing. HTTP path-based routing, SSL termination, and request-level features need a reverse proxy (Caddy or nginx) running as a Banyan service. |
| "Banyan encrypts my secrets end-to-end" | Secrets are encrypted at rest in etcd and transmitted over WireGuard (encrypted). But inside the container, they're plain environment variables — same as Docker, Kubernetes, and every other orchestrator. |
| "Banyan auto-scales based on traffic" | Banyan auto-scales based on CPU metrics. Traffic-based scaling (requests per second) requires an external metrics source. |

---

## Summary

Before using Banyan, internalize these five ideas:

1. **Three concepts.** Engine (brain), agent (hands), manifest (contract). Everything else is an implementation detail.

2. **Desired state, not commands.** You declare what should be running. Banyan continuously converges toward it. Every 10 seconds, it checks: does reality match the manifest?

3. **The engine is the single source of truth.** All state lives in the engine's store. Agents are stateless executors. The CLI is a thin client. If you lose an agent, you lose nothing permanent.

4. **Failure recovery is automatic.** Container crashes get restarted. Dead agents get their work rescheduled. Engine restarts trigger a full reconciliation pass. Your manifest is the anchor — Banyan always drifts back to it.

5. **Networking is built in.** Containers find each other by service name across servers. Traffic is encrypted between agents. Load balancing happens automatically. You don't configure any of this — it works when you deploy.

---

**Next: [Installation](/getting-started/installation/)** — Get Banyan on your servers.
