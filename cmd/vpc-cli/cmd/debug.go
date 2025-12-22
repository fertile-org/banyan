package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/fertile-org/banyan/pkg/vpc/debug"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Network debugging utilities",
	Long:  "Debug network connectivity, trace connections, and inspect iptables rules",
}

var debugTraceCmd = &cobra.Command{
	Use:   "trace <from-ip> <to-ip> <port>",
	Short: "Trace network path between two IPs",
	Long: `Trace the network path between two IP addresses and identify any blocking rules.

This command simulates or performs actual network path tracing to help diagnose
connectivity issues between containers or between a container and external services.

Examples:
  vpc-cli debug trace 10.0.1.5 10.0.1.10 5432
  vpc-cli debug trace 10.0.1.5 8.8.8.8 443
  vpc-cli debug trace 10.0.1.5 10.0.2.10 22`,
	Args: cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		fromIP := net.ParseIP(args[0])
		if fromIP == nil {
			return fmt.Errorf("invalid from IP: %s", args[0])
		}

		toIP := net.ParseIP(args[1])
		if toIP == nil {
			return fmt.Errorf("invalid to IP: %s", args[1])
		}

		var port int
		if _, err := fmt.Sscanf(args[2], "%d", &port); err != nil {
			return fmt.Errorf("invalid port: %s", args[2])
		}

		if port < 0 || port > 65535 {
			return fmt.Errorf("port must be between 0 and 65535")
		}

		manager := debug.NewManagerWithStore(getStore())
		result, err := manager.TraceConnection(context.Background(), fromIP, toIP, port)
		if err != nil {
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		// Human-readable output
		fmt.Printf("Trace: %s -> %s:%d\n", result.FromIP, result.ToIP, result.Port)
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Reachable: %v\n", result.Reachable)
		fmt.Println()

		if len(result.Hops) > 0 {
			fmt.Println("Hops:")
			for i, hop := range result.Hops {
				fmt.Printf("  %d. [%s] %s", i+1, hop.Type, hop.Address)
				if hop.Latency != "" {
					fmt.Printf(" (%s)", hop.Latency)
				}
				fmt.Println()
			}
			fmt.Println()
		}

		if result.Status == "blocked" {
			fmt.Printf("Blocked by: %s\n", result.BlockedBy)
			if result.BlockedByDetails != "" {
				fmt.Printf("Details: %s\n", result.BlockedByDetails)
			}
		} else if result.Status == "allowed" {
			fmt.Printf("Allowed by: %s\n", result.AllowedBy)
			if result.Latency != "" {
				fmt.Printf("Total latency: %s\n", result.Latency)
			}
		}

		if result.Error != "" {
			fmt.Printf("Error: %s\n", result.Error)
		}

		return nil
	},
}

var debugConnectivityCmd = &cobra.Command{
	Use:   "connectivity <container-id>",
	Short: "Check container network connectivity",
	Long: `Check the network connectivity status of a container.

This command diagnoses network health including IP assignment, gateway reachability,
DNS configuration, and internet access.

Examples:
  vpc-cli debug connectivity container-001
  vpc-cli debug connectivity my-web-app`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		containerID := args[0]

		manager := debug.NewManagerWithStore(getStore())
		result, err := manager.CheckConnectivity(context.Background(), containerID)
		if err != nil {
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		// Human-readable output
		fmt.Printf("Container: %s\n", result.ContainerID)
		fmt.Printf("Status: %s\n", result.Status)
		fmt.Printf("Has Network: %v\n", result.HasNetwork)
		fmt.Println()

		if result.IP != nil {
			fmt.Printf("IP Address: %s\n", result.IP)
		}
		if result.Gateway != nil {
			fmt.Printf("Gateway: %s\n", result.Gateway)
		}
		if len(result.DNS) > 0 {
			dnsStrs := make([]string, len(result.DNS))
			for i, dns := range result.DNS {
				dnsStrs[i] = dns.String()
			}
			fmt.Printf("DNS Servers: %s\n", strings.Join(dnsStrs, ", "))
		}
		if result.DefaultRoute != "" {
			fmt.Printf("Default Route: %s\n", result.DefaultRoute)
		}
		fmt.Println()

		fmt.Printf("Internet Access: %v\n", result.InternetAccess)
		fmt.Printf("External Ping: %v\n", result.ExternalPing)
		fmt.Printf("Internal Ping: %v\n", result.InternalPing)

		if len(result.Errors) > 0 {
			fmt.Println()
			fmt.Println("Errors:")
			for _, errMsg := range result.Errors {
				fmt.Printf("  - %s\n", errMsg)
			}
		}

		if result.Error != "" {
			fmt.Printf("\nError: %s\n", result.Error)
		}

		return nil
	},
}

