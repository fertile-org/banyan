#!/bin/bash
# Banyan E2E Test Runner
# Tests: deployment, VPC networking, blue-green redeployment, down command
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

TESTS_PASSED=0
TESTS_FAILED=0

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_test_pass() { echo -e "${GREEN}[PASS]${NC} $1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
log_test_fail() { echo -e "${RED}[FAIL]${NC} $1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    docker-compose down -v --remove-orphans 2>/dev/null || true
}

# Trap for cleanup on exit
trap cleanup EXIT

# Wait for engine to be healthy via CLI status check
wait_for_healthy() {
    local container=$1
    local max_wait=$2
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if docker exec "$container" banyan-cli status >/dev/null 2>&1; then
            return 0
        fi
        echo "  Waiting for $container... (${elapsed}s)"
        sleep 3
        elapsed=$((elapsed + 3))
    done
    log_error "$container did not become healthy within ${max_wait}s"
    docker logs "$container" 2>&1 | tail -20
    return 1
}

# Wait for containers to be running on a worker
# Usage: wait_for_containers <worker> <min_count> <timeout_seconds>
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
        echo "  Waiting for containers on $worker... ($count/$min_count, ${elapsed}s)"
        sleep 3
        elapsed=$((elapsed + 3))
    done
    return 1
}

# Get a container's IP by inspecting it on a worker
# Usage: get_container_ip <worker> <container_name>
get_container_ip() {
    local worker=$1
    local container=$2
    docker exec "$worker" nerdctl inspect "$container" 2>/dev/null \
        | grep -oP '"IPAddress":\s*"\K[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' \
        | head -1
}

# Get all running container names on a worker
# Usage: get_container_names <worker>
get_container_names() {
    local worker=$1
    docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null | grep -v '^$'
}

echo "========================================="
echo "Banyan E2E Test Suite"
echo "========================================="

# =================================================================
# Phase 1: Build and Start Cluster
# =================================================================
echo ""
echo "========================================="
echo "Phase 1: Build and Start Cluster"
echo "========================================="

# Step 1: Build all binaries locally (avoids Docker DNS issues with Go proxy)
log_info "Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-engine" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-engine" .)
(cd "$REPO_ROOT/cmd/banyan-agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-agent" .)
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
log_info "Binaries built at test/e2e/bin/"

# Step 2: Build Docker images
log_info "Building Docker images..."
docker-compose build

# Step 3: Start the cluster
log_info "Starting Banyan cluster (1 engine + 2 workers)..."
docker-compose up -d

# Step 4: Wait for engine to be healthy
log_info "Waiting for engine to be healthy..."
wait_for_healthy banyan-engine 60
log_info "Engine is healthy!"

# Step 5: Wait for agents to register and VPC networking to initialize
log_info "Waiting for agents to register and VPC networking to initialize..."
sleep 15

# Step 6: Check engine status (shows agents)
log_info "Checking engine status..."
docker exec banyan-engine banyan-engine status || log_warn "Engine status check failed"

# =================================================================
# Phase 2: Deploy and Verify Basic Operation
# =================================================================
echo ""
echo "========================================="
echo "Phase 2: Deploy and Verify"
echo "========================================="

# Deploy test application
log_info "Deploying test application..."
docker exec banyan-engine banyan-cli up --file /examples/banyan.yaml

# Wait for containers to appear on both workers
log_info "Waiting for containers to start..."
wait_for_containers banyan-worker-1 1 60 || log_warn "Timed out waiting for containers on worker-1"
wait_for_containers banyan-worker-2 1 60 || log_warn "Timed out waiting for containers on worker-2"

# Verify deployment status
log_info "Deployment status:"
docker exec banyan-engine banyan-cli status

# Verify containers on workers
log_info "Containers on workers:"
echo "  worker-1:"
docker exec banyan-worker-1 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-1"
echo "  worker-2:"
docker exec banyan-worker-2 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-2"

# =================================================================
# Phase 3: VPC Networking Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: VPC Networking Tests"
echo "========================================="

# Test 1: Verify VXLAN overlay interface exists on agents
log_info "Test: VXLAN overlay interface exists on agents"
if docker exec banyan-worker-1 ip link show banyan.1 2>/dev/null | grep -q "banyan.1"; then
    log_test_pass "VXLAN interface banyan.1 exists on worker-1"
else
    log_test_fail "VXLAN interface banyan.1 not found on worker-1"
fi

if docker exec banyan-worker-2 ip link show banyan.1 2>/dev/null | grep -q "banyan.1"; then
    log_test_pass "VXLAN interface banyan.1 exists on worker-2"
