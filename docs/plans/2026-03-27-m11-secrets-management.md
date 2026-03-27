# M11 — Secrets Management Implementation Plan

## Overview

Add secrets management to Banyan so users can store sensitive values (database passwords, API keys, tokens) encrypted in etcd and reference them in manifests — without putting plaintext in YAML files, `.env` files, or source control.

## Current State

- Environment variables are the only way to pass configuration to containers
- `environment:` and `env_file:` values are stored as plaintext JSON in etcd (in ServiceRecord, TaskRecord)
- No encryption at rest for any stored data
- No `secrets` field in manifest, proto, or record types
- Existing crypto: only `crypto/sha256` (tunnel IP derivation) and `golang.org/x/crypto/curve25519` (WireGuard keys)

## Desired End State

```bash
# Create a secret (value never stored in manifest or source control)
banyan-cli secret create DB_PASSWORD
Enter secret value: ********

# Reference in manifest
cat banyan.yaml
```
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
```
```bash
# Deploy — secrets resolved at runtime, never in etcd task records
banyan-cli up -f banyan.yaml
```

### Security properties:
- Secret values encrypted at rest in etcd (AES-256-GCM)
- Secret values NEVER stored in TaskRecord (only secret names as references)
- Secret values resolved just-in-time during PollTasks (in-memory only)
- Secret values transmitted over WireGuard (encrypted in transit)
- Encryption key stored on engine filesystem only (`/etc/banyan/keys/secrets.key`)

### Verification:
- `banyan-cli secret list` shows all secrets
- `banyan-cli secret get DB_PASSWORD --reveal` shows plaintext (with friction)
- `banyan-cli up` with invalid secret reference fails with clear error
- Deleting a secret referenced by a running deployment is blocked
- Running containers receive secrets as environment variables

## What We're NOT Doing

- File-based injection (`/run/secrets/`) — env vars cover 95% of use cases, file injection in v2
- Secret versioning/rotation — update + redeploy is sufficient for v1
- Per-deployment secret scoping — cluster-wide scope matches Banyan's trust model
- External secret backends (Vault, AWS SM) — future milestone
- Automatic secret propagation to running containers — requires redeploy (same as K8s)

## Implementation Approach

Five phases, each independently testable:

1. **Encryption & storage layer** — AES-256-GCM, SecretRecord, key generation
2. **Proto & gRPC** — messages, RPCs, PollTasks secret resolution
3. **CLI commands** — `secret create/list/get/delete`
4. **Manifest integration** — `secrets:` field, deploy-time validation, agent injection
5. **Tests & documentation** — unit tests, E2E test, docs

---

## Phase 1: Encryption & Storage Layer

### Overview
Build the core encryption engine and secret CRUD operations on etcd.

### Changes Required:

#### 1. Secret record type
**File**: `pkg/types/records.go`

Add key constant and record type:

```go
const (
    // ... existing constants ...
    KeySecrets = "secrets/"  // /banyan/secrets/<name>
)

// SecretRecord stores an encrypted secret in etcd.
type SecretRecord struct {
    Name           string    `json:"name"`
    EncryptedValue []byte    `json:"encrypted_value"` // AES-256-GCM ciphertext (nonce + ciphertext + tag)
    CreatedAt      time.Time `json:"created_at"`
    UpdatedAt      time.Time `json:"updated_at"`
}
```

#### 2. Encryption module
**File**: `pkg/engine/secrets.go` (NEW)

Core functions:

```go
package engine

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "fmt"
    "os"
    "context"
    "time"

    "github.com/fertile-org/banyan/pkg/storage"
    "github.com/fertile-org/banyan/pkg/types"
)

// SecretsManager handles encrypted secret storage.
type SecretsManager struct {
    store  storage.StateStore
    aesgcm cipher.AEAD
}

// NewSecretsManager creates a manager with the given encryption key.
func NewSecretsManager(store storage.StateStore, keyPath string) (*SecretsManager, error)

// GenerateSecretsKey creates a random 256-bit key file.
func GenerateSecretsKey(path string) error

// LoadSecretsKey reads a key file from disk.
func LoadSecretsKey(path string) ([]byte, error)

// Create stores a new encrypted secret. Errors if name already exists.
func (sm *SecretsManager) Create(ctx context.Context, name string, value []byte) error

