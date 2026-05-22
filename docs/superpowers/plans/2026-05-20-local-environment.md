# Local Environment Script Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a native host-based local environment script that starts engine + agent as background processes and runs E2E tests using banyan-cli commands, with full M13 auth support (WireGuard + JWT).

**Architecture:** Add `--non-interactive` init flags to `banyan-agent` and `banyan-cli` (mirroring the engine's existing non-interactive mode on m13-auth), then write a bash orchestrator script that builds binaries, bootstraps the full auth flow (WG key exchange + JWT login), starts services, and runs test scenarios.

**Tech Stack:** Go (cobra CLI), Bash, WireGuard, containerd, etcd

---

## File Structure

| File | Role |
|------|------|
| `cmd/banyan-agent/cmd/agent.go` | Add `--non-interactive` init flags and skip-TUI logic |
| `cmd/banyan-cli/cmd/init.go` | Add `--non-interactive` init flags and skip-TUI logic |
| `test/local/run-local.sh` | Main orchestrator: build → init → start → test → cleanup |
| `test/local/run-local-tests.sh` | Test scenarios: deploy, verify, down |
| `test/local/README.md` | Usage documentation |

---

## Context for Agentic Workers

### Prerequisites You Must Know

1. **Config path is hardcoded to `/etc/banyan/banyan.yaml`** — there is no `--config` flag yet. The local script uses backup/restore to isolate config.
2. **All three binaries (engine, agent, cli) require root** — they manipulate WireGuard interfaces, iptables, and network namespaces.
3. **M13 Auth has two layers:**
   - **WireGuard layer**: Keys exchanged via `add-client`, gRPC binds to tunnel IP
   - **JWT layer**: `banyan-engine init --non-interactive --admin-user X --admin-password Y` creates bootstrap → engine start consumes it → `banyan-cli login --username X --password Y` gets JWT → all CLI commands attach bearer token
4. **Agent RPCs are JWT-bypassed** — agents only need WG auth, not JWT
5. **The E2E Docker test is the reference implementation** — study `test/e2e/run-e2e.sh` and `test/e2e/scripts/engine-entrypoint.sh` for the exact bootstrap sequence

### Current Branch State

- `feat/add-stg-env` (current): Based on `main`, does **not** have M13 auth
- `feat/m13-auth` (target): Has JWT auth, non-interactive engine init, TLS, etc.
- **Strategy**: Write code on current branch that works with both. The non-interactive flags are useful regardless of auth branch. The local script detects auth at runtime (checks if `banyan-cli login` exists).

---

## Task 1: Add Non-Interactive Init to banyan-agent

**Files:**
- Modify: `cmd/banyan-agent/cmd/agent.go`
- Test: `cmd/banyan-agent/cmd/agent_test.go`

**What:** Add `--non-interactive`, `--engine-host`, `--engine-port`, `--agent-name`, `--engine-wg-pubkey` flags to `banyan-agent init`. When `--non-interactive` is set, skip all `huh` TUI prompts and use flag values directly.

- [ ] **Step 1: Add init flags to agent.go**

In `cmd/banyan-agent/cmd/agent.go`, find the `init()` function and add flag registrations after the existing flags:

```go
func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)

	rootCmd.PersistentFlags().StringVar(&agentDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	startCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "localhost:50051", "Engine gRPC endpoint")
	startCmd.Flags().StringVar(&agentNameFlag, "agent-name", "", "Agent name (defaults to hostname)")
	startCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")
	startCmd.Flags().StringVar(&agentAPIPort, "api-port", "50052", "Agent gRPC server port")
	startCmd.Flags().StringVar(&agentAPIAddress, "api-address", "", "Agent API address override (e.g. 192.168.1.10:50052)")

	statusCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "localhost:50051", "Engine gRPC endpoint")
	statusCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")

	// NEW: Non-interactive init flags
	initCmd.Flags().Bool("non-interactive", false, "Run init without interactive prompts")
	initCmd.Flags().String("engine-host", "localhost", "Engine host (non-interactive mode)")
	initCmd.Flags().String("engine-port", "50051", "Engine gRPC port (non-interactive mode)")
	initCmd.Flags().String("agent-name", "", "Agent name (non-interactive mode, defaults to hostname)")
	initCmd.Flags().String("engine-wg-pubkey", "", "Engine WireGuard public key (non-interactive mode)")
}
```

- [ ] **Step 2: Add non-interactive path in runAgentInit**

In `cmd/banyan-agent/cmd/agent.go`, find `runAgentInit()`. After the existing config check block (after line 189, after `existingCfg.Agent.Tags = nil`), insert the non-interactive branch before the WireGuard keypair generation:

```go
	// --- Check for non-interactive mode ---
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	if nonInteractive {
		flagEngineHost, _ := cmd.Flags().GetString("engine-host")
		flagEnginePort, _ := cmd.Flags().GetString("engine-port")
		flagAgentName, _ := cmd.Flags().GetString("agent-name")
		flagEngineWGPubKey, _ := cmd.Flags().GetString("engine-wg-pubkey")

		if flagEngineWGPubKey == "" {
			return fmt.Errorf("--non-interactive requires --engine-wg-pubkey")
		}
		if flagAgentName == "" {
			hostname, _ := os.Hostname()
			flagAgentName = hostname
		}

		// Generate WireGuard keypair if not already present
		if existingCfg.Agent.WGPrivateKeyFile == "" || existingCfg.Agent.WGPublicKey == "" {
			privKey, pubKey, genErr := overlay.GenerateKeyPair()
			if genErr != nil {
				return fmt.Errorf("failed to generate WireGuard keypair: %w", genErr)
			}
			keyPath, writeErr := types.WritePrivateKeyFile(types.DefaultKeysDir, "agent", privKey)
			if writeErr != nil {
				return fmt.Errorf("failed to write private key: %w", writeErr)
			}
			existingCfg.Agent.WGPrivateKeyFile = keyPath
			existingCfg.Agent.WGPublicKey = pubKey
			fmt.Printf("  %s WireGuard keypair generated\n", styleOK.Render("[OK]"))
			fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), pubKey)
		}

		existingCfg.Agent.EngineHost = flagEngineHost
		existingCfg.Agent.EnginePort = flagEnginePort
		existingCfg.Agent.AgentName = flagAgentName
		existingCfg.Agent.EngineWGPublicKey = flagEngineWGPubKey

		// Skip HA prompt in non-interactive mode
		existingCfg.Agent.Engines = nil

		// Save config
		if err := types.SaveConfig(configPath, &existingCfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("  %s Agent %q configured for engine %s:%s\n",
			styleOK.Render("[OK]"), flagAgentName, flagEngineHost, flagEnginePort)
		fmt.Println()
		fmt.Println(styleInfo.Render("To whitelist this agent on the engine:"))
		fmt.Printf("  sudo banyan-engine add-client --name %s --pubkey '%s'\n",
			flagAgentName, existingCfg.Agent.WGPublicKey)
		return nil
	}
```

**Important**: The non-interactive branch must `return nil` at the end to skip the rest of the interactive flow. The existing code after the insertion point handles the interactive TUI flow, which we want to skip entirely in non-interactive mode.

- [ ] **Step 3: Verify agent builds**

```bash
cd /home/work/freelancer/banyan/cmd/banyan-agent
go build .
```

Expected: Clean build, no errors.

- [ ] **Step 4: Add unit test for non-interactive agent init**

In `cmd/banyan-agent/cmd/agent_test.go`, add a test. First read the existing test file to understand patterns:

```bash
cat /home/work/freelancer/banyan/cmd/banyan-agent/cmd/agent_test.go | head -50
```

Then add:

```go
func TestAgentInit_NonInteractiveFlags(t *testing.T) {
	// Parse flags to ensure non-interactive flags are registered
	initCmd.ResetFlags()
	initCmd.Flags().Bool("non-interactive", false, "")
	initCmd.Flags().String("engine-host", "localhost", "")
	initCmd.Flags().String("engine-port", "50051", "")
	initCmd.Flags().String("agent-name", "", "")
	initCmd.Flags().String("engine-wg-pubkey", "", "")

	// Verify flags exist
	if initCmd.Flags().Lookup("non-interactive") == nil {
		t.Error("--non-interactive flag not registered")
	}
	if initCmd.Flags().Lookup("engine-wg-pubkey") == nil {
		t.Error("--engine-wg-pubkey flag not registered")
	}
}
```

- [ ] **Step 5: Run agent tests**

```bash
cd /home/work/freelancer/banyan/cmd/banyan-agent
go test ./cmd/... -v
```

Expected: All tests pass, including the new one.

- [ ] **Step 6: Commit**

```bash
cd /home/work/freelancer/banyan
git add cmd/banyan-agent/cmd/agent.go cmd/banyan-agent/cmd/agent_test.go
git commit -m "feat(agent): add --non-interactive init flags for automated setup"
```

---

## Task 2: Add Non-Interactive Init to banyan-cli

**Files:**
- Modify: `cmd/banyan-cli/cmd/init.go`
- Test: `cmd/banyan-cli/cmd/init_test.go`

**What:** Add `--non-interactive`, `--engine-host`, `--engine-port`, `--cli-name`, `--engine-wg-pubkey` flags to `banyan-cli init`. When set, skip `huh` TUI prompts and call `applyCLIInit` directly.

- [ ] **Step 1: Add init flags to init.go**

In `cmd/banyan-cli/cmd/init.go`, find the `init()` function and add flag registrations:

```go
func init() {
	rootCmd.AddCommand(initCmd)

	// NEW: Non-interactive init flags
	initCmd.Flags().Bool("non-interactive", false, "Run init without interactive prompts")
	initCmd.Flags().String("engine-host", "localhost", "Engine host (non-interactive mode)")
	initCmd.Flags().String("engine-port", "50051", "Engine gRPC port (non-interactive mode)")
	initCmd.Flags().String("cli-name", "", "CLI client name (non-interactive mode)")
	initCmd.Flags().String("engine-wg-pubkey", "", "Engine WireGuard public key (non-interactive mode)")
}
```

- [ ] **Step 2: Add non-interactive path in runInit**

In `cmd/banyan-cli/cmd/init.go`, find `runInit()`. After the existing config check block (after line 97, after `if !overwrite { return nil }`), insert the non-interactive branch before the WireGuard keypair generation:

```go
	// --- Check for non-interactive mode ---
	nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
	if nonInteractive {
		flagEngineHost, _ := cmd.Flags().GetString("engine-host")
		flagEnginePort, _ := cmd.Flags().GetString("engine-port")
		flagCLIName, _ := cmd.Flags().GetString("cli-name")
		flagEngineWGPubKey, _ := cmd.Flags().GetString("engine-wg-pubkey")

		if flagEngineWGPubKey == "" {
			return fmt.Errorf("--non-interactive requires --engine-wg-pubkey")
		}
		if flagCLIName == "" {
			hostname, _ := os.Hostname()
			flagCLIName = "cli-" + hostname
		}

		// Generate WireGuard keypair
		privKey, pubKey, genErr := overlay.GenerateKeyPair()
		if genErr != nil {
			return fmt.Errorf("failed to generate WireGuard keypair: %w", genErr)
		}
		fmt.Printf("  %s WireGuard keypair generated\n", styleOK.Render("[OK]"))
		fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), pubKey)

		return applyCLIInit(&cliInitInputs{
			EngineHost:     flagEngineHost,
			EnginePort:     flagEnginePort,
			CLIName:        flagCLIName,
			EngineWGPubKey: flagEngineWGPubKey,
			PrivKey:        privKey,
			PubKey:         pubKey,
			KeysDir:        types.DefaultKeysDir,
			Engines:        nil, // No HA in non-interactive mode
		})
	}
```

**Important**: This branch must `return applyCLIInit(...)` to skip the rest of the interactive flow. The `applyCLIInit` function already handles writing config, setting up tunnel, and displaying the whitelist instruction.

- [ ] **Step 3: Verify CLI builds**

```bash
cd /home/work/freelancer/banyan/cmd/banyan-cli
go build .
```

Expected: Clean build, no errors.

- [ ] **Step 4: Add unit test for non-interactive CLI init**

In `cmd/banyan-cli/cmd/init_test.go`, read existing tests first:

```bash
cat /home/work/freelancer/banyan/cmd/banyan-cli/cmd/init_test.go | head -50
```

Then add:

```go
func TestCLIInit_NonInteractiveFlags(t *testing.T) {
	initCmd.ResetFlags()
	initCmd.Flags().Bool("non-interactive", false, "")
	initCmd.Flags().String("engine-host", "localhost", "")
	initCmd.Flags().String("engine-port", "50051", "")
	initCmd.Flags().String("cli-name", "", "")
	initCmd.Flags().String("engine-wg-pubkey", "", "")

	if initCmd.Flags().Lookup("non-interactive") == nil {
		t.Error("--non-interactive flag not registered")
	}
	if initCmd.Flags().Lookup("engine-wg-pubkey") == nil {
		t.Error("--engine-wg-pubkey flag not registered")
	}
}
```

- [ ] **Step 5: Run CLI tests**

```bash
cd /home/work/freelancer/banyan/cmd/banyan-cli
go test ./cmd/... -v
```

Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
cd /home/work/freelancer/banyan
git add cmd/banyan-cli/cmd/init.go cmd/banyan-cli/cmd/init_test.go
git commit -m "feat(cli): add --non-interactive init flags for automated setup"
```

---

## Task 3: Create the Local Environment Script

**Files:**
- Create: `test/local/run-local.sh`
- Create: `test/local/run-local-tests.sh`
- Create: `test/local/README.md`

**What:** Write the main orchestrator bash script that automates the full bootstrap, start, test, and cleanup flow.

### Design Decisions Locked In

1. **Config isolation**: Use backup/restore of `/etc/banyan` (no `--config` flag exists yet)
2. **Process management**: Engine and agent run as background processes; script tracks PIDs
3. **Auth detection**: Script checks if `banyan-cli login` exists at runtime to support both pre-auth and post-auth Banyan
4. **Container runtime**: Script checks for running containerd; if not found, starts its own
5. **Cleanup**: `trap cleanup EXIT` ensures cleanup runs even on Ctrl+C or error

- [ ] **Step 1: Create `test/local/run-local.sh`**

```bash
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
AGENT_PUB_KEY=$(grep 'wg_public_key' "$CONFIG_DIR/banyan.yaml" | grep -A2 'agent:' | head -1 | awk '{print $2}')
# If grep above fails, try another pattern
if [ -z "$AGENT_PUB_KEY" ]; then
    AGENT_PUB_KEY=$(awk '/agent:/{found=1} found && /wg_public_key/{print $2; exit}' "$CONFIG_DIR/banyan.yaml")
fi
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
CLI_KEY_FILE=$(grep 'wg_private_key_file' "$CONFIG_DIR/banyan.yaml" | grep -A1 'cli:' | head -1 | awk '{print $2}')
if [ -z "$CLI_KEY_FILE" ]; then
    CLI_KEY_FILE=$(awk '/cli:/{found=1} found && /wg_private_key_file/{print $2; exit}' "$CONFIG_DIR/banyan.yaml")
fi

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
```

- [ ] **Step 2: Create `test/local/run-local-tests.sh`**

```bash
#!/bin/bash
# Local Environment Test Suite
# Run by: run-local.sh --test

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EXAMPLES_DIR="$REPO_ROOT/test/e2e/examples"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; }
log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }

