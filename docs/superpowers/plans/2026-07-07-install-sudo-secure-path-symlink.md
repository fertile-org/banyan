# Installer secure_path symlink — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After install, `sudo banyan-{engine,agent,cli}` work out of the box on every supported OS by symlinking the three binaries from `/usr/local/bin` into `/usr/sbin` (which is on sudo's `secure_path` everywhere), without touching sudo config.

**Architecture:** Add one shared helper `link_secure_path()` to `install-deps.sh` (sourced by both installers). It symlinks a named binary from `INSTALL_DIR` into `LINK_DIR` (default `/usr/sbin`), refusing to clobber a pre-existing real file. Each installer calls it after installing each banyan binary, and re-asserts `INSTALL_DIR`/`LINK_DIR` defaults locally so the entry scripts no longer depend implicitly on the sourced file.

**Tech Stack:** Bash (`set -euo pipefail`), shellcheck, plain-bash unit test harness.

Design spec: `docs/superpowers/specs/2026-07-07-install-sudo-secure-path-symlink-design.md`

---

## File structure

- `install-deps.sh` — shared library. Owns `INSTALL_DIR`/`LINK_DIR` defaults and the new `link_secure_path()` helper. Safe to `source` (no top-level execution beyond var/function definitions).
- `install.sh` — release-binary installer. Re-asserts defaults after sourcing; calls `link_secure_path` in `install_binary()`.
- `install-from-source.sh` — build-from-source installer. Re-asserts defaults after sourcing; calls `link_secure_path` in `build_binary()`.
- `test/os-compat/test-link-secure-path.sh` — new isolated unit test for `link_secure_path()` (no network, no root).

---

## Task 1: Add `LINK_DIR` + `link_secure_path()` helper to `install-deps.sh`

**Files:**
- Create: `test/os-compat/test-link-secure-path.sh`
- Modify: `install-deps.sh:7` (add `LINK_DIR` after `INSTALL_DIR`) and after the output helpers (`fatal()` at `install-deps.sh:27`) to add the function

- [ ] **Step 1: Write the failing test**

Create `test/os-compat/test-link-secure-path.sh` with exactly this content:

```bash
#!/usr/bin/env bash
# Unit tests for link_secure_path() in install-deps.sh.
# Sources the library and exercises the function against temp dirs.
# No network, no root required.
#
# Run: bash test/os-compat/test-link-secure-path.sh

# NOTE: deliberately no `set -e` — some cases assert a command's exit code.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# install-deps.sh is a sourced library (only var/function definitions at top
# level), so sourcing it here is safe.
# shellcheck source=../../install-deps.sh
source "${PROJECT_ROOT}/install-deps.sh"

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  FAIL: $1"; }

WORK=""
new_env() {
    WORK=$(mktemp -d)
    INSTALL_DIR="${WORK}/install"
    LINK_DIR="${WORK}/link"
    mkdir -p "$INSTALL_DIR" "$LINK_DIR"
    : > "${INSTALL_DIR}/banyan-cli"
    chmod +x "${INSTALL_DIR}/banyan-cli"
}
cleanup() { rm -rf "$WORK"; }

# 1. Creates a symlink that resolves to INSTALL_DIR/<name>
new_env
link_secure_path banyan-cli >/dev/null 2>&1
if [ -L "${LINK_DIR}/banyan-cli" ] && \
   [ "$(readlink "${LINK_DIR}/banyan-cli")" = "${INSTALL_DIR}/banyan-cli" ]; then
    pass "creates symlink to INSTALL_DIR"
else
    fail "creates symlink to INSTALL_DIR"
fi
cleanup

# 2. Idempotent — second call keeps a correct symlink and returns 0
new_env
link_secure_path banyan-cli >/dev/null 2>&1
link_secure_path banyan-cli >/dev/null 2>&1
rc=$?
if [ "$rc" -eq 0 ] && \
   [ "$(readlink "${LINK_DIR}/banyan-cli")" = "${INSTALL_DIR}/banyan-cli" ]; then
    pass "idempotent on repeat call"
else
    fail "idempotent on repeat call"
fi
cleanup

# 3. Self-link guard — LINK_DIR == INSTALL_DIR must not replace the real file
new_env
LINK_DIR="$INSTALL_DIR"
link_secure_path banyan-cli >/dev/null 2>&1
if [ -f "${INSTALL_DIR}/banyan-cli" ] && [ ! -L "${INSTALL_DIR}/banyan-cli" ]; then
    pass "self-link guard leaves file untouched"
else
    fail "self-link guard leaves file untouched"
fi
cleanup

# 4. Missing-dir guard — absent LINK_DIR is a no-op and returns 0
new_env
rm -rf "$LINK_DIR"
link_secure_path banyan-cli >/dev/null 2>&1
rc=$?
if [ "$rc" -eq 0 ] && [ ! -e "${LINK_DIR}/banyan-cli" ]; then
    pass "missing-dir guard is a no-op"
else
    fail "missing-dir guard is a no-op"
fi
cleanup

# 5. Real-file guard — a pre-existing regular file is left intact
new_env
echo "real" > "${LINK_DIR}/banyan-cli"
link_secure_path banyan-cli >/dev/null 2>&1
if [ ! -L "${LINK_DIR}/banyan-cli" ] && \
   [ "$(cat "${LINK_DIR}/banyan-cli")" = "real" ]; then
    pass "real-file guard leaves file intact"
else
    fail "real-file guard leaves file intact"
fi
cleanup

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"
[ "$FAIL" -eq 0 ]
```