// Update replaces the value of an existing secret. Errors if not found.
func (sm *SecretsManager) Update(ctx context.Context, name string, value []byte) error

// Get decrypts and returns a secret value. Errors if not found.
func (sm *SecretsManager) Get(ctx context.Context, name string) ([]byte, error)

// List returns all secret names with metadata (no values).
func (sm *SecretsManager) List(ctx context.Context) ([]types.SecretRecord, error)

// Delete removes a secret. Does NOT check references (caller's responsibility).
func (sm *SecretsManager) Delete(ctx context.Context, name string) error

// ResolveSecrets decrypts multiple secrets by name, returning a name→value map.
// Used by PollTasks for just-in-time secret resolution.
func (sm *SecretsManager) ResolveSecrets(ctx context.Context, names []string) (map[string]string, error)
```

Encryption: AES-256-GCM with random 12-byte nonce prepended to ciphertext.

```go
func (sm *SecretsManager) encrypt(plaintext []byte) ([]byte, error) {
    nonce := make([]byte, sm.aesgcm.NonceSize())
    if _, err := rand.Read(nonce); err != nil {
        return nil, err
    }
    return sm.aesgcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (sm *SecretsManager) decrypt(ciphertext []byte) ([]byte, error) {
    nonceSize := sm.aesgcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return nil, fmt.Errorf("ciphertext too short")
    }
    return sm.aesgcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}
```

Secret name validation (must be a valid env var identifier):

```go
func ValidateSecretName(name string) error {
    if name == "" {
        return fmt.Errorf("secret name cannot be empty")
    }
    if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
        return fmt.Errorf("secret name %q must be a valid environment variable name (letters, digits, underscores)", name)
    }
    return nil
}
```

#### 3. Key generation at engine init
**File**: `cmd/banyan-engine/cmd/engine.go`

During `banyan-engine init`, after WireGuard keypair generation:

```go
// Generate secrets encryption key
secretsKeyPath := filepath.Join(types.DefaultKeysDir, "secrets.key")
if _, err := os.Stat(secretsKeyPath); os.IsNotExist(err) {
    if err := engine.GenerateSecretsKey(secretsKeyPath); err != nil {
        return fmt.Errorf("failed to generate secrets key: %w", err)
    }
    fmt.Printf("  [OK] Secrets encryption key: %s\n", secretsKeyPath)
} else {
    fmt.Printf("  [OK] Secrets encryption key already exists: %s\n", secretsKeyPath)
}
```

#### 4. Engine loads SecretsManager at startup
**File**: `pkg/engine/engine.go`

Add `secrets *SecretsManager` field to `Engine` struct. Initialize in `New()`:

```go
// Load secrets encryption key (optional — secrets features disabled if key missing)
secretsKeyPath := filepath.Join(opts.DataDir, "keys", "secrets.key")
if _, err := os.Stat(secretsKeyPath); err == nil {
    sm, smErr := NewSecretsManager(store, secretsKeyPath)
    if smErr != nil {
        return nil, fmt.Errorf("failed to initialize secrets manager: %w", smErr)
    }
    e.secrets = sm
}
```

### Success Criteria:

#### Automated:
- [ ] `go build ./pkg/engine/...` compiles
- [ ] `go test ./pkg/engine/ -run TestSecrets` — unit tests for encrypt/decrypt, CRUD, name validation
- [ ] `go vet ./pkg/engine/...` passes
- [ ] `gofmt -l ./pkg/engine/secrets.go` returns empty

---

## Phase 2: Proto & gRPC RPCs

### Overview
Add proto messages for secrets, gRPC handlers, and wire secret resolution into PollTasks.

### Changes Required:

#### 1. Proto definitions
**File**: `pkg/rpc/proto/banyan/v1/engine.proto`

Add RPCs to EngineService:

```protobuf
rpc CreateSecret(CreateSecretRequest) returns (CreateSecretResponse);
rpc ListSecrets(ListSecretsRequest) returns (ListSecretsResponse);
rpc GetSecret(GetSecretRequest) returns (GetSecretResponse);
rpc DeleteSecret(DeleteSecretRequest) returns (DeleteSecretResponse);
```

Add messages:

```protobuf
message CreateSecretRequest {
  string name = 1;
  bytes value = 2;
}
message CreateSecretResponse {}

