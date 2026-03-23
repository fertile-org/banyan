#!/bin/bash
# Banyan Auto-Scaling & Scale Command E2E Test Runner
# Tests: banyan-cli scale command, incremental task operations
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$SCRIPT_DIR"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_test_pass() { echo -e "${GREEN}[PASS]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
log_test_fail() { echo -e "${RED}[FAIL]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

cleanup() {
    log_info "Cleaning up..."
    docker-compose down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

# Helper: count containers on a worker
count_containers() {
    local worker=$1
    docker exec "$worker" nerdctl ps -q 2>/dev/null | wc -l
}

echo "========================================="
echo "Banyan Scale E2E Test Suite"
echo "========================================="

# =================================================================
# Phase 1: Build and Start Cluster
# =================================================================
echo ""
echo "========================================="
echo "Phase 1: Build and Start Cluster"
echo "========================================="

log_info "Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-engine" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-engine" .)
(cd "$REPO_ROOT/cmd/banyan-agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-agent" .)
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
md5sum "$SCRIPT_DIR/bin/banyan-engine" "$SCRIPT_DIR/bin/banyan-agent" "$SCRIPT_DIR/bin/banyan-cli" \
    "$SCRIPT_DIR/scripts/engine-entrypoint.sh" "$SCRIPT_DIR/scripts/agent-entrypoint.sh" > "$SCRIPT_DIR/bin/.cache-bust"
log_info "Binaries built"

log_info "Building Docker images..."
docker-compose build 2>&1 | tail -3

log_info "Starting cluster..."
docker-compose up -d 2>&1 | tail -5

# Wait for engine to be healthy
log_info "Waiting for engine..."
for i in $(seq 1 40); do
    if docker exec banyan-engine banyan-cli engine 2>/dev/null | grep -qi "running"; then
        break
    fi
    sleep 3
done

# Wait for agents
log_info "Waiting for agents..."
sleep 20

# Pre-pull images
for worker in banyan-worker-1 banyan-worker-2; do
    docker exec "$worker" nerdctl pull redis:7-alpine >/dev/null 2>&1 || true
done

# =================================================================
# Phase 2: Deploy base application
# =================================================================
echo ""
echo "========================================="
echo "Phase 2: Deploy Base Application"
echo "========================================="

log_info "Deploying test app with 1 replica..."
docker exec banyan-engine banyan-cli up --file /examples/banyan-volumes.yaml 2>&1 || true
sleep 15

# Count initial containers
INITIAL_W1=$(count_containers banyan-worker-1)
INITIAL_W2=$(count_containers banyan-worker-2)
INITIAL_TOTAL=$((INITIAL_W1 + INITIAL_W2))

if [ "$INITIAL_TOTAL" -ge 1 ]; then
    log_test_pass "Base deployment: $INITIAL_TOTAL containers running"
else
    log_test_fail "Base deployment: no containers running"
fi

# =================================================================
# Phase 3: Scale Up
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: Scale Up"
echo "========================================="

log_info "Scaling db service to 3 replicas..."
docker exec banyan-engine banyan-cli scale e2e-volume-test db=3 2>&1 || {
    log_test_fail "Scale up command failed"
}

# Wait for new containers to start
log_info "Waiting for scale-up containers..."
sleep 20

SCALED_W1=$(count_containers banyan-worker-1)
SCALED_W2=$(count_containers banyan-worker-2)
SCALED_TOTAL=$((SCALED_W1 + SCALED_W2))

if [ "$SCALED_TOTAL" -ge 3 ]; then
    log_test_pass "Scale up: $INITIAL_TOTAL → $SCALED_TOTAL containers (expected >= 3)"
else
    log_test_fail "Scale up: expected >= 3 containers, got $SCALED_TOTAL"
fi

# Verify deployment is still RUNNING (not DEPLOYING)
DEPLOY_STATUS=$(docker exec banyan-engine banyan-cli deployment e2e-volume-test 2>&1) || true
if echo "$DEPLOY_STATUS" | grep -qi "running"; then
    log_test_pass "Scale up: deployment stays in RUNNING status"
else
    log_test_fail "Scale up: deployment should be RUNNING"
    echo "  Status: $DEPLOY_STATUS"
fi

# =================================================================
# Phase 4: Scale Down
# =================================================================
echo ""
echo "========================================="
echo "Phase 4: Scale Down"
echo "========================================="

log_info "Scaling db service to 1 replica..."
docker exec banyan-engine banyan-cli scale e2e-volume-test db=1 2>&1 || {
    log_test_fail "Scale down command failed"
}

# Wait for containers to be removed (includes 5s graceful drain)
log_info "Waiting for scale-down (includes graceful drain)..."
sleep 20

DOWN_W1=$(count_containers banyan-worker-1)
DOWN_W2=$(count_containers banyan-worker-2)
DOWN_TOTAL=$((DOWN_W1 + DOWN_W2))

# Should have fewer containers than after scale up
if [ "$DOWN_TOTAL" -lt "$SCALED_TOTAL" ]; then
    log_test_pass "Scale down: $SCALED_TOTAL → $DOWN_TOTAL containers"
else
    log_test_fail "Scale down: expected fewer than $SCALED_TOTAL, got $DOWN_TOTAL"
fi

# =================================================================
# Phase 5: Clean up
# =================================================================
echo ""
echo "========================================="
echo "Phase 5: Clean Up"
echo "========================================="

log_info "Running 'down'..."
docker exec banyan-engine banyan-cli down --file /examples/banyan-volumes.yaml 2>/dev/null || true
sleep 10

FINAL_W1=$(count_containers banyan-worker-1)
FINAL_W2=$(count_containers banyan-worker-2)
FINAL_TOTAL=$((FINAL_W1 + FINAL_W2))
if [ "$FINAL_TOTAL" -eq 0 ]; then
    log_test_pass "Clean up: all containers removed"
else
    log_test_fail "Clean up: $FINAL_TOTAL containers still running"
fi

# =================================================================
# Results
# =================================================================
echo ""
echo "========================================="
echo "Scale E2E Test Results"
echo "========================================="
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    log_error "Some tests failed!"
    exit 1
fi

log_info "All scale E2E tests passed!"
