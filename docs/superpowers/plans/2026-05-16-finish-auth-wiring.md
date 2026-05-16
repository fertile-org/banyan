# Finish Auth Wiring + E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the dormant M13 auth package to the running engine so authentication is actually enforced, then update the E2E test suite to exercise the real auth flow.

**Architecture:** The `pkg/engine/auth/` package (JWT, UserStore, RoleAuthorizer, middleware) is fully built and tested but never instantiated by the engine. This plan adds a `setupEngineAuth()` orchestrator that generates/loads the JWT signing key, consumes the `auth-bootstrap.json` admin seed written by `banyan-engine init`, and constructs an `auth.AuthDeps` bundle. That bundle is passed into `startEngineGRPC()`, which already knows how to install the auth interceptors when `AuthDeps != nil`. The CLI and engine init command get non-interactive flags so the Docker-based E2E suite can drive them without a TTY.

**Tech Stack:** Go 1.25, gRPC, etcd (`storage.StateStore`), Cobra CLI, `huh` TUI forms, Docker Compose E2E.

**Scope boundary — explicitly OUT of scope:**
- TLS transport wiring (gRPC `grpc.Creds`, CLI CA trust). The `tls.go` code exists and `banyan-engine init` already generates certs, but wiring TLS into the transport is a separate plan. JWT auth in this plan runs over the existing WireGuard-encrypted transport.
- CORS origin lockdown. `startConnectAPI` keeps `nil` origins (allow-all). The JWT middleware still protects every endpoint.
- Dashboard user-management page.

**Key design decision — JWT key storage:** The design doc said "encrypt the JWT key via SecretsManager." This plan instead stores the 32-byte HMAC key directly in etcd at `auth/jwt-key` (constant `types.KeyAuthJWTKey`, already defined), JSON-wrapped, unencrypted. Rationale: (1) etcd runs localhost-only on the engine, (2) the JWT key never leaves the engine — unlike container secrets which travel to agents, (3) `SecretsManager` can be `nil` when `secrets.key` is absent, and depending on it would create two code paths, (4) storing it as a named secret would leak its existence into `banyan secret list`. An attacker with localhost etcd access on the engine box already owns the engine. If you disagree with this deviation, stop here and discuss before implementing.

---

## File Structure

| File | Responsibility |
|------|----------------|
| `pkg/engine/auth_setup.go` (new) | JWT key lifecycle, bootstrap consumer, `setupEngineAuth()` orchestrator |
| `pkg/engine/auth_setup_test.go` (new) | Unit tests for the above |
| `pkg/engine/engine.go` (modify) | Add `ConfigDir` to `Options`; call `setupEngineAuth()` in `Run()`; pass `AuthDeps` to `startEngineGRPC` |
| `cmd/banyan-engine/cmd/engine.go` (modify) | Pass `ConfigDir` to `engine.New`; add `--non-interactive`/`--admin-user`/`--admin-password` init flags; gate the admin `huh` form |
| `cmd/banyan-cli/cmd/login.go` (modify) | Add `--username`/`--password` flags to `banyan login` for non-interactive use |
| `test/e2e/scripts/engine-entrypoint.sh` (modify) | Run init non-interactively; log in after engine is ready |
| `test/e2e/docker-compose.yml` (modify) | Set `HOME=/root` on the engine service; bump healthcheck `start_period` |
| `test/e2e/run-e2e.sh` (modify) | Add an early `banyan-cli whoami` sanity check that fails fast if auth is broken |

---

## Task 1: JWT signing key lifecycle

**Files:**
- Create: `pkg/engine/auth_setup.go`
- Create: `pkg/engine/auth_setup_test.go`

- [ ] **Step 1: Write the failing test**

Create `pkg/engine/auth_setup_test.go`:

```go
package engine

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/storage"
)

func TestLoadOrCreateJWTKey_GeneratesOnFirstCall(t *testing.T) {
	store := storage.NewMemoryStore()
	key, err := loadOrCreateJWTKey(context.Background(), store)
	if err != nil {
		t.Fatalf("loadOrCreateJWTKey: %v", err)
	}
	if len(key) != jwtKeySize {
		t.Errorf("key length = %d, want %d", len(key), jwtKeySize)
	}
}

func TestLoadOrCreateJWTKey_StableAcrossCalls(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	key1, err := loadOrCreateJWTKey(ctx, store)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	key2, err := loadOrCreateJWTKey(ctx, store)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(key1) != string(key2) {
		t.Error("key should be stable across calls — second call regenerated it")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestLoadOrCreateJWTKey -v`
