# Banyan Technical Architecture — White Paper Research

Deep technical analysis of Banyan's design, focusing on how complexity is hidden from the user.

---

## 1. The Three-Concept Model

Banyan's entire architecture rests on exactly three concepts: **Engine**, **Agent**, and **Manifest**. Every feature, every design decision, flows through these three primitives. There are no CRDs, no operators, no sidecars, no service meshes.

### Manifest

A `BanyanManifest` is structurally identical to a Docker Compose file. It contains a map of `services`, where each service specifies `image`, `ports`, `environment`, `command`, `depends_on`, and an optional `deploy` block with `replicas` and `placement` constraints. The YAML unmarshaling supports Docker Compose's dual forms (e.g., `build: ./path` as a string or `build: {context: ./path}` as an object). Default replica count is 1 — zero configuration needed for the common case.

### Engine (Control Plane)

A single process containing:
- Embedded etcd-backed state store
- Embedded OCI container registry
- gRPC server
- 3-second orchestration loop

The orchestration loop processes deployments through a state machine: `pending` → `deploying` → `running` (or `failed`), and `stopping` → `stopped`.

### Agent (Data Plane)

Each server runs one agent that connects to the engine via gRPC, registers itself, then enters three concurrent loops:
- Task polling (2-second interval)
- Heartbeat (15-second interval)
- Container health monitoring (10-second interval)

The agent executes container operations via `nerdctl` (the containerd CLI), avoiding the Docker daemon dependency.

---

## 2. Communication Model

### Engine Service — Agent-Facing RPCs

- `Register` — Agent joins the cluster, receives registry URL, VPC subnet allocation, and list of containers it should already be running (for restart recovery)
- `Heartbeat` — Agent sends system metrics (CPU, memory, disk), receives VPC peer list and service backends for load balancing
- `PollTasks` — Agent polls for pending tasks assigned to it
- `ReportTaskResult` — Agent reports task completion/failure
- `ReportContainerHealth` — Agent reports container status and IP addresses

### Engine Service — CLI-Facing RPCs

- `Deploy` — Submit a manifest, get back a deployment ID
- `Down` — Stop a deployment (all or specific services)
- `GetStatus` — Full cluster status (agents, deployments, tasks)
- `GetLogs` — Stream container logs (proxied through engine to the correct agent)
- `GetDashboardData` — Comprehensive cluster snapshot for the CLI dashboard

### Agent Service

A single RPC — `StreamLogs` — hosted on each agent. The engine acts as a proxy: CLI requests logs → engine finds which agent hosts the container → opens a gRPC stream to that agent's log provider.

### Key Design Insight: Pull-Based Architecture

Agents are **pull-based**: they poll the engine for tasks rather than receiving push-based commands. This makes the system resilient to network partitions — agents simply resume polling when connectivity returns. No message queue, no callback registry, no webhook infrastructure.

---

## 3. State Management

### StateStore Interface

Four operations: `Save`, `Get`, `Delete`, `List`. Primary backend is etcd, with an in-memory implementation for testing.

### Key Schema

- `/banyan/deployments/<id>` — DeploymentRecord (services, status, update strategy)
- `/banyan/nodes/<name>` — NodeRecord (status, API address, tags, last-seen timestamp)
- `/banyan/tasks/<agent-name>/<task-id>` — TaskRecord (container details, execution status, health)

### Namespace-Scoped Task Assignment

Tasks for agent "worker-1" live under `/banyan/tasks/worker-1/`. When the agent polls, it only reads its own prefix, making task dispatch O(1) per agent regardless of cluster size.

---

## 4. Scheduling

The scheduling algorithm is deliberately simple: **round-robin across available agents**. For services with placement constraints (`deploy.placement.node`), agents are filtered by glob pattern before distribution.

Agent availability: must have `status: "ready"` and tags must intersect with the deployment's tags. If both have no tags, they match.

This avoids the complexity of bin-packing or constraint solvers. The roadmap includes health-based scheduling (CPU/memory-aware) as a future milestone.

---

## 5. Blue-Green Deployment — Zero Downtime Without Complexity

