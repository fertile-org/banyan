package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:          "banyan-agent",
	Short:        "Banyan Agent - Container orchestration worker node",
	SilenceUsage: true,
	Long: `banyan-agent runs on worker nodes and manages containers.

Commands:
  init    Initialize Agent dependencies (containerd, CNI)
  start   Start the Agent (connects to Engine)
  stop    Stop the Agent
  status  Show Agent status

Quick Start:
  banyan-agent init
  banyan-agent start --agent-name my-worker`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
