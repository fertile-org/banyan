package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/spf13/cobra"
)

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "Manage DNS records",
	Long:  "Register, lookup, and manage DNS records for service discovery",
}

var dnsRegisterCmd = &cobra.Command{
	Use:   "register <hostname> <ip>",
	Short: "Register hostname with IP",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := net.ParseIP(args[1])
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", args[1])
		}

		manager := dns.NewManagerWithStore(getStore())
		if err := manager.RegisterHost(context.Background(), args[0], ip); err != nil {
			return err
		}
		fmt.Printf("✓ Registered %s -> %s\n", args[0], ip)
		return nil
	},
}

var dnsLookupCmd = &cobra.Command{
	Use:   "lookup <hostname>",
	Short: "Lookup hostname",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager := dns.NewManagerWithStore(getStore())
		ips, err := manager.LookupHost(context.Background(), args[0])
		if err != nil {
			return err
		}
		fmt.Printf("✓ %s resolves to:\n", args[0])
		for _, ip := range ips {
			fmt.Printf("  - %s\n", ip)
		}
		return nil
	},
}

var dnsUpdateHealthCmd = &cobra.Command{
	Use:   "update-health <hostname> <healthy>",
	Short: "Update host health status (true/false)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		healthy, err := strconv.ParseBool(args[1])
		if err != nil {
			return fmt.Errorf("invalid healthy value: %s (use true/false)", args[1])
		}

		manager := dns.NewManagerWithStore(getStore())
		if err := manager.UpdateHealth(context.Background(), args[0], healthy); err != nil {
			return err
		}
		fmt.Printf("✓ Updated %s health: %v\n", args[0], healthy)
		return nil
	},
}

var dnsUnregisterCmd = &cobra.Command{
	Use:   "unregister <hostname>",
	Short: "Unregister hostname",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager := dns.NewManagerWithStore(getStore())
		if err := manager.UnregisterHost(context.Background(), args[0]); err != nil {
			return err
		}
		fmt.Printf("✓ Unregistered %s\n", args[0])
		return nil
	},
}

var dnsServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Start DNS server for service discovery",
	Long: `Start a DNS server that resolves internal hostnames and forwards external queries.

The server listens on the specified address and resolves hostnames registered
with the DNS manager. External queries are forwarded to the upstream DNS server.

Example:
  vpc-cli dns server --bind 0.0.0.0:53 --zone internal --upstream 8.8.8.8:53`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bindAddr, _ := cmd.Flags().GetString("bind")
		zone, _ := cmd.Flags().GetString("zone")
		upstream, _ := cmd.Flags().GetString("upstream")

		manager := dns.NewManagerWithStore(getStore())
		config := dns.ServerConfig{
			BindAddr:     bindAddr,
			InternalZone: zone,
			UpstreamDNS:  upstream,
		}

		server := dns.NewServer(manager, config)
		if err := server.Start(); err != nil {
			return fmt.Errorf("failed to start DNS server: %w", err)
		}

		fmt.Printf("✓ DNS server started\n")
		fmt.Printf("  Bind address: %s\n", server.BindAddress())
		fmt.Printf("  Internal zone: %s\n", server.InternalZone())
		fmt.Printf("  Upstream DNS: %s\n", server.UpstreamDNS())
		fmt.Println("\nPress Ctrl+C to stop...")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nShutting down DNS server...")
		if err := server.Stop(); err != nil {
			return fmt.Errorf("error stopping DNS server: %w", err)
		}
		fmt.Println("✓ DNS server stopped")
		return nil
	},
}

func init() {
	// Server command flags
	dnsServerCmd.Flags().String("bind", "0.0.0.0:53", "Address to bind DNS server")
	dnsServerCmd.Flags().String("zone", "internal", "Internal zone for resolution")
	dnsServerCmd.Flags().String("upstream", "8.8.8.8:53", "Upstream DNS server for external queries")

	dnsCmd.AddCommand(dnsRegisterCmd)
	dnsCmd.AddCommand(dnsLookupCmd)
	dnsCmd.AddCommand(dnsUpdateHealthCmd)
	dnsCmd.AddCommand(dnsUnregisterCmd)
	dnsCmd.AddCommand(dnsServerCmd)
	rootCmd.AddCommand(dnsCmd)
}
