# Banyan E2E Testing Guide

A step-by-step guide to test the Banyan MVP end-to-end.

## Quick Start (Docker - Recommended)

The easiest way to test E2E is using Docker. No server setup required.

```bash
cd test/e2e

# Run the full E2E test (builds images, starts cluster, deploys app)
./run-e2e.sh

# Or manually:
docker-compose up -d
docker-compose exec engine banyan-cli deploy --file /examples/banyan.yaml
```

This creates:
- 1 Engine container (with etcd)
- 2 Worker containers (with containerd)
- Deploys a test application

---

## Docker E2E Setup (DinD)

### Architecture

```
┌─────────────────────────────────────────────────┐
│                 Docker Host                      │
│  ┌───────────────────────────────────────────┐  │
│  │         Docker Network: banyan-net         │  │
│  │                                            │  │
│  │  ┌──────────────┐    ┌──────────────────┐ │  │
│  │  │ engine       │    │ worker-1         │ │  │
│  │  │ - etcd       │◄───│ - containerd     │ │  │
│  │  │ - banyan-cli │    │ - banyan-cli     │ │  │
│  │  │   engine     │    │   agent          │ │  │
│  │  └──────────────┘    └──────────────────┘ │  │
│  │         ▲            ┌──────────────────┐ │  │
│  │         │            │ worker-2         │ │  │
│  │         └────────────│ - containerd     │ │  │
│  │                      │ - banyan-cli     │ │  │
│  │                      │   agent          │ │  │
│  │                      └──────────────────┘ │  │
│  └───────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Step 1: Build and Start

```bash
cd test/e2e

# Build images (first time only)
docker-compose build

# Start the cluster
docker-compose up -d

# Check status
docker-compose ps
```

### Step 2: Deploy an Application

```bash
# Deploy the example app
docker-compose exec engine banyan-cli deploy --file /examples/banyan.yaml

# Or validate first (dry-run)
docker-compose exec engine banyan-cli deploy --file /examples/banyan.yaml --dry-run
```

### Step 3: Verify

```bash
# Check engine status
docker-compose exec engine banyan-cli engine status

# Check worker status
docker-compose exec worker-1 banyan-cli agent status
docker-compose exec worker-2 banyan-cli agent status

# Check IPAM allocations
docker-compose exec engine banyan-cli ipam get-subnet worker-1
docker-compose exec engine banyan-cli ipam get-subnet worker-2

# Check DNS
docker-compose exec engine banyan-cli dns list
```

### Step 4: Cleanup

```bash
# Stop and remove everything
docker-compose down -v
```

### Useful Commands

```bash
# View logs
docker-compose logs -f engine
docker-compose logs -f worker-1

# Shell into a container
docker-compose exec engine bash
docker-compose exec worker-1 bash

# Restart a service
docker-compose restart worker-1
```

---

## Native Setup (Without Docker)

If you prefer to run directly on Linux without Docker:

### Prerequisites

- **Linux** with kernel 4.x+
- **etcd** installed (`apt install etcd-server` or download from https://etcd.io)
- **containerd** (for actual container creation)
- **Go 1.21+** (for building)

---

## Step 1: Build the CLI

```bash
cd /home/hungnguyenba/workspace/fertile/banyan

# Build the unified CLI
go build -o /tmp/banyan-cli ./cmd/banyan-cli

# Verify
/tmp/banyan-cli --help
```

**Expected output:**
```
banyan-cli is the unified command-line interface for Banyan container orchestration.

Commands:
  engine    Manage the Banyan Engine (control plane)
  agent     Manage the Banyan Agent (worker node)
  deploy    Deploy applications from banyan.yaml
  ipam      IP Address Management
  dns       DNS management
  debug     Debugging tools

Quick Start:
  # On the control plane node
  banyan-cli engine init
  banyan-cli engine start

  # On worker nodes
  banyan-cli agent init
  banyan-cli agent start --engine http://engine-host:2379

  # Deploy an application
  banyan-cli deploy --file banyan.yaml
