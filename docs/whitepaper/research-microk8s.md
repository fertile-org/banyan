# MicroK8s Deep Research (2025-2026)

Deep, unbiased analysis for Banyan white paper. MicroK8s claims to simplify Kubernetes — but does it simplify setup or usage?

---

## 1. Current State (2025-2026)

### Latest Version and Release Cadence

MicroK8s tracks upstream Kubernetes releases closely, publishing beta, RC, and final builds on the same day as upstream. Latest stable: **1.33** (January 27, 2026). Also maintains 1.32 and 1.31 on separate snap channels. Follows K8s three-releases-per-year cadence.

Recent improvements: dqlite datastore (concurrent queries, OpenTelemetry), updated addons (ingress-nginx 1.12.1, cert-manager v1.17.1), iptables/CNI fixes.

### Canonical's Commitment

- **12-year LTS**: Starting with K8s 1.32 (Feb 2025), Canonical provides 12-year long-term support. With Ubuntu Pro Legacy add-on, extends to **15 years** (to 2040)
- **Business model**: MicroK8s is free/open-source. Revenue via Ubuntu Pro subscriptions (Kernel Livepatch, FIPS, 24/7 support, extended security maintenance)
- **Strategic importance**: Part of Canonical's full Ubuntu infrastructure stack strategy, alongside Charmed Kubernetes
- **FedRAMP compliance** support — serious enterprise/government market play

### Community and Adoption

- **GitHub**: ~9.2k stars, ~822 forks (K3s has ~28k+ stars — 3x community engagement)
- **CNCF certified**: Passes same conformance tests as GKE, EKS, AKS
- **Users**: TheirStack lists ~89 companies; Canonical doesn't prominently publish case studies
- **Distribution**: Available via snap (pre-installed on Ubuntu — the most popular server Linux)

### Position in Lightweight K8s Landscape

| Distribution | Primary Use Case | Installation | HA Support | Backed By |
|---|---|---|---|---|
| **MicroK8s** | Dev, edge, IoT, small prod | snap install | Yes (3+ nodes, dqlite) | Canonical |
| **K3s** | Edge, IoT, prod-grade | Single binary | Yes (embedded etcd or external DB) | SUSE/Rancher |
| **minikube** | Local dev/learning only | VM or container | No | Kubernetes SIG |
| **kind** | CI/testing | Docker containers | No | Kubernetes SIG |

K3s is more widely adopted for production. MicroK8s stronger in Ubuntu ecosystem and IoT/edge.

---

## 2. What MicroK8s Actually Simplifies

### Installation — Genuinely Simple

```bash
sudo snap install microk8s --classic
```

One command installs a complete single-node K8s cluster:
- kube-apiserver, kube-controller-manager, kube-scheduler (control plane)
- kubelet, kube-proxy (node components)
- containerd (runtime)
- dqlite (lightweight distributed SQLite — replaces etcd for HA)
- Calico CNI (default, VXLAN backend)
- Built-in `kubectl` via `microk8s kubectl`

Compare to kubeadm: separately install container runtime, configure kubelet, run `kubeadm init`, set up CNI plugin, configure kubectl. MicroK8s collapses all into a single snap.

### The Addon System — Genuine Differentiator

Instead of manually installing K8s ecosystem tools, users run single commands:

**Core Addons (Canonical-maintained):**
- `dns` — CoreDNS
- `dashboard` — Kubernetes Dashboard
- `ingress` — Ingress controller (Traefik in 1.35+, previously NGINX)
- `storage` / `hostpath-storage` — Host directory storage class
- `registry` — Private container registry on localhost:32000
- `metrics-server` — K8s Metrics Server
- `prometheus` — Prometheus operator
- `gpu` — NVIDIA CUDA enablement
- `metallb` — Load balancer for bare metal
- `helm3` — Helm 3 package manager
- `rbac` — Role-Based Access Control
- `ha-cluster` — HA configuration with dqlite
- `cert-manager` — TLS certificate management
- `mayastor` — OpenEBS MayaStor storage

**Community Addons:**
- `istio`, `portainer`, `traefik`, `jaeger`, `knative`, `linkerd`, `multus`, `cilium`, `openebs`

Real simplification: setting up Prometheus on vanilla K8s requires choosing Helm charts, configuring storage, setting up ServiceMonitors, creating Ingress rules. With MicroK8s: `microk8s enable prometheus`. One command.

### HA Clustering

Automatic HA with 3+ nodes:
```bash
# On first node:
microk8s add-node
# On second/third node:
microk8s join <token>
```

