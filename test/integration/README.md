# Integration Tests

Go-based integration test scripts for Banyan VPC components. These tests use real Docker containers and system resources to verify end-to-end functionality.

## Docker-in-Docker (DinD) Based Testing

Integration tests run inside a Docker container with containerd, Flannel, and all dependencies pre-installed. This approach (similar to Kubernetes "kind") provides:

- **Isolated environment** - Tests don't affect host system
- **Reproducible** - Same environment across all machines
- **CI/CD ready** - No host dependencies required
- **Full network stack** - containerd, CNI, iptables, network namespaces

### Quick Start

```bash
# Build the Docker image (required on first run or after changes)
make test-integration-build

# Run all integration tests
make test-integration

# Run a specific test file
make test-integration FILE=./test/integration/vpc/run_dns_integration.go
make test-integration FILE=./test/integration/agent/run_task_executor_integration.go

# List available tests
make test-integration-list

# Start shell for debugging
make test-integration-shell
```

## Structure

```
test/integration/
├── helpers/                    # Reusable test helper packages
│   ├── docker.go               # Docker container operations
│   ├── network.go              # Network verification utilities
│   └── printer.go              # Colored output formatting
├── agent/                      # Agent-specific test scripts
│   └── run_task_executor_integration.go  # Task Executor integration tests
├── engine/                     # Engine-specific test scripts
│   ├── run_orchestrator_integration.go      # Orchestrator workflow tests
│   ├── run_state_manager_integration.go     # State management/drift tests
│   ├── run_vpc_coordinator_integration.go   # VPC Coordinator tests
│   └── run_engine_server_integration.go     # gRPC server lifecycle tests
├── integration/                # Engine-Agent integration test scripts
│   ├── run_agent_lifecycle_integration.go       # Agent registration/lifecycle tests
│   ├── run_simple_deployment_integration.go     # Full deployment workflow tests
│   ├── run_network_provisioning_integration.go  # VPC network provisioning tests
│   ├── run_health_monitoring_integration.go     # Container health monitoring tests
│   └── run_state_reconciliation_integration.go  # State drift/reconciliation tests
├── vpc/                        # VPC-specific test scripts
│   ├── run_cni_docker_integration.go   # CNI/containerd tests
│   ├── run_dns_integration.go          # DNS service discovery tests
│   ├── run_debug_integration.go        # Network debugging tests
│   ├── run_security_integration.go     # Security/iptables tests
│   └── run_multi_host_integration.go   # Multi-host VPC tests
├── Dockerfile                  # DinD test container
├── entrypoint.sh               # Test container entrypoint
├── containerd-config.toml      # containerd configuration
├── supervisord.conf            # Service manager config
├── run-integration-tests.sh    # Main test runner script
└── README.md                   # This file
```

## Why Go Instead of Bash?

We chose Go for integration tests because:

✅ **Type Safety** - Catch errors at compile time
✅ **Better Reuse** - Import actual packages as libraries
✅ **IDE Support** - Autocomplete, refactoring, go-to-definition
✅ **Consistency** - Same language as the codebase
✅ **Direct Integration** - Can import VPC packages directly
✅ **Cross-platform** - Works on Windows, Linux, macOS
✅ **Better Debugging** - Stack traces, debugger support

## Usage

### Prerequisites

