#!/bin/bash
# Local Environment Test Suite
# Run by: run-local.sh --test

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EXAMPLES_DIR="$REPO_ROOT/test/e2e/examples"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; }
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

TESTS_PASSED=0
TESTS_FAILED=0

pass() { log_pass "$1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
fail() { log_fail "$1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

# ---------------------------------------------------------------------------
# Test 1: Deploy basic application
# ---------------------------------------------------------------------------
log_info "Test 1: Deploy basic application"

banyan-cli up --file "$EXAMPLES_DIR/banyan.yaml"
sleep 10

DEPLOY_STATUS=$(banyan-cli deployment 2>/dev/null || true)
if echo "$DEPLOY_STATUS" | grep -qi "running"; then
    pass "Deployment is running"
else
    fail "Deployment not running"
    echo "$DEPLOY_STATUS"
fi

# ---------------------------------------------------------------------------
# Test 2: Verify agent shows up
# ---------------------------------------------------------------------------
log_info "Test 2: Verify agent registration"

AGENT_STATUS=$(banyan-cli agent 2>/dev/null || true)
if echo "$AGENT_STATUS" | grep -q "local-agent"; then
    pass "Agent local-agent is registered"
else
    fail "Agent local-agent not found"
    echo "$AGENT_STATUS"
fi

# ---------------------------------------------------------------------------
# Test 3: Verify containers are running
# ---------------------------------------------------------------------------
log_info "Test 3: Verify containers"

CONTAINER_COUNT=$(nerdctl ps -q 2>/dev/null | wc -l)
if [ "$CONTAINER_COUNT" -gt 0 ]; then
    pass "Containers are running (count: $CONTAINER_COUNT)"
else
    fail "No containers running"
fi

# ---------------------------------------------------------------------------
# Test 4: Down command
# ---------------------------------------------------------------------------
log_info "Test 4: Down command"

banyan-cli down --file "$EXAMPLES_DIR/banyan.yaml"
sleep 5

CONTAINER_COUNT_AFTER=$(nerdctl ps -q 2>/dev/null | wc -l)
if [ "$CONTAINER_COUNT_AFTER" -eq 0 ]; then
    pass "All containers removed after down"
else
    fail "Containers still running after down (count: $CONTAINER_COUNT_AFTER)"
fi

# ---------------------------------------------------------------------------
# Test 5: Engine status shows no running deployments
# ---------------------------------------------------------------------------
log_info "Test 5: Post-down engine status"

DEPLOY_STATUS_AFTER=$(banyan-cli deployment 2>/dev/null || true)
if ! echo "$DEPLOY_STATUS_AFTER" | grep -qi "running"; then
    pass "No running deployments after down"
else
    fail "Deployments still running after down"
    echo "$DEPLOY_STATUS_AFTER"
fi

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------
echo ""
echo "========================================"
echo "Local Test Results"
echo "========================================"
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================"

if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
fi

log_info "All local tests passed!"