Uses dqlite (distributed SQLite) instead of etcd. All nodes run control plane. Voter promotion happens automatically within ~30 seconds.

**However**, dqlite is less mature than etcd:
- Unexpected high CPU after 2+ years
- Consensus failures with even node counts after network splits
- Less tooling and community knowledge

### Kubernetes Version Upgrades

```bash
sudo snap refresh microk8s --channel=1.33/stable
```

Simpler than kubeadm upgrades (drain, upgrade kubeadm/kubelet/kubectl, uncordon). **However:**
- Skip-level upgrades NOT supported (must upgrade one minor at a time)
- Addons must be disabled/re-enabled during upgrades
- Backup functionality reported broken in certain versions
- Snap auto-refresh can break clusters (detailed below)

---

## 3. What MicroK8s Does NOT Simplify — The Key Question

MicroK8s simplifies Day 0 (installation/setup). It provides wrappers for Day 1 (enabling addons). But it does **almost nothing** to simplify the core Kubernetes experience for application deployment.

### Users Still Need Kubernetes Manifests

To deploy on MicroK8s, users write the same K8s YAML as any other distribution:

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

**45 lines across 3 resource types.** Compare to Docker Compose achieving the same:

```yaml
services:
  my-app:
    image: my-app:latest
    ports:
      - "80:8080"
    deploy:
      replicas: 3
```

**7 lines. MicroK8s does not bridge this gap at all.**

### Users Still Need Helm Charts

`microk8s enable helm3` makes Helm available, but users still:
- Find and evaluate Helm charts
- Understand `values.yaml` customization
- Debug templated YAML
- Manage chart versions and dependencies

### Users Still Need K8s Networking Knowledge

MicroK8s simplifies enabling components (`microk8s enable ingress`, `microk8s enable metallb`), but users still need to understand:
- Services: ClusterIP vs NodePort vs LoadBalancer
- Ingress rules, TLS termination, path-based routing
- NetworkPolicies (must write them manually)
- K8s DNS (`my-service.my-namespace.svc.cluster.local`)
- CNI concepts if default Calico doesn't work

### Users Still Need to Manage Security

- **RBAC**: Disabled by default (`--authorization-mode=AlwaysAllow`) — actually *less* secure by default than most K8s distributions
- Pod Security Standards: manual configuration
- NetworkPolicies: manual, no defaults
- Secrets: standard K8s (base64, not encrypted at rest by default)

### Day-2 Operations Not Reduced

- **Monitoring**: `microk8s enable prometheus` gets it running, but alerts/dashboards/retention still manual
- **Logging**: No built-in centralized logging
- **Backup**: Built-in backup has had reliability issues; persistent volumes separate
- **Scaling**: HPA requires metrics-server + writing HPA manifests
- **Upgrades**: One minor version at a time; addons require disable/re-enable
- **Troubleshooting**: Same kubectl describe/logs/get events workflow

### Cognitive Load for a K8s-Naive Developer

A Docker Compose developer would need to learn:
1. Pods — basic scheduling unit
2. Deployments — declarative pod management
3. Services — network abstraction (ClusterIP, NodePort, LoadBalancer)
4. Ingress — HTTP routing
5. ConfigMaps and Secrets — configuration
6. Persistent Volumes and Claims — storage
7. Namespaces — logical isolation
8. Labels and Selectors — resource referencing
9. kubectl — entirely new CLI
10. YAML structure — apiVersion/kind/metadata/spec pattern

**Same learning curve as any K8s distribution.** Docker Compose: 1-2 days. Kubernetes: weeks to months.

---

## 4. Real User Experiences

### What Users Like

- Installation speed — fastest path to running K8s on Ubuntu
- Addon system — reduces infrastructure setup toil
- Good learning tool for K8s
- Single-node development works well on Ubuntu Desktop
- Automatic security updates via snap
- Well-maintained documentation

### Common Pain Points

#### Snap Distribution Issues (Major)

Most contentious aspect:

- **Auto-refresh breaks clusters**: Snap refreshes every 6 hours by default. Multiple production users report cluster breakage: auto-refresh failing mid-update on 3 of 4 workers, hanging at "Copy snap data", cluster falling over. Another case: duplicate pods conflicting on host ports.
- **Non-Ubuntu Linux**: Requires snap, which needs manual installation on RHEL/Fedora/Debian
- **Strict confinement issues**: AppArmor denials and CrashLoopBackOff on Ubuntu 25.04
- **Mitigation**: Users can hold refreshes (`sudo snap refresh --hold microk8s`) — but must be explicitly configured, not default

