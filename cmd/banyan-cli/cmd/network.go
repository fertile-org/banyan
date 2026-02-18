package cmd

import (
	"context"
	"fmt"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/network"
	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage VPC networks",
	Long:  "Create, list, inspect, and delete VPC networks",
}

var networkCreateCmd = &cobra.Command{
	Use:   "create [name] [cidr]",
	Short: "Create a new VPC network",
	Long:  "Create a new VPC network with optional name and CIDR",
	Example: `  banyan-cli network create                    # Create with defaults
  banyan-cli network create my-vpc 10.5.0.0/16 # Create with custom CIDR`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config := vpc.NetworkConfig{
			Name: "test-network",
		}
		if len(args) > 0 {
			config.Name = args[0]
		}
		if len(args) > 1 {
			config.CIDR = args[1]
		}

		manager := network.NewManager(getStore())
		net, err := manager.CreateNetwork(context.Background(), config)
		if err != nil {
			return err
		}

		fmt.Printf("✓ Created network:\n")
		fmt.Printf("  ID:         %s\n", net.ID)
		fmt.Printf("  Name:       %s\n", net.Name)
		fmt.Printf("  CIDR:       %s\n", net.CIDR)
		fmt.Printf("  DNSSuffix:  %s\n", net.DNSSuffix)
		fmt.Printf("  VxlanID:    %d\n", net.VxlanID)
		fmt.Printf("  Status:     %s\n", net.Status)
		return nil
	},
}

var networkListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VPC networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		manager := network.NewManager(getStore())
		networks, err := manager.ListNetworks(context.Background())
		if err != nil {
			return err
		}

		fmt.Printf("✓ Found %d networks:\n", len(networks))
		for i, net := range networks {
			fmt.Printf("%d. %s (%s) - %s\n", i+1, net.Name, net.CIDR, net.ID)
		}
		return nil
	},
}

var networkGetCmd = &cobra.Command{
	Use:   "get <network-id>",
	Short: "Get VPC network details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager := network.NewManager(getStore())
		net, err := manager.GetNetwork(context.Background(), args[0])
		if err != nil {
			return err
		}

		fmt.Printf("✓ Network details:\n")
		fmt.Printf("  ID:         %s\n", net.ID)
		fmt.Printf("  Name:       %s\n", net.Name)
		fmt.Printf("  CIDR:       %s\n", net.CIDR)
		fmt.Printf("  DNSSuffix:  %s\n", net.DNSSuffix)
		fmt.Printf("  Status:     %s\n", net.Status)
		return nil
	},
}

var networkDeleteCmd = &cobra.Command{
	Use:   "delete <network-id>",
	Short: "Delete a VPC network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager := network.NewManager(getStore())
		if err := manager.DeleteNetwork(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Println("✓ Network deleted")
		return nil
	},
}

func init() {
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkGetCmd)
	networkCmd.AddCommand(networkDeleteCmd)
	rootCmd.AddCommand(networkCmd)
}
