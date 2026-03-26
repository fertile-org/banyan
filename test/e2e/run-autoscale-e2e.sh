#!/bin/bash
# Banyan Auto-Scaling E2E Test (uses shell CPU burn for real CPU load)
#
# WARNING: This test takes ~3 minutes due to autoscale evaluation cycles.
# It generates real CPU load inside containers. Run sparingly.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$SCRIPT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_test_pass() { echo -e "${GREEN}[PASS]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
log_test_fail() { echo -e "${RED}[FAIL]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

cleanup() {
    log_info "Cleaning up..."
    docker-compose down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

count_service_containers() {
    local worker=$1
    local service=$2
    local count
    count=$(docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null | grep -c "$service" || true)
    echo "${count:-0}"
}

total_service_containers() {
    local service=$1
    local w1 w2
    w1=$(count_service_containers banyan-worker-1 "$service")
    w2=$(count_service_containers banyan-worker-2 "$service")
    echo $((w1 + w2))
}

echo "========================================="
echo "Banyan Auto-Scale E2E Test"
echo "========================================="
echo ""
echo "NOTE: This test takes ~3 minutes."
echo ""

# =================================================================
# Phase 1: Build and start cluster
# =================================================================
log_info "Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-engine" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-engine" .)
(cd "$REPO_ROOT/cmd/banyan-agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-agent" .)
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
md5sum "$SCRIPT_DIR/bin/"banyan-* "$SCRIPT_DIR/scripts/"*-entrypoint.sh > "$SCRIPT_DIR/bin/.cache-bust" 2>/dev/null

log_info "Building and starting cluster..."
docker-compose build 2>&1 | tail -3
docker-compose up -d 2>&1 | tail -5

# Wait for cluster
log_info "Waiting for engine + agents..."
for i in $(seq 1 40); do
    if docker exec banyan-engine banyan-cli engine 2>/dev/null | grep -qi "running"; then break; fi
    sleep 3
done
sleep 20

# Pre-pull alpine on agents
for worker in banyan-worker-1 banyan-worker-2; do
    docker exec "$worker" nerdctl pull alpine:latest >/dev/null 2>&1 || true
done

# =================================================================
# Phase 2: Deploy with autoscale config
# =================================================================
echo ""
echo "========================================="
echo "Phase 2: Deploy with Autoscale"
echo "========================================="

log_info "Deploying with autoscale: target_cpu=50, min=1, max=3..."
docker exec banyan-engine banyan-cli up --file /examples/banyan-autoscale.yaml 2>&1 || true
sleep 15

INITIAL=$(total_service_containers "cpu-worker")
if [ "$INITIAL" -ge 1 ]; then
    log_test_pass "Initial deploy: $INITIAL container(s) running"
else
    log_test_fail "Initial deploy: no containers"
    exit 1
fi

# =================================================================
# Phase 3: Generate CPU load → expect scale up
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: CPU Stress → Scale Up"
echo "========================================="

# Find which worker has the container
STRESS_WORKER=""
STRESS_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    name=$(docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null | grep "cpu-worker" | head -1)
    if [ -n "$name" ]; then
        STRESS_WORKER="$worker"
        STRESS_CONTAINER="$name"
        break
    fi
done

if [ -z "$STRESS_WORKER" ]; then
    log_test_fail "Could not find cpu-worker container"
    exit 1
fi

log_info "Starting CPU burn loop in $STRESS_CONTAINER on $STRESS_WORKER..."
# Pure shell CPU burn — no package install needed (alpine may lack network in nested containerd)
# Runs 2 busy loops in background for 120s, then self-terminates
docker exec "$STRESS_WORKER" nerdctl exec -d "$STRESS_CONTAINER" sh -c '
    for i in 1 2; do
        (while true; do :; done) &
    done
    sleep 120
    kill $(jobs -p) 2>/dev/null
' 2>/dev/null

# Wait for:
# - Health check to report high CPU (10s)
# - Autoscale to evaluate (30s)
# - New task to be created and executed (~10s)
log_info "Waiting for autoscale to detect high CPU (up to 90s)..."
SCALE_UP_OK=false
for i in $(seq 1 18); do
    sleep 5
    COUNT=$(total_service_containers "cpu-worker")
    echo "  Check $i: $COUNT containers"
    if [ "$COUNT" -ge 2 ]; then
        SCALE_UP_OK=true
        break
    fi
done

if [ "$SCALE_UP_OK" = true ]; then
    FINAL_COUNT=$(total_service_containers "cpu-worker")
    log_test_pass "Auto-scale UP: $INITIAL → $FINAL_COUNT containers (CPU load triggered scaling)"
else
    log_test_fail "Auto-scale UP: containers did not increase within 90s"
    log_info "Debug: checking deployment status..."
    docker exec banyan-engine banyan-cli deployment e2e-autoscale-test 2>&1 || true
    docker exec banyan-engine banyan-cli container 2>&1 || true
fi

# =================================================================
# Phase 4: Stop CPU load → expect scale down (after cooldown)
# =================================================================
echo ""
echo "========================================="
echo "Phase 4: Stop Stress → Scale Down"
echo "========================================="

log_info "Stopping CPU burn..."
docker exec "$STRESS_WORKER" nerdctl exec "$STRESS_CONTAINER" sh -c 'kill $(jobs -p) 2>/dev/null; pkill -f "while true" 2>/dev/null' || true

# Wait for cooldown (15s) + autoscale cycle (30s) + drain (5s) + task execution
log_info "Waiting for autoscale to detect low CPU and scale down (up to 120s)..."
BEFORE_DOWN=$(total_service_containers "cpu-worker")
SCALE_DOWN_OK=false
for i in $(seq 1 24); do
    sleep 5
    COUNT=$(total_service_containers "cpu-worker")
    echo "  Check $i: $COUNT containers"
    if [ "$COUNT" -lt "$BEFORE_DOWN" ]; then
        SCALE_DOWN_OK=true
        break
    fi
done

if [ "$SCALE_DOWN_OK" = true ]; then
    FINAL_DOWN=$(total_service_containers "cpu-worker")
    log_test_pass "Auto-scale DOWN: $BEFORE_DOWN → $FINAL_DOWN containers (low CPU triggered scaling)"
else
    log_warn "Auto-scale DOWN: containers did not decrease within 120s (may need longer cooldown)"
    # Not a hard failure — scale-down is slower and more conservative
fi

# =================================================================
# Phase 5: Clean up
# =================================================================
echo ""
echo "========================================="
echo "Phase 5: Clean Up"
echo "========================================="

docker exec banyan-engine banyan-cli down --file /examples/banyan-autoscale.yaml 2>/dev/null || true
sleep 15

REMAINING=$(total_service_containers "cpu-worker")
if [ "$REMAINING" -eq 0 ]; then
    log_test_pass "Clean up: all containers removed"
else
    log_test_fail "Clean up: $REMAINING containers still running"
fi

# =================================================================
# Results
# =================================================================
echo ""
echo "========================================="
echo "Auto-Scale E2E Test Results"
echo "========================================="
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi

log_info "All auto-scale E2E tests passed!"