Expected: FAIL — `undefined: loadOrCreateJWTKey`, `undefined: jwtKeySize`

- [ ] **Step 3: Write minimal implementation**

Create `pkg/engine/auth_setup.go`:

```go
// auth_setup.go wires the pkg/engine/auth package into the running engine:
// it generates/loads the JWT signing key, consumes the init-time admin
// bootstrap file, and assembles the auth.AuthDeps bundle the gRPC and
// Connect-go servers need to enforce authentication.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/engine/auth"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// jwtKeySize is the byte length of the HMAC-SHA256 signing key.
const jwtKeySize = 32

// jwtKeyRecord is the etcd-stored wrapper for the JWT signing key.
type jwtKeyRecord struct {
	Key []byte `json:"key"`
}

// loadOrCreateJWTKey returns the engine's JWT signing key, generating and
// persisting a new one in etcd at types.KeyAuthJWTKey on first start.
func loadOrCreateJWTKey(ctx context.Context, store storage.StateStore) ([]byte, error) {
	var rec jwtKeyRecord
	if err := store.Get(ctx, types.KeyAuthJWTKey, &rec); err == nil && len(rec.Key) == jwtKeySize {
		return rec.Key, nil
	}

	key := make([]byte, jwtKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate JWT signing key: %w", err)
	}
	if err := store.Save(ctx, types.KeyAuthJWTKey, &jwtKeyRecord{Key: key}); err != nil {
		return nil, fmt.Errorf("failed to persist JWT signing key: %w", err)
	}
	return key, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestLoadOrCreateJWTKey -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add pkg/engine/auth_setup.go pkg/engine/auth_setup_test.go
git commit -m "feat(engine): add JWT signing key lifecycle"
```

---

## Task 2: Admin bootstrap consumer

**Files:**
- Modify: `pkg/engine/auth_setup.go`
- Modify: `pkg/engine/auth_setup_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/auth_setup_test.go`:

```go
func writeBootstrap(t *testing.T, dir string) string {
	t.Helper()
	// password_hash is a real bcrypt hash of "password123"
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	path := filepath.Join(dir, "auth-bootstrap.json")
	data := []byte(`{"username":"admin","password_hash":"` + hash + `","role":"admin"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	return path
}

func TestConsumeAuthBootstrap_CreatesAdminWhenNoUsers(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	path := writeBootstrap(t, t.TempDir())

	if err := consumeAuthBootstrap(context.Background(), users, path); err != nil {
		t.Fatalf("consumeAuthBootstrap: %v", err)
	}

	u, err := users.Get(context.Background(), "admin")
	if err != nil {
		t.Fatalf("admin user not created: %v", err)
	}
	if u.Role != auth.RoleAdmin {
		t.Errorf("admin role = %q, want %q", u.Role, auth.RoleAdmin)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("bootstrap file should be deleted after consumption")
	}
}

func TestConsumeAuthBootstrap_SkipsWhenUsersExist(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	ctx := context.Background()

	// Pre-existing user
	hash, _ := auth.HashPassword("existing")
	if err := users.Create(ctx, &auth.User{
		Username: "existing-admin", PasswordHash: hash, Role: auth.RoleAdmin,
	}); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	path := writeBootstrap(t, t.TempDir())
	if err := consumeAuthBootstrap(ctx, users, path); err != nil {
		t.Fatalf("consumeAuthBootstrap: %v", err)
	}

	// admin from bootstrap must NOT have been created
	if _, err := users.Get(ctx, "admin"); err == nil {
		t.Error("bootstrap admin should not be created when users already exist")
	}
	// stale bootstrap file should still be removed
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("stale bootstrap file should be deleted")
	}
}

