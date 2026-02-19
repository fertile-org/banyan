package cmd

import (
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
)

// configPath is the default path to the Banyan config file.
var configPath = types.DefaultConfigPath

var rootCmd = &cobra.Command{
	Use:   "banyan-cli",
	Short: "Banyan - Simple container orchestration",
	Long: `banyan-cli is the command-line client for Banyan container orchestration.

Commands:
  init      Initialize CLI configuration (engine host, port, password)
  deploy    Deploy applications from banyan.yaml
  down      Stop and remove deployed services
  status    Show cluster status
  logs      Stream container logs

Quick Start:
  # Initialize CLI config (run once)
  banyan-cli init

  # Deploy an application
  banyan-cli deploy --file banyan.yaml

  # Check status
  banyan-cli status

  # View logs
  banyan-cli logs my-app-web-0`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}
