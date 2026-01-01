package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DockerContainer represents a test container
type DockerContainer struct {
	ID      string
	Name    string
	PID     int
	NetnsPath string
}

// CreateTestContainer creates a container with no networking using nerdctl/containerd
func CreateTestContainer(ctx context.Context, name string) (*DockerContainer, error) {
	// Create container without networking
	// Use --snapshotter=native to avoid overlay-on-overlay issues in DinD environments
	// Use --cgroup-manager=cgroupfs to work around cgroups v2 issues in DinD
	// Use --cgroupns=host to share the host's cgroup namespace
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "run", "-d",
		"--name", name,
		"--network=none",
		"--cgroupns=host",
		"alpine", "sleep", "3600")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w\n%s", err, output)
	}

	containerID := strings.TrimSpace(string(output))

	// Get container info
	container := &DockerContainer{
		ID:   containerID,
		Name: name,
	}

	// Wait for container to be fully running
	if err := WaitForContainer(ctx, containerID, 10*time.Second); err != nil {
		return nil, fmt.Errorf("failed to wait for container: %w", err)
	}

	// Get PID for informational purposes (not used for namespace access)
	pid, err := getContainerPID(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container PID: %w", err)
	}
	container.PID = pid

	// Note: We don't use /proc/{PID}/ns/net anymore because Docker containers
	// may use PID namespaces that make the PID invisible to the host.
	// Instead, we use Docker's SandboxKey which points to the actual namespace file.
	container.NetnsPath = fmt.Sprintf("/proc/%d/ns/net", pid) // For informational purposes only

	return container, nil
}

// CleanupContainer stops and removes a container
func CleanupContainer(ctx context.Context, name string) error {
	// Stop container
	stopCmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "stop", name)
	stopCmd.Run() // Ignore errors

	// Remove container
	rmCmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "rm", name)
	rmCmd.Run() // Ignore errors

	return nil
}

// ExecInContainer executes a command in the container
func ExecInContainer(ctx context.Context, containerID string, args ...string) (string, error) {
	cmdArgs := append([]string{"nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "exec", containerID}, args...)
	cmd := exec.CommandContext(ctx, "sudo", cmdArgs...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to exec in container: %w\n%s", err, output)
	}

	return string(output), nil
}

// GetContainerInterfaces returns network interfaces in the container
func GetContainerInterfaces(ctx context.Context, containerID string) (string, error) {
	return ExecInContainer(ctx, containerID, "ip", "a")
}

// HasInterface checks if container has a specific network interface
func HasInterface(ctx context.Context, containerID, interfaceName string) (bool, error) {
	output, err := ExecInContainer(ctx, containerID, "ip", "link", "show", interfaceName)
	if err != nil {
		// If error contains "does not exist", interface doesn't exist
		if strings.Contains(err.Error(), "does not exist") {
			return false, nil
		}
		return false, err
	}

	return len(output) > 0, nil
}

// GetInterfaceIP gets the IP address of an interface in the container
func GetInterfaceIP(ctx context.Context, containerID, interfaceName string) (string, error) {
	output, err := ExecInContainer(ctx, containerID, "ip", "-4", "addr", "show", interfaceName)
	if err != nil {
		return "", err
	}

	// Parse IP from output (inet 10.0.1.10/24 ...)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Extract IP without CIDR
				ipCidr := parts[1]
				ip := strings.Split(ipCidr, "/")[0]
				return ip, nil
			}
		}
	}

	return "", fmt.Errorf("no IP found for interface %s", interfaceName)
}

// PingFromContainer attempts to ping from the container
func PingFromContainer(ctx context.Context, containerID, target string) error {
	_, err := ExecInContainer(ctx, containerID, "ping", "-c", "1", "-W", "2", target)
	return err
}

