---
title: Secrets
description: Store sensitive values encrypted and inject them into containers — no plaintext in manifests or source control.
sidebar:
  order: 6
---

Database passwords, API keys, tokens — sensitive values that shouldn't live in your manifest or `.env` files. Banyan encrypts them at rest and injects them into containers at runtime.

## Create a secret

```bash
banyan-cli secret create DB_PASSWORD
```

```
Enter secret value: ********
Secret "DB_PASSWORD" created.
```

The value is encrypted with AES-256-GCM and stored in etcd. It never appears in your manifest, your deploy logs, or your task records.

You can also provide the value non-interactively:

```bash
# From a file (good for CI/CD)
banyan-cli secret create DB_PASSWORD --from-file ./password.txt

# Inline (visible in shell history — use for testing only)
banyan-cli secret create DB_PASSWORD --value "s3cret"
```

Creating a secret that already exists updates its value.

## Reference secrets in your manifest

Add a `secrets:` list to any service. Each secret name becomes an environment variable inside the container:

```yaml
name: my-app

services:
  api:
    image: myapp/api:latest
    environment:
      - DB_HOST=db.my-app.internal
    secrets:
      - DB_PASSWORD
      - API_KEY
    ports:
      - "8080:8080"

  db:
    image: postgres:15
    secrets:
      - DB_PASSWORD
    environment:
      - POSTGRES_USER=banyan
```

Both `api` and `db` receive `DB_PASSWORD` as an environment variable. The `api` service also gets `API_KEY`. Regular `environment:` variables work alongside secrets.

Deploy as usual:

```bash
banyan-cli up -f banyan.yaml
```

If a secret doesn't exist, the deploy fails immediately with a clear message:

```
Error: service "api" references secret "API_KEY" which does not exist.
Create it with: banyan-cli secret create API_KEY
```

## Manage secrets

### List all secrets

```bash
banyan-cli secret list
```

```
NAME                           CREATED              UPDATED
------------------------------------------------------------------------
API_KEY                        2h ago               2h ago
DB_PASSWORD                    5d ago               1h ago
REDIS_AUTH                     5d ago               5d ago
```

### View secret metadata

```bash
banyan-cli secret get DB_PASSWORD
```

```
Secret: DB_PASSWORD
  Created:  2026-03-27T10:00:00Z
  Updated:  2026-03-27T15:30:00Z
```

To see the actual value (for debugging):

```bash
banyan-cli secret get DB_PASSWORD --reveal
```

```
Secret: DB_PASSWORD
  Created:  2026-03-27T10:00:00Z
  Updated:  2026-03-27T15:30:00Z
  Value:    s3cret-passw0rd
```

The `--reveal` flag prevents accidental exposure in shared terminals or screen recordings.

### Update a secret

Run `create` again with the same name — the value is replaced:

```bash
banyan-cli secret create DB_PASSWORD
```

```
Enter secret value: ********
Secret "DB_PASSWORD" created.
```

Running containers keep the old value until you redeploy:

```bash
banyan-cli up -f banyan.yaml
```

### Delete a secret

```bash
banyan-cli secret delete DB_PASSWORD
```

```
Secret "DB_PASSWORD" deleted.
```

If a running deployment references the secret, the delete is blocked:

```
Error: cannot delete secret "DB_PASSWORD": referenced by deployment "my-app" (service: api)
```

Tear down the deployment first, or update the manifest to remove the reference.

## How it works

### What's encrypted

Secret values are encrypted with AES-256-GCM before being stored in etcd. During `banyan-engine init`, the wizard asks whether to generate a new encryption key or provide an existing one. The key is stored at `/etc/banyan/keys/secrets.key` on the engine.

### How secrets reach containers

1. You create a secret — the value is encrypted and stored in etcd at `/banyan/secrets/<name>`.
2. You deploy with `secrets: [DB_PASSWORD]` — the engine stores only the **name** in the task record. The plaintext value is never written to the task record.
3. When an agent polls for tasks, the engine decrypts the secrets just-in-time and includes them in the gRPC response.
4. The agent injects each secret as an environment variable when starting the container.

All gRPC traffic travels over the WireGuard tunnel, so secrets are encrypted both at rest (etcd) and in transit (WireGuard).

### Naming rules

Secret names must be valid environment variable identifiers: letters, digits, and underscores, starting with a letter or underscore. Uppercase is conventional but not enforced.

```
DB_PASSWORD    ✓
API_KEY        ✓
my_token_123   ✓
3rd-party      ✗  (starts with digit, contains dash)
```

### Secret vs environment collision

If a secret has the same name as an `environment:` variable, the secret takes precedence. This lets you migrate from plaintext env vars to secrets without changing your application code.

## High availability

In a multi-engine setup, all engines must use the **same** `secrets.key`. The `banyan-engine init` wizard handles this:

1. **First engine**: Choose "Generate new key". Copy the key file to the other engine nodes:
   ```bash
   scp engine-1:/etc/banyan/keys/secrets.key engine-2:/etc/banyan/keys/secrets.key
   ```

2. **Additional engines**: Choose "Provide existing key file" and enter the path to the copied key.

Secrets are stored in the shared etcd cluster, so any engine can create, read, or delete them once the key is in place.

:::caution
If you lose `secrets.key`, existing secrets cannot be decrypted. Keep a backup in a secure location.
:::

## Limitations

- **Environment variables only** — secrets are injected as `-e` flags, not mounted as files. File-based injection (for certificates, service account JSON files) is planned.
- **No automatic rotation** — updating a secret requires a redeploy for running containers to pick up the new value.
- **Cluster-wide scope** — all secrets are available to all deployments. Per-deployment scoping is not yet supported.
- **No external backends** — secrets are stored in Banyan's etcd. Integration with Vault, AWS Secrets Manager, or similar tools is planned.

See the [Manifest Reference — Secrets](/reference/manifest/#secrets) for the full field reference and the [CLI Reference — secret](/reference/cli/#secret) for all command options.
