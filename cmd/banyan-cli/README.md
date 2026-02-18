# Banyan CLI

Command-line client for deploying and managing applications on Banyan, plus VPC networking tools.

## Build

```bash
go build -o banyan-cli .
```

## Setup

Before using deploy/status/down/logs commands, initialize the CLI:

```bash
sudo banyan-cli init
# Enter: engine host, gRPC port (default: 50051), cluster password
```

## Deploy Commands

```bash
# Deploy from manifest
banyan-cli deploy -f banyan.yaml

# Stop services
banyan-cli down --name my-app

# Check cluster status
banyan-cli status

# Stream container logs
banyan-cli logs my-app-web-0 -f
```

## VPC Network Commands

```bash
# Create network with defaults (CIDR: 10.0.0.0/16)
banyan-cli network create

# Create network with custom name and CIDR
banyan-cli network create my-vpc 10.5.0.0/16

# List all networks
banyan-cli network list

# Get network details
banyan-cli network get <network-id>

# Delete network
banyan-cli network delete <network-id>
```