var debugIptablesCmd = &cobra.Command{
	Use:   "iptables <ip>",
	Short: "Show iptables rules for an IP",
	Long: `Display iptables rules that affect traffic to/from the specified IP.

This command shows both filter and NAT table rules relevant to the IP address,
helping diagnose firewall issues.

Examples:
  vpc-cli debug iptables 10.0.1.5
  vpc-cli debug iptables 10.0.2.10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := net.ParseIP(args[0])
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", args[0])
		}

		manager := debug.NewManagerWithStore(getStore())
		rules, err := manager.GetIPTablesRules(context.Background(), ip)
		if err != nil {
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			data, err := json.MarshalIndent(rules, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		// Human-readable output
		fmt.Printf("IPTables rules for %s:\n\n", ip)

		if len(rules) == 0 {
			fmt.Println("No rules found.")
			return nil
		}

		for _, rule := range rules {
			fmt.Printf("  %s\n", rule)
		}

		return nil
	},
}

var debugPingCmd = &cobra.Command{
	Use:   "ping <ip>",
	Short: "Ping an IP address to check reachability",
	Long: `Perform a real ping to check if an IP address is reachable.

This command requires network access and will actually attempt to ping the target.

Examples:
  vpc-cli debug ping 10.0.1.5
  vpc-cli debug ping 8.8.8.8`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := net.ParseIP(args[0])
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", args[0])
		}

		manager := debug.NewManagerWithStore(getStore())
		reachable, latency, err := manager.Ping(context.Background(), ip)
		if err != nil {
			return err
		}

		if reachable {
			fmt.Printf("✓ %s is reachable (latency: %s)\n", ip, latency)
		} else {
			fmt.Printf("✗ %s is not reachable\n", ip)
		}

		return nil
	},
}

var debugPortCmd = &cobra.Command{
	Use:   "port <ip> <port>",
	Short: "Check if a port is open on an IP",
	Long: `Check if a specific TCP port is reachable on an IP address.

This command attempts to establish a TCP connection to verify port accessibility.

Examples:
  vpc-cli debug port 10.0.1.5 5432
  vpc-cli debug port 10.0.2.10 80`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ip := net.ParseIP(args[0])
		if ip == nil {
			return fmt.Errorf("invalid IP address: %s", args[0])
		}

		var port int
		if _, err := fmt.Sscanf(args[1], "%d", &port); err != nil {
			return fmt.Errorf("invalid port: %s", args[1])
		}

		if port < 1 || port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}

		manager := debug.NewManagerWithStore(getStore())
		open, err := manager.CheckPort(context.Background(), ip, port)
		if err != nil {
			return err
		}

		if open {
			fmt.Printf("✓ Port %d on %s is open\n", port, ip)
		} else {
			fmt.Printf("✗ Port %d on %s is closed or unreachable\n", port, ip)
		}

		return nil
	},
}

func init() {
	// Add JSON output flag to commands
	debugTraceCmd.Flags().Bool("json", false, "Output in JSON format")
	debugConnectivityCmd.Flags().Bool("json", false, "Output in JSON format")
	debugIptablesCmd.Flags().Bool("json", false, "Output in JSON format")

	debugCmd.AddCommand(debugTraceCmd)
	debugCmd.AddCommand(debugConnectivityCmd)
	debugCmd.AddCommand(debugIptablesCmd)
	debugCmd.AddCommand(debugPingCmd)
	debugCmd.AddCommand(debugPortCmd)
	rootCmd.AddCommand(debugCmd)
}
