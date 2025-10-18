# Banyan Agent Integration Guide

## Overview

This guide shows how the Banyan agent integrates with VPC for container networking. The agent can either use vpc-cli commands or import the VPC Go package directly.

## Integration Method

Choose one approach:

**Option A: Using vpc-cli commands**
- Simpler implementation using shell commands
- Agent executes commands like `vpc-cli cni add-container`

**Option B: Importing Go packages**
- Direct function calls with better type safety
- Import packages: `github.com/fertile/banyan/pkg/vpc/cni`, `github.com/fertile/banyan/pkg/vpc/ipam`

See `test/integration/vpc/test_cni_docker_integration.go` for working implementation examples.

## Step-by-Step Integration

### Step 1: One-Time Setup

Before deploying any containers, perform these one-time setup operations:

**1.1. Install containerd**
```bash
sudo apt-get update
sudo apt-get install -y containerd
```

**1.2. Configure containerd**
```bash
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml
```

**1.3. Start containerd**
```bash
sudo systemctl enable containerd
sudo systemctl start containerd
sudo systemctl status containerd
```

**1.4. Install nerdctl**

Download nerdctl from GitHub releases and install:
```bash
NERDCTL_VERSION="1.7.2"
wget https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
sudo tar -C /usr/local/bin -xzf nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
rm nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz
```

Verify:
```bash
sudo nerdctl ps
```

**1.5. Verify CNI plugins**

Check that CNI plugins are installed in `/opt/cni/bin/`:
```bash
ls -la /opt/cni/bin/
```

Required plugins: bridge, host-local, loopback, flannel

If missing, download from containernetworking/plugins GitHub releases.

**1.6. Setup CNI plugin**
```bash
vpc-cli cni setup-plugin flannel
```

**1.7. Allocate subnet for this host**
```bash
vpc-cli ipam allocate-subnet <host-id>
```

The `<host-id>` is a unique identifier for this host machine. Use the hostname or generate a UUID:
- Using hostname: `$(hostname)`
- Using UUID: `$(uuidgen)`

Example:
```bash
vpc-cli ipam allocate-subnet host-01
# or
vpc-cli ipam allocate-subnet $(hostname)
```

This allocates a subnet range (e.g., 10.0.1.0/24) for all containers on this host.

### Step 2: Deploy Container

For each container deployment:

**2.1. Create container without networking**
```bash
sudo nerdctl run -d --name <container-name> --network=none <image>
```

**2.2. Allocate IP address for container**
```bash
vpc-cli ipam allocate-ip <subnet-cidr>
```

**2.3. Attach container to network**
```bash
vpc-cli cni add-container <container-name> <network-id> <ip-address>
```

**2.4. Verify network interface created**
```bash
sudo nerdctl exec <container-name> ip a
```

You should see eth0 interface with the assigned IP.

### Step 3: Remove Container

When removing a container:

**3.1. Detach container from network**
```bash
vpc-cli cni remove-container <container-name> <network-id>
```

**3.2. Release IP address**
```bash
vpc-cli ipam release-ip <ip-address>
```

**3.3. Remove container**
```bash
sudo nerdctl rm -f <container-name>
```

### Step 4: Maintain Leases

Periodically renew the host subnet lease:

```bash
vpc-cli ipam renew-lease <host-id>
```

Run this every few minutes to keep the subnet allocation active.

## Important Notes

**Network Namespaces:**
The VPC component automatically creates and manages network namespaces at `/var/run/netns/<container-name>`. The agent does not need to create these manually.

**Error Handling:**
If step 2.4 (attach to network) fails, the agent must:
1. Release the allocated IP (step 3.2)
2. Remove the container (step 3.3)

**State Tracking:**
The agent must track which IP addresses are assigned to which containers for proper cleanup during container removal.

**Multi-Host Networking (Current Limitation):**
The current implementation supports single-host networking only. Containers on different servers cannot communicate with each other yet because:
- Each server uses a hardcoded subnet (10.0.1.0/24) causing IP conflicts
- No etcd cluster for state coordination between hosts
- Flannel daemon reads static configuration file instead of shared state

Multi-host networking will be available in the next phase (Milestone 4) with embedded etcd cluster integration. After that update, containers across different servers will communicate via VXLAN overlay network with automatic routing.

For now, only deploy containers that need to communicate on the same host.

## Deployment Setup

### Container Runtime

Install containerd (required because Docker 28.x has compatibility issues):

```bash
# Install containerd
sudo apt-get install -y containerd

# Configure containerd
containerd config default | sudo tee /etc/containerd/config.toml

# Start containerd
sudo systemctl enable containerd
sudo systemctl start containerd

# Verify
sudo systemctl status containerd
```

Install nerdctl for container management:
- Download from containerd/nerdctl GitHub releases
- Extract to `/usr/local/bin/`
- Verify: `sudo nerdctl ps`

### Required Permissions

The agent needs these Linux capabilities:
- CAP_NET_ADMIN - Network configuration
- CAP_SYS_ADMIN - Namespace management
- CAP_NET_RAW - Network operations
- CAP_DAC_OVERRIDE - Access CNI state files

Deploy as systemd service with specific capabilities (not full root). The service should depend on and start after containerd.

Why elevated privileges: All container platforms (Kubernetes, AWS ECS, Docker) require elevated privileges for network namespace and interface management.

### CNI Plugins

Verify required plugins exist in `/opt/cni/bin/`:

```bash
ls -la /opt/cni/bin/
```

Required plugins:
- bridge
- host-local
- loopback
- flannel

If missing, download from containernetworking/plugins GitHub releases.

## Testing

Run the integration test:

```bash
sudo go run test/integration/vpc/test_cni_docker_integration.go
```

Manual verification:

```bash
# Check containerd
sudo systemctl status containerd

# List containers
sudo nerdctl ps

# Check network namespaces
sudo ip netns list

# Verify CNI plugins
ls -la /opt/cni/bin/

# Check Flannel daemon
sudo systemctl status flanneld
```

## Troubleshooting

**"unknown FS magic" error:**
- Using Docker 28.x instead of containerd
- Check: `sudo stat -f -c %T /var/run/netns/<container-name>` (should be "nsfs")
- Solution: Switch to containerd

**"permission denied":**
- Missing required capabilities
- Check: `getcap /usr/local/bin/banyan-agent`
- Solution: Run with sudo or proper capabilities

**"failed to create network namespace":**
- Directory missing
- Create: `sudo mkdir -p /var/run/netns`
- Check: `ls -la /var/run/netns`

**No eth0 interface after attach:**
- CNI plugin failed
- Check Flannel: `sudo systemctl status flanneld`
- Check plugin: `ls -la /opt/cni/bin/flannel`
- View interfaces: `sudo nerdctl exec <container> ip a`

## Summary

The agent integrates with VPC through these operations:

1. Create container with `--network=none`
2. Allocate IP from IPAM
3. Attach container to network via vpc-cli or Go package
4. Track container-to-IP mappings
5. Clean up network and IP on container removal

VPC handles all network namespace and CNI plugin complexity internally.