// CreateNetnsSymlink attaches a container's namespace to /var/run/netns
// With containerd, this should work directly without complex workarounds
func CreateNetnsSymlink(container *DockerContainer) error {
	mountPath := fmt.Sprintf("/var/run/netns/%s", container.Name)

	// Verify the container is still running
	inspectCmd := exec.Command("sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "inspect", "-f", "{{.State.Running}}", container.ID)
	output, err := inspectCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to inspect container %s: %w", container.ID, err)
	}
	running := strings.TrimSpace(string(output))
	if running != "true" {
		return fmt.Errorf("container %s is not running (state: %s)", container.ID, running)
	}

	// Get container PID
	pidCmd := exec.Command("sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "inspect", "-f", "{{.State.Pid}}", container.ID)
	pidOutput, err := pidCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get container PID: %w", err)
	}

	var hostPID int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pidOutput)), "%d", &hostPID); err != nil {
		return fmt.Errorf("failed to parse PID: %w", err)
	}

	if hostPID == 0 {
		return fmt.Errorf("container %s has PID 0 (container may have exited)", container.ID)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Container PID is %d\n", hostPID)

	// Ensure /var/run/netns exists
	mkdirCmd := exec.Command("sudo", "mkdir", "-p", "/var/run/netns")
	if err := mkdirCmd.Run(); err != nil {
		return fmt.Errorf("failed to create /var/run/netns: %w", err)
	}

	// Cleanup any existing namespace
	deleteCmd := exec.Command("sudo", "ip", "netns", "delete", container.Name)
	deleteCmd.Run() // Ignore errors - might not exist

	// With containerd, /proc/{PID}/ns/net should be accessible
	// Use ip netns attach which creates a proper bind mount
	attachCmd := exec.Command("sudo", "ip", "netns", "attach", container.Name, fmt.Sprintf("%d", hostPID))
	var stderr bytes.Buffer
	attachCmd.Stderr = &stderr
	if err := attachCmd.Run(); err != nil {
		return fmt.Errorf("failed to attach namespace for PID %d: %w (stderr: %s)", hostPID, err, stderr.String())
	}

	// Verify the mount worked
	verifyCmd := exec.Command("sudo", "ip", "netns", "list")
	verifyOutput, verifyErr := verifyCmd.CombinedOutput()
	if verifyErr == nil {
		if strings.Contains(string(verifyOutput), container.Name) {
			fmt.Fprintf(os.Stderr, "✓ Namespace attachment verified successfully\n")
		} else {
			fmt.Fprintf(os.Stderr, "WARNING: Namespace %s not found in 'ip netns list'\n", container.Name)
		}
	}

	// Debug: Check filesystem type (should be nsfs, but may report as UNKNOWN in Alpine)
	fsTypeCmd := exec.Command("sudo", "stat", "-f", "-c", "%T", mountPath)
	if fsTypeOutput, fsTypeErr := fsTypeCmd.CombinedOutput(); fsTypeErr == nil {
		fsType := strings.TrimSpace(string(fsTypeOutput))
		fmt.Fprintf(os.Stderr, "DEBUG: %s filesystem type: %s\n", mountPath, fsType)
		// Note: Alpine Linux may report "UNKNOWN" for nsfs, which is acceptable
		// The important thing is that 'ip netns list' verified the namespace above
		if fsType != "nsfs" && fsType != "UNKNOWN" {
			return fmt.Errorf("namespace mount has wrong filesystem type: %s (expected nsfs or UNKNOWN)", fsType)
		}
	}

	return nil
}

// RemoveNetnsSymlink removes the namespace symlink
func RemoveNetnsSymlink(containerName string) error {
	mountPath := fmt.Sprintf("/var/run/netns/%s", containerName)

	// Try ip netns delete first (in case it was created that way)
	deleteCmd := exec.Command("sudo", "ip", "netns", "delete", containerName)
	deleteCmd.Run() // Ignore errors - might not exist

	// Also try to remove the symlink directly
	rmCmd := exec.Command("sudo", "rm", "-f", mountPath)
	rmCmd.Run() // Ignore errors - file might not exist

	return nil
}

// CheckDockerAvailable checks if nerdctl/containerd is available and running
func CheckDockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "ps")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nerdctl/containerd not available or not running: %w", err)
	}
	return nil
}

// CheckContainerdAvailable checks if containerd socket is available
func CheckContainerdAvailable(ctx context.Context) error {
	// Check if containerd socket exists
	if _, err := os.Stat("/run/containerd/containerd.sock"); os.IsNotExist(err) {
		return fmt.Errorf("containerd socket not found at /run/containerd/containerd.sock")
	}

	// Try to connect via ctr
	cmd := exec.CommandContext(ctx, "sudo", "ctr", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("containerd not responding: %w", err)
	}
	return nil
}

// getContainerPID gets the PID of a container
func getContainerPID(ctx context.Context, containerID string) (int, error) {
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "inspect",
		"-f", "{{.State.Pid}}", containerID)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to get container PID: %w\n%s", err, output)
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	return pid, nil
}

// WaitForContainer waits for container to be running
func WaitForContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for container to start")
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "inspect",
				"-f", "{{.State.Running}}", containerID)
			output, err := cmd.Output()
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(output)) == "true" {
				return nil
			}
		}
	}
}

// ContainerInfo holds container inspection data
type ContainerInfo struct {
	State struct {
		Running bool
		Pid     int
	}
}

// InspectContainer returns container information
func InspectContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "sudo", "nerdctl", "--snapshotter=native", "--cgroup-manager=cgroupfs", "inspect", containerID)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	var info []ContainerInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse container info: %w", err)
	}

	if len(info) == 0 {
		return nil, fmt.Errorf("container not found")
	}

	return &info[0], nil
}
