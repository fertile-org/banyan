package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/engine"
	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Engine configuration flags.
var (
	engineVPCCIDR       string
	engineDataDir       string
	engineRegistryPort  string
	engineGRPCPort      string
	engineStoreBackend  string
	engineStoreAddress  string
	engineAllowInsecure bool
)

// configPath is the default path to the Banyan config file.
var configPath = types.DefaultConfigPath

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
	Short: "Initialize Engine dependencies",
	RunE:  runEngineInit,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Banyan Engine",
	Long: `Start the Banyan Engine control plane.

This command:
  1. Starts managed etcd process (if configured) or connects to external etcd
  2. Initializes VPC networking (IPAM, DNS, Security)
  3. Starts the gRPC server (agents and CLI connect here)
  4. Watches for new deployments and dispatches tasks to agents
  5. Monitors task completion and updates deployment status`,
	RunE: runEngineStart,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Banyan Engine",
	RunE:  runEngineStop,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Engine status",
	RunE:  runEngineStatus,
}

var addClientCmd = &cobra.Command{
	Use:   "add-client",
	Short: "Whitelist a client (agent or CLI) public key",
	Long:  "Add a client's WireGuard public key to the whitelist so it can connect to this engine.",
	RunE:  runAddClient,
}

var (
	addClientName   string
	addClientPubkey string
)

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(addClientCmd)

	addClientCmd.Flags().StringVar(&addClientName, "name", "", "Client name (e.g., worker-1, cli-deploy)")
	addClientCmd.Flags().StringVar(&addClientPubkey, "pubkey", "", "Client's WireGuard public key")
	_ = addClientCmd.MarkFlagRequired("name")
	_ = addClientCmd.MarkFlagRequired("pubkey")

	rootCmd.PersistentFlags().StringVar(&engineDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	// Store backend flags (for external etcd override)
	startCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (etcd only)")
	startCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Etcd endpoints (for external etcd)")

	// General flags
	startCmd.Flags().StringVar(&engineVPCCIDR, "vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	startCmd.Flags().StringVar(&engineRegistryPort, "registry-port", "5000", "Embedded OCI registry port")
	startCmd.Flags().StringVar(&engineGRPCPort, "grpc-port", "50051", "Engine gRPC port")
	startCmd.Flags().BoolVar(&engineAllowInsecure, "allow-insecure", false, "Allow running without authentication (development only, NOT for production)")

	// Status flags
	statusCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (etcd only)")
	statusCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Etcd endpoints (for external etcd)")
}

func runEngineInit(cmd *cobra.Command, args []string) error {
	fmt.Println(styleTitle.Render("Banyan Engine - Initialization"))
	fmt.Println(styleDim.Render("========================================"))

	if os.Geteuid() != 0 {
		return fmt.Errorf("init must be run as root: sudo banyan-engine init")
	}

	// --- System setup (config dirs, sysctl) ---
	if err := runEngineSystemSetup(); err != nil {
		return err
	}

	// --- Directory creation ---
	dirs := []string{
		engineDataDir,
		filepath.Join(engineDataDir, "etcd"),
		filepath.Join(engineDataDir, "registry"),
		filepath.Join(engineDataDir, "vpc"),
		"/var/log",
		"/var/run",
	}

	fmt.Println(styleInfo.Render("\nCreating data directories..."))
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Printf("  %s %s: %v\n", styleWarn.Render("[WARN]"), dir, err)
		} else {
			fmt.Printf("  %s %s\n", styleOK.Render("[OK]"), dir)
		}
	}

	existingCfg, _ := types.LoadConfig(configPath)

	// --- Check for existing config ---
	if existingCfg.Engine.WGPrivateKeyFile != "" && existingCfg.Engine.WGPublicKey != "" && existingCfg.Engine.StoreBackend != "" {
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		if existingCfg.Engine.ManagedEtcd {
			fmt.Printf("         Store: managed etcd\n")
		} else {
			fmt.Printf("         Store: %s at %s\n", existingCfg.Engine.StoreBackend, existingCfg.Engine.StoreAddress)
		}

		var overwrite bool
		overwriteForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Overwrite existing configuration?").
					Value(&overwrite),
			),
		)
		if err := overwriteForm.Run(); err != nil {
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

		// Clear config sections so all prompts run fresh
		existingCfg.Engine.WGPrivateKeyFile = ""
		existingCfg.Engine.WGPublicKey = ""
		existingCfg.Engine.StoreBackend = ""
		existingCfg.Engine.StoreAddress = ""
		existingCfg.Engine.ManagedEtcd = false
		existingCfg.Engine.EtcdUsername = ""
		existingCfg.Engine.EtcdPassword = ""
		existingCfg.Engine.EtcdCertFile = ""
		existingCfg.Engine.EtcdKeyFile = ""
		existingCfg.Engine.EtcdCAFile = ""
		existingCfg.Engine.ManagedRegistry = false
		existingCfg.Engine.ExternalRegistryURL = ""
	}

	// --- Create whitelisted keys directory ---
	whitelistedKeysDir := existingCfg.Engine.WhitelistedKeysDir
	if whitelistedKeysDir == "" {
		whitelistedKeysDir = types.DefaultWhitelistedKeysDir
	}
	if err := os.MkdirAll(whitelistedKeysDir, 0o700); err != nil {
		fmt.Printf("  %s Failed to create whitelisted keys directory: %v\n", styleWarn.Render("[WARN]"), err)
	} else {
		fmt.Printf("  %s Whitelisted keys directory: %s\n", styleOK.Render("[OK]"), whitelistedKeysDir)
	}

	// --- WireGuard keypair generation ---
	fmt.Println(styleInfo.Render("\nGenerating WireGuard keypair..."))
	if existingCfg.Engine.WGPrivateKeyFile != "" && existingCfg.Engine.WGPublicKey != "" {
		fmt.Printf("  %s WireGuard keypair already configured\n", styleOK.Render("[OK]"))
		fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), existingCfg.Engine.WGPublicKey)
	} else {
		privKey, pubKey, genErr := overlay.GenerateKeyPair()
		if genErr != nil {
			return fmt.Errorf("failed to generate WireGuard keypair: %w", genErr)
		}
		keyPath, writeErr := types.WritePrivateKeyFile(types.DefaultKeysDir, "engine", privKey)
		if writeErr != nil {
			return fmt.Errorf("failed to write private key: %w", writeErr)
		}
		existingCfg.Engine.WGPrivateKeyFile = keyPath
		existingCfg.Engine.WGPublicKey = pubKey
		fmt.Printf("  %s WireGuard keypair generated\n", styleOK.Render("[OK]"))
		fmt.Printf("  %s Private key: %s\n", styleOK.Render("[OK]"), keyPath)
		fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), pubKey)
		fmt.Println()
		fmt.Println(styleInfo.Render("Share this public key with agents and CLI clients during their init."))
	}

	// --- Deployment mode ---
	// Ask first: single or multi-engine? This determines the etcd/registry flow.
	fmt.Println()
	multiEngine := existingCfg.Engine.MultiEngine
	if existingCfg.Engine.StoreBackend == "" {
		// Fresh config — ask deployment mode
		var modeChoice string
		modeForm := huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Deployment mode").
					Description("How many engines will run in your cluster?").
					Options(
						huh.NewOption("Single engine — zero config, everything managed for you (recommended)", "single"),
						huh.NewOption("Multi-engine HA — 2+ engines for high availability (requires your own etcd and registry)", "multi"),
					).
					Value(&modeChoice),
			),
		)
		if err := modeForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nInitialization cancelled.")
				return nil
			}
			return fmt.Errorf("deployment mode: %w", err)
		}
		multiEngine = modeChoice == "multi"
	}

	if multiEngine {
		// --- Multi-engine setup: external etcd + external registry required ---
		existingCfg.Engine.MultiEngine = true
		existingCfg.Engine.ManagedEtcd = false
		existingCfg.Engine.ManagedRegistry = false
		existingCfg.Engine.StoreBackend = "etcd"

		fmt.Println(styleInfo.Render("\nMulti-engine mode requires an external etcd cluster and registry."))
		fmt.Println(styleInfo.Render("All engines must point to the same etcd and registry.\n"))

		// Etcd endpoints
		etcdEndpoints := existingCfg.Engine.StoreAddress
		if etcdEndpoints == "" {
			etcdEndpoints = "http://etcd.internal:2379"
		}
		registryURL := existingCfg.Engine.ExternalRegistryURL
		if registryURL == "" {
			registryURL = "registry.internal:5000"
		}

		var authMethod string
		multiForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("External etcd endpoint").
					Description("Your etcd cluster address (shared between all engines)").
					Value(&etcdEndpoints),
				huh.NewInput().
					Title("External registry URL").
					Description("Your OCI registry address (shared between all engines, e.g. registry.internal:5000)").
					Value(&registryURL),
				huh.NewSelect[string]().
					Title("Etcd connection security").
					Options(
						huh.NewOption("None", "none"),
						huh.NewOption("Username & Password", "password"),
						huh.NewOption("TLS (CA certificate)", "tls"),
						huh.NewOption("mTLS (client certificates)", "mtls"),
					).
					Value(&authMethod),
			),
		)
		if err := multiForm.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nInitialization cancelled.")
				return nil
			}
			return fmt.Errorf("multi-engine setup: %w", err)
		}
		existingCfg.Engine.StoreAddress = etcdEndpoints
		existingCfg.Engine.ExternalRegistryURL = registryURL
		fmt.Printf("  %s External etcd: %s\n", styleOK.Render("[OK]"), etcdEndpoints)
		fmt.Printf("  %s External registry: %s\n", styleOK.Render("[OK]"), registryURL)

		// Collect auth-specific inputs for etcd
		if err := collectEtcdAuth(&existingCfg, authMethod); err != nil {
			return err
		}

		fmt.Printf("  %s Multi-engine HA mode enabled\n", styleOK.Render("[OK]"))
	} else {
		// --- Single-engine setup: managed or external etcd/registry ---
		existingCfg.Engine.MultiEngine = false

		// Etcd setup
		if existingCfg.Engine.StoreBackend != "" {
			if existingCfg.Engine.ManagedEtcd {
				fmt.Printf("  %s Managed etcd already configured\n", styleOK.Render("[OK]"))
			} else {
				fmt.Printf("  %s External etcd already configured: %s\n", styleOK.Render("[OK]"), existingCfg.Engine.StoreAddress)
			}
		} else {
			var etcdChoice string
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("Etcd setup").
						Description("Banyan requires etcd for distributed state storage").
						Options(
							huh.NewOption("Managed - Banyan runs etcd for you (recommended)", "managed"),
							huh.NewOption("External - Connect to your own etcd cluster", "external"),
						).
						Value(&etcdChoice),
				),
			)
			if err := form.Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					fmt.Println("\nInitialization cancelled.")
					return nil
				}
				return fmt.Errorf("etcd setup: %w", err)
			}

			existingCfg.Engine.StoreBackend = "etcd"

			if etcdChoice == "managed" {
				existingCfg.Engine.ManagedEtcd = true
			} else {
				existingCfg.Engine.ManagedEtcd = false
				var endpoints string
				var authMethod string
				endpoints = "http://localhost:2379"
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Etcd endpoints").
							Description("Comma-separated list of etcd endpoints").
							Value(&endpoints),
						huh.NewSelect[string]().
							Title("Connection security").
							Options(
								huh.NewOption("None", "none"),
								huh.NewOption("Username & Password", "password"),
								huh.NewOption("TLS (CA certificate)", "tls"),
								huh.NewOption("mTLS (client certificates)", "mtls"),
							).
							Value(&authMethod),
					),
				)
				if err := form.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Println("\nInitialization cancelled.")
						return nil
					}
					return fmt.Errorf("etcd endpoints: %w", err)
				}
				existingCfg.Engine.StoreAddress = endpoints
				if err := collectEtcdAuth(&existingCfg, authMethod); err != nil {
					return err
				}
			}
		}

		// Registry setup
		if existingCfg.Engine.ManagedRegistry || existingCfg.Engine.ExternalRegistryURL != "" {
			if existingCfg.Engine.ManagedRegistry {
				fmt.Printf("  %s Managed registry already configured\n", styleOK.Render("[OK]"))
			} else {
				fmt.Printf("  %s External registry already configured: %s\n", styleOK.Render("[OK]"), existingCfg.Engine.ExternalRegistryURL)
			}
		} else {
			var registryChoice string
			form := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title("OCI Registry setup").
						Description("Banyan requires an OCI registry for container image storage").
						Options(
							huh.NewOption("Managed - Banyan runs a registry for you (recommended)", "managed"),
							huh.NewOption("External - Use your own registry (Harbor, Docker Hub, etc.)", "external"),
						).
						Value(&registryChoice),
				),
			)
			if err := form.Run(); err != nil {
				if errors.Is(err, huh.ErrUserAborted) {
					fmt.Println("\nInitialization cancelled.")
					return nil
				}
				return fmt.Errorf("registry setup: %w", err)
			}

			if registryChoice == "managed" {
				existingCfg.Engine.ManagedRegistry = true
				fmt.Printf("  %s Managed registry will be started with the engine\n", styleOK.Render("[OK]"))
			} else {
				existingCfg.Engine.ManagedRegistry = false
				var registryURL string
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("External registry URL").
							Description("e.g. myregistry.example.com:5000 or registry.example.com/banyan").
							Value(&registryURL),
					),
				)
				if err := form.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Println("\nInitialization cancelled.")
						return nil
					}
					return fmt.Errorf("registry URL: %w", err)
				}
				existingCfg.Engine.ExternalRegistryURL = registryURL
				fmt.Printf("  %s External registry: %s\n", styleOK.Render("[OK]"), registryURL)
			}
		}
	}

	// Auto-generate engine ID (internal, never shown to user)
	if existingCfg.Engine.EngineID == "" {
		existingCfg.Engine.EngineID = engine.GenerateEngineID()
	}

	// --- Save config ---
	fmt.Println()
	if err := types.SaveConfig(configPath, &existingCfg); err != nil {
		fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
	} else {
		fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
	}

	fmt.Println()
	fmt.Println(styleDim.Render("========================================"))
	fmt.Println(styleOK.Render("Initialization complete!"))
	fmt.Println()
	fmt.Println(styleInfo.Render("Next steps:"))
	fmt.Println()
	fmt.Println("  sudo systemctl enable --now banyan-engine  # start + enable on boot")
	fmt.Println()
	fmt.Println(styleDim.Render("Or run in foreground (for development):"))
	fmt.Println()
	fmt.Println("  sudo banyan-engine start")
	return nil
}

