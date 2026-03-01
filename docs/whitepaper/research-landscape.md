# Container Orchestration Landscape Analysis (2025-2026)

Research for Banyan white paper. All statistics sourced and linked.

---

## 1. Kubernetes: The 800-Pound Gorilla

### Current State

Kubernetes dominates container orchestration. The CNCF 2025 Annual Survey reports production usage at **82%** across surveyed organizations. Over 90% of containerized organizations use or actively evaluate it. The cloud-native developer ecosystem has surged to 15.6 million developers.

### Where Kubernetes Genuinely Excels

It would be dishonest to dismiss Kubernetes. It is the right tool for many situations:

- **Large-scale, multi-tenant workloads.** Running hundreds of microservices across dozens of teams — nothing else comes close in maturity.
- **AI/ML workloads.** Kubernetes has become the de facto operating system for AI, with native support for GPU scheduling, KubeFlow, Ray, and custom schedulers for training/inference jobs.
- **Multi-cloud portability.** 65% of organizations run Kubernetes in multiple environments. The API is standardized; skills and configs transfer between clouds.
- **Self-healing and autoscaling.** Declarative state reconciliation, automatic pod rescheduling, and horizontal/vertical autoscaling are battle-tested at massive scale.
- **Ecosystem depth.** Prometheus, Grafana, ArgoCD, Flux, Istio, Linkerd, cert-manager — the tooling ecosystem is unmatched. If you need a capability, there is likely a mature project for it.
- **Enterprise features.** RBAC, network policies, pod security standards, namespace isolation, audit logging — the security and governance surface area is comprehensive.

### Real Pain Points

#### YAML Complexity and the "YAML Engineer" Phenomenon

A 2025 CNCF report found that **79% of Kubernetes production outages could be traced back to YAML misconfigurations**. Whitespace-sensitive indentation errors, implicit type coercion, and complex template integration are constant sources of operational friction. Teams often have only one or two engineers who truly understand the full Kubernetes setup, creating a dangerous knowledge silo. The community has even proposed KYAML, a safer YAML subset, as an acknowledgment that YAML itself is a problem.

#### CRD and Operator Proliferation

While the operator pattern is powerful, it creates real administrative burden. Each operator introduces its own CRDs, reconciliation loops, upgrade cycles, and compatibility matrices. Cluster administrators face a situation where listing all applications running on a cluster requires checking multiple operators and Helm releases — no single view exists. A small platform team can easily find itself maintaining hundreds of Helm charts, dozens of operators, and a labyrinthine set of CRDs before a single product feature is written.

#### Networking Complexity (CNI Choices)

The CNI decision alone is a minefield. As one practitioner put it: "Start with what your team can handle operationally, not what has the best benchmarks. Perfect networks fail because teams couldn't operate them." Cilium offers eBPF-based performance but requires specialized debugging skills. Calico needs BGP knowledge. Flannel is simple but lacks network policy support, making it unsuitable for production security requirements.

#### Day-2 Operations Burden

Day-2 operations — upgrades, security patching, cost optimization, observability, backup/restore — is where complexity compounds. Teams end up running service mesh, ingress controllers, GitOps operators, security scanners, backup tools, monitoring stacks, each with its own upgrade cycle and compatibility matrix. For 90% of teams, this adds unnecessary complexity compared to simpler alternatives.

#### Minimum Team Size and Expertise

The salary data tells the story:
- Average Kubernetes engineer salary in Q2 2025: **$166,836**
- Platform engineers average: **$172,038**, with 85% being senior-level roles
- Koyeb estimates total cost of self-hosted K8s: at least **$100K**, easily exceeding **$500K annually**, even for modest resources
- A team of ~4 engineers needed for 24/7 coverage — before infrastructure costs

#### Adoption by Company Size

- 34% of K8s users: companies with 20,000+ employees
- 34% of K8s users: companies with 1,000-5,000 employees
- **Only 9%** are in companies with under 1,000 employees
- Skill shortages impact 75% of organizations

