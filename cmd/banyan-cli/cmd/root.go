package cmd

import (
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/types"
)

// configPath is the default path to the Banyan config file.
var configPath = types.DefaultConfigPath

var rootCmd = &cobra.Command{
	Use:          "banyan-cli",
	Short:        "Banyan - Simple container orchestration",
	SilenceUsage: true,
	Long: `banyan-cli is the command-line client for Banyan container orchestration.

Commands:
  init         Initialize CLI configuration (engine host, port, WireGuard keys)
  login        Re-establish WireGuard tunnel after restart
  up           Deploy applications from banyan.yaml (alias: deploy)
  down         Stop and remove deployed services
  engine       Show engine status and cluster summary
  agent        List agents or show agent detail
  deployment   List deployments or show deployment detail
  container    List containers or show container detail
  events       List recent cluster events
  logs         Stream container logs

Quick Start:
  # Initialize CLI config (run once)
  banyan-cli init

  # Deploy an application
  banyan-cli up --file banyan.yaml

  # Check cluster status
  banyan-cli engine
  banyan-cli agent
  banyan-cli deployment

  # View logs
  banyan-cli logs my-app-web-0`,
}

// Execute runs the root command
func Execute() error {
	logging.Setup(nil)
	return rootCmd.Execute()
}
