# Development Guide

## Structure

```
banyan/
├── cmd/
│   ├── banyan-engine/ # Engine binary (control plane: data store, gRPC server, scheduling)
│   ├── banyan-agent/  # Agent binary (worker: gRPC client, task polling, container ops)
│   ├── banyan-cli/    # CLI binary (gRPC client for deploy/down/status/logs)
│                      # (VPC debug commands are in banyan-cli)
├── pkg/
│   ├── types/         # Shared types, config, auth, helpers
│   ├── rpc/           # gRPC proto definitions and generated code
│   └── vpc/           # VPC networking library
├── internal/
│   └── common/        # Private shared utilities
└── test/
    ├── unit/          # Unit tests
    ├── e2e/           # End-to-end tests (Docker Compose cluster)
    └── integration/   # Integration tests (DinD)
```

## Requirements

- Go 1.24+

## Quick Start

```bash
# One-time setup (includes pre-commit hooks)
make setup

# Development workflow
make lint-fix    # Format and fix issues
make test        # Run tests
make run-cli     # Test your changes
```

**Note**: `make setup` configures Git pre-commit hooks that automatically run `make lint` and `make test` before each commit.

## Development Commands

```bash
# Setup (run once)
make setup

# Run applications
make run-engine
make run-agent
make run-cli

# Run linter
make lint        # Check code quality
make lint-fix    # Auto-fix formatting and issues

# Testing
make test                                    # Run unit tests
make test-verbose                            # Run unit tests with verbose logging
make test-coverage                           # Run tests with coverage
make test-module MODULE=pkg/vpc/network      # Test specific module
make test-module MODULE=pkg/vpc/network VERBOSE=1  # Test module with verbose output

# Build
make build       # Build all binaries
make clean       # Clean build artifacts
```

## VPC Networking Setup

For VPC networking features (Phase 3+), you need CNI plugins installed on your system.

### Automated Installation (Recommended)

```bash
# Build all binaries
make build

# Install CNI plugins automatically (requires sudo)
sudo bin/banyan-cli setup cni
```

This installs:
- **Standard CNI plugins** (v1.8.0): bridge, host-local, portmap, loopback, etc.

Installation directory: `/opt/cni/bin/`

**Note**: Flannel is no longer required. Banyan uses a built-in overlay managed by the engine. WireGuard is the default overlay driver (encrypted L3 tunneling). VXLAN is kept as a fallback for environments without WireGuard kernel support. Both implement the `OverlayDriver` interface in `pkg/vpc/overlay/`.

### WireGuard Control Tunnel

In addition to the data plane overlay, Banyan uses a separate WireGuard tunnel (`wg-control`) to encrypt all control plane gRPC traffic between engine, agents, and CLI. This is distinct from the data plane overlay (`banyan-wg`).

```
Control plane (wg-control, port 51821/UDP):
  Engine (10.200.0.1) ←→ Agent (10.200.X.Y)    # encrypted gRPC
  Engine (10.200.0.1) ←→ CLI   (10.200.X.Y)    # encrypted gRPC

Data plane (banyan-wg, port 51820/UDP):
  Agent ←→ Agent                                 # encrypted container traffic
```

- Engine generates a keypair during `init` and displays its public key
- Agents/CLI provide the engine's public key during their `init` to enable the tunnel
- Tunnel IPs are deterministic (derived from the public key hash)
- If WireGuard is unavailable, gRPC falls back to direct TCP with public key metadata auth
- Implementation: `pkg/vpc/overlay/control_tunnel.go`

### Manual Installation

If you prefer manual installation:

```bash
# Install standard CNI plugins
sudo mkdir -p /opt/cni/bin
curl -L https://github.com/containernetworking/plugins/releases/download/v1.8.0/cni-plugins-linux-amd64-v1.8.0.tgz | \
  sudo tar -C /opt/cni/bin -xz
```

### Verify Installation

```bash
ls -lh /opt/cni/bin/
# Should show: bridge, host-local, portmap, loopback, and others
```

## Integration Testing (Docker-in-Docker)

Integration tests run inside a Docker container that provides an isolated environment with containerd, nerdctl, and etcd. This Docker-in-Docker (DinD) approach allows testing network functionality without affecting the host system.

### Prerequisites

- Docker installed and running
- Privileged container support (required for network namespaces and cgroups)

### Building the Test Container

```bash
# Build the integration test container image
docker build -t banyan-integration-test -f test/integration/Dockerfile .
```

### Running Integration Tests

```bash
# Run all integration tests
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test all

# Run specific test suites
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test dns        # DNS integration
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test debug      # Debug tools
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test security   # Security/iptables
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test cni        # CNI Docker integration
docker run --rm --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test multihost  # Multi-Host networking
```

### Available Test Suites

| Test Suite | Description | Services Required |
|------------|-------------|-------------------|
| `dns` | DNS server functionality | None |
| `debug` | Debug and diagnostic tools | None |
| `security` | iptables and security rules | iptables |
| `cni` | CNI plugin with containerd | containerd, etcd |
| `multihost` | Multi-host networking simulation | containerd, etcd |

### Debugging Integration Tests

```bash
# Start a shell inside the test container for debugging
docker run --rm -it --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test shell

# Start all services and keep the container running
docker run --rm -it --privileged -v /lib/modules:/lib/modules:ro banyan-integration-test services
```

### Test Container Architecture

The integration test container uses an isolated approach:

1. **Base Image**: `golang:1.24-alpine` with necessary tools
2. **Container Runtime**: containerd with nerdctl CLI
3. **Networking**: Built-in overlay managed by Engine (WireGuard default, VXLAN fallback)
4. **Coordination**: etcd for distributed state
5. **CNI Plugins**: Standard CNI plugins (bridge, host-local, portmap, loopback)

### Important Notes

- **Privileged mode** is required for:
  - Creating network namespaces
  - Managing iptables rules
  - Running nested containers (containerd inside Docker)

- **Kernel modules** (`/lib/modules:/lib/modules:ro`) are mounted for:
  - WireGuard and VXLAN module support
  - Bridge networking
  - Network filtering

- **Native snapshotter** is used instead of overlayfs to avoid overlay-on-overlay issues in nested containers

## Workspace

This project uses Go workspaces. Dependencies are managed via `go.work`.

## Troubleshooting

### Go PATH and Tools Issues

If you encounter "Go is not installed" or "gopls is not installed" errors when using MCP tools (like Serena), even though Go is installed, it's likely a PATH issue. To resolve this:

```bash
# Install gopls (Go language server)
go install golang.org/x/tools/gopls@latest

# Add Go bin directory to PATH permanently
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# Optional: Create symlinks to make Go available system-wide
sudo ln -sf /usr/local/go/bin/* /usr/bin/
```

This ensures gopls is installed and the Go bin directory is in your PATH for all sessions. The symlink step creates symbolic links from the Go installation (typically `/usr/local/go/bin/`) to the system binaries directory (`/usr/bin/`), making Go accessible to all processes including MCP servers.

**Note**: This assumes Go is installed in the standard location `/usr/local/go/`. Adjust the path if your Go installation is elsewhere.