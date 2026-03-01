# Banyan Positioning & Website Content — White Paper Research

Current website content inventory, messaging analysis, and white paper outline.

---

## 1. Current Website Content Inventory

### Landing Page (getbanyan.dev)

**Core messaging:**
- Title: "Container orchestration you already know"
- Tagline: "Run containers across multiple servers with the Docker Compose syntax you already know."
- Disclaimer: "Banyan is not yet production-ready" (under experiment)

**Trust signals:**
- "Secured by WireGuard" badge
- "Built with" badges: containerd, nerdctl, etcd, Go

**Narrative arc:**
- "From one server to many" — You know Docker Compose, it works on one machine, then you need more. The usual next step involves weeks of learning. Banyan takes a different approach: same YAML, distributed across servers.
- "One manifest, production-ready" — Shows a manifest with `deploy.replicas`, `deploy.placement.node`

**Six feature cards:**
1. "The YAML you already know" — same fields, same structure
2. "Three binaries, nothing else" — banyan-engine, banyan-agent, banyan-cli, no plugins
3. "Built-in image registry" — use `build:` and Banyan handles image distribution
4. "Containers talk across servers" — overlay network + DNS, encrypted with WireGuard
5. "Live terminal dashboard" — real-time TUI monitoring, no Grafana needed
6. "Open source, self-hosted" — Apache 2.0, no vendor lock-in

**Target audience section:**
- "Teams who've outgrown a single server but don't need — or don't want — Kubernetes."
- Team of 5 needing API on 3 servers
- Team of 50 wanting lighter option for staging/internal tools
- Anyone who wants to write YAML and ship, not operate a platform

### Documentation Pages

1. **Installation** — One-liner curl install, supports Ubuntu/Debian/CentOS/RHEL/Fedora/Rocky/Alma, x86_64 + ARM64
2. **Quickstart** — Single-machine tutorial: engine → agent → CLI → deploy nginx + redis
3. **Authentication** — Public key whitelist, WireGuard control tunnel, planned mTLS + OIDC
4. **Monitoring** — Terminal dashboard (TUI) + Prometheus metrics endpoint
5. **Multi-Node** — Adding workers, round-robin distribution, firewall requirements
6. **Redeployment** — Blue-green (full) + recreate (per-service), "run the same command again"
7. **CLI Reference** — Three binaries with all flags and example output
8. **Manifest Reference** — Docker Compose compatibility table (11 concepts mapped)
9. **Troubleshooting** — 18 issues across engine, agent, deployment, and general categories

### Roadmap

**Completed (7 milestones):**
1. Core Orchestration (MVP)
2. Service Observability
3. Security (WireGuard auth, keypairs, whitelist)
3.5. Agent Tags
3.6. Networking (WireGuard overlay, VXLAN, DNS, cross-host LB)
4. Blue-Green Redeployment
4.6. Live Terminal Dashboard

**Upcoming (7 milestones):**
5. Rootless Mode
6. Health-Based Scheduling
7. Multi-Engine HA
8. Auto-Scaling
9. Web Monitoring Dashboard
10. Advanced Security (ABAC, secrets, cert rotation)
11. Dynamic Workload Rebalancing
12. Advanced Networking (health-check routing, session affinity, network policies, L7 ingress)

---

## 2. Current Positioning Analysis

### Primary Positioning

"The bridge between `docker compose up` and production" — for teams that outgrew one server but find Kubernetes too complex.

### Core Differentiators vs Kubernetes

1. Same Docker Compose YAML syntax (no new language)
2. Three concepts only: engine, agent, manifest
3. Three static binaries, no plugins/package managers
4. Zero-config overlay networking (WireGuard encrypted)
5. Built-in image registry (no Docker Hub or private registry needed)
6. Built-in terminal dashboard (no Grafana setup)
7. "Scaling is adding a server, not editing a manifest"

### Pain Points Addressed

- "Weeks of learning" to go multi-server
- "Dozens of new concepts" in Kubernetes
- "Infrastructure heavier than your application"
- Need for dedicated DevOps/platform team
- Complex YAML templating (Helm charts, CRDs)
- External dependency sprawl (Consul, Vault, Prometheus)