func TestConsumeAuthBootstrap_NoFileIsNoOp(t *testing.T) {
	store := storage.NewMemoryStore()
	users := auth.NewEtcdUserStore(store)
	missing := filepath.Join(t.TempDir(), "auth-bootstrap.json")

	if err := consumeAuthBootstrap(context.Background(), users, missing); err != nil {
		t.Errorf("missing bootstrap file should be a no-op, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestConsumeAuthBootstrap -v`
Expected: FAIL — `undefined: consumeAuthBootstrap`

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/engine/auth_setup.go`:

```go
// bootstrapRecord mirrors the JSON written by `banyan-engine init` to
// auth-bootstrap.json. The password is already bcrypt-hashed by init.
type bootstrapRecord struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Role         string `json:"role"`
}

// consumeAuthBootstrap reads the init-time admin seed file and creates the
// first admin user if no users exist yet. The file is always removed after
// processing so it is consumed exactly once. A missing file is a no-op.
func consumeAuthBootstrap(ctx context.Context, users auth.UserStore, bootstrapPath string) error {
	data, err := os.ReadFile(bootstrapPath)
	if err != nil {
		return nil // no bootstrap file — nothing to do
	}

	var rec bootstrapRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return fmt.Errorf("invalid auth bootstrap file %s: %w", bootstrapPath, err)
	}

	existing, err := users.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to check existing users: %w", err)
	}
	if len(existing) > 0 {
		// Users already exist — the bootstrap seed is stale. Remove it.
		_ = os.Remove(bootstrapPath)
		return nil
	}

	if err := users.Create(ctx, &auth.User{
		Username:     rec.Username,
		PasswordHash: rec.PasswordHash,
		Role:         rec.Role,
		CreatedBy:    "init",
	}); err != nil {
		return fmt.Errorf("failed to create bootstrap admin user: %w", err)
	}
	_ = os.Remove(bootstrapPath)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestConsumeAuthBootstrap -v`
Expected: PASS (all three tests)

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add pkg/engine/auth_setup.go pkg/engine/auth_setup_test.go
git commit -m "feat(engine): consume admin bootstrap seed on first start"
```

---

## Task 3: setupEngineAuth orchestrator

**Files:**
- Modify: `pkg/engine/auth_setup.go`
- Modify: `pkg/engine/auth_setup_test.go`

- [ ] **Step 1: Write the failing test**

Append to `pkg/engine/auth_setup_test.go`:

```go
func TestSetupEngineAuth_InsecureReturnsNil(t *testing.T) {
	store := storage.NewMemoryStore()
	deps, err := setupEngineAuth(context.Background(), store, t.TempDir(), true)
	if err != nil {
		t.Fatalf("setupEngineAuth: %v", err)
	}
	if deps != nil {
		t.Error("allowInsecure=true should disable auth (nil AuthDeps)")
	}
}

func TestSetupEngineAuth_BuildsDepsAndCreatesAdmin(t *testing.T) {
	store := storage.NewMemoryStore()
	dir := t.TempDir()
	writeBootstrap(t, dir)

	deps, err := setupEngineAuth(context.Background(), store, dir, false)
	if err != nil {
		t.Fatalf("setupEngineAuth: %v", err)
	}
	if deps == nil {
		t.Fatal("expected non-nil AuthDeps when auth is enabled")
	}
	if deps.JWT == nil || deps.Users == nil || deps.Authorizer == nil {
		t.Error("AuthDeps fields must all be populated")
	}

	// The bootstrap admin should now exist via the constructed UserStore
	if _, err := deps.Users.Get(context.Background(), "admin"); err != nil {
		t.Errorf("bootstrap admin not created: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestSetupEngineAuth -v`
Expected: FAIL — `undefined: setupEngineAuth`

- [ ] **Step 3: Write minimal implementation**

Append to `pkg/engine/auth_setup.go`:

```go
// setupEngineAuth assembles the auth.AuthDeps bundle the engine's gRPC and
// Connect-go servers use to enforce authentication. It loads (or generates)
// the JWT signing key and consumes the init-time admin bootstrap file.
//
// Returns (nil, nil) when allowInsecure is true — auth is fully disabled,
// matching the --allow-insecure development escape hatch.
func setupEngineAuth(ctx context.Context, store storage.StateStore, configDir string, allowInsecure bool) (*auth.AuthDeps, error) {
	if allowInsecure {
		return nil, nil
	}

	jwtKey, err := loadOrCreateJWTKey(ctx, store)
	if err != nil {
		return nil, err
	}

	users := auth.NewEtcdUserStore(store)
	if err := consumeAuthBootstrap(ctx, users, filepath.Join(configDir, "auth-bootstrap.json")); err != nil {
		return nil, err
	}

	return &auth.AuthDeps{
		JWT:        auth.NewJWTManager(jwtKey, store),
		Users:      users,
		Authorizer: auth.NewRoleAuthorizer(),
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ -run TestSetupEngineAuth -v`
Expected: PASS (both tests)

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add pkg/engine/auth_setup.go pkg/engine/auth_setup_test.go
git commit -m "feat(engine): add setupEngineAuth orchestrator"
```

---

## Task 4: Wire setupEngineAuth into engine.Run()

**Files:**
- Modify: `pkg/engine/engine.go` (Options struct ~line 25-46; Run() ~line 235)

- [ ] **Step 1: Add ConfigDir to the Options struct**

In `pkg/engine/engine.go`, find the `Options` struct. After the line:

```go
	MultiEngine         bool              // enable multi-engine coordination
```

add (still inside the struct, before the closing `}`):

```go
	ConfigDir           string            // directory holding banyan.yaml + auth-bootstrap.json (default /etc/banyan)
