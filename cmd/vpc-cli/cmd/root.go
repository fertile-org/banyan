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
	Use:   "vpc-cli",
	Short: "VPC management CLI for Banyan",
	Long: `vpc-cli is a unified command-line interface for managing VPC networking
in Banyan. It provides commands for networks, IPAM, CNI, security, DNS, and debugging.`,
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