```

---

## Step 2: Initialize and Start the Engine

The Engine is the control plane that manages deployments. It automatically starts etcd.

```bash
# Initialize dependencies (creates directories, checks etcd)
sudo /tmp/banyan-cli engine init

# Start Engine (automatically starts etcd if not running)
sudo /tmp/banyan-cli engine start
```

**Expected output:**
```
Banyan Engine
========================================
Starting etcd...
etcd started (PID: 12345)
Waiting for etcd to be ready...
etcd is ready
Connecting to etcd at http://localhost:2379...
Connected to etcd
Initializing VPC network with CIDR 10.0.0.0/16...
VPC components initialized: IPAM, DNS, Security
Engine components initialized: Registry, State, Orchestrator
========================================
Engine is running. Waiting for agents to register...

Usage:
  Deploy:      banyan-cli deploy --file banyan.yaml
  Agent start: banyan-cli agent start --engine http://localhost:2379

Press Ctrl+C to stop
```

**Engine subcommands:**
```bash
# Check engine status
/tmp/banyan-cli engine status

# Stop engine and etcd
/tmp/banyan-cli engine stop
```

---

## Step 3: Initialize and Start an Agent

The Agent runs on worker nodes and manages containers.

```bash
# In a new terminal

# Initialize dependencies (creates directories, checks containerd and CNI)
sudo /tmp/banyan-cli agent init

# Start Agent (connects to Engine)
sudo /tmp/banyan-cli agent start --engine http://localhost:2379
```

**Expected output:**
```
Banyan Agent
========================================
Node name: your-hostname
Connecting to Engine at http://localhost:2379...
Connected to Engine
Storage initialized
VPC components initialized: Security
CNI runtime initialized
========================================
Agent is running. Ready to receive container tasks.

Press Ctrl+C to stop
```

**Agent subcommands:**
```bash
# Check agent status
/tmp/banyan-cli agent status

# Stop agent
/tmp/banyan-cli agent stop
```

---

## Step 4: Deploy an Application

Create a `banyan.yaml` file:

```yaml
# banyan.yaml
name: my-app
services:
  web:
    image: nginx:alpine
    replicas: 2
    ports:
      - "80:80"

  api:
    image: myapp/api:v1
    replicas: 3
    ports:
      - "8080:8080"
    env:
      - LOG_LEVEL=info
    depends_on:
      - web
```

Deploy it:

```bash
# Validate manifest (dry-run)
/tmp/banyan-cli deploy --file banyan.yaml --dry-run

# Deploy
/tmp/banyan-cli deploy --file banyan.yaml
```

**Expected output:**
```
Banyan Deploy
========================================
Reading manifest: banyan.yaml
Application: my-app
Services: 2
  - web: nginx:alpine (replicas: 2)
  - api: myapp/api:v1 (replicas: 3)

Connecting to Engine at http://localhost:2379...

Creating services...
  Created service: web
  Created service: api

========================================
Deployment 'my-app' created successfully!
Deployment ID: my-app-1704307200

The Engine will schedule containers to available agents.
Use 'banyan-cli status' to check deployment status.
```

---

## Step 5: Test VPC Networking

Test the VPC networking using the CLI:

```bash
# IPAM - Allocate/release IPs
/tmp/banyan-cli ipam allocate-subnet test-host
/tmp/banyan-cli ipam allocate-ip test-host
/tmp/banyan-cli ipam get-subnet test-host

# DNS - Register/lookup services
/tmp/banyan-cli dns register web.internal 10.0.1.10
/tmp/banyan-cli dns lookup web.internal

# Debug - Trace connections
/tmp/banyan-cli debug trace 10.0.1.10 10.0.1.20 80
/tmp/banyan-cli debug connectivity test-container
```

---

## Multi-Host Testing

To test with multiple agents:

```bash
# Host 1 (Engine + Agent)
sudo /tmp/banyan-cli engine start &
sudo /tmp/banyan-cli agent start --node-name host-1

