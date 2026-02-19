package cmd

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

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
	fmt.Println(styleTitle.Render("Banyan CLI - Initialization"))
	fmt.Println(styleDim.Render("========================================"))

	// Check for existing config
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.CLI.EngineHost != "" && existingCfg.Security.Password != "" {
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("         Engine: %s:%s (password set)\n", existingCfg.CLI.EngineHost, existingCfg.CLI.EnginePort)

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

	engineHost := "localhost"
	enginePort := "50051"
	var password string

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
				Title("Banyan cluster password").
				Description("Must match the engine password to connect").
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
