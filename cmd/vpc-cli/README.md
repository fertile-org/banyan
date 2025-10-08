# VPC CLI

Command-line interface for managing VPC networking in Banyan.

## Installation

```bash
go build -o vpc-cli ./cmd/vpc-cli/
```

## Storage

State is persisted to `~/.vpc/state.json`

## Network Commands

```bash
# Create network with defaults (CIDR: 10.0.0.0/16)
vpc-cli network create

# Create network with custom name and CIDR
vpc-cli network create my-vpc 10.5.0.0/16

# List all networks
vpc-cli network list

# Get network details
vpc-cli network get <network-id>

# Delete network
vpc-cli network delete <network-id>
```

## Example

```bash
$ vpc-cli network create test-net 10.1.0.0/16
✓ Created network:
  ID:         abc123...
  Name:       test-net
  CIDR:       10.1.0.0/16

$ vpc-cli network list
✓ Found 1 networks:
1. test-net (10.1.0.0/16) - abc123...
```