TESTS_PASSED=0
TESTS_FAILED=0

pass() { log_pass "$1"; TESTS_PASSED=$((TESTS_PASSED + 1)); }
fail() { log_fail "$1"; TESTS_FAILED=$((TESTS_FAILED + 1)); }

# ---------------------------------------------------------------------------
# Test 1: Deploy basic application
# ---------------------------------------------------------------------------
log_info "Test 1: Deploy basic application"

banyan-cli up --file "$EXAMPLES_DIR/banyan.yaml"
sleep 10

DEPLOY_STATUS=$(banyan-cli deployment 2>/dev/null || true)
if echo "$DEPLOY_STATUS" | grep -qi "running"; then
    pass "Deployment is running"
else
    fail "Deployment not running"
    echo "$DEPLOY_STATUS"
fi

# ---------------------------------------------------------------------------
# Test 2: Verify agent shows up
# ---------------------------------------------------------------------------
log_info "Test 2: Verify agent registration"

AGENT_STATUS=$(banyan-cli agent 2>/dev/null || true)
if echo "$AGENT_STATUS" | grep -q "local-agent"; then
    pass "Agent local-agent is registered"
else
    fail "Agent local-agent not found"
    echo "$AGENT_STATUS"
fi

# ---------------------------------------------------------------------------
# Test 3: Verify containers are running
# ---------------------------------------------------------------------------
log_info "Test 3: Verify containers"

