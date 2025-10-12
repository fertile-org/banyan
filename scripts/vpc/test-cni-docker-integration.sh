#!/bin/bash
# Test CNI integration with real Docker containers
# This script verifies that our CNI runtime can attach networking to real Docker containers

set -euo pipefail

# Get script directory and source helpers
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../common/docker-helpers.sh"
source "$SCRIPT_DIR/../common/network-helpers.sh"
source "$SCRIPT_DIR/../common/build-helpers.sh"

# Configuration
CONTAINER_NAME="test-cni-integration"
NETWORK_ID="network-001"
TEST_IP="10.0.1.10"
VPC_CLI="/tmp/vpc-cli"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print colored message
print_msg() {
    local color=$1
    shift
    echo -e "${color}$@${NC}"
}

print_step() {
    echo ""
    print_msg "$YELLOW" "===> $@"
}

print_success() {
    print_msg "$GREEN" "✓ $@"
}

print_error() {
    print_msg "$RED" "✗ $@"
}

# Cleanup function
cleanup() {
    local exit_code=$?
    print_step "Cleaning up..."

    # Remove container from network (if attached)
    if check_vpc_cli "$VPC_CLI"; then
        sudo "$VPC_CLI" cni remove-container "$CONTAINER_NAME" "$NETWORK_ID" 2>/dev/null || true
    fi

    # Remove netns symlink
    docker_remove_netns_symlink "$CONTAINER_NAME" || true

    # Remove container
    docker_cleanup_container "$CONTAINER_NAME"

    if [ $exit_code -eq 0 ]; then
        print_success "Test completed successfully!"
    else
        print_error "Test failed with exit code $exit_code"
    fi

    exit $exit_code
}

trap cleanup EXIT INT TERM

# Main test flow
main() {
    print_msg "$GREEN" "🧪 Testing CNI Integration with Real Docker Container"
    echo "=================================================="

    # Step 1: Check prerequisites
    print_step "Step 1: Checking prerequisites"

    if ! docker_check_available; then
        print_error "Docker not available"
        exit 1
    fi

    if ! verify_flanneld_running; then
        print_error "Flannel daemon not running. Please run: sudo vpc-cli cni setup-plugin flannel"
        exit 1
    fi

    # Step 2: Build vpc-cli
    print_step "Step 2: Building vpc-cli"
    cd "$(get_project_root)"
    build_vpc_cli "$VPC_CLI"

    # Step 3: Create Docker container
    print_step "Step 3: Creating Docker container without networking"
    CONTAINER_ID=$(docker_create_test_container "$CONTAINER_NAME")
    print_success "Created container: $CONTAINER_ID"

    # Step 4: Verify container has no network
    print_step "Step 4: Verifying container has no networking"
    show_container_interfaces "$CONTAINER_ID"

    if verify_interface_exists "$CONTAINER_ID" "eth0"; then
        print_error "Container unexpectedly has eth0 interface"
        exit 1
    fi
    print_success "Container has no eth0 interface (as expected)"

    # Step 5: Create netns symlink
    print_step "Step 5: Creating network namespace symlink"
    docker_symlink_netns "$CONTAINER_ID"

    # Step 6: Attach container to network
    print_step "Step 6: Attaching container to network using vpc-cli"
    print_msg "$YELLOW" "Running: sudo $VPC_CLI cni add-container $CONTAINER_NAME $NETWORK_ID $TEST_IP"

    if sudo "$VPC_CLI" cni add-container "$CONTAINER_NAME" "$NETWORK_ID" "$TEST_IP"; then
        print_success "Container attached successfully"
    else
        print_error "Failed to attach container"
        exit 1
    fi

    # Step 7: Verify networking
    print_step "Step 7: Verifying container networking"

    show_container_interfaces "$CONTAINER_ID"

    if ! verify_interface_exists "$CONTAINER_ID" "eth0"; then
        print_error "eth0 interface not created"
        exit 1
    fi

    if ! verify_ip_address "$CONTAINER_ID" "eth0" "$TEST_IP"; then
        print_error "IP address not correct"
        exit 1
    fi

    # Step 8: Check host-side network setup
    print_step "Step 8: Checking host-side network setup"
    verify_veth_exists || true  # Don't fail if veth not found (depends on CNI plugin)

    # Step 9: Test connectivity (optional - depends on network setup)
    print_step "Step 9: Testing connectivity"
    print_msg "$YELLOW" "Testing connectivity to gateway (optional)..."
    verify_connectivity "$CONTAINER_ID" "10.0.1.1" || print_msg "$YELLOW" "Note: Gateway connectivity check failed (may be expected)"

    # Step 10: Remove container from network
    print_step "Step 10: Removing container from network"
    if sudo "$VPC_CLI" cni remove-container "$CONTAINER_NAME" "$NETWORK_ID"; then
        print_success "Container removed successfully"
    else
        print_error "Failed to remove container"
        exit 1
    fi

    # Step 11: Verify networking removed
    print_step "Step 11: Verifying networking removed"
    show_container_interfaces "$CONTAINER_ID"

    if verify_interface_exists "$CONTAINER_ID" "eth0"; then
        print_error "eth0 interface still exists after removal"
        exit 1
    fi
    print_success "eth0 interface removed successfully"

    print_msg "$GREEN" "\n✅ All tests passed!"
}

# Run main function
main
