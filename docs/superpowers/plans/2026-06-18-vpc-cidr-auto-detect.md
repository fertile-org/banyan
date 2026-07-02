# VPC CIDR Auto-Detection & Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Detect a conflict-free private VPC CIDR, let the user view/configure it during `banyan-engine init`, persist the choice so `start` uses it, and give a helpful suggestion when a conflict is detected.

**Architecture:** Add a `VPCCIDR` field to `EngineConfig` so the choice persists. Add a `suggestFreeCIDR()` helper in `pkg/engine` that reuses the existing host-interface conflict check to pick the first free candidate range. Wire `init` to prompt (interactive) or auto-pick (non-interactive), and wire `start` to read the value from config with flag precedence. Augment the start-time conflict error with a suggested range.

**Tech Stack:** Go (multi-module workspace via `go.work`), `huh` TUI forms, `cobra` CLI.

> **Module note:** This is a multi-module repo. Run each module's tests from inside that module directory, e.g. `cd pkg/engine && go test ./...`. Do NOT run `go test ./...` from the repo root.

---

## File Structure

| File | Responsibility |
|------|----------------|
| `pkg/types/config.go` | Add `VPCCIDR` field to `EngineConfig` (persisted in YAML). |
| `pkg/types/config_test.go` | Round-trip test covering the new field. |
| `pkg/engine/engine.go` | Add `candidateCIDRs` + `suggestFreeCIDR()`; augment the start-time conflict error message. |
| `pkg/engine/engine_test.go` | Unit tests for `suggestFreeCIDR()`. |
| `cmd/banyan-engine/cmd/engine.go` | Add VPC CIDR step to the `init` wizard (interactive + non-interactive); read VPC CIDR from config with flag precedence in `start`. |

---

## Task 1: Persist VPC CIDR in EngineConfig

**Files:**
- Modify: `pkg/types/config.go` (struct `EngineConfig`)
- Test: `pkg/types/config_test.go`

- [ ] **Step 1: Write the failing round-trip test**

Add this test to `pkg/types/config_test.go`:

```go
func TestEngineConfigVPCCIDRRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")

	cfg := BanyanConfig{
		Engine: EngineConfig{
			GRPCPort: "50051",
			VPCCIDR:  "10.20.0.0/16",
		},
	}

	if err := SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Engine.VPCCIDR != "10.20.0.0/16" {
		t.Errorf("expected vpc_cidr=10.20.0.0/16, got %q", loaded.Engine.VPCCIDR)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd pkg/types && go test ./... -run TestEngineConfigVPCCIDRRoundTrip -v`
Expected: FAIL — compile error `unknown field 'VPCCIDR' in struct literal of type EngineConfig`.

- [ ] **Step 3: Add the field**

In `pkg/types/config.go`, inside `type EngineConfig struct`, add the field next to the other string fields (e.g. directly after `StoreAddress`):

```go
	VPCCIDR             string `yaml:"vpc_cidr,omitempty"`
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd pkg/types && go test ./... -run TestEngineConfigVPCCIDRRoundTrip -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/types/config.go pkg/types/config_test.go
git commit -m "feat(config): add VPCCIDR field to EngineConfig"
```

---

## Task 2: Free-CIDR detection helper

**Files:**
- Modify: `pkg/engine/engine.go`
- Test: `pkg/engine/engine_test.go`

This reuses the existing `listInterfaceAddrsFunc` (a swappable var, `engine.go:1053`),
`ifaceAddr` type (`engine.go:1026`), and `banyanInterfaces` map (`engine.go:1057`).

- [ ] **Step 1: Write the failing tests**

Add to `pkg/engine/engine_test.go`:

