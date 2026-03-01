# From One Server to Many: Container Orchestration Without the Complexity Tax

*Banyan — Container orchestration you already know*

---

## Abstract

Most teams today use containers. And for most of them, Docker Compose is where it starts — a YAML file, a few services, `docker compose up`, done. It works great on one machine.

And when it's time to go to production, many of us keep using Docker Compose there too — because a single server is all we need. It's familiar, it's simple, and it gets the job done.

Then you need a second server. Maybe traffic grew, or you want high availability, or the database needs its own hardware. And suddenly, `docker compose up` can't help — it deploys to a single host.

Docker did try to solve this with Docker Swarm, which reads the same Compose file format and distributes containers across servers via `docker stack deploy`. The setup is genuinely simple — two commands and you have a cluster. But Swarm has its own problems: overlay encryption [carries a severe performance penalty](https://github.com/moby/moby/issues/33133) (IPsec, with reports of up to 99% throughput loss), there's no built-in observability (you need to deploy a separate monitoring stack), and the project has stagnated — maintained by Mirantis through 2030, but not actively evolving. We cover Swarm in more detail in [Section 3](#docker-swarm--the-road-not-taken).

So the usual next step is Kubernetes. But Kubernetes is a platform built for platform teams — and if your team is five engineers shipping a product, spending months learning Deployments, Services, Ingress, Helm charts, CNI plugins, and RBAC is a hard sell.

There should be something in between. That's what Banyan is: a container orchestrator where everything is built in, the manifest feels like the Docker Compose file you already know, and the project is forever open source.

This paper looks honestly at the container orchestration landscape — where Kubernetes is the right choice, where it isn't, what alternatives exist and what they trade off — and then explains how Banyan works and why it's designed the way it is.

---

## Table of Contents

1. [The Problem: Why Container Orchestration Needs a Third Option](#part-i-why--the-problem)
   - [The Single-Server Ceiling](#1-the-single-server-ceiling)
   - [The Kubernetes Cliff](#2-the-kubernetes-cliff)
   - [The Landscape Today](#3-the-landscape-today)
   - [What Small Teams Actually Need](#4-what-small-teams-actually-need)
2. [The Approach: Design Principles Behind Banyan](#part-ii-how--design-principles)
   - [Three Concepts, Not Thirty](#5-three-concepts-not-thirty)
   - [Everything Built In](#6-everything-built-in)
   - [Your Compose File, Not a New Language](#7-your-compose-file-not-a-new-language)
   - [Convergence Over Coordination](#8-convergence-over-coordination)
   - [Open Source, Fully and Forever](#9-open-source-fully-and-forever)
3. [The Architecture: How Complexity Disappears](#part-iii-what--technical-architecture)
   - [The Engine-Agent Model](#10-the-engine-agent-model)
   - [Overlay Networking](#11-overlay-networking--encrypted-by-default)
   - [Service Discovery](#12-service-discovery--dns-not-a-service-mesh)
   - [Cross-Host Load Balancing](#13-cross-host-load-balancing)
   - [Zero-Downtime Deployment](#14-zero-downtime-deployment)
   - [Security](#15-security)
   - [Observability](#16-observability)
4. [Honest Assessment](#part-iv-honest-assessment)
   - [When to Use Banyan](#17-when-to-use-banyan)
   - [When NOT to Use Banyan](#18-when-not-to-use-banyan)
   - [Current Limitations](#19-current-limitations)
   - [Roadmap](#20-roadmap)
5. [Try Banyan](#try-banyan)
6. [References](#references)

---

## Part I: WHY — The Problem

### 1. The Single-Server Ceiling

Docker Compose works beautifully on one machine. You write a YAML file describing your services — their images, ports, environment variables, how they connect. Run `docker compose up` and everything is running. The mental model is simple: services are processes, the Compose file is the manifest, your laptop or server is the platform.

For a lot of teams, this is genuinely enough. A small business app, an internal tool, an early-stage startup — one server handles it fine. Running Docker Compose in production is a perfectly valid choice when your workload fits on a single host.

Then you hit the ceiling. Traffic grows. The database wants its own hardware. You need high availability because one server going down takes everything with it. And the moment you need a second server, Docker Compose has nothing to offer. It was designed for one machine.

Here's the thing: the gap between "one server" and "many servers" isn't a single missing feature. It's a **bundle of things** you need all at once:

- **Multi-host distribution** — putting containers on different servers
- **Service discovery** — services finding each other by name, across hosts
- **Health-based rescheduling** — restarting failed containers on healthy nodes
- **Zero-downtime deploys** — updating without taking the service down
- **Cross-host networking** — containers on different servers talking to each other, securely
- **Load balancing** — spreading traffic across replicas on different hosts

You can't just bolt these onto Docker Compose one at a time. You need an orchestration layer. And for a long time, the only serious option has been Kubernetes.

### 2. The Kubernetes Cliff

Let's be honest about Kubernetes up front: it is the right tool for a lot of situations. The [CNCF 2025 survey](https://www.cncf.io/announcements/2026/01/20/kubernetes-established-as-the-de-facto-operating-system-for-ai-as-production-use-hits-82-in-2025-cncf-annual-survey/) reports production usage at 82%. When you're running dozens of teams, hundreds of microservices, or AI/ML workloads with GPU scheduling — nothing else comes close in maturity and ecosystem.

Dismissing Kubernetes would be dishonest. This paper isn't about that.

But the data tells a more nuanced story about *who* actually uses it.

**Kubernetes skews heavily toward large organizations.** According to [Jeevi Academy's 2025 analysis](https://www.jeeviacademy.com/kubernetes-adoption-statistics-and-trends-for-2025/), only about 9% of K8s users are in companies with under 1,000 employees. The majority are in organizations with thousands or tens of thousands of people — the kind of places that can afford dedicated platform teams.

**The financial cost is real.** Kubernetes engineers average [$166K/year](https://kube.careers/state-of-kubernetes-jobs-2025-q2), platform engineers [$172K/year](https://kube.careers/state-of-kubernetes-jobs-2025-q1). [Koyeb estimates](https://www.koyeb.com/blog/the-true-cost-of-kubernetes-people-time-and-productivity) the total cost of self-hosted K8s with 24/7 coverage at roughly $569K/year — and that's before infrastructure costs. You need about 4 engineers for round-the-clock coverage.

If your team has 10 engineers total, putting 2–3 on platform work means **20–30% of your engineering capacity goes to infrastructure** instead of your product. And [75% of organizations](https://www.tigera.io/learn/guides/kubernetes-security/kubernetes-statistics/) report K8s skill shortages, so finding those engineers isn't easy either.

**The cognitive cost adds up too.** A [2025 CNCF report](https://www.cncf.io/blog/2026/01/20/kubernetes-fuels-ai-growth-organizational-culture-remains-the-decisive-factor/) found that 79% of Kubernetes production outages trace back to YAML misconfigurations. The community has even proposed KYAML — a safer YAML subset — as an acknowledgment that the YAML itself is part of the problem.

Here's a concrete example that illustrates the gap. Deploying a web application with 3 replicas on port 80:

**In Docker Compose — 7 lines:**
```yaml
services:
  my-app:
    image: my-app:latest
    ports:
      - "80:8080"
    deploy:
      replicas: 3
```

**In Kubernetes — 45 lines across 3 resource types:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
      - name: my-app
        image: my-app:latest
        ports:
        - containerPort: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: my-app
spec:
  selector:
    app: my-app
  ports:
  - port: 80
    targetPort: 8080
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app
spec:
  rules:
  - http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-app
            port:
              number: 80
```

Three resource types (Deployment, Service, Ingress), each with its own API version, kind, metadata, and spec. The Deployment references labels that the Service must match. The Ingress references the Service by name and port. A typo in any of those references and the deployment silently fails.

This isn't a cherry-picked example. This is the minimum for a web app accessible on port 80.

**Do managed services (GKE, EKS, AKS) solve this?**

They help with the infrastructure side — no node management, automatic upgrades, managed control plane. But they don't touch the application-layer complexity. You still write the same Deployments, Services, Ingresses, ConfigMaps, Secrets, and Helm charts. You still choose your CNI, your service mesh, your observability stack. Managed K8s makes the control plane someone else's problem. It doesn't make *your* YAML simpler.

The 2025 CNCF survey shows a revealing shift: complexity dropped to 34% as a cited challenge, but "cultural changes with the development team" rose to 47% as the *top* challenge. Complexity didn't get solved — the teams that couldn't handle it already left or never adopted. The survivors now face organizational friction.

### 3. The Landscape Today

Between Docker Compose and Kubernetes, several tools try to fill the gap. Each makes trade-offs worth looking at honestly.

#### Docker Swarm — The Road Not Taken

Swarm's simplicity claim is real. Two commands — `docker swarm init` and `docker swarm join` — and you have a multi-node cluster. Deploy a Compose file with `docker stack deploy`. The learning curve from Docker Compose is genuinely small.

But the cracks run deep:

- **Compose compatibility is only partial.** Swarm uses the [legacy Compose v3 format](https://docs.docker.com/engine/swarm/stack-deploy/), not the modern Compose spec. Many fields are silently ignored in production: `build` (completely ignored), `depends_on` (ignored), `container_name` (ignored), `.env` substitution (not supported). "Just use your existing Compose file" doesn't quite match reality.
- **Encryption costs too much performance.** Swarm uses IPsec for overlay encryption. A [GitHub issue](https://github.com/moby/moby/issues/33133) reported up to 99% throughput loss in extreme cases. You end up choosing between security and performance.
- **No built-in observability.** `docker service logs` is CLI-only, per-service, no search, no dashboards. You need to deploy ELK or Prometheus+Grafana — which starts to undermine the simplicity story.

Swarm isn't dead — [Mirantis committed to support through 2030](https://www.mirantis.com/blog/mirantis-guarantees-long-term-support-for-swarm/), and there are 100+ enterprise customers running it. But it's maintained, not evolving. And Mirantis's next-gen platform (MKE 4) is Kubernetes-only.

#### HashiCorp Nomad — Simple Core, Complex Ecosystem

Nomad's core scheduler is genuinely simpler than Kubernetes. A small team can manage a Nomad cluster with basic sysadmin skills. It handles containers, VMs, and batch jobs, and clusters come up fast.

But Nomad illustrates a pattern you see across the orchestration landscape: **the core might be simple, but production needs an ecosystem.**

Need service discovery? Add **Consul** — a separate distributed system with its own cluster, config, and ops burden. Need secrets? Add **Vault** — another distributed system with its own HA setup and seal/unseal ceremony. Need a service mesh? Consul Connect. Monitoring? Separate stack.

Each piece is well-built. But now you're running three clusters (Nomad + Consul + Vault), three config languages, three upgrade cycles, three things that can break independently. The "simplicity" of Nomad's core gets offset by the complexity of assembling a production system from separate pieces.

On top of that, [HashiCorp switched all products to BSL](https://www.hashicorp.com/en/blog/hashicorp-adopts-business-source-license) in August 2023, and [IBM completed its $6.4B acquisition](https://newsroom.ibm.com/2025-02-27-ibm-completes-acquisition-of-hashicorp,-creates-comprehensive,-end-to-end-hybrid-cloud-platform) in February 2025. The strategic focus seems pointed at Terraform and Vault + Red Hat integration, leaving Nomad's future a bit uncertain.

#### Lightweight Kubernetes — K3s and MicroK8s

K3s packages Kubernetes into a single <40MB binary. [MicroK8s](https://canonical.com/microk8s) offers one-command installation on Ubuntu with a handy addon system.

Both solve a real problem: making Kubernetes easier to **install**. But neither makes Kubernetes easier to **use**.

After installing K3s or MicroK8s, you still write the same 45-line YAML for a web app. You still need Helm, still debug with `kubectl describe`, still configure Ingress controllers, still manage RBAC. The binary is smaller; the cognitive load is the same.

MicroK8s's addon system (`microk8s enable prometheus`) is genuinely convenient for infrastructure setup. But enabling Prometheus is the easy part — you still need to write ServiceMonitors, set up storage, create alert rules, and build dashboards.

**K3s and MicroK8s make it easier to get Kubernetes running. They don't make it easier to *be* a Kubernetes user.**

#### Talos Linux — Simplifying the Layer Below

[Talos Linux](https://www.talos.dev/) takes a completely different approach. It's a minimal, immutable OS built only for Kubernetes: no SSH, no shell, no package manager, just 12 binaries (Ubuntu has ~2,780). The security model is genuinely impressive — Talos wasn't even vulnerable to the XZ Utils backdoor because xz simply isn't shipped.

Talos simplifies what's **below** the Kubernetes API: OS security, cluster bootstrapping, node lifecycle, upgrades. It does this very well. JYSK runs it across 3,400 stores, Civo uses it as their cloud infrastructure base, PostFinance runs 35 clusters on it.

But Talos changes nothing **above** the Kubernetes API. After spinning up a beautiful, secure Talos cluster, your team still faces the same application deployment complexity: Deployments, Services, Helm charts, CNI choices, storage classes. The developer experience is identical to any other K8s distribution.

Talos addresses maybe 10–20% of the total complexity — the infrastructure layer. The other 80–90% — actually deploying and running your apps — stays the same.

Worth noting: Sidero Labs' commercial management platform, **Omni**, uses the [Business Source License](https://github.com/siderolabs/omni). Talos itself is open source (MPL-2.0), but the tool that makes it practical at scale has licensing restrictions.

#### Other Options

- **Kamal** (37signals): Pushes Docker containers over SSH. No running daemon, no service discovery, no auto-healing. Great for Rails apps on Hetzner; not really an orchestrator.
- **ECS/Fargate**: Solid if you're all-in on AWS. Abstracts cluster management but locks you into proprietary APIs. [Fargate runs 20–30% more](https://cloudburn.io/blog/amazon-ecs-pricing) than managed EC2.
- **Self-hosted PaaS** (Coolify, Dokku, CapRover): Fundamentally single-server tools. Solve "easy deploy" but not "scale beyond one server."
- **Cloud PaaS** (Railway, Render, Fly.io): Fast to start, but vendor lock-in and pricing surprises at scale.

#### The Missing Middle

| Tier | Solution | What It Requires | Where It Breaks |
|------|----------|-----------------|-----------------|
| Single server | Docker Compose | Docker knowledge | Can't scale beyond one host |
| Push-based deploy | Kamal | SSH + Docker | No orchestration, no service discovery |
| Lightweight K8s | K3s, MicroK8s | Full K8s knowledge | Same K8s complexity, smaller binary |
| Full orchestration | Kubernetes | Dedicated platform team | Massive overhead for small teams |
| Managed cloud | ECS, Railway, Render | Vendor commitment | Lock-in, pricing surprises |

The gap is real. Teams of 2–30 engineers who need multi-host deployment, service discovery, and zero-downtime deploys — but don't have the budget or expertise for a K8s platform team — don't have great options.

### 4. What Small Teams Actually Need

For a small team, "production-ready" doesn't mean what it means for an enterprise running hundreds of microservices across multiple clouds. It means:

1. **Deployment confidence.** Can you deploy on a Friday afternoon without anxiety? Can you push an update without taking the service down?
2. **Basic high availability.** If one server dies, do the others keep running?
3. **Service discovery.** Can services find each other by name across hosts, without you managing IPs manually?
4. **Observability.** Can you see what's running and what's failing — without deploying a whole monitoring stack first?
5. **Reasonable security.** Is traffic encrypted? Is there some form of access control?
6. **Operational simplicity.** Can the same people who write the code also deploy it, without a 3-month learning curve?

Kubernetes gives you all of this and a *lot* more — autoscaling, network policies, RBAC, custom resources, operator patterns, service meshes. The question is whether the "lot more" is worth the cost for teams that just need the basics.

For many small teams, the answer is no. But until now, the basics haven't been available without the overhead.

---

## Part II: HOW — Design Principles

### 5. Three Concepts, Not Thirty

Banyan's entire architecture rests on three concepts:

- **Engine** — the control plane. A single process that stores state, runs the orchestration loop, and serves as the communication hub.
- **Agent** — the data plane. One per server. Runs containers, reports health, maintains networking.
- **Manifest** — the intent. A Docker Compose file describing what should be running.

Every feature in Banyan flows through these three things. There are no Custom Resource Definitions, no operators, no sidecars, no service meshes, no Helm charts, no package managers.

Kubernetes has 50+ resource types in its core API: Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CronJob, Service, Ingress, ConfigMap, Secret, PersistentVolume, PersistentVolumeClaim, StorageClass, Namespace, ServiceAccount, Role, RoleBinding, ClusterRole, ClusterRoleBinding, NetworkPolicy, and many more. Each one is a concept that engineers need to understand, configure correctly, and debug when things break.

Banyan has three. Not because the problems are simpler — but because the solutions are built into the system instead of assembled from parts.

### 6. Everything Built In

This is the heart of what makes Banyan different: **everything you need for production deployment is already in the box.**

Here's the pattern you see across the orchestration landscape. A tool claims simplicity for its core, but production use requires assembling an ecosystem around it:

| Capability | Kubernetes | Nomad | Docker Swarm | Banyan |
|-----------|-----------|-------|-------------|--------|
| **Service discovery** | CoreDNS (addon) or Consul | Consul (separate cluster) | Built-in DNS | **Built in** |
| **Secret management** | K8s Secrets (base64) or Vault | Vault (separate cluster) | Built-in Docker Secrets | **Planned** *(coming soon)* |
| **Container registry** | Docker Hub / ECR / Harbor | External registry | External registry | **Built in** |
| **Observability** | Prometheus + Grafana (separate) | Separate stack | None | **Built in** |
| **Overlay networking** | Pick a CNI (Calico? Cilium? Flannel?) | Consul Connect or CNI | VXLAN (IPsec, performance hit) | **Built in** (WireGuard) |
| **Load balancing** | kube-proxy + pick an Ingress controller | Fabio / Traefik / Consul | IPVS routing mesh | **Built in** |
| **State store** | etcd (manage it, or use managed K8s) | Separate Consul/etcd | Embedded Raft | **Managed** (you don't touch it) |

Take Nomad. The scheduler itself is simple. But a real production setup usually means running three separate distributed systems: Nomad for orchestration, Consul for service discovery, and Vault for secrets. Each has its own cluster topology, config format, upgrade cycle, and failure modes. Any one piece is well-engineered. Together, the operational surface area adds up fast.

With Kubernetes, the ecosystem is even wider. CoreDNS, a CNI plugin, an Ingress controller, cert-manager, Prometheus + Grafana, a logging stack, a secrets solution, a GitOps tool. All well-built. All another thing to learn, configure, keep running, and upgrade.

Banyan takes a different approach: **you shouldn't have to research, evaluate, install, or integrate external tools just to deploy containers across servers.** Service discovery, networking, load balancing, observability, and a container registry are all part of the engine and agents. The state store (etcd) is embedded — you don't operate it, back it up, or even think about it.

This is an opinionated stance. Banyan picks sensible defaults instead of offering maximum configurability. The DNS server uses 60-second TTLs. The overlay uses WireGuard. Deployments default to blue-green. The load balancer uses probability-based iptables rules. These aren't the only valid choices — but they're good choices that work for the target use case without you having to make them.

Think of how TCP works. TCP hides packet retransmission, congestion control, flow control, and connection management from you. You just connect. The complexity is real — you just don't deal with it. Banyan applies the same idea to container orchestration.

### 7. Your Compose File, Not a New Language

Banyan uses Docker Compose syntax — the same fields, the same structure. If you've written a Compose file before, a Banyan manifest will feel immediately familiar:

```yaml
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      - API_URL=http://api:3000
    depends_on:
      - api

  api:
    build: ./api
    ports:
      - "3000:3000"
    environment:
      - DATABASE_URL=postgres://db:5432/myapp
    deploy:
      replicas: 3

  db:
    image: postgres:16
    environment:
      - POSTGRES_DB=myapp
    ports:
      - "5432:5432"
```

Run `banyan up` and this deploys across your servers. The `build` directive works — Banyan builds the image and distributes it through its built-in registry. `depends_on` is respected. `deploy.replicas` distributes instances across available agents.

Knowledge transfer cost: zero, if your team already knows Docker Compose. No Helm charts to write, no CRDs to learn, no template language to figure out, no package manager to install.

Docker Swarm also promises Compose compatibility, but it's stuck on the legacy v3 format. Fields like `build`, `depends_on`, `container_name`, and `.env` substitution are [silently ignored](https://docs.docker.com/engine/swarm/stack-deploy/) when you deploy to a Swarm. That gap between "works locally" and "works in production" is exactly the kind of surprise you don't want. Banyan treats the Compose file as the source of truth — what it says is what happens.

### 8. Convergence Over Coordination

Most distributed systems work through synchronous coordination: a control plane pushes commands to workers, waits for acknowledgment, handles failures, rolls back on partial success. Distributed transactions, saga patterns, two-phase commit.

Banyan does something simpler: **convergence loops.**

- The engine runs a **3-second orchestration loop** — checking deployment state, creating tasks, updating status
- Agents **poll for tasks every 2 seconds** — pulling work instead of receiving pushes
- Agents send **heartbeats every 15 seconds** — reporting health, receiving peer lists and service backends
- Agents check **container health every 10 seconds** — monitoring what's actually running

These loops run independently. There's no distributed transaction, no two-phase commit, no leader election among agents, no message queue.

State converges through repeated polling and idempotent operations. If an agent misses a heartbeat, the next one catches up. If a task poll fails, the next poll succeeds. If the engine restarts, agents reconnect and re-register. Everything is designed to be safely retried.

This is the same model that makes DNS reliable — eventual consistency through TTL-based refresh. Same idea behind BGP — periodic route advertisements. The trade-off is convergence delay: seconds, not milliseconds. A new service backend takes about 25 seconds to be routable everywhere (10 seconds for health check + 15 seconds for heartbeat).

For deploying web apps, APIs, and databases across a handful of servers, seconds of delay is fine. This isn't high-frequency trading. The resilience you get from avoiding synchronous coordination is well worth it.

### 9. Open Source, Fully and Forever

Banyan is licensed under **Apache 2.0**. Everything — engine, agent, CLI, overlay networking, service discovery, load balancing, built-in registry, terminal dashboard — is open source. There are no closed-source components, no "enterprise edition" with gated features, no BSL-licensed management layer.

This matters because the infrastructure world has been through a wave of license changes that have burned a lot of people:

- **HashiCorp** [switched Terraform, Vault, Consul, and Nomad to BSL](https://www.hashicorp.com/en/blog/hashicorp-adopts-business-source-license) in August 2023, prompting the OpenTofu fork.
- **Sidero Labs** released Omni under [BSL-1.1](https://github.com/siderolabs/omni) — the core (Talos Linux) is open source, but the management tool needed at scale is not.
- **Redis Labs**, **MongoDB**, **Elastic**, and **CockroachDB** have all changed licenses to restrict cloud providers, often catching their user communities in the crossfire.
- **Red Hat** restricted CentOS Stream and RHEL source access, upending the downstream ecosystem.

The pattern is familiar: build community around open-source software, then change the terms once adoption hits critical mass. Users who built on these tools get to choose between accepting new terms or undertaking painful migrations.

Banyan's position is simple: **the open-source version is the full version.** If Banyan offers a cloud service in the future, it'll be a managed deployment of the same open-source software — not a feature-gated product with things held back. No "Banyan Enterprise." No BSL conversion schedule. No features that require a commercial license.

This isn't charity — it's a deliberate choice. The people Banyan is built for are small teams who need to trust that their infrastructure won't change terms underneath them. Apache 2.0 provides that trust in a way BSL and proprietary licenses can't.

---

## Part III: WHAT — Technical Architecture

This section covers how Banyan works at a high level — the key design decisions and why they were made.

### 10. The Engine-Agent Model

The engine is a single process that bundles four things you'd normally set up separately: a state store (etcd, embedded — you never touch it), a container registry (for `build:` directives — no Docker Hub needed), a gRPC server, and an orchestration loop that runs every 3 seconds.

Each server runs one agent. Agents are **pull-based** — they poll the engine for tasks every 2 seconds, send heartbeats every 15 seconds, and check container health every 10 seconds. If the network drops, agents just keep polling until it's back. No message queue, no callback infrastructure.

Containers run through containerd (via nerdctl), not the Docker daemon. If the agent crashes and restarts, running containers are unaffected — they're independent processes.

### 11. Overlay Networking — Encrypted by Default

Containers on different hosts need to talk to each other. Banyan creates a virtual overlay network with zero configuration.

The engine allocates each agent a /24 subnet. The overlay uses **WireGuard** — encrypted at the kernel level with roughly 4% overhead. Compare that to Docker Swarm's IPsec, which adds 10%+ overhead (and [much worse in practice](https://github.com/moby/moby/issues/33133)).

Peer discovery piggybacks on the heartbeat that already exists — no gossip protocol, no Consul. A new agent becomes reachable within about 15 seconds.

You don't choose a CNI, configure subnets, or manage peers. Containers across hosts just talk to each other.

### 12. Service Discovery — DNS, Not a Service Mesh

Each agent runs its own DNS server. The engine distributes all running service backends to every agent through the heartbeat, so each agent has a complete, cluster-wide view. When a container queries `db`, its local agent's DNS resolves it to the actual container IP — which might be on a completely different host. The overlay network (WireGuard) carries the traffic there directly. Only healthy containers appear in DNS responses.

No Consul, no CoreDNS configuration, no `my-service.my-namespace.svc.cluster.local`. Just the service name.

### 13. Cross-Host Load Balancing

DNS handles container-to-container traffic within the overlay. But for **published ports** (external traffic hitting a host), Banyan writes iptables DNAT rules on every agent — the same probability-based approach Kubernetes' kube-proxy uses. The Linux kernel handles all packet forwarding; no userspace proxy in the path.

You set `replicas: 3` and traffic spreads across all three, regardless of which servers they're on.

### 14. Zero-Downtime Deployment

Banyan defaults to **blue-green** deployment. Run `banyan up` again and new containers start alongside old ones (no port conflicts — iptables handles the mapping). Once the new deployment is healthy, the old one is torn down. If the new deployment fails, the old one stays running.

No strategy flags, no rollout configuration. You run the same command; blue-green happens internally.

### 15. Security

All gRPC traffic (engine ↔ agents ↔ CLI) is encrypted through a dedicated WireGuard control tunnel, separate from the data plane overlay. Authentication uses a public key whitelist — you paste a key during `banyan init`, and everything is encrypted and authenticated from there. No certificates to manage, no CA to operate.

### 16. Observability

A live terminal dashboard (`banyan-cli dashboard`) shows your cluster across six views: overview, agents, deployments, containers, engine metrics, and events. Updates every 5 seconds. No Grafana, no browser, no config files.

The engine also exposes a Prometheus-compatible metrics endpoint, so if you already run Prometheus, Banyan plugs right in.

---

## Part IV: Honest Assessment

### 17. When to Use Banyan

Banyan is designed for a specific situation:

- **2–30 engineers** who've outgrown a single server but don't have (or want) a platform team
- **Same people write code and deploy** — no separate DevOps/SRE team
- **Already know Docker Compose** — it's what you use for development
- **Self-hosted infrastructure** — your own servers, VPS, or bare metal
- **Standard web workloads** — APIs, web apps, databases, background workers, caches

If that sounds like your team, Banyan cuts out the weeks of learning and infrastructure setup Kubernetes requires, while giving you the things you actually need: multi-host deployment, service discovery, zero-downtime deploys, encrypted networking, and load balancing.

### 18. When NOT to Use Banyan

Banyan is not the right tool for everything. Honestly:

**Use Kubernetes when:**
- You have **50+ microservices and multiple teams** — K8s namespace isolation, RBAC, network policies, and resource quotas exist for good reason
- You need **AI/ML with GPU scheduling** — K8s has native GPU support, KubeFlow, Ray, custom schedulers
- You have **regulatory compliance requirements** — SOC 2, HIPAA, PCI-DSS need RBAC, audit logging, pod security standards
- You need **multi-cloud portability** — K8s gives you a standardized API across AWS, GCP, Azure, on-prem

**Use Docker Compose when:**
- A single server is enough. Don't add orchestration you don't need.

**Use ECS/Fargate when:**
- You're all-in on AWS with no plans to leave.

### 19. Current Limitations

We'd rather be upfront about what Banyan can't do yet than have you find out the hard way.

- **No volume support yet.** Persistent storage isn't implemented. For now, stateful services like databases can use placement constraints to pin to a specific agent, or use external managed databases. Distributed volume support is on the roadmap.
- **No autoscaling.** Replica counts are manual. Metric-based scaling is on the roadmap.
- **Single engine = single point of failure.** If the engine goes down, running containers keep going (they're independent containerd processes), but no new deploys or rescheduling happen until it's back. Multi-engine HA is planned.
- **No access control beyond key whitelisting.** ABAC (attribute-based access control) is coming.
- **No secrets management yet.** Environment variables work the same way as Docker Compose (in the manifest YAML, or via `env_file`), but there's no encrypted secrets store like Docker Swarm secrets or Vault. Built-in secrets management is on the roadmap.
- **L4 proxy only.** iptables handles TCP/UDP forwarding. Path-based routing, TLS termination, header routing need an external reverse proxy (Nginx, Traefik).
- **No session affinity.** Traffic distributes randomly. Sticky sessions are planned.
- **No network policies.** All containers in the overlay can reach all others. Segmentation is planned.
- **Not yet production-ready.** We say this on the website too. Banyan is under active development.

### 20. Roadmap

**Near term:**
- Rootless container mode
- Health-based scheduling (CPU/memory-aware placement)
- Built-in secrets management

**Medium term:**
- Multi-engine HA (eliminate the single point of failure)
- Persistent volume support (local volumes with placement-aware scheduling, and distributed storage)
- Autoscaling based on resource utilization
- Web monitoring dashboard

**Long term:**
- Advanced security (ABAC, certificate rotation, audit logging)
- Advanced networking (L7 ingress, session affinity, network policies)
- Dynamic workload rebalancing

---

## Try Banyan

If you'd like to try Banyan, head to [getbanyan.dev](https://getbanyan.dev) — installation, quickstart, and documentation are all there.

---

## The Complexity Budget

Every orchestration tool deals with real complexity — networking, service discovery, load balancing, state management, security. The question isn't whether complexity exists. It's **who deals with it.**

| What You Do | What Actually Happens |
|---|---|
| Write a Docker Compose file | Manifest parsed, validated, tasks scheduled round-robin across agents |
| Run `banyan up` | Blue-green: new containers alongside old, proxy rules updated, health confirmed, old torn down |
| Your app connects to `db` | DNS search domain `.internal` → agent-local DNS → in-memory store populated by heartbeat |
| Traffic crosses hosts | Bridge → WireGuard tunnel → encrypted → remote bridge → destination container |
| Traffic hits a replicated service | iptables DNAT with probability-based rules, kernel handles forwarding |
| You add a server | Install agent, init, whitelist key, start. Your manifest doesn't change. |
| You scale to 5 replicas | Change `replicas: 5`, run `up`. Distributed automatically. |
| You check cluster status | `banyan-cli dashboard` — live TUI, six views, updated every 5 seconds |
| You read logs | `banyan-cli logs` — engine proxies to the right agent, streams live |

The right column is real. Banyan doesn't eliminate it. It **absorbs** it — so you get the simplicity on the left.

---

## References

### Industry Data
- CNCF 2025 Annual Cloud Native Survey — [cncf.io](https://www.cncf.io/announcements/2026/01/20/kubernetes-established-as-the-de-facto-operating-system-for-ai-as-production-use-hits-82-in-2025-cncf-annual-survey/)
- Kubernetes Adoption by Company Size — [Jeevi Academy](https://www.jeeviacademy.com/kubernetes-adoption-statistics-and-trends-for-2025/)
- K8s Engineer Salaries Q2 2025 — [kube.careers](https://kube.careers/state-of-kubernetes-jobs-2025-q2)
- Platform Engineer Salaries Q1 2025 — [kube.careers](https://kube.careers/state-of-kubernetes-jobs-2025-q1)
- K8s Total Cost of Ownership — [Koyeb](https://www.koyeb.com/blog/the-true-cost-of-kubernetes-people-time-and-productivity)
- K8s Skill Shortages — [Tigera](https://www.tigera.io/learn/guides/kubernetes-security/kubernetes-statistics/)
- YAML Misconfiguration & Cultural Challenges — [CNCF 2025](https://www.cncf.io/blog/2026/01/20/kubernetes-fuels-ai-growth-organizational-culture-remains-the-decisive-factor/)
- Container Usage & Docker Ecosystem — [Docker State of App Dev 2025](https://www.docker.com/blog/2025-docker-state-of-app-dev/)
- Cloud-Native Developer Population — [CNCF/SlashData](https://www.cncf.io/announcements/2025/11/11/cncf-and-slashdata-survey-finds-cloud-native-ecosystem-surges-to-15-6m-developers/)

### Competitor Analysis
- Docker Swarm Long-Term Support — [Mirantis](https://www.mirantis.com/blog/mirantis-guarantees-long-term-support-for-swarm/)
- Swarm Compose v3 Limitation — [Docker Docs](https://docs.docker.com/engine/swarm/stack-deploy/)
- IPsec Overlay Performance Issue — [moby/moby #33133](https://github.com/moby/moby/issues/33133)
- HashiCorp BSL License Change — [HashiCorp](https://www.hashicorp.com/en/blog/hashicorp-adopts-business-source-license)
- IBM/HashiCorp Acquisition — [IBM](https://newsroom.ibm.com/2025-02-27-ibm-completes-acquisition-of-hashicorp,-creates-comprehensive,-end-to-end-hybrid-cloud-platform)
- MicroK8s — [Canonical](https://canonical.com/microk8s)
- Talos Linux — [talos.dev](https://www.talos.dev/)
- Omni License — [GitHub](https://github.com/siderolabs/omni)
- ECS/Fargate Pricing — [CloudBurn](https://cloudburn.io/blog/amazon-ecs-pricing)
- K3s — [k3s.io](https://docs.k3s.io/)

### Banyan
- Website — [getbanyan.dev](https://getbanyan.dev)
- Documentation — [getbanyan.dev/docs](https://getbanyan.dev/docs)
- License — Apache 2.0

---

*Banyan is open source under the Apache 2.0 license. Built with containerd, nerdctl, etcd, WireGuard, and Go.*
