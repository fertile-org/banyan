---
name: user-docs
description: Write user-facing documentation for Banyan that speaks to the target audience — teams who want container orchestration without Kubernetes complexity. Use when creating, updating, or reviewing any user documentation, guides, quickstarts, or reference pages. Triggers on "write docs", "update docs", "user guide", "documentation for users", "doc review", or any work in website/.
---

# Banyan User-Oriented Documentation

Write documentation that makes Banyan's target users feel understood, confident, and productive. Every doc page should reinforce why they chose Banyan over Kubernetes.

## Before Writing: UX Audit (mandatory)

**Before documenting any feature, you MUST evaluate the feature's UX from the end-user perspective.**

This is the most critical step. Good documentation cannot fix bad UX. If you encounter any of the following, **STOP writing docs and report the UX issue to the prompter**:

### UX red flags to report

1. **Too many steps for a simple outcome** — If a user needs more than 3 commands to accomplish a basic task, flag it: *"This feature requires X steps to do Y. Consider: [suggested simplification]"*

2. **Inconsistent CLI patterns** — If a command behaves differently from similar commands, or flags aren't consistent across binaries, flag it: *"banyan-agent uses --node-name but banyan-cli uses --name for a similar concept. This will confuse users."*

3. **Missing sensible defaults** — If a user must provide configuration that could have an obvious default, flag it: *"Users must specify --node-name every time they start the agent, but the hostname is a perfectly good default. This adds friction."*

4. **Error messages that don't help** — If an error doesn't tell the user what to do next, flag it: *"When auth fails, the error says 'Unauthenticated' but doesn't suggest running 'banyan-cli auth'. Users will be stuck."*

5. **Requiring sudo when it shouldn't be needed** — If a read-only operation needs root, flag it: *"banyan-cli status requires sudo on some setups because /etc/banyan/banyan.yaml is root-only. The CLI config should be user-readable."*

6. **Concepts that need explaining are design smells** — If you find yourself writing a long explanation for why something works a certain way, the feature might need redesign. Flag it: *"I need 3 paragraphs to explain container naming. The naming scheme might be too complex."*

Report format:
```
UX CONCERN: [feature name]
Impact: [how this affects the user experience]
Suggestion: [concrete improvement]
Severity: [blocks-docs | worth-fixing | minor-friction]
```

Only proceed with documentation after the prompter acknowledges the concern.

## Target Audience

### Primary: Small to medium teams (2-30 engineers)

These teams:
- **Already know Docker Compose** — they've shipped with `docker compose up` and hit its ceiling
- **Don't have a dedicated DevOps/platform team** — the same engineers who write code also deploy it
- **Tried or evaluated Kubernetes** — and found it too complex, too heavy, or too much to learn
- **Want to ship, not operate infrastructure** — they'll spend an hour on setup, not a week
- **Value simplicity over configurability** — they'd rather have 5 things that work than 50 options

### Secondary: Larger teams exploring alternatives

These teams:
- May use Kubernetes already but find it overkill for some workloads
- Want a simpler option for staging, internal tools, or smaller services
- Have the expertise but want to reduce operational overhead

**Writing for both**: Never position Banyan as "only for small teams." Write docs that feel professional and capable. A small team should feel empowered; a large team should feel this is a serious tool, not a toy.

## Documentation as Product Marketing

Banyan's documentation IS the product experience. For open source, docs are the storefront, the sales pitch, and the onboarding — all in one. Every page should build conviction that Banyan is not "just another orchestration tool" but a genuinely different approach to a real problem.

### Core Positioning

**Banyan's story is NOT**: "We're a simpler Kubernetes."
**Banyan's story IS**: "We bridge the gap between `docker compose up` and production — the step that shouldn't require a PhD in infrastructure."

This distinction matters. "Simpler K8s" positions Banyan as a lesser version of something else. The bridge narrative positions Banyan as the **correct tool for a specific, painful gap** that Kubernetes was never designed to fill.

### The Pain-Gain Narrative

Every doc page should subtly carry this narrative arc. Not every page needs all three beats, but the reader should always feel the contrast:

**1. Acknowledge the real pain (briefly)**
Don't dwell on it, but name it so the reader feels understood:
- "You've outgrown a single server. Your Docker Compose file works, but it only runs on one machine."
- "Setting up Kubernetes for 3 servers and 5 services feels like building a highway to cross the street."

**2. Show the Banyan way (the bulk of the doc)**
Let the experience speak. When the reader sees a 3-step deployment that mirrors their Docker Compose workflow, they feel the contrast without you having to say "see how much simpler this is."

**3. Land the outcome (always end with success)**
Every guide should end with the user seeing a working result. The feeling of "I did that in 10 minutes" is more persuasive than any marketing copy.

### Competitive Positioning (without bashing)