// resolveStoreConfig reads the store backend/address from config, applying flag overrides.
func resolveStoreConfig(cmd *cobra.Command) (backend, address string) {
	cfg, _ := types.LoadConfig(configPath)
	backend = cfg.Engine.GetStoreBackend()
	address = cfg.Engine.StoreAddress

	if cmd.Flags().Changed("store-backend") {
		backend = engineStoreBackend
	}
	if cmd.Flags().Changed("store-address") {
		address = engineStoreAddress
	}

	return backend, address
}

// collectEtcdAuth prompts for etcd authentication credentials based on the selected method.
func collectEtcdAuth(cfg *types.BanyanConfig, authMethod string) error {
	switch authMethod {
	case "password":
		var username, password string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("Etcd username").Value(&username),
				huh.NewInput().Title("Etcd password").EchoMode(huh.EchoModePassword).Value(&password),
			),
		)
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("etcd credentials: %w", err)
		}
		cfg.Engine.EtcdUsername = username
		cfg.Engine.EtcdPassword = password
	case "tls":
		var caFile string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("CA certificate path").Value(&caFile),
			),
		)
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("etcd TLS: %w", err)
		}
		cfg.Engine.EtcdCAFile = caFile
	case "mtls":
		var caFile, certFile, keyFile string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().Title("CA certificate path").Value(&caFile),
				huh.NewInput().Title("Client certificate path").Value(&certFile),
				huh.NewInput().Title("Client key path").Value(&keyFile),
			),
		)
		if err := form.Run(); err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return fmt.Errorf("etcd mTLS: %w", err)
		}
		cfg.Engine.EtcdCAFile = caFile
		cfg.Engine.EtcdCertFile = certFile
		cfg.Engine.EtcdKeyFile = keyFile
	}
	return nil
}