else
    log_test_fail "VXLAN interface banyan.1 not found on worker-2"
fi

# Test 2: Verify containers get overlay IPs (10.0.x.x range)
log_info "Test: Containers have overlay IPs"
WORKER1_CONTAINERS=$(get_container_names banyan-worker-1)
WORKER2_CONTAINERS=$(get_container_names banyan-worker-2)

WORKER1_IP=""
WORKER1_CONTAINER=""
for container in $WORKER1_CONTAINERS; do
    IP=$(get_container_ip banyan-worker-1 "$container")
    if [[ "$IP" == 10.0.* ]]; then
        log_test_pass "Container $container on worker-1 has overlay IP: $IP"
        WORKER1_IP="$IP"
        WORKER1_CONTAINER="$container"
    elif [ -n "$IP" ]; then
        log_test_fail "Container $container on worker-1 has non-overlay IP: $IP (expected 10.0.x.x)"
    else
        log_warn "  Could not get IP for container $container on worker-1"
    fi
done

WORKER2_IP=""
WORKER2_CONTAINER=""
for container in $WORKER2_CONTAINERS; do
    IP=$(get_container_ip banyan-worker-2 "$container")
    if [[ "$IP" == 10.0.* ]]; then
        log_test_pass "Container $container on worker-2 has overlay IP: $IP"
        WORKER2_IP="$IP"
        WORKER2_CONTAINER="$container"
    elif [ -n "$IP" ]; then
        log_test_fail "Container $container on worker-2 has non-overlay IP: $IP (expected 10.0.x.x)"
    else
        log_warn "  Could not get IP for container $container on worker-2"
    fi
done

# Test 3: Cross-host container connectivity
log_info "Test: Cross-host container connectivity via VXLAN overlay"
if [ -n "$WORKER1_IP" ] && [ -n "$WORKER2_IP" ] && [ -n "$WORKER1_CONTAINER" ] && [ -n "$WORKER2_CONTAINER" ]; then
    # Ping from worker-1 container to worker-2 container
    if docker exec banyan-worker-1 nerdctl exec "$WORKER1_CONTAINER" ping -c 3 -W 5 "$WORKER2_IP" 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "Cross-host ping: $WORKER1_CONTAINER ($WORKER1_IP) -> $WORKER2_IP"
    else
        log_test_fail "Cross-host ping failed: $WORKER1_CONTAINER ($WORKER1_IP) -> $WORKER2_IP"
        # Debug: show ping output
        docker exec banyan-worker-1 nerdctl exec "$WORKER1_CONTAINER" ping -c 3 -W 5 "$WORKER2_IP" 2>&1 || true
    fi

    # Ping from worker-2 container to worker-1 container
    if docker exec banyan-worker-2 nerdctl exec "$WORKER2_CONTAINER" ping -c 3 -W 5 "$WORKER1_IP" 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "Cross-host ping: $WORKER2_CONTAINER ($WORKER2_IP) -> $WORKER1_IP"
    else
        log_test_fail "Cross-host ping failed: $WORKER2_CONTAINER ($WORKER2_IP) -> $WORKER1_IP"
        docker exec banyan-worker-2 nerdctl exec "$WORKER2_CONTAINER" ping -c 3 -W 5 "$WORKER1_IP" 2>&1 || true
    fi
else
    log_warn "Skipping cross-host ping (need at least one container with overlay IP on each worker)"
fi

# Test 4: Unique overlay IPs across replicas
log_info "Test: Unique IPs across all containers"
ALL_IPS=""
DUPLICATE_FOUND=false
for worker in banyan-worker-1 banyan-worker-2; do
    CONTAINERS=$(get_container_names "$worker")
    for container in $CONTAINERS; do
        IP=$(get_container_ip "$worker" "$container")
        if [ -n "$IP" ]; then
            if echo "$ALL_IPS" | grep -qw "$IP"; then
                log_test_fail "Duplicate IP $IP found on $worker/$container"
                DUPLICATE_FOUND=true
            fi
            ALL_IPS="$ALL_IPS $IP"
        fi
    done
done
if [ "$DUPLICATE_FOUND" = false ] && [ -n "$ALL_IPS" ]; then
    log_test_pass "All container IPs are unique:$ALL_IPS"
fi

# =================================================================
# Phase 4: Blue-Green Redeployment Test
# =================================================================
echo ""
echo "========================================="
echo "Phase 4: Blue-Green Redeployment"
echo "========================================="

