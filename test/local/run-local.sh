#!/bin/bash
# Banyan Native Local Environment
# Runs engine + agent on localhost for E2E testing without Docker.
#
# Usage:
#   sudo ./test/local/run-local.sh          # interactive mode (keeps cluster running)
#   sudo ./test/local/run-local.sh --test   # auto-test mode (runs tests then exits)
#   sudo ./test/local/run-local.sh --clean  # force cleanup only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()  { echo -e "${BLUE}[STEP]${NC} $1"; }

# Config
CONFIG_DIR="/etc/banyan"
BACKUP_DIR=""
ADMIN_USER="admin"
ADMIN_PASS="banyan-local-admin"
ENGINE_PID=""
AGENT_PID=""
CONTAINERD_PID=""
TESTS_PASSED=0
TESTS_FAILED=0

# Modes
AUTO_TEST=false
CLEAN_ONLY=false

# Parse args
for arg in "$@"; do
    case "$arg" in
        --test) AUTO_TEST=true ;;
        --clean) CLEAN_ONLY=true ;;
    esac
done

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
cleanup() {
    log_step "Cleaning up local environment..."

    if [ -n "$ENGINE_PID" ] && kill -0 "$ENGINE_PID" 2>/dev/null; then
        log_info "Stopping engine (PID: $ENGINE_PID)..."
        kill -TERM "$ENGINE_PID" 2>/dev/null || true
        wait "$ENGINE_PID" 2>/dev/null || true
    fi

    if [ -n "$AGENT_PID" ] && kill -0 "$AGENT_PID" 2>/dev/null; then
        log_info "Stopping agent (PID: $AGENT_PID)..."
        kill -TERM "$AGENT_PID" 2>/dev/null || true
        wait "$AGENT_PID" 2>/dev/null || true
    fi

    if [ -n "$CONTAINERD_PID" ] && kill -0 "$CONTAINERD_PID" 2>/dev/null; then
        log_info "Stopping containerd (PID: $CONTAINERD_PID)..."
        kill -TERM "$CONTAINERD_PID" 2>/dev/null || true
        wait "$CONTAINERD_PID" 2>/dev/null || true
    fi

    # Remove WireGuard interfaces (best effort)
    for iface in wg-ctl-eng wg-ctl-agt wg-ctl-cli; do
        if ip link show "$iface" >/dev/null 2>&1; then
            ip link del "$iface" 2>/dev/null || true
        fi
    done

    # Restore config backup
    if [ -n "$BACKUP_DIR" ] && [ -d "$BACKUP_DIR" ]; then
        log_info "Restoring original config from $BACKUP_DIR..."
        rm -rf "$CONFIG_DIR"
        mv "$BACKUP_DIR" "$CONFIG_DIR"
    elif [ -d "$CONFIG_DIR" ]; then
        log_info "Removing local config..."
        rm -rf "$CONFIG_DIR"
    fi

    log_info "Cleanup complete."
}

if [ "$CLEAN_ONLY" = true ]; then
    cleanup
    exit 0
fi

trap cleanup EXIT

# ---------------------------------------------------------------------------
# Pre-flight checks
# ---------------------------------------------------------------------------
log_step "Pre-flight checks"

if [ "$EUID" -ne 0 ]; then
    log_error "This script must be run as root (sudo)"
    exit 1
fi

# Check dependencies
for cmd in go wg ip iptables nerdctl; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        log_warn "$cmd not found in PATH"
    fi
done

# Detect if banyan-cli has login command (M13 auth)
HAS_JWT_AUTH=false
if "$REPO_ROOT/bin/banyan-cli" login --help >/dev/null 2>&1; then
    HAS_JWT_AUTH=true
    log_info "M13 JWT auth detected"
else
    log_info "Pre-M13 mode (no JWT auth)"
fi

# ---------------------------------------------------------------------------
# Phase 1: Build binaries
# ---------------------------------------------------------------------------
log_step "Phase 1: Build binaries"

mkdir -p "$REPO_ROOT/bin"
log_info "Building banyan-engine..."
(cd "$REPO_ROOT/cmd/banyan-engine" && go build -o "$REPO_ROOT/bin/banyan-engine" .)
log_info "Building banyan-agent..."
(cd "$REPO_ROOT/cmd/banyan-agent" && go build -o "$REPO_ROOT/bin/banyan-agent" .)
log_info "Building banyan-cli..."
(cd "$REPO_ROOT/cmd/banyan-cli" && go build -o "$REPO_ROOT/bin/banyan-cli" .)

export PATH="$REPO_ROOT/bin:$PATH"

# ---------------------------------------------------------------------------
# Phase 2: Isolate config
# ---------------------------------------------------------------------------
log_step "Phase 2: Isolate config"

if [ -d "$CONFIG_DIR" ]; then
    BACKUP_DIR="${CONFIG_DIR}.local-backup.$(date +%s)"
    log_info "Backing up existing config to $BACKUP_DIR..."
    cp -a "$CONFIG_DIR" "$BACKUP_DIR"
