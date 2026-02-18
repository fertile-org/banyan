#!/bin/bash
set -e

# Entrypoint script for Banyan integration test container
# Generic runner that accepts test file path as parameter

echo "========================================"
echo "Banyan Integration Test Container"
echo "========================================"

# Function to wait for a service to be ready
wait_for_service() {
    local name=$1
    local check_cmd=$2
    local max_attempts=${3:-30}
    local attempt=1

    echo "Waiting for $name to be ready..."
    while ! eval "$check_cmd" &>/dev/null; do
        if [ $attempt -ge $max_attempts ]; then
            echo "ERROR: $name failed to start after $max_attempts attempts"
            return 1
        fi
        echo "  Attempt $attempt/$max_attempts..."
        sleep 1
        ((attempt++))
    done
    echo "$name is ready!"
}

# Start containerd
start_containerd() {
    if pgrep containerd > /dev/null; then
        echo "containerd already running"
        return 0
    fi
    echo "Starting containerd..."
    containerd --log-level=warn > /var/log/containerd.log 2>&1 &
    wait_for_service "containerd" "ctr version" 30
}

# Pre-pull container images needed for tests
prepull_images() {
    echo "Pre-pulling test images..."
    # Pull into multiple namespaces - tests may use different namespaces
    # Use fully qualified image reference and native snapshotter for DinD compatibility
    for namespace in default banyan-test; do
        if ! nerdctl --namespace="$namespace" --snapshotter=native --cgroup-manager=cgroupfs images | grep -q "docker.io/library/alpine"; then
            echo "  Pulling alpine image to namespace: $namespace..."
            nerdctl --namespace="$namespace" --snapshotter=native --cgroup-manager=cgroupfs pull docker.io/library/alpine:latest
        else
            echo "  Alpine image already present in namespace: $namespace"
        fi
    done
    echo "Test images ready!"
}

# Start etcd
start_etcd() {
    if pgrep etcd > /dev/null; then
        echo "etcd already running"
        return 0
    fi
    echo "Starting etcd..."
    etcd --data-dir=/var/lib/etcd \
         --listen-client-urls=http://0.0.0.0:2379 \
         --advertise-client-urls=http://127.0.0.1:2379 &
    wait_for_service "etcd" "etcdctl endpoint health --endpoints=http://127.0.0.1:2379" 30
}

# Initialize Flannel network in etcd
init_flannel_network() {
    echo "Initializing Flannel network in etcd..."
    etcdctl --endpoints=http://127.0.0.1:2379 put /coreos.com/network/config \
        '{"Network":"10.0.0.0/16","SubnetLen":24,"SubnetMin":"10.0.1.0","SubnetMax":"10.0.254.0","Backend":{"Type":"vxlan"}}'
}

# Start Flannel
start_flannel() {
    if pgrep flanneld > /dev/null; then
        echo "flanneld already running"
        return 0
    fi
    echo "Starting Flannel daemon..."
    mkdir -p /run/flannel
    /usr/local/bin/flanneld \
        --etcd-endpoints=http://127.0.0.1:2379 \
        --iface=eth0 \
        --subnet-file=/run/flannel/subnet.env &
    wait_for_service "flanneld" "test -f /run/flannel/subnet.env" 30
    echo "Flannel subnet: $(cat /run/flannel/subnet.env)"
}

# Build banyan-cli if not already built
build_vpc_cli() {
    if [ ! -f /tmp/banyan-cli ]; then
        echo "Building banyan-cli..."
        cd /app
        go build -o /tmp/banyan-cli ./cmd/banyan-cli/
    fi
    echo "banyan-cli ready at /tmp/banyan-cli"
}

# Start all services
start_all_services() {
    start_containerd
    prepull_images
    start_etcd
    init_flannel_network
    start_flannel
    build_vpc_cli
}

# Run a single test file
run_single_test() {
    local test_file=$1
    local test_name=$(basename "$test_file" .go)

    echo ""
    echo "========================================"
    echo "Running: $test_name"
    echo "========================================"

    cd /app
    if go run "$test_file"; then
        echo ""
        echo "========================================"
        echo "PASSED: $test_name"
        echo "========================================"
        return 0
    else
        echo ""
        echo "========================================"
        echo "FAILED: $test_name"
        echo "========================================"
        return 1
    fi
}

# Run all tests in a directory
run_all_tests() {
    local test_dir=${1:-./test/integration}
    local failed=0
    local passed=0

    echo "Finding all integration tests in $test_dir..."

    # Find all run_*.go files
    while IFS= read -r -d '' test_file; do
        if run_single_test "$test_file"; then
            passed=$((passed + 1))
        else
            failed=$((failed + 1))
        fi
    done < <(find "$test_dir" -name "run_*.go" -type f -print0)

    echo ""
    echo "========================================"
    echo "Test Summary"
    echo "========================================"
    echo "Passed: $passed"
    echo "Failed: $failed"
    echo ""

    if [ $failed -gt 0 ]; then
        echo "RESULT: SOME TESTS FAILED"
        return 1
    else
        echo "RESULT: ALL TESTS PASSED"
        return 0
    fi
}

# Show usage
show_usage() {
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  <test-file>   - Run a specific test file (e.g., ./test/integration/vpc/run_dns_integration.go)"
    echo "  all           - Run all integration tests"
    echo "  shell         - Start a shell for debugging"
    echo "  services      - Start all services and keep running"
    echo ""
    echo "Examples:"
    echo "  $0 ./test/integration/vpc/run_dns_integration.go"
    echo "  $0 ./test/integration/agent/run_task_executor_integration.go"
    echo "  $0 all"
    echo "  $0 shell"
}

# Main entry point
case "${1:-}" in
    ""|--help|-h)
        show_usage
        exit 0
        ;;
    all)
        echo "Running all integration tests..."
        start_all_services
        run_all_tests ./test/integration
        ;;
    shell)
        echo "Starting shell for debugging..."
        start_all_services
        exec /bin/bash
        ;;
    services)
        echo "Starting all services (for debugging)..."
        start_all_services
        echo "All services started. Container will stay running."
        tail -f /dev/null
        ;;
    *)
        # Treat as a test file path
        TEST_FILE="$1"

        # Validate file exists
        if [ ! -f "$TEST_FILE" ]; then
            echo "ERROR: Test file not found: $TEST_FILE"
            echo ""
            show_usage
            exit 1
        fi

        # Start all services (tests can use what they need)
        start_all_services

        # Run the test
        run_single_test "$TEST_FILE"
        ;;
esac
