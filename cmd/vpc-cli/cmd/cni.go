package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

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

		ctx := context.Background()
		store := getStore()
		runtime := cni.NewRuntime(store)

		// Handle Flannel specially - start daemon
		if plugin == "flannel" {
			fmt.Println("🚀 Starting Flannel daemon...")
			if err := startFlannelDaemon(); err != nil {
				return fmt.Errorf("failed to start Flannel daemon: %w", err)
			}

			config := []byte(`{
	"name": "flannel",
	"type": "flannel",
	"subnetFile": "/run/flannel/subnet.env",
	"dataDir": "/var/lib/cni/flannel",
	"delegate": {
		"hairpinMode": true,
		"isDefaultGateway": true
	}
}`)
			if err := runtime.SetupPlugin(ctx, plugin, config); err != nil {
				return fmt.Errorf("failed to setup plugin: %w", err)
			}

			fmt.Println("✓ Flannel daemon started")
			fmt.Printf("✓ Plugin '%s' configured successfully\n", plugin)
			return nil
		}

		// Handle other plugins (e.g., calico)
		var config []byte
		switch plugin {
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

func startFlannelDaemon() error {
	// Check if already running
	if isFlanneldRunning() {
		fmt.Println("✓ Flannel daemon already running")
		return nil
	}

	// Create subnet configuration file
	if err := createFlannelSubnetConfig(); err != nil {
		return fmt.Errorf("failed to create subnet config: %w", err)
	}

	// Start flanneld in background
	cmd := exec.Command("flanneld",
		"--iface=eth0", // Use default interface
		"--subnet-file=/run/flannel/subnet.env",
		"--ip-masq",
	)

	// Create log file
	logFile, err := os.OpenFile("/var/log/flanneld.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start flanneld: %w", err)
	}

	// Save PID for management
	pidFile := "/var/run/flanneld.pid"
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to save PID: %w", err)
	}

	logFile.Close()

	// Wait a bit for daemon to initialize
	fmt.Println("Waiting for Flannel daemon to initialize...")
	time.Sleep(2 * time.Second)

	// Verify it's running
	if !isFlanneldRunning() {
		return fmt.Errorf("flanneld failed to start (check /var/log/flanneld.log)")
	}

	return nil
}

func isFlanneldRunning() bool {
	// Check if PID file exists and process is running
	pidFile := "/var/run/flanneld.pid"
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	pid := string(data)
	// Check if process exists
	err = exec.Command("kill", "-0", pid).Run()
	return err == nil
}

func createFlannelSubnetConfig() error {
	// Create /run/flannel directory
	if err := os.MkdirAll("/run/flannel", 0755); err != nil {
		return fmt.Errorf("failed to create /run/flannel: %w", err)
	}

	// Create subnet configuration
	// This tells Flannel to use 10.0.0.0/16 network with /24 subnets
	config := `FLANNEL_NETWORK=10.0.0.0/16
FLANNEL_SUBNET=10.0.1.0/24
FLANNEL_MTU=1450
FLANNEL_IPMASQ=true
`

	if err := os.WriteFile("/run/flannel/subnet.env", []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write subnet config: %w", err)
	}

	return nil
}

func init() {
	cniCmd.AddCommand(cniSetupPluginCmd)
	cniCmd.AddCommand(cniAddContainerCmd)
	cniCmd.AddCommand(cniRemoveContainerCmd)
	cniCmd.AddCommand(cniGetStatusCmd)
	rootCmd.AddCommand(cniCmd)
}
