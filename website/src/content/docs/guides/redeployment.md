---
title: Redeployment
description: Update running applications with zero downtime using blue-green deployments.
sidebar:
  order: 3
---

You've deployed your app. Now you need to push a code change. With most orchestration tools, this is where the complexity starts — rollout strategies, health check configurations, rollback procedures.

With Banyan, you run the same command again.

## How redeployment works

When you run `banyan-cli up` for an application that's already running, Banyan automatically replaces the old containers using a **blue-green** strategy:

1. New containers start alongside the old ones
2. Banyan waits for all new containers to reach `running` status. If any service has a [`healthcheck`](/reference/manifest/#service) configured, Banyan also waits for those containers to report `healthy` before proceeding.
3. Old containers are torn down

If the new deployment fails, the old containers keep running. No downtime, no manual rollback.

```bash
# First deploy
banyan-cli up -f banyan.yaml

# Later, after a code change — same command
banyan-cli up -f banyan.yaml
```

That's it. No `--strategy` flag, no rollout configuration. Blue-green is the default because it's the safest approach for most teams.

## What you see during a redeployment

```
Banyan Up
========================================
Reading manifest: banyan.yaml
Building images...
  Building web → my-app-web:latest
Images built successfully.

Application: my-app
Services: 2
  - web: my-app-web:latest (replicas: 2)
  - api: my-app-api:latest (replicas: 3)

Connecting to Engine at 192.168.1.10:50051...
Deployment 'my-app' created (ID: my-app-1708123456)
Waiting for deployment to complete...
  Status: deploying (tasks dispatched to agents)
  Status: running

========================================
Deployment 'my-app' is RUNNING!
```

Behind the scenes, Banyan ran both old and new containers simultaneously, confirmed the new ones were healthy (including passing any configured healthchecks), then removed the old ones. The transition typically takes a few seconds.

## Updating specific services

You don't always need to redeploy everything. If only your `web` service changed, redeploy just that service:

```bash
banyan-cli up -f banyan.yaml web
```

Or update multiple services at once:

```bash
banyan-cli up -f banyan.yaml web api
```

:::caution
Per-service deploys use the same **blue-green** strategy as full deploys — new containers start alongside old ones and old containers are torn down only after the new deployment is healthy. Other services continue running uninterrupted.
:::

### Dependency validation

Banyan validates `depends_on` for per-service deploys. If a service you're deploying depends on another service, that dependency must either be already running or included in the same deploy command.

```yaml
services:
  db:
    image: postgres:15-alpine

  api:
    image: my-api:latest
    depends_on:
      - db

  web:
    image: my-web:latest
    depends_on:
      - api
```

```bash
# Works: db is already running
banyan-cli up -f banyan.yaml api

# Fails: api is not running and not being deployed
banyan-cli up -f banyan.yaml web

# Works: deploying both together satisfies the dependency
banyan-cli up -f banyan.yaml web api
```

## What happens when a deployment fails

**Full deploy (blue-green)**: The old containers keep running. The failed deployment is marked as `failed`, but your application stays up. Fix the issue and run `up` again.

```bash
# Check what went wrong
banyan-cli deployment my-app

# Look at container logs for the failed deployment
banyan-cli logs my-app-1708123456-web-0

# Fix the issue and redeploy
banyan-cli up -f banyan.yaml
```

**Per-service deploy (recreate)**: The old containers for the targeted services are stopped before new ones start. If the new containers fail, those specific services will be down. Other services in the deployment are unaffected.

```bash
# Fix the issue and retry just the failed service
banyan-cli up -f banyan.yaml web
```

:::tip
For critical services, prefer full deploys (`banyan-cli up -f banyan.yaml` without service names) so you get the blue-green safety net. Use per-service deploys for quick iterations on non-critical services during development.
:::

## Container naming during redeployment

During a blue-green deploy, new containers get a deployment-ID prefix to avoid name conflicts while both old and new containers run simultaneously:

| Phase | Old containers | New containers |
|-------|---------------|----------------|
| Both running | `my-app-web-0` | `my-app-1708123456-web-0` |
| After teardown | *(removed)* | `my-app-1708123456-web-0` |

Per-service deploys reuse the original naming pattern (`my-app-web-0`) since old containers are stopped before new ones start.

:::note
Container names may change after a blue-green redeployment. Always use the service DNS name (e.g., `DB_HOST=db.my-app.internal`) instead of container names in environment variables. DNS names are stable across redeployments.
:::

## Strategy comparison

| | Blue-green (full deploy) | Recreate (per-service) |
|---|---|---|
| **Command** | `banyan-cli up -f banyan.yaml` | `banyan-cli up -f banyan.yaml web` |
| **Downtime** | None — old and new run simultaneously; waits for healthchecks | Brief — old stopped before new starts |
| **On failure** | Old containers keep running | Targeted services go down |
| **Port conflicts** | Avoided via deployment-ID naming | Avoided by stopping old first |
| **Best for** | Production updates | Development iterations, non-critical services |

## Common workflows

### Push a code change

```bash
# Make changes to your code, then:
banyan-cli up -f banyan.yaml
```

Banyan rebuilds any services with `build:` directives, pushes the new images to the built-in registry, and does a blue-green swap.

### Scale a service

Update the replica count in your manifest and redeploy:

```yaml
services:
  api:
    image: my-api:latest
    deploy:
      replicas: 5    # was 3
```

```bash
banyan-cli up -f banyan.yaml
```

### Update environment variables

Change the environment in your manifest and redeploy:

```yaml
services:
  api:
    image: my-api:latest
    environment:
      - DB_HOST=new-db-host
      - LOG_LEVEL=debug    # added
```

```bash
banyan-cli up -f banyan.yaml api
```

### Roll back to a previous version

There's no `rollback` command — redeployment handles it. Point your manifest at the previous image and deploy:

```yaml
services:
  api:
    image: my-api:v1.2.0    # was v1.3.0
```

```bash
banyan-cli up -f banyan.yaml
```

If a deployment fails mid-way, there's nothing to roll back — the old containers are still running.
