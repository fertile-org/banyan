# Docker Swarm Deep Research (2025-2026)

Deep, unbiased analysis for Banyan white paper. Docker Swarm also claims simplicity — how does it compare?

---

## 1. Current State (2025-2026)

### Is Docker Swarm Still Actively Developed?

**Yes, but with important caveats.** Docker Swarm Mode (SwarmKit) is built directly into the Docker Engine — it is NOT a separate product. When you install Docker, you already have Swarm.

- Docker Engine v28.x (2025) includes Swarm-specific fixes: fixing potential resource leaks when a node leaves a Swarm, fixing Swarm ingress caused by incorrect iptables ordering, fixing `volume.subpath` being ignored in Swarm mode
- SwarmKit lives at `github.com/moby/swarmkit` as part of the Moby project

**What happened to Classic Swarm vs Swarm Mode:**
- **Docker Swarm Classic** (2014): Standalone clustering tool requiring external service discovery (Consul, etcd, Zookeeper). **Archived** at `docker-archive/classicswarm`. Dead.
- **SwarmKit** (2016): Complete rewrite, released as open source
- **Docker Swarm Mode** (Docker 1.12+, 2016): SwarmKit integrated into Docker Engine. This is what "Docker Swarm" means today. No external dependencies.

### Mirantis Commitment

Mirantis acquired Docker Enterprise in November 2019. Their commitment:

- **July 2025**: Publicly committed to long-term Swarm support **through at least 2030** on Mirantis Kubernetes Engine 3 (MKE 3)
- **Active feature development** (not just maintenance):
  - Added API support for **Seccomp security profiles**
  - Added API support for **AppArmor security profiles**
  - Added **Container Storage Interface (CSI)** support (going through upstream validation)
  - Added **image pruning** for automated node maintenance
  - Added **resource constraints** (mandatory CPU/memory requests)
  - Added **Mirantis Secure Registry (MSR)** deployment on Swarm clusters
- **Roadmap**: PingFederate SSO, OSTree Linux, IPv6, ARM support

**However**: Mirantis also released MKE 4, which is **Kubernetes-only**. This signals that while Swarm will be maintained on MKE 3, the strategic direction for Mirantis's next-gen platform is Kubernetes.

### Who Is Still Using It?

- **100+ enterprise customers** running Swarm in production (Mirantis data)
- ~10,000 nodes across ~1,000 clusters, 100,000+ containers
- **Named customers**: GlaxoSmithKline, MetLife, Royal Bank of Canada, S&P Global
- **Verticals**: Manufacturing, financial services, energy, defense
- **Market share**: ~2.5-5% of container orchestration (K8s holds ~92%)
- **Job market**: 1,200+ K8s job postings vs only 176 Swarm-related positions in Q1 2025
- Docker Compose/Swarm usage among PHP developers rose from 17% (2024) to 24% (2025)

---

## 2. The Simplicity Claim — How Does It Work?

### Setup: Genuinely Simple

**Manager node (1 command):**
```bash
docker swarm init --advertise-addr <MANAGER_IP>
```

This single command:
- Initializes the Raft consensus store
- Generates a self-signed root CA
- Issues TLS certificates for the manager node
- Creates worker/manager join tokens
- Creates the `ingress` overlay network for routing mesh
- Creates the `docker_gwbridge` network for outbound traffic

**Worker node (1 command):**
```bash
docker swarm join --token <TOKEN> <MANAGER_IP>:2377
```

That's the complete cluster setup. Two commands. No config files, no YAML, no external dependencies.

**For comparison**: A minimal K8s cluster requires installing container runtime, kubelet, kubeadm, kubectl, running `kubeadm init` with config, applying a CNI addon, joining workers — typically 15-30 commands per node.

### Docker Compose → Swarm Stack Deployment

```bash
docker stack deploy -c docker-compose.yml myapp
```

Takes a Docker Compose file and deploys it as a "stack" to the Swarm. Services distributed across nodes, overlay networks created, routing mesh handles load balancing.

### Docker Compose Compatibility — The Cracks

`docker stack deploy` uses the **legacy Compose file version 3 format**, NOT the modern Compose specification. This is a critical limitation.

**Supported in Swarm:**
- `image` (required — no building on deploy)
- `ports` (published via routing mesh or host mode)
- `networks` (overlay, created automatically)
- `volumes` (named volumes only, with significant limitations)
- `environment` / `env_file`
- `deploy:` section (replicas, placement, resources, update_config, rollback_config, restart_policy)
- `secrets` and `configs` (Swarm-native features)
- `healthcheck`

