#!/bin/bash
# Banyan Secrets Management E2E Test
# Tests: secret create/list/get/delete, deploy with secrets, container env injection
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

echo "========================================="
echo "Banyan Secrets E2E Test Suite"
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
md5sum "$SCRIPT_DIR/bin/"banyan-* "$SCRIPT_DIR/scripts/"*-entrypoint.sh > "$SCRIPT_DIR/bin/.cache-bust" 2>/dev/null
log_info "Binaries built"

log_info "Building and starting cluster..."
docker-compose build 2>&1 | tail -3
docker-compose up -d 2>&1 | tail -5

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
# Phase 2: Create Secrets
# =================================================================
echo ""
echo "========================================="
echo "Phase 2: Create Secrets"
echo "========================================="

log_info "Creating DB_PASSWORD secret..."
CREATE_OUT=$(docker exec banyan-engine banyan-cli secret create DB_PASSWORD --value "test-password-123" 2>&1)
if echo "$CREATE_OUT" | grep -q 'created'; then
    log_test_pass "Secret DB_PASSWORD created"
else
    log_test_fail "Failed to create DB_PASSWORD: $CREATE_OUT"
fi

log_info "Creating API_KEY secret..."
CREATE_OUT=$(docker exec banyan-engine banyan-cli secret create API_KEY --value "key-abc-456" 2>&1)
if echo "$CREATE_OUT" | grep -q 'created'; then
    log_test_pass "Secret API_KEY created"
else
    log_test_fail "Failed to create API_KEY: $CREATE_OUT"
fi

# =================================================================
# Phase 3: List Secrets
# =================================================================
echo ""
echo "========================================="
echo "Phase 3: List and Get Secrets"
echo "========================================="

log_info "Listing secrets..."
LIST_OUT=$(docker exec banyan-engine banyan-cli secret list 2>&1)
echo "$LIST_OUT"
if echo "$LIST_OUT" | grep -q "DB_PASSWORD" && echo "$LIST_OUT" | grep -q "API_KEY"; then
    log_test_pass "Secret list shows both secrets"
else
    log_test_fail "Secret list missing expected secrets"
fi

log_info "Getting DB_PASSWORD metadata..."
GET_OUT=$(docker exec banyan-engine banyan-cli secret get DB_PASSWORD 2>&1)
echo "$GET_OUT"
if echo "$GET_OUT" | grep -q "DB_PASSWORD" && ! echo "$GET_OUT" | grep -q "test-password"; then
    log_test_pass "Secret get shows metadata without value"
else
    log_test_fail "Secret get unexpected output"
fi

log_info "Getting DB_PASSWORD with --reveal..."
REVEAL_OUT=$(docker exec banyan-engine banyan-cli secret get DB_PASSWORD --reveal 2>&1)
if echo "$REVEAL_OUT" | grep -q "test-password-123"; then
    log_test_pass "Secret get --reveal shows plaintext value"
else
    log_test_fail "Secret get --reveal did not show value: $REVEAL_OUT"
fi

# =================================================================
# Phase 4: Deploy with Secrets
# =================================================================
echo ""
echo "========================================="
echo "Phase 4: Deploy with Secrets"
echo "========================================="

log_info "Deploying app with secret references..."
docker exec banyan-engine banyan-cli up --file /examples/banyan-secrets.yaml 2>&1 || true
sleep 15

# Find which worker has the container
APP_WORKER=""
APP_CONTAINER=""
for worker in banyan-worker-1 banyan-worker-2; do
    name=$(docker exec "$worker" nerdctl ps --format '{{.Names}}' 2>/dev/null | grep "e2e-secrets-test-app" | head -1)
    if [ -n "$name" ]; then
        APP_WORKER="$worker"
        APP_CONTAINER="$name"
        break
    fi
done

if [ -z "$APP_WORKER" ]; then
    log_test_fail "Could not find app container"
