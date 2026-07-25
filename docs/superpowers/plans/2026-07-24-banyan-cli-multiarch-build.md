# banyan-cli multi-arch build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `banyan-cli up` build container images for the architecture(s) the target agents actually run, so `build:` services work on a mixed-arch (amd64/arm64) cluster without hand-run cross-build scripts.

**Architecture:** Two phases. Phase 1 adds a per-service `platform:` field + a global `--platform` flag + a QEMU-binfmt preflight check, so cross-arch builds work by manual declaration. Phase 2 makes agents report `runtime.GOARCH` at registration, stores it on the engine, exposes it via the status API, and lets the CLI auto-resolve each service's build platform from the target agents' arch. A single multi-arch OCI image index (`nerdctl build --platform=... ` + `push --all-platforms`) means the engine/scheduler need no changes — containerd on the agent selects the right arch at pull time.

**Tech Stack:** Go, cobra CLI, gRPC/protobuf (protoc via `make proto`), containerd + nerdctl + BuildKit, QEMU `binfmt_misc`, bash installer.

> **IMPORTANT — no commits.** The user has instructed: do not commit anything, anywhere. Every task below ends with a **build+test verification** step instead of a `git commit`. Do not run `git add`/`git commit` at any point.

> **Convention note:** All `banyan-cli` commands on the build box run under `sudo` (rootful containerd/buildkit). Go unit tests do **not** need sudo. Run Go commands from the module that owns the file: CLI code lives in the `banyan-cli` submodule (`cd cmd/banyan-cli`), engine/agent/types in the root module (this is a Go workspace, `go.work`). When unsure, run `go test ./...` from repo root — the workspace resolves both.

---

## File Structure

**Phase 1 (manual declaration + preflight):**
- `pkg/types/manifest.go` — add `Platform` field to `ManifestService` (root module).
- `cmd/banyan-cli/cmd/deploy.go` — thread `platform` through build/push; add `--platform` flag; wire preflight.
- `cmd/banyan-cli/cmd/preflight.go` *(new)* — QEMU binfmt detection + actionable error.
- `cmd/banyan-cli/cmd/preflight_test.go` *(new)* — unit tests for binfmt detection.
- `cmd/banyan-cli/cmd/deploy_test.go` — extend `buildImageArgs` / build-resolution tests.
- `install-deps.sh` — add `install_qemu_binfmt()`, call it where buildkit is installed.

**Phase 2 (auto-detect arch):**
- `pkg/rpc/proto/banyan/v1/engine.proto` — `arch` field on `RegisterRequest` and `AgentInfo`.
- `pkg/rpc/banyanpb/*.pb.go` — regenerated (via `make proto`), not hand-edited.
- `pkg/agent/engine_client.go` — send `runtime.GOARCH` in Register.
- `pkg/types/records.go` — `Arch` on `NodeRecord`.
- `pkg/engine/grpc_handlers_agent.go` — persist `req.Arch`.
- `pkg/engine/grpc_handlers_cli.go` — return `node.Arch` in `AgentInfo`.
- `cmd/banyan-cli/cmd/platform.go` *(new)* — per-service platform resolution logic (pure, testable).
- `cmd/banyan-cli/cmd/platform_test.go` *(new)* — resolution-rule table tests.
- `cmd/banyan-cli/cmd/deploy.go` — reorder `runDeploy` to fetch agents before building; apply resolution.

---

# PHASE 1 — Manual declaration + preflight

## Task 1: Add `Platform` field to `ManifestService`

**Files:**
- Modify: `pkg/types/manifest.go:113-127`
- Test: `pkg/types/manifest_test.go` (create if absent, else append)

- [ ] **Step 1: Write the failing test**

Append to `pkg/types/manifest_test.go` (create the file with this package header if it does not exist):

```go
package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestManifestService_PlatformParses(t *testing.T) {
	src := `
image: myimg:latest
platform: linux/arm64
`
	var svc ManifestService
	if err := yaml.Unmarshal([]byte(src), &svc); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if svc.Platform != "linux/arm64" {
		t.Errorf("expected platform linux/arm64, got %q", svc.Platform)
	}
}

func TestManifestService_PlatformOmittedIsEmpty(t *testing.T) {
	var svc ManifestService
	if err := yaml.Unmarshal([]byte("image: myimg:latest\n"), &svc); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if svc.Platform != "" {
		t.Errorf("expected empty platform, got %q", svc.Platform)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/types/ -run TestManifestService_Platform -v`
Expected: FAIL — `svc.Platform undefined (type ManifestService has no field or method Platform)`.

- [ ] **Step 3: Add the field**

In `pkg/types/manifest.go`, add `Platform` to the `ManifestService` struct (place it right after `Build`):

```go
type ManifestService struct {
	Image       string               `yaml:"image"`
	Build       *ManifestBuild       `yaml:"build,omitempty"`
	Platform    string               `yaml:"platform,omitempty"` // e.g., "linux/arm64"; empty = build host arch (or auto-detect)
	Deploy      *ManifestDeploy      `yaml:"deploy,omitempty"`
	Healthcheck *ManifestHealthcheck `yaml:"healthcheck,omitempty"`
	Ports       []string             `yaml:"ports,omitempty"`
	Environment []string             `yaml:"environment,omitempty"`
	EnvFile     EnvFile              `yaml:"env_file,omitempty"`
	Command     []string             `yaml:"command,omitempty"`
	DependsOn   DependsOnConfig      `yaml:"depends_on,omitempty"`
	Restart     string               `yaml:"restart,omitempty"`
	Entrypoint  ShellCommand         `yaml:"entrypoint,omitempty"`
	Volumes     VolumeMounts         `yaml:"volumes,omitempty"`
	Secrets     []string             `yaml:"secrets,omitempty"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/types/ -run TestManifestService_Platform -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Verify the module still builds**

Run: `cd /home/work/freelancer/banyan && go build ./pkg/types/`
Expected: no output, exit 0. (No commit — per no-commit instruction.)

---

## Task 2: Thread `platform` through `buildImageArgs` and `buildImage`

**Files:**
- Modify: `cmd/banyan-cli/cmd/deploy.go:107-115` (`buildImageArgs`), `:314-328` (`buildImage`), `:24-28` (`buildImageFunc` var signature)
- Test: `cmd/banyan-cli/cmd/deploy_test.go:109-137` (extend `TestBuildImageArgs`)