#### dqlite Reliability

- Less mature than etcd, not as battle-tested
- High CPU after long-running clusters
- Consensus issues after network partitions with even node counts
- Recovery tools depend on API server being available — circular dependency

#### Resource Usage

Idle cluster, single node benchmarks:
- **MicroK8s**: ~540-685MB memory, **8.83% CPU** on 2 cores
- **K3s**: ~512MB memory, **3.77% CPU** on 2 cores
- **Vanilla K8s**: higher memory, 4.27% CPU

MicroK8s has notably **higher CPU** than K3s. With addons (dns, RBAC, hostpath-storage, metrics-server): ~335MB per worker. Users on 1GB instances report system freezes.

#### Limited Production Track Record

- G2 reviewers describe it as better for learning than production
- Few publicly named enterprise production users
- Community consensus leans toward K3s for lightweight production

---

## 5. MicroK8s vs Banyan — The Core Difference

### The Fundamental Distinction

| Aspect | MicroK8s | Banyan |
|---|---|---|
| **What it simplifies** | K8s cluster SETUP | The ENTIRE deployment experience |
| **What users write** | K8s manifests (Deployments, Services, Ingress) | Docker Compose files |
| **Prerequisite knowledge** | K8s concepts, kubectl, YAML API | Docker Compose syntax |
| **Abstraction level** | Full K8s API exposed | Three concepts: engine, agent, manifest |
| **Networking** | Services, ClusterIP, NodePort, Ingress, CNI | Built-in overlay, automatic DNS, cross-host LB |
| **Learning curve** | Weeks to months | Hours (if you know Docker Compose) |

### The Concrete Example

A team of 5 engineers deploying a web app + database + Redis across 3 servers:

**With MicroK8s:**
1. Install MicroK8s on 3 nodes (simple)
2. Join into cluster (simple)
3. Enable addons: dns, ingress, storage (simple)
4. **Learn K8s concepts**: Pods, Deployments, Services, Ingress, PVCs, ConfigMaps, Secrets
5. **Write Deployment manifests** for web, database, Redis (3 files, ~150 lines YAML)
6. **Write Service manifests** for each (3 files, ~60 lines)
7. **Write Ingress manifest** (~20 lines)
8. **Write PVC** for database (~15 lines)
9. **Write ConfigMaps/Secrets** (~20 lines)
10. Apply with `kubectl apply -f`
11. Debug with `kubectl describe/logs/get events`
12. Set up monitoring, logging, backup separately

Steps 1-3 simplified. Steps 4-12 = standard K8s complexity, unchanged.

**With Banyan:**
1. Install engine on 1 node, agent on 3 nodes
2. Write Docker Compose file (~20 lines, syntax already known)
3. Run `banyan up`

### Target Audience Overlap

**Minimal overlap:**
- **MicroK8s**: Teams that have decided to use K8s and want easier installation. Accept K8s learning curve, want full ecosystem.
- **Banyan**: Teams that explicitly do NOT want K8s. Want Docker Compose semantics at multi-server scale.

Overlap exists only for teams evaluating whether to adopt K8s at all. MicroK8s = "easiest on-ramp to K8s." Banyan = "you don't need K8s at all."

---

## 6. MicroK8s vs K3s

| Aspect | MicroK8s | K3s |
|---|---|---|
| **Packaging** | Snap package (~180MB) | Single binary (~65MB) |
| **Datastore** | dqlite (HA) or etcd (single) | SQLite, embedded etcd, or external DB |
| **Runtime** | containerd | containerd |
| **Default CNI** | Calico (VXLAN) | Flannel (VXLAN) |
| **Linux support** | Ubuntu native; others via snap | Any Linux distribution |
| **Idle memory** | ~540-685MB | ~512MB |
| **Idle CPU** | ~8.83% (2 cores) | ~3.77% (2 cores) |
| **Backed by** | Canonical | SUSE/Rancher |
| **GitHub stars** | ~9.2k | ~28k+ |
| **LTS** | 12-15 years | Community + SUSE commercial |

**Choose MicroK8s when**: Ubuntu native, want addon system, need Canonical enterprise support, IoT/edge on Ubuntu Core, want 12-15 year LTS.

**Choose K3s when**: Non-Ubuntu Linux, need lowest resources (especially CPU), want single-binary, prefer larger community, need production-grade lightweight K8s.

### Are They Solving the Right Problem?