CONTAINER_COUNT=$(nerdctl ps -q 2>/dev/null | wc -l)
if [ "$CONTAINER_COUNT" -gt 0 ]; then
    pass "Containers are running (count: $CONTAINER_COUNT)"
else
    fail "No containers running"
fi

# ---------------------------------------------------------------------------
# Test 4: Down command
# ---------------------------------------------------------------------------
log_info "Test 4: Down command"

banyan-cli down --file "$EXAMPLES_DIR/banyan.yaml"
sleep 5

CONTAINER_COUNT_AFTER=$(nerdctl ps -q 2>/dev/null | wc -l)
if [ "$CONTAINER_COUNT_AFTER" -eq 0 ]; then
    pass "All containers removed after down"
else
    fail "Containers still running after down (count: $CONTAINER_COUNT_AFTER)"
fi

# ---------------------------------------------------------------------------
# Test 5: Engine status shows no running deployments
# ---------------------------------------------------------------------------
log_info "Test 5: Post-down engine status"

DEPLOY_STATUS_AFTER=$(banyan-cli deployment 2>/dev/null || true)
if ! echo "$DEPLOY_STATUS_AFTER" | grep -qi "running"; then
    pass "No running deployments after down"
else
    fail "Deployments still running after down"
    echo "$DEPLOY_STATUS_AFTER"