- [ ] **Step 1: Write the failing test**

Replace the whole `func TestBuildImageArgs` in `cmd/banyan-cli/cmd/deploy_test.go` with:

```go
func TestBuildImageArgs(t *testing.T) {
	t.Run("without dockerfile, no platform", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "", "")
		expected := []string{"build", "-t", "my-app:latest", "./web"}
		assertArgs(t, expected, args)
	})

	t.Run("with dockerfile, no platform", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "Dockerfile.prod", "")
		expectedPath := filepath.Join("./web", "Dockerfile.prod")
		expected := []string{"build", "-t", "my-app:latest", "-f", expectedPath, "./web"}
		assertArgs(t, expected, args)
	})

	t.Run("with platform", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "", "linux/arm64")
		expected := []string{"build", "-t", "my-app:latest", "--platform", "linux/arm64", "./web"}
		assertArgs(t, expected, args)
	})

	t.Run("with multiple platforms", func(t *testing.T) {
		args := buildImageArgs("my-app:latest", "./web", "", "linux/amd64,linux/arm64")
		expected := []string{"build", "-t", "my-app:latest", "--platform", "linux/amd64,linux/arm64", "./web"}
		assertArgs(t, expected, args)
	})
}

func assertArgs(t *testing.T, expected, args []string) {
	t.Helper()
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, exp := range expected {
		if args[i] != exp {
			t.Errorf("arg[%d]: expected %q, got %q", i, exp, args[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run TestBuildImageArgs -v`
Expected: FAIL — `too many arguments in call to buildImageArgs` (compile error).

- [ ] **Step 3: Update `buildImageArgs`, `buildImage`, and the func var**

In `cmd/banyan-cli/cmd/deploy.go`, change `buildImageArgs` (currently line 108):

```go
// buildImageArgs constructs nerdctl build arguments.
func buildImageArgs(imageName, contextPath, dockerfile, platform string) []string {
	args := []string{"build", "-t", imageName}
	if dockerfile != "" {
		args = append(args, "-f", filepath.Join(contextPath, dockerfile))
	}
	if platform != "" {
		args = append(args, "--platform", platform)
	}
	args = append(args, contextPath)
	return args
}
```

Change `buildImage` (currently line 314) to take `platform` and delegate to `buildImageArgs` (removing the duplicated arg-building):

```go
func buildImage(imageName, contextPath, dockerfile, platform string) error {
	args := buildImageArgs(imageName, contextPath, dockerfile, platform)

	buildCmd := exec.Command("nerdctl", args...)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("nerdctl build failed (is buildkitd running?): %w", err)
	}
	return nil
}
```

Change the func var declaration (currently line 25):

```go
var (
	buildImageFunc = buildImage
	tagImageFunc   = tagImage
	pushImageFunc  = pushImage
)
```

The type of `buildImageFunc` is inferred from `buildImage`, so its new signature `func(imageName, contextPath, dockerfile, platform string) error` propagates automatically.

- [ ] **Step 4: Fix the caller in `buildServiceImages` and the test mocks (compile only)**

In `cmd/banyan-cli/cmd/deploy.go`, `buildServiceImages` currently calls (line ~305) `buildImageFunc(imageName, contextPath, svc.Build.Dockerfile)`. Change it to pass the service platform:

```go
		logging.Info("Building image", "service", name, "image", imageName)
		if err := buildImageFunc(imageName, contextPath, svc.Build.Dockerfile, svc.Platform); err != nil {
			return fmt.Errorf("failed to build image for service %q: %w", name, err)
		}
```

In `cmd/banyan-cli/cmd/deploy_test.go`, every mock assignment `buildImageFunc = func(imageName, contextPath, dockerfile string) error {` (there are three — around lines 248, 278, 302) must gain the `platform string` param:

```go
	buildImageFunc = func(imageName, contextPath, dockerfile, platform string) error {
```