**Ignored/Unsupported in Swarm:**
- `build` — completely ignored; must pre-build and push images
- `depends_on` — ignored; services restart on failure instead of ordering
- `container_name` — ignored; auto-generated for scaling
- `restart` — ignored; replaced by `deploy.restart_policy`
- `network_mode` — ignored
- `privileged` — produces "Ignoring unsupported options: privileged"
- `links`, `external_links` — ignored
- `.env` file variable substitution — not supported
- `devices`, `cap_add`/`cap_drop` — not supported
- `dns_search`, `domainname`, `security_opt`, `tmpfs` — not supported
- Local-scoped networks (macvlan, etc.) — not supported

**Critical gap**: The latest Compose specification format is **not compatible** with `docker stack deploy`. Teams using modern `docker compose` (v2) features will find their compose files don't work with Swarm without modification. The "just use your existing Compose file" marketing doesn't match reality.

### Overlay Networking

Swarm uses VXLAN for overlay networking:
- Linux bridge created per overlay network, with VXLAN interfaces
- MAC-in-UDP encapsulation (L2 frames inside underlay IP/UDP header)
- 24-bit VNI (Virtual Network Identifier)
- Only instantiated on hosts when a container using that network is scheduled there

**Encryption**: Optional via `--opt encrypted`. Uses IPsec AES-GCM at VXLAN level. **Significant performance penalty** — GitHub issue #33133 reported 99% throughput loss (extreme case from 2017). Expected overhead was designed to be ~10%.

**Required ports**: TCP/UDP 7946 (container network discovery), UDP 4789 (container ingress).

### Service Discovery

Built-in DNS:
- Every service gets a DNS entry automatically
- Containers query embedded Docker DNS resolver
- Two modes:
  - **VIP mode (default)**: Each service gets a virtual IP. DNS resolves to VIP, IPVS load-balances to containers
  - **DNSRR mode**: DNS returns list of all container IPs. Client picks one (round-robin at DNS level)

### Load Balancing (Routing Mesh)

Cluster-wide L4 load balancer:
- Uses Linux IPVS and iptables
- **Any node** accepts connections on published ports, even if no container for that service runs there
- IPVS round-robins requests across all healthy replicas
- The `ingress` overlay network carries routed traffic between nodes
- **No L7 load balancing** built in — need Traefik/Nginx/HAProxy for path-based routing, TLS termination

### Rolling Updates

Complete rolling update mechanism:
```bash
docker service update --image myapp:v2 myservice
```

Configuration:
- `--update-parallelism N` (default: 1) — tasks updated at once
- `--update-delay` (default: 0s) — delay between batches
- `--update-failure-action` (continue | pause | rollback)
- `--update-order` (stop-first | start-first)
- `--update-max-failure-ratio` — failure rate tolerance
- `--rollback-parallelism` — separate parallelism for rollbacks
- **Automatic rollback** if failure rate exceeds threshold

### Learning Curve from Docker Compose

**Genuinely low.** A Swarm deployment file is a Docker Compose file with an added `deploy:` section. Quantified: a Node.js + MongoDB stack in Swarm requires 42 lines in one file with 5 new concepts. Same stack in K8s requires 4-6 YAML files totaling 170+ lines with 25+ new concepts.

However, the curve steepens when teams hit unsupported features or need to debug overlay networking.

---

## 3. Where Swarm's Simplicity Breaks Down

### Overlay Network Issues (Most Frequently Reported)

