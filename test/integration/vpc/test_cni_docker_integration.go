//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fertile/banyan/test/integration/helpers"
)

const (
	containerName = "test-cni-integration"
	networkID     = "network-001"
	testIP        = "10.0.1.10"
	vpcCLIPath    = "/tmp/vpc-cli"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Setup cleanup
	cleanup := func(exitCode int) {
		p.Step("Cleaning up")

		// Remove container from network
		removeCmd := exec.Command("sudo", vpcCLIPath, "cni", "remove-container", containerName, networkID)
		removeCmd.Run() // Ignore errors

		// Remove netns symlink
		helpers.RemoveNetnsSymlink(containerName)

		// Remove container
		helpers.CleanupContainer(ctx, containerName)

		if exitCode == 0 {
			p.Result(true, "Test completed successfully!")
		} else {
			p.Result(false, fmt.Sprintf("Test failed with exit code %d", exitCode))
		}

		os.Exit(exitCode)
	}

	// Run test
	exitCode := runTest(ctx, p)
	cleanup(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Testing CNI Integration with Real Docker Container")

	// Step 1: Check prerequisites
	p.Step("Checking prerequisites")

	if err := helpers.CheckDockerAvailable(ctx); err != nil {
		p.Error(fmt.Sprintf("Docker not available: %v", err))
		return 1
	}
	p.Success("Docker is available")

	if err := helpers.VerifyFlanneldRunning(ctx); err != nil {
		p.Error("Flannel daemon not running")
		p.Info("Please run: sudo vpc-cli cni setup-plugin flannel")
		return 1
	}
	p.Success("Flannel daemon is running")

	// Step 2: Build vpc-cli
	p.Step("Building vpc-cli")

	projectRoot := getProjectRoot()
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", vpcCLIPath, "./cmd/vpc-cli/")
	buildCmd.Dir = projectRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to build vpc-cli: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Built vpc-cli successfully")

	// Step 3: Create Docker container
	p.Step("Creating Docker container without networking")

	container, err := helpers.CreateTestContainer(ctx, containerName)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create container: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created container: %s (PID: %d)", container.ID, container.PID))

	// Wait for container to be fully running
	if err := helpers.WaitForContainer(ctx, container.ID, 5*time.Second); err != nil {
		p.Error(fmt.Sprintf("Container failed to start: %v", err))
		return 1
	}

	// Step 4: Verify container has no network
	p.Step("Verifying container has no networking")

	interfaces, err := helpers.GetContainerInterfaces(ctx, container.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get interfaces: %v", err))
		return 1
	}
	p.Code(interfaces)

	hasEth0, _ := helpers.HasInterface(ctx, container.ID, "eth0")
	if hasEth0 {
		p.Error("Container unexpectedly has eth0 interface")
		return 1
	}
	p.Success("Container has no eth0 interface (as expected)")

	// Step 5: Create netns symlink
	p.Step("Creating network namespace symlink")

	if err := helpers.CreateNetnsSymlink(container); err != nil {
		p.Error(fmt.Sprintf("Failed to create symlink: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created symlink: /var/run/netns/%s -> %s", container.Name, container.NetnsPath))

	// Step 6: Attach container to network
	p.Step("Attaching container to network using vpc-cli")
	p.Info(fmt.Sprintf("Running: sudo %s cni add-container %s %s %s", vpcCLIPath, containerName, networkID, testIP))

	addCmd := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "add-container", containerName, networkID, testIP)
	if output, err := addCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to attach container: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container attached successfully")

	// Step 7: Verify networking
	p.Step("Verifying container networking")

	interfaces, err = helpers.GetContainerInterfaces(ctx, container.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get interfaces: %v", err))
		return 1
	}
	p.Code(interfaces)

	hasEth0, err = helpers.HasInterface(ctx, container.ID, "eth0")
	if err != nil || !hasEth0 {
		p.Error("eth0 interface not created")
		return 1
	}
	p.Success("eth0 interface exists")

	actualIP, err := helpers.GetInterfaceIP(ctx, container.ID, "eth0")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get IP address: %v", err))
		return 1
	}

	if actualIP != testIP {
		p.Error(fmt.Sprintf("IP address mismatch: expected %s, got %s", testIP, actualIP))
		return 1
	}
	p.Success(fmt.Sprintf("Container has correct IP: %s", actualIP))

	// Step 8: Check host-side network setup
	p.Step("Checking host-side network setup")

	veths, err := helpers.ListVethInterfaces(ctx)
	if err != nil {
		p.Warning(fmt.Sprintf("Failed to list veth interfaces: %v", err))
	} else if len(veths) > 0 {
		p.Success("veth interfaces found on host:")
		for _, veth := range veths {
			p.Info(fmt.Sprintf("  %s", veth))
		}
	} else {
		p.Info("No veth interfaces found (may be expected depending on CNI plugin)")
	}

	// Step 9: Test connectivity (optional)
	p.Step("Testing connectivity")
	p.Info("Testing connectivity to gateway (optional)...")

	if err := helpers.PingFromContainer(ctx, container.ID, "10.0.1.1"); err != nil {
		p.Warning("Gateway connectivity check failed (may be expected)")
	} else {
		p.Success("Container can reach gateway")
	}

	// Step 10: Remove container from network
	p.Step("Removing container from network")

	removeCmd := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "remove-container", containerName, networkID)
	if output, err := removeCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to remove container: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container removed successfully")

	// Step 11: Verify networking removed
	p.Step("Verifying networking removed")

	interfaces, err = helpers.GetContainerInterfaces(ctx, container.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get interfaces: %v", err))
		return 1
	}
	p.Code(interfaces)

	hasEth0, _ = helpers.HasInterface(ctx, container.ID, "eth0")
	if hasEth0 {
		p.Error("eth0 interface still exists after removal")
		return 1
	}
	p.Success("eth0 interface removed successfully")

	return 0
}

// getProjectRoot returns the project root directory
func getProjectRoot() string {
	// Get current working directory
	wd, err := os.Getwd()
	if err != nil {
		// Fallback to relative path
		return "../../.."
	}
	// If we're in test/integration/vpc, go up 3 levels
	if filepath.Base(wd) == "vpc" {
		return filepath.Join(wd, "..", "..", "..")
	}
	// Otherwise assume we're at project root
	return wd
}
