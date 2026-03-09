# [HIGH-010] Install Script Has No Checksum Verification

**Severity**: High
**Status**: FIXED
**Responsibility**: Mitigation Gap
**Component**: Install Script
**File(s)**:
- `install.sh:102-119` (`install_binary` — no checksums)
- `install.sh:148-277` (dependency downloads — no checksums)

## Description

The `install_binary` function downloads binaries via `curl -fsSL` and immediately installs them to `/usr/local/bin/` with no checksum or GPG signature verification. The same applies to all dependency downloads (etcd, nerdctl, CNI plugins, BuildKit).

While HTTPS is used (positive) and versions are pinned (positive), there is no verification that the downloaded content matches expected hashes.

## Impact

- **Who**: Supply chain attacker who compromises GitHub releases, CDN, or performs a CDN cache poisoning attack
- **What they gain**: Execute arbitrary code as root on every machine that runs the install script
- **Blast radius**: Every new Banyan installation

## Recommendation

1. Publish SHA-256 checksums alongside releases
2. Download and verify checksums in the install script:
   ```bash
   curl -fsSL "$url" -o "$tmp"
   echo "$expected_sha256  $tmp" | sha256sum --check --strict
   ```
3. For dependencies, use the official checksum files published by each project (etcd, nerdctl, etc.)

## Fix

Added a `verify_checksum()` function in `install.sh` that downloads SHA-256 checksum files from the release and verifies downloaded binaries against them before installation. All binary downloads now go through checksum verification.
