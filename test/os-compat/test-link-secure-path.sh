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
