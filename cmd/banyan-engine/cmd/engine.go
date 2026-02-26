package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/engine"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Engine configuration flags.
var (
	engineVPCCIDR      string
	engineDataDir      string
	engineRegistryPort string
	engineGRPCPort     string
	engineStoreBackend string
	engineStoreAddress string
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

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)

	rootCmd.PersistentFlags().StringVar(&engineDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	// Store backend flags (for external etcd override)
	startCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (etcd only)")
	startCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Etcd endpoints (for external etcd)")

	// General flags
	startCmd.Flags().StringVar(&engineVPCCIDR, "vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	startCmd.Flags().StringVar(&engineRegistryPort, "registry-port", "5000", "Embedded OCI registry port")
	startCmd.Flags().StringVar(&engineGRPCPort, "grpc-port", "50051", "Engine gRPC port")

	// Status flags
	statusCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (etcd only)")
	statusCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Etcd endpoints (for external etcd)")
}

func runEngineInit(cmd *cobra.Command, args []string) error {
	fmt.Println(styleTitle.Render("Banyan Engine - Initialization"))
	fmt.Println(styleDim.Render("========================================"))

	if os.Geteuid() != 0 {
		fmt.Println(styleWarn.Render("Warning: Not running as root. Some operations may require sudo."))
	}

	// --- Directory creation ---
	dirs := []string{
		engineDataDir,
		filepath.Join(engineDataDir, "etcd"),
		filepath.Join(engineDataDir, "vpc"),
		"/var/log",
		"/var/run",
	}

	fmt.Println(styleInfo.Render("\nCreating directories..."))
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
	}

	// --- Create whitelisted keys directory ---
	whitelistedKeysDir := existingCfg.Engine.WhitelistedKeysDir
	if whitelistedKeysDir == "" {
		whitelistedKeysDir = types.DefaultWhitelistedKeysDir
	}
	if err := os.MkdirAll(whitelistedKeysDir, 0o755); err != nil {
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

	// --- Etcd setup ---
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

			// Collect endpoints and auth method
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

			// Collect auth-specific inputs
			switch authMethod {
			case "password":
				var username, password string
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Etcd username").
							Value(&username),
						huh.NewInput().
							Title("Etcd password").
							EchoMode(huh.EchoModePassword).
							Value(&password),
					),
				)
				if err := form.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Println("\nInitialization cancelled.")
						return nil
					}
					return fmt.Errorf("etcd credentials: %w", err)
				}
				existingCfg.Engine.EtcdUsername = username
				existingCfg.Engine.EtcdPassword = password
			case "tls":
				var caFile string
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("CA certificate path").
							Value(&caFile),
					),
				)
				if err := form.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Println("\nInitialization cancelled.")
						return nil
					}
					return fmt.Errorf("etcd TLS: %w", err)
				}
				existingCfg.Engine.EtcdCAFile = caFile
			case "mtls":
				var caFile, certFile, keyFile string
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("CA certificate path").
							Value(&caFile),
						huh.NewInput().
							Title("Client certificate path").
							Value(&certFile),
						huh.NewInput().
							Title("Client key path").
							Value(&keyFile),
					),
				)
				if err := form.Run(); err != nil {
					if errors.Is(err, huh.ErrUserAborted) {
						fmt.Println("\nInitialization cancelled.")
						return nil
					}
					return fmt.Errorf("etcd TLS: %w", err)
				}
				existingCfg.Engine.EtcdCAFile = caFile
				existingCfg.Engine.EtcdCertFile = certFile
				existingCfg.Engine.EtcdKeyFile = keyFile
			}
		}
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
	fmt.Println(styleInfo.Render("Next step: banyan-engine start"))
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