else
    log_test_pass "App container running: $APP_CONTAINER on $APP_WORKER"

    # Verify secrets injected as env vars
    log_info "Checking DB_PASSWORD env var in container..."
    DB_VAL=$(docker exec "$APP_WORKER" nerdctl exec "$APP_CONTAINER" printenv DB_PASSWORD 2>/dev/null || true)
    if [ "$DB_VAL" = "test-password-123" ]; then
        log_test_pass "DB_PASSWORD injected correctly: $DB_VAL"
    else
        log_test_fail "DB_PASSWORD mismatch: got '$DB_VAL', expected 'test-password-123'"
    fi

    log_info "Checking API_KEY env var in container..."
    API_VAL=$(docker exec "$APP_WORKER" nerdctl exec "$APP_CONTAINER" printenv API_KEY 2>/dev/null || true)
    if [ "$API_VAL" = "key-abc-456" ]; then
        log_test_pass "API_KEY injected correctly: $API_VAL"
    else
        log_test_fail "API_KEY mismatch: got '$API_VAL', expected 'key-abc-456'"
    fi

    # Verify regular env var also present
    log_info "Checking APP_NAME env var (regular environment)..."
    APP_VAL=$(docker exec "$APP_WORKER" nerdctl exec "$APP_CONTAINER" printenv APP_NAME 2>/dev/null || true)
    if [ "$APP_VAL" = "secrets-test" ]; then
        log_test_pass "Regular env var APP_NAME present: $APP_VAL"
    else
        log_test_fail "Regular env var APP_NAME mismatch: got '$APP_VAL', expected 'secrets-test'"
    fi
fi

# =================================================================
# Phase 5: Delete In-Use Secret (should fail)
# =================================================================
echo ""
echo "========================================="
echo "Phase 5: Delete In-Use Secret"
echo "========================================="

log_info "Trying to delete DB_PASSWORD while deployment is running..."
DEL_OUT=$(docker exec banyan-engine banyan-cli secret delete DB_PASSWORD 2>&1 || true)
if echo "$DEL_OUT" | grep -qi "cannot delete\|referenced"; then
    log_test_pass "Delete blocked: in-use secret protected"
else
    log_test_fail "Delete should have been blocked: $DEL_OUT"
fi

# =================================================================
# Phase 6: Tear Down and Delete
# =================================================================
echo ""
echo "========================================="
echo "Phase 6: Tear Down and Delete"
echo "========================================="

log_info "Tearing down deployment..."
docker exec banyan-engine banyan-cli down --file /examples/banyan-secrets.yaml 2>/dev/null || true
sleep 15

log_info "Deleting DB_PASSWORD after teardown..."
DEL_OUT=$(docker exec banyan-engine banyan-cli secret delete DB_PASSWORD 2>&1)
if echo "$DEL_OUT" | grep -q "deleted"; then
    log_test_pass "Secret DB_PASSWORD deleted after teardown"
else
    log_test_fail "Failed to delete DB_PASSWORD: $DEL_OUT"
fi

log_info "Deleting API_KEY..."
DEL_OUT=$(docker exec banyan-engine banyan-cli secret delete API_KEY 2>&1)
if echo "$DEL_OUT" | grep -q "deleted"; then
    log_test_pass "Secret API_KEY deleted"
else
    log_test_fail "Failed to delete API_KEY: $DEL_OUT"
fi

# Verify list is empty
LIST_OUT=$(docker exec banyan-engine banyan-cli secret list 2>&1)
if echo "$LIST_OUT" | grep -q "No secrets found"; then
    log_test_pass "Secret list is empty after deletion"
else
    log_test_fail "Secret list not empty: $LIST_OUT"
fi

# =================================================================
# Phase 7: Deploy with Missing Secret (should fail)
# =================================================================
echo ""
echo "========================================="
echo "Phase 7: Deploy with Missing Secret"
echo "========================================="

log_info "Deploying with missing secret reference..."
DEPLOY_OUT=$(docker exec banyan-engine banyan-cli up --file /examples/banyan-secrets.yaml 2>&1 || true)
if echo "$DEPLOY_OUT" | grep -qi "does not exist\|not found\|secret.*create"; then
    log_test_pass "Deploy blocked: missing secret detected"
else
    log_test_fail "Deploy should have failed for missing secret: $DEPLOY_OUT"
fi

# =================================================================
# Results
# =================================================================
echo ""
echo "========================================="
echo "Secrets E2E Test Results"
echo "========================================="
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================="

if [ "$TESTS_FAILED" -gt 0 ]; then
    echo -e "${RED}Some tests failed!${NC}"
    exit 1
fi

log_info "All secrets E2E tests passed!"
