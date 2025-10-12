# Integration Tests

Go-based integration test scripts for Banyan VPC components. These tests use real Docker containers and system resources to verify end-to-end functionality.

## Structure

```
test/integration/
├── helpers/                    # Reusable test helper packages
│   ├── docker.go               # Docker container operations
│   ├── network.go              # Network verification utilities
│   └── printer.go              # Colored output formatting
├── vpc/                        # VPC-specific test scripts
│   └── test_cni_docker_integration.go
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

- Docker installed and running
- Flannel daemon running: `sudo vpc-cli cni setup-plugin flannel`
- Root privileges for network operations

### Running Tests

**From project root:**
```bash
go run ./test/integration/vpc/test_cni_docker_integration.go
```

**From test directory:**
```bash
cd test/integration/vpc
go run test_cni_docker_integration.go
```

## Available Tests

### VPC Tests (`test/integration/vpc/`)

#### `test_cni_docker_integration.go`

Tests CNI runtime integration with real Docker containers.

**What it tests:**
1. Prerequisites (Docker, Flannel daemon)
2. Building vpc-cli
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
go run ./test/integration/vpc/test_cni_docker_integration.go
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
Start Flannel: `sudo vpc-cli cni setup-plugin flannel`

### "Permission denied"
Use `sudo` for network operations: `sudo go run ./test/integration/vpc/test_cni_docker_integration.go`

### "Cannot find package"
Run from project root or ensure Go modules are initialized:
```bash
go mod download
go mod tidy
```

## Future Tests

Planned integration tests:
- Multi-container networking tests
- Cross-host communication tests (requires multiple hosts)
- Security group enforcement tests
- DNS service discovery tests
- Performance benchmarking tests