message ListSecretsRequest {}
message ListSecretsResponse {
  repeated SecretInfo secrets = 1;
}

message SecretInfo {
  string name = 1;
  string created_at = 2;
  string updated_at = 3;
}

message GetSecretRequest {
  string name = 1;
  bool reveal = 2;  // if true, include plaintext value
}
message GetSecretResponse {
  string name = 1;
  string created_at = 2;
  string updated_at = 3;
  bytes value = 4;   // only populated if reveal=true
}

message DeleteSecretRequest {
  string name = 1;
}
message DeleteSecretResponse {}
```

Add `secret_refs` and `resolved_secrets` to TaskRecord:

```protobuf
message TaskRecord {
  // ... existing fields ...
  repeated string secret_refs = 22;            // stored in etcd (names only)
  map<string, string> resolved_secrets = 23;   // populated at PollTasks time, never stored
}
```

Add `secrets` to ManifestService:

```protobuf
message ManifestService {
  // ... existing fields ...
  repeated string secrets = 15;  // secret names to inject as env vars
}
```

#### 2. Generate proto
```bash
cd pkg/rpc && make generate
```

#### 3. gRPC handlers
**File**: `pkg/engine/grpc_server.go`

Implement the 4 secret RPCs:

```go
func (s *engineGRPCServer) CreateSecret(ctx context.Context, req *banyanpb.CreateSecretRequest) (*banyanpb.CreateSecretResponse, error) {
    if s.engine.secrets == nil {
        return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key)")
    }
    if err := ValidateSecretName(req.Name); err != nil {
        return nil, status.Errorf(codes.InvalidArgument, "%v", err)
    }
    if err := s.engine.secrets.Create(ctx, req.Name, req.Value); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to create secret: %v", err)
    }
    s.engine.emitEvent("secret.created", fmt.Sprintf("Secret %q created", req.Name), "info")
    return &banyanpb.CreateSecretResponse{}, nil
}

// ListSecrets, GetSecret, DeleteSecret follow same pattern
```

For `DeleteSecret` — check if any running deployment references this secret:

```go
func (s *engineGRPCServer) DeleteSecret(ctx context.Context, req *banyanpb.DeleteSecretRequest) (*banyanpb.DeleteSecretResponse, error) {
    // Check all running deployments for references
    keys, _ := s.store.List(ctx, types.KeyDeployments)
    for _, key := range keys {
        var dep types.DeploymentRecord
        if err := s.store.Get(ctx, key, &dep); err != nil { continue }
        if dep.Status != types.StatusRunning { continue }
        for svcName, svc := range dep.Services {
            for _, ref := range svc.Secrets {
                if ref == req.Name {
                    return nil, status.Errorf(codes.FailedPrecondition,
                        "cannot delete secret %q: referenced by deployment %q (service: %s)",
                        req.Name, dep.Name, svcName)
                }
            }
        }
    }
    // Safe to delete
    if err := s.engine.secrets.Delete(ctx, req.Name); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to delete secret: %v", err)
    }
    return &banyanpb.DeleteSecretResponse{}, nil
}
```

#### 4. PollTasks: just-in-time secret resolution
**File**: `pkg/engine/grpc_server.go` — PollTasks handler

After building the proto TaskRecord, resolve secrets before returning:

```go
// In PollTasks, after building pbTask:
if len(task.SecretRefs) > 0 && s.engine.secrets != nil {
    resolved, err := s.engine.secrets.ResolveSecrets(ctx, task.SecretRefs)
    if err != nil {
        s.logger().Warn("Failed to resolve secrets for task", "task", task.ID, "error", err)
    } else {
        pbTask.ResolvedSecrets = resolved
    }
}
```

### Success Criteria:

#### Automated:
- [ ] `make generate` in pkg/rpc succeeds
- [ ] `go build ./pkg/engine/...` compiles
- [ ] `go build ./pkg/rpc/...` compiles
- [ ] `go test ./pkg/engine/ -run TestCreateSecret` passes
- [ ] `go test ./pkg/engine/ -run TestDeleteSecretInUse` passes

---

## Phase 3: CLI Commands

### Overview
Add `banyan-cli secret` subcommand with create/list/get/delete operations.

### Changes Required:

#### 1. Secret command
**File**: `cmd/banyan-cli/cmd/secret.go` (NEW)

```go
var secretCmd = &cobra.Command{
    Use:   "secret",
    Short: "Manage secrets",
}

