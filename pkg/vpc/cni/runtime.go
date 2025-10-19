package cni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

type Runtime struct {
	store           storage.StateStore
	securityManager vpc.SecurityManager
	cniConfigPath   string
	cniBinPath      string
}

func NewRuntime(store storage.StateStore, securityManager vpc.SecurityManager) *Runtime {
	return &Runtime{
		store:           store,
		securityManager: securityManager,
		cniConfigPath:   "/etc/cni/net.d",
		cniBinPath:      "/opt/cni/bin",
	}
}

func (r *Runtime) SetupPlugin(ctx context.Context, plugin string, config []byte) error {
	// Validate inputs
	if plugin == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}
	if len(config) == 0 {
		return fmt.Errorf("config cannot be empty")
	}

	// Validate JSON config
	var configMap map[string]interface{}
	if err := json.Unmarshal(config, &configMap); err != nil {
		return fmt.Errorf("invalid JSON config: %w", err)
	}

	// Check if plugin is supported (only flannel and calico for now)
	if plugin != "flannel" && plugin != "calico" {
		return fmt.Errorf("unsupported plugin: %s (supported: flannel, calico)", plugin)
	}

	// Write CNI config to file
	configFile := fmt.Sprintf("%s/10-%s.conf", r.cniConfigPath, plugin)

	if err := os.MkdirAll(r.cniConfigPath, 0755); err != nil {
		return fmt.Errorf("failed to create CNI config dir: %w", err)
	}

	if err := os.WriteFile(configFile, config, 0644); err != nil {
		return fmt.Errorf("failed to write CNI config: %w", err)
	}

	// Save plugin status
	status := &vpc.PluginStatus{
		Name:    plugin,
		Version: "1.0.0",
		Status:  "active",
	}

	key := fmt.Sprintf("cni/plugins/%s", plugin)
	return r.store.Save(ctx, key, status)
}