func runEngineStart(cmd *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("start must be run as root: sudo banyan-engine start")
	}

	logging.Setup(nil)
	log := logging.New("engine")
	log.Info("Banyan Engine starting")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Shutting down")
		cancel()
	}()

	// Load config
	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		log.Warn("Failed to load config", "error", err)
	}

	// Early auth check — fail fast before starting etcd, WireGuard, registry, etc.
	if !engineAllowInsecure {
		whitelistedKeysDir := cfg.Engine.WhitelistedKeysDir
		if whitelistedKeysDir == "" {
			whitelistedKeysDir = types.DefaultWhitelistedKeysDir
		}
		earlyKeys, _ := types.LoadWhitelistedKeys(whitelistedKeysDir)
		if len(earlyKeys) == 0 {
			return fmt.Errorf("no whitelisted client keys found in %s\n"+
				"  Add client keys with: sudo banyan-engine add-client --name <name> --pubkey <key>\n"+
				"  Or use --allow-insecure for development only (NOT for production)", whitelistedKeysDir)
		}
	}

	// Read gRPC port from config if not overridden by flags
	if !cmd.Flags().Changed("grpc-port") && cfg.Engine.GRPCPort != "" {
		engineGRPCPort = cfg.Engine.GRPCPort
	}

	// Determine store backend and address
	storeBackend, storeAddress := resolveStoreConfig(cmd)
	log.Info("Store backend configured", "backend", storeBackend)

	// Handle managed etcd
	if cfg.Engine.ManagedEtcd {
		etcdDataDir := filepath.Join(engineDataDir, "etcd")
		log.Info("Starting managed etcd", "data_dir", etcdDataDir)
		etcdCmd, etcdErr := startManagedEtcd(etcdDataDir)
		if etcdErr != nil {
			return fmt.Errorf("failed to start managed etcd: %w", etcdErr)
		}
		defer stopManagedEtcd(etcdCmd)
		storeAddress = managedEtcdClientURL
		log.Info("Managed etcd started", "address", storeAddress)
	} else {
		// Resolve default address for external etcd
		storeAddress = resolveDefaultStoreAddress(storeBackend, storeAddress)
	}

	// Only etcd is supported
	if storeBackend != "etcd" {
		return fmt.Errorf("unsupported store backend: %s (only etcd is supported)", storeBackend)
	}

	log.Info("Connecting to store", "backend", storeBackend, "address", storeAddress)

	// Load whitelisted agent public keys
	whitelistedKeysDir := cfg.Engine.WhitelistedKeysDir
	if whitelistedKeysDir == "" {
		whitelistedKeysDir = types.DefaultWhitelistedKeysDir
	}
	whitelistedKeys, wkErr := types.LoadWhitelistedKeys(whitelistedKeysDir)
	if wkErr != nil {
		log.Warn("Failed to load whitelisted keys", "error", wkErr)
	}
	if len(whitelistedKeys) > 0 {
		log.Info("Loaded whitelisted agent keys", "count", len(whitelistedKeys), "dir", whitelistedKeysDir)
	}

	// Set up WireGuard control tunnel (required when keys are configured)
	// Engine's tunnel IP is derived from its own public key — same derivation agents/CLI use.
	var engineTunnelIP string
	if cfg.Engine.WGPrivateKeyFile != "" {
		wgPrivateKey, readErr := types.ReadPrivateKeyFile(cfg.Engine.WGPrivateKeyFile)
		if readErr != nil {
			return fmt.Errorf("failed to load WireGuard private key: %w", readErr)
		}
		tunnelIP := types.TunnelIPFromPublicKey(cfg.Engine.WGPublicKey)
		engineTunnelIP = tunnelIP.String()
		log.Info("Setting up WireGuard control tunnel")
		if tunnelErr := overlay.SetupControlTunnelExec(types.ControlIfaceEngine, wgPrivateKey, tunnelIP, types.ControlTunnelPort); tunnelErr != nil {
			return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure wireguard kernel module is loaded)", tunnelErr)
		}
		defer overlay.CleanupControlTunnelExec(types.ControlIfaceEngine) //nolint:errcheck // best-effort cleanup on exit
		log.Info("Control tunnel ready", "ip", engineTunnelIP, "port", types.ControlTunnelPort)

		// Add whitelisted keys as control tunnel peers
		for pubKey, name := range whitelistedKeys {
			tunnelIP := types.TunnelIPFromPublicKey(pubKey)
			if peerErr := overlay.AddControlPeerExec(types.ControlIfaceEngine, pubKey, "", tunnelIP); peerErr != nil {
				log.Warn("Failed to add control peer", "name", name, "error", peerErr)
			} else {
				log.Info("Control peer added", "name", name, "tunnel_ip", tunnelIP)
			}
		}
	}

	// Validate multi-engine prerequisites
	if cfg.Engine.MultiEngine {
		if cfg.Engine.ManagedEtcd {
			return fmt.Errorf("multi-engine mode requires external etcd (set managed_etcd: false and provide store_address)")
		}
		if cfg.Engine.ManagedRegistry || cfg.Engine.ExternalRegistryURL == "" {
			return fmt.Errorf("multi-engine mode requires external registry (set managed_registry: false and provide external_registry_url)")
		}
	}

	// Handle managed or external registry
	var registryURL string
	controlTunnelActive := cfg.Engine.WGPrivateKeyFile != ""
	if cfg.Engine.ExternalRegistryURL != "" {
		// User-provided external registry
		registryURL = cfg.Engine.ExternalRegistryURL
		log.Info("Using external registry", "url", registryURL)
	} else if cfg.Engine.ManagedRegistry {
		// Managed Distribution registry subprocess
		registryDataDir := filepath.Join(engineDataDir, "registry")
		registryBindAddr := "127.0.0.1"
		if controlTunnelActive && engineTunnelIP != "" {
			registryBindAddr = engineTunnelIP
		}
		log.Info("Starting managed registry", "data_dir", registryDataDir, "bind", registryBindAddr, "port", managedRegistryPort)
		registryCmd, regErr := startManagedRegistry(registryDataDir, registryBindAddr, managedRegistryPort)
		if regErr != nil {
			return fmt.Errorf("failed to start managed registry: %w\n"+
				"  Install the registry binary: sudo bash install.sh --role engine\n"+
				"  Or use an external registry: set managed_registry: false and external_registry_url in config", regErr)
		}
		{
			defer stopManagedRegistry(registryCmd)

			registryHost := registryBindAddr
			if registryHost == "127.0.0.1" {
				engineIP, ipErr := engine.DetermineEngineIP()
				if ipErr != nil {
					return fmt.Errorf("failed to determine engine IP for registry: %w", ipErr)
				}
				registryHost = engineIP
			}
			registryURL = fmt.Sprintf("%s:%s", registryHost, managedRegistryPort)
			log.Info("Managed registry started", "url", registryURL)
		}
	}
	// If registryURL is empty, engine.Run() will start the in-memory fallback

	// Resolve metrics port from config
	metricsPort := cfg.Engine.MetricsPort

	eng, err := engine.New(&engine.Options{
		StoreBackend:        storeBackend,
		StoreAddress:        storeAddress,
		VPCCIDR:             engineVPCCIDR,
		RegistryPort:        engineRegistryPort,
		GRPCPort:            engineGRPCPort,
		MetricsPort:         metricsPort,
		DataDir:             engineDataDir,
		EtcdUsername:        cfg.Engine.EtcdUsername,
		EtcdPassword:        cfg.Engine.EtcdPassword,
		EtcdCertFile:        cfg.Engine.EtcdCertFile,
		EtcdKeyFile:         cfg.Engine.EtcdKeyFile,
		EtcdCAFile:          cfg.Engine.EtcdCAFile,
		WhitelistedKeys:     whitelistedKeys,
		AllowInsecure:       engineAllowInsecure,
		ControlTunnelActive: controlTunnelActive,
		TunnelIP:            engineTunnelIP,
		ExternalRegistryURL: registryURL,
		EngineID:            cfg.Engine.EngineID,
		MultiEngine:         cfg.Engine.MultiEngine,
	})
	if err != nil {
		return err
	}
	defer eng.Close()

	log.Info("Connected to store", "backend", storeBackend)
	log.Info("Engine is running, watching for deployments")

	if runErr := eng.Run(ctx); runErr != nil {
		return runErr
	}

	log.Info("Engine stopped")
	return nil
}

