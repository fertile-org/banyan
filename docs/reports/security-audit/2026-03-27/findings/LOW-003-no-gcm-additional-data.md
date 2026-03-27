# LOW-003: AES-GCM encryption uses no additional authenticated data

**Severity**: Low
**Responsibility**: Mitigation Gap
**Component**: Engine — Secrets Manager
**File(s)**: `pkg/engine/secrets.go:217`

## Description

`aesgcm.Seal(nonce, nonce, plaintext, nil)` passes `nil` for additional authenticated data (AAD). Using the secret name as AAD would bind the ciphertext to the specific key path, preventing ciphertext relocation attacks.

## Impact

An attacker with etcd write access could copy one secret's ciphertext to another key. Without AAD, the engine would decrypt it successfully under the wrong name.

## Recommendation

Use the secret name as AAD: `aesgcm.Seal(nonce, nonce, plaintext, []byte(name))`. Requires passing the name to encrypt/decrypt methods.