Banyan's default update strategy is blue-green. The user just runs `banyan up` again.

### The Flow

1. **User runs `up`**: Engine scans for existing deployments with same name and tags. Non-running deployments (pending/deploying/failed) are torn down immediately. The most recent running deployment's ID becomes `ReplacesID`.

2. **New deployment created**: Saved with `UpdateStrategy: "blue-green"` and `ReplacesID` pointing to the old deployment.

3. **Container naming**: When `ReplacesID` is set, container names use the deployment ID as prefix (e.g., `myapp-1735689600-web-0`) instead of the app name (`myapp-web-0`). This avoids name conflicts while both old and new containers run simultaneously.

4. **TCP proxy eliminates port conflicts**: The agent's iptables-based proxy handles port mapping at the kernel level (DNAT rules). Containers never bind host ports directly. Old and new containers can both exist because the proxy manages the mapping.

5. **Health confirmation**: Engine waits until all `create_and_start` tasks are completed. When new deployment reaches `StatusRunning`, old deployment teardown begins.

6. **Graceful teardown**: Old deployment's containers receive `stop_and_remove` tasks. Services that exist in the old deployment but NOT in the new (selective deploy) are "adopted" — their task records are reassigned to the new deployment ID so they keep running.

7. **Failure safety**: If the new deployment fails, the old one stays running. No automatic rollback — the user retries manually. Explicit design choice: a half-broken rollback is worse than a known-good old deployment plus a known-bad new one.

### What the User Experiences

They run the same command again. No strategy flags, no deployment objects, no rollout configuration. The system handles blue-green internally.

---

## 6. Overlay Network — The Most Hidden Complexity

The VPC networking subsystem is where Banyan hides the most complexity. Containers across different hosts communicate as if on a flat network, with no user configuration required.

### OverlayDriver Interface

Abstracts the data plane behind four methods: `Init`, `ReconcilePeers`, `WriteCNIConfig`, `Cleanup`. Two implementations:

