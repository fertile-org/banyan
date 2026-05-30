# [HIGH-003] Dependency binaries have no checksum verification

**Severity**: High
**Responsibility**: Mitigation Gap
**Component**: Install Script
**File(s)**: `install-deps.sh:101-111,125-134,182-186,202-206,249-257`

## Description

The main `install.sh` verifies SHA-256 checksums for Banyan binaries before installation. However, `install-deps.sh` — which installs containerd, nerdctl, etcd, CNI plugins, BuildKit, and the OCI registry — has **no checksum verification** for any dependency.

## Evidence

**etcd** (`install-deps.sh:101-111`):
```bash
local url="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz"
if ! curl -fsSL "$url" | tar -xz -C "$tmp" --strip-components=1; then
# No checksum verification
```

Same pattern for:
- Distribution registry (line 125-134)
- nerdctl (line 182-186)
- CNI plugins (line 202-206)
- BuildKit (line 249-257)

Compare to `install.sh:67-84` which does checksum verification:
```bash
expected=$(grep "${name}-linux-${ARCH}" "$checksums_tmp" | awk '{print $1}')
if ! verify_checksum "$tmp" "$expected"; then
    fatal "Checksum verification failed"
fi
```

## Impact

**Who can exploit**: Supply chain attacker who can compromise a GitHub release or perform MITM on downloads.

**What they gain**: Execute arbitrary code as root on every machine that runs the install script — by substituting malicious binaries for any dependency.

**Blast radius**: All Banyan installations are affected if a dependency download is compromised.

## Recommendation

Apply the same checksum verification pattern from `install.sh` to `install-deps.sh`:

1. Fetch checksums.txt from each project's releases
2. Verify each downloaded binary against its checksum before extraction
3. Fail the install if any verification fails

Example for etcd:
```bash
# Get checksums
curl -fsSL "https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz.sha256" -o /tmp/etcd.sha256

# Extract expected hash
expected=$(grep "etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz" /tmp/etcd.sha256 | awk '{print $1}')

# Download and verify
curl -fsSL "$url" | sha256sum | grep "$expected" || fatal "Checksum failed"
```

## Secure Default Consideration

**Checklist I1**: "Binary checksum verification in install script — ENFORCE — Download checksums file, verify binary integrity before installation."

Banyan binaries are verified but dependencies are not — this is a gap that could be exploited via a compromised GitHub release.