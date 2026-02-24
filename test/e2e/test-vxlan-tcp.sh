#!/bin/bash
# Focused test: Cross-host TCP through VXLAN overlay
# Tests ONLY the specific case of container-to-container TCP across workers.
# Much faster than the full E2E suite for iterating on VXLAN fixes.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$SCRIPT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_debug() { echo -e "${CYAN}[DEBUG]${NC} $1"; }

cleanup() {
    log_info "Cleaning up..."
    docker-compose down -v --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

get_container_ip() {
    local worker=$1 container=$2
    docker exec "$worker" nerdctl inspect "$container" 2>/dev/null \
        | grep -oP '"IPAddress":\s*"\K[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' \
        | head -1
}

get_container_names() {
    local worker=$1
    docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null | grep -v '^$'
}

echo "========================================="
echo "VXLAN Cross-Host TCP Test"
echo "========================================="

# --- Step 1: Build ---
log_info "Building binaries..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-engine" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-engine" .)
(cd "$REPO_ROOT/cmd/banyan-agent" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-agent" .)
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
md5sum "$SCRIPT_DIR/bin/banyan-engine" "$SCRIPT_DIR/bin/banyan-agent" "$SCRIPT_DIR/bin/banyan-cli" > "$SCRIPT_DIR/bin/.cache-bust"
log_info "Binaries built"

# --- Step 2: Start cluster ---
log_info "Building Docker image & starting cluster..."
docker-compose build --quiet
docker-compose up -d

# Wait for engine
log_info "Waiting for engine..."
for i in $(seq 1 20); do
    if docker exec banyan-engine banyan-cli status >/dev/null 2>&1; then break; fi
    sleep 3
done

# Wait for agents to register + VPC init
log_info "Waiting for agents (15s)..."
sleep 15

# --- Step 3: Pre-pull + Deploy ---
log_info "Pre-pulling images..."
for worker in banyan-worker-1 banyan-worker-2; do
    for img in redis:7-alpine node:22-alpine; do
        for attempt in 1 2 3; do
            if docker exec "$worker" nerdctl pull "$img" >/dev/null 2>&1; then break; fi
            sleep 3
        done
    done
done
log_info "Deploying app..."
docker exec banyan-engine banyan-cli up --file /examples/banyan.yaml

# Wait for containers
log_info "Waiting for containers..."
for i in $(seq 1 20); do
    W1=$(docker exec banyan-worker-1 nerdctl ps -q 2>/dev/null | wc -l)
    W2=$(docker exec banyan-worker-2 nerdctl ps -q 2>/dev/null | wc -l)
    if [ "$W1" -ge 1 ] && [ "$W2" -ge 1 ]; then break; fi
    sleep 3
done
sleep 5  # extra settle time

# --- Step 4: Identify containers ---
DB_WORKER="" DB_CONTAINER="" DB_IP=""
API_WORKER="" API_CONTAINER="" API_IP=""

for worker in banyan-worker-1 banyan-worker-2; do
    for container in $(get_container_names "$worker"); do
        if [[ "$container" == *"-db-"* ]]; then
            DB_WORKER="$worker"
            DB_CONTAINER="$container"
            DB_IP=$(get_container_ip "$worker" "$container")
        fi
        if [[ "$container" == *"-api-"* ]] && [ -z "$API_CONTAINER" ]; then
            API_WORKER="$worker"
            API_CONTAINER="$container"
            API_IP=$(get_container_ip "$worker" "$container")
        fi
    done
done

log_info "DB:  $DB_CONTAINER on $DB_WORKER (IP: $DB_IP)"
log_info "API: $API_CONTAINER on $API_WORKER (IP: $API_IP)"

if [ -z "$DB_CONTAINER" ] || [ -z "$API_CONTAINER" ]; then
    log_error "Could not find API and DB containers"
    docker exec banyan-worker-1 nerdctl ps 2>/dev/null || true
    docker exec banyan-worker-2 nerdctl ps 2>/dev/null || true
    exit 1
fi

# Wait for DNS propagation
log_info "Waiting for heartbeat/DNS propagation (20s)..."
sleep 20

# --- Step 5: Comprehensive diagnostics ---
echo ""
echo "========================================="
echo "Network Diagnostics"
echo "========================================="

for worker in banyan-worker-1 banyan-worker-2; do
    echo ""
    log_debug "=== $worker ==="
    echo "  ip_forward: $(docker exec "$worker" cat /proc/sys/net/ipv4/ip_forward 2>&1)"
    echo "  rp_filter (all): $(docker exec "$worker" cat /proc/sys/net/ipv4/conf/all/rp_filter 2>&1)"
    echo "  rp_filter (banyan0): $(docker exec "$worker" cat /proc/sys/net/ipv4/conf/banyan0/rp_filter 2>&1 || echo 'N/A')"
    echo "  rp_filter (banyan.1): $(docker exec "$worker" cat /proc/sys/net/ipv4/conf/banyan.1/rp_filter 2>&1 || echo 'N/A')"
    echo ""
    echo "  banyan0 MAC:"
    docker exec "$worker" ip link show banyan0 2>&1 | grep -E "link/ether" || echo "    N/A"
    echo "  banyan.1 MAC:"
    docker exec "$worker" ip link show banyan.1 2>&1 | grep -E "link/ether" || echo "    N/A"
    echo ""
    echo "  Bridge FDB (banyan.1):"
    docker exec "$worker" bridge fdb show dev banyan.1 2>&1 | head -5 || true
    echo "  ARP (banyan.1):"
    docker exec "$worker" ip neigh show dev banyan.1 2>&1 || true
    echo "  Routes (overlay):"
    docker exec "$worker" ip route show 2>&1 | grep -E "^10\." || echo "    none"
    echo "  iptables FORWARD policy:"
    docker exec "$worker" iptables -L FORWARD -n 2>&1 | head -3 || true
    echo "  TX checksum offload (banyan.1):"
    docker exec "$worker" ethtool -k banyan.1 2>&1 | grep -E "tx-checksum" || true
done

# --- Step 6: Tests ---
echo ""
echo "========================================="
echo "Cross-Host Connectivity Tests"
echo "========================================="

FAILED=0

# Test A: Host-to-host ICMP through VXLAN (baseline)
log_info "Test A: Host-to-host ICMP (worker → remote container IP)"
if docker exec "$API_WORKER" ping -c 2 -W 3 "$DB_IP" 2>/dev/null | grep -q "0% packet loss"; then
    echo -e "  ${GREEN}PASS${NC}: Host ping $API_WORKER → $DB_IP"
else
    echo -e "  ${RED}FAIL${NC}: Host ping $API_WORKER → $DB_IP"
    docker exec "$API_WORKER" ping -c 2 -W 3 "$DB_IP" 2>&1 || true
    FAILED=1
fi

# Test B: Container-to-container ICMP through VXLAN
log_info "Test B: Container-to-container ICMP"
if [ "$API_WORKER" != "$DB_WORKER" ]; then
    if docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 "$DB_IP" 2>/dev/null | grep -q "0% packet loss"; then
        echo -e "  ${GREEN}PASS${NC}: Container ICMP $API_CONTAINER → $DB_IP"
    else
        echo -e "  ${RED}FAIL${NC}: Container ICMP $API_CONTAINER → $DB_IP"
        docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" ping -c 2 -W 5 "$DB_IP" 2>&1 || true
        FAILED=1
    fi
else
    echo "  SKIP: API and DB on same worker"
fi

# Test C: Same-host TCP to Redis (verify Redis is working)
log_info "Test C: Same-host TCP to Redis"
docker exec "$DB_WORKER" nerdctl exec "$DB_CONTAINER" redis-cli -h "$DB_IP" ping 2>&1 || true

# Test D: Cross-host TCP to Redis (the actual problematic test)
log_info "Test D: Cross-host TCP to Redis"
if [ "$API_WORKER" != "$DB_WORKER" ]; then
    # Start tcpdump on multiple interfaces SIMULTANEOUSLY
    log_debug "Starting packet captures..."

    # Capture on receiver's banyan.1 (VXLAN interface)
    docker exec "$DB_WORKER" timeout 8 tcpdump -i banyan.1 -nn -c 30 "port 6379" 2>&1 > /tmp/tcpdump_recv_vxlan.log &
    PID_RECV_VXLAN=$!

    # Capture on receiver's banyan0 (bridge interface)
    docker exec "$DB_WORKER" timeout 8 tcpdump -i banyan0 -nn -c 30 "port 6379" 2>&1 > /tmp/tcpdump_recv_bridge.log &
    PID_RECV_BRIDGE=$!

    # Capture on sender's banyan.1
    docker exec "$API_WORKER" timeout 8 tcpdump -i banyan.1 -nn -c 30 "port 6379" 2>&1 > /tmp/tcpdump_send_vxlan.log &
    PID_SEND_VXLAN=$!

    sleep 1  # let tcpdump settle

    # The actual TCP connection attempt
    log_debug "Attempting cross-host TCP: $API_CONTAINER → $DB_IP:6379"
    docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" node -e "
      const net = require('net');
      const s = net.createConnection(6379, '$DB_IP');
      s.setTimeout(5000);
      s.on('connect', () => { console.log('CONNECTED'); s.write('PING\r\n'); });
      s.on('data', d => { console.log('RESPONSE: ' + d.toString().trim()); s.destroy(); process.exit(0); });
      s.on('error', e => { console.log('ERROR: ' + e.message); process.exit(1); });
      s.on('timeout', () => { console.log('TIMEOUT'); s.destroy(); process.exit(1); });
    " 2>&1
    TCP_EXIT=$?

    # Wait for tcpdump to finish
    wait $PID_RECV_VXLAN 2>/dev/null || true
    wait $PID_RECV_BRIDGE 2>/dev/null || true
    wait $PID_SEND_VXLAN 2>/dev/null || true

    if [ $TCP_EXIT -eq 0 ]; then
        echo -e "  ${GREEN}PASS${NC}: Cross-host TCP to Redis"
    else
        echo -e "  ${RED}FAIL${NC}: Cross-host TCP to Redis (exit code: $TCP_EXIT)"
        FAILED=1
    fi

    # Show packet capture results
    echo ""
    log_debug "Packet captures:"
    echo "  --- Sender $API_WORKER banyan.1 (VXLAN) ---"
    cat /tmp/tcpdump_send_vxlan.log 2>/dev/null || echo "    (empty)"
    echo "  --- Receiver $DB_WORKER banyan.1 (VXLAN) ---"
    cat /tmp/tcpdump_recv_vxlan.log 2>/dev/null || echo "    (empty)"
    echo "  --- Receiver $DB_WORKER banyan0 (BRIDGE) ---"
    cat /tmp/tcpdump_recv_bridge.log 2>/dev/null || echo "    (empty)"
else
    log_warn "API and DB on same worker — cross-host TCP test not applicable"
fi

# Test E: App-level DNS test (API /db endpoint → Redis via db.internal)
log_info "Test E: App-level API → Redis via DNS"
DB_RESPONSE=""
DB_RESPONSE=$(docker exec "$API_WORKER" nerdctl exec "$API_CONTAINER" wget -qO- "http://127.0.0.1:3000/db" 2>&1) || true
if echo "$DB_RESPONSE" | grep -q "PONG"; then
    echo -e "  ${GREEN}PASS${NC}: API /db returned '$DB_RESPONSE'"
else
    echo -e "  ${RED}FAIL${NC}: API /db returned '$DB_RESPONSE'"
    FAILED=1
fi

# --- Results ---
echo ""
echo "========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}ALL TESTS PASSED${NC}"
else
    echo -e "${RED}SOME TESTS FAILED${NC}"
fi
echo "========================================="
exit $FAILED