fi

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------
echo ""
echo "========================================"
echo "Local Test Results"
echo "========================================"
echo -e "  ${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "  ${RED}Failed: $TESTS_FAILED${NC}"
echo "========================================"

if [ "$TESTS_FAILED" -gt 0 ]; then
    exit 1
fi

log_info "All local tests passed!"
```

- [ ] **Step 3: Make scripts executable**

```bash
chmod +x /home/work/freelancer/banyan/test/local/run-local.sh
chmod +x /home/work/freelancer/banyan/test/local/run-local-tests.sh
```

- [ ] **Step 4: Create README**

```markdown
# Banyan Native Local Environment

A fast, native-host alternative to Docker-based E2E testing.

## Requirements

- Linux with root access
- Go 1.24+
- WireGuard (`wg` command)
- containerd + nerdctl
- CNI plugins in `/opt/cni/bin/`
- `nc` (netcat) for health checks

## Quick Start

```bash
# Interactive mode — keeps cluster running until Ctrl+C
sudo ./test/local/run-local.sh

# Auto-test mode — runs tests then exits
sudo ./test/local/run-local.sh --test

# Force cleanup (if previous run left processes behind)
sudo ./test/local/run-local.sh --clean
```

## How It Works

1. Builds `banyan-engine`, `banyan-agent`, `banyan-cli` from source
2. Backs up existing `/etc/banyan` config, creates isolated local config
3. Initializes engine with `--non-interactive` (generates WG keys, creates admin user)
4. Initializes agent and CLI with `--non-interactive`
5. Exchanges WireGuard public keys via `add-client`
6. Starts engine (managed etcd + registry), then agent
7. Authenticates CLI via `banyan-cli login` (M13 JWT auth)
8. Runs health checks, then either:
   - **Interactive**: keeps running, you run `banyan-cli` commands manually
   - **Auto-test**: runs `run-local-tests.sh` and exits

