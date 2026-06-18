# Multi-OS Support & Install Script Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the `BASH_SOURCE` unbound variable bug when running `curl | bash -s`, refactor OS detection into an extensible family-based registry, and add Docker-based OS compatibility testing.

**Architecture:** Replace per-function `case "$OS"` blocks with a centralized OS Family Registry using bash associative arrays. Package names are resolved via `install_pkg()` which looks up OS-specific overrides, then family defaults. A Docker-based test runner validates the install script across multiple OS containers.

**Tech Stack:** Bash, Docker, shellcheck

---

## File Structure

| File | Responsibility |
|------|----------------|
| `install.sh` | Main installer (release binaries). Modified to fix `BASH_SOURCE` fallback. |
| `install-from-source.sh` | Source installer. Modified to fix `BASH_SOURCE` fallback. |
| `install-deps.sh` | Shared dependency functions. Refactored: add OS Family Registry, `install_pkg()` helper, simplify install functions. |
| `test/os-compat/test-os.sh` | Docker-based test runner. Spins up containers, runs install, verifies results. |
| `test/os-compat/os-list.txt` | List of Docker OS images to test against. |

---

## Task 1: Fix BASH_SOURCE in install.sh

**Files:**
- Modify: `install.sh:20`

- [ ] **Step 1: Read current install.sh**

Read `/home/work/freelancer/banyan/install.sh` to confirm the current line 20.

- [ ] **Step 2: Apply the fix**

Replace:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
```

With:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
```

This uses `$0` as fallback when `BASH_SOURCE[0]` is unset (e.g., `curl | bash -s`).

- [ ] **Step 3: Verify the fix with a local dry-run**

Run:
```bash
cd /home/work/freelancer/banyan
echo 'set -euo pipefail' | bash -s -c 'SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"; echo "$SCRIPT_DIR"'
```

Expected output: a path (no "unbound variable" error).

- [ ] **Step 4: Lint with shellcheck**

Run:
```bash
cd /home/work/freelancer/banyan
shellcheck install.sh
```

Expected: No errors or warnings.

- [ ] **Step 5: Commit**

```bash
git add install.sh
git commit -m "fix(install): handle BASH_SOURCE unbound when piped via curl"
```

---

## Task 2: Fix BASH_SOURCE in install-from-source.sh

**Files:**
- Modify: `install-from-source.sh:17`

- [ ] **Step 1: Read current install-from-source.sh**

Read `/home/work/freelancer/banyan/install-from-source.sh` to confirm the current line 17.

- [ ] **Step 2: Apply the same fix**

Replace:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
```

With:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
```

- [ ] **Step 3: Verify with shellcheck**

Run:
```bash
cd /home/work/freelancer/banyan
shellcheck install-from-source.sh
```

Expected: No errors or warnings.

- [ ] **Step 4: Commit**

```bash
git add install-from-source.sh
git commit -m "fix(install-from-source): handle BASH_SOURCE unbound when piped via curl"
```

---

## Task 3: Add OS Family Registry to install-deps.sh

**Files:**
- Modify: `install-deps.sh`

- [ ] **Step 1: Read current install-deps.sh**

Read the full file to understand the current `detect_os()` and install functions.

- [ ] **Step 2: Add OS Family Registry after the output helpers**

Insert after the `fatal()` function and before `detect_os()`:

```bash
# --- OS Family Registry ---

# Map OS ID -> family
declare -A OS_FAMILY=(
    [ubuntu]="debian"       [debian]="debian"       [pop]="debian"
    [linuxmint]="debian"    [zorin]="debian"        [elementary]="debian"
    [neon]="debian"
    [rhel]="rhel"           [centos]="rhel"         [fedora]="rhel"
    [rocky]="rhel"          [almalinux]="rhel"      [ol]="rhel"
    [amazon]="rhel"
    [arch]="arch"
    [sles]="suse"           [opensuse-leap]="suse"
    [alpine]="alpine"
)

# Package name overrides: family -> package name
# If an OS needs a different name than its family default, add OS-specific entry
declare -A PKG_ETCD=(
    [debian]="etcd-server"
)

declare -A PKG_CONTAINERD=(
    [debian]="containerd"
    [rhel]="containerd"
)

declare -A PKG_NFS=(
    [debian]="nfs-common"
    [rhel]="nfs-utils"
)

declare -A PKG_WIREGUARD=(
    [debian]="wireguard-tools"
    [rhel]="wireguard-tools"
    [alpine]="wireguard-tools"
)

get_family() {
    echo "${OS_FAMILY[$OS]:-unknown}"
}

# install_pkg <key>
# Looks up PKG_<KEY>[$OS], then PKG_<KEY>[$FAMILY], then falls back to <key>
install_pkg() {
    local key=$1
    local var_name="PKG_${key^^}"
    local pkg_name=""

    # OS-specific override
    eval "pkg_name=\${${var_name}[$OS]:-}"

    # Family default
    if [ -z "$pkg_name" ]; then
        local family
        family=$(get_family)
        eval "pkg_name=\${${var_name}[$family]:-}"
    fi

    # Fallback to key itself
    pkg_name="${pkg_name:-$key}"

    $PKG_INSTALL "$pkg_name"
}
```

