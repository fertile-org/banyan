# Test Scripts

This directory contains test scripts for Banyan VPC components. Scripts are organized by component and use shared helper functions for maintainability.

## Directory Structure

```
scripts/
├── common/                     # Reusable helper functions
│   ├── docker-helpers.sh       # Docker container operations
│   ├── network-helpers.sh      # Network verification utilities
│   └── build-helpers.sh        # Build and compilation helpers
├── vpc/                        # VPC-specific test scripts
│   └── test-cni-docker-integration.sh
└── README.md                   # This file
```

## Usage

### Prerequisites

All scripts assume you're running from the project root directory. Most VPC tests require:
- Docker installed and running
- Flannel daemon running (`sudo vpc-cli cni setup-plugin flannel`)
- Root privileges for network operations

### Running Tests

Make scripts executable first:
```bash
chmod +x scripts/vpc/*.sh
chmod +x scripts/common/*.sh
```

Run a test script:
```bash
./scripts/vpc/test-cni-docker-integration.sh
```

## Available Scripts

### VPC Scripts (`scripts/vpc/`)

#### `test-cni-docker-integration.sh`
Tests CNI runtime integration with real Docker containers.

**What it tests:**
- Creating Docker container without networking
- Attaching container to VPC network via CNI
- Verifying network interfaces and IP addresses
- Removing container from network
- Cleanup of network resources

**Prerequisites:**
- Docker running
- Flannel daemon running
- Root privileges (uses sudo)

**Usage:**
```bash
./scripts/vpc/test-cni-docker-integration.sh
```

## Helper Functions

### Docker Helpers (`scripts/common/docker-helpers.sh`)

- `docker_create_test_container <name>` - Create container without networking
- `docker_get_pid <container-id>` - Get container PID
- `docker_get_netns <container-id>` - Get network namespace path
- `docker_symlink_netns <container-id>` - Create netns symlink
- `docker_remove_netns_symlink <container-id>` - Remove netns symlink
- `docker_exec <container-id> <command...>` - Execute command in container
- `docker_cleanup_container <name>` - Stop and remove container
- `docker_check_available` - Check if Docker is available

### Network Helpers (`scripts/common/network-helpers.sh`)

- `verify_interface_exists <container-id> <interface>` - Check if interface exists
- `verify_ip_address <container-id> <interface> <expected-ip>` - Verify IP address
- `verify_connectivity <container-id> <target-ip>` - Test network connectivity
- `show_container_interfaces <container-id>` - Display network interfaces
- `verify_flanneld_running` - Check if Flannel daemon is running
- `verify_netns_exists <namespace>` - Check if namespace exists
- `verify_veth_exists` - Check for veth pairs on host

### Build Helpers (`scripts/common/build-helpers.sh`)

- `build_vpc_cli [output-path]` - Build vpc-cli binary
- `check_vpc_cli [path]` - Check if vpc-cli exists
- `get_project_root` - Get project root directory path

## Writing New Test Scripts

When creating new test scripts:

1. **Use the common helpers** - Source the appropriate helper files:
   ```bash
   SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
   source "$SCRIPT_DIR/../common/docker-helpers.sh"
   source "$SCRIPT_DIR/../common/network-helpers.sh"
   ```

2. **Follow naming conventions**:
   - `test-*` - Integration tests
   - `verify-*` - Verification/validation scripts
   - `cleanup-*` - Cleanup utilities

3. **Add error handling**:
   ```bash
   set -euo pipefail
   trap cleanup EXIT INT TERM
   ```

4. **Provide clear output**:
   - Use step markers (`print_step`)
   - Use success/error indicators (`print_success`, `print_error`)
   - Show what's being tested

5. **Clean up resources** - Always clean up in trap handler:
   ```bash
   cleanup() {
       # Remove containers
       # Remove network resources
       # Delete temporary files
   }
   ```

6. **Document prerequisites** - Clearly state what's needed to run the script

## Example Script Template

```bash
#!/bin/bash
# Brief description of what this script tests

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../common/docker-helpers.sh"
source "$SCRIPT_DIR/../common/network-helpers.sh"

cleanup() {
    # Cleanup logic here
    exit $?
}

trap cleanup EXIT INT TERM

main() {
    echo "🧪 Test Name"

    # Check prerequisites
    docker_check_available || exit 1

    # Test steps...

    echo "✅ Test passed!"
}

main
```

## Troubleshooting

### "Docker daemon is not running"
Start Docker: `sudo systemctl start docker`

### "Flannel daemon is NOT running"
Start Flannel: `sudo vpc-cli cni setup-plugin flannel`

### Permission denied errors
Most VPC network operations require root: use `sudo`

### Script not executable
Make it executable: `chmod +x scripts/vpc/<script>.sh`

## Contributing

When adding new test scripts:
1. Place them in the appropriate subdirectory (`vpc/`, etc.)
2. Use and extend common helpers when possible
3. Add documentation to this README
4. Follow the naming conventions
5. Include cleanup logic