### Do Managed Services (EKS, GKE, AKS) Solve It?

Partially. GKE Autopilot comes closest to reducing operational burden — no node management, auto-addons — and is considered the lowest-complexity option. AKS is moderate but has verbose CLI and occasional disruptive upgrades. EKS has the highest complexity with manual multi-tenancy and complex IAM policies.

**Critical insight: Managed services handle infrastructure-layer complexity; they do not handle application-layer Kubernetes complexity.** You still write the Deployments, Services, Ingresses, ConfigMaps, Secrets, NetworkPolicies, HPA configs, PDB configs, and Helm charts. You still choose and operate your CNI, service mesh, observability stack, and GitOps tooling.

### The 2025 CNCF Survey's Revealing Shift

In the 2025 survey, **complexity dropped to 34% as a cited challenge** (down from prior years). But the primary challenge shifted to "cultural changes with the development team" at **47%**. This does not mean complexity is solved — it means the organizations that survived the complexity gauntlet now face organizational friction. The teams that could not handle the complexity already left or never adopted.

---

## 2. Docker Swarm: The Road Not Taken

### Current State

Docker Swarm is **not deprecated**. Mirantis committed to long-term Swarm support through 2030 after acquiring Docker Enterprise. Enterprise customers include MetLife, Royal Bank of Canada, S&P Global, federal agencies, and financial services providers.

However, development pace has slowed considerably. It is maintained, not actively evolved.

### What It Got Right

- **Setup in hours, not days.** Uses familiar Docker commands; developers can manage orchestration without learning a new conceptual model.
- **Native Docker integration.** No additional orchestration software needed. Works out of the box with existing Docker pipelines.
- **Genuinely simple multi-host clustering.** `docker swarm init` and `docker swarm join` — straightforward mental model.

### What It Got Wrong / Why Teams Leave

- **Limited advanced features.** Struggles in hybrid and multi-cloud environments.
- **Weak ecosystem.** Limited integration with external tools and plugins. The K8s ecosystem has no Swarm equivalent.
- **Less robust automation.** Self-healing and autoscaling not as mature as Kubernetes.
- **Declining community.** As K8s became the industry standard, community involvement and support declined, creating a negative feedback loop.
- **Perception problem.** Even if Swarm meets technical needs, hiring managers and engineers see it as a dead-end skill.

---

## 3. HashiCorp Nomad: Simpler, But Uncertain Future

### Current State

Nomad is actively developed under IBM/HashiCorp, with recent releases adding system job deployments, secret blocks for job specs, and GPU/MIG orchestration. It remains a production-grade orchestrator.

### The BSL License Change

In August 2023, HashiCorp switched from MPL 2.0 to the Business Source License (BSL) for all products including Nomad. End users can still copy, modify, and redistribute for all use except creating competitive offerings. Code transitions to open source after 4 years.

### IBM Acquisition

IBM completed its $6.4 billion acquisition of HashiCorp in February 2025. The strategic focus appears to be on Terraform and Vault integration with Red Hat's portfolio. Industry analysts note that resources may be diverted away from products like Nomad.

### Complexity vs. Kubernetes

Nomad's primary advantage is operational simplicity. A small team can effectively manage a Nomad cluster with basic systems administration knowledge, whereas K8s typically requires specialized expertise. Nomad supports a wider range of workloads beyond containers (VMs, Java apps, batch jobs), and teams can launch clusters in minutes.

### Risks

- BSL license uncertainty for companies that might build competitive products
- IBM's strategic priorities may not align with Nomad's continued evolution
- Smaller ecosystem than K8s (fewer integrations, smaller hiring pool)
- Neither open source nor proprietary — awkward middle ground

---

## 4. AWS ECS / Fargate: Simplicity at the Cost of Freedom

### Vendor Lock-in