var secretCreateCmd = &cobra.Command{
    Use:   "create <name>",
    Short: "Create or update a secret",
    Args:  cobra.ExactArgs(1),
    RunE:  runSecretCreate,
}

var secretListCmd = &cobra.Command{
    Use:   "list",
    Short: "List all secrets",
    RunE:  runSecretList,
}

var secretGetCmd = &cobra.Command{
    Use:   "get <name>",
    Short: "Show secret metadata (use --reveal for value)",
    Args:  cobra.ExactArgs(1),
    RunE:  runSecretGet,
}

var secretDeleteCmd = &cobra.Command{
    Use:   "delete <name>",
    Short: "Delete a secret",
    Args:  cobra.ExactArgs(1),
    RunE:  runSecretDelete,
}

func init() {
    rootCmd.AddCommand(secretCmd)
    secretCmd.AddCommand(secretCreateCmd)
    secretCmd.AddCommand(secretListCmd)
    secretCmd.AddCommand(secretGetCmd)
    secretCmd.AddCommand(secretDeleteCmd)

    secretCreateCmd.Flags().String("from-file", "", "Read secret value from file")
    secretCreateCmd.Flags().String("value", "", "Secret value (prefer stdin or --from-file for security)")
    secretGetCmd.Flags().Bool("reveal", false, "Show the secret value (not just metadata)")
}
```

`runSecretCreate` flow:
1. If `--from-file`: read file content as value
2. If `--value`: use flag value (warn: visible in shell history)
3. Otherwise: prompt with hidden input (`term.ReadPassword`)
4. Call `CreateSecret` RPC

```
$ banyan-cli secret create DB_PASSWORD
Enter secret value: ********
Secret "DB_PASSWORD" created.

$ banyan-cli secret create API_KEY --from-file ./api-key.txt
Secret "API_KEY" created.
```

`runSecretList` output:
```
NAME                 CREATED              UPDATED
-------------------------------------------------------------
DB_PASSWORD          2h ago               2h ago
API_KEY              1d ago               5m ago
REDIS_AUTH           3d ago               3d ago
```

`runSecretGet` output:
```
$ banyan-cli secret get DB_PASSWORD
Secret: DB_PASSWORD
  Created:  2026-03-27 10:00:00 UTC
  Updated:  2026-03-27 10:00:00 UTC

$ banyan-cli secret get DB_PASSWORD --reveal
Secret: DB_PASSWORD
  Created:  2026-03-27 10:00:00 UTC
  Updated:  2026-03-27 10:00:00 UTC
  Value:    s3cret-passw0rd
```

`runSecretDelete` output:
```
$ banyan-cli secret delete DB_PASSWORD
Secret "DB_PASSWORD" deleted.

