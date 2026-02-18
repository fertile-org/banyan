#!/bin/bash
# Banyan E2E Test Runner
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    docker-compose down -v --remove-orphans 2>/dev/null || true
}

# Trap for cleanup on exit
trap cleanup EXIT

# Wait for a container's health check to pass
wait_for_healthy() {
    local container=$1
    local max_wait=$2
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if docker exec "$container" curl -sf http://localhost:2379/health >/dev/null 2>&1; then
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

echo "========================================="
echo "Banyan E2E Test"
echo "========================================="

# Step 1: Build banyan-cli binary locally (avoids Docker DNS issues with Go proxy)
log_info "Building banyan-cli binary..."
mkdir -p "$SCRIPT_DIR/bin"
(cd "$REPO_ROOT/cmd/banyan-cli" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$SCRIPT_DIR/bin/banyan-cli" .)
log_info "Binary built at test/e2e/bin/banyan-cli"

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

# Step 5: Wait for agents to connect
log_info "Waiting for agents to register..."
sleep 10

# Step 6: Check engine status (shows agents and deployments)
log_info "Checking engine status..."
docker exec banyan-engine banyan-cli engine status || log_warn "Engine status check failed"

# Step 7: Deploy test application
log_info "Deploying test application..."
docker exec banyan-engine banyan-cli deploy --file /examples/banyan.yaml --etcd http://localhost:2379

# Step 8: Verify deployment
log_info "Verifying deployment status..."
sleep 5
docker exec banyan-engine banyan-cli engine status

# Step 9: Verify containers on workers
log_info "Checking containers on workers..."
echo "  worker-1:"
docker exec banyan-worker-1 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-1"
echo "  worker-2:"
docker exec banyan-worker-2 nerdctl ps 2>/dev/null || log_warn "  Could not list containers on worker-2"

echo ""
echo "========================================="
log_info "E2E Test Complete!"
echo "========================================="
echo ""
echo "Cluster is running. You can interact with it:"
echo "  docker exec banyan-engine banyan-cli engine status"
echo "  docker exec banyan-engine banyan-cli deploy --file /examples/banyan.yaml --etcd http://localhost:2379"
echo "  docker exec banyan-worker-1 nerdctl ps"
echo ""
echo "To stop the cluster:"
echo "  cd test/e2e && docker-compose down -v"
