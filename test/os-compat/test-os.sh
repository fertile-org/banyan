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
        PASS=$((PASS + 1))
    else
        echo -e "${RED}FAIL${NC}: $os_image"
        cat "/tmp/banyan-test-${os_id}.log"
        FAIL=$((FAIL + 1))
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