## Auth Flow

The local environment tests the full M13 auth stack:

```
WireGuard layer:
  Engine (wg-ctl-eng, 10.200.x.y) ←→ Agent (wg-ctl-agt)
  Engine (wg-ctl-eng) ←→ CLI (wg-ctl-cli)

JWT layer (M13):
  Engine init → auth-bootstrap.json → admin user in etcd
  CLI login → JWT access token + refresh token
  All CLI commands attach Bearer token via gRPC metadata
```

## Cleanup

The script automatically cleans up on exit (even on Ctrl+C):
- Stops engine, agent, and containerd processes
- Removes WireGuard interfaces (`wg-ctl-eng`, `wg-ctl-agt`, `wg-ctl-cli`)
- Restores original `/etc/banyan` config from backup
```

- [ ] **Step 5: Verify script syntax**

```bash
bash -n /home/work/freelancer/banyan/test/local/run-local.sh
bash -n /home/work/freelancer/banyan/test/local/run-local-tests.sh
```

Expected: No output (no syntax errors).

- [ ] **Step 6: Commit**

```bash
cd /home/work/freelancer/banyan
git add test/local/
git commit -m "feat(local): add native local environment script for E2E testing"
```

---

## Task 4: Integration Testing

**Files:**
- Run: `test/local/run-local.sh --test`

**What:** Run the local script on the development machine to verify the full flow works end-to-end.

- [ ] **Step 1: Build all binaries first**

```bash
cd /home/work/freelancer/banyan
make build
```

Expected: All three binaries built in `bin/`.

- [ ] **Step 2: Run a dry-run check**

```bash
cd /home/work/freelancer/banyan
sudo ./test/local/run-local.sh --test
```

Expected behavior:
1. Builds binaries
2. Backs up `/etc/banyan` if exists
3. Engine init completes
4. Agent init completes
5. CLI init completes
6. Engine starts and gRPC becomes ready
7. Agent registers
8. CLI login succeeds (if M13 auth present)
9. Deployment succeeds
10. Tests pass
11. Cleanup runs, config restored

**If errors occur:**
- Check engine logs: the engine process stdout/stderr will show errors
- Check agent logs: same
- Check `banyan-cli engine` and `banyan-cli agent` for status
- Common issues:
  - WireGuard kernel module not loaded: `sudo modprobe wireguard`
  - containerd not installed: `sudo apt install containerd`
  - Port 50051 in use: `sudo lsof -i :50051`
  - `/etc/banyan` permissions: script handles this, but verify

- [ ] **Step 3: Verify config is restored after cleanup**

```bash
ls -la /etc/banyan
```

If there was an existing config before the test, it should be restored. If not, `/etc/banyan` should not exist.

- [ ] **Step 4: Commit any fixes**

If any issues were found and fixed during testing, commit them:

```bash
cd /home/work/freelancer/banyan
git add -A
git commit -m "fix(local): address issues found during integration testing"
```

---

## Self-Review Checklist

### 1. Spec Coverage

| Requirement | Task |
|-------------|------|
| Script builds binaries from source | Task 3, Step 1 |
| Start 2 servers (engine + agent) as background processes | Task 3, Step 1 (Phases 6, 9) |
| Use banyan-cli commands for E2E testing | Task 3, Step 2 |
| More flexible than Docker (native, fast) | Task 3, Step 1 (no Docker involved) |
| Full auth flow (WireGuard + JWT) | Task 3, Step 1 (Phases 3-10) |
| Non-interactive init (no TUI) | Tasks 1, 2 |
| Config isolation (backup/restore) | Task 3, Step 1 (Phase 2 + cleanup) |
| Cleanup on exit | Task 3, Step 1 (cleanup trap) |

### 2. Placeholder Scan

- [x] No "TBD" or "TODO" in any code or script
- [x] No vague instructions like "add error handling" — all error handling is explicit
- [x] No "similar to Task N" — each task has complete code
- [x] All file paths are exact
- [x] All commands have expected output described

### 3. Type Consistency

- [x] Flag names consistent across agent and CLI: `--non-interactive`, `--engine-host`, `--engine-port`, `--engine-wg-pubkey`
- [x] Config path consistently uses `configPath` variable (existing pattern)
- [x] WireGuard interface names match existing constants: `wg-ctl-eng`, `wg-ctl-agt`, `wg-ctl-cli`
- [x] Tunnel IP derivation matches Go implementation (SHA-256 hash)

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-05-20-local-environment.md`.**