$ banyan-cli secret delete DB_PASSWORD  # if in use
Error: cannot delete secret "DB_PASSWORD": referenced by deployment "my-app" (service: api)
```

#### 2. Client methods
**File**: `cmd/banyan-cli/cmd/client.go`

Add wrapper methods:

```go
func (c *EngineClient) CreateSecret(ctx context.Context, name string, value []byte) error
func (c *EngineClient) ListSecrets(ctx context.Context) ([]*banyanpb.SecretInfo, error)
func (c *EngineClient) GetSecret(ctx context.Context, name string, reveal bool) (*banyanpb.GetSecretResponse, error)
func (c *EngineClient) DeleteSecret(ctx context.Context, name string) error
```

### Success Criteria:

#### Automated:
- [ ] `go build ./cmd/banyan-cli/...` compiles
- [ ] `banyan-cli secret --help` shows subcommands
- [ ] `go test ./cmd/banyan-cli/cmd/ -run TestSecret` passes

#### Manual:
- [ ] `banyan-cli secret create` prompts for value when no flags given
- [ ] `banyan-cli secret create --from-file` reads file correctly
- [ ] `banyan-cli secret list` displays table
- [ ] `banyan-cli secret get --reveal` shows value
- [ ] `banyan-cli secret delete` of in-use secret shows error

---

## Phase 4: Manifest Integration & Agent Injection

### Overview
Add `secrets:` field to manifests, validate at deploy time, and inject into containers.

### Changes Required:

#### 1. Manifest types
**File**: `pkg/types/manifest.go`

Add to `ManifestService`:
```go
type ManifestService struct {
    // ... existing fields ...
    Secrets []string `yaml:"secrets,omitempty"` // secret names to inject as env vars
}
```

#### 2. Record types
**File**: `pkg/types/records.go`

Add to `ServiceRecord`:
```go
type ServiceRecord struct {
    // ... existing fields ...
    Secrets []string `json:"secrets,omitempty"` // secret names referenced
}
```

Add to `TaskRecord`:
```go
type TaskRecord struct {
    // ... existing fields ...
    SecretRefs []string `json:"secret_refs,omitempty"` // secret names (values resolved at PollTasks time)
}
```

#### 3. BuildServiceRecords
**File**: `pkg/types/helpers.go`

Copy `Secrets` from ManifestService to ServiceRecord:
```go
services[name] = ServiceRecord{
    // ... existing fields ...
    Secrets: svc.Secrets,
}
```

#### 4. BuildTasksForDeployment
**File**: `pkg/types/helpers.go`

Copy `Secrets` from ServiceRecord to TaskRecord:
```go
task := &TaskRecord{
    // ... existing fields ...
    SecretRefs: svc.Secrets,
}
```

#### 5. Deploy-time validation
**File**: `pkg/engine/grpc_server.go` — Deploy handler

After building service records, validate all referenced secrets exist:

```go
// Validate secret references
if s.engine.secrets != nil {
    for svcName, svc := range allServices {
        for _, secretName := range svc.Secrets {
            if _, err := s.engine.secrets.Get(ctx, secretName); err != nil {
                return nil, status.Errorf(codes.InvalidArgument,
                    "service %q references secret %q which does not exist. Create it with: banyan-cli secret create %s",
                    svcName, secretName, secretName)
            }
        }
    }
}
```

If secrets are referenced but `secrets.key` is missing:
```go
if s.engine.secrets == nil {
    for _, svc := range allServices {
        if len(svc.Secrets) > 0 {
            return nil, status.Errorf(codes.FailedPrecondition,
                "manifest references secrets but secrets encryption is not enabled (missing secrets.key on engine)")
        }
    }
}
```

#### 6. Scale handler
**File**: `pkg/engine/autoscale.go` — `scaleService()`

Copy `Secrets` (from ServiceRecord) to new TaskRecords created during scale-up:
```go
task := &types.TaskRecord{
    // ... existing fields ...
    SecretRefs: svc.Secrets,
}
```

#### 7. Proto serialization — CLI side
**File**: `cmd/banyan-cli/cmd/client.go` — `manifestToProto()`

```go
ms.Secrets = svc.Secrets
```

#### 8. Proto deserialization — Engine side
**File**: `pkg/engine/grpc_server.go` — `protoToManifest()`

```go
ms.Secrets = svc.Secrets
```

#### 9. PollTasks — populate resolved_secrets and secret_refs
**File**: `pkg/engine/grpc_server.go` — PollTasks handler

When building proto TaskRecord:
```go
pbTask.SecretRefs = task.SecretRefs
// Resolve secrets just-in-time (values never stored in etcd)
if len(task.SecretRefs) > 0 && s.engine.secrets != nil {
    resolved, err := s.engine.secrets.ResolveSecrets(ctx, task.SecretRefs)
    if err != nil {
        s.logger().Warn("Failed to resolve secrets", "task", task.ID, "error", err)
    } else {
        pbTask.ResolvedSecrets = resolved
    }
}
```

#### 10. Agent — merge secrets into nerdctl args
**File**: `pkg/agent/agent.go`

In `pbTaskToLocal()`, store resolved secrets:
```go
task.ResolvedSecrets = pb.ResolvedSecrets  // map[string]string, in-memory only
```

Add `ResolvedSecrets map[string]string` to `TaskRecord` (runtime only, not JSON-serialized):
```go
type TaskRecord struct {
    // ... existing fields ...
    ResolvedSecrets map[string]string `json:"-"` // runtime only, never persisted
}
```

In `buildNerdctlRunArgs()`, inject secrets as env vars (after regular environment):
```go
// Inject resolved secrets as environment variables
for name, value := range task.ResolvedSecrets {
    args = append(args, "-e", name+"="+value)
}
```

Secrets override regular environment (if collision):
```go
// Build env map: environment first, then secrets override
envMap := make(map[string]string)
for _, env := range task.Environment {
    parts := strings.SplitN(env, "=", 2)
    if len(parts) == 2 {
        envMap[parts[0]] = parts[1]
    }
}
for name, value := range task.ResolvedSecrets {
    envMap[name] = value  // secret overrides env var
}
// Convert back to args
for name, value := range envMap {
    args = append(args, "-e", name+"="+value)
}
```

Wait — this changes the existing env var injection. Instead, keep existing env var injection as-is and just append secrets after. nerdctl uses the last `-e` value for duplicate keys, so secrets naturally override.

Simpler approach:
```go
// Existing: regular environment variables
for _, env := range task.Environment {
    args = append(args, "-e", env)
}
// Secrets override (nerdctl uses last value for duplicates)
for name, value := range task.ResolvedSecrets {
    args = append(args, "-e", name+"="+value)
}
```

### Success Criteria:

#### Automated:
- [ ] `go build ./...` compiles across all modules
- [ ] `go test ./pkg/types/ -run TestSecrets` — manifest parsing with secrets field
- [ ] `go test ./pkg/engine/ -run TestDeployWithSecrets` — deploy validation
- [ ] `go test ./pkg/engine/ -run TestDeployMissingSecret` — error on unknown secret
- [ ] `go test ./pkg/agent/ -run TestBuildNerdctlRunArgsSecrets` — secret injection

#### Manual:
- [ ] Deploy with `secrets:` field, verify container has env var via `nerdctl exec <container> env`
- [ ] Deploy with missing secret reference, verify clear error message

---

## Phase 5: Tests & Documentation

### Overview
Comprehensive unit tests, E2E test, and documentation updates.

### Unit Tests:

#### `pkg/engine/secrets_test.go` (NEW)

| Test | What it verifies |
|------|-----------------|
| `TestGenerateSecretsKey` | Key file created with correct size and permissions |
| `TestLoadSecretsKey` | Key loaded, wrong size rejected |
| `TestEncryptDecrypt` | Round-trip: encrypt → decrypt returns original |
| `TestEncryptDifferentNonce` | Same plaintext produces different ciphertext |
| `TestDecryptTampered` | Modified ciphertext fails authentication |
| `TestCreateSecret` | Secret stored in etcd encrypted |
| `TestCreateSecretDuplicate` | Error on duplicate name |
| `TestUpdateSecret` | Value replaced, timestamps updated |
| `TestGetSecret` | Decrypt and return correct value |
| `TestGetSecretNotFound` | Error on missing secret |
| `TestDeleteSecret` | Secret removed from etcd |
| `TestListSecrets` | Returns all names with metadata (no values) |
| `TestResolveSecrets` | Multiple secrets resolved to name→value map |
| `TestResolveSecretsMissing` | Error when referenced secret doesn't exist |
| `TestValidateSecretName` | Valid names pass, invalid names rejected |
| `TestValidateSecretNameInvalid` | Names with dashes, dots, spaces, starting with digit rejected |
| `TestDeleteSecretInUse` | Blocked when referenced by running deployment |
| `TestPollTasksResolvesSecrets` | PollTasks populates resolved_secrets field |
| `TestDeployValidatesSecrets` | Deploy fails when referencing nonexistent secret |
| `TestDeployNoSecretsKey` | Deploy with secrets refs fails if secrets.key missing |

#### `pkg/agent/agent_test.go` (additions)

| Test | What it verifies |
|------|-----------------|
| `TestBuildNerdctlRunArgsWithSecrets` | Secrets added as `-e` flags |
| `TestBuildNerdctlRunArgsSecretsOverrideEnv` | Secret `-e` appears after env `-e` |

### E2E Test:

**File**: `test/e2e/run-secrets-e2e.sh` (NEW)

```bash
# Phase 1: Create secrets
banyan-cli secret create DB_PASSWORD --value "test-password-123"
banyan-cli secret create API_KEY --value "key-abc-456"

