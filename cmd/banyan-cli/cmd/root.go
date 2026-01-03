package cmd

import (
	"os"
	"path/filepath"

	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "banyan-cli",
	Short: "Banyan - Simple container orchestration",
	Long: `banyan-cli is the unified command-line interface for Banyan container orchestration.

Commands:
  engine    Manage the Banyan Engine (control plane)
  agent     Manage the Banyan Agent (worker node)
  deploy    Deploy applications from banyan.yaml
  ipam      IP Address Management
  dns       DNS management
  debug     Debugging tools

Quick Start:
  # On the control plane node
  banyan-cli engine init
  banyan-cli engine start

  # On worker nodes
  banyan-cli agent init
  banyan-cli agent start --etcd http://engine-host:2379

  # Deploy an application
  banyan-cli deploy --file banyan.yaml`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// Global store instance
var globalStore storage.StateStore

// Global security manager instance
var globalSecurityManager vpc.SecurityManager

func init() {
	var stateFile string

	// Use shared storage location for consistency
	// Root operations (like setup-plugin) and user reads use the same file
	if os.Geteuid() == 0 {
		// Running as root: use system-wide location
		stateFile = "/var/lib/banyan/vpc/state.json"
	} else {
		// Running as user: try system location first, fallback to user home
		stateFile = "/var/lib/banyan/vpc/state.json"
		if _, err := os.Stat(stateFile); os.IsNotExist(err) {
			// System file doesn't exist, use user's home directory
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
		// Fallback to in-memory only
		globalStore = storage.NewMemoryStore()
		return
	}

	globalStore = store

	// Initialize security manager with resolver
	resolver := security.NewRuntimeServiceResolver(globalStore)
	globalSecurityManager = security.NewManager(resolver, false) // Not dry-run
}

// getStore returns the global store instance
func getStore() storage.StateStore {
	return globalStore
}

// getSecurityManager returns the global security manager instance
func getSecurityManager() vpc.SecurityManager {
	return globalSecurityManager
}