Make it executable:

```bash
chmod +x test/os-compat/test-link-secure-path.sh
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash test/os-compat/test-link-secure-path.sh`
Expected: FAIL — every case fails because `link_secure_path` is not defined yet (bash prints `link_secure_path: command not found` and the assertions fail), ending with a non-zero `Results: 0 passed, 5 failed`.

- [ ] **Step 3: Add `LINK_DIR` default in `install-deps.sh`**

In `install-deps.sh`, right after line 7 (`INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"`), add:

```bash
# Directory on sudo's secure_path where the banyan commands are exposed via
# symlink, so `sudo banyan-*` resolves even when INSTALL_DIR (e.g.
# /usr/local/bin) is not on sudo's secure_path (default on RHEL/Oracle/Fedora).
LINK_DIR="${LINK_DIR:-/usr/sbin}"
```

- [ ] **Step 4: Add the `link_secure_path()` helper**

In `install-deps.sh`, immediately after the output-helpers block (after `fatal() { error "$*"; exit 1; }` at line 27), add:

```bash

# link_secure_path exposes an installed binary on sudo's secure_path by
# symlinking it from INSTALL_DIR into LINK_DIR (default /usr/sbin). On RHEL/
# Oracle/Fedora, sudo's secure_path excludes /usr/local/bin, so `sudo banyan-*`
# fails with command-not-found even though the binary is installed. /usr/sbin
# and /usr/bin are on the default secure_path of every supported OS.
#
# It never clobbers a real file: if LINK_DIR/<name> already exists as a regular
# file (distro package, user-placed), it is left untouched with a warning. An
# existing symlink (ours) is refreshed idempotently.
link_secure_path() {
    local name=$1
    [ "$LINK_DIR" = "$INSTALL_DIR" ] && return 0   # avoid self-referential link
    [ -d "$LINK_DIR" ] || return 0                 # skip if target dir absent
    local target="${LINK_DIR}/${name}"
    if [ -e "$target" ] && [ ! -L "$target" ]; then
        warn "${target} is a real file — skipping symlink; use ${INSTALL_DIR}/${name} or the full path"
        return 0
    fi
    ln -sf "${INSTALL_DIR}/${name}" "$target"
    info "Linked ${target} -> ${INSTALL_DIR}/${name} (so 'sudo ${name}' works)"
}
```

Note: the success `info` line lives inside the helper (not in callers) so both installers report it consistently (DRY).

- [ ] **Step 5: Run test to verify it passes**

Run: `bash test/os-compat/test-link-secure-path.sh`
Expected: PASS — `Results: 5 passed, 0 failed`, exit code 0.

- [ ] **Step 6: Lint**

