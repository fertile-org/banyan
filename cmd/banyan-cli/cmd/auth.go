package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/agent"
	"github.com/fertile-org/banyan/pkg/types"
)

var cliAuthPassword string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Re-authenticate with the engine",
	Long: `Re-authenticate this CLI client with the engine.

Reads engine connection details from the existing config and prompts
only for the cluster password. Use this when a token has been revoked
or the engine has been re-initialized.

If no config exists, run 'banyan-cli init' first.

Example:
  sudo banyan-cli auth
  sudo banyan-cli auth --password "my-cluster-secret"`,
	RunE: runCLIAuth,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.Flags().StringVar(&cliAuthPassword, "password", "", "Cluster password (non-interactive mode)")
}

func runCLIAuth(cmd *cobra.Command, args []string) error {
	fmt.Println(styleTitle.Render("Banyan CLI - Re-authenticate"))
	fmt.Println(styleDim.Render("========================================"))

	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := validateCLIConfig(&cfg); err != nil {
		return err
	}

	// Derive CLI name: config > hostname
	cliName := cfg.CLI.Name
	if cliName == "" {
		hostname, _ := os.Hostname()
		cliName = "cli-" + hostname
	}

	var password string
	if cliAuthPassword != "" {
		password = cliAuthPassword
	} else {
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Cluster password").
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
				fmt.Println("\nAuthentication cancelled.")
				return nil
			}
			return fmt.Errorf("password input: %w", err)
		}
	}

	engineAddr := fmt.Sprintf("%s:%s", cfg.CLI.EngineHost, cfg.CLI.EnginePort)
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

	cfg.CLI.AuthToken = token
	cfg.CLI.Name = cliName
	if err := types.SaveConfig(configPath, &cfg); err != nil {
		fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
	return nil
}

// validateCLIConfig checks that the CLI config has the required fields
// for re-authentication.
func validateCLIConfig(cfg *types.BanyanConfig) error {
	if cfg.CLI.EngineHost == "" || cfg.CLI.EnginePort == "" {
		return fmt.Errorf("no CLI config found. Run 'banyan-cli init' first")
	}
	return nil
}
