package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/agent"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Agent configuration flags.
var (
	agentEngineEndpoint string
	agentDataDir        string
	agentNodeName       string
	agentPidFile        string
	agentAPIPort        string
	agentAPIAddress     string
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
	Short: "Initialize Agent dependencies",
	RunE:  runAgentInit,
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Banyan Agent",
	Long: `Start the Banyan Agent on this worker node.

This command:
  1. Connects to the Engine via gRPC
  2. Registers this node
  3. Watches for tasks and executes them (pull images, create containers)
  4. Reports task results and heartbeat`,
	RunE: runAgentStart,
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Banyan Agent",
	RunE:  runAgentStop,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Agent status",
	RunE:  runAgentStatus,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)

	rootCmd.PersistentFlags().StringVar(&agentDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	startCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "localhost:50051", "Engine gRPC endpoint")
	startCmd.Flags().StringVar(&agentNodeName, "node-name", "", "Node name (defaults to hostname)")
	startCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")
	startCmd.Flags().StringVar(&agentAPIPort, "api-port", "50052", "Agent gRPC server port")
	startCmd.Flags().StringVar(&agentAPIAddress, "api-address", "", "Agent API address override (e.g. 192.168.1.10:50052)")

	statusCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "localhost:50051", "Engine gRPC endpoint")
	statusCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")
}

func runAgentInit(cmd *cobra.Command, args []string) error {
	fmt.Println(styleTitle.Render("Banyan Agent - Initialization"))
	fmt.Println(styleDim.Render("========================================"))

	if os.Geteuid() != 0 {
		fmt.Println(styleWarn.Render("Warning: Not running as root. Some operations may require sudo."))
	}

	// --- Directory creation ---
	dirs := []string{
		agentDataDir,
		filepath.Join(agentDataDir, "containers"),
		"/etc/cni/net.d",
		"/opt/cni/bin",
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

	// --- Dependency checks ---
	fmt.Println(styleInfo.Render("\nChecking dependencies..."))
	if containerdPath, err := exec.LookPath("containerd"); err != nil {
		fmt.Printf("  %s containerd not found in PATH\n", styleWarn.Render("[WARN]"))
		fmt.Printf("         Install with: apt install containerd\n")
	} else {
		fmt.Printf("  %s containerd found at %s\n", styleOK.Render("[OK]"), containerdPath)
	}

	if nerdctlPath, err := exec.LookPath("nerdctl"); err != nil {
		fmt.Printf("  %s nerdctl not found in PATH\n", styleWarn.Render("[WARN]"))
		fmt.Printf("         Install from: https://github.com/containerd/nerdctl\n")
	} else {
		fmt.Printf("  %s nerdctl found at %s\n", styleOK.Render("[OK]"), nerdctlPath)
	}

	cniPlugins := []string{"bridge", "loopback", "host-local", "portmap"}
	for _, plugin := range cniPlugins {
		pluginPath := "/opt/cni/bin/" + plugin
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			fmt.Printf("  %s CNI plugin %s not found\n", styleWarn.Render("[WARN]"), plugin)
		} else {
			fmt.Printf("  %s CNI plugin %s\n", styleOK.Render("[OK]"), plugin)
		}
	}

	// --- Check for existing config ---
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.Agent.EngineHost != "" && existingCfg.Agent.WGPublicKey != "" {
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("         Engine: %s:%s\n", existingCfg.Agent.EngineHost, existingCfg.Agent.EnginePort)

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
		existingCfg.Agent.WGPrivateKeyFile = ""
		existingCfg.Agent.WGPublicKey = ""
		existingCfg.Agent.EngineHost = ""
		existingCfg.Agent.EnginePort = ""
		existingCfg.Agent.NodeName = ""
		existingCfg.Agent.EngineWGPublicKey = ""
		existingCfg.Agent.Tags = nil
	}

	// --- WireGuard keypair generation ---
	fmt.Println(styleInfo.Render("\nGenerating WireGuard keypair..."))
	if existingCfg.Agent.WGPrivateKeyFile != "" && existingCfg.Agent.WGPublicKey != "" {
		fmt.Printf("  %s WireGuard keypair already configured\n", styleOK.Render("[OK]"))
		fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), existingCfg.Agent.WGPublicKey)
	} else {
		privKey, pubKey, genErr := overlay.GenerateKeyPair()
		if genErr != nil {
			return fmt.Errorf("failed to generate WireGuard keypair: %w", genErr)
		}
		keyPath, writeErr := types.WritePrivateKeyFile(types.DefaultKeysDir, "agent", privKey)
		if writeErr != nil {
			return fmt.Errorf("failed to write private key: %w", writeErr)
		}
		existingCfg.Agent.WGPrivateKeyFile = keyPath
		existingCfg.Agent.WGPublicKey = pubKey
		fmt.Printf("  %s WireGuard keypair generated\n", styleOK.Render("[OK]"))
		fmt.Printf("  %s Private key: %s\n", styleOK.Render("[OK]"), keyPath)
		fmt.Printf("  %s Public key: %s\n", styleInfo.Render("[INFO]"), pubKey)
	}

	// --- Engine connection + config ---
	fmt.Println()
	hostname, _ := os.Hostname()
	engineHost := existingCfg.Agent.EngineHost
	if engineHost == "" {
		engineHost = "localhost"
	}
	enginePort := existingCfg.Agent.EnginePort
	if enginePort == "" {
		enginePort = "50051"
	}
	nodeName := existingCfg.Agent.NodeName
	if nodeName == "" {
		nodeName = hostname
	}
	engineWGPubKey := existingCfg.Agent.EngineWGPublicKey
	var tagsInput string

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
				Title("Node name").
				Description("Unique name for this agent node").
				Value(&nodeName),
			huh.NewInput().
				Title("Engine WireGuard public key").
				Description("Displayed during 'banyan-engine init' (optional, enables encrypted tunnel)").
				Value(&engineWGPubKey),
			huh.NewInput().
				Title("Tags").
				Description("Comma-separated tags for environment isolation (optional)").
				Value(&tagsInput),
		),
	)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Println("\nInitialization cancelled.")
			return nil
		}
		return fmt.Errorf("agent config input: %w", err)
	}

	existingCfg.Agent.EngineHost = engineHost
	existingCfg.Agent.EnginePort = enginePort
	existingCfg.Agent.NodeName = nodeName
	existingCfg.Agent.EngineWGPublicKey = engineWGPubKey
	existingCfg.Agent.Tags = parseTags(tagsInput)

	// --- Save config ---
	if err := types.SaveConfig(configPath, &existingCfg); err != nil {
		fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
	} else {
		fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
	}

	// --- Display next steps for public key auth ---
	if existingCfg.Agent.WGPublicKey != "" {
		keyFileName := existingCfg.Agent.NodeName
		if keyFileName == "" {
			keyFileName, _ = os.Hostname()
		}
		fmt.Println()
		fmt.Println(styleInfo.Render("To whitelist this agent on the engine:"))
		fmt.Printf("  echo '%s' > /etc/banyan/whitelisted-keys/%s.pub\n",
			existingCfg.Agent.WGPublicKey,
			keyFileName)
	}

	fmt.Println()
	fmt.Println(styleDim.Render("========================================"))
	fmt.Println(styleOK.Render("Initialization complete!"))
	fmt.Println()
	fmt.Println(styleInfo.Render("Next step: banyan-agent start"))
	return nil
}

