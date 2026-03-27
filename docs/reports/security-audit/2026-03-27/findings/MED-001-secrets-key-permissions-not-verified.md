# MED-001: secrets.key file permissions not verified at runtime

**Severity**: Medium
**Responsibility**: Mitigation Gap
**Component**: Engine — Secrets Manager
**File(s)**: `pkg/engine/secrets.go:77-86`

## Description

`LoadSecretsKey()` reads the key file without checking file permissions. If the file is world-readable (e.g., 0644 after a misconfigured backup restore), the AES-256 key is exposed.

## Impact

- **Who**: Any user on the engine host
- **What**: Read the encryption key, decrypt all secrets from etcd
- **Blast radius**: All secrets in the cluster

## Recommendation

Check permissions in `LoadSecretsKey()` and refuse to load if not 0600:
```go
info, _ := os.Stat(path)
if info.Mode().Perm() != 0o600 {
    return nil, fmt.Errorf("secrets.key has insecure permissions %o (must be 0600)", info.Mode().Perm())
}
```
