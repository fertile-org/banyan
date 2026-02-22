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
| Engine | `banyan-engine`, `banyan-cli`, etcd |
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

- **Engine node**: etcd is required. By default, Banyan manages etcd for you automatically (see [Etcd](#etcd-state-store)).
- **Worker nodes**: containerd, nerdctl, BuildKit (see the [install script](https://github.com/fertile-org/banyan/blob/main/install.sh) for exact commands)

### Etcd (state store)

Banyan uses etcd to store cluster state (deployments, tasks, agent registrations). You choose how to run etcd during `banyan-engine init`:

| Mode | What happens | When to use |
|------|-------------|-------------|
| **Managed** (default) | Banyan starts and manages its own etcd process. Data stored in `<data-dir>/etcd/`. | Recommended for most setups. Zero setup. |
| **External** | You run etcd yourself, Banyan connects to it. | If you already have an etcd cluster, or need custom HA/backup. |

#### Managed etcd

Nothing to configure. Banyan starts etcd on `127.0.0.1:2379` when the engine starts and stops it when the engine stops. Data persists in `/var/lib/banyan/etcd/` by default.

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

## Important: init order

The engine must be running before you run `banyan-agent init` or `banyan-cli init`. During init, agents and CLI clients connect to the engine to exchange the cluster password for an auth token (see [Authentication](/guides/authentication/) for details). This means the setup order is always:

1. `banyan-engine init` + `banyan-engine start`
2. `banyan-agent init` (on each worker)
3. `banyan-cli init` (on any deploy machine)

## Verify

```bash
banyan-engine --help   # On engine node
banyan-agent --help    # On worker nodes
banyan-cli --help      # On any machine
```

## Next steps

Head to the [Quickstart](/getting-started/quickstart/) to deploy your first application.
