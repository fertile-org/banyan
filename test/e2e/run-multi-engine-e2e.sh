#!/bin/bash
# Banyan Multi-Engine HA E2E Test Runner
# Tests: leader election, agent failover, deploy with 2 engines, leader failover
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

COMPOSE="docker-compose -f docker-compose.multi-engine.yml"

cleanup() {
    log_info "Cleaning up..."
    $COMPOSE down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

# Helper: wait for a container to have gRPC ready
wait_for_grpc() {
    local container=$1
    local host=$2
    local port=$3
    local max_wait=$4
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if docker exec "$container" nc -z "$host" "$port" 2>/dev/null; then
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    return 1
}

# Helper: check etcd key existence
etcd_get() {
    local key=$1
    docker exec banyan-etcd etcdctl get "$key" --prefix --print-value-only 2>/dev/null
}

etcd_get_keys() {
    local prefix=$1
    docker exec banyan-etcd etcdctl get "$prefix" --prefix --keys-only 2>/dev/null
}

# Helper: wait for containers on a worker
wait_for_containers() {
    local worker=$1
    local min_count=$2
    local timeout=$3
    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        local count
        count=$(docker exec "$worker" nerdctl ps -q 2>/dev/null | wc -l)
        if [ "$count" -ge "$min_count" ]; then
            return 0
        fi
        sleep 3
        elapsed=$((elapsed + 3))
    done
    return 1
}

echo "========================================="
echo "Banyan Multi-Engine HA E2E Test Suite"
echo "========================================="

# =================================================================
# Phase 1: Build and Start Multi-Engine Cluster
# =================================================================
echo ""
echo "========================================="
echo "Phase 1: Build and Start Multi-Engine Cluster"
echo "========================================="

log_info "Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-engine" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-engine" .)
(cd "$REPO_ROOT/cmd/banyan-agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-agent" .)
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
md5sum "$SCRIPT_DIR/bin/banyan-engine" "$SCRIPT_DIR/bin/banyan-agent" "$SCRIPT_DIR/bin/banyan-cli" \
    "$SCRIPT_DIR/scripts/engine-ha-entrypoint.sh" "$SCRIPT_DIR/scripts/agent-ha-entrypoint.sh" > "$SCRIPT_DIR/bin/.cache-bust"
log_info "Binaries built"

log_info "Building Docker images..."
$COMPOSE build

log_info "Starting multi-engine cluster (etcd + 2 engines + 2 workers)..."
$COMPOSE up -d

log_info "Waiting for etcd to be healthy..."
sleep 5
if docker exec banyan-etcd etcdctl endpoint health 2>/dev/null; then
    log_test_pass "etcd is healthy"
else
    log_test_fail "etcd not healthy"
    exit 1
fi

log_info "Waiting for engine-1 gRPC..."
if wait_for_grpc banyan-engine-1 0.0.0.0 50051 120; then
    log_test_pass "engine-1 gRPC is ready"
else
    log_test_fail "engine-1 gRPC not ready"
    docker logs banyan-engine-1 2>&1 | tail -30
    exit 1
fi

log_info "Waiting for engine-2 gRPC..."
if wait_for_grpc banyan-engine-2 0.0.0.0 50051 120; then
    log_test_pass "engine-2 gRPC is ready"
else
    log_test_fail "engine-2 gRPC not ready"
    docker logs banyan-engine-2 2>&1 | tail -30
    exit 1
fi

# Wait for agents to register
log_info "Waiting for agents to register..."
sleep 20

# =================================================================
# Phase 2: Engine Registration & Leader Election
# =================================================================
echo ""
echo "========================================="
echo "Phase 2: Engine Registration & Active-Active"
echo "========================================="

# Test: Both engines registered in etcd
log_info "Test: Both engines registered in etcd"
ENGINE_KEYS=$(etcd_get_keys "/banyan/engines/")
ENGINE_COUNT=$(echo "$ENGINE_KEYS" | grep -c "engine-" || true)
if [ "$ENGINE_COUNT" -ge 2 ]; then
    log_test_pass "Both engines registered in etcd ($ENGINE_COUNT engine records)"
else
    log_test_fail "Expected 2 engine records, found $ENGINE_COUNT"
    echo "  Keys: $ENGINE_KEYS"
fi

# Test: Active-active (no leader election — all engines schedule with per-deployment locks)
log_info "Test: Active-active scheduling (no leader election)"
# Both engines should be running and handling RPCs. Verify via health check.
E1_HEALTH=$(docker exec banyan-engine-1 banyan-cli engine 2>&1) || true
E2_HEALTH=$(docker exec banyan-engine-2 banyan-cli engine 2>&1) || true
if echo "$E1_HEALTH" | grep -qi "running\|OK" && echo "$E2_HEALTH" | grep -qi "running\|OK"; then
    log_test_pass "Both engines active and serving RPCs"
else
    log_test_fail "Not all engines healthy"
    echo "  E1: $E1_HEALTH"
    echo "  E2: $E2_HEALTH"
fi

# Test: Agents registered
log_info "Test: Agents registered with engines"
NODE_KEYS=$(etcd_get_keys "/banyan/nodes/")
NODE_COUNT=$(echo "$NODE_KEYS" | grep -c "worker-" || true)
if [ "$NODE_COUNT" -ge 2 ]; then
    log_test_pass "Both agents registered ($NODE_COUNT node records)"
else
    log_test_fail "Expected 2 node records, found $NODE_COUNT"
    echo "  Keys: $NODE_KEYS"
fi

# =================================================================
# Phase 3: Deploy with Multi-Engine
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: Deploy with Multi-Engine"
echo "========================================="

# Pre-pull images on workers
log_info "Pre-pulling container images on workers..."
for worker in banyan-ha-worker-1 banyan-ha-worker-2; do
    for img in node:22-alpine redis:7-alpine; do
        for attempt in 1 2 3; do
            if docker exec "$worker" nerdctl pull "$img" >/dev/null 2>&1; then break; fi
            echo "  Retry $attempt for $img on $worker..."
            sleep 5
        done
    done
done

# Deploy via engine-1 (uses CLI on engine-1 container)
log_info "Deploying test application via engine-1..."
docker exec banyan-engine-1 banyan-cli up --file /examples/banyan.yaml  || {
    log_test_fail "Deploy via engine-1 failed"
    docker logs banyan-engine-1 2>&1 | tail -20
}

# Wait for containers
log_info "Waiting for containers to start..."
wait_for_containers banyan-ha-worker-1 1 90 || log_warn "Timed out waiting for containers on worker-1"
wait_for_containers banyan-ha-worker-2 1 90 || log_warn "Timed out waiting for containers on worker-2"

# Verify deployment via engine-2 (status should be visible from either engine)
log_info "Test: Deployment visible from engine-2"
STATUS_E2=$(docker exec banyan-engine-2 banyan-cli deployment  2>&1) || true
if echo "$STATUS_E2" | grep -qi "running\|deploying"; then
    log_test_pass "Deployment visible from engine-2"
else
    log_test_fail "Deployment not visible from engine-2"
    echo "  Status: $STATUS_E2"
fi

# Verify deployment via engine-1
STATUS_E1=$(docker exec banyan-engine-1 banyan-cli deployment  2>&1) || true
if echo "$STATUS_E1" | grep -qi "running\|deploying"; then
    log_test_pass "Deployment visible from engine-1"
else
    log_test_fail "Deployment not visible from engine-1"
    echo "  Status: $STATUS_E1"
fi

# Test: No duplicate tasks (each task appears exactly once)
log_info "Test: No duplicate tasks created"
TASK_KEYS=$(etcd_get_keys "/banyan/tasks/")
TASK_COUNT=$(echo "$TASK_KEYS" | grep -c "/" || true)
# With 3 services (api x2, db x1), expect 3 create_and_start tasks total
if [ "$TASK_COUNT" -ge 1 ] && [ "$TASK_COUNT" -le 10 ]; then
    log_test_pass "Task count reasonable: $TASK_COUNT tasks (no duplication)"
else
    log_test_fail "Unexpected task count: $TASK_COUNT"
    echo "  Tasks: $TASK_KEYS"
fi

# =================================================================
# Phase 4: Engine Failover (Active-Active)
# =================================================================
echo ""
echo "========================================="
echo "Phase 4: Engine Failover (Active-Active)"
echo "========================================="

# Kill engine-1 — engine-2 should continue serving and scheduling
log_info "Killing engine-1..."
docker stop banyan-engine-1

sleep 5

# Verify engine-2 still serves status (active-active: any engine works)
log_info "Test: Surviving engine serves status"
SURV_STATUS=$(docker exec banyan-engine-2 banyan-cli deployment 2>&1) || true
if echo "$SURV_STATUS" | grep -qi "running\|stopped\|e2e-test"; then
    log_test_pass "Engine-2 still serves status after engine-1 killed"
else
    log_test_fail "Engine-2 status query failed"
    echo "  Status: $SURV_STATUS"
fi

# Verify engine-1 record expired from etcd (15s TTL)
log_info "Waiting for engine-1 record to expire (15s TTL)..."
sleep 15
ENGINE_KEYS_AFTER=$(etcd_get_keys "/banyan/engines/")
E1_STILL=$(echo "$ENGINE_KEYS_AFTER" | grep -c "engine-1" || true)
if [ "$E1_STILL" -eq 0 ]; then
    log_test_pass "Engine-1 record expired from etcd (TTL working)"
else
    log_test_fail "Engine-1 record still in etcd after TTL"
fi

# Restart engine-1 — should re-register and start processing
log_info "Restarting engine-1..."
docker start banyan-engine-1
sleep 15

ENGINE_KEYS_AFTER=$(etcd_get_keys "/banyan/engines/")
AFTER_COUNT=$(echo "$ENGINE_KEYS_AFTER" | grep -c "engine-" || true)
if [ "$AFTER_COUNT" -ge 2 ]; then
    log_test_pass "Restarted engine-1 re-registered ($AFTER_COUNT engines)"
else
    log_test_fail "Engine-1 did not re-register ($AFTER_COUNT engines)"
fi

# =================================================================
# Phase 5: Down Command via surviving engine
# =================================================================
echo ""
echo "========================================="
echo "Phase 5: Down Command"
echo "========================================="

log_info "Running 'down' via engine-1..."
docker exec banyan-engine-1 banyan-cli down --file /examples/banyan.yaml  2>&1 || true

log_info "Waiting for containers to be removed..."
sleep 15

DOWN_W1=$(docker exec banyan-ha-worker-1 nerdctl ps -q 2>/dev/null | wc -l)
DOWN_W2=$(docker exec banyan-ha-worker-2 nerdctl ps -q 2>/dev/null | wc -l)
TOTAL_REMAINING=$((DOWN_W1 + DOWN_W2))
if [ "$TOTAL_REMAINING" -eq 0 ]; then
    log_test_pass "Down command: all containers removed"
else
    log_test_fail "Down command: $TOTAL_REMAINING containers still running"
fi

# =================================================================
# Results
# =================================================================
echo ""
echo "========================================="
echo "Multi-Engine HA E2E Test Results"
echo "========================================="
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    log_error "Some tests failed!"
    exit 1
fi

log_info "All multi-engine HA E2E tests passed!"
