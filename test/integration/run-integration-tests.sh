#!/bin/bash
set -e

# Integration Test Runner Script
# Builds and runs integration tests inside a Docker container

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IMAGE_NAME="banyan-integration-tests"
CONTAINER_NAME="banyan-integration-test-runner"

# Parse arguments
TEST_TARGET=""
BUILD_ONLY=false
FORCE_REBUILD=false

for arg in "$@"; do
    case "$arg" in
        --build)
            BUILD_ONLY=true
            FORCE_REBUILD=true
            ;;
        --rebuild)
            FORCE_REBUILD=true
            ;;
        --help|-h)
            TEST_TARGET="--help"
            ;;
        *)
            TEST_TARGET="$arg"
            ;;
    esac
done

# Help
if [ "$TEST_TARGET" = "--help" ] || [ "$TEST_TARGET" = "-h" ]; then
    echo "Usage: $0 <test-file|command> [options]"
    echo ""
    echo "Commands:"
    echo "  <test-file>   - Run a specific test file"
    echo "  all           - Run all integration tests"
    echo "  shell         - Start a shell for debugging"
    echo "  services      - Start all services and keep running"
    echo ""
    echo "Options:"
    echo "  --build       - Build/rebuild Docker image only (no test run)"
    echo "  --rebuild     - Force rebuild of Docker image before running"
    echo ""
    echo "Examples:"
    echo "  $0 --build                                              # Build image only"
    echo "  $0 ./test/integration/vpc/run_dns_integration.go        # Run specific test"
    echo "  $0 ./test/integration/agent/run_task_executor_integration.go"
    echo "  $0 all                                                  # Run all tests"
    echo "  $0 all --rebuild                                        # Rebuild and run all"
    echo "  $0 shell                                                # Debug shell"
    exit 0
fi

echo "========================================"
echo "Banyan Integration Test Runner"
echo "========================================"
echo "Project Root: $PROJECT_ROOT"
echo "Image Name: $IMAGE_NAME"
echo ""

# Check if Docker is available
if ! command -v docker &> /dev/null; then
    echo "ERROR: Docker is not installed or not in PATH"
    exit 1
fi

# Check if Docker daemon is running
if ! docker info &> /dev/null; then
    echo "ERROR: Docker daemon is not running"
    echo "Please start Docker and try again"
    exit 1
fi

# Build image if it doesn't exist or rebuild is requested
if [ "$FORCE_REBUILD" = true ] || ! docker image inspect "$IMAGE_NAME" &> /dev/null; then
    echo "Building Docker image: $IMAGE_NAME"
    echo "This may take a few minutes on first run..."
    echo ""

    cd "$PROJECT_ROOT"
    docker build \
        -t "$IMAGE_NAME" \
        -f test/integration/Dockerfile \
        .

    echo ""
    echo "Image built successfully!"
else
    echo "Using existing image: $IMAGE_NAME"
    echo "(Use --build or --rebuild to force rebuild)"
fi

# If build-only mode, exit here
if [ "$BUILD_ONLY" = true ]; then
    echo ""
    echo "========================================"
    echo "Build complete. No tests were run."
    echo "========================================"
    exit 0
fi

# Validate we have a test target
if [ -z "$TEST_TARGET" ]; then
    echo "ERROR: No test target specified"
    echo "Usage: $0 <test-file|all|shell|services>"
    exit 1
fi

echo ""
echo "Running: $TEST_TARGET"
echo "========================================"

# Cleanup any existing container
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# Run the container
# --privileged is required for:
#   - containerd to work inside container
#   - iptables rules management
#   - network namespace operations
#   - CNI plugin operations
# --cgroupns=host and cgroup mounts are needed for nested containerd
# Mount project source so changes don't require rebuild
docker run \
    --rm \
    --privileged \
    --name "$CONTAINER_NAME" \
    --cgroupns=host \
    -v /lib/modules:/lib/modules:ro \
    -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
    -v "$PROJECT_ROOT:/app:ro" \
    "$IMAGE_NAME" \
    "$TEST_TARGET"

EXIT_CODE=$?

echo ""
if [ $EXIT_CODE -eq 0 ]; then
    echo "========================================"
    echo "Integration tests completed successfully!"
    echo "========================================"
else
    echo "========================================"
    echo "Integration tests FAILED with exit code: $EXIT_CODE"
    echo "========================================"
fi

exit $EXIT_CODE