```

- [ ] **Step 2: Call setupEngineAuth in Run() and pass AuthDeps**

In `pkg/engine/engine.go`, find this block in `Run()`:

```go
	grpcBindAddr := "127.0.0.1"
	if e.opts.ControlTunnelActive {
		grpcBindAddr = e.opts.TunnelIP
	} else if e.opts.AllowInsecure && e.multiEngine {
		grpcBindAddr = "0.0.0.0"
	}
	grpcSrv, err := startEngineGRPC(ctx, &grpcServerOptions{
```

Replace it with:

```go
	// Set up application-layer authentication (JWT + RBAC).
	// Returns nil AuthDeps in --allow-insecure mode.
	configDir := e.opts.ConfigDir
	if configDir == "" {
		configDir = "/etc/banyan"
	}
	authDeps, authErr := setupEngineAuth(ctx, e.store, configDir, e.opts.AllowInsecure)
	if authErr != nil {
		return fmt.Errorf("failed to set up authentication: %w", authErr)
	}
	if authDeps != nil {
		e.logger().Info("Application-layer authentication enabled (JWT + RBAC)")
	} else {
		e.logger().Warn("Application-layer authentication DISABLED (--allow-insecure)")
	}

	grpcBindAddr := "127.0.0.1"
	if e.opts.ControlTunnelActive {
		grpcBindAddr = e.opts.TunnelIP
	} else if e.opts.AllowInsecure && e.multiEngine {
		grpcBindAddr = "0.0.0.0"
	}
	grpcSrv, err := startEngineGRPC(ctx, &grpcServerOptions{
```

- [ ] **Step 3: Add the AuthDeps field to the grpcServerOptions literal**

Still in `Run()`, in the `&grpcServerOptions{...}` literal that follows, find the line:

```go
		Secrets:         e.secrets,
```

and add immediately after it:

```go
		AuthDeps:        authDeps,
```

- [ ] **Step 4: Run build + full engine test suite to verify nothing broke**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go build ./pkg/engine/... && go test ./pkg/engine/ ./pkg/engine/auth/ -count=1`
Expected: build succeeds; all tests PASS (existing engine tests + new auth_setup tests + auth package tests)

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add pkg/engine/engine.go
git commit -m "feat(engine): enforce JWT auth — wire AuthDeps into gRPC + Connect servers"
```

---

## Task 5: Pass ConfigDir from the engine start command

**Files:**
- Modify: `cmd/banyan-engine/cmd/engine.go` (the `engine.New(&engine.Options{...})` call ~line 788)

- [ ] **Step 1: Add ConfigDir to the Options literal**

In `cmd/banyan-engine/cmd/engine.go`, find the `engine.New(&engine.Options{` literal inside `runEngineStart`. Find the line:

```go
		MultiEngine:         cfg.Engine.MultiEngine,
```

and add immediately after it:

```go
		ConfigDir:           filepath.Dir(configPath),
```

(`filepath` is already imported in this file; `configPath` is the package-level var set to `types.DefaultConfigPath`.)

- [ ] **Step 2: Run build to verify it compiles**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go build ./cmd/banyan-engine/...`
Expected: build succeeds

- [ ] **Step 3: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add cmd/banyan-engine/cmd/engine.go
git commit -m "feat(engine-cmd): pass ConfigDir so engine finds auth-bootstrap.json"
```

---

## Task 6: Non-interactive engine init flags

**Files:**
- Modify: `cmd/banyan-engine/cmd/engine.go` (the `init()` func ~line 118; the admin-form block inside `runEngineInit`)

- [ ] **Step 1: Register the three new flags**

In `cmd/banyan-engine/cmd/engine.go`, find the `init()` function. After this existing line:

```go
	startCmd.Flags().BoolVar(&engineAllowInsecure, "allow-insecure", false, "Allow running without authentication (development only, NOT for production)")
```

add:

```go
	initCmd.Flags().Bool("non-interactive", false, "Run init without interactive prompts (requires --admin-user and --admin-password)")
	initCmd.Flags().String("admin-user", "", "Admin username (non-interactive mode)")
	initCmd.Flags().String("admin-password", "", "Admin password (non-interactive mode, min 8 chars)")
```

- [ ] **Step 2: Replace the admin-form block with flag-aware logic**

In `cmd/banyan-engine/cmd/engine.go`, inside `runEngineInit`, find this exact block:

```go
	// --- Auth setup (admin user) ---
	fmt.Println()
	fmt.Println(styleTitle.Render("=== Authentication Setup ==="))
	fmt.Println(styleDim.Render("  Create an admin user for Banyan."))
	fmt.Println(styleDim.Render("  All CLI and dashboard access requires authentication."))
	fmt.Println()

	var adminUsername, adminPassword string
	adminForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Admin username").Value(&adminUsername).Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("username cannot be empty")
				}
				return nil
			}),
			huh.NewInput().Title("Admin password").EchoMode(huh.EchoModePassword).Value(&adminPassword).Validate(func(s string) error {
				if len(s) < 8 {
					return fmt.Errorf("password must be at least 8 characters")
				}
				return nil
			}),
		),
	)
	if err := adminForm.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println(styleWarn.Render("  Skipped admin user setup. You can create users later with: banyan user add"))
		} else {
			return fmt.Errorf("admin user form error: %w", err)
		}
	} else {
		hash, hashErr := auth.HashPassword(adminPassword)
		if hashErr != nil {
			return fmt.Errorf("failed to hash password: %w", hashErr)
		}
		// Store admin credentials in a bootstrap file for engine to read on first start
		bootstrapPath := filepath.Join(filepath.Dir(configPath), "auth-bootstrap.json")
		bootstrapData := fmt.Sprintf(`{"username":%q,"password_hash":%q,"role":"admin"}`, adminUsername, hash)
		if writeErr := os.WriteFile(bootstrapPath, []byte(bootstrapData), 0o600); writeErr != nil {
			return fmt.Errorf("failed to write auth bootstrap: %w", writeErr)
		}
		fmt.Printf("  %s Admin user %q configured\n", styleOK.Render("[OK]"), adminUsername)
		fmt.Println(styleDim.Render("  The admin account will be created when the engine starts."))
	}
