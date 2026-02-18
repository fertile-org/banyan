//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fertile-org/banyan/test/integration/helpers"
)

const (
	container1Name = "test-cni-container-1"
	container2Name = "test-cni-container-2"
	networkID      = "network-001"
	testIP1        = "10.0.1.10"
	testIP2        = "10.0.1.11"
	vpcCLIPath     = "/tmp/banyan-cli"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	// Setup cleanup
	cleanup := func(exitCode int) {
		p.Step("Cleaning up")

		// Remove both containers from network
		removeCmd1 := exec.Command("sudo", vpcCLIPath, "cni", "remove-container", container1Name, networkID)
		removeCmd1.Run() // Ignore errors

		removeCmd2 := exec.Command("sudo", vpcCLIPath, "cni", "remove-container", container2Name, networkID)
		removeCmd2.Run() // Ignore errors

		// Remove netns attachments
		helpers.RemoveNetnsSymlink(container1Name)
		helpers.RemoveNetnsSymlink(container2Name)

		// Remove containers
		helpers.CleanupContainer(ctx, container1Name)
		helpers.CleanupContainer(ctx, container2Name)

		// Clean up CNI state files (best effort)
		exec.Command("sudo", "rm", "-f", fmt.Sprintf("/var/lib/cni/flannel/%s", container1Name)).Run()
		exec.Command("sudo", "rm", "-f", fmt.Sprintf("/var/lib/cni/flannel/%s", container2Name)).Run()
		exec.Command("sudo", "rm", "-rf", fmt.Sprintf("/var/lib/cni/networks/%s", networkID)).Run()

		// Clean up network interfaces (best effort)
		exec.Command("sudo", "ip", "link", "delete", "cni0").Run()
		cleanupVeths := exec.Command("bash", "-c", "sudo ip link show | grep veth | awk '{print $2}' | cut -d@ -f1 | xargs -r -n1 sudo ip link delete")
		cleanupVeths.Run()

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
	p.Title("Testing CNI Integration with Container-to-Container Communication")

	// Step 1: Check prerequisites
	p.Step("Checking prerequisites")

	if err := helpers.CheckDockerAvailable(ctx); err != nil {
		p.Error(fmt.Sprintf("Docker not available: %v", err))
		return 1
	}
	p.Success("Docker is available")

	if err := helpers.VerifyFlanneldRunning(ctx); err != nil {
		p.Error("Flannel daemon not running")
		p.Info("Please run: sudo banyan-cli cni setup-plugin flannel")
		return 1
	}
	p.Success("Flannel daemon is running")

	// Step 1.5: Clean up any stale CNI state from previous runs
	p.Step("Cleaning up stale CNI state")

	// Remove any existing containers and state
	helpers.CleanupContainer(ctx, container1Name)
	helpers.CleanupContainer(ctx, container2Name)
	helpers.RemoveNetnsSymlink(container1Name)
	helpers.RemoveNetnsSymlink(container2Name)
	exec.Command("sudo", "rm", "-f", fmt.Sprintf("/var/lib/cni/flannel/%s", container1Name)).Run()
	exec.Command("sudo", "rm", "-f", fmt.Sprintf("/var/lib/cni/flannel/%s", container2Name)).Run()
	exec.Command("sudo", "rm", "-rf", fmt.Sprintf("/var/lib/cni/networks/%s", networkID)).Run()

	// Remove existing CNI bridges to ensure clean state
	exec.Command("sudo", "ip", "link", "delete", "cni0").Run()
	exec.Command("sudo", "ip", "link", "delete", "cni-network-001").Run()

	// Clean up any stale veth interfaces (from previous failed runs)
	// List all veth interfaces and delete them
	cleanupVeths := exec.Command("bash", "-c", "sudo ip link show | grep veth | awk '{print $2}' | cut -d@ -f1 | xargs -r -n1 sudo ip link delete")
	cleanupVeths.Run() // Ignore errors

	p.Success("Cleaned up stale state")

	// Step 2: Build banyan-cli
	p.Step("Building banyan-cli")

	projectRoot := getProjectRoot()
	// Use -a flag to force rebuild (ignore cache)
	buildCmd := exec.CommandContext(ctx, "go", "build", "-a", "-o", vpcCLIPath, "./cmd/banyan-cli/")
	buildCmd.Dir = projectRoot

	if output, err := buildCmd.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to build banyan-cli: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Built banyan-cli successfully")

	// Step 3: Create first Docker container
	p.Step("Creating first container without networking")

	container1, err := helpers.CreateTestContainer(ctx, container1Name)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create container 1: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created container 1: %s (PID: %d)", container1.ID, container1.PID))

	// Wait for container to be fully running
	if err := helpers.WaitForContainer(ctx, container1.ID, 5*time.Second); err != nil {
		p.Error(fmt.Sprintf("Container 1 failed to start: %v", err))
		return 1
	}

	// Step 4: Create second Docker container
	p.Step("Creating second container without networking")

	container2, err := helpers.CreateTestContainer(ctx, container2Name)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create container 2: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created container 2: %s (PID: %d)", container2.ID, container2.PID))

	// Wait for container to be fully running
	if err := helpers.WaitForContainer(ctx, container2.ID, 5*time.Second); err != nil {
		p.Error(fmt.Sprintf("Container 2 failed to start: %v", err))
		return 1
	}

	// Step 5: Verify containers have no network
	p.Step("Verifying containers have no networking")

	hasEth0, _ := helpers.HasInterface(ctx, container1.ID, "eth0")
	if hasEth0 {
		p.Error("Container 1 unexpectedly has eth0 interface")
		return 1
	}

	hasEth0, _ = helpers.HasInterface(ctx, container2.ID, "eth0")
	if hasEth0 {
		p.Error("Container 2 unexpectedly has eth0 interface")
		return 1
	}
	p.Success("Both containers have no eth0 interface (as expected)")

	// Step 6: Create netns symlinks
	p.Step("Creating network namespace symlinks")

	if err := helpers.CreateNetnsSymlink(container1); err != nil {
		p.Error(fmt.Sprintf("Failed to create symlink for container 1: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created symlink for container 1: /var/run/netns/%s", container1.Name))

	if err := helpers.CreateNetnsSymlink(container2); err != nil {
		p.Error(fmt.Sprintf("Failed to create symlink for container 2: %v", err))
		return 1
	}
	p.Success(fmt.Sprintf("Created symlink for container 2: /var/run/netns/%s", container2.Name))

	// Step 7: Attach first container to network
	p.Step("Attaching first container to network using banyan-cli")
	p.Info(fmt.Sprintf("Running: sudo %s cni add-container %s %s %s", vpcCLIPath, container1Name, networkID, testIP1))

	addCmd1 := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "add-container", container1Name, networkID, testIP1)
	if output, err := addCmd1.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to attach container 1: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container 1 attached successfully")

	// Step 8: Attach second container to network
	p.Step("Attaching second container to network using banyan-cli")
	p.Info(fmt.Sprintf("Running: sudo %s cni add-container %s %s %s", vpcCLIPath, container2Name, networkID, testIP2))

	addCmd2 := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "add-container", container2Name, networkID, testIP2)
	if output, err := addCmd2.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to attach container 2: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container 2 attached successfully")

	// Step 9: Verify container 1 networking
	p.Step("Verifying container 1 networking")

	hasEth0, err = helpers.HasInterface(ctx, container1.ID, "eth0")
	if err != nil || !hasEth0 {
		p.Error("Container 1 eth0 interface not created")
		return 1
	}
	p.Success("Container 1 eth0 interface exists")

	actualIP1, err := helpers.GetInterfaceIP(ctx, container1.ID, "eth0")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container 1 IP address: %v", err))
		return 1
	}

	if actualIP1 == "" {
		p.Error("Container 1 has no IP address assigned")
		return 1
	}
	p.Success(fmt.Sprintf("Container 1 has IP: %s (auto-assigned by Flannel)", actualIP1))

	// Step 10: Verify container 2 networking
	p.Step("Verifying container 2 networking")

	hasEth0, err = helpers.HasInterface(ctx, container2.ID, "eth0")
	if err != nil || !hasEth0 {
		p.Error("Container 2 eth0 interface not created")
		return 1
	}
	p.Success("Container 2 eth0 interface exists")

	actualIP2, err := helpers.GetInterfaceIP(ctx, container2.ID, "eth0")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container 2 IP address: %v", err))
		return 1
	}

	if actualIP2 == "" {
		p.Error("Container 2 has no IP address assigned")
		return 1
	}
	p.Success(fmt.Sprintf("Container 2 has IP: %s (auto-assigned by Flannel)", actualIP2))
	p.Info(fmt.Sprintf("Note: Requested IPs were ignored (Flannel auto-assigns IPs)"))

	// Step 11: Check host-side network setup
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

	// Step 12: Test container-to-container connectivity (CRITICAL TEST)
	p.Step("Testing container-to-container connectivity")
	p.Info(fmt.Sprintf("Pinging from container 1 (%s) to container 2 (%s)...", actualIP1, actualIP2))

	if err := helpers.PingFromContainer(ctx, container1.ID, actualIP2); err != nil {
		p.Error(fmt.Sprintf("Container 1 cannot reach container 2: %v", err))
		p.Info("This is a critical failure - containers on the same host must be able to communicate")
		return 1
	}
	p.Success(fmt.Sprintf("Container 1 can ping container 2 at %s", actualIP2))

	p.Info(fmt.Sprintf("Pinging from container 2 (%s) to container 1 (%s)...", actualIP2, actualIP1))

	if err := helpers.PingFromContainer(ctx, container2.ID, actualIP1); err != nil {
		p.Error(fmt.Sprintf("Container 2 cannot reach container 1: %v", err))
		p.Info("This is a critical failure - containers on the same host must be able to communicate")
		return 1
	}
	p.Success(fmt.Sprintf("Container 2 can ping container 1 at %s", actualIP1))

	p.Success("✓ Bidirectional container-to-container communication works!")

	// Step 13: Test gateway connectivity (optional)
	p.Step("Testing gateway connectivity (optional)")

	if err := helpers.PingFromContainer(ctx, container1.ID, "10.0.1.1"); err != nil {
		p.Warning("Gateway connectivity check failed (may be expected)")
	} else {
		p.Success("Container 1 can reach gateway")
	}

	// Step 14: Remove first container from network
	p.Step("Removing first container from network")

	removeCmd1 := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "remove-container", container1Name, networkID)
	if output, err := removeCmd1.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to remove container 1: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container 1 removed successfully")

	// Step 15: Remove second container from network
	p.Step("Removing second container from network")

	removeCmd2 := exec.CommandContext(ctx, "sudo", vpcCLIPath, "cni", "remove-container", container2Name, networkID)
	if output, err := removeCmd2.CombinedOutput(); err != nil {
		p.Error(fmt.Sprintf("Failed to remove container 2: %v", err))
		p.Code(string(output))
		return 1
	}
	p.Success("Container 2 removed successfully")

	// Step 16: Verify networking removed from both containers
	p.Step("Verifying networking removed from both containers")

	hasEth0, _ = helpers.HasInterface(ctx, container1.ID, "eth0")
	if hasEth0 {
		p.Error("Container 1 eth0 interface still exists after removal")
		return 1
	}
	p.Success("Container 1 eth0 interface removed successfully")

	hasEth0, _ = helpers.HasInterface(ctx, container2.ID, "eth0")
	if hasEth0 {
		p.Error("Container 2 eth0 interface still exists after removal")
		return 1
	}
	p.Success("Container 2 eth0 interface removed successfully")

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
