package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/cmd/banyan-cli/dashboard"
)

var agentCmd = &cobra.Command{
	Use:   "agent [NAME]",
	Short: "List agents or show agent detail",
	Long: `List all agents in the cluster, or show detailed info for a specific agent.

Examples:
  banyan-cli agent
  banyan-cli agent worker-1
  banyan-cli agent -o json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAgent,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format (json)")
}

func runAgent(_ *cobra.Command, args []string) error {
	data, err := fetchClusterData()
	if err != nil {
		return err
	}

	if len(args) == 1 {
		return showAgentDetail(data.Agents, args[0])
	}
	return listAgents(data.Agents)
}

func listAgents(agents []dashboard.AgentData) error {
	if outputFormat == "json" {
		return printJSON(agents)
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered")
		return nil
	}

	// Table header
	fmt.Printf("%-20s %-12s %10s %8s %8s %s\n",
		"NAME", "STATUS", "CONTAINERS", "CPU", "MEM", "TAGS")
	fmt.Println(strings.Repeat("-", 75))

	for i := range agents {
		a := &agents[i]
		tags := ""
		if len(a.Tags) > 0 {
			tags = strings.Join(a.Tags, ",")
		}
		memPct := ""
		if a.MemTotal > 0 {
			memPct = formatPercent(float64(a.MemUsed) / float64(a.MemTotal))
		}
		fmt.Printf("%-20s %-12s %10d %8s %8s %s\n",
			a.Name, a.Status, a.ContainerCount,
			formatPercent(a.CPU), memPct, tags)
	}

	return nil
}

func showAgentDetail(agents []dashboard.AgentData, name string) error {
	var agent *dashboard.AgentData
	for i := range agents {
		if agents[i].Name == name {
			agent = &agents[i]
			break
		}
	}
	if agent == nil {
		return fmt.Errorf("agent %q not found", name)
	}

	if outputFormat == "json" {
		return printJSON(agent)
	}

	fmt.Printf("Agent: %s\n", agent.Name)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Status:       %s\n", agent.Status)
	fmt.Printf("  API Address:  %s\n", agent.APIAddress)
	fmt.Printf("  VPC Subnet:   %s\n", agent.VPCSubnet)
	fmt.Printf("  Tags:         %s\n", strings.Join(agent.Tags, ", "))
	fmt.Printf("  Containers:   %d\n", agent.ContainerCount)
	fmt.Printf("  Last Seen:    %s ago\n", humanDuration(time.Since(agent.LastSeen)))
	fmt.Printf("  Created:      %s ago\n", humanDuration(time.Since(agent.CreatedAt)))

	fmt.Println()
	fmt.Println("Resources")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("  CPU:     %s (%d cores)\n", formatPercent(agent.CPU), agent.CPUCores)
	fmt.Printf("  Memory:  %s / %s\n", formatBytes(agent.MemUsed), formatBytes(agent.MemTotal))
	fmt.Printf("  Disk:    %s / %s\n", formatBytes(agent.DiskUsed), formatBytes(agent.DiskTotal))

	return nil
}
