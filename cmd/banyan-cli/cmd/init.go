package cmd

import (
	"fmt"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CLI configuration",
	Long: `Initialize the Banyan CLI configuration.

This command prompts for the engine host, port, and cluster password,
then writes the configuration to /etc/banyan/banyan.yaml.

Run this once on any machine where you want to use banyan-cli commands
(deploy, down, status, logs).

Example:
  sudo banyan-cli init`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan CLI - Initialization")
	fmt.Println("========================================")

	// Check for existing config
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.CLI.EngineHost != "" && existingCfg.Security.Password != "" {
		fmt.Printf("Config already exists at %s\n", configPath)
		fmt.Printf("  Engine: %s:%s (password set)\n", existingCfg.CLI.EngineHost, existingCfg.CLI.EnginePort)
		fmt.Print("\nOverwrite? (y/N): ")
		answer := types.ReadLine()
		if answer != "y" && answer != "Y" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	fmt.Println("\n1. Configuring engine connection...")

	fmt.Print("   Engine host (e.g. 192.168.1.10): ")
	engineHost := types.ReadLine()
	if engineHost == "" {
		return fmt.Errorf("engine host is required")
	}
	fmt.Printf("   [OK] Engine host: %s\n", engineHost)

	fmt.Print("   Engine port (default: 8443): ")
	enginePort := types.ReadLine()
	if enginePort == "" {
		enginePort = "8443"
	}
	fmt.Printf("   [OK] Engine port: %s\n", enginePort)

	fmt.Println("\n2. Configuring authentication...")
	fmt.Print("   Enter cluster password: ")
	password := types.ReadLine()
	if password == "" {
		return fmt.Errorf("password is required for authentication")
	}
	fmt.Println("   [OK] Password set")

	// Load existing config to preserve other sections (agent, engine)
	cfg, _ := types.LoadConfig(configPath)
	cfg.Security = types.SecurityConfig{
		AuthType: "password",
		Password: password,
	}
	cfg.CLI = types.CLIConfig{
		EngineHost: engineHost,
		EnginePort: enginePort,
	}

	if err := types.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n   [OK] Config saved to %s\n", configPath)
	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Printf("\nYou can now use: banyan-cli deploy, status, down, logs\n")
	return nil
}