```

Replace it entirely with:

```go
	// --- Auth setup (admin user) ---
	fmt.Println()
	fmt.Println(styleTitle.Render("=== Authentication Setup ==="))
	fmt.Println(styleDim.Render("  Create an admin user for Banyan."))
	fmt.Println(styleDim.Render("  All CLI and dashboard access requires authentication."))
	fmt.Println()

	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	flagAdminUser, _ := cmd.Flags().GetString("admin-user")
	flagAdminPass, _ := cmd.Flags().GetString("admin-password")

	var adminUsername, adminPassword string
	skipAdmin := false

	if nonInteractive {
		if flagAdminUser == "" || flagAdminPass == "" {
			return fmt.Errorf("--non-interactive requires --admin-user and --admin-password")
		}
		if len(flagAdminPass) < 8 {
			return fmt.Errorf("--admin-password must be at least 8 characters")
		}
		adminUsername = flagAdminUser
		adminPassword = flagAdminPass
	} else {
		adminForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("Admin username").Value(&adminUsername).Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("username cannot be empty")
					}
					return nil
				}),
				huh.NewInput().Title("Admin password").EchoMode(huh.EchoModePassword).Value(&adminPassword).Validate(func(s string) error {
					if len(s) < 8 {
						return fmt.Errorf("password must be at least 8 characters")
					}
					return nil
				}),
			),
		)
		if err := adminForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println(styleWarn.Render("  Skipped admin user setup. You can create users later with: banyan user add"))
				skipAdmin = true
			} else {
				return fmt.Errorf("admin user form error: %w", err)
			}
		}
	}

	if !skipAdmin {
		hash, hashErr := auth.HashPassword(adminPassword)
		if hashErr != nil {
			return fmt.Errorf("failed to hash password: %w", hashErr)
		}
		// Store admin credentials in a bootstrap file for engine to read on first start
		bootstrapPath := filepath.Join(filepath.Dir(configPath), "auth-bootstrap.json")
		bootstrapData := fmt.Sprintf(`{"username":%q,"password_hash":%q,"role":"admin"}`, adminUsername, hash)
		if writeErr := os.WriteFile(bootstrapPath, []byte(bootstrapData), 0o600); writeErr != nil {
			return fmt.Errorf("failed to write auth bootstrap: %w", writeErr)
		}
		fmt.Printf("  %s Admin user %q configured\n", styleOK.Render("[OK]"), adminUsername)
		fmt.Println(styleDim.Render("  The admin account will be created when the engine starts."))
	}
```

- [ ] **Step 3: Run build to verify it compiles**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go build ./cmd/banyan-engine/...`
Expected: build succeeds