// resolveDefaultStoreAddress returns the store address, applying defaults per backend.
func resolveDefaultStoreAddress(backend, address string) string {
	if address != "" {
		return address
	}
	// Only etcd is supported now
	switch backend {
	case "etcd":
		return "http://127.0.0.1:2379"
	default:
		return address
	}
}

// managedEtcdClientURL is the default client URL for managed etcd (used for health checks and local access).
const managedEtcdClientURL = "http://127.0.0.1:2379"

// managedEtcdListenURL is the listen address for managed etcd.
// Listens on localhost only — agents no longer need direct etcd access
// (overlay networking is managed by the engine via gRPC).
const managedEtcdListenURL = "http://127.0.0.1:2379"

// startManagedEtcd starts an etcd process using the system-installed etcd binary.
// It waits for etcd to become healthy before returning.
func startManagedEtcd(dataDir string) (*exec.Cmd, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create etcd data dir: %w", err)
	}

	etcdCmd := exec.Command("etcd",
		"--data-dir", dataDir,
		"--listen-client-urls", managedEtcdListenURL,
		"--advertise-client-urls", managedEtcdClientURL,
		"--listen-peer-urls", "http://127.0.0.1:2380",
	)
	etcdCmd.Stdout = os.Stdout
	etcdCmd.Stderr = os.Stderr

	if err := etcdCmd.Start(); err != nil {
		return nil, fmt.Errorf("start etcd: %w", err)
	}

	// Wait for etcd to become healthy
	if err := waitForEtcd(managedEtcdClientURL, 10*time.Second); err != nil {
		// etcd failed to start — kill the process
		_ = etcdCmd.Process.Kill()
		return nil, fmt.Errorf("etcd did not become healthy: %w", err)
	}

	return etcdCmd, nil
}