```go
func TestSuggestFreeCIDR(t *testing.T) {
	origFunc := listInterfaceAddrsFunc
	t.Cleanup(func() { listInterfaceAddrsFunc = origFunc })

	t.Run("returns first candidate when nothing conflicts", func(t *testing.T) {
		listInterfaceAddrsFunc = func() ([]ifaceAddr, error) {
			return []ifaceAddr{
				{Name: "eth0", Addr: &net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)}},
			}, nil
		}

		got, err := suggestFreeCIDR()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "10.10.0.0/16" {
			t.Errorf("expected first candidate 10.10.0.0/16, got %q", got)
		}
	})

	t.Run("skips occupied candidates", func(t *testing.T) {
		listInterfaceAddrsFunc = func() ([]ifaceAddr, error) {
			return []ifaceAddr{
				{Name: "eth0", Addr: &net.IPNet{IP: net.ParseIP("10.10.0.5"), Mask: net.CIDRMask(24, 32)}},
				{Name: "eth1", Addr: &net.IPNet{IP: net.ParseIP("10.20.0.5"), Mask: net.CIDRMask(24, 32)}},
			}, nil
		}

		got, err := suggestFreeCIDR()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "10.30.0.0/16" {
			t.Errorf("expected 10.30.0.0/16 (first two occupied), got %q", got)
		}
	})

	t.Run("returns empty string when all candidates conflict", func(t *testing.T) {
		listInterfaceAddrsFunc = func() ([]ifaceAddr, error) {
			var addrs []ifaceAddr
			for i, c := range candidateCIDRs {
				ip, _, _ := net.ParseCIDR(c)
				addrs = append(addrs, ifaceAddr{
					Name: "eth" + string(rune('0'+i)),
					Addr: &net.IPNet{IP: ip, Mask: net.CIDRMask(24, 32)},
				})
			}
			return addrs, nil
		}

		got, err := suggestFreeCIDR()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty string when all conflict, got %q", got)
		}
	})

	t.Run("ignores banyan-managed interfaces", func(t *testing.T) {
		listInterfaceAddrsFunc = func() ([]ifaceAddr, error) {
			return []ifaceAddr{
				{Name: "banyan0", Addr: &net.IPNet{IP: net.ParseIP("10.10.0.1"), Mask: net.CIDRMask(24, 32)}},
			}, nil
		}

		got, err := suggestFreeCIDR()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "10.10.0.0/16" {
			t.Errorf("expected 10.10.0.0/16 (banyan0 ignored), got %q", got)
		}
	})
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd pkg/engine && go test ./... -run TestSuggestFreeCIDR -v`
Expected: FAIL — `undefined: suggestFreeCIDR` and `undefined: candidateCIDRs`.

- [ ] **Step 3: Implement the helper**

In `pkg/engine/engine.go`, add directly after the `checkCIDRConflict` function (after line ~1090):

```go
// candidateCIDRs are private ranges tried in order when suggesting a
// conflict-free VPC CIDR. 10.0.0.0/16 is excluded (it is the common cloud
// default and frequently collides with the host subnet), and 10.200.0.0/16 is
// excluded (reserved for the control tunnel, see pkg/types.ControlTunnelCIDR).
var candidateCIDRs = []string{
	"10.10.0.0/16", "10.20.0.0/16", "10.30.0.0/16",
	"10.50.0.0/16", "10.100.0.0/16",
	"172.20.0.0/16", "172.30.0.0/16",
}

// suggestFreeCIDR returns the first candidate range that does not overlap any
// host network interface, or "" if every candidate conflicts.
func suggestFreeCIDR() (string, error) {
	for _, candidate := range candidateCIDRs {
		if err := checkCIDRConflict(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd pkg/engine && go test ./... -run TestSuggestFreeCIDR -v`
Expected: PASS (all four sub-tests).

- [ ] **Step 5: Run the existing conflict test to confirm no regression**

Run: `cd pkg/engine && go test ./... -run TestCheckCIDRConflict -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/engine/engine.go pkg/engine/engine_test.go
git commit -m "feat(engine): add suggestFreeCIDR helper for conflict-free VPC ranges"
```

---

## Task 3: Augment the start-time conflict error with a suggestion

**Files:**
- Modify: `pkg/engine/engine.go` (around line 188-191, inside `Run`)

- [ ] **Step 1: Update the error message**

In `pkg/engine/engine.go`, replace the existing conflict block:

```go
		if err := checkCIDRConflict(e.opts.VPCCIDR); err != nil {
			return fmt.Errorf("VPC CIDR conflict: %w", err)
		}
```

With:

```go
		if err := checkCIDRConflict(e.opts.VPCCIDR); err != nil {
			suggestion, _ := suggestFreeCIDR()
			if suggestion != "" {
				return fmt.Errorf("VPC CIDR conflict: %w\n"+
					"  → Suggested free range: %s\n"+
					"  → Fix: re-run `sudo banyan-engine init` to choose a range, "+
					"or start with --vpc-cidr %s",
					err, suggestion, suggestion)
			}
			return fmt.Errorf("VPC CIDR conflict: %w\n"+
				"  → No free candidate range found; specify one with --vpc-cidr", err)
		}
```