### What's Missing from Current Website

- No comparison page against specific competitors
- No case studies or testimonials
- No benchmarks or performance data
- No white paper or technical deep-dive
- No "when NOT to use Banyan" guidance

---

## 3. White Paper Outline — WHY → HOW → WHAT

### Title Options

- "Bridging the Gap: Container Orchestration for Teams Between Docker Compose and Kubernetes"
- "The Missing Middle: Why Container Orchestration Needs a Third Option"
- "From One Server to Many: Container Orchestration Without the Complexity Tax"

### Structure

#### Part I: WHY — The Problem

**1. The Single-Server Ceiling**
- Docker Compose works beautifully on one machine
- 92% of IT professionals use containers (Docker 2025)
- The moment you need a second server, everything changes
- The gap: multi-host distribution, service discovery, health-based rescheduling, zero-downtime deploys, cross-host networking, load balancing — needed as a bundle, not incrementally

**2. The Kubernetes Cliff**
- K8s production usage at 82% — but only 9% of users are in companies under 1,000 employees
- 79% of K8s production outages trace to YAML misconfiguration
- Average K8s engineer: $166K/yr; platform engineer: $172K/yr
- TCO: ~$569K/yr for self-hosted with 24/7 coverage
- For a team of 10, dedicating 2-3 to platform = 20-30% capacity to infrastructure, not product
- Managed K8s (GKE/EKS/AKS) reduces infra complexity but NOT application-layer complexity

**3. The Landscape Today**
- Docker Swarm: right idea, maintained but not evolving, perception problem
- Nomad: simpler but BSL license + IBM acquisition uncertainty
- ECS/Fargate: AWS lock-in, complex billing
- Kamal: deployment without orchestration (no service discovery, no auto-healing)
- Cloud PaaS: vendor lock-in, pricing surprises
- K3s: lighter K8s, same cognitive complexity
- Self-hosted PaaS (Coolify/Dokku): single-server limit
- **Table: The Missing Middle** — gap between Docker Compose and Kubernetes

**4. What Small Teams Actually Need**
- Deployment confidence (deploy Friday afternoon without anxiety)
- Basic HA (one server dies, services keep running)
- Service discovery (services find each other across hosts)
- Observability (what's running, what's failing, why)
- Reasonable security (encrypted comms, access control)
- Operational simplicity (same engineers write code and deploy)
- K8s provides all of this + much more. The question: is the "much more" worth the cost?

#### Part II: HOW — Design Principles

**5. Three Concepts, Not Thirty**
- Engine (control plane), Agent (data plane), Manifest (intent)
- No CRDs, no operators, no sidecars, no service meshes
- Every feature flows through these three primitives
- Comparison: K8s has 50+ resource types; Banyan has 3

**6. Compatibility Over Novelty**
- Use Docker Compose syntax, not a new YAML dialect
- Same fields: services, image, ports, environment, command, depends_on, replicas
- Knowledge transfer: zero (if you know Compose)
- Anti-pattern avoided: Helm charts, CRDs, custom resource types, YAML templating

**7. Convergence Over Coordination**
- Pull-based architecture: agents poll engine for tasks
- No distributed transactions, no two-phase commit, no leader election
- State converges through polling and idempotent operations
- Same model as DNS (TTL-based refresh) and BGP (periodic advertisements)
- Trade-off: seconds of convergence delay, acceptable for deployment (not HFT)

**8. Complexity Hidden, Not Eliminated**
- The networking, service discovery, load balancing, and deployment strategies exist — but users don't configure them
- Analogy: TCP hides packet retransmission, routing, congestion control. Users just connect.
- Table: "What you do" vs "What actually happens"

#### Part III: WHAT — Technical Architecture (Design, Not Code)

**9. The Engine-Agent Model**
- Engine: single process (etcd + registry + gRPC server + orchestration loop)
- Agent: single process (gRPC client + task executor + health monitor + DNS server + overlay driver)
- Communication: gRPC with pull-based task polling
- State: etcd key-value store, namespace-scoped by agent