(Do not change the mock bodies.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestBuildImageArgs|TestBuildServiceImages' -v`
Expected: PASS.

- [ ] **Step 6: Verify the whole CLI module builds and its tests pass**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./... && go test ./cmd/`
Expected: build clean; tests PASS.

---

## Task 3: Push a multi-platform index when a service was built for non-host arch

**Files:**
- Modify: `cmd/banyan-cli/cmd/deploy.go:388-398` (`pushImage`) and its func var + the `pushServiceImages` caller
- Test: `cmd/banyan-cli/cmd/deploy_test.go` (append `TestPushImageArgs`)

Rationale: `nerdctl push --all-platforms` uploads every arch in the local image index. When a service declares `platform:` with a comma (multi-arch) — or any non-host platform — we must push `--all-platforms` or the agent's arch layer never reaches the registry.

- [ ] **Step 1: Write the failing test**

Append to `cmd/banyan-cli/cmd/deploy_test.go`:

```go
func TestPushImageArgs(t *testing.T) {
	t.Run("single host arch", func(t *testing.T) {
		args := pushImageArgs("reg:5000/app:latest", false)
		expected := []string{"push", "--insecure-registry", "reg:5000/app:latest"}
		assertArgs(t, expected, args)
	})

	t.Run("all platforms", func(t *testing.T) {
		args := pushImageArgs("reg:5000/app:latest", true)
		expected := []string{"push", "--insecure-registry", "--all-platforms", "reg:5000/app:latest"}
		assertArgs(t, expected, args)
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run TestPushImageArgs -v`
Expected: FAIL — `undefined: pushImageArgs`.

- [ ] **Step 3: Extract `pushImageArgs` and thread `allPlatforms`**

In `cmd/banyan-cli/cmd/deploy.go`, replace `pushImage` (currently line ~388) with:

```go
// pushImageArgs constructs nerdctl push arguments. allPlatforms pushes the full
// multi-arch image index (needed when the image was built for a non-host arch).
func pushImageArgs(image string, allPlatforms bool) []string {
	args := []string{"push", "--insecure-registry"}
	if allPlatforms {
		args = append(args, "--all-platforms")
	}
	return append(args, image)
}

func pushImage(image string, allPlatforms bool) error {
	cmd := exec.Command("nerdctl", pushImageArgs(image, allPlatforms)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl push failed: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Update the `pushServiceImages` caller and the mock**

In `pushServiceImages` (`deploy.go` ~line 365-372), the call is currently `pushImageFunc(registryImage)`. Change it to pass whether this service targets a non-host platform:

```go
		logging.Info("Pushing image", "image", registryImage)
		if err := pushImageFunc(registryImage, svc.Platform != ""); err != nil {
			return fmt.Errorf("failed to push image for service %q: %w", name, err)
		}
```

Rationale for `svc.Platform != ""`: in Phase 1 a non-empty `platform:` is exactly the "built for a specific (possibly non-host) arch" signal. Phase 2 refines this.

In `cmd/banyan-cli/cmd/deploy_test.go`, find every mock `pushImageFunc = func(image string) error {` (search the file) and change the signature to:

```go
	pushImageFunc = func(image string, allPlatforms bool) error {
```

(Leave bodies unchanged.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestPushImageArgs|TestPushServiceImages' -v`
Expected: PASS.

- [ ] **Step 6: Verify the module builds and full CLI test suite passes**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./... && go test ./cmd/`
Expected: build clean; PASS.

---

## Task 4: QEMU binfmt preflight check

**Files:**
- Create: `cmd/banyan-cli/cmd/preflight.go`
- Create: `cmd/banyan-cli/cmd/preflight_test.go`

The check reads `/proc/sys/fs/binfmt_misc/qemu-<arch>` where `<arch>` is the QEMU CPU name (`aarch64` for arm64, `x86_64` for amd64). It only matters when the requested platform's arch differs from the host arch.

- [ ] **Step 1: Write the failing test**

Create `cmd/banyan-cli/cmd/preflight_test.go`:

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQemuCPUName(t *testing.T) {
	cases := map[string]string{
		"linux/arm64": "aarch64",
		"arm64":       "aarch64",
		"linux/amd64": "x86_64",
		"amd64":       "x86_64",
	}
	for platform, want := range cases {
		if got := qemuCPUName(platform); got != want {
			t.Errorf("qemuCPUName(%q) = %q, want %q", platform, got, want)
		}
	}
}

func TestBinfmtRegistered(t *testing.T) {
	dir := t.TempDir()
	// Simulate a registered, enabled handler.
	if err := os.WriteFile(filepath.Join(dir, "qemu-aarch64"), []byte("enabled\ninterpreter /usr/bin/qemu-aarch64-static\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !binfmtRegistered(dir, "aarch64") {
		t.Error("expected aarch64 to be registered")
	}
	if binfmtRegistered(dir, "x86_64") {
		t.Error("expected x86_64 to be unregistered (no file)")
	}
}

func TestBinfmtRegistered_DisabledHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qemu-aarch64"), []byte("disabled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if binfmtRegistered(dir, "aarch64") {
		t.Error("expected disabled handler to count as not registered")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestQemuCPUName|TestBinfmtRegistered' -v`
Expected: FAIL — `undefined: qemuCPUName`, `undefined: binfmtRegistered`.

- [ ] **Step 3: Implement `preflight.go`**

Create `cmd/banyan-cli/cmd/preflight.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const binfmtDir = "/proc/sys/fs/binfmt_misc"

// qemuCPUName maps a platform string (e.g. "linux/arm64" or "arm64") to the
// QEMU user-static CPU name used in the binfmt_misc handler filename.
func qemuCPUName(platform string) string {
	arch := platform
	if i := strings.LastIndex(platform, "/"); i >= 0 {
		arch = platform[i+1:]
	}
	switch arch {
	case "arm64", "aarch64":
		return "aarch64"
	case "amd64", "x86_64":
		return "x86_64"
	default:
		return arch
	}
}

// binfmtRegistered reports whether an enabled qemu-<cpu> handler exists under dir.
func binfmtRegistered(dir, cpu string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "qemu-"+cpu))
	if err != nil {
		return false
	}
	// The handler file's first line is "enabled" or "disabled".
	return strings.HasPrefix(strings.TrimSpace(string(data)), "enabled")
}

// hostCPU returns the QEMU CPU name for the machine running the CLI.
func hostCPU() string {
	return qemuCPUName(runtime.GOARCH)
}

// checkEmulation verifies that every non-host platform in platforms has a
// registered QEMU binfmt handler. Returns an actionable error if not.
func checkEmulation(platforms []string) error {
	host := hostCPU()
	var missing []string
	seen := map[string]bool{}
	for _, p := range platforms {
		cpu := qemuCPUName(p)
		if cpu == host || seen[cpu] {
			continue
		}
		seen[cpu] = true
		if !binfmtRegistered(binfmtDir, cpu) {
			missing = append(missing, cpu)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"cross-arch build requires QEMU emulation for %s, but no binfmt handler is registered.\n"+
			"Install it with either:\n"+
			"  sudo bash install-deps.sh   # installs qemu-user-static + binfmt-support\n"+
			"or:\n"+
			"  sudo nerdctl run --privileged --rm tonistiigi/binfmt --install %s",
		strings.Join(missing, ", "), strings.Join(missing, ","))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestQemuCPUName|TestBinfmtRegistered' -v`
Expected: PASS (all three).

- [ ] **Step 5: Verify build**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./...`
Expected: clean.

---

## Task 5: Wire preflight + `--platform` flag into `runDeploy`

**Files:**
- Modify: `cmd/banyan-cli/cmd/deploy.go` — add `deployPlatform` var + flag (init, ~line 68-73); apply the flag to services and run preflight before building (in `runDeploy`, before the `buildServiceImages` call at line ~147)
- Test: `cmd/banyan-cli/cmd/deploy_test.go` (append `TestCollectBuildPlatforms` + `TestApplyPlatformOverride`)

- [ ] **Step 1: Write the failing test**

Append to `cmd/banyan-cli/cmd/deploy_test.go`:

```go
func TestApplyPlatformOverride(t *testing.T) {
	services := map[string]types.ManifestService{
		"web": {Build: &types.ManifestBuild{Context: "./web"}},
		"api": {Build: &types.ManifestBuild{Context: "./api"}, Platform: "linux/amd64"},
		"db":  {Image: "mysql:8.0"}, // no build — untouched
	}
	applyPlatformOverride(services, "linux/arm64")
	if services["web"].Platform != "linux/arm64" {
		t.Errorf("web: override should set empty platform, got %q", services["web"].Platform)
	}
	if services["api"].Platform != "linux/arm64" {
		t.Errorf("api: global flag should win over per-service, got %q", services["api"].Platform)
	}
	if services["db"].Platform != "" {
		t.Errorf("db: non-build service must be untouched, got %q", services["db"].Platform)
	}
}

func TestCollectBuildPlatforms(t *testing.T) {
	services := map[string]types.ManifestService{
		"web": {Build: &types.ManifestBuild{Context: "./web"}, Platform: "linux/arm64"},
		"api": {Build: &types.ManifestBuild{Context: "./api"}}, // no platform
		"db":  {Image: "mysql:8.0", Platform: "linux/arm64"},   // no build — ignored
	}
	got := collectBuildPlatforms(services)
	if len(got) != 1 || got[0] != "linux/arm64" {
		t.Errorf("expected [linux/arm64], got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestApplyPlatformOverride|TestCollectBuildPlatforms' -v`
Expected: FAIL — `undefined: applyPlatformOverride`, `undefined: collectBuildPlatforms`.

- [ ] **Step 3: Implement the two helpers**

Add to `cmd/banyan-cli/cmd/deploy.go` (near `buildServiceImages`):

```go
// applyPlatformOverride sets Platform on every build service to the given value.
// Used by the global --platform flag; wins over per-service platform:.
func applyPlatformOverride(services map[string]types.ManifestService, platform string) {
	if platform == "" {
		return
	}
	for name, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil {
			continue
		}
		svc.Platform = platform
		services[name] = svc
	}
}

// collectBuildPlatforms returns the distinct platform strings across all build
// services (skipping empty ones). Used for the emulation preflight.
func collectBuildPlatforms(services map[string]types.ManifestService) []string {
	seen := map[string]bool{}
	var out []string
	for _, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil || svc.Platform == "" {
			continue
		}
		if !seen[svc.Platform] {
			seen[svc.Platform] = true
			out = append(out, svc.Platform)
		}
	}
	return out
}
```

- [ ] **Step 4: Add the flag and wire both helpers into `runDeploy`**

In `cmd/banyan-cli/cmd/deploy.go`, add the var next to the other deploy vars (top of file, the `var (...)` block around line 17-22):

```go
	deployPlatform string
```

In `init()` (line ~68), add:

```go
	deployCmd.Flags().StringVar(&deployPlatform, "platform", "", "Override build platform for all build services (e.g., linux/arm64)")
```

In `runDeploy`, immediately **before** the `buildServiceImages` call (currently line 147), insert:

```go
	// Apply the global --platform override, then verify QEMU emulation is
	// available for any non-host build platform before we attempt a build.
	applyPlatformOverride(manifest.Services, deployPlatform)
	if emuErr := checkEmulation(collectBuildPlatforms(manifest.Services)); emuErr != nil {
		return emuErr
	}
```

- [ ] **Step 5: Run tests to verify pass**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestApplyPlatformOverride|TestCollectBuildPlatforms' -v`
Expected: PASS.

- [ ] **Step 6: Full CLI module build + test**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./... && go test ./cmd/`
Expected: build clean; PASS.

---

## Task 6: Installer — `install_qemu_binfmt()`

**Files:**
- Modify: `install-deps.sh` — add `install_qemu_binfmt()` (near `install_buildkit`, ~line 329), call it in the agent/all branch of `install_deps()` (~line 586-592)

Rationale: build boxes install the same builder deps as an agent (containerd + nerdctl + buildkit). QEMU binfmt belongs beside buildkit. This replaces the "install buildx" idea — the stack is BuildKit, not Docker, so `buildx` does not apply.

- [ ] **Step 1: Add the function**

In `install-deps.sh`, add after `install_buildkit()` (ends ~line 367):

```bash
install_qemu_binfmt() {
    # Cross-arch image builds run foreign binaries under QEMU user emulation,
    # registered via binfmt_misc. Needed on any box that cross-builds (e.g. an
    # amd64 build host targeting arm64 agents).
    if [ -e /proc/sys/fs/binfmt_misc/qemu-aarch64 ] && [ -e /proc/sys/fs/binfmt_misc/qemu-x86_64 ]; then
        info "QEMU binfmt handlers already registered, skipping."
        return
    fi

    info "Installing QEMU user-static emulation (for cross-arch builds)..."

    local family
    family=$(get_family)
    if [ "$family" = "debian" ]; then
        $PKG_UPDATE
        install_pkg "qemu-user-static"
        install_pkg "binfmt-support"
    else
        install_pkg "qemu-user-static"
    fi

    # Register handlers for all arches with the buildkit/containerd worker.
    # tonistiigi/binfmt is the most reliable cross-distro registrar.
    if command -v nerdctl &>/dev/null; then
        nerdctl run --privileged --rm tonistiigi/binfmt --install all >/dev/null 2>&1 || \
            warn "tonistiigi/binfmt registration failed; qemu-user-static package handlers will be used."
    fi

    info "QEMU binfmt emulation installed."
}
```

- [ ] **Step 2: Call it in the agent/all branch**

In `install_deps()` (~line 586), add the call after `install_buildkit`:

```bash
    if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
        install_containerd
        install_nerdctl
        install_cni
        install_wireguard
        install_buildkit
        install_qemu_binfmt  # For cross-arch image builds
        install_nfs_client   # For NFS volume mounts
    fi
```

- [ ] **Step 3: Verify the script parses**

Run: `cd /home/work/freelancer/banyan && bash -n install-deps.sh`
Expected: no output, exit 0 (syntax OK).

- [ ] **Step 4: Confirm the function is discoverable when sourced**

Run: `cd /home/work/freelancer/banyan && bash -c 'set -e; source install-deps.sh >/dev/null 2>&1 || true; declare -F install_qemu_binfmt'`
Expected: prints `install_qemu_binfmt`.

> **Note:** Actually installing QEMU requires `sudo` and a real environment — the user runs `sudo bash install-deps.sh` (or sources the function) in their own terminal. Do not run the install here.

---

## Task 7: Phase 1 docs

**Files:**
- Modify: `docs/` — find the manifest reference doc and document `platform:` + the QEMU requirement.

- [ ] **Step 1: Locate the manifest reference doc**

Run: `cd /home/work/freelancer/banyan && rg -l "placement|build:|banyan.yaml" docs/ | head`
Expected: a list of docs; pick the manifest/reference one (e.g. a compose-compat or manifest doc). If none documents service fields, add a short section to `README.md` under the manifest description instead.

- [ ] **Step 2: Add the documentation**

Add this section to the chosen doc:

```markdown
### Building for a specific architecture (`platform:`)

`banyan-cli` builds images on the machine running the CLI. If your agents run a
different CPU architecture (e.g. arm64 Ampere servers while you build on an
amd64 laptop), set `platform:` on each build service:

```yaml
services:
  api:
    build: ./api
    platform: linux/arm64
```

Or override all build services at once:

```bash
sudo banyan-cli up --platform linux/arm64
```

Cross-arch builds run under QEMU emulation, which must be installed on the build
box (`install-deps.sh` installs it, or run
`sudo nerdctl run --privileged --rm tonistiigi/binfmt --install all`). If it is
missing, `banyan-cli up` fails fast with install instructions.
```

- [ ] **Step 3: Verify no broken markdown / dead file**

Run: `cd /home/work/freelancer/banyan && rg -n "platform: linux/arm64" docs/ README.md`
Expected: the new lines appear. (No commit.)

---

# PHASE 2 — Auto-detect arch from the cluster

## Task 8: Add `arch` to the proto and regenerate

**Files:**
- Modify: `pkg/rpc/proto/banyan/v1/engine.proto:81-88` (`RegisterRequest`), `:323-330` (`AgentInfo`)
- Regenerate: `pkg/rpc/banyanpb/*.pb.go` via `make proto`

- [ ] **Step 1: Edit the proto**

In `RegisterRequest` (line 81), add field 7:

```proto
message RegisterRequest {
  string agent_name = 1;
  string api_address = 2;
  string session_token = 3;
  repeated string tags = 4;
  string wg_public_key = 5;       // agent's WireGuard public key
  string host_ip = 6;             // agent's data-plane host IP (for overlay peer endpoint)
  string arch = 7;                // agent's CPU arch (runtime.GOARCH: "amd64" | "arm64")
}
```

In `AgentInfo` (line 323), add field 7:

```proto
message AgentInfo {
  string name = 1;
  string status = 2;
  string api_address = 3;
  int64 last_seen_unix = 4;
  int64 created_at_unix = 5;
  repeated string tags = 6;
  string arch = 7;                // agent's CPU arch, empty if agent predates arch reporting
}
```

- [ ] **Step 2: Regenerate the Go bindings**

Run: `cd /home/work/freelancer/banyan && make proto`
Expected: exit 0; `git status --short pkg/rpc/banyanpb/` shows modified `engine.pb.go` (do NOT commit).

If `protoc` or its plugins are missing, install per `README`/`DEVELOPMENT.md` first. Verify the field exists:

Run: `cd /home/work/freelancer/banyan && rg -n "Arch " pkg/rpc/banyanpb/engine.pb.go | head`
Expected: generated `Arch string` getters on `RegisterRequest` and `AgentInfo`.

- [ ] **Step 3: Verify everything still builds after regen**

Run: `cd /home/work/freelancer/banyan && go build ./...`
Expected: clean (no consumers broke — new fields are additive).

---

## Task 9: Agent reports `runtime.GOARCH` at registration

**Files:**
- Modify: `pkg/agent/engine_client.go:101-116` (`RegisterRequest` struct + the proto call)
- Test: `pkg/agent/engine_client_test.go` (append a test asserting the arch is sent)

- [ ] **Step 1: Write the failing test**

Append to `pkg/agent/engine_client_test.go`:

```go
func TestRegister_SendsArch(t *testing.T) {
	var gotArch string
	srv := &testEngineServer{
		registerFunc: func(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
			gotArch = req.Arch
			return &banyanpb.RegisterResponse{RegistryUrl: "reg:5000"}, nil
		},
	}
	client, cleanup := newTestEngineClient(t, srv)
	defer cleanup()

	_, _, _, err := client.Register(context.Background(), RegisterRequest{Name: "worker-1", APIAddr: "addr"})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if gotArch != runtime.GOARCH {
		t.Errorf("expected arch %q, got %q", runtime.GOARCH, gotArch)
	}
}
```

> Before writing, confirm the helper names: run `rg -n "func newTestEngineClient|registerFunc" pkg/agent/engine_client_test.go`. The file already defines a `testEngineServer` with a `registerFunc` field (used at line ~70) and a client constructor helper — reuse the exact names it uses. If the constructor helper has a different name, substitute it. Add `"runtime"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/agent/ -run TestRegister_SendsArch -v`
Expected: FAIL — `gotArch` is `""`, not `runtime.GOARCH` (or a compile error on `req.Arch` if Task 8 was skipped — Task 8 must be done first).

- [ ] **Step 3: Send the arch**

In `pkg/agent/engine_client.go`, add `"runtime"` to imports, then set `Arch` in the proto call (line 110):

```go
	resp, err := ec.client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:   req.Name,
		ApiAddress:  req.APIAddr,
		Tags:        req.Tags,
		WgPublicKey: req.WGPublicKey,
		HostIp:      req.HostIP,
		Arch:        runtime.GOARCH,
	})
```

(The `RegisterRequest` Go struct at line 101 does not need an `Arch` field — the arch is sourced from `runtime.GOARCH` at call time, not passed by callers. This keeps every existing caller unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/agent/ -run TestRegister_SendsArch -v`
Expected: PASS.

- [ ] **Step 5: Verify the agent package builds + tests pass**

Run: `cd /home/work/freelancer/banyan && go build ./pkg/agent/ && go test ./pkg/agent/`
Expected: clean; PASS.

---

## Task 10: Persist arch on the engine's `NodeRecord`

**Files:**
- Modify: `pkg/types/records.go:134-145` (`NodeRecord`)
- Modify: `pkg/engine/grpc_handlers_agent.go:41-48` (Register handler node creation)
- Test: `pkg/engine/grpc_handlers_agent_test.go` (append; verify stored arch)

- [ ] **Step 1: Add the field to `NodeRecord`**

In `pkg/types/records.go`, add to `NodeRecord`:

```go
type NodeRecord struct {
	LastSeen         time.Time `json:"last_seen"`
	CreatedAt        time.Time `json:"created_at"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	APIAddress       string    `json:"api_address,omitempty"`
	Arch             string    `json:"arch,omitempty"`
	Tags             []string  `json:"tags,omitempty"`
	MemoryTotalBytes uint64    `json:"memory_total_bytes,omitempty"`
	MemoryUsedBytes  uint64    `json:"memory_used_bytes,omitempty"`
	CPUCores         uint32    `json:"cpu_cores,omitempty"`
	CPUUsageRatio    float64   `json:"cpu_usage_ratio,omitempty"`
}
```

- [ ] **Step 2: Write the failing test**

First inspect an existing Register handler test to reuse its harness: `rg -n "func Test.*Register" pkg/engine/grpc_handlers_agent_test.go`. Mirror its setup (store, server construction). Append a test that registers with `Arch: "arm64"` and asserts the saved `NodeRecord.Arch`:

```go
func TestRegister_PersistsArch(t *testing.T) {
	s, store := newTestEngineServer(t) // reuse whatever the file's existing helper is named
	ctx := context.Background()

	_, err := s.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:  "worker-1",
		ApiAddress: "10.0.0.5:9100",
		Arch:       "arm64",
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	var node types.NodeRecord
	if err := store.Get(ctx, types.KeyNodes+"worker-1", &node); err != nil {
		t.Fatalf("get node: %v", err)
	}
	if node.Arch != "arm64" {
		t.Errorf("expected arch arm64, got %q", node.Arch)
	}
}
```

> If the file's helper has a different name/signature than `newTestEngineServer`, adapt to match (check with the `rg` above). The VPC allocator may be nil in the harness — that path is guarded by `if s.allocator != nil`, so registration still succeeds.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/engine/ -run TestRegister_PersistsArch -v`
Expected: FAIL — `node.Arch` is `""`.

- [ ] **Step 4: Store the arch in the handler**

In `pkg/engine/grpc_handlers_agent.go`, add `Arch` to the node record (line ~42):

```go
	node := &types.NodeRecord{
		Name:       req.AgentName,
		Status:     "ready",
		APIAddress: req.ApiAddress,
		Arch:       req.Arch,
		Tags:       req.Tags,
		LastSeen:   time.Now(),
		CreatedAt:  time.Now(),
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/engine/ -run TestRegister_PersistsArch -v`
Expected: PASS.

- [ ] **Step 6: Verify engine + types build and tests pass**

Run: `cd /home/work/freelancer/banyan && go build ./pkg/engine/ ./pkg/types/ && go test ./pkg/engine/ ./pkg/types/`
Expected: clean; PASS.

---

## Task 11: Expose arch via `GetStatus` (AgentInfo)

**Files:**
- Modify: `pkg/engine/grpc_handlers_cli.go:225-233` (the `AgentInfo` construction inside `GetStatus`)
- Test: `pkg/engine/grpc_handlers_cli_test.go` (append or extend an existing GetStatus test)

> Note: `GetInfo` (line 357) only returns the registry URL and does not list agents. The agent list the CLI consumes comes from `GetStatus` → `GetStatusResponse.Agents`. So we surface arch there.

- [ ] **Step 1: Write the failing test**

Inspect existing GetStatus tests: `rg -n "func Test.*GetStatus" pkg/engine/grpc_handlers_cli_test.go`. Reuse the harness. Append:

```go
func TestGetStatus_ReturnsAgentArch(t *testing.T) {
	s, store := newTestEngineServer(t) // match the file's helper name
	ctx := context.Background()

	node := &types.NodeRecord{Name: "worker-1", Status: "ready", Arch: "arm64"}
	if err := store.Save(ctx, types.KeyNodes+"worker-1", node); err != nil {
		t.Fatal(err)
	}

	resp, err := s.GetStatus(ctx, &banyanpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	var found *banyanpb.AgentInfo
	for _, a := range resp.Agents {
		if a.Name == "worker-1" {
			found = a
		}
	}
	if found == nil {
		t.Fatal("worker-1 not in agents")
	}
	if found.Arch != "arm64" {
		t.Errorf("expected arch arm64, got %q", found.Arch)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/engine/ -run TestGetStatus_ReturnsAgentArch -v`
Expected: FAIL — `found.Arch` is `""`.

- [ ] **Step 3: Populate `Arch` in the AgentInfo**

In `pkg/engine/grpc_handlers_cli.go` (line ~226), add `Arch`:

```go
		agents = append(agents, &banyanpb.AgentInfo{
			Name:          node.Name,
			Status:        node.Status,
			ApiAddress:    node.APIAddress,
			LastSeenUnix:  node.LastSeen.Unix(),
			CreatedAtUnix: node.CreatedAt.Unix(),
			Tags:          node.Tags,
			Arch:          node.Arch,
		})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan && go test ./pkg/engine/ -run TestGetStatus_ReturnsAgentArch -v`
Expected: PASS.

- [ ] **Step 5: Verify engine build + tests**

Run: `cd /home/work/freelancer/banyan && go build ./pkg/engine/ && go test ./pkg/engine/`
Expected: clean; PASS.

---

## Task 12: Per-service platform resolution logic

**Files:**
- Create: `cmd/banyan-cli/cmd/platform.go`
- Create: `cmd/banyan-cli/cmd/platform_test.go`

This is the pure decision core (the 5 precedence rules from the spec), separated from any gRPC/IO so it is fully table-testable.

- [ ] **Step 1: Write the failing test**

Create `cmd/banyan-cli/cmd/platform_test.go`:

```go
package cmd

import (
	"sort"
	"strings"
	"testing"

	"github.com/fertile-org/banyan/pkg/types"
)

func TestResolveServicePlatform(t *testing.T) {
	archOf := map[string]string{
		"node-amd": "amd64",
		"node-arm": "arm64",
	}
	all := []string{"amd64", "arm64"}

	tests := []struct {
		name string
		svc  types.ManifestService
		want string
	}{
		{
			name: "explicit platform wins",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Platform: "linux/riscv64"},
			want: "linux/riscv64",
		},
		{
			name: "placement node arch",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Deploy: &types.ManifestDeploy{Placement: &types.ManifestPlacement{Node: "node-arm"}}},
			want: "linux/arm64",
		},
		{
			name: "no constraint -> union of all cluster arches",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}},
			want: "linux/amd64,linux/arm64",
		},
		{
			name: "placement to unknown node -> host fallback (empty)",
			svc:  types.ManifestService{Build: &types.ManifestBuild{Context: "."}, Deploy: &types.ManifestDeploy{Placement: &types.ManifestPlacement{Node: "ghost"}}},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveServicePlatform(tc.svc, archOf, all)
			if normalizePlatforms(got) != normalizePlatforms(tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// normalizePlatforms sorts a comma-list so order does not matter in assertions.
func normalizePlatforms(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func TestClusterArches(t *testing.T) {
	agents := []*fakeAgent{{name: "a", arch: "amd64"}, {name: "b", arch: "arm64"}, {name: "c", arch: ""}}
	archOf, all := clusterArches(toAgentInfo(agents))
	if archOf["a"] != "amd64" || archOf["b"] != "arm64" {
		t.Errorf("archOf wrong: %v", archOf)
	}
	if _, ok := archOf["c"]; ok {
		t.Error("empty-arch agent should be omitted from archOf")
	}
	if normalizePlatforms(strings.Join(all, ",")) != "amd64,arm64" {
		t.Errorf("all arches wrong: %v", all)
	}
}
```

> The `fakeAgent`/`toAgentInfo` helpers below convert to `*banyanpb.AgentInfo`. Add them to `platform_test.go`:

```go
type fakeAgent struct{ name, arch string }

func toAgentInfo(fs []*fakeAgent) []*banyanpb.AgentInfo {
	var out []*banyanpb.AgentInfo
	for _, f := range fs {
		out = append(out, &banyanpb.AgentInfo{Name: f.name, Arch: f.arch})
	}
	return out
}
```

Add `"github.com/fertile-org/banyan/pkg/rpc/banyanpb"` to the test imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestResolveServicePlatform|TestClusterArches' -v`
Expected: FAIL — `undefined: resolveServicePlatform`, `undefined: clusterArches`.

- [ ] **Step 3: Implement `platform.go`**

Create `cmd/banyan-cli/cmd/platform.go`:

```go
package cmd

import (
	"sort"
	"strings"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// clusterArches builds a name→arch map (omitting agents that report no arch)
// and the sorted set of distinct arches present in the cluster.
func clusterArches(agents []*banyanpb.AgentInfo) (map[string]string, []string) {
	archOf := map[string]string{}
	set := map[string]bool{}
	for _, a := range agents {
		if a.Arch == "" {
			continue
		}
		archOf[a.Name] = a.Arch
		set[a.Arch] = true
	}
	var all []string
	for arch := range set {
		all = append(all, arch)
	}
	sort.Strings(all)
	return archOf, all
}

// resolveServicePlatform returns the nerdctl --platform value for one build
// service, applying the precedence rules. Returns "" to mean "build host arch"
// (the caller's fallback), which also covers the unknown/offline cases.
func resolveServicePlatform(svc types.ManifestService, archOf map[string]string, allArches []string) string {
	// Rule 1: explicit platform wins.
	if svc.Platform != "" {
		return svc.Platform
	}
	// Rule 2: pinned to a node — use that node's arch (if known).
	if svc.Deploy != nil && svc.Deploy.Placement != nil && svc.Deploy.Placement.Node != "" {
		if arch, ok := archOf[svc.Deploy.Placement.Node]; ok {
			return "linux/" + arch
		}
		return "" // unknown node arch → host fallback
	}
	// Rule 4: no placement constraint — union of all cluster arches.
	// (Rule 3, tag-based union, is folded into "all" for now: without a tag
	// filter we target every known arch, which is always safe.)
	if len(allArches) == 0 {
		return "" // no arch info at all → host fallback
	}
	var platforms []string
	for _, a := range allArches {
		platforms = append(platforms, "linux/"+a)
	}
	return strings.Join(platforms, ",")
}
```

> **Scope note (Rule 3):** tag-based arch narrowing is intentionally folded into the "all cluster arches" union — targeting a superset of arches is always safe (the extra layers are just unused). A precise tag→agent→arch intersection is a future refinement, not needed for correctness. This is called out in the spec's testing section.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/ -run 'TestResolveServicePlatform|TestClusterArches' -v`
Expected: PASS.

- [ ] **Step 5: Verify module builds**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./...`
Expected: clean.

---

## Task 13: Wire auto-detection into `runDeploy`

**Files:**
- Modify: `cmd/banyan-cli/cmd/deploy.go` — reorder so the engine/agent list is fetched before building; apply `resolveServicePlatform` to each build service; keep host-arch fallback + warning
- Test: manual (integration) — documented; no new unit test (covered by Task 12 core)

The reorder: today `buildServiceImages` runs at line 147, before the engine client exists. We move the "connect + get status + resolve platforms" ahead of the build, while keeping `--dry-run` and engine-offline working via fallback.

- [ ] **Step 1: Add a platform-resolution helper that warns on fallback**

Add to `cmd/banyan-cli/cmd/platform.go`:

```go
import "github.com/fertile-org/banyan/pkg/logging"

// applyResolvedPlatforms fills Platform on every build service that does not
// already have one, using cluster arch info. Services left empty fall back to
// the build host arch; we warn so the operator knows why.
func applyResolvedPlatforms(services map[string]types.ManifestService, agents []*banyanpb.AgentInfo) {
	archOf, all := clusterArches(agents)
	for name, svc := range services { //nolint:gocritic // map iteration
		if svc.Build == nil || svc.Platform != "" {
			continue
		}
		resolved := resolveServicePlatform(svc, archOf, all)
		if resolved == "" {
			logging.Warn("Could not determine target arch; building for host arch",
				"service", name,
				"hint", "upgrade agents to report arch, or set platform: on the service")
			continue
		}
		svc.Platform = resolved
		services[name] = svc
	}
}
```

> Move the `logging` import into the existing import block of `platform.go` (do not create a second `import` statement).

- [ ] **Step 2: Reorder `runDeploy`**

In `cmd/banyan-cli/cmd/deploy.go`, restructure `runDeploy` so the flow is: parse → validate → (dry-run branch unchanged, still builds host-arch) → connect engine → `Status()` → `applyResolvedPlatforms` → `applyPlatformOverride` (flag still wins) → `checkEmulation` → `buildServiceImages` → push → deploy.

Concretely: **remove** the early build block at line ~146-149:

```go
	// Build images for services with build config
	manifestDir := filepath.Dir(deployFile)
	if buildErr := buildServiceImages(manifestDir, manifest.Name, manifest.Services); buildErr != nil {
		return buildErr
	}
```

Keep `manifestDir := filepath.Dir(deployFile)` (it is used later for env/volume resolution) — leave that assignment, just remove the build call. Then in the **dry-run branch** (line ~159), build host-arch first so `--dry-run` still validates the build locally offline:

```go
	if deployDryRun {
		// Offline: build for host arch (no engine to ask). --platform still honored.
		applyPlatformOverride(manifest.Services, deployPlatform)
		if emuErr := checkEmulation(collectBuildPlatforms(manifest.Services)); emuErr != nil {
			return emuErr
		}
		if buildErr := buildServiceImages(manifestDir, manifest.Name, manifest.Services); buildErr != nil {
			return buildErr
		}
		services := types.BuildServiceRecords(manifest.Services)
		fmt.Printf("Application: %s\n", manifest.Name)
		fmt.Printf("Services: %d\n", len(manifest.Services))
		for name, svc := range services { //nolint:gocritic // display-only loop
			fmt.Printf("  - %s: %s (replicas: %d)\n", name, svc.Image, svc.Replicas)
		}
		fmt.Println("\n[DRY-RUN] Manifest is valid. No changes made.")
		return nil
	}
```

After the engine client + `info` are obtained (the block around line 180-191), and **before** `pushServiceImages`, insert the resolve+build sequence:

```go
	// Resolve each build service's target platform from the cluster's agents,
	// then build. --platform flag overrides everything; missing arch info falls
	// back to host arch (with a warning) so offline-ish clusters still work.
	status, statusErr := client.Status(ctx)
	if statusErr != nil {
		logging.Warn("Could not list agents; building for host arch", "err", statusErr)
	} else {
		applyResolvedPlatforms(manifest.Services, status.Agents)
	}
	applyPlatformOverride(manifest.Services, deployPlatform)
	if emuErr := checkEmulation(collectBuildPlatforms(manifest.Services)); emuErr != nil {
		return emuErr
	}
	if buildErr := buildServiceImages(manifestDir, manifest.Name, manifest.Services); buildErr != nil {
		return buildErr
	}

	// Push built images to registry
	if pushErr := pushServiceImages(info.RegistryUrl, manifest.Services); pushErr != nil {
		return pushErr
	}
```

> Ensure `pushServiceImages` still runs after the build (it already followed the old push site). The push at line ~192 that already exists should be the one shown above — do not double-call it.

- [ ] **Step 3: Verify it compiles**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go build ./...`
Expected: clean. If `status` shadows an import or unused-var errors appear, rename the local to `st`.

- [ ] **Step 4: Run the full CLI test suite**

Run: `cd /home/work/freelancer/banyan/cmd/banyan-cli && go test ./cmd/`
Expected: PASS (existing deploy tests still green; the reorder does not change dry-run's public behavior beyond building host-arch, which the mocks tolerate).

- [ ] **Step 5: Full-workspace build + vet**

Run: `cd /home/work/freelancer/banyan && go build ./... && go vet ./...`
Expected: clean.

---

## Task 14: End-to-end verification (manual, user-run)

**Files:** none (operational verification on the real takahiro-dev cluster)

> This task is run by the user in their own terminal (server access is theirs, not the agent's). The plan lists the commands; the agent guides and interprets output.

- [ ] **Step 1: Rebuild + install the CLI from source**

```bash
cd /home/work/freelancer/banyan
make build   # or: go build ./cmd/banyan-cli
sudo install -m 0755 ./bin/banyan-cli /usr/local/bin/banyan-cli   # adjust path to the built binary
```

- [ ] **Step 2: Switch takahiro-dev manifest back to `build:`**

Edit `deploy/takahiro-dev/banyan.yaml`: for the four app services (`caddy`, `admin`, `survey`, `backend`) replace the pinned `image: 10.200.185.161:5000/taka-dev-*:latest` lines with their original `build:` blocks (context dirs per `build-arm64.sh`), and add `platform: linux/arm64` under each. Leave `mysql` on `image: mysql:8.0`.

- [ ] **Step 3: Deploy (CLI now cross-builds arm64 automatically)**

```bash
sudo banyan-cli up -f deploy/takahiro-dev/banyan.yaml
```

Expected: preflight passes (QEMU present); each build service builds `--platform=linux/arm64`; push uses `--all-platforms`; deployment proceeds. `mysql` reaches `healthy`; `caddy`/`admin`/`survey` `running`; `backend` may stay `restarting` until the DB dump is seeded (separate operational step, out of scope here).

- [ ] **Step 4: Confirm agent arch is now reported**

```bash
sudo banyan-cli agent list   # or the status command; confirm arch column shows arm64
```

Expected: `banyan-agent-1` shows `arm64`. (This requires the engine + agent binaries to be rebuilt from this branch and restarted — a user-run server step.)

- [ ] **Step 5: Verify running containers are arm64-native (no restart loop)**

Expected: `caddy`/`admin`/`survey` stay `running` (not `restarting`), confirming the pulled layers match the agent arch.

---

## Self-Review notes (for the executor)

- **Task ordering matters:** Task 8 (proto regen) must precede Tasks 9–11 (they reference `req.Arch` / `AgentInfo.Arch`). Phase 1 (Tasks 1–7) is independent of Phase 2 and already unblocks takahiro-dev on its own.
- **No commits anywhere** — every "verify" step replaces the usual commit. If using subagent-driven execution, instruct each subagent explicitly not to commit.
- **Helper names in engine/agent tests** (`newTestEngineServer`, `newTestEngineClient`, `testEngineServer.registerFunc`) are reused from existing test files; confirm exact names with the `rg` commands noted in each task before writing, and adapt if they differ.
- **Spec coverage:** platform field (T1), --platform flag (T5), preflight (T4), installer QEMU (T6), docs (T7), proto arch (T8), agent reports (T9), engine stores (T10), status exposes (T11), resolution rules (T12), auto-detect wiring + host fallback/degrade (T13), E2E gap acknowledged as manual (T14). MySQL migration and remote-builder remain out of scope per spec.