- [ ] **Step 2: Verify the package builds and tests pass**

Run: `cd pkg/engine && go build ./... && go test ./... -run 'TestCheckCIDRConflict|TestSuggestFreeCIDR' -v`
Expected: build succeeds; tests PASS.

- [ ] **Step 3: Commit**

```bash
git add pkg/engine/engine.go
git commit -m "feat(engine): suggest a free range in VPC CIDR conflict error"
```

---

## Task 4: Read VPC CIDR from config at start (flag precedence)

**Files:**
- Modify: `cmd/banyan-engine/cmd/engine.go` (inside `runEngineStart`, after config load at line ~767)

This mirrors the existing pattern for `grpc-port` at `engine.go:775`.

- [ ] **Step 1: Add the config-read block**

In `cmd/banyan-engine/cmd/engine.go`, inside `runEngineStart`, immediately after the existing
gRPC-port block:

```go
	// Read gRPC port from config if not overridden by flags
	if !cmd.Flags().Changed("grpc-port") && cfg.Engine.GRPCPort != "" {
		engineGRPCPort = cfg.Engine.GRPCPort
	}
```

add:

```go
	// Read VPC CIDR from config if not overridden by flags
	if !cmd.Flags().Changed("vpc-cidr") && cfg.Engine.VPCCIDR != "" {
		engineVPCCIDR = cfg.Engine.VPCCIDR
	}
```

(`engineVPCCIDR` is the package-level var bound to the `--vpc-cidr` flag at `engine.go:121`,
and is already passed to `engine.New` at `engine.go:883`.)

- [ ] **Step 2: Verify the module builds**

Run: `cd cmd/banyan-engine && go build ./...`
Expected: build succeeds, no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/banyan-engine/cmd/engine.go
git commit -m "feat(engine-cli): use VPC CIDR from config at start with flag precedence"
```

---

## Task 5: VPC CIDR step in the init wizard

**Files:**
- Modify: `cmd/banyan-engine/cmd/engine.go` (inside `runEngineInit`)

Two insertion points, both before the `--- Save config ---` block at `engine.go:650`:
the WireGuard keypair block ends at `engine.go:283`. Place the new step after it.

This task has no unit test (it is interactive TUI wiring); it is verified by build +
a manual non-interactive run. The underlying logic (`suggestFreeCIDR`, config persistence)
is already covered by Tasks 1-2.

- [ ] **Step 1: Add the VPC CIDR selection step**

In `cmd/banyan-engine/cmd/engine.go`, in `runEngineInit`, insert the following block
immediately after the WireGuard keypair block (after line ~283, before the
`// --- Deployment mode ---` comment):

```go
	// --- VPC CIDR selection ---
	fmt.Println()
	fmt.Println(styleInfo.Render("Configuring VPC CIDR (internal network range for containers)..."))

	suggested, _ := engine.SuggestFreeVPCCIDR()
	flagCIDR, _ := cmd.Flags().GetString("vpc-cidr")
	cidrFlagSet := cmd.Flags().Changed("vpc-cidr")

	if nonInteractive {
		chosen := flagCIDR
		if !cidrFlagSet {
			chosen = suggested
		}
		if chosen == "" {
			return fmt.Errorf("no free VPC CIDR found automatically; specify one with --vpc-cidr")
		}
		if err := engine.ValidateVPCCIDR(chosen); err != nil {
			if suggested != "" {
				return fmt.Errorf("%w (suggested free range: %s)", err, suggested)
			}
			return err
		}
		existingCfg.Engine.VPCCIDR = chosen
		fmt.Printf("  %s VPC CIDR: %s\n", styleOK.Render("[OK]"), chosen)
	} else {
		vpcCIDR := suggested
		if vpcCIDR == "" {
			fmt.Printf("  %s No free range auto-detected; please enter one manually.\n", styleWarn.Render("[WARN]"))
		}
		cidrForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("VPC CIDR (internal network range for containers)").
					Description("Must not overlap any host interface. Avoid 10.0.0.0/16 on cloud VMs.").
					Value(&vpcCIDR).
					Validate(func(s string) error {
						return engine.ValidateVPCCIDR(s)
					}),
			),
		)
		if err := cidrForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nInitialization cancelled.")
				return nil
			}
			return fmt.Errorf("VPC CIDR form error: %w", err)
		}
		existingCfg.Engine.VPCCIDR = vpcCIDR
		fmt.Printf("  %s VPC CIDR: %s\n", styleOK.Render("[OK]"), vpcCIDR)
	}
```