// waitForEtcd polls the etcd health endpoint until it responds OK or the timeout expires.
func waitForEtcd(clientURL string, timeout time.Duration) error {
	healthURL := clientURL + "/health"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL) //nolint:gosec // managed etcd on localhost
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for etcd at %s", clientURL)
}

// managedRegistryPort is the default port for the managed Distribution registry.
const managedRegistryPort = "5000"

// startManagedRegistry starts a Distribution (Docker Registry v2) process.
// It writes a minimal config, starts the binary, and waits for it to become healthy.
func startManagedRegistry(dataDir, bindAddr, port string) (*exec.Cmd, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create registry data dir: %w", err)
	}

	// Write a minimal Distribution config
	configContent := fmt.Sprintf(`version: 0.1
storage:
  filesystem:
    rootdirectory: %s
http:
  addr: %s:%s
`, dataDir, bindAddr, port)

	regConfigPath := filepath.Join(dataDir, "config.yml")
	if err := os.WriteFile(regConfigPath, []byte(configContent), 0o600); err != nil {
		return nil, fmt.Errorf("write registry config: %w", err)
	}

	registryCmd := exec.Command("registry", "serve", regConfigPath) //nolint:gosec // config path is constructed internally
	registryCmd.Stdout = os.Stdout
	registryCmd.Stderr = os.Stderr

	if err := registryCmd.Start(); err != nil {
		return nil, fmt.Errorf("start registry: %w", err)
	}

	registryURL := fmt.Sprintf("http://%s:%s", bindAddr, port)
	if err := waitForRegistry(registryURL, 10*time.Second); err != nil {
		_ = registryCmd.Process.Kill()
		return nil, fmt.Errorf("registry did not become healthy: %w", err)
	}

	return registryCmd, nil
}