fi

rm -rf "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR" "$CONFIG_DIR/keys" "$CONFIG_DIR/whitelisted-keys"
chmod 700 "$CONFIG_DIR" "$CONFIG_DIR/keys" "$CONFIG_DIR/whitelisted-keys"

# ---------------------------------------------------------------------------
# Phase 3: Engine init
# ---------------------------------------------------------------------------
log_step "Phase 3: Initialize engine"

# Write minimal base config
cat > "$CONFIG_DIR/banyan.yaml" <<'EOF'
engine:
    grpc_port: "50051"
    store_backend: "etcd"
    managed_etcd: true
    managed_registry: true
EOF
chmod 600 "$CONFIG_DIR/banyan.yaml"

# Engine init (non-interactive)
if [ "$HAS_JWT_AUTH" = true ]; then
    log_info "Running engine init with auth bootstrap..."
    banyan-engine init \
        --non-interactive \
        --admin-user "$ADMIN_USER" \
        --admin-password "$ADMIN_PASS"
else
    log_info "Running engine init (pre-auth mode)..."
    banyan-engine init
fi

# Extract engine public key
ENGINE_PUB_KEY=$(grep 'wg_public_key' "$CONFIG_DIR/banyan.yaml" | head -1 | awk '{print $2}')
if [ -z "$ENGINE_PUB_KEY" ]; then
    log_error "Failed to extract engine public key from config"
    exit 1
fi
log_info "Engine public key: $ENGINE_PUB_KEY"