func runAgentStart(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Agent")
	fmt.Println("========================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Load config
	cfg, cfgErr := types.LoadConfig(configPath)
	if cfgErr != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", cfgErr)
	}

	// Get node name: flag > config > hostname
	nodeName := agentNodeName
	if nodeName == "" {
		nodeName = cfg.Agent.NodeName
	}
	if nodeName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
		nodeName = hostname
	}
	fmt.Printf("Node name: %s\n", nodeName)

	// Resolve engine endpoint: explicit flag > config file > error
	if !cmd.Flags().Changed("engine") {
		if cfgEndpoint := types.GetConfigEngineEndpoint(configPath); cfgEndpoint != "" {
			agentEngineEndpoint = cfgEndpoint
		} else {
			return fmt.Errorf("engine endpoint not configured. Run 'banyan-agent init' to set engine host/port, or pass --engine flag")
		}
	}

	// Verify public key auth is configured
	publicKey := cfg.Agent.WGPublicKey
	if publicKey == "" {
		return fmt.Errorf("no authentication configured (missing WireGuard public key). Run 'banyan-agent init' to generate a keypair")
	}

	// Check for nerdctl
	if nerdctlPath, lookErr := exec.LookPath("nerdctl"); lookErr != nil {
		fmt.Println("Warning: nerdctl not found. Container operations will fail.")
		fmt.Println("Install from: https://github.com/containerd/nerdctl")
	} else {
		fmt.Printf("Container runtime: nerdctl (%s)\n", nerdctlPath)
	}

	// Write PID file
	pidDir := filepath.Dir(agentPidFile)
	if mkdirErr := os.MkdirAll(pidDir, 0o755); mkdirErr != nil {
		return fmt.Errorf("failed to create PID directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(agentPidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); writeErr != nil {
		return fmt.Errorf("failed to write PID file: %w", writeErr)
	}

	// Set up WireGuard control tunnel (required when engine public key is configured)
	controlTunnelActive := false
	var agentWGPrivateKey string
	if cfg.Agent.WGPrivateKeyFile != "" {
		var readErr error
		agentWGPrivateKey, readErr = types.ReadPrivateKeyFile(cfg.Agent.WGPrivateKeyFile)
		if readErr != nil {
			return fmt.Errorf("failed to load WireGuard private key: %w", readErr)
		}
	}
	if cfg.Agent.EngineWGPublicKey != "" && agentWGPrivateKey != "" {
		myTunnelIP := types.TunnelIPFromPublicKey(cfg.Agent.WGPublicKey)
		fmt.Printf("Setting up WireGuard control tunnel (%s)...\n", myTunnelIP)
		engineEndpointWG := cfg.Agent.EngineHost + ":" + fmt.Sprintf("%d", types.ControlTunnelPort)
		if tunnelErr := overlay.SetupControlTunnelExec(types.ControlIfaceAgent, agentWGPrivateKey, myTunnelIP, 0); tunnelErr != nil {
			return fmt.Errorf("WireGuard control tunnel setup failed: %w (ensure wireguard kernel module is loaded)", tunnelErr)
		}
		engineIP := net.ParseIP(types.ControlTunnelEngineIP)
		if peerErr := overlay.AddControlPeerExec(types.ControlIfaceAgent, cfg.Agent.EngineWGPublicKey, engineEndpointWG, engineIP); peerErr != nil {
			_ = overlay.CleanupControlTunnelExec(types.ControlIfaceAgent)
			return fmt.Errorf("failed to add engine peer to control tunnel: %w", peerErr)
		}
		controlTunnelActive = true
		// Override gRPC endpoint to route through tunnel
		enginePort := cfg.Agent.EnginePort
		if enginePort == "" {
			enginePort = "50051"
		}
		agentEngineEndpoint = types.ControlTunnelEngineIP + ":" + enginePort
		fmt.Printf("Control tunnel ready, gRPC via %s\n", agentEngineEndpoint)
	}

	// Determine API address — use tunnel IP if control tunnel is active
	apiAddress := agentAPIAddress
	if controlTunnelActive && apiAddress == "" {
		tunnelIP := types.TunnelIPFromPublicKey(cfg.Agent.WGPublicKey)
		apiAddress = tunnelIP.String() + ":" + agentAPIPort
	}

	a, err := agent.New(&agent.Options{
		NodeName:       nodeName,
		EngineEndpoint: agentEngineEndpoint,
		PublicKey:      publicKey,
		WGPrivateKey:   agentWGPrivateKey,
		WGPublicKey:    cfg.Agent.WGPublicKey,
		APIPort:        agentAPIPort,
		APIAddress:     apiAddress,
		PidFile:        agentPidFile,
		Tags:           cfg.Agent.Tags,
	})
	if err != nil {
		return err
	}

	fmt.Println("========================================")
	fmt.Println("Agent is running. Watching for tasks...")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	runErr := a.Run(ctx)

	// Cleanup control tunnel
	if controlTunnelActive {
		_ = overlay.CleanupControlTunnelExec(types.ControlIfaceAgent)
	}

	// Cleanup PID file
	os.Remove(agentPidFile)

	return runErr
}

func runAgentStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Stopping Banyan Agent...")

	if isAgentRunning() {
		pidBytes, err := os.ReadFile(agentPidFile)
		if err != nil {
			return fmt.Errorf("failed to read PID file: %w", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if err != nil {
			return fmt.Errorf("invalid PID: %w", err)
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("failed to find process: %w", err)
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}
		os.Remove(agentPidFile)
		fmt.Println("Agent stopped")
	} else {
		fmt.Println("Agent is not running")
	}

	return nil
}

func runAgentStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Agent - Status")
	fmt.Println("========================================")

	fmt.Print("Agent: ")
	if isAgentRunning() {
		pidBytes, _ := os.ReadFile(agentPidFile)
		fmt.Printf("RUNNING (PID: %s)\n", strings.TrimSpace(string(pidBytes)))
	} else {
		fmt.Println("NOT RUNNING")
	}

	// Resolve engine endpoint: explicit flag > config file > default
	if !cmd.Flags().Changed("engine") {
		if cfgEndpoint := types.GetConfigEngineEndpoint(configPath); cfgEndpoint != "" {
			agentEngineEndpoint = cfgEndpoint
		}
	}

	fmt.Print("Engine connection: ")
	statusCfg, _ := types.LoadConfig(configPath)
	statusPubKey := statusCfg.Agent.WGPublicKey
	if statusPubKey == "" {
		fmt.Println("NOT CONFIGURED (no public key)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, connErr := agent.NewEngineClient(agentEngineEndpoint, statusPubKey)
		if connErr != nil {
			fmt.Printf("FAILED (%v)\n", connErr)
		} else {
			defer client.Close()
			if err := client.Health(ctx); err != nil {
				fmt.Printf("FAILED (%v)\n", err)
			} else {
				fmt.Println("OK")
				fmt.Println("Authentication: OK")
			}
		}
	}

	fmt.Println("========================================")
	return nil
}

func isAgentRunning() bool {
	if _, err := os.Stat(agentPidFile); os.IsNotExist(err) {
		return false
	}
	pidBytes, err := os.ReadFile(agentPidFile)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// parseTags splits a comma-separated string into a trimmed tag slice.
// Returns nil for empty input.
func parseTags(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var tags []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