**10. Overlay Networking**
- WireGuard driver (default, encrypted) or VXLAN driver (fallback)
- Subnet allocation: engine carves /24 per agent from VPC CIDR
- Peer distribution: piggybacks on heartbeat — no gossip protocol, no Consul
- Bridge + CNI: standard bridge plugin with host-local IPAM
- Convergence: ~15 seconds for new peer visibility
- Deterministic MAC addresses eliminate MAC exchange

**11. Service Discovery**
- Agent-local DNS server on bridge gateway IP
- Immediate local registration + cluster-wide distribution via heartbeat
- Containers resolve `db` → `db.internal` → container IP(s)
- 60-second TTL, health-aware resolution
- No CoreDNS, no Consul, no service mesh

**12. Zero-Downtime Deployment**
- Default strategy: blue-green
- iptables proxy eliminates port conflicts (DNAT, not host port binding)
- Health confirmation before old teardown
- Service adoption for selective deploys
- Failure safety: old stays running if new fails

**13. Cross-Host Load Balancing**
- iptables DNAT with probability-based rules (kube-proxy model)
- Backend distribution via heartbeat
- Convergence: local immediate, remote ~25 seconds
- Kernel handles all packet forwarding — no userspace proxy

**14. Security Model**
- WireGuard control tunnel (all gRPC encrypted)
- Public key whitelist (no passwords, no shared secrets)
- Session tokens for engine-to-agent auth
- Deterministic IP assignment from public key
- Fallback to direct TCP if WireGuard unavailable

#### Part IV: Honest Assessment

**15. When to Use Banyan**
- 2-30 engineers, outgrown single server, don't have/want K8s platform team
- Same engineers write code and deploy
- Docker Compose familiarity
- Self-hosted infrastructure preferred

**16. When NOT to Use Banyan**
- 50+ services, multi-team orgs → Kubernetes
- AI/ML with GPU scheduling → Kubernetes
- Regulatory/compliance needs (RBAC, audit logging) → Kubernetes
- All-in on AWS → ECS/Fargate may be simpler
- Single server is sufficient → Docker Compose is fine

**17. Current Limitations (Transparent)**
- No volume support (persistent storage)
- No autoscaling
- Single engine is SPOF (multi-engine HA on roadmap)
- No RBAC/ABAC
- No secrets management
- Not yet production-ready
- L4 proxy only (no L7 ingress)
- No session affinity, no network policies

**18. Roadmap**
- Near term: rootless mode, health-based scheduling
- Medium term: multi-engine HA, auto-scaling, web dashboard
- Long term: advanced security (ABAC, secrets), advanced networking (L7, session affinity)

#### Part V: Getting Started

**19. From Reading to Deploying**
- Three commands to a running cluster
- Architecture diagram
- Links to quickstart, installation, docs

---

## 4. Key Statistics for the White Paper (Quick Reference)

| Statistic | Value | Source |
|-----------|-------|--------|
| K8s production usage | 82% | CNCF 2025 Survey |
| K8s outages from YAML misconfig | 79% | CNCF 2025 Report |
| K8s users in companies <1000 | 9% | Jeevi Academy |
| K8s complexity as challenge | 34% | CNCF 2025 Survey |
| Cultural change as challenge | 47% | CNCF 2025 Survey |
| K8s engineer avg salary | $166,836 | kube.careers Q2 2025 |
| Platform engineer avg salary | $172,038 | kube.careers Q1 2025 |
| K8s TCO (self-hosted, 24/7) | ~$569K/yr | Koyeb |
| Container usage among IT pros | 92% | Docker 2025 |
| Cloud-native developers | 15.6M | CNCF/SlashData |
| Orgs with K8s skill shortages | 75% | Tigera |
| Orgs running K8s multi-cloud | 65% | CNCF 2025 Survey |
| Docker Swarm support through | 2030 | Mirantis |
| IBM HashiCorp acquisition | $6.4B, Feb 2025 | IBM |
| Fargate price premium vs EC2 | 20-30% | CloudBurn |