# Save old container names
log_info "Recording current containers before redeployment..."
OLD_W1_CONTAINERS=$(get_container_names banyan-worker-1)
OLD_W2_CONTAINERS=$(get_container_names banyan-worker-2)
OLD_COUNT_W1=$(echo "$OLD_W1_CONTAINERS" | wc -w)
OLD_COUNT_W2=$(echo "$OLD_W2_CONTAINERS" | wc -w)
log_info "  Before: worker-1 has $OLD_COUNT_W1, worker-2 has $OLD_COUNT_W2 containers"

# Run 'up' again (triggers blue-green redeployment)
log_info "Running 'up' again (blue-green redeployment)..."
docker exec banyan-engine banyan-cli up --file /examples/banyan.yaml

# Wait for new containers to start
log_info "Waiting for redeployment to complete..."
sleep 20

# Verify new containers are running
NEW_W1_CONTAINERS=$(get_container_names banyan-worker-1)
NEW_W2_CONTAINERS=$(get_container_names banyan-worker-2)
NEW_COUNT_W1=$(echo "$NEW_W1_CONTAINERS" | wc -w)
NEW_COUNT_W2=$(echo "$NEW_W2_CONTAINERS" | wc -w)
log_info "  After: worker-1 has $NEW_COUNT_W1, worker-2 has $NEW_COUNT_W2 containers"

# The total container count should be similar (old ones torn down, new ones created)
TOTAL_OLD=$((OLD_COUNT_W1 + OLD_COUNT_W2))
TOTAL_NEW=$((NEW_COUNT_W1 + NEW_COUNT_W2))
if [ "$TOTAL_NEW" -ge "$TOTAL_OLD" ]; then
    log_test_pass "Blue-green redeployment: $TOTAL_NEW containers running (was $TOTAL_OLD)"
else
    log_test_fail "Blue-green redeployment: only $TOTAL_NEW containers (expected >= $TOTAL_OLD)"
fi

# Verify new containers also have overlay IPs
NEW_VPC_COUNT=0
for worker in banyan-worker-1 banyan-worker-2; do
    CONTAINERS=$(get_container_names "$worker")
    for container in $CONTAINERS; do
        IP=$(get_container_ip "$worker" "$container")
        if [[ "$IP" == 10.0.* ]]; then
            NEW_VPC_COUNT=$((NEW_VPC_COUNT + 1))
        fi
    done
done
if [ "$NEW_VPC_COUNT" -ge "$TOTAL_NEW" ] && [ "$NEW_VPC_COUNT" -gt 0 ]; then
    log_test_pass "Blue-green: all $NEW_VPC_COUNT redeployed containers have overlay IPs"
else
    log_test_fail "Blue-green: only $NEW_VPC_COUNT/$TOTAL_NEW containers have overlay IPs"
fi

# Show deployment status after redeployment
docker exec banyan-engine banyan-cli status

# =================================================================
# Phase 5: Down Command Test
# =================================================================
echo ""
echo "========================================="
echo "Phase 5: Down Command"
echo "========================================="

log_info "Running 'down' to tear down all services..."
docker exec banyan-engine banyan-cli down --file /examples/banyan.yaml

# Wait for containers to be removed
log_info "Waiting for containers to be removed..."
sleep 10

# Verify no app containers remain
DOWN_W1=$(docker exec banyan-worker-1 nerdctl ps -q 2>/dev/null | wc -l)
DOWN_W2=$(docker exec banyan-worker-2 nerdctl ps -q 2>/dev/null | wc -l)
TOTAL_REMAINING=$((DOWN_W1 + DOWN_W2))
if [ "$TOTAL_REMAINING" -eq 0 ]; then
    log_test_pass "Down command: all containers removed (0 remaining)"
else
    log_test_fail "Down command: $TOTAL_REMAINING containers still running"
    docker exec banyan-worker-1 nerdctl ps 2>/dev/null || true
    docker exec banyan-worker-2 nerdctl ps 2>/dev/null || true
fi

# Verify no running deployments remain (stopped records are expected)
DOWN_STATUS=$(docker exec banyan-engine banyan-cli status 2>&1)
if echo "$DOWN_STATUS" | grep -q "status: running"; then
    log_test_fail "Down command: engine still shows running deployments"
    echo "$DOWN_STATUS"
else
    log_test_pass "Down command: no running deployments remain"
fi

# =================================================================
# Results
# =================================================================
echo ""
echo "========================================="
echo "E2E Test Results"
echo "========================================="
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    log_error "Some tests failed!"
    exit 1
fi

log_info "All E2E tests passed!"
