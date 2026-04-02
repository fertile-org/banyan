---
title: Why Banyan?
description: A short introduction to Banyan and the problem it solves.
sidebar:
  order: 0
---

## The short version

You know Docker Compose. You write a YAML file, run `docker compose up`, and your app is running. It works great — until you need a second server.

The usual next step is Kubernetes. But Kubernetes is built for platform teams running hundreds of microservices. If your team is 5–30 engineers shipping a product, the learning curve and operational overhead are a hard trade-off.

**Banyan sits in between.** It's a container orchestrator where:

- **Everything is built in** — networking, service discovery, load balancing, a container registry, a terminal dashboard. No external tools to assemble.
- **The manifest feels like Docker Compose** — same `services`, `build`, `ports`, `environment`, `depends_on` you already know. Add `deploy.replicas` to scale across servers.
- **Three concepts, not thirty** — engine (control plane), agent (one per server), manifest (your YAML file). No CRDs, no Helm charts, no operators.
- **Self-healing by default** — containers crash? Restarted automatically. Server dies? Work rescheduled to healthy servers. You declare what should run, Banyan keeps it running.
- **High availability when you need it** — run multiple engines and the cluster keeps working when a server goes down. Start with one engine, add more later.
- **Forever open source** — Apache 2.0, no enterprise edition, no BSL conversion.

## Who is it for?

Teams who've outgrown a single server but don't need — or don't want — Kubernetes. The same engineers who write the code also deploy it.

## Want the full story?

Read the [white paper](/blog/from-one-server-to-many/) — it covers the orchestration landscape honestly (where Kubernetes is the right choice, where it isn't), explains Banyan's design principles, and walks through the technical architecture.

## How does it work?

Read [How Banyan Works](/getting-started/how-it-works/) — the mental model behind the whole system, explained in 5 ideas. Takes 10 minutes and makes everything else click.

## Ready to try it?

Head to the [quickstart](/getting-started/quickstart/) to deploy your first application.
