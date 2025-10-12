#!/bin/bash
# Docker helper functions for test scripts

set -euo pipefail

# Create a Docker container without networking
# Usage: docker_create_test_container <container-name>
# Returns: Container ID
docker_create_test_container() {
    local name=$1
    echo "Creating Docker container: $name"
    docker run -d --name "$name" --network=none alpine sleep 3600
    docker ps -aqf "name=$name"
}

# Get container PID
# Usage: docker_get_pid <container-id>
# Returns: Container PID
docker_get_pid() {
    local container_id=$1
    docker inspect -f '{{.State.Pid}}' "$container_id"
}

# Get container network namespace path
# Usage: docker_get_netns <container-id>
# Returns: Network namespace path (/proc/<pid>/ns/net)
docker_get_netns() {
    local container_id=$1
    local pid=$(docker_get_pid "$container_id")
    echo "/proc/$pid/ns/net"
}

# Create symlink from /var/run/netns to container netns
# Usage: docker_symlink_netns <container-id>
docker_symlink_netns() {
    local container_id=$1
    local netns_path=$(docker_get_netns "$container_id")

    sudo mkdir -p /var/run/netns
    sudo ln -sf "$netns_path" "/var/run/netns/$container_id"
    echo "Created symlink: /var/run/netns/$container_id -> $netns_path"
}

# Remove netns symlink
# Usage: docker_remove_netns_symlink <container-id>
docker_remove_netns_symlink() {
    local container_id=$1
    if [ -L "/var/run/netns/$container_id" ]; then
        sudo rm "/var/run/netns/$container_id"
        echo "Removed symlink: /var/run/netns/$container_id"
    fi
}

# Execute command in container
# Usage: docker_exec <container-id> <command...>
docker_exec() {
    local container_id=$1
    shift
    docker exec "$container_id" "$@"
}

# Stop and remove container
# Usage: docker_cleanup_container <container-name>
docker_cleanup_container() {
    local name=$1
    echo "Cleaning up container: $name"
    docker stop "$name" 2>/dev/null || true
    docker rm "$name" 2>/dev/null || true
}

# Check if Docker is available and running
# Usage: docker_check_available
docker_check_available() {
    if ! command -v docker &> /dev/null; then
        echo "Error: Docker is not installed"
        return 1
    fi

    if ! docker ps &> /dev/null; then
        echo "Error: Docker daemon is not running or permission denied"
        return 1
    fi

    return 0
}