- [ ] **Step 3: Simplify install_etcd()**

Replace the existing `install_etcd()` function with:

```bash
install_etcd() {
    if command -v etcd &>/dev/null; then
        info "etcd already installed, skipping."
        return
    fi

    info "Installing etcd..."

    local family
    family=$(get_family)

    if [ "$family" = "debian" ]; then
        install_pkg "etcd"
    else
        info "Downloading etcd v${ETCD_VERSION} binary..."
        local url="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz"
        local tmp
        tmp=$(mktemp -d)
        if ! curl -fsSL "$url" | tar -xz -C "$tmp" --strip-components=1; then
            rm -rf "$tmp"
            fatal "Failed to download etcd from ${url}"
        fi
        mv "$tmp/etcd" "$tmp/etcdctl" "${INSTALL_DIR}/"
        rm -rf "$tmp"
    fi

    info "etcd installed."
}
```

- [ ] **Step 4: Simplify install_containerd()**

Replace the existing `install_containerd()` function with:

```bash
install_containerd() {
    if command -v containerd &>/dev/null; then
        info "containerd already installed, skipping."
    else
        info "Installing containerd..."

        local family
        family=$(get_family)

        if [ "$family" = "debian" ]; then
            $PKG_UPDATE
            install_pkg "containerd"
        else
            $PKG_INSTALL containerd.io 2>/dev/null || install_pkg "containerd"
        fi
    fi

    if systemctl is-active --quiet containerd 2>/dev/null; then
        info "containerd is running."
    else
        info "Starting containerd..."
        systemctl enable --now containerd
        info "containerd started."
    fi
}
```

- [ ] **Step 5: Simplify install_wireguard()**

Replace the existing `install_wireguard()` function with:

```bash
install_wireguard() {
    if command -v wg &>/dev/null; then
        info "wireguard-tools already installed, skipping."
    else
        info "Installing wireguard-tools..."

        local family
        family=$(get_family)

        if [ "$family" = "debian" ]; then
            $PKG_UPDATE
        fi

        install_pkg "wireguard"
    fi

    if ip link add wg-test type wireguard 2>/dev/null; then
        ip link delete wg-test 2>/dev/null
        info "WireGuard kernel support: OK"
    else
        error "WireGuard kernel module not available."
        error "  - WireGuard is required for overlay networking and the control tunnel."
        error "To enable WireGuard: modprobe wireguard (requires Linux 5.6+ or wireguard-dkms)"
        if [ "$ROLE" = "agent" ] || [ "$ROLE" = "all" ]; then
            fatal "WireGuard is required for agents. Install the kernel module and try again."
        fi
    fi
}
```

- [ ] **Step 6: Simplify install_nfs_client()**

Replace the existing `install_nfs_client()` function with:

```bash
install_nfs_client() {
    if command -v mount.nfs &>/dev/null; then
        info "NFS client already installed, skipping."
        return
    fi

    info "Installing NFS client tools..."

    local family
    family=$(get_family)

    if [ "$family" = "debian" ]; then
        $PKG_UPDATE
    fi

    install_pkg "nfs"

    info "NFS client installed."
}
```

- [ ] **Step 7: Verify with shellcheck**

Run:
```bash
cd /home/work/freelancer/banyan
shellcheck install-deps.sh
```

Expected: No errors or warnings.

- [ ] **Step 8: Test the script parses correctly**

Run:
```bash
cd /home/work/freelancer/banyan
bash -n install-deps.sh
```

Expected: No output (success).

- [ ] **Step 9: Commit**

```bash
git add install-deps.sh
git commit -m "refactor(install): add OS Family Registry, simplify install functions"
```

---

## Task 4: Create OS Compatibility Test Runner

**Files:**
- Create: `test/os-compat/test-os.sh`
- Create: `test/os-compat/os-list.txt`

- [ ] **Step 1: Create test directory**

```bash
mkdir -p /home/work/freelancer/banyan/test/os-compat
```