func (r *Runtime) AddToNetwork(ctx context.Context, containerID, networkID string, ip net.IP) error {
	// NOTE: The ip parameter is currently not used when using Flannel CNI plugin.
	// Flannel automatically assigns IPs from its configured subnet range.
	// TODO: Support explicit IP assignment for plugins that support it (e.g., host-local IPAM)

	// Validate inputs
	if containerID == "" {
		return fmt.Errorf("containerID cannot be empty")
	}
	if networkID == "" {
		return fmt.Errorf("networkID cannot be empty")
	}

	// Check if container already attached
	key := fmt.Sprintf("cni/containers/%s", containerID)
	var existingContainer vpc.Container
	if err := r.store.Get(ctx, key, &existingContainer); err == nil {
		return fmt.Errorf("container %s already attached to network", containerID)
	}

	// If IP is nil, we should auto-assign (for now just return error)
	if ip == nil {
		return fmt.Errorf("IP auto-assignment not yet implemented, please provide IP")
	}

	// Validate IP is in expected range (10.0.0.0/16)
	_, vpcCIDR, _ := net.ParseCIDR("10.0.0.0/16")
	if !vpcCIDR.Contains(ip) {
		return fmt.Errorf("IP %s is outside VPC range %s", ip.String(), vpcCIDR.String())
	}

	// Auto-create network namespace if it doesn't exist
	netnsPath := fmt.Sprintf("/var/run/netns/%s", containerID)
	// Use Lstat to check if bind mount or file exists (without following symlink)
	if _, err := os.Lstat(netnsPath); os.IsNotExist(err) {
		// Create /var/run/netns directory if needed
		if err := os.MkdirAll("/var/run/netns", 0755); err != nil {
			return fmt.Errorf("failed to create netns directory: %w", err)
		}

		// Create network namespace
		cmd := exec.CommandContext(ctx, "ip", "netns", "add", containerID)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create network namespace: %w (stderr: %s)", err, stderr.String())
		}
	}

	// Ensure CNI state directory exists (required by host-local IPAM plugin)
	// This directory is used by CNI plugins to store IP allocation state
	if err := os.MkdirAll("/var/lib/cni/networks", 0755); err != nil {
		return fmt.Errorf("failed to create CNI state directory: %w", err)
	}

	// Create CNI input for Flannel
	// Flannel reads subnet info from /run/flannel/subnet.env
	// We specify the delegate configuration for the bridge plugin
	cniInput := map[string]interface{}{
		"cniVersion": "0.4.0",
		"name":       networkID,
		"type":       "flannel",
		"delegate": map[string]interface{}{
			"bridge":           "cni0",
			"hairpinMode":      true,
			"isDefaultGateway": true,
		},
	}

	inputJSON, err := json.Marshal(cniInput)
	if err != nil {
		return fmt.Errorf("failed to marshal CNI input: %w", err)
	}

	// Execute CNI ADD command
	flannelBin := fmt.Sprintf("%s/flannel", r.cniBinPath)
	cmd := exec.CommandContext(ctx, flannelBin)
	cmd.Env = append(os.Environ(),
		"CNI_COMMAND=ADD",
		"CNI_CONTAINERID="+containerID,
		"CNI_NETNS="+netnsPath,
		"CNI_IFNAME=eth0",
		"CNI_PATH="+r.cniBinPath,
	)
	cmd.Stdin = bytes.NewReader(inputJSON)

	// Debug: Check namespace inodes to verify they're different
	selfNsCmd := exec.Command("stat", "-L", "-c", "%i", "/proc/self/ns/net")
	selfNsOutput, _ := selfNsCmd.Output()
	targetNsCmd := exec.Command("stat", "-L", "-c", "%i", netnsPath)
	targetNsOutput, _ := targetNsCmd.Output()
	fmt.Fprintf(os.Stderr, "DEBUG: Current process netns inode: %s", string(selfNsOutput))
	fmt.Fprintf(os.Stderr, "DEBUG: Target CNI_NETNS (%s) inode: %s", netnsPath, string(targetNsOutput))
	if string(selfNsOutput) == string(targetNsOutput) {
		fmt.Fprintf(os.Stderr, "WARNING: Current netns and target netns have the SAME inode!\n")
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("CNI ADD failed: %w (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}

	// Save container info
	container := &vpc.Container{
		ID:        containerID,
		NetworkID: networkID,
		IP:        ip,
		Status:    "attached",
	}

	if err := r.store.Save(ctx, key, container); err != nil {
		return err
	}

	// Apply security rules for this network
	if r.securityManager != nil {
		if err := r.securityManager.ApplyRules(ctx, networkID); err != nil {
			// Log warning but don't fail container creation
			fmt.Fprintf(os.Stderr, "Warning: failed to apply security rules: %v\n", err)
		}
	}

	return nil
}

func (r *Runtime) RemoveFromNetwork(ctx context.Context, containerID, networkID string) error {
	// Validate inputs
	if containerID == "" {
		return fmt.Errorf("containerID cannot be empty")
	}
	if networkID == "" {
		return fmt.Errorf("networkID cannot be empty")
	}

	// Check if container exists
	key := fmt.Sprintf("cni/containers/%s", containerID)
	var container vpc.Container
	if err := r.store.Get(ctx, key, &container); err != nil {
		return fmt.Errorf("container %s not found: %w", containerID, err)
	}

	netnsPath := fmt.Sprintf("/var/run/netns/%s", containerID)

	// Execute CNI DEL command (if namespace still exists)
	if _, err := os.Stat(netnsPath); err == nil {
		// Create CNI input for Flannel (same as ADD)
		cniInput := map[string]interface{}{
			"cniVersion": "0.4.0",
			"name":       networkID,
			"type":       "flannel",
			"delegate": map[string]interface{}{
				"hairpinMode":      true,
				"isDefaultGateway": true,
			},
		}

		inputJSON, err := json.Marshal(cniInput)
		if err != nil {
			return fmt.Errorf("failed to marshal CNI input: %w", err)
		}

		flannelBin := fmt.Sprintf("%s/flannel", r.cniBinPath)
		cmd := exec.CommandContext(ctx, flannelBin)
		cmd.Env = append(os.Environ(),
			"CNI_COMMAND=DEL",
			"CNI_CONTAINERID="+containerID,
			"CNI_NETNS="+netnsPath,
			"CNI_IFNAME=eth0",
			"CNI_PATH="+r.cniBinPath,
		)
		cmd.Stdin = bytes.NewReader(inputJSON)

		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("CNI DEL failed: %w (stderr: %s)", err, stderr.String())
		}
	}

	// Clean up network namespace
	if _, err := os.Stat(netnsPath); err == nil {
		cmd := exec.CommandContext(ctx, "ip", "netns", "delete", containerID)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			// Log but don't fail - namespace cleanup is optional
			fmt.Fprintf(os.Stderr, "Warning: failed to delete network namespace: %v (stderr: %s)\n", err, stderr.String())
		}
	}

	// Remove container info
	return r.store.Delete(ctx, key)
}

func (r *Runtime) GetPluginStatus(ctx context.Context, plugin string) (*vpc.PluginStatus, error) {
	// Validate input
	if plugin == "" {
		return nil, fmt.Errorf("plugin name cannot be empty")
	}

	key := fmt.Sprintf("cni/plugins/%s", plugin)

	var status vpc.PluginStatus
	if err := r.store.Get(ctx, key, &status); err != nil {
		// Plugin not configured, return inactive status
		return &vpc.PluginStatus{
			Name:    plugin,
			Version: "unknown",
			Status:  "inactive",
		}, nil
	}

	return &status, nil
}

// Ensure Runtime implements CNIRuntime interface
var _ vpc.CNIRuntime = (*Runtime)(nil)