ECS uses proprietary APIs — task definitions, services, and clusters are AWS-specific concepts. Migrating from ECS requires rewriting task definitions as Kubernetes manifests, restructuring IAM roles, and updating deployment pipelines. In contrast, EKS applications are generally portable to any Kubernetes environment.

### Complexity for Small Teams

For teams already on AWS, ECS can be an excellent choice because it abstracts away cluster management. AWS recently launched ECS Express Mode to further simplify deployments. However, the underlying billing is complex — multiple pricing dimensions for compute, storage, and networking make cost prediction difficult.

### Pricing Issues

Fargate's convenience comes with a 20-30% price premium over well-managed EC2. Even with zero traffic, an Application Load Balancer costs $16-25/month. The trade-off: Fargate reduces operational overhead by approximately 50% compared to EC2-based setups, which may justify the premium for small teams.

### Bottom Line

ECS/Fargate is a reasonable choice for small teams already committed to AWS that do not anticipate needing multi-cloud portability. It trades long-term flexibility for short-term simplicity.

---

## 5. Kamal (37signals/Basecamp): Just Enough Orchestration

### What Problem It Solves

Kamal was created by 37signals to deploy Basecamp, HEY, and other apps without cloud vendor dependency. It is "Capistrano for containers" — an imperative deployment tool that uses SSH to push Docker containers to bare-metal or VPS servers. Zero-downtime deploys, rolling restarts, and remote builds are built in.

### Philosophy Difference

Kamal takes a fundamentally different approach. It is intentionally imperative, not declarative. You specify a sequence of commands that creates the end state, rather than declaring the end state and letting a reconciliation loop maintain it. There is no running daemon — deployments are push-based.

37signals advocates over-provisioning for traffic spikes rather than autoscaling, arguing that the baseline cost savings from moving to Hetzner-class providers make over-provisioning cheaper than autoscaling infrastructure complexity.

### Limitations

- **No multi-server load balancing.** Horizontal scaling requires an external load balancer in front.
- **No service discovery.** Containers on the same server cannot communicate between themselves by design. Cross-service communication requires manual configuration.
- **No autoscaling.** Since there is no daemon, autoscaling is impossible.
- **No health-based orchestration.** If a container crashes, no automatic restart or rescheduling to a healthy node.
- **Ruby-centric ecosystem.** Built for Rails apps; community, tooling, and docs skew heavily Ruby/Rails.
- **SSL not included by default.** Must set up your own SSL termination.

---

## 6. Docker Compose in Production: The Comfortable Trap

### Why Teams Use It

Container usage hit 92% among IT professionals in 2025. For many, Docker Compose is the first orchestration tool they learn and the one they're most comfortable with. For smaller-scale applications, personal projects, small business apps, or internal tools, Docker Compose in production can genuinely be sufficient.

### Where It Breaks Down

- **Single-host limitation.** Designed for a single host. If that machine goes down, all services crash. No failover, no rescheduling, no HA.
- **No cross-host scaling.** Can scale replicas on one host, cannot distribute them across servers.
- **No autoscaling.** Manual intervention required for dynamic workloads.
- **Limited service discovery.** Lacks advanced service discovery for registration, routing, or geographic distribution.
- **No rolling updates across hosts.** Zero-downtime deploys across multiple hosts are not supported.

### The Gap

**The gap between `docker compose up` and "production" is not a single missing feature — it is a bundle of capabilities teams need at once.** Multi-host distribution, service discovery, health-based rescheduling, zero-downtime deploys, cross-host networking, and load balancing. You cannot add these incrementally to Docker Compose; you need an orchestration layer. And historically, the only real option has been Kubernetes.

---

## 7. Other Alternatives

### Self-Hosted PaaS: Coolify, CapRover, Dokku

**Coolify:** Modern web dashboard, strong community (15,000+ GitHub stars by mid-2025). Currently lacks auto-scaling; Docker Swarm mode is experimental.

