---
title: Installation
description: Install Banyan and its dependencies.
sidebar:
  order: 1
---

## Quick install

The install script detects your OS, downloads `banyan-cli`, and installs all dependencies for the role you choose.

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
| Engine | `banyan-cli`, etcd |
| Agent | `banyan-cli`, containerd, nerdctl, CNI plugins |

Supported distros: Ubuntu, Debian, CentOS, RHEL, Fedora, Rocky Linux, AlmaLinux. Architectures: x86_64, ARM64.

### Install a specific version

```bash
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --version v0.1.0
```

## Build from source

If you prefer to build yourself, you need Go 1.24+ on the build machine only.

```bash
git clone https://github.com/fertile-org/banyan.git
cd banyan/cmd/banyan-cli
go build -o banyan-cli .
sudo mv banyan-cli /usr/local/bin/
```

Cross-compile for remote servers:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o banyan-cli .
scp banyan-cli user@server:/usr/local/bin/
```

When building from source, you still need to install dependencies on each node manually:

- **Engine node**: etcd (`sudo apt-get install etcd-server` on Debian/Ubuntu)
- **Worker nodes**: containerd and nerdctl (see the [install script](https://github.com/fertile-org/banyan/blob/main/install.sh) for exact commands)

## Verify

```bash
banyan-cli --help
```

## Next steps

Head to the [Quickstart](/getting-started/quickstart/) to deploy your first application.