# Compute engine tunnel IP (SHA-256 of pubkey → 10.200.x.y)
ENGINE_HASH=$(echo -n "$ENGINE_PUB_KEY" | sha256sum | head -c 4)
ENGINE_O3=$((16#${ENGINE_HASH:0:2}))
ENGINE_O4=$((16#${ENGINE_HASH:2:2}))
if [ "$ENGINE_O3" -eq 0 ] && [ "$ENGINE_O4" -le 1 ]; then
    ENGINE_O4=$((ENGINE_O4 + 2))
fi
ENGINE_TUNNEL_IP="10.200.${ENGINE_O3}.${ENGINE_O4}"
log_info "Engine tunnel IP: $ENGINE_TUNNEL_IP"

# ---------------------------------------------------------------------------
# Phase 4: Agent init
# ---------------------------------------------------------------------------
log_step "Phase 4: Initialize agent"

banyan-agent init \
    --non-interactive \
    --engine-host localhost \
    --engine-port 50051 \
    --agent-name local-agent \
    --engine-wg-pubkey "$ENGINE_PUB_KEY"

# Extract agent public key
AGENT_PUB_KEY=$(awk '/agent:/{found=1} found && /wg_public_key/{print $2; exit}' "$CONFIG_DIR/banyan.yaml")
if [ -z "$AGENT_PUB_KEY" ]; then
    log_error "Failed to extract agent public key"
    exit 1
fi
log_info "Agent public key: $AGENT_PUB_KEY"

# Whitelist agent
banyan-engine add-client --name local-agent --pubkey "$AGENT_PUB_KEY"
log_info "Agent whitelisted"

# ---------------------------------------------------------------------------
# Phase 5: CLI init
# ---------------------------------------------------------------------------
log_step "Phase 5: Initialize CLI"

banyan-cli init \
    --non-interactive \
    --engine-host localhost \
    --engine-port 50051 \
    --cli-name local-cli \
    --engine-wg-pubkey "$ENGINE_PUB_KEY"

# Extract CLI public key
CLI_PUB_KEY=$(awk '/cli:/{found=1} found && /wg_public_key/{print $2; exit}' "$CONFIG_DIR/banyan.yaml")
if [ -z "$CLI_PUB_KEY" ]; then
    log_error "Failed to extract CLI public key"
    exit 1
fi
log_info "CLI public key: $CLI_PUB_KEY"

# Whitelist CLI
banyan-engine add-client --name local-cli --pubkey "$CLI_PUB_KEY"
log_info "CLI whitelisted"

# ---------------------------------------------------------------------------
# Phase 6: Start engine
# ---------------------------------------------------------------------------
log_step "Phase 6: Start engine"

banyan-engine start &
ENGINE_PID=$!
log_info "Engine started (PID: $ENGINE_PID)"

# Wait for engine gRPC to be ready
log_info "Waiting for engine gRPC to be ready..."
for i in $(seq 1 60); do
    if nc -z "$ENGINE_TUNNEL_IP" 50051 2>/dev/null; then
        log_info "Engine gRPC is ready on $ENGINE_TUNNEL_IP:50051"
        break
    fi
    if ! kill -0 "$ENGINE_PID" 2>/dev/null; then
        log_error "Engine process died unexpectedly"
        exit 1
    fi
    sleep 1
done

# ---------------------------------------------------------------------------
# Phase 7: Setup CLI WireGuard tunnel
# ---------------------------------------------------------------------------
log_step "Phase 7: Setup CLI WireGuard tunnel"

# Compute CLI tunnel IP
CLI_HASH=$(echo -n "$CLI_PUB_KEY" | sha256sum | head -c 4)
CLI_O3=$((16#${CLI_HASH:0:2}))
CLI_O4=$((16#${CLI_HASH:2:2}))
if [ "$CLI_O3" -eq 0 ] && [ "$CLI_O4" -le 1 ]; then
    CLI_O4=$((CLI_O4 + 2))
fi
CLI_TUNNEL_IP="10.200.${CLI_O3}.${CLI_O4}"
log_info "CLI tunnel IP: $CLI_TUNNEL_IP"

# Create WG interface
CLI_KEY_FILE=$(awk '/cli:/{found=1} found && /wg_private_key_file/{print $2; exit}' "$CONFIG_DIR/banyan.yaml")

ip link add wg-ctl-cli type wireguard 2>/dev/null || true
wg set wg-ctl-cli private-key "$CLI_KEY_FILE"
ip addr add "${CLI_TUNNEL_IP}/16" dev wg-ctl-cli
ip link set wg-ctl-cli up
wg set wg-ctl-cli peer "$ENGINE_PUB_KEY" allowed-ips "${ENGINE_TUNNEL_IP}/32" endpoint 127.0.0.1:51821
log_info "CLI tunnel ready"

# ---------------------------------------------------------------------------
# Phase 8: Start containerd (if not already running)
# ---------------------------------------------------------------------------
log_step "Phase 8: Start containerd"

if pgrep -x containerd >/dev/null 2>&1; then
    log_info "containerd already running, using existing instance"
else
    log_info "Starting containerd..."
    containerd &
    CONTAINERD_PID=$!
    sleep 3
    if ! kill -0 "$CONTAINERD_PID" 2>/dev/null; then
        log_error "containerd failed to start"
        exit 1
    fi
    log_info "containerd started (PID: $CONTAINERD_PID)"
fi

# ---------------------------------------------------------------------------
# Phase 9: Start agent
# ---------------------------------------------------------------------------
log_step "Phase 9: Start agent"

banyan-agent start --agent-name local-agent &
AGENT_PID=$!
log_info "Agent started (PID: $AGENT_PID)"

# Wait for agent to register
log_info "Waiting for agent to register..."
for i in $(seq 1 30); do
    if banyan-cli agent 2>/dev/null | grep -q "local-agent"; then
        log_info "Agent registered successfully"
        break
    fi
    if ! kill -0 "$AGENT_PID" 2>/dev/null; then
        log_error "Agent process died unexpectedly"
        exit 1
    fi
    sleep 1
done

# ---------------------------------------------------------------------------
# Phase 10: CLI login (M13 auth only)
# ---------------------------------------------------------------------------
log_step "Phase 10: Authenticate CLI"

if [ "$HAS_JWT_AUTH" = true ]; then
    log_info "Logging in CLI with JWT..."
    LOGIN_OK=false
    for i in $(seq 1 15); do
        if banyan-cli login --username "$ADMIN_USER" --password "$ADMIN_PASS" 2>/dev/null; then
            LOGIN_OK=true
            log_info "CLI authenticated (JWT session created)"
            break
        fi
        log_warn "Login attempt $i failed, retrying..."
        sleep 2
    done
    if [ "$LOGIN_OK" = false ]; then
        log_error "CLI login failed after 15 attempts"
        exit 1
    fi
else
    log_info "Pre-M13 mode: skipping JWT login"
fi

# ---------------------------------------------------------------------------
# Phase 11: Health check
# ---------------------------------------------------------------------------
log_step "Phase 11: Health check"

log_info "Engine status:"
banyan-cli engine || log_warn "Engine status check failed"

log_info "Agent status:"
banyan-cli agent || log_warn "Agent status check failed"

log_info "Deployments:"
banyan-cli deployment || true

# ---------------------------------------------------------------------------
# Phase 12: Run tests or enter interactive mode
# ---------------------------------------------------------------------------
log_step "Phase 12: Run tests"

if [ "$AUTO_TEST" = true ]; then
    log_info "Running automated tests..."
    bash "$SCRIPT_DIR/run-local-tests.sh"
    log_info "Tests complete. Exiting."
    exit 0
else
    echo ""
    echo "========================================"
    echo "  Local environment is READY"
    echo "========================================"
    echo ""
    echo "Engine:    $ENGINE_TUNNEL_IP:50051"
    echo "Agent:     local-agent (PID: $AGENT_PID)"
    echo "Config:    $CONFIG_DIR"
    echo ""
    echo "Available commands:"
    echo "  banyan-cli up --file <manifest>"
    echo "  banyan-cli deployment"
    echo "  banyan-cli down --file <manifest>"
    echo "  banyan-cli agent"
    echo "  banyan-cli engine"
    echo ""
    echo "Press Ctrl+C to stop and cleanup."
    echo ""

    # Keep running until interrupted
    wait "$ENGINE_PID"
fi
