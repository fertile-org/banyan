package helpers

import (
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

// CreateTestContainer creates a Docker container with no networking
func CreateTestContainer(ctx context.Context, name string) (*DockerContainer, error) {
	// Create container without networking
	cmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", name,
		"--network=none",
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

	// Get PID
	pid, err := getContainerPID(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container PID: %w", err)
	}
	container.PID = pid
	container.NetnsPath = fmt.Sprintf("/proc/%d/ns/net", pid)

	return container, nil
}

// CleanupContainer stops and removes a container
func CleanupContainer(ctx context.Context, name string) error {
	// Stop container
	stopCmd := exec.CommandContext(ctx, "docker", "stop", name)
	stopCmd.Run() // Ignore errors

	// Remove container
	rmCmd := exec.CommandContext(ctx, "docker", "rm", name)
	rmCmd.Run() // Ignore errors

	return nil
}

// ExecInContainer executes a command in the container
func ExecInContainer(ctx context.Context, containerID string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", containerID}, args...)
	cmd := exec.CommandContext(ctx, "docker", cmdArgs...)

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

// CreateNetnsSymlink creates a symlink from /var/run/netns to container netns
func CreateNetnsSymlink(container *DockerContainer) error {
	// Ensure /var/run/netns exists
	if err := os.MkdirAll("/var/run/netns", 0755); err != nil {
		return fmt.Errorf("failed to create /var/run/netns: %w", err)
	}

	// Create symlink
	symlinkPath := fmt.Sprintf("/var/run/netns/%s", container.Name)
	if err := os.Symlink(container.NetnsPath, symlinkPath); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
	}

	return nil
}

// RemoveNetnsSymlink removes the netns symlink
func RemoveNetnsSymlink(containerName string) error {
	symlinkPath := fmt.Sprintf("/var/run/netns/%s", containerName)
	if err := os.Remove(symlinkPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove symlink: %w", err)
		}
	}
	return nil
}

// CheckDockerAvailable checks if Docker is available and running
func CheckDockerAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "ps")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker not available or not running: %w", err)
	}
	return nil
}

// getContainerPID gets the PID of a container
func getContainerPID(ctx context.Context, containerID string) (int, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect",
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
			cmd := exec.CommandContext(ctx, "docker", "inspect",
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
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
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