// waitForRegistry polls the registry /v2/ endpoint until it responds OK or the timeout expires.
func waitForRegistry(baseURL string, timeout time.Duration) error {
	healthURL := baseURL + "/v2/"
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL) //nolint:gosec // managed registry on localhost
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for registry at %s", baseURL)
}

// stopManagedRegistry gracefully stops the managed registry process.
func stopManagedRegistry(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	logging.Info("Managed registry stopped")
}

// stopManagedEtcd gracefully stops the managed etcd process.
func stopManagedEtcd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Send SIGTERM for graceful shutdown
	_ = cmd.Process.Signal(syscall.SIGTERM)

	// Wait up to 5 seconds for clean exit
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// Process exited
	case <-time.After(5 * time.Second):
		// Force kill if it didn't stop
		_ = cmd.Process.Kill()
		<-done
	}
	logging.Info("Managed etcd stopped")
}

func runEngineStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine stopped.")
	return nil
}

func runAddClient(cmd *cobra.Command, args []string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("add-client must be run as root: sudo banyan-engine add-client --name <name> --pubkey <key>")
	}

	cfg, _ := types.LoadConfig(configPath)
	keysDir := cfg.Engine.WhitelistedKeysDir
	if keysDir == "" {
		keysDir = types.DefaultWhitelistedKeysDir
	}
	if err := os.MkdirAll(keysDir, 0o700); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	name := strings.TrimSpace(addClientName)
	pubkey := strings.TrimSpace(addClientPubkey)

	keyFile := filepath.Join(keysDir, name+".pub")
	if err := os.WriteFile(keyFile, []byte(pubkey+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	fmt.Printf("Client %q whitelisted at %s\n", name, keyFile)
	fmt.Println("Restart the engine for the change to take effect.")
	return nil
}

func runEngineStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine - Status")
	fmt.Println("========================================")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Determine store backend and address from config/flags
	storeBackend, storeAddress := resolveStoreConfig(cmd)

	// Use default address per backend if not configured
	if storeAddress == "" && storeBackend == "etcd" {
		storeAddress = "http://127.0.0.1:2379"
	}

	cfg, _ := types.LoadConfig(configPath)
	store, err := storage.NewStoreWithOptions(&storage.EtcdOptions{
		Endpoints: []string{storeAddress},
		Prefix:    "/banyan",
		Username:  cfg.Engine.EtcdUsername,
		Password:  cfg.Engine.EtcdPassword,
		CertFile:  cfg.Engine.EtcdCertFile,
		KeyFile:   cfg.Engine.EtcdKeyFile,
		CAFile:    cfg.Engine.EtcdCAFile,
	})
	if err != nil {
		fmt.Printf("Store (%s): NOT RUNNING\n", storeBackend)
		fmt.Printf("Connection: FAILED (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}
	defer store.Close()

	fmt.Printf("Store (%s): RUNNING\n", storeBackend)
	fmt.Println("Connection: OK")

	nodeKeys, _ := store.List(ctx, types.KeyNodes)
	fmt.Printf("\nAgents: %d\n", len(nodeKeys))
	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := store.Get(ctx, key, &node); err != nil {
			continue
		}
		age := time.Since(node.LastSeen).Truncate(time.Second)
		tagsStr := ""
		if len(node.Tags) > 0 {
			tagsStr = fmt.Sprintf(", tags: %v", node.Tags)
		}
		fmt.Printf("  - %s (status: %s, last seen: %s ago%s)\n", node.Name, node.Status, age, tagsStr)
	}

	deployKeys, _ := store.List(ctx, types.KeyDeployments)
	fmt.Printf("\nDeployments: %d\n", len(deployKeys))
	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := store.Get(ctx, key, &record); err != nil {
			continue
		}

		allTasks := types.CollectDeploymentTasks(ctx, store, record.ID)
		var tasks []types.TaskRecord
		for i := range allTasks {
			if allTasks[i].Type == types.TaskTypeCreateAndStart {
				tasks = append(tasks, allTasks[i])
			}
		}

		healthy := 0
		for i := range tasks {
			if tasks[i].ContainerStatus == types.StatusRunning {
				healthy++
			}
		}

		fmt.Printf("  - %s (status: %s, containers: %d/%d healthy)\n",
			record.Name, record.Status, healthy, len(tasks))

		grouped := types.GroupTasksByService(tasks)
		for _, svcName := range types.SortedServiceNames(grouped) {
			fmt.Printf("    %s:\n", svcName)
			svcTasks := grouped[svcName]
			for i := range svcTasks {
				containerStatus := svcTasks[i].ContainerStatus
				if containerStatus == "" {
					containerStatus = "pending"
				}
				checkedInfo := ""
				if !svcTasks[i].ContainerCheckedAt.IsZero() {
					ago := time.Since(svcTasks[i].ContainerCheckedAt).Truncate(time.Second)
					checkedInfo = fmt.Sprintf(" (checked %s ago)", ago)
				}
				fmt.Printf("      %s on %s: %s%s\n", svcTasks[i].ContainerName, svcTasks[i].AgentID, containerStatus, checkedInfo)
			}
		}
	}

	fmt.Println("\n========================================")
	return nil
}

