---
title: Installation
description: Install Banyan on your servers. One command per machine, under 2 minutes.
sidebar:
  order: 1
---

Banyan runs as three binaries — install only what each machine needs.

## Quick install (~1 minute)

The install script detects your OS, downloads the Banyan binaries, and installs all runtime dependencies.

**Engine node** (control plane — runs the scheduler, data store, and image registry):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine
```

**Worker node** (runs your containers):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
```

**Both** (single-machine setup — engine + agent + CLI on one server):

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash
```

The script installs:

| Role | What gets installed |
|------|-------------------|
| Engine | `banyan-engine`, `banyan-cli`, etcd |
| Agent | `banyan-agent`, `banyan-cli`, containerd, nerdctl, CNI plugins, BuildKit |

Supported distros: Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky Linux, AlmaLinux. Architectures: x86_64, ARM64.

### Install a specific version

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --version v0.1.0
```

## Build from source

If you prefer to build yourself, you need Go 1.24+ on the build machine only. The compiled binaries have no Go dependency.

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

When building from source, you still need runtime dependencies on each node:

- **Engine node**: etcd (Banyan can manage this for you — see [Etcd](#etcd-state-store) below).
- **Worker nodes**: containerd, nerdctl, BuildKit. See the [install script](https://github.com/fertile-org/banyan/blob/main/install.sh) for exact commands.

### Etcd (state store)

Banyan uses etcd to store cluster state (deployments, container status, agent registrations). You choose how to run etcd during `banyan-engine init`:

| Mode | What happens | When to use |
|------|-------------|-------------|
| **Managed** (default) | Banyan starts and manages its own etcd process. Data stored in `/var/lib/banyan/etcd/`. | Recommended for most setups. Zero configuration. |
| **External** | You run etcd yourself, Banyan connects to it. | If you already have an etcd cluster, or need custom HA/backup. |

#### Managed etcd

Nothing to configure. Banyan starts etcd on `127.0.0.1:2379` when the engine starts and stops it when the engine stops. Data persists across restarts.

#### External etcd

If you choose "External" during `banyan-engine init`, the wizard asks for:

1. **Endpoints** — comma-separated etcd addresses (e.g. `http://10.0.0.1:2379,http://10.0.0.2:2379`)
2. **Connection security** — how to authenticate:

| Option | What you provide |
|--------|-----------------|
| None | Nothing — plain HTTP connection |
| Username & Password | Etcd username and password |
| TLS (CA certificate) | Path to the CA certificate file |
| mTLS (client certificates) | Paths to CA cert, client cert, and client key files |

You must install, run, and manage external etcd yourself. See the [etcd documentation](https://etcd.io/docs/) for setup instructions.

## Setup order

The engine must be running before you initialize agents or the CLI. During init, each component connects to the engine to exchange the cluster password for an auth token (see [Authentication](/guides/authentication/) for details).

The order is always:

1. **Engine**: `banyan-engine init` → `banyan-engine start`
2. **Agents**: `banyan-agent init` → `banyan-agent start` (on each worker)
3. **CLI**: `banyan-cli init` (on any machine where you want to deploy from)

## Verify

```bash
banyan-engine --help   # On engine node
banyan-agent --help    # On worker nodes
banyan-cli --help      # On any machine
```

## Next steps

Head to the [Quickstart](/getting-started/quickstart/) to deploy your first application.