- Docker installed and running (that's it!)

### Running Tests

**Recommended: Using DinD Container (no host dependencies)**
```bash
# Build image first (or after any changes to test infrastructure)
make test-integration-build

# Run all tests
make test-integration

# Run a specific test file
make test-integration FILE=./test/integration/vpc/run_dns_integration.go

# List all available test files
make test-integration-list
```

**Alternative: Running directly on host (requires containerd, flannel, etc.)**
```bash
# Requires: containerd, nerdctl, flannel, iptables, root privileges
sudo go run ./test/integration/vpc/run_cni_docker_integration.go
```

## Available Tests

### Agent Tests (`test/integration/agent/`)

#### `run_task_executor_integration.go`

Tests the Agent Task Executor with real containerd.

**What it tests:**
1. Check containerd availability
2. Create real service chain (ContainerdRuntime → ContainerUseCase → TaskExecutor)
3. Test container.create task
4. Verify container exists in containerd
5. Test container.start task
6. Verify container is running
7. Test container.stop task
8. Test container.remove task
9. Verify container cleanup

**Usage:**
```bash
make test-integration FILE=./test/integration/agent/run_task_executor_integration.go
```

### Engine Tests (`test/integration/engine/`)

These tests verify the Engine components without requiring Docker or external resources.
They use in-memory adapters for fast, isolated testing.

#### `run_orchestrator_integration.go`

Tests the full deployment workflow.

**What it tests:**
1. Creating deployment from banyan.yml
2. Parsing services correctly
3. Generating deployment plan (dry-run)
4. Verifying dependency ordering
5. Executing deployment
6. Verifying deployment state
7. Plugin hooks execution
8. Task dispatching to agents
9. Rollback functionality
10. Listing deployments
11. Circular dependency detection
12. Deleting deployment

**Usage:**
```bash
go run ./test/integration/engine/run_orchestrator_integration.go
```

#### `run_state_manager_integration.go`

Tests state management and drift detection.

**What it tests:**
1. Setting desired state
2. Retrieving desired state
3. Setting actual state (no drift)
4. Detecting replica mismatch drift
5. Detecting unhealthy instance drift
6. Detecting missing service drift
7. Triggering reconciliation
8. Verifying dispatched actions
9. Generating drift report
10. Detecting extra service drift
11. Deleting desired state
12. Managing multiple deployments

**Usage:**
```bash
go run ./test/integration/engine/run_state_manager_integration.go
```

#### `run_vpc_coordinator_integration.go`

Tests the VPC Coordinator for network management.

**What it tests:**
1. Provisioning VPC network
2. Verifying subnets creation
3. Allocating container networks
4. Verifying unique IP assignment
5. Creating security groups
6. Applying network policies
7. Registering DNS entries
8. Resolving DNS entries
9. Getting network status
10. Getting container network
11. Releasing container networks
12. Unregistering DNS
13. Deleting security policies
14. Deleting security groups
15. Deleting VPC network
16. Verifying network cleanup
17. Managing multiple VPCs

**Usage:**
```bash
go run ./test/integration/engine/run_vpc_coordinator_integration.go
```

#### `run_engine_server_integration.go`

Tests the Engine gRPC server lifecycle.

**What it tests:**
1. Creating server with default config
2. Verifying service registration
3. Starting server
4. Testing gRPC connection
5. Testing health check
6. Testing port conflict detection
7. Testing GetServices
8. Testing GetGRPCServer
9. Testing graceful shutdown
10. Testing factory pattern
11. Testing server without services
12. Testing stop without start

**Usage:**
```bash
go run ./test/integration/engine/run_engine_server_integration.go
```

### VPC Tests (`test/integration/vpc/`)

#### `run_cni_docker_integration.go`

Tests CNI runtime integration with real Docker containers.

**What it tests:**
1. Prerequisites (Docker, Flannel daemon)
2. Building banyan-cli
3. Creating Docker container without networking
4. Verifying no initial networking
5. Creating network namespace symlink
6. Attaching container to VPC network
7. Verifying network interfaces and IP addresses
8. Checking host-side network setup
9. Testing connectivity (optional)
10. Removing container from network
11. Verifying cleanup

**Usage:**
```bash
make test-integration FILE=./test/integration/vpc/run_cni_docker_integration.go
```

#### `run_dns_integration.go`

Tests DNS service discovery functionality.

**Usage:**
```bash
make test-integration FILE=./test/integration/vpc/run_dns_integration.go
```

#### `run_security_integration.go`

Tests security/iptables rule management.

**Usage:**
```bash
make test-integration FILE=./test/integration/vpc/run_security_integration.go
```

### Integration Tests (`test/integration/integration/`)

These tests verify complete Engine-Agent integration flows using in-memory adapters.
They simulate the full orchestration workflow without requiring real containerd or Docker.

#### `run_agent_lifecycle_integration.go`

Tests the complete Agent registration and lifecycle management.

**What it tests:**
1. Setting up Engine components (registry, events, gRPC server)
2. Starting Engine gRPC server
3. Agent registration with Engine
4. Verifying Agent appears in registry
5. Agent heartbeat processing
6. Agent status updates
7. Agent deregistration
8. Verifying Agent removal from registry
9. Event publication verification

**Usage:**
```bash
go run ./test/integration/integration/run_agent_lifecycle_integration.go
```

#### `run_simple_deployment_integration.go`

Tests a complete deployment workflow from banyan.yml to running containers.

**What it tests:**
1. Setting up Engine orchestrator with all dependencies
2. Connecting Agent to Engine
3. Creating deployment from banyan.yml
4. Parsing services and dependencies
5. Generating and verifying deployment plan
6. Executing deployment
7. Task dispatching to Agent
8. Verifying container creation
9. Verifying deployment state
10. Health check configuration
11. Rolling update simulation
12. Deployment cleanup

**Usage:**
```bash
go run ./test/integration/integration/run_simple_deployment_integration.go
```

#### `run_network_provisioning_integration.go`

Tests VPC network provisioning through the Engine VPC Coordinator.

**What it tests:**
1. Setting up Engine VPC Coordinator
2. Provisioning VPC network
3. Verifying subnet creation
4. Allocating container networks
5. Verifying unique IP assignment
6. Creating security groups
7. Applying network policies
8. Registering DNS entries
9. Resolving DNS entries
10. Getting network status
11. Getting container network info
12. Releasing container networks
13. Unregistering DNS
14. Deleting security policies
15. Deleting security groups
16. Deleting VPC network
17. Verifying network cleanup
18. Managing multiple VPCs
19. Cleanup verification

**Usage:**
```bash
go run ./test/integration/integration/run_network_provisioning_integration.go
```

#### `run_health_monitoring_integration.go`

Tests container health monitoring and automatic recovery.

**What it tests:**
1. Setting up Engine state management
2. Setting up Agent health monitoring
3. Creating deployment with health checks
4. Verifying desired state
5. Simulating container startup
6. Simulating successful health checks
7. Processing heartbeat with healthy status
8. Verifying no drift detected
9. Simulating container health failure
10. Detecting unhealthy container drift
11. Triggering reconciliation
12. Verifying restart action dispatched
13. Simulating container restart
14. Simulating health recovery
15. Processing recovery heartbeat
16. Verifying drift resolved
17. Generating drift report
18. Simulating Agent offline
19. Detecting Agent unreachable drift
20. Testing reconnection recovery
21. Processing reconnection heartbeat
22. Cleanup verification

**Usage:**
```bash
go run ./test/integration/integration/run_health_monitoring_integration.go
```

#### `run_state_reconciliation_integration.go`

Tests state drift detection and automatic reconciliation.

**What it tests:**
1. Setting up Engine orchestrator
2. Setting up Agent task executor
3. Creating initial deployment
4. Verifying deployment in desired state
5. Starting containers via task executor
6. Syncing actual state to Engine
7. Verifying no drift (states match)
8. Simulating container crash
9. Reporting crashed state to Engine
10. Detecting container drift
11. Triggering reconciliation
12. Generating recovery tasks
13. Executing recovery on Agent
14. Syncing recovered state
15. Verifying drift resolved
16. Simulating extra container
17. Detecting extra container drift
18. Triggering extra container removal
19. Executing removal on Agent
20. Syncing cleaned state
21. Verifying extra drift resolved
22. Simulating replica mismatch
23. Detecting replica drift
24. Triggering replica adjustment
25. Cleanup verification

**Usage:**
```bash
go run ./test/integration/integration/run_state_reconciliation_integration.go
```

## Helper Packages

### `helpers.docker` - Docker Operations

```go
import "github.com/fertile-org/banyan/test/integration/helpers"

// Create test container
container, err := helpers.CreateTestContainer(ctx, "test-container")

// Check if interface exists
hasEth0, err := helpers.HasInterface(ctx, container.ID, "eth0")

// Get interface IP
ip, err := helpers.GetInterfaceIP(ctx, container.ID, "eth0")

// Execute command in container
output, err := helpers.ExecInContainer(ctx, container.ID, "ip", "a")

// Cleanup
helpers.CleanupContainer(ctx, "test-container")
```

### `helpers.network` - Network Verification

```go
// Check if flanneld is running
err := helpers.VerifyFlanneldRunning(ctx)

// List veth interfaces
veths, err := helpers.ListVethInterfaces(ctx)

// Check if namespace exists
exists, err := helpers.VerifyNetnsExists(ctx, "test-ns")
```

### `helpers.Printer` - Formatted Output

```go
p := helpers.NewPrinter()

p.Title("My Test")
p.Step("Doing something")
p.Success("It worked!")
p.Error("It failed!")
p.Warning("Be careful")
p.Info("FYI")
p.Result(true, "All tests passed!")
```

## Writing New Tests

### Template

```go
//go:build ignore

package main

import (
    "context"
    "os"

    "github.com/fertile-org/banyan/test/integration/helpers"
)

func main() {
    ctx := context.Background()
    p := helpers.NewPrinter()

    // Setup cleanup
    cleanup := func(exitCode int) {
        p.Step("Cleaning up")
        // Your cleanup logic
        os.Exit(exitCode)
    }

    // Run test
    exitCode := runTest(ctx, p)
    cleanup(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
    p.Title("My Test")

    p.Step("Step 1")
    // Your test logic

    if err != nil {
        p.Error("Failed")
        return 1
    }
    p.Success("Passed")

    return 0
}
```

### Best Practices

1. **Use context for cancellation**
   ```go
   ctx := context.Background()
   ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
   defer cancel()
   ```

2. **Always cleanup resources**
   ```go
   defer helpers.CleanupContainer(ctx, containerName)
   defer helpers.RemoveNetnsSymlink(containerName)
   ```

3. **Print clear steps**
   ```go
   p.Step("Creating container")
   p.Success("Container created")
   p.Error("Failed to create container")
   ```

4. **Return proper exit codes**
   ```go
   if err != nil {
       p.Error(fmt.Sprintf("Error: %v", err))
       return 1  // Failure
   }
   return 0  // Success
   ```

5. **Verify prerequisites early**
   ```go
   if err := helpers.CheckDockerAvailable(ctx); err != nil {
       p.Error("Docker not available")
       return 1
   }
   ```

6. **Name test files with `run_` prefix**
   ```
   run_my_feature_integration.go
   ```
   This allows the `all` command to auto-discover tests.

## Importing VPC Packages

You can directly import and use VPC packages in tests:

```go
import (
    "github.com/fertile-org/banyan/pkg/vpc/cni"
    "github.com/fertile-org/banyan/pkg/vpc/storage"
)

func runTest(ctx context.Context, p *helpers.Printer) int {
    // Use actual VPC code
    store := storage.NewMemoryStore()
    runtime := cni.NewRuntime(store)

    // Test directly
    err := runtime.AddToNetwork(ctx, containerID, networkID, ip)
    // ...
}
```

This allows testing at the Go API level instead of just CLI level.

## Troubleshooting

### "Docker not available"
Start Docker: `sudo systemctl start docker`

### "Flannel daemon not running"
Start Flannel: `sudo banyan-cli cni setup-plugin flannel`

### "Permission denied"
Use `sudo` for network operations: `sudo go run ./test/integration/vpc/run_cni_docker_integration.go`

### "Cannot find package"
Run from project root or ensure Go modules are initialized:
```bash
go mod download
go mod tidy
```

## Adding New Tests

To add a new integration test:

1. Create a new file with `run_` prefix: `test/integration/<category>/run_my_test_integration.go`
2. Follow the template above
3. Rebuild the Docker image: `make test-integration-build`
4. Run with: `make test-integration FILE=./test/integration/<category>/run_my_test_integration.go`

The test will automatically be included when running `make test-integration`.
