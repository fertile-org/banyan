package cmd

import (
	"context"
	"fmt"
	"net"

	"github.com/fertile/banyan/pkg/vpc/cni"
	"github.com/spf13/cobra"
)

var cniCmd = &cobra.Command{
	Use:   "cni",
	Short: "CNI management commands",
	Long:  "Manage CNI plugins and container network attachments",
}

var cniSetupPluginCmd = &cobra.Command{
	Use:   "setup-plugin <plugin-name>",
	Short: "Setup a CNI plugin",
	Long: `Setup and configure a CNI plugin (flannel or calico).

Example:
  vpc-cli cni setup-plugin flannel`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugin := args[0]

		// Default configurations for supported plugins
		var config []byte
		switch plugin {
		case "flannel":
			config = []byte(`{
	"name": "flannel",
	"type": "flannel",
	"subnetFile": "/run/flannel/subnet.env",
	"dataDir": "/var/lib/cni/flannel",
	"delegate": {
		"hairpinMode": true,
		"isDefaultGateway": true
	}
}`)
		case "calico":
			config = []byte(`{
	"name": "calico",
	"type": "calico",
	"etcd_endpoints": "http://localhost:2379",
	"log_level": "info"
}`)
		default:
			return fmt.Errorf("unsupported plugin: %s (supported: flannel, calico)", plugin)
		}

		ctx := context.Background()
		store := getStore()
		runtime := cni.NewRuntime(store)

		if err := runtime.SetupPlugin(ctx, plugin, config); err != nil {
			return fmt.Errorf("failed to setup plugin: %w", err)
		}

		fmt.Printf("✓ Plugin '%s' configured successfully\n", plugin)
		return nil
	},
}

var cniAddContainerCmd = &cobra.Command{
	Use:   "add-container <container-id> <network-id> <ip>",
	Short: "Add a container to a network",
	Long: `Attach a container to a network with the specified IP address.

Example:
  vpc-cli cni add-container container-001 network-001 10.0.1.5`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]
		networkID := args[1]
		ipStr := args[2]

		ip := net.ParseIP(ipStr)
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", ipStr)
		}

		ctx := context.Background()
		store := getStore()
		runtime := cni.NewRuntime(store)

		if err := runtime.AddToNetwork(ctx, containerID, networkID, ip); err != nil {
			return fmt.Errorf("failed to add container: %w", err)
		}

		fmt.Printf("✓ Container '%s' added to network '%s' with IP %s\n", containerID, networkID, ipStr)
		return nil
	},
}

var cniRemoveContainerCmd = &cobra.Command{
	Use:   "remove-container <container-id> <network-id>",
	Short: "Remove a container from a network",
	Long: `Detach a container from a network.

Example:
  vpc-cli cni remove-container container-001 network-001`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]
		networkID := args[1]

		ctx := context.Background()
		store := getStore()
		runtime := cni.NewRuntime(store)

		if err := runtime.RemoveFromNetwork(ctx, containerID, networkID); err != nil {
			return fmt.Errorf("failed to remove container: %w", err)
		}

		fmt.Printf("✓ Container '%s' removed from network '%s'\n", containerID, networkID)
		return nil
	},
}

var cniGetStatusCmd = &cobra.Command{
	Use:   "get-status <plugin-name>",
	Short: "Get plugin status",
	Long: `Get the current status of a CNI plugin.

Example:
  vpc-cli cni get-status flannel`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugin := args[0]

		ctx := context.Background()
		store := getStore()
		runtime := cni.NewRuntime(store)

		status, err := runtime.GetPluginStatus(ctx, plugin)
		if err != nil {
			return fmt.Errorf("failed to get plugin status: %w", err)
		}

		fmt.Printf("Plugin: %s\n", status.Name)
		fmt.Printf("Version: %s\n", status.Version)
		fmt.Printf("Status: %s\n", status.Status)
		return nil
	},
}

func init() {
	cniCmd.AddCommand(cniSetupPluginCmd)
	cniCmd.AddCommand(cniAddContainerCmd)
	cniCmd.AddCommand(cniRemoveContainerCmd)
	cniCmd.AddCommand(cniGetStatusCmd)
	rootCmd.AddCommand(cniCmd)
}
