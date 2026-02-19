---
title: Installation
description: Install Banyan and its dependencies.
sidebar:
  order: 1
---

## Quick install

The install script detects your OS, downloads the Banyan binaries, and installs all dependencies for the role you choose.

**Engine node** (control plane):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine
```

**Worker node** (runs containers):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
```

**Both** (single-machine setup):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash
```

The script installs:

| Role | What gets installed |
|------|-------------------|
| Engine | `banyan-engine`, `banyan-cli` (BadgerDB is embedded — no external store needed by default) |
| Agent | `banyan-agent`, `banyan-cli`, containerd, nerdctl, CNI plugins, BuildKit |

Supported distros: Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky Linux, AlmaLinux. Architectures: x86_64, ARM64.

### Install a specific version

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --version v0.1.0
```

## Build from source

If you prefer to build yourself, you need Go 1.24+ on the build machine only.

```bash
git clone https://github.com/fertile-org/banyan.git
cd banyan

# Build all binaries
cd cmd/banyan-engine && go build -o banyan-engine . && cd ../..
cd cmd/banyan-agent && go build -o banyan-agent . && cd ../..
cd cmd/banyan-cli && go build -o banyan-cli . && cd ../..

# Install
sudo mv cmd/banyan-engine/banyan-engine /usr/local/bin/
sudo mv cmd/banyan-agent/banyan-agent /usr/local/bin/
sudo mv cmd/banyan-cli/banyan-cli /usr/local/bin/
```

Cross-compile for remote servers:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o banyan-engine ./cmd/banyan-engine/
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o banyan-agent ./cmd/banyan-agent/
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o banyan-cli ./cmd/banyan-cli/

# Copy the right binaries to each server
scp banyan-engine banyan-cli user@engine-server:/usr/local/bin/
scp banyan-agent banyan-cli user@worker-server:/usr/local/bin/
```

When building from source, you still need to install runtime dependencies on each node manually:

- **Engine node**: No external store needed — BadgerDB is embedded by default. If you choose Redis or etcd as the backend, install them separately (see [Store Backend](#store-backend)).
- **Worker nodes**: containerd, nerdctl, BuildKit (see the [install script](https://github.com/fertile-org/banyan/blob/main/install.sh) for exact commands)

### Store backend

The Engine needs a key-value store. **BadgerDB is the default** — it's embedded in the binary, requires no external process, and persists data to disk.

| Backend | Install | When to use |
|---------|---------|-------------|
| BadgerDB (default) | Nothing to install — embedded in the binary | Recommended for most setups. Zero dependencies. |
| Redis | `sudo apt-get install redis-server` | If you already run Redis, or prefer a network-accessible store. |
| etcd | `sudo apt-get install etcd-server` | If you already run etcd, or need VPC networking (Flannel requires etcd). |

You choose the backend during `banyan-engine init`. For BadgerDB, no additional configuration is needed. For Redis or etcd, you must install and run the server yourself, then provide its address during init (e.g., `localhost:6379` for Redis, `http://localhost:2379` for etcd).

## Verify

```bash
banyan-engine --help   # On engine node
banyan-agent --help    # On worker nodes
banyan-cli --help      # On any machine
```

## Next steps

Head to the [Quickstart](/getting-started/quickstart/) to deploy your first application.
