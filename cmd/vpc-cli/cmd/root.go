package cmd

import (
	"os"
	"path/filepath"

	"github.com/fertile/banyan/pkg/vpc/storage"
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

func init() {
	// Use persistent storage in ~/.vpc/state.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to in-memory only
		globalStore = storage.NewMemoryStore()
		return
	}

	stateFile := filepath.Join(homeDir, ".vpc", "state.json")
	store, err := storage.NewMemoryStoreWithFile(stateFile)
	if err != nil {
		// Fallback to in-memory only
		globalStore = storage.NewMemoryStore()
		return
	}

	globalStore = store
}

// getStore returns the global store instance
func getStore() storage.StateStore {
	return globalStore
}