# Host 2 (Agent only - point to Host 1's Engine)
sudo /tmp/banyan-cli agent start --node-name host-2 --engine http://host1-ip:2379

# Host 3
sudo /tmp/banyan-cli agent start --node-name host-3 --engine http://host1-ip:2379
```

Each agent will get its own /24 subnet:
- host-1: 10.0.1.0/24
- host-2: 10.0.2.0/24
- host-3: 10.0.3.0/24

---

## Integration Tests

Run the automated integration tests:

```bash
# All integration tests
go run ./test/integration/integration/run_simple_deployment_integration.go

# Individual tests
go run ./test/integration/integration/run_agent_lifecycle_integration.go
go run ./test/integration/integration/run_health_monitoring_integration.go
go run ./test/integration/integration/run_network_provisioning_integration.go
go run ./test/integration/integration/run_state_reconciliation_integration.go

# VPC tests (require sudo + containerd)
sudo go run ./test/integration/vpc/run_dns_integration.go
```

---

## Troubleshooting

### etcd not running

```
Failed to connect to etcd: ...
```

**Solution:** The `engine start` command auto-starts etcd. If it fails:
```bash
# Check if etcd binary is installed
which etcd

# Install etcd
apt install etcd-server

# Manually start etcd (if needed)
etcd &
```

### Permission denied

```
Error: permission denied
```

**Solution:** Run with sudo for CNI/network operations:
```bash
sudo /tmp/banyan-cli engine start
sudo /tmp/banyan-cli agent start
```

### Subnet already allocated

```
Failed to allocate subnet: subnet already allocated
```

**Solution:** This is normal on restart. The agent will use the existing subnet.

---

## Command Reference

| Command | Description |
|---------|-------------|
| `banyan-cli engine init` | Initialize engine dependencies |
| `banyan-cli engine start` | Start engine (auto-starts etcd) |
| `banyan-cli engine stop` | Stop engine and etcd |
| `banyan-cli engine status` | Show engine status |
| `banyan-cli agent init` | Initialize agent dependencies |
| `banyan-cli agent start` | Start agent |
| `banyan-cli agent stop` | Stop agent |
| `banyan-cli agent status` | Show agent status |
| `banyan-cli deploy --file X` | Deploy from manifest |
| `banyan-cli deploy --dry-run` | Validate manifest |
| `banyan-cli ipam allocate-subnet` | Allocate subnet |
| `banyan-cli dns register` | Register DNS record |
| `banyan-cli debug trace` | Trace network path |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         User                                 │
│                    banyan.yaml                               │
│                         │                                    │
│            banyan-cli deploy --file banyan.yaml              │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│               banyan-cli engine start                        │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐            │
│  │ Orchestrator│ │   Registry  │ │    State    │            │
│  └─────────────┘ └─────────────┘ └─────────────┘            │
│                         │                                    │
│  ┌──────────────────────┴───────────────────────┐           │
│  │                  VPC Layer                    │           │
│  │  ┌──────┐ ┌──────┐ ┌──────────┐ ┌──────────┐ │           │
│  │  │ IPAM │ │ DNS  │ │ Security │ │   CNI    │ │           │
│  │  └──────┘ └──────┘ └──────────┘ └──────────┘ │           │
│  └──────────────────────────────────────────────┘           │
└────────────────────────┬────────────────────────────────────┘
                         │ etcd (auto-started)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│               banyan-cli agent start                         │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐            │
│  │  Container  │ │   Network   │ │   Health    │            │
│  │   Runtime   │ │   (VPC)     │ │  Monitor    │            │
│  └─────────────┘ └─────────────┘ └─────────────┘            │
│                         │                                    │
│                    containerd                                │
└─────────────────────────────────────────────────────────────┘
```
