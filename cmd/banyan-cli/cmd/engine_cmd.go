package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Show engine status and cluster summary",
	Long: `Show the Banyan engine status, resource usage, and cluster summary.

Examples:
  banyan-cli engine
  banyan-cli engine -o json`,
	RunE: runEngine,
}

func init() {
	rootCmd.AddCommand(engineCmd)
	engineCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "Output format (json)")
}

func runEngine(_ *cobra.Command, _ []string) error {
	data, err := fetchClusterData()
	if err != nil {
		return err
	}

	if outputFormat == "json" {
		return printJSON(data.Engine)
	}

	e := data.Engine
	s := data.Summary

	fmt.Println("Engine")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Printf("  Status:    %s\n", e.Status)
	uptime := time.Since(e.StartedAt)
	fmt.Printf("  Uptime:    %s\n", humanDuration(uptime))
	fmt.Printf("  CPU:       %s (%d cores)\n", formatPercent(e.CPU), e.CPUCores)
	fmt.Printf("  Memory:    %s / %s\n", formatBytes(e.MemUsed), formatBytes(e.MemTotal))
	fmt.Printf("  Disk:      %s / %s\n", formatBytes(e.DiskUsed), formatBytes(e.DiskTotal))

	fmt.Println()
	fmt.Println("Cluster Summary")
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("  Agents:       %d/%d connected\n", s.ConnectedAgents, s.TotalAgents)
	fmt.Printf("  Deployments:  %d/%d running\n", s.RunningDeployments, s.TotalDeployments)
	fmt.Printf("  Containers:   %d/%d healthy\n", s.HealthyContainers, s.TotalContainers)
	fmt.Printf("  Tasks:        %d completed, %d failed\n", s.CompletedTasks, s.FailedTasks)

	return nil
}
