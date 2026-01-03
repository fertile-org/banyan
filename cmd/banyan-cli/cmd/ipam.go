package cmd

import (
	"context"
	"fmt"
	"net"

	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/spf13/cobra"
)

var ipamCmd = &cobra.Command{
	Use:   "ipam",
	Short: "Manage IP address allocation",
	Long:  "Allocate and manage IP addresses and host subnets",
}

var ipamAllocateSubnetCmd = &cobra.Command{
	Use:   "allocate-subnet <host-id>",
	Short: "Allocate /24 subnet for host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
		if err != nil {
			return err
		}

		subnet, err := manager.AllocateHostSubnet(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("✓ Allocated subnet for %s: %s\n", args[0], subnet.String())
		return nil
	},
}

var ipamAllocateIPCmd = &cobra.Command{
	Use:   "allocate-ip <host-id>",
	Short: "Allocate IP from host's subnet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
		if err != nil {
			return err
		}

		subnet, err := manager.GetHostSubnet(context.Background(), args[0])
		if err != nil {
			return err
		}

		ip, err := manager.AllocateIP(context.Background(), subnet)
		if err != nil {
			return err
		}
		fmt.Printf("✓ Allocated IP: %s (from subnet %s)\n", ip.String(), subnet.String())
		return nil
	},
}

var ipamReleaseIPCmd = &cobra.Command{
	Use:   "release-ip <ip-address>",
	Short: "Release allocated IP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := net.ParseIP(args[0])
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", args[0])
		}

		manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
		if err != nil {
			return err
		}

		if err := manager.ReleaseIP(context.Background(), ip); err != nil {
			return err
		}
		fmt.Printf("✓ Released IP: %s\n", ip.String())
		return nil
	},
}

var ipamRenewLeaseCmd = &cobra.Command{
	Use:   "renew-lease <host-id>",
	Short: "Renew subnet lease for host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
		if err != nil {
			return err
		}

		if err := manager.RenewLease(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Printf("✓ Renewed lease for %s\n", args[0])
		return nil
	},
}

var ipamGetSubnetCmd = &cobra.Command{
	Use:   "get-subnet <host-id>",
	Short: "Get host's allocated subnet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := ipam.NewManager(getStore(), "10.0.0.0/16")
		if err != nil {
			return err
		}

		subnet, err := manager.GetHostSubnet(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("✓ Host %s subnet: %s\n", args[0], subnet.String())
		return nil
	},
}

func init() {
	ipamCmd.AddCommand(ipamAllocateSubnetCmd)
	ipamCmd.AddCommand(ipamAllocateIPCmd)
	ipamCmd.AddCommand(ipamReleaseIPCmd)
	ipamCmd.AddCommand(ipamRenewLeaseCmd)
	ipamCmd.AddCommand(ipamGetSubnetCmd)
	rootCmd.AddCommand(ipamCmd)
}
