package cmd

import (
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
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
  ipam      IP Address Management
  dns       DNS management
  debug     Debugging tools

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

// Global store instance (used by VPC commands only)
var globalStore storage.StateStore

// Global security manager instance
var globalSecurityManager vpc.SecurityManager

func init() {
	var stateFile string

	if os.Geteuid() == 0 {
		stateFile = "/var/lib/banyan/vpc/state.json"
	} else {
		stateFile = "/var/lib/banyan/vpc/state.json"
		if _, err := os.Stat(stateFile); os.IsNotExist(err) {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				globalStore = storage.NewMemoryStore()
				return
			}
			stateFile = filepath.Join(homeDir, ".vpc", "state.json")
		}
	}

	store, err := storage.NewMemoryStoreWithFile(stateFile)
	if err != nil {
		globalStore = storage.NewMemoryStore()
		return
	}

	globalStore = store

	resolver := security.NewRuntimeServiceResolver(globalStore)
	globalSecurityManager = security.NewManager(resolver, false)
}

func getStore() storage.StateStore {
	return globalStore
}

func getSecurityManager() vpc.SecurityManager {
	return globalSecurityManager
}
