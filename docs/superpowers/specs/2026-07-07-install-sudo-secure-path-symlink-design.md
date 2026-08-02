# Installer: expose banyan binaries on sudo's secure_path — Design Spec

**Date:** 2026-07-07
**Status:** Approved (pending spec review)

## Problem

The install scripts place the banyan binaries in `/usr/local/bin` (`INSTALL_DIR` default). On Oracle Linux / RHEL / Fedora / CentOS, `sudo` runs with `env_reset` and resets `PATH` to a fixed `secure_path` declared in `/etc/sudoers`. That default `secure_path` (`/sbin:/bin:/usr/sbin:/usr/bin`) **does not include `/usr/local/bin`**.

As a result, after a successful install, typing `sudo banyan-engine init` fails with `command not found` — even though the binary is installed and executable at `/usr/local/bin/banyan-engine`. The problem is purely that the command name does not resolve under `sudo` because `/usr/local/bin` is absent from `secure_path`. Debian/Ubuntu include `/usr/local/bin` in their default `secure_path`, so they are unaffected; RHEL-family and some others are affected.

Observed on staging (2026-07-07, Oracle Linux 9.7 / arm64 / OCI): install log reported `banyan-engine ... installed to /usr/local/bin/banyan-engine` and `verify` passed, yet `sudo banyan-engine init` → `sudo: banyan-engine: command not found`.

### Secondary issue (code smell)

`install.sh` and `install-from-source.sh` both reference `$INSTALL_DIR` (e.g. `mv "$tmp" "${INSTALL_DIR}/${name}"`) but neither script defines it. The value only exists because they source `install-deps.sh`, which sets `INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"` at line 7. This is an implicit coupling: reading `install.sh` alone, the variable appears undefined. It is not a live bug today (`install.sh` uses `set -euo pipefail`, so an unset `INSTALL_DIR` would abort loudly rather than misbehave, and the source happens before first use), but it is fragile against reordering/refactoring.

## Goal

After install, `sudo banyan-engine`, `sudo banyan-agent`, and `sudo banyan-cli` work out of the box on every supported OS, without the operator having to edit sudo config or use full paths. Keep the real binaries at `/usr/local/bin` (FHS-correct home for locally installed software) and make the `INSTALL_DIR`/`LINK_DIR` configuration explicit in each entry script.

## Chosen approach

Expose the three banyan commands on `secure_path` by **symlinking** them from `INSTALL_DIR` into a directory that is on the default `secure_path` of every supported OS (`/usr/sbin`). This has the smallest blast radius: it exposes only the three banyan names, without widening `secure_path` for all sudo commands (which is what RHEL deliberately avoids) and without moving the binaries out of `/usr/local/bin`.

### Approaches considered (and rejected)

- **sudoers.d drop-in adding `/usr/local/bin` to `secure_path`.** Rejected: changes `secure_path` for *all* sudo invocations on the host — the exact hardening RHEL disables by default — so an installer doing it silently weakens system policy. Also requires `visudo -c` validation and 0440 perms.
- **Install binaries directly into `/usr/bin`.** Rejected: `/usr/bin` is conventionally package-manager territory; a `curl | bash` installer writing there is less clean than keeping `/usr/local/bin` as the real home.

## Design

### 1. Shared config + helper in `install-deps.sh`

`install-deps.sh` is sourced by both installers, so the symlink logic lives here once (DRY).

Alongside `INSTALL_DIR` (line 7), add:

```bash
LINK_DIR="${LINK_DIR:-/usr/sbin}"
```

Add a helper:

```bash
# link_secure_path exposes an installed binary on sudo's secure_path by
# symlinking it from INSTALL_DIR into LINK_DIR (default /usr/sbin). On RHEL/
# Oracle/Fedora, sudo's secure_path excludes /usr/local/bin, so `sudo banyan-*`
# fails with command-not-found even though the binary is installed. /usr/sbin
# and /usr/bin are on the default secure_path of every supported OS.
#
# It never clobbers a real file: if LINK_DIR/<name> already exists as a regular
# file (e.g. placed by a distro package or the user), it is left untouched and a
# warning is emitted. An existing symlink (ours) is refreshed idempotently.
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
}
```