Run: `shellcheck install-deps.sh test/os-compat/test-link-secure-path.sh`
Expected: no new warnings. (Pre-existing SC1091 "not following sourced file" on the test's `source` line is acceptable — it is a dynamic path; the `# shellcheck source=` directive should suppress it.)

- [ ] **Step 7: Commit**

```bash
git add install-deps.sh test/os-compat/test-link-secure-path.sh
git commit -m "feat(install): add link_secure_path helper to expose banyan on sudo secure_path"
```

---

## Task 2: Wire `link_secure_path` into `install.sh` + explicit defaults

**Files:**
- Modify: `install.sh` after the source block (`install.sh:45`) and inside `install_binary()` (after `install.sh:110`)

- [ ] **Step 1: Re-assert defaults after sourcing**

In `install.sh`, immediately after line 45 (`[ -n "$DEPS_TMP" ] && rm -f "$DEPS_TMP"`), add:

```bash

# INSTALL_DIR / LINK_DIR are provided by install-deps.sh; re-assert defaults
# here so this script is correct on its own even if sourcing is reordered.
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
LINK_DIR="${LINK_DIR:-/usr/sbin}"
```

- [ ] **Step 2: Call the helper in `install_binary()`**

In `install.sh`, inside `install_binary()`, the tail currently reads (lines 107-111):

```bash
    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} ${BANYAN_VERSION} installed to ${INSTALL_DIR}/${name}"
}
```

Change it to:

```bash
    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} ${BANYAN_VERSION} installed to ${INSTALL_DIR}/${name}"
    link_secure_path "$name"
}
```

- [ ] **Step 3: Syntax check**

Run: `bash -n install.sh`
Expected: no output, exit code 0 (valid syntax).

- [ ] **Step 4: Confirm the wiring is present and correctly placed**

Run: `grep -n 'link_secure_path\|LINK_DIR' install.sh`
Expected: shows the `LINK_DIR` re-assert line and the `link_secure_path "$name"` call inside `install_binary()`.

- [ ] **Step 5: Lint**

Run: `shellcheck install.sh`
Expected: no new warnings.

- [ ] **Step 6: Commit**

```bash
git add install.sh
git commit -m "feat(install): symlink banyan binaries onto secure_path in install.sh"
```

---

## Task 3: Wire `link_secure_path` into `install-from-source.sh` + explicit defaults

**Files:**
- Modify: `install-from-source.sh` after the source block (`install-from-source.sh:42`) and inside `build_binary()` (after `install-from-source.sh:67`)

- [ ] **Step 1: Re-assert defaults after sourcing**

In `install-from-source.sh`, immediately after line 42 (`[ -n "$DEPS_TMP" ] && rm -f "$DEPS_TMP"`), add:

```bash

# INSTALL_DIR / LINK_DIR are provided by install-deps.sh; re-assert defaults
# here so this script is correct on its own even if sourcing is reordered.
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
LINK_DIR="${LINK_DIR:-/usr/sbin}"
```

- [ ] **Step 2: Call the helper in `build_binary()`**

In `install-from-source.sh`, inside `build_binary()`, the tail currently reads (lines 65-68):

```bash
    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} installed to ${INSTALL_DIR}/${name} (built from source)"
}
```

Change it to:

```bash
    chmod +x "$tmp"
    mv "$tmp" "${INSTALL_DIR}/${name}"

    info "${name} installed to ${INSTALL_DIR}/${name} (built from source)"
    link_secure_path "$name"
}
```

- [ ] **Step 3: Syntax check**

Run: `bash -n install-from-source.sh`
Expected: no output, exit code 0.

- [ ] **Step 4: Confirm the wiring is present and correctly placed**

Run: `grep -n 'link_secure_path\|LINK_DIR' install-from-source.sh`
Expected: shows the `LINK_DIR` re-assert line and the `link_secure_path "$name"` call inside `build_binary()`.

- [ ] **Step 5: Lint**

Run: `shellcheck install-from-source.sh`
Expected: no new warnings.

- [ ] **Step 6: Commit**

```bash
git add install-from-source.sh
git commit -m "feat(install): symlink banyan binaries onto secure_path in install-from-source.sh"
```

---

## Self-review notes

- **Spec coverage:** Design §1 (LINK_DIR + helper) → Task 1. §2 (call after each binary) → Tasks 2 & 3. §3 (explicit defaults) → Tasks 2 & 3 Step 1. Testing (5 cases + shellcheck) → Task 1 Steps 1/5/6 + `bash -n`/shellcheck in Tasks 2/3.
- **No placeholders:** all steps show exact code/commands and expected output.
- **Type/name consistency:** helper is `link_secure_path`, vars `INSTALL_DIR`/`LINK_DIR`, everywhere.
- **Deviation from spec (intentional):** the success `info` line is emitted inside `link_secure_path` rather than by each caller — DRY, keeps caller sites to a single `link_secure_path "$name"` line. Behavior is unchanged.
```
