package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
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

This command generates a WireGuard keypair for authentication and
prompts for the engine host, port, and engine WireGuard public key.
Configuration is written to /etc/banyan/banyan.yaml.

After running init, whitelist the CLI public key on the engine.

Run this once on any machine where you want to use banyan-cli commands
(up, down, status, logs).

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
	if existingCfg.CLI.EngineHost != "" && existingCfg.CLI.WGPublicKey != "" {
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("         Engine: %s:%s\n", existingCfg.CLI.EngineHost, existingCfg.CLI.EnginePort)

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

	// Generate WireGuard keypair
	fmt.Println(styleInfo.Render("\nGenerating WireGuard keypair..."))
	privKey, pubKey, genErr := overlay.GenerateKeyPair()
	if genErr != nil {
		return fmt.Errorf("failed to generate WireGuard keypair: %w", genErr)
	}
	fmt.Printf("  %s WireGuard keypair generated\n", styleOK.Render("[OK]"))
	fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), pubKey)

	hostname, _ := os.Hostname()
	engineHost := existingCfg.CLI.EngineHost
	if engineHost == "" {
		engineHost = "localhost"
	}
	enginePort := existingCfg.CLI.EnginePort
	if enginePort == "" {
		enginePort = "50051"
	}
	cliName := existingCfg.CLI.Name
	if cliName == "" {
		cliName = "cli-" + hostname
	}
	engineWGPubKey := existingCfg.CLI.EngineWGPublicKey

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
				Title("Engine WireGuard public key").
				Description("Displayed during 'banyan-engine init' (optional, enables encrypted tunnel)").
				Value(&engineWGPubKey),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("\nInitialization cancelled.")
			return nil
		}
		return fmt.Errorf("cli config input: %w", err)
	}

	// Write private key to file
	keyPath, writeErr := types.WritePrivateKeyFile(types.DefaultKeysDir, "cli", privKey)
	if writeErr != nil {
		return fmt.Errorf("failed to write private key: %w", writeErr)
	}
	fmt.Printf("  %s Private key: %s\n", styleOK.Render("[OK]"), keyPath)

	// Load existing config to preserve other sections (agent, engine)
	cfg, _ := types.LoadConfig(configPath)
	cfg.CLI = types.CLIConfig{
		EngineHost:        engineHost,
		EnginePort:        enginePort,
		Name:              cliName,
		WGPrivateKeyFile:  keyPath,
		WGPublicKey:       pubKey,
		EngineWGPublicKey: engineWGPubKey,
	}

	if err := types.SaveConfig(configPath, &cfg); err != nil {
		fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
	} else {
		fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
	}

	// Set up WireGuard control tunnel (if engine public key provided)
	if engineWGPubKey != "" {
		myTunnelIP := types.TunnelIPFromPublicKey(pubKey)
		fmt.Printf("\n  %s Setting up WireGuard control tunnel (%s)...\n", styleInfo.Render("[..]"), myTunnelIP)
		engineEndpointWG := engineHost + ":" + fmt.Sprintf("%d", types.ControlTunnelPort)
		if tunnelErr := overlay.SetupControlTunnelExec(types.ControlIfaceCLI, privKey, myTunnelIP, 0); tunnelErr != nil {
			return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure this runs with sudo and wireguard kernel module is loaded)", tunnelErr)
		}
		engineIP := net.ParseIP(types.ControlTunnelEngineIP)
		if peerErr := overlay.AddControlPeerExec(types.ControlIfaceCLI, engineWGPubKey, engineEndpointWG, engineIP); peerErr != nil {
			_ = overlay.CleanupControlTunnelExec(types.ControlIfaceCLI)
			return fmt.Errorf("failed to add engine peer to control tunnel: %w", peerErr)
		}
		fmt.Printf("  %s Control tunnel ready (persists as kernel interface)\n", styleOK.Render("[OK]"))
	}

	// Display next steps for public key auth
	fmt.Println()
	fmt.Println(styleInfo.Render("To whitelist this CLI on the engine:"))
	fmt.Printf("  echo '%s' > /etc/banyan/whitelisted-keys/%s.pub\n", pubKey, cliName)

	fmt.Println()
	fmt.Println(styleDim.Render("========================================"))
	fmt.Println(styleOK.Render("Initialization complete!"))
	fmt.Println()
	fmt.Println(styleInfo.Render("You can now use: banyan-cli up, status, down, logs"))
	return nil
}