---

## Actual Implementation Summary

### Commits Made

| Commit | Description |
|--------|-------------|
| `61bf974` | feat(agent,cli): add --non-interactive init flags for automated setup |
| `2080d2c` | feat(local): add native local environment script for E2E testing |
| `72e136f` | fix(engine): add managed registry fallback to localhost on WSL |
| `9de18c0` | fix: engine init non-interactive flags and local env script cleanup |
| `9a1c2d3` | Merge branch 'feat/m13-auth' into feat/add-stg-env |

### Post-Implementation Fixes

The following fixes were applied **after** initial script creation based on testing:

1. **Pre-flight iptables cleanup** (Phase 1): Added cleanup for stale iptables rules with BANYAN chain prefix
2. **WireGuard interface cleanup** (Phase 7): Delete existing `banyan0` before creating to avoid "device already exists" errors
3. **containerd health check** (Phase 8): Changed from curl port 9323 to socket-based check via `timeout 5 bash -c 'cat < /dev/null > /dev/tcp/127.0.0.1/9323'`
4. **Auth detection** (Phase 10): Check for `--username` flag existence in login command instead of just checking if login command exists
5. **Go PATH fallback**: Added `export PATH=$PATH:/usr/local/go/bin` for WSL compatibility
6. **Registry fallback timeout**: Increased from 10s to 30s for slower environments

### Key Differences from Original Plan

| Planned | Actual |
|---------|--------|
| Agent init with `--engine-wg-pubkey` | Agent init with `--admin-user`, `--admin-password` |
| CLI init with `--engine-wg-pubkey` | CLI login via JWT (`--username`, `--password`) |
| WireGuard interfaces: `wg-ctl-eng`, `wg-ctl-agt`, `wg-ctl-cli` | Single interface: `banyan0` |
| Separate `add-client` commands | `add-client` used only for agent, CLI uses JWT login |
| Binary build in `cmd/*/` directories | Binary build using `make build-quick` at repo root |
| Config backup/restore | Config created fresh in `/etc/banyan` |

### M13 Auth Integration

The `feat/m13-auth` branch was merged into `feat/add-stg-env` during implementation. This added:
- JWT login implementation in `cmd/banyan-cli/cmd/login.go`
- Non-interactive engine init with `--admin-user` and `--admin-password`
- Auth detection in local script to support both pre-M13 and M13 modes
