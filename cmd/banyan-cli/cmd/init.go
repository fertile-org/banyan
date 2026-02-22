package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/agent"
	"github.com/fertile-org/banyan/pkg/types"
)

// TUI styles for the init wizard.
var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

var cliInitPassword string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CLI configuration",
	Long: `Initialize the Banyan CLI configuration.

This command prompts for the engine host, port, and cluster password,
then exchanges the password for an auth token and writes the
configuration to /etc/banyan/banyan.yaml.

The engine must be running for this command to succeed.

Run this once on any machine where you want to use banyan-cli commands
(deploy, down, status, logs).

Example:
  sudo banyan-cli init
  sudo banyan-cli init --password "my-cluster-secret"`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&cliInitPassword, "password", "", "Cluster password (non-interactive mode)")
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println(styleTitle.Render("Banyan CLI - Initialization"))
	fmt.Println(styleDim.Render("========================================"))

	// Check for existing config
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.CLI.EngineHost != "" && existingCfg.CLI.AuthToken != "" {
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("         Engine: %s:%s (token set)\n", existingCfg.CLI.EngineHost, existingCfg.CLI.EnginePort)

		var overwrite bool
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Overwrite existing configuration?").
					Value(&overwrite),
			),
		)
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nInitialization cancelled.")
				return nil
			}
			return fmt.Errorf("confirm prompt: %w", err)
		}
		if !overwrite {
			fmt.Println("Aborted.")
			return nil
		}
	}

	hostname, _ := os.Hostname()
	engineHost := "localhost"
	enginePort := "50051"
	cliName := "cli-" + hostname
	var password string

	if cliInitPassword != "" && existingCfg.CLI.EngineHost != "" {
		// Non-interactive: password via flag, connection details from config.
		password = cliInitPassword
		engineHost = existingCfg.CLI.EngineHost
		enginePort = existingCfg.CLI.EnginePort
		if enginePort == "" {
			enginePort = "50051"
		}
		if existingCfg.CLI.Name != "" {
			cliName = existingCfg.CLI.Name
		}
	} else {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Engine host").
					Description("Hostname or IP of the Banyan engine").
					Value(&engineHost),
				huh.NewInput().
					Title("Engine gRPC port").
					Value(&enginePort),
				huh.NewInput().
					Title("CLI name").
					Description("Unique name for this CLI client").
					Value(&cliName),
				huh.NewInput().
					Title("Banyan cluster password").
					Description("Used once to obtain an auth token from the engine").
					EchoMode(huh.EchoModePassword).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("password is required")
						}
						return nil
					}).
					Value(&password),
			),
		)
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nInitialization cancelled.")
				return nil
			}
			return fmt.Errorf("cli config input: %w", err)
		}
	}

	// Connect to engine and exchange password for token
	engineAddr := fmt.Sprintf("%s:%s", engineHost, enginePort)
	fmt.Printf("  %s Connecting to engine at %s...\n", styleInfo.Render("[..]"), engineAddr)

	client, connErr := agent.NewEngineClientWithPassword(engineAddr, password)
	if connErr != nil {
		return fmt.Errorf("failed to connect to engine: %w", connErr)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	token, tokenErr := client.ExchangeToken(ctx, cliName, "cli")
	if tokenErr != nil {
		fmt.Printf("  %s Token exchange failed: %v\n", styleWarn.Render("[FAIL]"), tokenErr)
		return fmt.Errorf("token exchange failed: %w", tokenErr)
	}

	fmt.Printf("  %s Token obtained from engine\n", styleOK.Render("[OK]"))

	// Load existing config to preserve other sections (agent, engine)
	cfg, _ := types.LoadConfig(configPath)
	cfg.CLI = types.CLIConfig{
		EngineHost: engineHost,
		EnginePort: enginePort,
		AuthToken:  token,
		Name:       cliName,
	}

	if err := types.SaveConfig(configPath, &cfg); err != nil {
		fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
	} else {
		fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
	}

	fmt.Println()
	fmt.Println(styleDim.Render("========================================"))
	fmt.Println(styleOK.Render("Initialization complete!"))
	fmt.Println()
	fmt.Println(styleInfo.Render("You can now use: banyan-cli deploy, status, down, logs"))
	return nil
}
