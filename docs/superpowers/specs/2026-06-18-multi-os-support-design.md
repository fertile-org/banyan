# Multi-OS Support & Install Script Fix

**Date:** 2026-06-18
**Status:** Approved
**Author:** opencode

## Problem

1. **BASH_SOURCE bug:** `install.sh` crashes with `unbound variable` when run via `curl | bash -s` because `set -euo pipefail` (the `-u` flag) treats unset `BASH_SOURCE[0]` as an error.

2. **OS support is hard to extend:** Each install function (`install_etcd`, `install_wireguard`, `install_nfs_client`, etc.) has its own `case "$OS"` block. Adding a new OS requires editing every function. Supporting 20-30 distros this way is unmaintainable.

## Solution

### 1. Fix BASH_SOURCE

Replace:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
```

With:
```bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
```

This works for both `curl | bash -s` (where `BASH_SOURCE` is unset, falls back to `$0`) and `bash install.sh` (where `BASH_SOURCE[0]` is set).

Apply to: `install.sh`, `install-from-source.sh`

### 2. OS Family Registry

Move package name resolution from per-function `case` blocks to a centralized registry in `install-deps.sh`.

**Structure:**

```bash
# OS → Family mapping
declare -A OS_FAMILY=(
    [ubuntu]="debian"   [debian]="debian"   [pop]="debian"
    [linuxmint]="debian" [zorin]="debian"   [elementary]="debian" [neon]="debian"
    [rhel]="rhel"       [centos]="rhel"     [fedora]="rhel"
    [rocky]="rhel"      [almalinux]="rhel"  [ol]="rhel"          [amazon]="rhel"
    [arch]="arch"
    [sles]="suse"       [opensuse-leap]="suse"
    [alpine]="alpine"
)

# Package name overrides (by OS ID or family)
declare -A PKG_ETCD=(
    [debian]="etcd-server"
    # rhel/arch/suse/alpine: install from binary (no package)
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
```

**Helper function:**

```bash
get_family() { echo "${OS_FAMILY[$OS]:-unknown}" }

# install_pkg "nfs" → looks up PKG_NFS[$OS], then PKG_NFS[$FAMILY], then falls back to arg
install_pkg() {
    local key=$1
    local var_prefix="PKG_${key^^}"
    local pkg_name
    
    # OS-specific override
    eval "pkg_name=\${${var_prefix}[$OS]:-}"
    
    # Family default
    if [ -z "$pkg_name" ]; then
        local family
        family=$(get_family)
        eval "pkg_name=\${${var_prefix}[$family]:-}"
    fi
    
    # Fallback to the key itself
    pkg_name="${pkg_name:-$key}"
    
    $PKG_INSTALL "$pkg_name"
}
```

**Install functions become:**

```bash
install_etcd() {
    if command -v etcd &>/dev/null; then return; fi
    case "$(get_family)" in
        debian) install_pkg "etcd" ;;
        *) install_etcd_binary ;;  # download from GitHub
    esac
}

install_nfs_client() {
    if command -v mount.nfs &>/dev/null; then return; fi
    install_pkg "nfs"
}
```

**Adding a new OS:** Add 1 line to `OS_FAMILY` + package overrides only if names differ from family default.

### 3. Test Infrastructure

```
test/os-compat/
├── test-os.sh          # Test runner
└── os-list.txt         # OS images to test
```

**test-os.sh** iterates over `os-list.txt`, for each OS:
1. Creates a Docker container with install scripts mounted
2. Runs `bash install.sh --role engine` (or agent)
3. Verifies binaries exist, services are configured
4. Reports PASS/FAIL with logs

**Initial os-list.txt:**
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

**Usage:**
```bash
./test/os-compat/test-os.sh                    # All OSes
./test/os-compat/test-os.sh oraclelinux:9      # Single OS
./test/os-compat/test-os.sh --role engine      # Specific role
```

## Files Changed

| File | Change |
|------|--------|
| `install.sh` | Fix `BASH_SOURCE` fallback |
| `install-from-source.sh` | Fix `BASH_SOURCE` fallback |
| `install-deps.sh` | Add OS Family Registry, `install_pkg()` helper, simplify install functions |
| `test/os-compat/test-os.sh` | New: test runner |
| `test/os-compat/os-list.txt` | New: OS image list |