Never trash Kubernetes, Docker Swarm, or Nomad. Instead, use **respectful contrast**:

| Instead of | Write |
|---|---|
| "Kubernetes is too complex" | "If your team needs multi-server deployment without the overhead of a full orchestration platform..." |
| "Helm charts are a mess" | "No templating languages to learn — your manifest is the same YAML you already write" |
| "K8s has too many concepts" | "Three concepts: engine, agent, manifest. That's the whole mental model." |
| "Docker Swarm is abandoned" | (Don't mention it. Let Banyan stand on its own.) |
| "Nomad requires Consul/Vault" | "Everything is included — no external dependencies to set up and maintain" |

**The rule**: Describe what Banyan IS, not what others AREN'T. Let the reader make the comparison themselves.

### Strategic Patterns for Each Page Type

**Landing page / Index**: This is the hook. Lead with the user's situation ("You know Docker Compose..."), show the minimal diff to Banyan, and end with the three-command cluster. The reader should think: "Wait, that's it?"

**Quickstart**: This is the proof. The reader is testing your claim. If they can deploy in 5 minutes, your marketing writes itself. If they can't — no amount of nice copy will save it. Time-to-first-deploy is the most important metric.

**Guides**: These build confidence and stickiness. Each guide should solve a real problem (not demonstrate a feature). Frame titles as user goals:
- "Deploy across multiple servers" (not "Multi-node architecture")
- "See what's running and fix problems" (not "Monitoring and troubleshooting")
- "Secure your cluster" (not "Authentication configuration")

**Reference**: This is where serious evaluators look. Clean, complete reference docs signal maturity. A tool with great reference docs feels production-ready even if it's early-stage.

### "Aha Moments" to Engineer into Docs

Design each major doc to deliver at least one moment where the reader thinks "oh, that's clever" or "I wish K8s did this":

| Page | Target "aha moment" |
|---|---|
| Landing page | "The diff between docker-compose.yml and banyan.yaml is 2 lines" |
| Quickstart | "I have a running cluster and I haven't left the terminal" |
| Multi-node | "I added a server and my manifest didn't change at all" |
| Manifest reference | "I already know all of this from Docker Compose" |
| CLI reference | "There are only 6 commands total" |
| Authentication | "The password is used once, then it's all tokens — I didn't configure anything" |

### Strategic Honesty as Marketing

Being upfront about limitations is a deliberate marketing strategy, not just ethics:

- **Builds trust**: "Volumes aren't supported yet" makes everything else you claim more credible
- **Sets expectations**: Users who hit a limitation they were warned about stay; users who discover one unexpectedly leave
- **Shows confidence**: Only insecure projects hide their gaps. Transparent roadmaps signal a team that knows where it's going
- **Creates advocates**: Users who feel respected by honest docs become vocal supporters

Pattern for limitations:
```markdown
> **Current limitation**: Banyan doesn't support persistent volumes yet.
> For databases, we recommend running them on a dedicated server or using
> a managed database service. Volume support is tracked on the [roadmap](/roadmap/).
```

### Metrics That Matter for Docs

When evaluating doc quality, optimize for these (in order):

1. **Time to first successful deploy** — Can someone go from zero to running in under 15 minutes?
2. **Zero-confusion flow** — Can they follow the quickstart without opening a second tab to Google something?
3. **Return visits** — Do they come back to the reference docs, or do they memorize the 6 commands?
4. **Shareability** — Would someone send the landing page to a colleague with "check this out"?

## Voice and Tone

### Be direct, be practical, be confident

The voice is someone who has been where the reader is, built something better, and is showing them the way. Not salesy. Not humble-braggy. Confident and helpful — like a senior engineer recommending a tool to a friend.

| Do | Don't |
|---|---|
| "Run this command" | "You might want to consider running..." |
| "This takes about 5 minutes" | "Follow these simple steps" (let them judge) |
| "Banyan distributes your containers across servers" | "Banyan leverages distributed orchestration paradigms" |
| "You need root to start the engine because it manages containerd" | (silently require sudo without explaining) |
| "Volumes aren't supported yet" | "Volume support is coming in a future release" (be honest about gaps) |
| "Your manifest doesn't change — you just have more servers" | "Banyan provides seamless multi-node scaling capabilities" |
| "Three concepts: engine, agent, manifest" | "Banyan's lightweight architecture reduces cognitive overhead" |

### Speak to their experience and pain

- **Reference Docker Compose** — it's what they know. "Same `services:` block you already use."
- **Name the gap they feel** — "You've outgrown one server but Kubernetes feels like overkill." This builds instant rapport.
- **Contrast with K8s pain points** — but don't trash K8s. "No Helm charts, no YAML templating, no CRDs." Let the absence speak.
- **Acknowledge the step up** — going from single-server to multi-server is real. Don't minimize it. Then show how Banyan makes it a small step, not a leap.
- **Time-box everything** — "This takes ~5 minutes", "You'll have a running cluster in under an hour." Concrete time claims are more convincing than adjectives.

### What NOT to say

- "Simple" or "easy" — let the experience speak for itself. If it really is easy, 3 commands will prove it better than the word "easy"
- "Just" — ("just run this command") dismisses complexity the user might feel
- "Obviously" or "of course" — makes users feel dumb when it's not obvious to them
- "Powerful" or "flexible" — these are meaningless marketing words. Show, don't tell
- "Best-in-class", "enterprise-grade", "cutting-edge" — corporate buzzwords destroy credibility with engineers
- "Unlike Kubernetes..." — don't compare directly. Let the reader make the comparison from what they see
- Technical jargon without context — "gRPC", "etcd", "VXLAN" need explanation on first use in user docs

## Documentation Structure

### Every doc page follows this structure

```
1. What the user wants to accomplish (1 sentence)
2. Prerequisites (what they need before starting)
3. Steps (numbered, with expected output after each)
4. What just happened (brief explanation of what Banyan did)
5. Next steps (what to do next, with links)
```

### Page types and their focus

#### Getting Started pages
- **Goal**: First successful deployment in under 15 minutes of reading
- **Tone**: Encouraging, momentum-building
- **Structure**: Linear flow, no branching, no "if you want X instead..."
- **Key principle**: Show the shortest path. Advanced options go in reference docs.

#### Guide pages
- **Goal**: Help users accomplish a specific real-world task
- **Tone**: Practical, solution-oriented
- **Structure**: Problem → Solution → Verify
- **Key principle**: Start with the user's goal, not the feature name. "Deploy across multiple servers" not "Multi-node architecture"

#### Reference pages
- **Goal**: Complete, searchable information
- **Tone**: Precise, no-nonsense
- **Structure**: Tables, flags, examples for every option
- **Key principle**: Every flag gets an example. Every example is copy-pasteable.

#### Troubleshooting pages
- **Goal**: Get users unstuck fast
- **Tone**: Empathetic but efficient
- **Structure**: Symptom → Cause → Fix
- **Key principle**: Use the exact error message as the heading so users can search/find it.

## Writing Rules

### 1. Show expected output for every command

Bad:
```markdown
Run `banyan-cli status` to check your cluster.
```

Good:
```markdown
Check your cluster:

\`\`\`bash
banyan-cli status
\`\`\`

\`\`\`
Banyan Cluster - Status
========================================
Engine: RUNNING
Connection: localhost:50051

Agents: 1
  - local-worker (status: ready, last seen: 2s ago)

Deployments: 0
========================================
\`\`\`
```

### 2. Every code block must be copy-pasteable

- No `<placeholder>` — use realistic values and explain what to change
- No `...` truncation — show the full command
- Use comments for what to customize: `# Replace 192.168.1.10 with your engine IP`

### 3. Compare with Docker Compose when relevant

When documenting a Banyan feature, show the Docker Compose equivalent if one exists. This bridges the knowledge gap:

```markdown
## Port mapping

Same as Docker Compose:

\`\`\`yaml
# Docker Compose         # Banyan (identical)
ports:                    ports:
  - "80:80"                 - "80:80"
  - "8080:8080"             - "8080:8080"
\`\`\`
```

### 4. Time-box setup steps

Add time estimates to section headers when the task takes more than 1 minute:

```markdown
## 1. Start the Engine (~2 minutes)
## 2. Start Workers (~1 minute per worker)
## 3. Deploy your app (~3 minutes including image build)
```

### 5. Address the "what if" before it's asked

Anticipate questions and answer them inline:

```markdown
The Engine runs in the foreground. Open a new terminal for the next steps.
(To run it in the background, see [Running as a service](/guides/systemd/).)
```

### 6. Be honest about limitations

Don't hide what's missing. Users trust docs that are honest:

```markdown
> **Not yet supported**: Persistent volumes, health check probes, and resource limits
> are on the [roadmap](/roadmap/). For now, use external storage solutions.
```

### 7. Use callouts purposefully

Banyan docs use Starlight (Astro) with these callout types:

```markdown
:::note
Background info that helps understanding but isn't required to proceed.
:::

:::tip
Shortcuts or best practices that save time.
:::

:::caution
Something that might cause confusion or unexpected behavior.
:::

:::danger
Data loss, security risk, or breaking changes.
:::
```

## Banyan's Value Propositions (reference for writing)

Use these when framing features. Don't list them all — weave them naturally. Each value has a surface claim and a deeper "why it matters" that connects to real pain:

| Value | How to communicate it | Why it resonates (the pain it solves) |
|---|---|---|
| Docker Compose syntax | "Write the same YAML you already know" | Teams spent months learning Compose. Banyan says: that investment carries forward. |
| 1-hour setup | "From zero to running cluster in under an hour" | K8s clusters take days to set up properly. Teams need to ship this sprint, not next month. |
| Three binaries | "No package managers, no plugins — three focused binaries" | K8s has kubelet, kube-proxy, kube-apiserver, etcd, controller-manager, scheduler, CoreDNS, kubectl, Helm, Tiller... |
| Built-in registry | "No Docker Hub account needed, no private registry to manage" | Setting up Harbor or ECR is a whole project. Banyan includes it. |
| Built-in monitoring | "`banyan-cli status` shows everything — no dashboards to set up" | Grafana + Prometheus + Loki stack takes longer to configure than the actual application. |
| Prometheus-compatible | "Works with your existing Prometheus setup" | Teams already invested in Prometheus don't want to learn a new metrics format. |
| No Helm charts | "No templating languages, no chart repositories, no values.yaml" | Helm is the #1 complaint about K8s DX. Banyan eliminates the entire layer. |
| VPC networking | "Containers talk across servers — Banyan handles the networking" | Cross-node networking is where most K8s debugging hours go. |
| Open source | "Inspect the code, modify it, self-host everything" | No vendor lock-in, no usage-based pricing surprises, no "contact sales." |
| Minimal mental model | "Three concepts: engine, agent, manifest" | K8s has 50+ resource types. Banyan has 3 things to understand. |
| No YAML templating | "Your manifest is the deployment — no translation layer" | K8s manifests go through Helm → templates → values → overrides. Banyan is what-you-write-is-what-runs. |

## Current Feature Status (check before writing)

Before documenting any feature, verify its implementation status:

| Feature | Status | Doc approach |
|---|---|---|
| Core orchestration (deploy, status, logs, down) | Done | Document fully |
| Multi-node deployment | Done | Document fully |
| Authentication (password + token) | Done | Document fully |
| Built-in image registry | Done | Document fully |
| VPC networking | Done | Document fully |
| Prometheus metrics | Planned (M4) | Mention on roadmap only |
| CLI terminal dashboard | Planned (M4) | Mention on roadmap only |
| Resource-aware scheduling | Planned (M5) | Mention on roadmap only |
| Multi-engine HA | Planned (M6) | Don't document |
| Auto-scaling | Planned (M7) | Don't document |
| Volumes | Not planned yet | Note as limitation |
| mTLS | Planned | Mention as upcoming |

**Rule**: Never document a planned feature as if it exists. Use "coming in [milestone]" or link to the roadmap.

## Documentation File Locations

User documentation lives in: `website/src/content/docs/`

```
getting-started/
  installation.md     — Install Banyan and dependencies
  quickstart.md       — First deployment in 5 minutes

guides/                       — Task-oriented: "How do I do X?"
  multi-node.md       — Deploy across multiple servers
  authentication.md   — Secure cluster communication

reference/                    — Lookup-oriented: "What's the syntax for X?"
  manifest.md         — All banyan.yaml fields
  cli.md              — All commands and flags
  troubleshooting.md  — Common issues and fixes

roadmap.md            — What's next
index.mdx             — Landing page
```

Format: Starlight (Astro) with YAML frontmatter:

```yaml
---
title: Page Title
description: One-line description for SEO and sidebar.
sidebar:
  order: 1  # Lower numbers appear first
---
```

## Checklist Before Publishing

For every doc page, verify:

### Quality gates
- [ ] **UX audit done** — No UX concerns that should be flagged
- [ ] **Target audience fit** — Would a team of 5 engineers without a DevOps person understand this?
- [ ] **Copy-pasteable** — Every command can be pasted and run (with noted customizations)
- [ ] **Expected output shown** — User knows what success looks like
- [ ] **Time estimates** — User knows how long each section takes
- [ ] **Honest about limitations** — Nothing promised that doesn't exist yet
- [ ] **Docker Compose bridge** — Familiar concepts referenced where applicable
- [ ] **No jargon without context** — Technical terms explained on first use
- [ ] **Tested the flow** — Commands actually work in the order presented
- [ ] **Links verified** — All internal links point to real pages

### Marketing-awareness gates
- [ ] **Pain-gain present** — The page connects to a real problem the user has (even subtly)
- [ ] **"Aha moment" exists** — There's at least one point where the reader should feel "this is better than what I'm used to"
- [ ] **No generic tone** — Doesn't read like "yet another tool's docs." Reads like a team that understands your problem built something specific for it
- [ ] **Shareability test** — Would someone send this page to a colleague? If not, what would make them want to?
- [ ] **Confidence without arrogance** — The docs feel like a capable tool, not a toy, not an enterprise pitch
- [ ] **Competitive contrast implicit** — The reader naturally thinks "this is better than K8s for my use case" without you saying it
