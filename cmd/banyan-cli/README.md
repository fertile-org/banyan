# Banyan CLI

Command-line interface for deploying and managing applications on Banyan, plus VPC networking tools.

## Build

```bash
go build -o banyan-cli ./cmd/banyan-cli/
```

## Deploy Commands

```bash
# Deploy from manifest
banyan-cli deploy -f banyan.yaml --engine http://engine:8443

# Stop services
banyan-cli down --name my-app --engine http://engine:8443

# Check cluster status
banyan-cli status --engine http://engine:8443

# Stream container logs
banyan-cli logs my-app-web-0 --engine http://engine:8443
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

## Example

```bash
$ banyan-cli network create test-net 10.1.0.0/16
Created network:
  ID:         abc123...
  Name:       test-net
  CIDR:       10.1.0.0/16

$ banyan-cli network list
Found 1 networks:
1. test-net (10.1.0.0/16) - abc123...
```