// runEngineSystemSetup performs one-time system configuration:
// creates /etc/banyan/ directories and enables IP forwarding.
func runEngineSystemSetup() error {
	// Step 1: Create config directories
	fmt.Print(styleInfo.Render("\nConfiguring system...") + "\n")
	fmt.Print("  Creating /etc/banyan/ directories... ")
	configDirs := []string{"/etc/banyan", "/etc/banyan/keys", "/etc/banyan/whitelisted-keys"}
	for _, dir := range configDirs {
		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			fmt.Println("[FAIL]")
			return fmt.Errorf("create %s: %w", dir, mkErr)
		}
	}
	fmt.Println(styleOK.Render("[OK]"))

	// Step 2: Enable IP forwarding
	fmt.Print("  Enabling net.ipv4.ip_forward... ")
	if err := enableSysctlPersistent("net.ipv4.ip_forward"); err != nil {
		fmt.Println("[FAIL]")
		return err
	}
	fmt.Println(styleOK.Render("[OK]"))

	return nil
}

// enableSysctlPersistent sets a sysctl value persistently via /etc/sysctl.d/ and applies it.
func enableSysctlPersistent(key string) error {
	value := "1"
	confFile := "/etc/sysctl.d/99-banyan.conf"
	existing, _ := os.ReadFile(confFile)
	line := key + " = " + value
	if !strings.Contains(string(existing), line) {
		content := string(existing)
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += line + "\n"
		if err := os.WriteFile(confFile, []byte(content), 0o644); err != nil { //nolint:gosec // sysctl.d config must be world-readable
			return fmt.Errorf("write sysctl config: %w", err)
		}
	}
	return runInitCmd("sysctl", "-w", key+"="+value)
}

// runInitCmd runs a command and returns an error if it fails.
func runInitCmd(name string, args ...string) error {
	c := exec.Command(name, args...) //nolint:gosec // args are constructed internally
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
