#!/bin/bash
# Build helper functions for test scripts

set -euo pipefail

# Build vpc-cli
# Usage: build_vpc_cli [output-path]
# Default output: /tmp/vpc-cli
build_vpc_cli() {
    local output=${1:-/tmp/vpc-cli}
    echo "Building vpc-cli to $output..."

    go build -o "$output" ./cmd/vpc-cli/

    if [ -f "$output" ]; then
        echo "✓ Built vpc-cli successfully"
        return 0
    else
        echo "✗ Failed to build vpc-cli"
        return 1
    fi
}

# Check if vpc-cli exists
# Usage: check_vpc_cli [path]
# Default path: /tmp/vpc-cli
check_vpc_cli() {
    local path=${1:-/tmp/vpc-cli}

    if [ -f "$path" ]; then
        echo "✓ vpc-cli found at $path"
        return 0
    else
        echo "✗ vpc-cli not found at $path"
        return 1
    fi
}

# Get project root directory
# Usage: get_project_root
# Returns: Path to project root
get_project_root() {
    cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd
}
