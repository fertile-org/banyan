package cmd

import (
	"context"
	"errors"
	"fmt"
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
)

// Agent configuration flags.
var (
	agentEngineEndpoint string
	agentDataDir        string
	agentNodeName       string
	agentPidFile        string
	agentAPIPort        string
	agentAPIAddress     string
	agentInitPassword   string
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

	// Init flags
	initCmd.Flags().StringVar(&agentInitPassword, "password", "", "Cluster password (non-interactive mode)")

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

	// --- Engine connection + token exchange ---
	existingCfg, _ := types.LoadConfig(configPath)
	fmt.Println()
	switch {
	case existingCfg.Agent.AuthToken != "" && existingCfg.Agent.EngineHost != "":
		fmt.Printf("  %s Config already exists at %s\n", styleOK.Render("[OK]"), configPath)
		fmt.Printf("         Engine: %s:%s (token set)\n", existingCfg.Agent.EngineHost, existingCfg.Agent.EnginePort)
	case agentInitPassword != "" && existingCfg.Agent.EngineHost != "":
		// Password provided via --password flag — exchange token non-interactively.
		hostname, _ := os.Hostname()
		nodeName := existingCfg.Agent.NodeName
		if nodeName == "" {
			nodeName = hostname
		}
		enginePort := existingCfg.Agent.EnginePort
		if enginePort == "" {
			enginePort = "50051"
		}

		engineAddr := fmt.Sprintf("%s:%s", existingCfg.Agent.EngineHost, enginePort)
		fmt.Printf("  %s Connecting to engine at %s...\n", styleInfo.Render("[..]"), engineAddr)

		client, connErr := agent.NewEngineClientWithPassword(engineAddr, agentInitPassword)
		if connErr != nil {
			return fmt.Errorf("failed to connect to engine: %w", connErr)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		token, tokenErr := client.ExchangeToken(ctx, nodeName, "agent")
		if tokenErr != nil {
			fmt.Printf("  %s Token exchange failed: %v\n", styleWarn.Render("[FAIL]"), tokenErr)
			return fmt.Errorf("token exchange failed: %w", tokenErr)
		}

		fmt.Printf("  %s Token obtained from engine\n", styleOK.Render("[OK]"))

		existingCfg.Agent.AuthToken = token
		existingCfg.Agent.NodeName = nodeName
		existingCfg.Agent.EnginePort = enginePort

		if err := types.SaveConfig(configPath, &existingCfg); err != nil {
			fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
		} else {
			fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
		}
	default:
		hostname, _ := os.Hostname()
		engineHost := "localhost"
		enginePort := "50051"
		nodeName := hostname
		var password string
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
					Title("Tags").
					Description("Comma-separated tags for environment isolation (optional)").
					Value(&tagsInput),
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
			return fmt.Errorf("agent config input: %w", err)
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

		token, tokenErr := client.ExchangeToken(ctx, nodeName, "agent")
		if tokenErr != nil {
			fmt.Printf("  %s Token exchange failed: %v\n", styleWarn.Render("[FAIL]"), tokenErr)
			return fmt.Errorf("token exchange failed: %w", tokenErr)
		}

		fmt.Printf("  %s Token obtained from engine\n", styleOK.Render("[OK]"))

		cfg := existingCfg
		cfg.Agent = types.AgentConfig{
			EngineHost: engineHost,
			EnginePort: enginePort,
			AuthToken:  token,
			NodeName:   nodeName,
			Tags:       parseTags(tagsInput),
		}

		if err := types.SaveConfig(configPath, &cfg); err != nil {
			fmt.Printf("  %s Failed to save config: %v\n", styleWarn.Render("[WARN]"), err)
		} else {
			fmt.Printf("  %s Config saved to %s\n", styleOK.Render("[OK]"), configPath)
		}
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
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
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

	// Get auth token from config
	authToken := types.GetAgentAuthToken(configPath)
	if authToken == "" {
		return fmt.Errorf("auth token not configured. Run 'banyan-agent init' to obtain a token from the engine")
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

	a, err := agent.New(&agent.Options{
		NodeName:       nodeName,
		EngineEndpoint: agentEngineEndpoint,
		AuthToken:      authToken,
		APIPort:        agentAPIPort,
		APIAddress:     agentAPIAddress,
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
	authToken := types.GetAgentAuthToken(configPath)
	if authToken == "" {
		fmt.Println("NOT CONFIGURED (no auth token)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := agent.NewEngineClient(agentEngineEndpoint, authToken)
		if err != nil {
			fmt.Printf("FAILED (%v)\n", err)
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