- [ ] **Step 2: Export the helpers needed by the CLI**

The CLI (`cmd/banyan-engine`) is a separate module and can only call exported
identifiers from `pkg/engine`. Add these exported wrappers to `pkg/engine/engine.go`,
directly after `suggestFreeCIDR`:

```go
// SuggestFreeVPCCIDR returns the first candidate VPC CIDR that does not overlap
// any host interface, or "" if none is free. Exported for the engine CLI.
func SuggestFreeVPCCIDR() (string, error) {
	return suggestFreeCIDR()
}

// ValidateVPCCIDR returns an error if cidr is not a valid CIDR or overlaps a
// host network interface. Exported for the engine CLI.
func ValidateVPCCIDR(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("VPC CIDR cannot be empty")
	}
	return checkCIDRConflict(cidr)
}
```

(`checkCIDRConflict` already returns `invalid VPC CIDR: ...` for unparseable input,
so `ValidateVPCCIDR` covers both parse errors and host overlaps.)

- [ ] **Step 3: Verify both modules build**

Run: `cd pkg/engine && go build ./... && cd ../../cmd/banyan-engine && go build ./...`
Expected: both build succeed.

- [ ] **Step 4: Run the engine package tests**

Run: `cd pkg/engine && go test ./... -run 'TestCheckCIDRConflict|TestSuggestFreeCIDR' -v`
Expected: PASS (exported wrappers delegate to tested functions).

- [ ] **Step 5: Manual smoke test (non-interactive)**

Build the binary and run init non-interactively in a throwaway dir to confirm a CIDR is
selected and persisted:

```bash
cd cmd/banyan-engine && go build -o /tmp/banyan-engine .
sudo /tmp/banyan-engine init --non-interactive --admin-user admin --admin-password adminpass123
grep vpc_cidr /etc/banyan/config.yaml
```

Expected: init completes; `config.yaml` contains a `vpc_cidr:` line with a non-conflicting
range (not `10.0.0.0/16` on a cloud host). Note: this writes to `/etc/banyan`; run only on
a disposable machine/VM/container.

- [ ] **Step 6: Commit**

```bash
git add pkg/engine/engine.go cmd/banyan-engine/cmd/engine.go
git commit -m "feat(engine-cli): prompt for and persist VPC CIDR during init"
```

---

## Self-Review Checklist

After completing all tasks, verify:

1. **Spec coverage:**
   - [ ] Persist VPC CIDR in config → Task 1
   - [ ] `suggestFreeCIDR()` + candidate pool (10.x → 172.x, excludes 10.0/16 & 10.200/16) → Task 2
   - [ ] Improved conflict error at start → Task 3
   - [ ] Start reads config with flag precedence → Task 4
   - [ ] Wizard prompt (interactive) + non-interactive handling → Task 5

2. **Placeholder scan:**
   - [ ] No "TBD"/"TODO"/"handle edge cases" — every code step shows full code.

3. **Type/name consistency:**
   - [ ] `suggestFreeCIDR()` (internal) and `SuggestFreeVPCCIDR()` (exported wrapper) used consistently.
   - [ ] `ValidateVPCCIDR()` used in both interactive and non-interactive branches of Task 5.
   - [ ] `candidateCIDRs` referenced consistently in Task 2 implementation and tests.
   - [ ] `cfg.Engine.VPCCIDR` / `existingCfg.Engine.VPCCIDR` field name matches Task 1.

4. **Build & tests:**
   - [ ] `cd pkg/types && go test ./...` passes
   - [ ] `cd pkg/engine && go test ./...` passes
   - [ ] `cd cmd/banyan-engine && go build ./...` succeeds

5. **Integration:**
   - [ ] All commits made
   - [ ] No uncommitted changes remain