func runEngineStart(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine")
	fmt.Println("========================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Load config
	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", err)
	}

	// Read gRPC port from config if not overridden by flags
	if !cmd.Flags().Changed("grpc-port") && cfg.Engine.GRPCPort != "" {
		engineGRPCPort = cfg.Engine.GRPCPort
	}

	// Determine store backend and address
	storeBackend, storeAddress := resolveStoreConfig(cmd)
	fmt.Printf("Store backend: %s\n", storeBackend)

	// Handle managed etcd
	if cfg.Engine.ManagedEtcd {
		etcdDataDir := filepath.Join(engineDataDir, "etcd")
		fmt.Printf("Starting managed etcd (data: %s)...\n", etcdDataDir)
		etcdCmd, etcdErr := startManagedEtcd(etcdDataDir)
		if etcdErr != nil {
			return fmt.Errorf("failed to start managed etcd: %w", etcdErr)
		}
		defer stopManagedEtcd(etcdCmd)
		storeAddress = managedEtcdClientURL
		fmt.Printf("Managed etcd started: %s\n", storeAddress)
	} else {
		// Resolve default address for external etcd
		storeAddress = resolveDefaultStoreAddress(storeBackend, storeAddress)
	}

	// Only etcd is supported
	if storeBackend != "etcd" {
		return fmt.Errorf("unsupported store backend: %s (only etcd is supported)", storeBackend)
	}

	fmt.Printf("Connecting to %s at %s...\n", storeBackend, storeAddress)

	// Load whitelisted agent public keys
	whitelistedKeysDir := cfg.Engine.WhitelistedKeysDir
	if whitelistedKeysDir == "" {
		whitelistedKeysDir = types.DefaultWhitelistedKeysDir
	}
	whitelistedKeys, wkErr := types.LoadWhitelistedKeys(whitelistedKeysDir)
	if wkErr != nil {
		fmt.Printf("Warning: Failed to load whitelisted keys: %v\n", wkErr)
	}
	if len(whitelistedKeys) > 0 {
		fmt.Printf("Loaded %d whitelisted agent key(s) from %s\n", len(whitelistedKeys), whitelistedKeysDir)
	}

	// Set up WireGuard control tunnel (required when keys are configured)
	if cfg.Engine.WGPrivateKeyFile != "" {
		wgPrivateKey, readErr := types.ReadPrivateKeyFile(cfg.Engine.WGPrivateKeyFile)
		if readErr != nil {
			return fmt.Errorf("failed to load WireGuard private key: %w", readErr)
		}
		fmt.Println("Setting up WireGuard control tunnel...")
		engineIP := net.ParseIP(types.ControlTunnelEngineIP)
		if tunnelErr := overlay.SetupControlTunnelExec(types.ControlIfaceEngine, wgPrivateKey, engineIP, types.ControlTunnelPort); tunnelErr != nil {
			return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure wireguard kernel module is loaded)", tunnelErr)
		}
		defer overlay.CleanupControlTunnelExec(types.ControlIfaceEngine) //nolint:errcheck // best-effort cleanup on exit
		fmt.Printf("Control tunnel ready on %s (port %d)\n", types.ControlTunnelEngineIP, types.ControlTunnelPort)

		// Add whitelisted keys as control tunnel peers
		for pubKey, name := range whitelistedKeys {
			tunnelIP := types.TunnelIPFromPublicKey(pubKey)
			if peerErr := overlay.AddControlPeerExec(types.ControlIfaceEngine, pubKey, "", tunnelIP); peerErr != nil {
				fmt.Printf("Warning: Failed to add control peer %s: %v\n", name, peerErr)
			} else {
				fmt.Printf("  Control peer: %s → %s\n", name, tunnelIP)
			}
		}
	}

	// Determine overlay type
	overlayType := cfg.Engine.OverlayType
	if overlayType == "" {
		overlayType = "vxlan" // default for backwards compat; wireguard when whitelisted keys exist
		if len(whitelistedKeys) > 0 {
			overlayType = "wireguard"
		}
	}

	// Resolve metrics port from config
	metricsPort := cfg.Engine.MetricsPort

	eng, err := engine.New(&engine.Options{
		StoreBackend:    storeBackend,
		StoreAddress:    storeAddress,
		VPCCIDR:         engineVPCCIDR,
		RegistryPort:    engineRegistryPort,
		GRPCPort:        engineGRPCPort,
		MetricsPort:     metricsPort,
		DataDir:         engineDataDir,
		EtcdUsername:    cfg.Engine.EtcdUsername,
		EtcdPassword:    cfg.Engine.EtcdPassword,
		EtcdCertFile:    cfg.Engine.EtcdCertFile,
		EtcdKeyFile:     cfg.Engine.EtcdKeyFile,
		EtcdCAFile:      cfg.Engine.EtcdCAFile,
		WhitelistedKeys: whitelistedKeys,
		OverlayType:     overlayType,
	})
	if err != nil {
		return err
	}
	defer eng.Close()

	fmt.Printf("Connected to %s\n", storeBackend)

	fmt.Println("========================================")
	fmt.Println("Engine is running. Watching for deployments...")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Printf("  Deploy:      banyan-cli deploy --file banyan.yaml\n")
	fmt.Println("  Agent start: banyan-agent start --node-name <name>")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	if runErr := eng.Run(ctx); runErr != nil {
		return runErr
	}

	fmt.Println("Engine stopped")
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
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
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
	fmt.Println("Managed etcd stopped")
}

func runEngineStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine stopped.")
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
