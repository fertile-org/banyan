# [HIGH-006] Environment Variables (Secrets) Stored Plaintext in etcd

**Status**: WONTFIX (by design) — Banyan stores environment variables as plaintext, matching Docker Compose behavior. A proper secrets management feature (encrypt at rest, inject into containers) is planned in [Milestone 10 — Advanced Security](/roadmap/#milestone-10--advanced-security).
**Severity**: High
**Responsibility**: Platform Issue
**Component**: Storage — etcd
**File(s)**:
- `pkg/types/records.go:69` (`ServiceRecord.Environment []string`)
- `pkg/types/records.go:102` (`TaskRecord.Environment []string`)
- `pkg/storage/etcd.go` (JSON serialization, no encryption)

## Description

Deployment manifests commonly include secrets as environment variables (e.g., `DATABASE_PASSWORD=hunter2`, `API_KEY=sk-xxx`). These are stored in etcd as JSON-serialized `[]string` with no encryption.

The data path: manifest YAML -> gRPC protobuf -> etcd JSON. At no point is the data encrypted at the application level. etcd itself does not encrypt data at rest by default.

## Impact

- **Who**: Any process on the engine host (managed etcd has no auth — see MED-002), or anyone with etcd credentials (external etcd)
- **What they gain**: All application secrets for all deployments ever stored
- **Blast radius**: All deployments — every secret ever passed as an environment variable

## Recommendation

Short-term: Document that environment variables are stored in plaintext and advise users to limit secrets in env vars.

Medium-term: Add application-level encryption for environment variables before storing in etcd. Decrypt only when distributing to agents.

Long-term: Implement a proper secrets management system (encrypted secret store with access control).