- [ ] **Step 4: Manually verify the flag error path**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go run ./cmd/banyan-engine init --non-interactive 2>&1 | tail -3`
Expected: output contains `--non-interactive requires --admin-user and --admin-password` (it may print earlier wizard output or other errors first — only the flag-validation message matters; if the wizard exits earlier for an unrelated reason like missing root, that is acceptable for this check).

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add cmd/banyan-engine/cmd/engine.go
git commit -m "feat(engine-cmd): add non-interactive init flags for automated setup"
```

---

## Task 7: Non-interactive CLI login flags

**Files:**
- Modify: `cmd/banyan-cli/cmd/login.go`

- [ ] **Step 1: Register the flags in init()**

In `cmd/banyan-cli/cmd/login.go`, find the `init()` function:

```go
func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
}
```

Replace it with:

```go
func init() {
	loginCmd.Flags().String("username", "", "Username (non-interactive login)")
	loginCmd.Flags().String("password", "", "Password (non-interactive login)")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(whoamiCmd)
}
```

- [ ] **Step 2: Make runAuthLogin flag-aware**

In `cmd/banyan-cli/cmd/login.go`, find the start of `runAuthLogin`:

```go
func runAuthLogin(cmd *cobra.Command, args []string) error {
	fmt.Print("Username: ")
	var username string
	if _, err := fmt.Scanln(&username); err != nil {
		return fmt.Errorf("failed to read username: %w", err)
	}
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after hidden input
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password := string(passwordBytes)
	if password == "" {
		return fmt.Errorf("password cannot be empty")
	}
```

Replace that portion (down to and including the `password == ""` check) with:

```go
func runAuthLogin(cmd *cobra.Command, args []string) error {
	flagUser, _ := cmd.Flags().GetString("username")
	flagPass, _ := cmd.Flags().GetString("password")

	var username, password string

	if flagUser != "" && flagPass != "" {
		// Non-interactive: both flags supplied
		username = flagUser
		password = flagPass
	} else {
		fmt.Print("Username: ")
		if _, err := fmt.Scanln(&username); err != nil {
			return fmt.Errorf("failed to read username: %w", err)
		}
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}

		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // newline after hidden input
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		password = string(passwordBytes)
		if password == "" {
			return fmt.Errorf("password cannot be empty")
		}
	}
```

The rest of `runAuthLogin` (the `NewAutoEngineClient` call onward) is unchanged — it already uses the `username` and `password` variables.

- [ ] **Step 3: Run build to verify it compiles**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go build ./cmd/banyan-cli/...`
Expected: build succeeds

- [ ] **Step 4: Verify the CLI test suite still passes**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./cmd/banyan-cli/... -count=1`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add cmd/banyan-cli/cmd/login.go
git commit -m "feat(cli): add non-interactive login flags (--username/--password)"
```

---

## Task 8: Update the E2E engine entrypoint

**Files:**
- Modify: `test/e2e/scripts/engine-entrypoint.sh`

- [ ] **Step 1: Switch init to non-interactive mode**

In `test/e2e/scripts/engine-entrypoint.sh`, find:

```bash
# 2. Init engine (generates WireGuard keypair, no --password)
echo "Initializing engine..."
banyan-engine init
```

Replace with:

```bash
# 2. Init engine (generates WireGuard keypair + auth bootstrap, non-interactive)
echo "Initializing engine..."
E2E_ADMIN_USER="admin"
E2E_ADMIN_PASS="banyan-e2e-admin"
banyan-engine init \
    --non-interactive \
    --admin-user "$E2E_ADMIN_USER" \
    --admin-password "$E2E_ADMIN_PASS"
```

- [ ] **Step 2: Add a login step after the CLI control tunnel is ready**

In `test/e2e/scripts/engine-entrypoint.sh`, find the end of the CLI control tunnel section:

```bash
# Add engine as peer (engine listens on 127.0.0.1:51821 inside same container)
# Use engine's derived tunnel IP (not hardcoded 10.200.0.1)
wg set wg-ctl-cli peer "$ENGINE_PUB_KEY" allowed-ips ${ENGINE_TUNNEL_IP}/32 endpoint 127.0.0.1:51821
echo "CLI control tunnel ready."

# Wait for the engine process (keeps the container running)
wait $ENGINE_PID
```

Replace with:

```bash
# Add engine as peer (engine listens on 127.0.0.1:51821 inside same container)
# Use engine's derived tunnel IP (not hardcoded 10.200.0.1)
wg set wg-ctl-cli peer "$ENGINE_PUB_KEY" allowed-ips ${ENGINE_TUNNEL_IP}/32 endpoint 127.0.0.1:51821
echo "CLI control tunnel ready."