# Phase 2: List secrets (verify created)
banyan-cli secret list  # expect 2 secrets

# Phase 3: Get secret metadata
banyan-cli secret get DB_PASSWORD  # shows dates, no value

# Phase 4: Deploy with secrets reference
banyan-cli up -f /examples/banyan-secrets.yaml
# Wait for deployment

# Phase 5: Verify container has secret as env var
nerdctl exec <container> printenv DB_PASSWORD  # expect "test-password-123"
nerdctl exec <container> printenv API_KEY      # expect "key-abc-456"

# Phase 6: Try deleting in-use secret (should fail)
banyan-cli secret delete DB_PASSWORD  # expect error

# Phase 7: Tear down, then delete
banyan-cli down --name e2e-secrets-test
banyan-cli secret delete DB_PASSWORD  # now succeeds

# Phase 8: Deploy with missing secret (should fail)
banyan-cli secret delete API_KEY
banyan-cli up -f /examples/banyan-secrets.yaml  # expect error about API_KEY
```

**File**: `test/e2e/examples/banyan-secrets.yaml` (NEW)

```yaml
name: e2e-secrets-test

services:
  app:
    image: alpine:latest
    command: ["sleep", "3600"]
    environment:
      - APP_NAME=secrets-test
    secrets:
      - DB_PASSWORD
      - API_KEY
