package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:          "banyan-engine",
	Short:        "Banyan Engine - Container orchestration control plane",
	SilenceUsage: true,
	Long: `banyan-engine is the control plane for Banyan container orchestration.

Commands:
  init    Initialize Engine dependencies (etcd, directories)
  start   Start the Engine (etcd, registry, HTTP API, orchestration)
  stop    Stop the Engine
  status  Show Engine status

Quick Start:
  banyan-engine init
  banyan-engine start`,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