- [ ] **Step 2: Create os-list.txt**

Create `/home/work/freelancer/banyan/test/os-compat/os-list.txt`:

```
ubuntu:24.04
ubuntu:22.04
debian:12
oraclelinux:9
rockylinux:9
almalinux:9
fedora:40
amazonlinux:2023
archlinux:latest
opensuse/leap:15
alpine:3.20
```

- [ ] **Step 3: Create test-os.sh**

Create `/home/work/freelancer/banyan/test/os-compat/test-os.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Banyan OS Compatibility Test Runner
# Tests install scripts across multiple OS containers
#
# Usage:
#   ./test-os.sh                    # Test all OSes
#   ./test-os.sh oraclelinux:9      # Test single OS
#   ./test-os.sh --role engine      # Test with specific role

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OS_LIST="${SCRIPT_DIR}/os-list.txt"

ROLE="all"
SINGLE_OS=""

# Parse args
while [ $# -gt 0 ]; do
    case "$1" in
        --role)
            ROLE="${2:-}"
            shift 2
            ;;
        *)
            SINGLE_OS="$1"
            shift
            ;;
    esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0

run_test() {
    local os_image=$1
    local os_id
    os_id=$(echo "$os_image" | tr '/:' '-')
    echo "========================================"
    echo "Testing: $os_image (role: $ROLE)"
    echo "========================================"

    # Skip if Docker not available
    if ! command -v docker &>/dev/null; then
        echo -e "${YELLOW}SKIP${NC}: docker not available"
        return 0
    fi

    # Create container
    local container_name="banyan-test-${os_id}"
    docker rm -f "$container_name" 2>/dev/null || true

    if ! docker run -d --name "$container_name" \
        --privileged \
        -v "${PROJECT_ROOT}/install.sh:/tmp/install.sh:ro" \
        -v "${PROJECT_ROOT}/install-deps.sh:/tmp/install-deps.sh:ro" \
        -v "${PROJECT_ROOT}/install-from-source.sh:/tmp/install-from-source.sh:ro" \
        "$os_image" \
        sleep 3600 >/dev/null 2>&1; then
        echo -e "${YELLOW}SKIP${NC}: Could not start container for $os_image"
        return 0
    fi

    # Install prerequisites in container
    docker exec "$container_name" bash -c '
        if command -v apt-get &>/dev/null; then
            apt-get update -qq && apt-get install -y -qq curl tar gzip ca-certificates procps iproute2
        elif command -v dnf &>/dev/null; then
            dnf install -y -q curl tar gzip ca-certificates procps iproute
        elif command -v apk &>/dev/null; then
            apk add --no-cache curl tar gzip ca-certificates procps iproute2
        elif command -v pacman &>/dev/null; then
            pacman -Sy --noconfirm curl tar gzip ca-certificates procps iproute2
        elif command -v zypper &>/dev/null; then
            zypper install -y curl tar gzip ca-certificates procps iproute2
        fi
    ' >/dev/null 2>&1 || true

    # Run install script (dry-run mode for testing - just verify it parses and detects OS)
    local test_script
    test_script=$(cat <<'EOF'
set -euo pipefail
cd /tmp
# Source install-deps to test detect_os
source install-deps.sh
# Mock ROLE for testing
ROLE="all"
detect_os
detect_arch
echo "DETECTED_OS=$OS"
echo "DETECTED_FAMILY=$(get_family)"
echo "DETECTED_ARCH=$ARCH"
echo "PKG_INSTALL=$PKG_INSTALL"
EOF
)

    if docker exec "$container_name" bash -c "$test_script" >"/tmp/banyan-test-${os_id}.log" 2>&1; then
        echo -e "${GREEN}PASS${NC}: $os_image"
        cat "/tmp/banyan-test-${os_id}.log"
        ((PASS++))
    else
        echo -e "${RED}FAIL${NC}: $os_image"
        cat "/tmp/banyan-test-${os_id}.log"
        ((FAIL++))
    fi

    docker rm -f "$container_name" >/dev/null 2>&1 || true
}

# Run tests
if [ -n "$SINGLE_OS" ]; then
    run_test "$SINGLE_OS"
else
    while IFS= read -r os_image || [ -n "$os_image" ]; do
        # Skip comments and empty lines
        [[ "$os_image" =~ ^#.*$ ]] && continue
        [ -z "$os_image" ] && continue
        run_test "$os_image"
    done < "$OS_LIST"
fi

echo ""
echo "========================================"
echo "Results: $PASS passed, $FAIL failed"
echo "========================================"

[ "$FAIL" -eq 0 ]
```