**CapRover:** Web GUI with relative simplicity. Apps are single-container only — significant limitation for multi-service applications.

**Dokku:** The lightest option, CLI-based. CI/CD, multi-service environments, autoscaling, and observability require custom plugins or manual setup.

**Common limitation:** These are fundamentally **single-server tools** with limited multi-host support. They solve "easy deploy" but not "scale beyond one server."

### Cloud PaaS: Railway, Render, Fly.io

**Railway:** Usage-based pricing ($5/month minimum + consumption). Fast spin-ups, instant deployment. Best for prototypes and lightweight apps.

**Render:** Free tier (limited, services spin down), paid plans from $7/month per service. Cost-friendly for simple apps. Horizontal autoscaling only on paid plans.

**Fly.io:** Global edge deployment with usage-based pricing. Strengths: global presence, latency-sensitive workloads. Limitations: no fully managed database, confusing bandwidth pricing.

**Common limitation:** Vendor lock-in (different kind than AWS, but still lock-in), pricing unpredictability at scale, limited control over infrastructure.

### K3s: Lightweight Kubernetes

K3s packages Kubernetes into a single <40MB binary with reduced memory footprint. Fully K8s-API compatible, production-grade, popular for edge/IoT.

**However:** K3s makes Kubernetes lighter to run, not simpler to use. You still write the same Deployments, Services, and Ingresses. You still need the same K8s expertise. K3s solves the resource overhead problem, not the cognitive complexity problem.

---

## 8. The "Missing Middle" — Key Analysis

### The Gap Is Real

| Tier | Solution | Team Size | Limitation |
|------|----------|-----------|------------|
| Single server | Docker Compose, Dokku, Coolify | 1-5 devs | Cannot scale beyond one host |
| Push-based deployment | Kamal | 2-10 devs | No orchestration, no service discovery, no auto-healing |
| Lightweight K8s | K3s | 5-15 devs with K8s knowledge | Same K8s cognitive complexity, just smaller binary |
| Full orchestration | Kubernetes | 10+ devs with dedicated platform team | Massive overhead for small teams |
| Managed PaaS | Railway, Render, Fly.io | 1-10 devs | Vendor lock-in, pricing surprises, limited control |
| AWS-native | ECS/Fargate | 2-15 devs on AWS | AWS lock-in, complex billing |

The gap is between **"single server with Docker Compose"** and **"full Kubernetes"**. Teams with 2-30 engineers who need multi-host deployment, service discovery, and health-based orchestration — but do not have the budget or expertise for a K8s platform team — have limited options.

### The Real Cost of Kubernetes for Small Teams

Koyeb's analysis: total cost of ownership at approximately **$569K annually** for self-hosted K8s with proper 24/7 coverage. Even with managed K8s at half that cost, the engineering time for YAML configuration, Helm chart maintenance, operator management, and day-2 operations represents significant investment small teams cannot afford.

For a team of 10 engineers total, dedicating 2-3 to platform work means **20-30% of engineering capacity goes to infrastructure** rather than product.

### What "Production-Ready" Actually Means for Small Teams

For small teams, production-ready does not mean the same thing as for enterprises. It means:

1. **Deployment confidence.** Can you deploy on a Friday afternoon without anxiety? Zero-downtime deploys?
2. **Basic high availability.** If one server dies, do services keep running?
3. **Service discovery.** Can services find and communicate with each other across hosts?
4. **Observability.** Can you see what's running, what's failing, and why?
5. **Reasonable security.** Encrypted communication, network isolation, access control.
6. **Operational simplicity.** Can the same engineers who write code also operate the deployment? Without a 3-month learning curve?

Kubernetes provides all of these and much more. The question is whether the "much more" is worth the cost for teams that only need the basics.

### The Deployment Confidence Gap