# 8c. Authenticate the CLI — every banyan-cli command needs a JWT session.
# Retry because the engine consumes the admin bootstrap shortly after start.
echo "Logging in CLI..."
LOGIN_OK=false
for i in $(seq 1 15); do
    if banyan-cli login --username "$E2E_ADMIN_USER" --password "$E2E_ADMIN_PASS" 2>/dev/null; then
        LOGIN_OK=true
        break
    fi
    echo "  login attempt $i failed, retrying..."
    sleep 2
done
if [ "$LOGIN_OK" = true ]; then
    echo "CLI authenticated."
else
    echo "ERROR: CLI login failed after 15 attempts" >&2
fi

# Wait for the engine process (keeps the container running)
wait $ENGINE_PID
```

- [ ] **Step 3: Verify the script is syntactically valid**

Run: `bash -n /home/hungnguyenba/workspace/fertile/banyan/test/e2e/scripts/engine-entrypoint.sh && echo "syntax OK"`
Expected: `syntax OK`

- [ ] **Step 4: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add test/e2e/scripts/engine-entrypoint.sh
git commit -m "test(e2e): non-interactive engine init + CLI login in entrypoint"
```

---

## Task 9: Update E2E docker-compose for stable CLI credentials

**Files:**
- Modify: `test/e2e/docker-compose.yml` (the `engine` service)

**Why:** `banyan login` writes credentials to `$HOME/.config/banyan/credentials.json`. The entrypoint and every `docker exec banyan-engine banyan-cli ...` call must resolve the same `$HOME`. Setting `HOME=/root` on the container guarantees `docker exec` processes inherit it. The healthcheck (`banyan-cli engine`, which calls the `GetDashboardData` RPC and now requires a token) reads the same credentials file. `start_period` is bumped so login completes before the healthcheck starts probing.

- [ ] **Step 1: Add HOME env and bump healthcheck start_period**

In `test/e2e/docker-compose.yml`, find the `engine` service block:

```yaml
  engine:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: banyan-engine
    hostname: engine
    privileged: true
    command: ["/entrypoint-engine.sh"]
    ports:
      - "50051:50051"
    networks:
      banyan-net:
        ipv4_address: 172.28.0.10
    volumes:
      - keys-exchange:/tmp/keys-exchange
      - engine-data:/var/lib/banyan
      - ./examples:/examples:ro
    healthcheck:
      test: ["CMD", "banyan-cli", "engine"]
      interval: 5s
      timeout: 5s
      retries: 15
      start_period: 45s
```

Replace it with:

```yaml
  engine:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: banyan-engine
    hostname: engine
    privileged: true
    command: ["/entrypoint-engine.sh"]
    environment:
      - HOME=/root
    ports:
      - "50051:50051"
    networks:
      banyan-net:
        ipv4_address: 172.28.0.10
    volumes:
      - keys-exchange:/tmp/keys-exchange
      - engine-data:/var/lib/banyan
      - ./examples:/examples:ro
    healthcheck:
      test: ["CMD", "banyan-cli", "engine"]
      interval: 5s
      timeout: 5s
      retries: 20
      start_period: 60s
```

- [ ] **Step 2: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add test/e2e/docker-compose.yml
git commit -m "test(e2e): set HOME=/root and extend healthcheck window for auth"
```

---

## Task 10: Add an auth sanity check to run-e2e.sh

**Files:**
- Modify: `test/e2e/run-e2e.sh`

**Why:** If login silently fails, every downstream CLI test fails with confusing `Unauthenticated` errors. A single explicit `whoami` check right after the cluster is healthy makes auth failures obvious and fast.

- [ ] **Step 1: Insert the sanity check after the engine status check**

In `test/e2e/run-e2e.sh`, find:

```bash
# Step 6: Check engine status (shows agents)
log_info "Checking engine status..."
docker exec banyan-engine banyan-engine status || log_warn "Engine status check failed"
```

Replace with:

```bash
# Step 6: Check engine status (shows agents)
log_info "Checking engine status..."
docker exec banyan-engine banyan-engine status || log_warn "Engine status check failed"

# Step 6b: Verify CLI authentication works (fail fast if auth is broken)
log_info "Verifying CLI authentication..."
WHOAMI_OUT=$(docker exec banyan-engine banyan-cli whoami 2>&1) || true
if echo "$WHOAMI_OUT" | grep -qi "role:"; then
    log_test_pass "Auth: CLI authenticated ($(echo "$WHOAMI_OUT" | tr '\n' ' '))"
else
    log_test_fail "Auth: CLI not authenticated — login failed in entrypoint"
    echo "  whoami output: $WHOAMI_OUT"
    echo "  engine entrypoint log (last 30 lines):"
    docker logs banyan-engine 2>&1 | tail -30
    exit 1
