package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

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
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// Function variables for tunnel operations, enabling test mocking.
var (
	setupControlTunnelFn   = overlay.SetupControlTunnelExec
	addControlPeerFn       = overlay.AddControlPeerExec
	cleanupControlTunnelFn = overlay.CleanupControlTunnelExec
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
(up, down, logs, dashboard, engine, agent, deployment, container, events).

Example:
  sudo banyan-cli init`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

// cliInitInputs holds the inputs collected from the init wizard.
type cliInitInputs struct {
	EngineHost     string
	EnginePort     string
	CLIName        string
	EngineWGPubKey string
	PrivKey        string
	PubKey         string
	KeysDir        string
	Engines        []types.EngineEndpoint
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
				Description("Required — displayed during 'banyan-engine init'").
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

	// --- Additional engine endpoints for HA ---
	var engines []types.EngineEndpoint
	if len(existingCfg.CLI.Engines) > 0 {
		engines = existingCfg.CLI.Engines
		fmt.Printf("  %s HA engines already configured:\n", styleOK.Render("[OK]"))
		for i, eng := range engines {
			fmt.Printf("    %d. %s\n", i+1, eng.Address)
		}
	} else {
		primaryAddr := engineHost + ":" + enginePort
		engines = collectCLIEngineEndpoints(primaryAddr, engineWGPubKey)
		if engines != nil {
			fmt.Printf("  %s HA engines configured:\n", styleOK.Render("[OK]"))
			for i, eng := range engines {
				label := ""
				if i == 0 {
					label = " (primary)"
				}
				fmt.Printf("    %d. %s%s\n", i+1, eng.Address, label)
			}
		}
	}

	return applyCLIInit(&cliInitInputs{
		EngineHost:     engineHost,
		EnginePort:     enginePort,
		CLIName:        cliName,
		EngineWGPubKey: engineWGPubKey,
		PrivKey:        privKey,
		PubKey:         pubKey,
		KeysDir:        types.DefaultKeysDir,
		Engines:        engines,
	})
}

// applyCLIInit validates inputs, writes keys and config, and sets up the WireGuard tunnel.
func applyCLIInit(inputs *cliInitInputs) error {
	if inputs.EngineWGPubKey == "" {
		return fmt.Errorf("engine WireGuard public key is required. Get it from the engine operator (displayed during 'banyan-engine init')")
	}

	// Write private key to file
	keyPath, writeErr := types.WritePrivateKeyFile(inputs.KeysDir, "cli", inputs.PrivKey)
	if writeErr != nil {
		return fmt.Errorf("failed to write private key: %w", writeErr)
	}
	fmt.Printf("  %s Private key: %s\n", styleOK.Render("[OK]"), keyPath)

	// Load existing config to preserve other sections (agent, engine)
	cfg, _ := types.LoadConfig(configPath)
	cfg.CLI = types.CLIConfig{
		EngineHost:        inputs.EngineHost,
		EnginePort:        inputs.EnginePort,
		Name:              inputs.CLIName,
		WGPrivateKeyFile:  keyPath,
		WGPublicKey:       inputs.PubKey,
		EngineWGPublicKey: inputs.EngineWGPubKey,
		Engines:           inputs.Engines,
	}

	if err := types.SaveConfig(configPath, &cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)

	// Set up WireGuard control tunnel
	if err := setupCLITunnel(inputs); err != nil {
		return err
	}

	// Display next steps for public key auth
	fmt.Println()
	fmt.Println(styleInfo.Render("To whitelist this CLI on the engine:"))
	fmt.Printf("  echo '%s' > /etc/banyan/whitelisted-keys/%s.pub\n", inputs.PubKey, inputs.CLIName)

	fmt.Println()
	fmt.Println(styleDim.Render("========================================"))
	fmt.Println(styleOK.Render("Initialization complete!"))
	fmt.Println()
	fmt.Println(styleInfo.Render("You can now use: banyan-cli up, status, down, logs"))
	return nil
}

// setupCLITunnel creates the WireGuard control tunnel to all configured engines.
func setupCLITunnel(inputs *cliInitInputs) error {
	myTunnelIP := types.TunnelIPFromPublicKey(inputs.PubKey)
	fmt.Printf("\n  %s Setting up WireGuard control tunnel (%s)...\n", styleInfo.Render("[..]"), myTunnelIP)
	if tunnelErr := setupControlTunnelFn(types.ControlIfaceCLI, inputs.PrivKey, myTunnelIP, 0); tunnelErr != nil {
		return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure this runs with sudo and wireguard kernel module is loaded)", tunnelErr)
	}

	// Add all engine peers — each engine has its own WG key and derived tunnel IP
	engines := inputs.Engines
	if len(engines) == 0 && inputs.EngineWGPubKey != "" {
		// Single-engine: create a 1-entry list from old fields
		engines = []types.EngineEndpoint{
			{Address: inputs.EngineHost + ":50051", WGPublicKey: inputs.EngineWGPubKey},
		}
	}
	for _, eng := range engines {
		engineHost, _, _ := net.SplitHostPort(eng.Address)
		engineEndpointWG := engineHost + ":" + fmt.Sprintf("%d", types.ControlTunnelPort)
		engineTunnelIP := types.TunnelIPFromPublicKey(eng.WGPublicKey)
		if peerErr := addControlPeerFn(types.ControlIfaceCLI, eng.WGPublicKey, engineEndpointWG, engineTunnelIP); peerErr != nil {
			_ = cleanupControlTunnelFn(types.ControlIfaceCLI)
			return fmt.Errorf("failed to add engine peer to control tunnel: %w", peerErr)
		}
	}

	fmt.Printf("  %s Control tunnel ready (persists as kernel interface)\n", styleOK.Render("[OK]"))
	return nil
}

// collectCLIEngineEndpoints prompts the user to add engine endpoints (address + WG key)
// one by one for HA failover. Returns nil if the user doesn't want HA.
func collectCLIEngineEndpoints(primaryAddr, primaryWGKey string) []types.EngineEndpoint {
	var addMore bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Add additional engine endpoints for high availability?").
				Description("For single-engine setups, choose No").
				Value(&addMore),
		),
	)
	if err := confirmForm.Run(); err != nil || !addMore {
		return nil
	}

	engines := []types.EngineEndpoint{
		{Address: primaryAddr, WGPublicKey: primaryWGKey},
	}

	for {
		var address, wgKey string
		epForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("Engine #%d — address", len(engines)+1)).
					Description("host:port (or leave empty to finish)").
					Value(&address),
			),
		)
		if err := epForm.Run(); err != nil {
			break
		}
		address = strings.TrimSpace(address)
		if address == "" {
			break
		}

		keyForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title(fmt.Sprintf("Engine #%d — WireGuard public key", len(engines)+1)).
					Description("Displayed during 'banyan-engine init' on that server").
					Value(&wgKey),
			),
		)
		if err := keyForm.Run(); err != nil {
			break
		}
		wgKey = strings.TrimSpace(wgKey)
		if wgKey == "" {
			fmt.Printf("  %s WireGuard public key is required for each engine\n", styleInfo.Render("[WARN]"))
			continue
		}

		engines = append(engines, types.EngineEndpoint{Address: address, WGPublicKey: wgKey})
	}

	if len(engines) <= 1 {
		return nil
	}
	return engines
}