There is a psychological dimension. Teams running Docker Compose on a single server know their setup is fragile. They want to improve it but face a binary choice: stay with a known-fragile setup, or invest months learning Kubernetes. This creates a confidence gap — the distance between "I deployed successfully today" and "I am confident my system will stay running tomorrow."

The 2025 CNCF survey finding that organizational culture is now the top challenge (47%) reflects this gap. The technical solutions exist, but teams struggle to adopt them because the learning curve and operational overhead conflict with their primary job of building product.

---

## 9. When Each Tool Is the Right Choice

| Tool | Right choice when... | Wrong choice when... |
|------|---------------------|----------------------|
| **Kubernetes** | 50+ services, multi-team, dedicated platform eng, multi-cloud portability, AI/ML at scale | Small team (<15), no dedicated platform eng, few services, single cloud |
| **Managed K8s (GKE/EKS/AKS)** | Same as above but want to offload control plane ops | Same — managed does not eliminate application-layer complexity |
| **K3s** | Team already knows K8s, constrained resources, edge/IoT | Team that finds K8s concepts too complex (doesn't simplify the API) |
| **Docker Swarm** | Legacy systems that already run it, simple HA needs | New greenfield projects (limited investment and community) |
| **Nomad** | Multi-type workloads (containers + VMs + batch), prefer simplicity, already in HashiCorp ecosystem | Concerned about BSL license or IBM's long-term commitment |
| **ECS/Fargate** | All-in on AWS, want managed simplicity, small team | Need multi-cloud, concerned about vendor lock-in |
| **Kamal** | Rails/web apps, comfortable with push-based deploys, Hetzner-class providers | Need auto-healing, service discovery, autoscaling |
| **Docker Compose** | Single server is sufficient, dev/staging, internal tools | Multi-host, HA, any real production SLA |
| **Coolify/Dokku/CapRover** | Solo dev, single-server hobby/small projects | Multi-host scaling, complex multi-service architectures |
| **Railway/Render/Fly.io** | Prototypes, small SaaS, latency-sensitive global apps (Fly.io) | Cost-sensitive at scale, need infrastructure control, vendor independence |

---

## Sources

- [CNCF 2025 Annual Cloud Native Survey](https://www.cncf.io/announcements/2026/01/20/kubernetes-established-as-the-de-facto-operating-system-for-ai-as-production-use-hits-82-in-2025-cncf-annual-cloud-native-survey/)
- [CNCF - Organizational Culture as Decisive Factor](https://www.cncf.io/blog/2026/01/20/kubernetes-fuels-ai-growth-organizational-culture-remains-the-decisive-factor/)
- [CNCF - Cloud Native Ecosystem Surges to 15.6M Developers](https://www.cncf.io/announcements/2025/11/11/cncf-and-slashdata-survey-finds-cloud-native-ecosystem-surges-to-15-6m-developers/)
- [Kubernetes Adoption Statistics 2025 - Jeevi Academy](https://www.jeeviacademy.com/kubernetes-adoption-statistics-and-trends-for-2025/)
- [36 Kubernetes Statistics - Tigera](https://www.tigera.io/learn/guides/kubernetes-security/kubernetes-statistics/)
- [State of Kubernetes Jobs Q2 2025](https://kube.careers/state-of-kubernetes-jobs-2025-q2)
- [State of Kubernetes Jobs Q1 2025](https://kube.careers/state-of-kubernetes-jobs-2025-q1)
- [The True Cost of Kubernetes - Koyeb](https://www.koyeb.com/blog/the-true-cost-of-kubernetes-people-time-and-productivity)
- [Hidden Costs of Kubernetes at Scale - Upsun](https://upsun.com/blog/hidden-costs-of-running-kubernetes-at-scale/)
- [YAML Complexity and KYAML Proposal - The New Stack](https://thenewstack.io/kubernetes-is-getting-a-better-yaml/)
- [From YAML to Intelligence - CNCF](https://www.cncf.io/blog/2025/07/22/from-yaml-to-intelligence-the-evolution-of-platform-engineering/)
- [Kubernetes Operator Complexity - Rakuten Symphony](https://symphony.rakuten.com/blog/kubernetes-operators-maximizing-efficiency-or-adding-complexity)
- [Kubernetes Day-2 Operations - SUSE](https://www.suse.com/c/untangling-kubernetes-the-steep-climb-of-deployment-and-day-2-operations/)
- [CNI Performance Comparison 2025](https://sanj.dev/post/cilium-calico-flannel-cni-performance-comparison)
- [Cilium vs Calico vs Flannel - Civo](https://www.civo.com/blog/calico-vs-flannel-vs-cilium)
- [EKS vs GKE vs AKS 2026 - Atmosly](https://atmosly.com/blog/eks-vs-gke-vs-aks-which-managed-kubernetes-is-best-2025)
- [Docker Swarm in 2025 - Niksa Makitan](https://medium.com/@niksa.makitan/docker-swarm-in-2025-0d2f2bc5d929)
- [Mirantis Long-Term Swarm Support Through 2030](https://www.mirantis.com/blog/mirantis-guarantees-long-term-support-for-swarm/)
- [Docker Swarm vs Kubernetes - Portainer](https://www.portainer.io/blog/docker-swarm-vs-kubernetes)
- [Docker Swarm vs Kubernetes - IBM](https://www.ibm.com/cloud/blog/docker-swarm-vs-kubernetes-a-comparison)
- [HashiCorp BSL License Change](https://www.hashicorp.com/en/blog/hashicorp-adopts-business-source-license)
- [IBM Completes HashiCorp Acquisition](https://newsroom.ibm.com/2025-02-27-ibm-completes-acquisition-of-hashicorp,-creates-comprehensive,-end-to-end-hybrid-cloud-platform)
- [Nomad vs Kubernetes 2025](https://medium.com/@Krishnajlathi/kubernetes-vs-nomad-7-key-differences-that-matter-in-2025-41b7c042032e)
- [ECS Pricing Guide 2025 - Microtica](https://www.microtica.com/blog/ecs-pricing-guide)
- [Amazon ECS Pricing - CloudBurn](https://cloudburn.io/blog/amazon-ecs-pricing)
- [AWS ECS Express Mode - InfoQ](https://www.infoq.com/news/2025/12/aws-ecs-express-mode/)
- [Kamal Official Site](https://kamal-deploy.org/)
- [Kamal 2.0 Released - 37signals](https://dev.37signals.com/kamal-2/)
- [Kamal: Developer's Take - mkdev](https://mkdev.me/posts/thoughts-on-kamal-30)
- [Kamal: Just Enough Orchestration - Semaphore](https://semaphore.io/blog/mrsk)
- [Docker Compose Production Ready - Bunnyshell](https://www.bunnyshell.com/blog/is-docker-compose-production-ready/)
- [Docker State of App Dev 2025](https://www.docker.com/blog/2025-docker-state-of-app-dev/)
- [Coolify vs Dokku vs CapRover - CyberSnowden](https://cybersnowden.com/coolify-vs-dokku-vs-caprover-self-hosted-platform/)
- [Self-Hosted PaaS Comparison - KloudShift](https://kloudshift.net/blog/comparing-self-hostable-paas-solutions-caprover-coolify-dokploy-reviewed/)
- [Railway vs Render 2026 - Northflank](https://northflank.com/blog/railway-vs-render)
- [Fly.io Pricing 2025 - Orb](https://www.withorb.com/blog/flyio-pricing)
- [K3s Documentation](https://docs.k3s.io/)
- [K3s vs K8s - Spacelift](https://spacelift.io/blog/k3s-vs-k8s)
- [Production Readiness Checklist - Cortex](https://www.cortex.io/post/how-to-create-a-great-production-readiness-checklist)