The `-e && ! -L` guard means: only skip when a **real (non-symlink) file** already occupies the target. In the actual bug scenario `LINK_DIR/<name>` does not exist at all (the installer only ever wrote `/usr/local/bin`), so the guard does not trigger and the symlink is created — the fix applies. The guard only affects the rare case of a pre-existing real file, which it declines to destroy.

### 2. Call the helper after each banyan binary is installed

- `install.sh` → at the end of `install_binary()` (after the `mv` + info at lines 108/110): `link_secure_path "$name"`, then an info line noting the symlink.
- `install-from-source.sh` → at the end of its build-install function (after the `mv` + info at lines 65/67): `link_secure_path "$name"`.

Only the three banyan binaries (`banyan-cli`, `banyan-engine`, `banyan-agent`) are symlinked — exactly the commands an operator types. Dependencies installed into `INSTALL_DIR` by `install-deps.sh` (etcd, registry, nerdctl, buildkit) are not symlinked: they are invoked by the services via absolute paths, not typed by the operator (YAGNI).

### 3. Make `INSTALL_DIR` / `LINK_DIR` explicit in the entry scripts

Immediately after sourcing `install-deps.sh`, in **both** `install.sh` and `install-from-source.sh`:

```bash
# INSTALL_DIR / LINK_DIR are provided by install-deps.sh; re-assert defaults
# here so this script is correct on its own even if sourcing is reordered.
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
LINK_DIR="${LINK_DIR:-/usr/sbin}"
```

Because of `:-`, defining them twice is harmless: `install-deps.sh` set them first, and these lines keep those values. Reading either entry script alone now shows both variables defined.

## Non-Goals (YAGNI)

- **No uninstall / symlink removal.** No uninstall script exists in the repo today. When one is added later, it must also `rm` the `LINK_DIR` symlinks (see Risks: dangling symlinks). Out of scope here.
- **No symlinking of dependency binaries** (etcd, registry, nerdctl, buildkit).
- **No change to `secure_path` or any sudo configuration.**
- **No change to systemd units.** `ExecStart` already uses the absolute `${INSTALL_DIR}/banyan-*` path, so services are unaffected by this change.

## Risks & notes

- **Dangling symlink (most realistic future footgun).** Because there is no uninstaller, if the real binary in `/usr/local/bin` is removed while the `/usr/sbin` symlink remains, `sudo banyan-*` fails with `No such file or directory`. Mitigation deferred to a future uninstall script; low likelihood since the binary only disappears if the user deletes it.
- **Real file at `LINK_DIR/<name>`.** Handled by the `-e && ! -L` guard: never clobbered, warning emitted, operator falls back to the full path. Practically never happens for the banyan names (not distro-packaged).
- **usrmerge systems** where `/usr/sbin` is a symlink to `/usr/bin`: `ln -sf` resolves fine and the result is still on `secure_path`.
- **Non-root LINK_DIR write:** installers already enforce root, so `/usr/sbin` is writable.

## Testing

- **Unit (isolated), for `link_secure_path`** — source `install-deps.sh`, point `INSTALL_DIR`/`LINK_DIR` at temp dirs, `touch` a fake binary, then assert:
  - symlink is created and resolves to `INSTALL_DIR/<name>`;
  - idempotent — calling twice leaves a correct symlink, no error;
  - self-link guard — `LINK_DIR == INSTALL_DIR` creates nothing;
  - missing-dir guard — absent `LINK_DIR` is a no-op, no error;
  - real-file guard — a pre-existing regular file at the target is left intact and no symlink replaces it (warning path).
- **Lint:** `shellcheck` on all three scripts — no new warnings.
- **Manual on Oracle Linux 9:** after install, `sudo banyan-cli --help` runs without any sudoers change; `ls -la /usr/sbin/banyan-*` shows symlinks into `/usr/local/bin`.

## Affected files

| File | Change |
|------|--------|
| `install-deps.sh` | Add `LINK_DIR` default; add `link_secure_path()` helper |
| `install.sh` | Re-assert `INSTALL_DIR`/`LINK_DIR` defaults; call `link_secure_path "$name"` in `install_binary()` |
| `install-from-source.sh` | Re-assert `INSTALL_DIR`/`LINK_DIR` defaults; call `link_secure_path "$name"` after build-install |
| (test) | Add isolated test for `link_secure_path` |
