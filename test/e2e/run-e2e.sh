#!/bin/bash
# Banyan E2E Test Runner
# Tests: deployment, resource-aware scheduling, VPC networking, blue-green redeployment, down command
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
        if docker exec "$container" banyan-cli engine >/dev/null 2>&1; then
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

# Test proxy by curling through a worker's host port
# Usage: test_proxy <source_container> <target_ip> <port> <description>
test_proxy() {
    local source=$1
    local target_ip=$2
    local port=$3
    local desc=$4
    local retries=10
    local i=0
    local http_code
    while [ $i -lt $retries ]; do
        http_code=$(docker exec "$source" curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://${target_ip}:${port}/" 2>/dev/null) || http_code="000"
        if [ "$http_code" = "200" ]; then
            log_test_pass "$desc (HTTP $http_code)"
            return 0
        fi
        i=$((i + 1))
        [ $i -lt $retries ] && sleep 3
    done
    log_test_fail "$desc (HTTP $http_code after $retries attempts)"
    # Debug: show verbose curl output and routing info on failure
    echo "  Debug: verbose curl from $source to ${target_ip}:${port}"
    docker exec "$source" curl -v --connect-timeout 5 "http://${target_ip}:${port}/" 2>&1 | head -20 || true
    echo "  Debug: iptables DNAT state on $source"
    docker exec "$source" iptables -t nat -L BANYAN-P-SVC-3000 -n 2>&1 || true
    echo "  Debug: routes on $source"
    docker exec "$source" ip route show 2>&1 | grep -E "^10\." || true
    echo "  Debug: ARP table on $source (overlay entries)"
    docker exec "$source" ip neigh show dev banyan0 2>&1 || true
    echo "  Debug: conntrack state for port $port on $source"
    docker exec "$source" conntrack -L -p tcp --dport "$port" 2>&1 | head -5 || true
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
# Generate cache-bust file so Docker detects binary and script changes
md5sum "$SCRIPT_DIR/bin/banyan-engine" "$SCRIPT_DIR/bin/banyan-agent" "$SCRIPT_DIR/bin/banyan-cli" \
    "$SCRIPT_DIR/scripts/engine-entrypoint.sh" "$SCRIPT_DIR/scripts/agent-entrypoint.sh" > "$SCRIPT_DIR/bin/.cache-bust"
log_info "Binaries built at test/e2e/bin/"

# Step 2: Build Docker images
log_info "Building Docker images..."
docker-compose build

# Step 3: Start the cluster
log_info "Starting Banyan cluster (1 engine + 2 workers)..."
docker-compose up -d

# Step 4: Wait for engine to be healthy
log_info "Waiting for engine to be healthy..."
wait_for_healthy banyan-engine 120
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

# Pre-pull images on workers to avoid deployment timeout
log_info "Pre-pulling container images on workers..."
for worker in banyan-worker-1 banyan-worker-2; do
    for img in node:22-alpine redis:7-alpine; do
        for attempt in 1 2 3; do
            if docker exec "$worker" nerdctl pull "$img" >/dev/null 2>&1; then break; fi
            echo "  Retry $attempt for $img on $worker..."
            sleep 5
        done
    done
done
log_info "Images pre-pulled"

# Deploy test application
log_info "Deploying test application..."
docker exec banyan-engine banyan-cli up --file /examples/banyan.yaml

# Wait for containers to appear on both workers
log_info "Waiting for containers to start..."
wait_for_containers banyan-worker-1 1 60 || log_warn "Timed out waiting for containers on worker-1"
wait_for_containers banyan-worker-2 1 60 || log_warn "Timed out waiting for containers on worker-2"

# Verify deployment status
log_info "Deployment status:"
docker exec banyan-engine banyan-cli deployment

# Verify containers on workers
log_info "Containers on workers:"
echo "  worker-1:"
docker exec banyan-worker-1 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-1"
echo "  worker-2:"
docker exec banyan-worker-2 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-2"

# Wait for API containers to be fully ready (Node.js needs time to start listening)
# Check from inside the container directly — curling localhost on the worker host
# doesn't work because iptables DNAT + 127.0.0.1 source requires route_localnet=1.
log_info "Waiting for API containers to become ready..."
API_READY_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    for c in $(docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null); do
        if [[ "$c" == *"-api-"* ]]; then
            API_READY_CONTAINER="$worker:$c"
            break 2
        fi
    done
done
READY=false
if [ -n "$API_READY_CONTAINER" ]; then
    WORKER="${API_READY_CONTAINER%%:*}"
    CONTAINER="${API_READY_CONTAINER##*:}"
    for i in $(seq 1 10); do
        if docker exec "$WORKER" nerdctl exec "$CONTAINER" wget -qO- --timeout=2 http://127.0.0.1:3000/ 2>/dev/null | grep -q "OK"; then
            READY=true
            break
        fi
        sleep 2
    done
fi
if [ "$READY" = true ]; then
    log_info "API containers are ready"
else
    log_warn "API containers may not be fully ready yet, continuing..."
fi

# =================================================================
# Phase 2b: env_file Resolution Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 2b: env_file Resolution Tests"
echo "========================================="

log_info "Test: env_file variables resolved into container environment"
ENVFILE_API_WORKER=""
ENVFILE_API_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    for c in $(get_container_names "$worker"); do
        if [[ "$c" == *"-api-"* ]]; then
            ENVFILE_API_WORKER="$worker"
            ENVFILE_API_CONTAINER="$c"
            break 2
        fi
    done
done

if [ -n "$ENVFILE_API_CONTAINER" ]; then
    # Test 1: REDIS_HOST from .env file is set
    REDIS_HOST_VAL=$(docker exec "$ENVFILE_API_WORKER" nerdctl exec "$ENVFILE_API_CONTAINER" printenv REDIS_HOST 2>/dev/null) || REDIS_HOST_VAL=""
    if [ "$REDIS_HOST_VAL" = "db.e2e-test-app.internal" ]; then
        log_test_pass "env_file: REDIS_HOST=db.e2e-test-app.internal set from .env file"
    else
        log_test_fail "env_file: REDIS_HOST expected 'db.e2e-test-app.internal', got '${REDIS_HOST_VAL}'"
    fi

    # Test 2: APP_MODE from .env file is set (second variable in file)
    APP_MODE_VAL=$(docker exec "$ENVFILE_API_WORKER" nerdctl exec "$ENVFILE_API_CONTAINER" printenv APP_MODE 2>/dev/null) || APP_MODE_VAL=""
    if [ "$APP_MODE_VAL" = "e2e-test" ]; then
        log_test_pass "env_file: APP_MODE=e2e-test set from .env file"
    else
        log_test_fail "env_file: APP_MODE expected 'e2e-test', got '${APP_MODE_VAL}'"
    fi
else
    log_warn "Skipping env_file tests (no API container found)"
fi

# =================================================================
# Phase 2c: Healthcheck and depends_on Condition Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 2c: Healthcheck & depends_on Tests"
echo "========================================="

# Test: depends_on long form with condition: service_started deployed successfully.
# The manifest uses long-form depends_on — if parsing the long-form YAML failed,
# the deploy in Phase 2 would have failed.
log_info "Test: depends_on long form (condition: service_started) parsed and deployed"
DEPLOY_STATUS=$(docker exec banyan-engine banyan-cli deployment e2e-test-app 2>&1) || true
if echo "$DEPLOY_STATUS" | grep -qi "running\|deploying"; then
    log_test_pass "depends_on: long form deployed successfully"
else
    log_test_fail "depends_on: deployment not running after long-form depends_on"
    echo "$DEPLOY_STATUS"
fi

# Test: DB container is running (validates depends_on doesn't block deployment)
log_info "Test: DB container running (depends_on resolved)"
DB_CONTAINER_RUNNING=false
for worker in banyan-worker-1 banyan-worker-2; do
    for c in $(get_container_names "$worker"); do
        if [[ "$c" == *"-db-"* ]]; then
            DB_CONTAINER_RUNNING=true
            log_test_pass "depends_on: DB container running on $worker"
            break 2
        fi
    done
done
if [ "$DB_CONTAINER_RUNNING" = false ]; then
    log_test_fail "depends_on: DB container not found on any worker"
fi

# =================================================================
# Phase 2d: Resource-Aware Scheduling Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 2d: Resource-Aware Scheduling Tests"
echo "========================================="

# Test: Agent metrics are reported via heartbeat
log_info "Test: Agent metrics populated after heartbeat"
AGENT_JSON=$(docker exec banyan-engine banyan-cli agent -o json 2>&1) || true

# Count agents with MemTotal > 0 (heartbeat reported SystemMetrics, engine stored on NodeRecord)
AGENTS_WITH_METRICS=0
for worker_name in worker-1 worker-2; do
    # Extract MemTotal for this agent — look for "MemTotal":NNNN where NNNN > 0
    AGENT_DETAIL=$(docker exec banyan-engine banyan-cli agent "$worker_name" -o json 2>&1) || true
    MEM_TOTAL=$(echo "$AGENT_DETAIL" | grep -oP '"MemTotal":\s*\K[0-9]+' | head -1) || MEM_TOTAL="0"
    CPU_CORES=$(echo "$AGENT_DETAIL" | grep -oP '"CPUCores":\s*\K[0-9]+' | head -1) || CPU_CORES="0"
    if [ "${MEM_TOTAL:-0}" -gt 0 ] 2>/dev/null && [ "${CPU_CORES:-0}" -gt 0 ] 2>/dev/null; then
        log_test_pass "Resource scheduling: $worker_name reports MemTotal=$MEM_TOTAL CPUCores=$CPU_CORES"
        AGENTS_WITH_METRICS=$((AGENTS_WITH_METRICS + 1))
    else
        log_test_fail "Resource scheduling: $worker_name has MemTotal=$MEM_TOTAL CPUCores=$CPU_CORES (expected > 0)"
    fi
done

# Test: Deployment succeeded with resource-aware scheduling (default: 512MB memory, 1 CPU per service)
log_info "Test: Deployment succeeded with resource-aware scheduling"
DEPLOY_STATUS=$(docker exec banyan-engine banyan-cli deployment e2e-test-app 2>&1) || true
if echo "$DEPLOY_STATUS" | grep -qi "running"; then
    log_test_pass "Resource scheduling: deployment running with resource requests"
else
    log_test_fail "Resource scheduling: deployment status not running"
    echo "$DEPLOY_STATUS"
fi

# Test: Tasks are distributed across agents (not all on one agent)
log_info "Test: Tasks distributed across agents"
SCHED_W1=$(docker exec banyan-worker-1 nerdctl ps -q 2>/dev/null | wc -l)
SCHED_W2=$(docker exec banyan-worker-2 nerdctl ps -q 2>/dev/null | wc -l)
if [ "$SCHED_W1" -gt 0 ] && [ "$SCHED_W2" -gt 0 ]; then
    log_test_pass "Resource scheduling: tasks spread across workers (w1=$SCHED_W1, w2=$SCHED_W2)"
else
    log_test_fail "Resource scheduling: tasks not distributed (w1=$SCHED_W1, w2=$SCHED_W2)"
fi

# =================================================================
# Phase 3: VPC Networking Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: VPC Networking Tests"
echo "========================================="

# Test 1: Verify containers get overlay IPs (10.0.x.x range)
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

# Test 2: Cross-host container connectivity
# Wait for VPC peer convergence: agent health reports (10s) + heartbeat peer exchange (15s)
log_info "Test: Cross-host container connectivity via overlay"
if [ -n "$WORKER1_IP" ] && [ -n "$WORKER2_IP" ] && [ -n "$WORKER1_CONTAINER" ] && [ -n "$WORKER2_CONTAINER" ]; then
    # Wait for cross-host connectivity with retries (peers converge via heartbeat)
    PING_OK_1TO2=false
    PING_OK_2TO1=false
    for attempt in $(seq 1 10); do
        if [ "$PING_OK_1TO2" = false ]; then
            if docker exec banyan-worker-1 nerdctl exec "$WORKER1_CONTAINER" ping -c 1 -W 3 "$WORKER2_IP" >/dev/null 2>&1; then
                PING_OK_1TO2=true
            fi
        fi
        if [ "$PING_OK_2TO1" = false ]; then
            if docker exec banyan-worker-2 nerdctl exec "$WORKER2_CONTAINER" ping -c 1 -W 3 "$WORKER1_IP" >/dev/null 2>&1; then
                PING_OK_2TO1=true
            fi
        fi
        if [ "$PING_OK_1TO2" = true ] && [ "$PING_OK_2TO1" = true ]; then
            break
        fi
        sleep 3
    done

    if [ "$PING_OK_1TO2" = true ]; then
        log_test_pass "Cross-host ping: $WORKER1_CONTAINER ($WORKER1_IP) -> $WORKER2_IP"
    else
        log_test_fail "Cross-host ping failed: $WORKER1_CONTAINER ($WORKER1_IP) -> $WORKER2_IP"
        docker exec banyan-worker-1 nerdctl exec "$WORKER1_CONTAINER" ping -c 3 -W 5 "$WORKER2_IP" 2>&1 || true
    fi

    if [ "$PING_OK_2TO1" = true ]; then
        log_test_pass "Cross-host ping: $WORKER2_CONTAINER ($WORKER2_IP) -> $WORKER1_IP"
    else
        log_test_fail "Cross-host ping failed: $WORKER2_CONTAINER ($WORKER2_IP) -> $WORKER1_IP"
        docker exec banyan-worker-2 nerdctl exec "$WORKER2_CONTAINER" ping -c 3 -W 5 "$WORKER1_IP" 2>&1 || true
    fi
else
    log_warn "Skipping cross-host ping (need at least one container with overlay IP on each worker)"
fi

# Test 3: Unique overlay IPs across replicas
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

# Test 4: Service proxy via iptables DNAT
log_info "Test: Service proxy via iptables DNAT"

# Debug: IP forwarding status
for worker in banyan-worker-1 banyan-worker-2; do
    echo "  $worker ip_forward: $(docker exec "$worker" cat /proc/sys/net/ipv4/ip_forward 2>&1)"
done

# Debug: Show iptables state on both workers
log_info "Debug: iptables rules on workers"
for worker in banyan-worker-1 banyan-worker-2; do
    echo "  $worker NAT chains:"
    docker exec "$worker" iptables -t nat -L BANYAN-P-SERVICES -n 2>/dev/null || echo "    (no BANYAN-P-SERVICES chain)"
    docker exec "$worker" iptables -t nat -L BANYAN-P-SVC-3000 -n 2>/dev/null || echo "    (no BANYAN-P-SVC-3000 chain)"
    echo "  $worker FORWARD chain (first 10 rules):"
    docker exec "$worker" iptables -L FORWARD -n 2>/dev/null | head -12 || true
done

# Show overlay routes on each worker (for debugging cross-host issues)
for worker in banyan-worker-1 banyan-worker-2; do
    echo "  $worker overlay routes:"
    docker exec "$worker" ip route show 2>&1 | grep -E "^10\." || echo "    (no overlay routes)"
done

test_proxy banyan-engine 172.28.0.11 3000 "Proxy: engine -> worker-1:3000"
test_proxy banyan-engine 172.28.0.12 3000 "Proxy: engine -> worker-2:3000"
test_proxy banyan-worker-1 172.28.0.12 3000 "Cross-host proxy: worker-1 -> worker-2:3000"
test_proxy banyan-worker-2 172.28.0.11 3000 "Cross-host proxy: worker-2 -> worker-1:3000"

# =================================================================
# Phase 3b: DNS Resolution Tests
# =================================================================
echo ""
echo "========================================="
echo "Phase 3b: DNS Resolution Tests"
echo "========================================="

# Wait for DNS server to be ready and heartbeat to propagate backends
log_info "Waiting for DNS records to propagate via heartbeat..."
sleep 20

# Find containers by service name
API_WORKER=""
API_CONTAINER=""
DB_WORKER=""
DB_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    CONTAINERS=$(get_container_names "$worker")
    for container in $CONTAINERS; do
        if [[ "$container" == *"-api-"* ]]; then
            API_WORKER="$worker"
            API_CONTAINER="$container"
        fi
        if [[ "$container" == *"-db-"* ]]; then
            DB_WORKER="$worker"
            DB_CONTAINER="$container"
        fi
    done
done

if [ -n "$API_CONTAINER" ] && [ -n "$DB_CONTAINER" ]; then
    # Debug: Verify Redis binding and connectivity
    DB_IP=$(get_container_ip "$DB_WORKER" "$DB_CONTAINER")
    log_info "Debug: Redis container inspection"
    echo "  Redis container: $DB_CONTAINER on $DB_WORKER (IP: $DB_IP)"
    echo "  Redis process:"
    docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" cat /proc/1/cmdline 2>&1 | tr '\0' ' ' || true
    echo ""
    echo "  Redis localhost ping:"
    docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" redis-cli -h 127.0.0.1 ping 2>&1 || true
    echo "  Redis overlay IP ping ($DB_IP):"
    docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" redis-cli -h "$DB_IP" ping 2>&1 || true
    # Same-host test: API on same worker as Redis → Redis overlay IP
    SAME_HOST_API=""
    for container in $(get_container_names "$DB_WORKER"); do
        if [[ "$container" == *"-api-"* ]]; then
            SAME_HOST_API="$container"
            break
        fi
    done
    if [ -n "$SAME_HOST_API" ]; then
        echo "  Same-host Redis TCP test: $SAME_HOST_API → $DB_IP:6379"
        docker exec "$DB_WORKER" nerdctl exec "$SAME_HOST_API" node -e "
          const net = require('net');
          const s = net.createConnection(6379, '$DB_IP');
          s.setTimeout(3000);
          s.on('connect', () => { console.log('CONNECTED'); s.write('PING\r\n'); });
          s.on('data', d => { console.log('RESPONSE: ' + d.toString().trim()); s.destroy(); process.exit(0); });
          s.on('error', e => { console.log('ERROR: ' + e.message); process.exit(1); });
          s.on('timeout', () => { console.log('TIMEOUT'); s.destroy(); process.exit(1); });
        " 2>&1 || true
    fi

    # Cross-host Redis TCP test: API on different worker → Redis overlay IP
    if [ "$API_WORKER" != "$DB_WORKER" ]; then
        echo "  Cross-host TCP: $API_CONTAINER ($API_WORKER) → Redis $DB_IP:6379 ($DB_WORKER)"
        CROSS_HOST_RESULT=$(docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" node -e "
          const net = require('net');
          const s = net.createConnection(6379, '$DB_IP');
          s.setTimeout(5000);
          s.on('connect', () => { console.log('CONNECTED'); s.write('PING\r\n'); });
          s.on('data', d => { console.log(d.toString().trim()); s.destroy(); process.exit(0); });
          s.on('error', e => { console.log('ERROR: ' + e.message); process.exit(1); });
          s.on('timeout', () => { console.log('TIMEOUT'); s.destroy(); process.exit(1); });
        " 2>&1) || true
        if echo "$CROSS_HOST_RESULT" | grep -q "PONG"; then
            log_test_pass "Cross-host TCP: Redis PONG via overlay ($API_WORKER → $DB_WORKER)"
        else
            log_test_fail "Cross-host TCP: Redis connection failed ($CROSS_HOST_RESULT)"
            echo "  Debug: WireGuard peer status on $API_WORKER:"
            docker exec "$API_WORKER" wg show banyan-wg 2>&1 || true
            echo "  Debug: Overlay routes on $API_WORKER:"
            docker exec "$API_WORKER" ip route show 2>&1 | grep -E "^10\." || true
        fi
    fi

    # Test 1: DNS resolution from API container → db.e2e-test-app.internal
    log_info "Test: DNS resolution db.e2e-test-app.internal from API container"
    if docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 db.e2e-test-app.internal 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "DNS: API container can ping db.e2e-test-app.internal"
    else
        log_test_fail "DNS: API container cannot ping db.e2e-test-app.internal"
        docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 db.e2e-test-app.internal 2>&1 || true
    fi

    # Test 2: DNS resolution from DB container → api.e2e-test-app.internal
    log_info "Test: DNS resolution api.e2e-test-app.internal from DB container"
    if docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" ping -c 2 -W 5 api.e2e-test-app.internal 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "DNS: DB container can ping api.e2e-test-app.internal"
    else
        log_test_fail "DNS: DB container cannot ping api.e2e-test-app.internal"
        docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" ping -c 2 -W 5 api.e2e-test-app.internal 2>&1 || true
    fi

    # Test 3: dns-search domain — plain 'db' should resolve via search domain 'internal'
    # Note: Alpine/musl libc may not support search domains correctly in all environments
    log_info "Test: DNS search domain (plain 'db' from API container)"
    echo "  Debug: container resolv.conf:"
    docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" cat /etc/resolv.conf 2>&1 || true
    if docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 db 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "DNS search: API container can ping 'db' (via dns-search internal)"
    else
        log_warn "DNS search: plain 'db' not resolvable (dns-search may not work with musl libc). Use 'db.e2e-test-app.internal' instead."
        docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 db 2>&1 || true
    fi

    # Test 4: Application-level DNS — API server connects to Redis via DNS name
    log_info "Test: API → Redis connection via DNS (GET /db)"
    API_IP=$(get_container_ip "$API_WORKER" "$API_CONTAINER")
    if [ -n "$API_IP" ]; then
        # Use separate capture to avoid losing response body on non-200 status
        DB_RESPONSE=""
        DB_RESPONSE=$(docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" wget -qO- "http://127.0.0.1:3000/db" 2>&1) || true
        if echo "$DB_RESPONSE" | grep -q "PONG"; then
            log_test_pass "App-level DNS: API /db returned '$DB_RESPONSE' (Redis connected via DNS)"
        else
            log_test_fail "App-level DNS: API /db returned '$DB_RESPONSE' (expected PONG)"
            # Debug: check DNS resolution and Redis connectivity directly
            echo "  Debug: DNS resolution of db.e2e-test-app.internal from API container:"
            docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" nslookup db.e2e-test-app.internal 2>&1 | head -10 || true
            echo "  Debug: Full error from API /db endpoint (via Node.js):"
            docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" node -e "
              const http = require('http');
              http.get('http://127.0.0.1:3000/db', res => {
                let body = '';
                res.on('data', d => body += d);
                res.on('end', () => console.log('HTTP ' + res.statusCode + ': ' + body));
              }).on('error', e => console.log('REQUEST ERROR: ' + e.message));
            " 2>&1 || true
            echo "  Debug: Direct Redis PING test from API container (via Node.js):"
            DB_IP=$(docker exec "$DB_WORKER" nerdctl inspect "$DB_CONTAINER" 2>/dev/null \
                | grep -oP '"IPAddress":\s*"\K[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | head -1)
            if [ -n "$DB_IP" ]; then
                echo "  Redis container IP: $DB_IP"
                echo "  Debug: Overlay routes on $API_WORKER:"
                docker exec "$API_WORKER" ip route show 2>&1 | grep -E "^10\." || true
                docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" node -e "
                  const net = require('net');
                  console.log('Connecting to $DB_IP:6379...');
                  const s = net.createConnection(6379, '$DB_IP');
                  s.setTimeout(5000);
                  s.on('connect', () => { console.log('Connected! Sending PING'); s.write('PING\r\n'); });
                  s.on('data', d => { console.log('Redis response: ' + d.toString().trim()); s.destroy(); process.exit(0); });
                  s.on('error', e => { console.log('Connection error: ' + e.message); process.exit(1); });
                  s.on('timeout', () => { console.log('Connection timeout after 5s'); s.destroy(); process.exit(1); });
                " 2>&1 || true
            fi
        fi
    else
        log_warn "  Could not get API container IP, skipping app-level DNS test"
    fi
else
    log_warn "Skipping DNS tests (need api and db containers)"
    [ -z "$API_CONTAINER" ] && log_warn "  No API container found"
    [ -z "$DB_CONTAINER" ] && log_warn "  No DB container found"
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

# Test: Cross-host connectivity after redeployment
log_info "Test: Cross-host connectivity after redeployment"
POST_W1_CONTAINER=""
POST_W1_IP=""
POST_W2_CONTAINER=""
POST_W2_IP=""
for container in $(get_container_names banyan-worker-1); do
    IP=$(get_container_ip banyan-worker-1 "$container")
    if [[ "$IP" == 10.0.* ]]; then
        POST_W1_CONTAINER="$container"
        POST_W1_IP="$IP"
        break
    fi
done
for container in $(get_container_names banyan-worker-2); do
    IP=$(get_container_ip banyan-worker-2 "$container")
    if [[ "$IP" == 10.0.* ]]; then
        POST_W2_CONTAINER="$container"
        POST_W2_IP="$IP"
        break
    fi
done

if [ -n "$POST_W1_IP" ] && [ -n "$POST_W2_IP" ] && [ -n "$POST_W1_CONTAINER" ] && [ -n "$POST_W2_CONTAINER" ]; then
    # Wait for peer convergence after redeployment
    POST_PING_1TO2=false
    POST_PING_2TO1=false
    for attempt in $(seq 1 10); do
        if [ "$POST_PING_1TO2" = false ]; then
            if docker exec banyan-worker-1 nerdctl exec "$POST_W1_CONTAINER" ping -c 1 -W 3 "$POST_W2_IP" >/dev/null 2>&1; then
                POST_PING_1TO2=true
            fi
        fi
        if [ "$POST_PING_2TO1" = false ]; then
            if docker exec banyan-worker-2 nerdctl exec "$POST_W2_CONTAINER" ping -c 1 -W 3 "$POST_W1_IP" >/dev/null 2>&1; then
                POST_PING_2TO1=true
            fi
        fi
        if [ "$POST_PING_1TO2" = true ] && [ "$POST_PING_2TO1" = true ]; then
            break
        fi
        sleep 3
    done

    if [ "$POST_PING_1TO2" = true ]; then
        log_test_pass "Post-redeploy cross-host: $POST_W1_CONTAINER -> $POST_W2_IP"
    else
        log_test_fail "Post-redeploy cross-host: $POST_W1_CONTAINER -> $POST_W2_IP"
        docker exec banyan-worker-1 nerdctl exec "$POST_W1_CONTAINER" ping -c 3 -W 5 "$POST_W2_IP" 2>&1 || true
    fi

    if [ "$POST_PING_2TO1" = true ]; then
        log_test_pass "Post-redeploy cross-host: $POST_W2_CONTAINER -> $POST_W1_IP"
    else
        log_test_fail "Post-redeploy cross-host: $POST_W2_CONTAINER -> $POST_W1_IP"
        docker exec banyan-worker-2 nerdctl exec "$POST_W2_CONTAINER" ping -c 3 -W 5 "$POST_W1_IP" 2>&1 || true
    fi
else
    log_warn "Skipping post-redeploy cross-host test (need overlay IP containers on both workers)"
fi

# Test: Service proxy works after redeployment
log_info "Test: Service proxy after redeployment"
test_proxy banyan-engine 172.28.0.11 3000 "Post-redeploy proxy: engine -> worker-1:3000"
test_proxy banyan-engine 172.28.0.12 3000 "Post-redeploy proxy: engine -> worker-2:3000"

# Test: DNS still works after redeployment
log_info "Test: DNS resolution after redeployment"
POST_API_WORKER=""
POST_API_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    CONTAINERS=$(get_container_names "$worker")
    for container in $CONTAINERS; do
        if [[ "$container" == *"-api-"* ]]; then
            POST_API_WORKER="$worker"
            POST_API_CONTAINER="$container"
            break 2
        fi
    done
done

if [ -n "$POST_API_CONTAINER" ]; then
    # Wait for DNS to propagate for new containers
    sleep 15
    if docker exec "$POST_API_WORKER" nerdctl exec "$POST_API_CONTAINER" ping -c 2 -W 5 db.e2e-test-app.internal 2>/dev/null | grep -q "0% packet loss"; then
        log_test_pass "Post-redeploy DNS: API container can ping db.e2e-test-app.internal"
    else
        log_test_fail "Post-redeploy DNS: API container cannot ping db.e2e-test-app.internal"
        docker exec "$POST_API_WORKER" nerdctl exec "$POST_API_CONTAINER" ping -c 2 -W 5 db.e2e-test-app.internal 2>&1 || true
    fi
else
    log_warn "Skipping post-redeploy DNS test (no API container found)"
fi

# Show deployment status after redeployment
docker exec banyan-engine banyan-cli deployment

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
DOWN_STATUS=$(docker exec banyan-engine banyan-cli deployment 2>&1)
if echo "$DOWN_STATUS" | grep -qw "running"; then
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