```

### Documentation:

#### Manifest reference (`website/src/content/docs/reference/manifest.md`)
- Add `secrets` to Docker Compose comparison table (Banyan-specific)
- Add `secrets` to Service fields table
- Add "Secrets" section with syntax, examples, and behavior

#### CLI reference (`website/src/content/docs/reference/cli.md`)
- Add `secret` command group with create/list/get/delete subcommands
- Add to CLI binary table summary

#### New guide (`website/src/content/docs/guides/secrets.md`)
- Creating and managing secrets
- Referencing in manifests
- Security model (what's encrypted, what isn't)
- HA considerations (sharing secrets.key)
- Limitations (env var only, no auto-rotation)

#### Roadmap (`website/src/content/docs/roadmap.md`)
- Mark M11 as Done

#### Homepage (`website/src/content/docs/index.mdx`)
- Update feature cards to mention secrets

### Success Criteria:

#### Automated:
- [ ] All unit tests pass: `go test ./pkg/engine/ ./pkg/agent/ ./pkg/types/`
- [ ] E2E test passes: `./test/e2e/run-secrets-e2e.sh`
- [ ] Website builds: `cd website && npm run build`
- [ ] Code coverage for `pkg/engine/secrets.go` > 90%

#### Manual:
- [ ] Full workflow: create secret → deploy → verify in container → update → redeploy → delete
- [ ] HA: secrets created on engine-1 accessible from engine-2 (shared etcd + secrets.key)
- [ ] Error messages are actionable for every failure mode

---

## Testing Strategy

### Unit Tests:
- Encryption round-trip (encrypt/decrypt)
- Each CRUD operation on SecretsManager
- Name validation (valid/invalid)
- Delete-in-use blocking
- PollTasks secret resolution
- Deploy-time validation
- Agent nerdctl args with secrets

### Integration/E2E Tests:
- Full lifecycle: create → deploy → verify → update → redeploy → delete
- Error cases: missing secret, delete in use, no secrets key

### Edge Cases:
- Empty secret value (allowed — some tokens are empty strings)
- Very long secret value (up to etcd 1.5MB limit)
- Secret name collision with environment variable (secret wins)
- Deploy with secrets but no secrets.key on engine (clear error)
- Agent receives task with secrets but engine secrets manager was disabled between deploy and poll (graceful degradation)

## HA Considerations

- `secrets.key` must be present on ALL engine nodes (same file)
- Secrets stored in shared etcd (encrypted)
- Any engine can create/read/delete secrets
- Document in HA guide: copy `secrets.key` alongside etcd config

## References

- Current env var flow: `pkg/types/envfile.go`, `cmd/banyan-cli/cmd/deploy.go:152`
- etcd storage interface: `pkg/storage/interface.go`
- Key management pattern: `pkg/types/config.go:204-228` (WritePrivateKeyFile/ReadPrivateKeyFile)
- Proto definitions: `pkg/rpc/proto/banyan/v1/engine.proto`
- PollTasks handler: `pkg/engine/grpc_server.go:479-543`
- Agent nerdctl args: `pkg/agent/agent.go:496-595`
