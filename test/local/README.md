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
