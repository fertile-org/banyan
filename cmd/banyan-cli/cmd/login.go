package cmd

import (
	"fmt"
	"net"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/types"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Re-establish WireGuard control tunnel",
	Long: `Re-establish the WireGuard control tunnel to the engine.

Use this command after a machine restart to reconnect without
re-running 'init'. It reads the existing configuration and
private key from /etc/banyan/banyan.yaml.

Requires sudo. Run 'banyan-cli init' first if no config exists.

Example:
  sudo banyan-cli login`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w (run 'sudo banyan-cli init' first)", err)
	}

	if cfg.CLI.WGPublicKey == "" || cfg.CLI.EngineWGPublicKey == "" || cfg.CLI.WGPrivateKeyFile == "" {
		return fmt.Errorf("incomplete CLI configuration. Run 'sudo banyan-cli init' first")
	}

	if controlTunnelExistsFn(types.ControlIfaceCLI) {
		fmt.Printf("  %s Control tunnel already active\n", styleOK.Render("[OK]"))
		return nil
	}

	privKey, err := types.ReadPrivateKeyFile(cfg.CLI.WGPrivateKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read private key from %s: %w", cfg.CLI.WGPrivateKeyFile, err)
	}

	return loginSetupTunnel(&cfg, privKey)
}

func loginSetupTunnel(cfg *types.BanyanConfig, privKey string) error {
	// Build engine list — same as NewAutoEngineClient
	engines := cfg.CLI.Engines
	if len(engines) == 0 && cfg.CLI.EngineWGPublicKey != "" {
		port := cfg.CLI.EnginePort
		if port == "" {
			port = "50051"
		}
		host := cfg.CLI.EngineHost
		if host == "" {
			host = "localhost"
		}
		engines = []types.EngineEndpoint{
			{Address: host + ":" + port, WGPublicKey: cfg.CLI.EngineWGPublicKey},
		}
	}

	myTunnelIP := types.TunnelIPFromPublicKey(cfg.CLI.WGPublicKey)
	fmt.Printf("  %s Setting up WireGuard control tunnel (%s)...\n", styleInfo.Render("[..]"), myTunnelIP)

	if err := setupControlTunnelFn(types.ControlIfaceCLI, privKey, myTunnelIP, 0); err != nil {
		return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure this runs with sudo and wireguard kernel module is loaded)", err)
	}

	// Add all engine peers — each engine has its own WG key and derived tunnel IP
	for _, eng := range engines {
		engineHost, _, _ := net.SplitHostPort(eng.Address)
		engineEndpointWG := engineHost + ":" + fmt.Sprintf("%d", types.ControlTunnelPort)
		engineTunnelIP := types.TunnelIPFromPublicKey(eng.WGPublicKey)
		if err := addControlPeerFn(types.ControlIfaceCLI, eng.WGPublicKey, engineEndpointWG, engineTunnelIP); err != nil {
			_ = cleanupControlTunnelFn(types.ControlIfaceCLI)
			return fmt.Errorf("failed to add engine peer to control tunnel: %w", err)
		}
	}

	fmt.Printf("  %s Control tunnel ready\n", styleOK.Render("[OK]"))
	return nil
}
