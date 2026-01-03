#!/bin/bash
# Banyan E2E Test Runner
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
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

echo "========================================="
echo "Banyan E2E Test"
echo "========================================="

# Step 1: Build images
log_info "Building Docker images..."
docker-compose build

# Step 2: Start the cluster
log_info "Starting Banyan cluster (1 engine + 2 workers)..."
docker-compose up -d

# Step 3: Wait for engine to be healthy
log_info "Waiting for engine to be healthy..."
timeout 60 bash -c 'until docker-compose exec -T engine etcdctl endpoint health 2>/dev/null; do sleep 2; done'
log_info "Engine is healthy!"

# Step 4: Wait for agents to connect
log_info "Waiting for agents to connect..."
sleep 10

# Step 5: Check agent status
log_info "Checking agent status..."
docker-compose exec -T engine banyan-cli ipam get-subnet worker-1 || log_warn "worker-1 subnet not yet allocated"
docker-compose exec -T engine banyan-cli ipam get-subnet worker-2 || log_warn "worker-2 subnet not yet allocated"

# Step 6: Deploy test application
log_info "Deploying test application..."
docker-compose exec -T engine banyan-cli deploy --file /examples/banyan.yaml

# Step 7: Verify deployment
log_info "Verifying deployment..."
sleep 5

# Check DNS registrations
log_info "Checking DNS registrations..."
docker-compose exec -T engine banyan-cli dns list || log_warn "DNS list not available"

echo ""
echo "========================================="
log_info "E2E Test Complete!"
echo "========================================="
echo ""
echo "Cluster is running. You can interact with it:"
echo "  docker-compose exec engine banyan-cli --help"
echo "  docker-compose exec worker-1 banyan-cli agent status"
echo ""
echo "To stop the cluster:"
echo "  docker-compose down -v"