fi
```

- [ ] **Step 2: Verify the script is syntactically valid**

Run: `bash -n /home/hungnguyenba/workspace/fertile/banyan/test/e2e/run-e2e.sh && echo "syntax OK"`
Expected: `syntax OK`

- [ ] **Step 3: Commit**

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add test/e2e/run-e2e.sh
git commit -m "test(e2e): add fail-fast CLI auth sanity check"
```

---

## Task 11: Run the E2E suite and iterate

**Files:** none (verification only)

- [ ] **Step 1: Confirm all binaries build**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go build ./pkg/engine/... ./cmd/banyan-engine/... ./cmd/banyan-agent/... ./cmd/banyan-cli/...`
Expected: build succeeds with no output

- [ ] **Step 2: Run the full Go test suite**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan && go test ./pkg/engine/ ./pkg/engine/auth/ ./cmd/banyan-cli/... -count=1`
Expected: all PASS

- [ ] **Step 3: Run the E2E suite**

Run: `cd /home/hungnguyenba/workspace/fertile/banyan/test/e2e && ./run-e2e.sh`
Expected: ends with `All E2E tests passed!` and exit code 0. The new `Auth: CLI authenticated` line should appear in Phase 1.

- [ ] **Step 4: If the E2E fails, diagnose with this decision tree**

```
  Failure point                  →  Likely cause
  ────────────────────────────────────────────────────────────────
  "Initializing engine..." hangs  →  init still hitting an interactive
                                     huh form (a form other than the
                                     admin form). Check which form;
                                     gate it on --non-interactive too.
  "CLI login failed"              →  engine hasn't consumed bootstrap.
                                     Check `docker logs banyan-engine`
                                     for "Application-layer auth enabled"
                                     and for setupEngineAuth errors.
  Healthcheck never healthy       →  banyan-cli engine has no token.
                                     Confirm HOME=/root took effect:
                                     `docker exec banyan-engine env | grep HOME`
  "Auth: CLI not authenticated"   →  whoami found no creds file.
                                     Confirm credentials.json path:
                                     `docker exec banyan-engine \
                                       cat /root/.config/banyan/credentials.json`
  Deploy/down/agent tests fail    →  token attached but authz denies.
                                     admin role should allow everything;
                                     check the RPC→permission map.
```

Fix the root cause, rebuild, re-run. Repeat until green. Maximum 3 iterations before escalating — if still failing, stop and report the exact failure with logs.

- [ ] **Step 5: Final commit (only if E2E required fixes)**

If steps 1-4 required code changes, commit them:

```bash
cd /home/hungnguyenba/workspace/fertile/banyan
git add -A
git commit -m "fix(e2e): resolve auth wiring issues found by E2E run"
```

If the E2E passed with no changes, skip this step.

---

## Self-Review

**Spec coverage** — every gap identified in the verification conversation maps to a task:
- AuthDeps never constructed → Tasks 3, 4
- JWT key has no lifecycle → Task 1
- `auth-bootstrap.json` never read → Task 2
- `AuthDeps` not passed to `startEngineGRPC` → Task 4 Step 3
- Engine doesn't know the config directory → Tasks 4 Step 1, 5
- Interactive init hangs in Docker → Task 6
- Interactive CLI login hangs in Docker → Task 7
- E2E entrypoint runs bare `init` → Task 8
- `$HOME` inconsistency breaks credential lookup → Task 9
- Silent auth failure produces confusing errors → Task 10
- Nothing verifies the whole flow → Task 11

**Type consistency** — `loadOrCreateJWTKey`, `consumeAuthBootstrap`, `setupEngineAuth`, `jwtKeyRecord`, `bootstrapRecord`, `jwtKeySize` are used with identical signatures/names across Tasks 1-4. `auth.AuthDeps{JWT, Users, Authorizer}` field names match the struct defined in `pkg/engine/auth/middleware.go`. `auth.NewEtcdUserStore`, `auth.NewJWTManager`, `auth.NewRoleAuthorizer`, `auth.HashPassword`, `auth.User`, `auth.RoleAdmin` all match the existing auth package API.

**Placeholder scan** — no TBDs, every code step shows complete code, every command shows expected output.

**Known residual risk** — Task 6 only gates the *admin* form on `--non-interactive`. If the init wizard has another interactive form that isn't already skipped by pre-existing config in the E2E environment, Task 11's E2E run will surface it (decision tree, row 1). The current E2E already runs bare `init` successfully today, so all *other* forms are known to be non-blocking in that environment — the admin form is the only new blocker.