- **Intermittent connectivity failures**: Containers on same overlay periodically can't reach each other (GitHub #32738 — long-standing)
- **Stale IPVS entries**: During service updates, old container IPs linger in IPVS table for 5 minutes, routing traffic to dead containers (GitHub #36878)
- **Conntrack issues under load**: IPVS conntrack sporadically marks valid packets as invalid, causing connection resets (GitHub #40374)
- **Overlay breaks after force-new-cluster**: Recovery operations can break networking entirely (GitHub #495)
- **Debugging is painful**: No built-in tools. Must deploy `netshoot` container in privileged mode and manually inspect iptables/network namespaces. Requires deep Linux networking expertise.
- **Encryption overhead**: IPsec causes up to 99% throughput loss in extreme cases

### Logging and Monitoring — No Native Solution

- `docker service logs` is CLI-only, per-service, no search
- No multi-service log aggregation
- No log retention after services removed
- No metrics collection, dashboards, or alerting
- Must deploy third-party stacks (ELK, Loki, Prometheus+Grafana) — undermines simplicity proposition

### Secret Management Limitations

- Secrets are **immutable** once created — cannot update or rotate without removing/recreating the service
- 500KB size limit
- Available only to Swarm services, not standalone containers
- No audit logging, no dynamic secrets, no versioning
- Production-grade needs → layer HashiCorp Vault on top

### Storage/Volume Handling — Arguably Biggest Weakness

- **Volumes do NOT share across nodes.** If container rescheduled to different node, gets new empty volume. Old data orphaned.
- No native replicated or multi-host volumes
- Must bolt on external shared storage (NFS, GlusterFS, Amazon EFS) or use volume plugins
- Mirantis addressing with CSI support (still going through validation)

### Health Check Limitations

- Can only execute commands inside container (no external probes)
- DNS blocked until healthcheck passes — chicken-and-egg for services needing other services to be healthy
- No readiness probes (only liveness)
- No startup probes for slow-starting containers
- Unhealthy containers simply killed and restarted — no fine-grained control

### Node Management at Scale

- Max recommended: **7 manager nodes** (Raft consensus overhead)
- Embedded Raft store becomes bottleneck as cluster grows
- Failed node returning doesn't rebalance workloads
- No cluster autoscaling
- Manager nodes sensitive to resource starvation

### Debugging — The Achilles Heel

- No dashboard or web UI built in (Portainer fills gap, but separate product)
- Network issues require deep Linux networking knowledge
- Raft consensus problems can leave cluster in broken state
- `--force-new-cluster` recovery can create more problems than it solves
- Log access is primitive compared to K8s ecosystem

---

## 4. Why Teams Move Away

### Real Migration Story: Single Music (CNCF Case Study)

Digital music distribution for major artists. Chose Swarm in 2017. Key problems:
- **Stateful workloads**: "While Swarm is great for scaling stateless applications, it's less than ideal for managed stateful applications." Needed to scale Redis and RabbitMQ.
- **Disaster recovery fear**: If management plane crashed, facing "being offline for eight hours doing a manual rebuild." 80-120 hours to automate.
- **Operational risk**: Handled releases from well-known artists, couldn't tolerate extended outages.

Migration to EKS took ~3 weeks with 30-minute cutover. Called it "probably the easiest platform migration."

### Common Migration Drivers

1. **Stateful workload support** — K8s StatefulSets, operators, CSI ecosystem
2. **Ecosystem and tooling** — more tools are K8s-first
3. **Advanced networking** — network policies, service mesh, multi-cluster
4. **Hiring and skills** — 1,200+ K8s jobs vs 176 Swarm positions
5. **Managed services** — every cloud offers managed K8s, none offer managed Swarm

### The "Dead Project" Perception — Is It Fair?

**No, but understandable:**
- Docker, Inc. pivoted away from Swarm orchestration
- Mirantis acquisition created uncertainty
- MKE 4 being K8s-only reinforced the narrative
- Overwhelming K8s marketing drowns out Swarm's existence
- Community sentiment reflexively dismissive

**Bret Fisher** (Docker Captain): "These aren't informed opinions — they're reflexes, with people who've never run Swarm in production confidently telling others to avoid it."

**Reality**: Maintained, receives security updates, committed backer through 2030, 100+ enterprise customers, 100,000+ containers. Not dead. But also not growing.

---

## 5. Docker Swarm vs Banyan — Fair Comparison

### Docker Compose Compatibility

| Aspect | Docker Swarm | Banyan |
|--------|-------------|--------|
| Compose format | Legacy v3 only; modern spec NOT compatible | Modern Compose specification |
| `build` directive | Ignored | Supported (built-in registry) |
| `depends_on` | Ignored | Respected for ordering |
| `container_name` | Ignored (auto-generated) | Supported |
| `restart` | Ignored (replaced by deploy.restart_policy) | Supported |
| `privileged` | Ignored with warning | Supported |
| `network_mode` | Ignored | Supported where applicable |
| Deploy command | `docker stack deploy -c compose.yml name` | `banyan up -f compose.yml` |

**Key difference**: Swarm's Compose compatibility is a subset — many fields silently ignored in production. Banyan treats Compose file as source of truth.

### Overlay Networking

| Aspect | Docker Swarm | Banyan |
|--------|-------------|--------|
| Technology | VXLAN (built into Docker Engine) | WireGuard (default) with VXLAN fallback |
| Encryption | Optional IPsec, severe performance penalty | WireGuard by default, minimal overhead |
| Network creation | `docker network create --driver overlay` | Automatic, engine-managed |
| Subnet allocation | Docker Engine managed | Engine-side SubnetAllocator (/24 per agent) |
| MAC addressing | Dynamically assigned | Deterministic formula |
| Peer discovery | Gossip protocol via Serf | Heartbeat-based (15s convergence) |

**Key difference**: Swarm uses VXLAN with optional (expensive) IPsec. Banyan uses WireGuard as default — encryption with minimal overhead. Banyan also has a separate WireGuard control tunnel for gRPC traffic.

### Service Discovery

| Aspect | Docker Swarm | Banyan |
|--------|-------------|--------|
| Implementation | Embedded in Docker Engine DNS | Agent-local DNS server (miekg/dns) |
| Modes | VIP (default) or DNSRR | Direct A-record resolution |
| Naming | `service_name` within overlay | `service_name.internal` or just `service_name` |
| Cross-host awareness | Built into overlay gossip | All agents aware via heartbeat |
| Update speed | Real-time via overlay gossip | Local: immediate; Remote: ~25s |

### Load Balancing

| Aspect | Docker Swarm | Banyan |
|--------|-------------|--------|
| Mechanism | IPVS routing mesh (L4) | iptables DNAT rules per agent |
| Routing mesh | Yes — any node accepts traffic on published ports | No — each agent handles its own ports |
| L7 support | None built-in | None built-in |
| Known issues | Stale IPVS entries, conntrack bugs | Newer implementation |

**Key difference**: Swarm's routing mesh is more sophisticated — any node accepts incoming traffic on published ports, even without the service. Genuinely useful for external LB integration. Banyan's approach is simpler but requires external LB per agent.

### Security Model

| Aspect | Docker Swarm | Banyan |
|--------|-------------|--------|
| Control plane | mTLS, auto-rotation every 90 days | WireGuard control tunnel |
| Data plane | Optional IPsec (high overhead) | WireGuard by default (low overhead) |
| Cert management | Automatic CA, auto-rotation | WireGuard keypair exchange at init |
| Node auth | TLS certificate with node ID | WireGuard public key whitelist |
| Secrets | Built-in docker secrets (tmpfs mount) | Not yet implemented |
| Security profiles | Seccomp + AppArmor (added 2025) | N/A |
| Key rotation | Gossip key rotated every 12 hours | WireGuard protocol handles |

**Key difference**: Swarm has a more complete security model (secrets, CA, security profiles). Banyan's WireGuard approach provides stronger encryption with better performance but lacks secret management.

### What Swarm Does That Banyan Doesn't (Yet)

1. **Routing mesh** — any node accepts traffic on published ports
2. **Secret management** — encrypted at rest, distributed via Raft, tmpfs mount
3. **Config objects** — non-sensitive config distributed to services
4. **Rolling updates with auto-rollback** — configurable parallelism, delay, failure thresholds
5. **Placement constraints and preferences** — `node.role`, `node.labels`, `engine.labels`
6. **Global services** — exactly one replica per node
7. **Manager node HA** — built-in Raft consensus across multiple managers
8. **Ecosystem** — Portainer web UI, Traefik deep integration

### What Banyan Does That Swarm Doesn't

1. **WireGuard-encrypted overlay by default** — no performance penalty
2. **Separate control plane encryption** — WireGuard control tunnel
3. **nerdctl/containerd native** — no Docker daemon dependency
4. **Modern Compose specification** — not locked to legacy v3
5. **Deterministic networking** — MACs and VTEP IPs derived from config
6. **Blue-green deployments** — deploy new, confirm healthy, teardown old
7. **Built-in terminal dashboard** — live TUI, no Portainer needed
8. **Built-in image registry** — no Docker Hub or private registry needed
9. **Three-concept model** — Engine + Agent + Manifest vs Swarm's 10+ concepts (manager, worker, service, task, stack, overlay, ingress, node, secret, config)

---

## Summary Assessment

### Docker Swarm's Position Is Honest

Not dead, not dying, not abandoned. A **stable, maintained, narrowly-scoped** orchestration tool that excels when teams know Docker Compose, need multi-node deployment, and don't need full K8s features. Mirantis's 2030 commitment and active feature development demonstrate real investment.

However, Swarm is not innovating at its former pace. The Compose compatibility gap (stuck on v3), missing observability, storage limitations, and overlay encryption performance issues are real problems that have persisted for years.

### Where Banyan Differentiates from Swarm

1. **Encryption without performance penalty**: WireGuard eliminates Swarm's forced choice between security and performance
2. **Modern Compose compatibility**: Current spec rather than legacy v3, eliminating "works locally, silently ignored in production"
3. **Container runtime independence**: containerd/nerdctl, not tied to Docker Engine's strategic decisions
4. **Built-in observability**: Terminal dashboard and Prometheus metrics — Swarm has neither
5. **Built-in image registry**: No external registry needed

### What Banyan Should Honestly Acknowledge

Swarm has a **significant head start** in:
- Secret management (Banyan has none)
- Routing mesh (Banyan requires external LB per agent)
- Rolling updates with auto-rollback (Banyan has blue-green, less configurable)
- Manager HA via Raft (Banyan engine is SPOF)
- Ecosystem maturity (Portainer, Traefik, years of battle-testing)
- 100+ enterprise customers in production

Any fair comparison must acknowledge these gaps. Dismissing Swarm as "dead" would undermine credibility with the exact audience Banyan targets.

---

## Sources

- [Mirantis: Long-Term Support for Swarm Through 2030](https://www.mirantis.com/blog/mirantis-guarantees-long-term-support-for-swarm/)
- [Mirantis: Swarm is Here to Stay](https://www.mirantis.com/blog/swarm-is-here-to-stay-and-keeps-getting-better-in-security-and-ease-of-operations/)
- [Docker Engine v28 Release Notes](https://docs.docker.com/engine/release-notes/28/)
- [Docker Swarm in 2025 - Niksa Makitan](https://medium.com/@niksa.makitan/docker-swarm-in-2025-0d2f2bc5d929)
- [Docker Swarm vs Kubernetes in 2026 - The Decipherist](https://thedecipherist.com/articles/docker_swarm_vs_kubernetes/)
- [Is Swarm Dead? - Bret Fisher (Docker Captain)](https://www.bretfisher.com/blog/is-swarm-dead-answered-by-a-docker-captain)
- [Create a Swarm - Docker Docs](https://docs.docker.com/engine/swarm/swarm-tutorial/create-swarm/)
- [Deploy a Stack to a Swarm - Docker Docs](https://docs.docker.com/engine/swarm/stack-deploy/)
- [Overlay Network Driver - Docker Docs](https://docs.docker.com/engine/network/drivers/overlay/)
- [Swarm Routing Mesh - Docker Docs](https://docs.docker.com/engine/swarm/ingress/)
- [Apply Rolling Updates - Docker Docs](https://docs.docker.com/engine/swarm/swarm-tutorial/rolling-update/)
- [Port Docker Compose to Swarm - Planetary Quantum](https://docs.planetary-quantum.com/getting-started/port-docker-compose-to-docker-swarm/)
- [99% Performance Loss with Encrypted Overlay - moby/moby #33133](https://github.com/moby/moby/issues/33133)
- [Overlay Network Stops Working - moby/moby #32738](https://github.com/moby/moby/issues/32738)
- [Stale IPVS Entries - moby/moby #36878](https://github.com/moby/moby/issues/36878)
- [Conntrack Issues - moby/moby #40374](https://github.com/moby/moby/issues/40374)
- [Docker Secrets Docs](https://docs.docker.com/engine/swarm/secrets/)
- [Volume Sharing in Swarm - DEV.to](https://dev.to/iamrj846/how-does-docker-swarm-implement-volume-sharing-51dc)
- [Logging in Docker Swarm - Last9](https://last9.io/blog/logging-in-docker-swarm/)
- [Swarm Limitations - Bobcares](https://bobcares.com/blog/docker-swarm-limitations/)
- [Leaving the Swarm: Road to K8s - CNCF](https://www.cncf.io/blog/2020/09/14/leaving-the-swarm-the-road-to-kubernetes/)
- [Swarm to K8s Migration - Mirantis](https://www.mirantis.com/blog/swarm-to-kubernetes/)
- [Docker Swarm Still Thriving - Mirantis](https://www.mirantis.com/company/press-center/company-news/docker-swarm-still-thriving-three-years-after-mirantis-acquisition-often-running-side-by-side-with-kubernetes/)