**WireGuard Driver (default):**
- Creates `banyan-wg` interface on port 51820/UDP
- Encrypted L3 tunneling using kernel WireGuard
- Each agent has a keypair
- Peer configuration includes public key, endpoint, and allowed IPs (the peer's /24 subnet)

**VXLAN Driver (fallback):**
- Creates `banyan.1` VXLAN interface (VNI 1, port 8472/UDP)
- L2 overlay using Linux kernel VXLAN support
- Requires explicit FDB and ARP entries for each peer

Both drivers share the same bridge (`banyan0`) and CNI configuration. Containers connect to the bridge via standard CNI bridge plugin with host-local IPAM.

### Subnet Allocation

Engine runs a `SubnetAllocator` that carves /24 subnets from the VPC CIDR (e.g., `10.0.0.0/16`). Each agent gets its own /24 (e.g., `10.0.45.0/24`), providing 254 container IPs per agent. Allocation is idempotent — re-registering returns the same subnet.

### Peer Distribution via Heartbeat

**Key design decision**: Instead of a separate gossip protocol or consul-like service mesh, Banyan piggybacks peer information on the existing heartbeat RPC. The `PeerTracker` on the engine records each agent's subnet, host IP, and VTEP MAC (or WireGuard public key). On every heartbeat response, the engine returns all peers excluding the requesting agent. The agent then calls `ReconcilePeers()` to update routing tables.

Convergence time for a new peer: approximately **15 seconds** (one heartbeat interval). All operations are idempotent.

### Deterministic MAC Addresses (VXLAN Mode)

MAC addresses follow the formula: `02:42:<ip[0]>:<ip[1]>:<ip[2]>:01` — derived from the subnet IP. This eliminates the need for MAC address exchange between agents. The `02` prefix marks it as a locally-administered unicast address.

### What the User Experiences

Nothing. They don't configure a CNI, don't choose an overlay driver, don't allocate subnets, don't manage peers. Containers across hosts can talk to each other. That's it.

---

## 7. Service Discovery — DNS Without Configuration

### Architecture

Each agent runs a DNS server (miekg/dns library) bound to its bridge gateway IP (e.g., `10.0.45.1:53`). Two types of queries:

1. **Internal queries** (`.internal` domain): Resolved from in-memory store mapping hostnames to IPs
2. **External queries**: Forwarded to upstream DNS (`8.8.8.8:53`)

### Registration Flow

Two sources for DNS entries:

1. **Immediate local registration**: After container creation, agent immediately registers `<service-name>.internal` → `<container-IP>`. Instant resolution for containers on the same host.

2. **Cluster-wide distribution via heartbeat**: Engine gathers all running container backends. On each heartbeat response, all backends are sent to every agent. Agent's `reconcileDNS()` rebuilds all DNS entries, removing stale hostnames and adding new ones.

### Container DNS Configuration

When VPC is enabled, nerdctl run arguments include `--dns <gateway-ip> --dns-search internal`. This makes the agent's DNS server the container's resolver and adds `internal` as a search domain.

Result: `dig web` inside a container resolves to `web.internal`, which returns all container IPs for that service.

TTL is 60 seconds. Health-aware resolution only returns entries marked `healthy`.

### What the User Experiences

They write `db` in their application code. It resolves. No service mesh, no Consul, no CoreDNS configuration.

---

## 8. Cross-Host Load Balancing

### The iptables Proxy Model

Follows the kube-proxy iptables model — the agent writes iptables rules but never touches actual traffic. The Linux kernel handles all packet forwarding.

Chain structure:
```
PREROUTING / OUTPUT → BANYAN-P-SERVICES → BANYAN-P-SVC-<port> → DNAT rules
FORWARD → BANYAN-P-FWD → per-backend ACCEPT rules
POSTROUTING → BANYAN-P-POSTROUTING → MASQUERADE for DNAT'd traffic
```

For each host port, probability-based DNAT rules achieve uniform random distribution. With N backends, the i-th rule has probability `1.0 / (N - i)`. This is identical to kube-proxy's iptables mode.

### Cross-Host Data Flow

1. Agent creates container, gets its IP via `nerdctl inspect`
2. Agent reports IP in `ReportContainerHealth` to engine
3. Engine stores IP in `TaskRecord.ContainerIP`
4. Engine's `collectServiceBackends()` gathers all running backends
5. Engine returns all backends in `HeartbeatResponse.service_backends`
6. Agent's `reconcileRemoteBackends()` adds/removes iptables DNAT rules for remote containers

Convergence: local = immediate, remote = ~25 seconds (10s health + 15s heartbeat).

### What the User Experiences

They set `replicas: 3` and traffic distributes across all replicas regardless of which host they're on.

---

## 9. WireGuard Control Tunnel — Security by Default

A separate WireGuard interface (`wg-control`, port 51821/UDP) encrypts all gRPC traffic between engine, agents, and CLI. Distinct from the data plane overlay (`banyan-wg`, port 51820/UDP).

### IP Assignment

Engine: `10.200.0.1`. Agents and CLI: deterministic IPs from `TunnelIPFromPublicKey()` — SHA-256 hash of the base64 public key, using first two bytes as octets in `10.200.x.y`.

### Authentication Model

Two layers:

1. **Public key authentication**: Agent/CLI attaches WireGuard public key as gRPC metadata (`x-banyan-public-key`). Engine validates against whitelist. Health RPCs exempt.

2. **Session token authentication**: For engine-to-agent calls (e.g., log streaming). Agent generates random 32-byte token at startup, passes to engine during Register. Validated using constant-time comparison.

**Fallback**: If WireGuard not available, gRPC falls back to direct TCP with pubkey metadata auth.

### What the User Experiences

They paste a public key during `init`. All communication is encrypted. No certificates to manage, no CA to operate, no TLS termination to configure.

---

## 10. Resilience and Recovery

### Agent Reconnection

After 3 consecutive heartbeat failures, agent enters reconnection mode with exponential backoff (2s initial, 60s max). Waits for engine's gRPC health check, then re-registers. Engine returns active containers during registration so agent can restore proxy rules and container tracking.

### Container Tracking Across Restarts

Register response includes all containers the agent should be running. Agent verifies each container is actually running via `nerdctl inspect` before restoring proxy rules. Handles the case where agent restarted but containers kept running (they run as containerd processes, independent of the agent).

### Bridge Preservation

Both VXLAN and WireGuard drivers preserve the bridge during cleanup and only delete/recreate the tunnel interface. Running containers have veth pairs attached to the bridge — deleting it would break their networking.

### DNS Port Reclaim

If a previous agent process died without cleanup, DNS port may be held by a stale process. Agent detects `address already in use`, finds owning PID via `/proc/net/udp`, kills it, and retries.

---

## 11. Metrics and Observability

### Prometheus Metrics

Engine runs a Prometheus HTTP server (default port 9090). Metrics include:
- Engine: uptime, goroutines, memory usage, RPC counts, event store size, VPC status
- Cluster: total agents, connected agents, total/healthy/unhealthy containers, tasks by status
- Per-agent: CPU, memory, disk, containers, reported status
- Per-deployment: replicas total/healthy/unhealthy, status
- Events: total event count

All use `banyan_` prefix.

### Event System

Records lifecycle events (agent registration, deployment creation/failure, etc.) using either:
- In-memory ring buffer
- WAL-backed event store (newline-delimited JSON, auto-compacts at 2x max events)

### Terminal Dashboard

CLI-based TUI (Bubbletea framework) with 6 views: Overview, Agents, Deploys, Containers, Engine, Events. Updates every 5 seconds. Keyboard navigation (vim keys), filtering, CSV export, command palette.

No browser, no Grafana, no configuration files.

---

## 12. Complexity Budget — What the User Sees vs. What Happens

| What the User Does | What Actually Happens |
|---|---|
| Writes Docker Compose YAML | Manifest parsed, validated, tasks scheduled round-robin across agents |
| Runs `banyan up` | Blue-green: new containers start alongside old, proxy rules updated, health confirmed, old torn down |
| Service resolves `db` | DNS search domain `.internal` appended → agent-local DNS server → in-memory store populated by heartbeat |
| Traffic crosses hosts | Packet routes through bridge → WireGuard/VXLAN tunnel → encrypted → remote bridge → destination container |
| Load balancing | iptables DNAT with probability-based rules (kube-proxy model), kernel handles packet forwarding |
| Adds a new server | Install, init, whitelist key, start. Manifest doesn't change. |
| Scales to 5 replicas | Change `replicas: 5` in manifest, run `up`. Distributed automatically. |
| Checks cluster status | `banyan-cli dashboard` — live TUI, no Grafana setup |
| Views logs | `banyan-cli logs` — engine proxies to correct agent, streams in real-time |

---

## 13. The Fundamental Architectural Insight

**Convergence loops replace synchronous orchestration.** The engine's 3-second deployment loop, the agent's 2-second task poll, the 15-second heartbeat, and the 10-second health check all operate independently. There is no distributed transaction, no two-phase commit, no leader election.

State converges through repeated polling and idempotent operations. This makes the system remarkably resilient to partial failures — any component can crash and restart, and state will converge within seconds.

This is the same model that makes DNS resilient (eventual consistency through TTL-based refresh) and the same model that makes BGP resilient (periodic route advertisements). The trade-off is convergence delay (seconds, not milliseconds), which is acceptable for the target use case of multi-server deployment rather than high-frequency trading.

---

## 14. Current Limitations (For Honest Assessment Section)

- **No volume support** — persistent storage not yet implemented
- **No autoscaling** — manual replica management only
- **Single engine is SPOF** — no multi-engine HA yet (on roadmap)
- **No RBAC/ABAC** — public key whitelist is the only access control
- **No secrets management** — environment variables in plaintext in manifest
- **No health-based scheduling** — round-robin only, no CPU/memory awareness
- **Not yet production-ready** — stated explicitly on the website
- **No L7 ingress** — iptables-based L4 proxy only
- **No session affinity** — random distribution only
- **No network policies** — all containers in the VPC can reach all others