Both solve **making K8s easier to install and operate**. Neither solves **making K8s easier to use for application deployment**. A team on K3s or MicroK8s still writes the same manifests, needs Helm, debugs with kubectl, manages RBAC.

The "lightweight K8s" category assumes the problem is K8s being too heavy to install. For many small teams, the problem is K8s being **too complex to use** — regardless of installation ease. Teams "end up drowning in YAML debt before shipping real value."

**MicroK8s and K3s make it easier to get Kubernetes running. They do not make it easier to be a Kubernetes user.**

---

## Summary Assessment

**What MicroK8s genuinely does well:**
- Single-command installation on Ubuntu
- Addon system for infrastructure components
- Automatic HA with 3+ nodes
- Snap-channel K8s version tracking
- 12-15 year LTS commitment
- Good learning/development tool

**What MicroK8s does not change:**
- Users still write K8s manifests (Deployments, Services, Ingress)
- Users still need Helm charts
- Users still need K8s networking knowledge
- Users still need to manage RBAC, security, secrets
- Users still debug with kubectl
- Day-2 operations unchanged

**Honest assessment**: MicroK8s reduces installation friction to near-zero on Ubuntu. Its addon system provides genuine value. Canonical's commitment is credible. But it is **fundamentally still Kubernetes** — with all the conceptual complexity, YAML verbosity, and operational burden that entails.

For teams whose pain point is "K8s is hard to install" → MicroK8s is excellent.
For teams whose pain point is "K8s is hard to use" → MicroK8s offers no relief.

The snap distribution introduces its own risks (auto-refresh breaking clusters) that partially offset simplicity gains.

---

## Sources

- [MicroK8s Release Notes - Canonical](https://canonical.com/microk8s/docs/release-notes)
- [MicroK8s GitHub](https://github.com/canonical/microk8s)
- [Canonical 12-Year LTS for K8s](https://canonical.com/blog/12-year-lts-for-kubernetes)
- [Canonical 15-Year Extended Support - The New Stack](https://thenewstack.io/canonical-extends-kubernetes-long-term-support-to-15-years/)
- [MicroK8s Getting Started](https://canonical.com/microk8s/docs/getting-started)
- [MicroK8s Addons](https://microk8s.io/docs/addons)
- [MicroK8s HA](https://microk8s.io/docs/high-availability)
- [MicroK8s Upgrading](https://microk8s.io/docs/upgrading)
- [Snap Auto-Refresh Issue #1022](https://github.com/canonical/microk8s/issues/1022)
- [Snap Refresh Pod Issue #4691](https://github.com/canonical/microk8s/issues/4691)
- [Strict Confinement Issue #5140](https://github.com/canonical/microk8s/issues/5140)
- [Ubuntu Blog: MicroK8s Memory Optimization](https://ubuntu.com/blog/microk8s-memory-optimisation)
- [Portainer Resource Comparison: K0s vs K3s](https://www.portainer.io/blog/k0s-vs-k3s)
- [Switching to MicroK8s 6 Months Later](https://benbrougher.tech/posts/microk8s-6-months-later/)
- [K8s Cluster Meltdown Lessons](https://www.gocoder.one/blog/lessons-learned-from-a-home-kubernetes-cluster-collapse/)
- [IT Pro Today: Lightweight K8s Showdown](https://www.itprotoday.com/edge-computing/lightweight-kubernetes-showdown-minikube-vs-k3s-vs-microk8s)
- [Canonical Compare Page](https://canonical.com/microk8s/compare)
- [G2 MicroK8s Reviews](https://www.g2.com/products/canonical-microk8s/reviews)
- [CNCF Helm Charts on MicroK8s](https://www.cncf.io/blog/2021/03/23/quick-application-deployments-on-microk8s-using-helm-charts/)
- [MicroK8s CIS Hardening](https://microk8s.io/docs/how-to-cis-harden)
- [DataCamp: Docker Compose vs K8s](https://www.datacamp.com/blog/docker-compose-vs-kubernetes)
- [K8s Is Overkill - DEV.to](https://dev.to/anderson_leite/kubernetes-overkill-when-your-architecture-is-more-complex-than-your-business-17gn)
- [Companies Ditching K8s - ByteIota](https://byteiota.com/kubernetes-is-overkill-why-companies-are-ditching-k8s/)
- [93% of Enterprise Teams Struggle - DEVOPSdigest](https://www.devopsdigest.com/ai-and-kubernetes-challenges-93-of-enterprise-platform-teams-struggle-with-complexity-and-costs)