- [ ] **Step 4: Make test-os.sh executable**

```bash
chmod +x /home/work/freelancer/banyan/test/os-compat/test-os.sh
```

- [ ] **Step 5: Test the test script itself**

Run:
```bash
cd /home/work/freelancer/banyan
bash -n test/os-compat/test-os.sh
```

Expected: No output (success).

- [ ] **Step 6: Lint with shellcheck**

Run:
```bash
cd /home/work/freelancer/banyan
shellcheck test/os-compat/test-os.sh
```

Expected: No errors or warnings.

- [ ] **Step 7: Run a quick smoke test on one OS**

Run:
```bash
cd /home/work/freelancer/banyan
./test/os-compat/test-os.sh ubuntu:24.04
```

Expected: Container starts, OS is detected correctly, output shows `DETECTED_OS`, `DETECTED_FAMILY`, `DETECTED_ARCH`, `PKG_INSTALL`.

- [ ] **Step 8: Commit**

```bash
git add test/os-compat/
git commit -m "test(os-compat): add Docker-based OS compatibility test runner"
```

---

## Task 5: Update Makefile with test target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add test target**

Append to `Makefile`:

```makefile
# OS Compatibility Tests
# Test install scripts against multiple OS containers
# Requires: Docker
# Usage: make test-os-compat                    # Test all OSes
#        make test-os-compat OS=oraclelinux:9   # Test single OS
.PHONY: test-os-compat
test-os-compat:
ifdef OS
	./test/os-compat/test-os.sh $(OS)
else
	./test/os-compat/test-os.sh
endif
```

- [ ] **Step 2: Verify syntax**

Run:
```bash
cd /home/work/freelancer/banyan
make -n test-os-compat OS=ubuntu:24.04
```

Expected: Shows the command that would be run (dry-run).

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build(make): add test-os-compat target for OS compatibility testing"
```

---

## Task 6: Update Documentation

**Files:**
- Modify: `setup-stg-env.md`

- [ ] **Step 1: Update the install instructions**

Replace the temporary workaround sections with a note that the bug is fixed:

In both engine and agent install sections, replace:
```markdown
- Chạy lệnh trên thì bị lỗi này: 
bash: line 20: BASH_SOURCE[0]: unbound variable
bash: line 25: /home/opc/install-deps.sh: No such file or directory

- Nguyên nhân: ...
-> Đây là 1 bug trong docs ...

- Cách fix tạm thay vì pipe trực tiếp, tải script về rồi chạy:
```bash
cd ~ 
curl -sSLO https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh
curl -sSLO https://raw.githubusercontent.com/fertile-org/banyan/main/install-deps.sh
sudo bash install.sh --role engine
```
```

With:
```markdown
```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine
```
```

And similarly for the agent section:
```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
```

- [ ] **Step 2: Remove the workaround symlink sections**

Remove these lines from the engine section:
```bash
sudo ln -s /usr/local/bin/banyan-engine /usr/sbin/banyan-engine
sudo ln -s /usr/local/bin/banyan-cli /usr/sbin/banyan-cli
sudo ln -s /usr/local/bin/etcd /usr/sbin/etcd
```

Remove these lines from the agent section:
```bash
sudo ln -s /usr/local/bin/banyan-agent /usr/sbin/banyan-agent
sudo ln -s /usr/local/bin/banyan-cli /usr/sbin/banyan-cli
```

- [ ] **Step 3: Commit**

```bash
git add setup-stg-env.md
git commit -m "docs: update install instructions now that curl | bash works"
```

---

## Self-Review Checklist

After completing all tasks, run this checklist:

1. **Spec coverage:** Does every requirement in the design spec have a task?
   - [ ] Fix BASH_SOURCE in install.sh -> Task 1
   - [ ] Fix BASH_SOURCE in install-from-source.sh -> Task 2
   - [ ] OS Family Registry -> Task 3
   - [ ] Test infrastructure -> Task 4
   - [ ] Makefile target -> Task 5
   - [ ] Documentation update -> Task 6

2. **Placeholder scan:** Any "TBD", "TODO", "implement later"?
   - [ ] No placeholders found

3. **Type consistency:** Are function names consistent?
   - [ ] `get_family()` used consistently
   - [ ] `install_pkg()` used consistently
   - [ ] `OS_FAMILY` variable name consistent

4. **Manual verification:**
   - [ ] `bash -n` passes for all modified scripts
   - [ ] `shellcheck` passes for all modified scripts
   - [ ] At least one OS test passes in Docker

5. **Integration:**
   - [ ] All commits are made
   - [ ] No uncommitted changes remain
