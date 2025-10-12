package helpers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// VerifyFlanneldRunning checks if flanneld daemon is running
func VerifyFlanneldRunning(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pgrep", "flanneld")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("flanneld daemon not running")
	}
	return nil
}

// VerifyNetnsExists checks if a network namespace exists
func VerifyNetnsExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ip", "netns", "list")
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to list network namespaces: %w", err)
	}

	namespaces := string(output)
	return strings.Contains(namespaces, name), nil
}

// ListVethInterfaces lists veth interfaces on the host
func ListVethInterfaces(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ip", "link", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	var veths []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "veth") {
			veths = append(veths, strings.TrimSpace(line))
		}
	}

	return veths, nil
}

// GetHostRoutes returns routing table
func GetHostRoutes(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "ip", "route")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get routes: %w", err)
	}
	return string(output), nil
}

// CheckInterfaceExists checks if an interface exists on host
func CheckInterfaceExists(ctx context.Context, interfaceName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ip", "link", "show", interfaceName)
	err := cmd.Run()
	return err == nil, nil
}
