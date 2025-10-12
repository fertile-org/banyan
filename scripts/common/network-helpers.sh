#!/bin/bash
# Network verification helper functions for test scripts

set -euo pipefail

# Verify container has a network interface
# Usage: verify_interface_exists <container-id> <interface-name>
# Returns: 0 if exists, 1 otherwise
verify_interface_exists() {
    local container_id=$1
    local interface=$2

    if docker exec "$container_id" ip link show "$interface" &> /dev/null; then
        echo "✓ Interface $interface exists in container"
        return 0
    else
        echo "✗ Interface $interface NOT found in container"
        return 1
    fi
}

# Verify container has IP address on interface
# Usage: verify_ip_address <container-id> <interface-name> <expected-ip>
# Returns: 0 if IP matches, 1 otherwise
verify_ip_address() {
    local container_id=$1
    local interface=$2
    local expected_ip=$3

    local actual_ip=$(docker exec "$container_id" ip -4 addr show "$interface" | grep -oP '(?<=inet\s)\d+(\.\d+){3}' || echo "")

    if [ "$actual_ip" = "$expected_ip" ]; then
        echo "✓ Container has IP $actual_ip on $interface"
        return 0
    else
        echo "✗ Expected IP $expected_ip but got $actual_ip"
        return 1
    fi
}

# Verify container can reach an IP
# Usage: verify_connectivity <container-id> <target-ip>
# Returns: 0 if reachable, 1 otherwise
verify_connectivity() {
    local container_id=$1
    local target_ip=$2

    if docker exec "$container_id" ping -c 1 -W 2 "$target_ip" &> /dev/null; then
        echo "✓ Container can reach $target_ip"
        return 0
    else
        echo "✗ Container cannot reach $target_ip"
        return 1
    fi
}

# Show container network interfaces
# Usage: show_container_interfaces <container-id>
show_container_interfaces() {
    local container_id=$1
    echo "Container network interfaces:"
    docker exec "$container_id" ip a
}

# Check if flanneld daemon is running
# Usage: verify_flanneld_running
# Returns: 0 if running, 1 otherwise
verify_flanneld_running() {
    if pgrep flanneld > /dev/null; then
        echo "✓ Flannel daemon is running"
        return 0
    else
        echo "✗ Flannel daemon is NOT running"
        return 1
    fi
}

# Verify network namespace exists
# Usage: verify_netns_exists <namespace-name>
# Returns: 0 if exists, 1 otherwise
verify_netns_exists() {
    local ns_name=$1

    if ip netns list | grep -q "^${ns_name}"; then
        echo "✓ Network namespace $ns_name exists"
        return 0
    elif [ -L "/var/run/netns/$ns_name" ]; then
        echo "✓ Network namespace symlink $ns_name exists"
        return 0
    else
        echo "✗ Network namespace $ns_name NOT found"
        return 1
    fi
}

# Verify veth pair exists on host
# Usage: verify_veth_exists
# Returns: 0 if veth interfaces found, 1 otherwise
verify_veth_exists() {
    if ip link show | grep -q "veth"; then
        echo "✓ veth interfaces found on host:"
        ip link show | grep veth
        return 0
    else
        echo "✗ No veth interfaces found on host"
        return 1
    fi
}
