package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

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
)

// configPath is the default path to the Banyan config file.
var configPath = types.DefaultConfigPath

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
	fmt.Println("Banyan Agent - Initialization")
	fmt.Println("========================================")

	if os.Geteuid() != 0 {
		fmt.Println("Warning: Not running as root. Some operations may require sudo.")
	}

	dirs := []string{
		agentDataDir,
		filepath.Join(agentDataDir, "containers"),
		"/etc/cni/net.d",
		"/opt/cni/bin",
		"/var/run",
	}

	fmt.Println("\n1. Creating directories...")
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Printf("   [WARN] Failed to create %s: %v\n", dir, err)
		} else {
			fmt.Printf("   [OK] %s\n", dir)
		}
	}

	fmt.Println("\n2. Checking containerd...")
	containerdPath, err := exec.LookPath("containerd")
	if err != nil {
		fmt.Println("   [WARN] containerd not found in PATH")
		fmt.Println("   Install with: apt install containerd")
	} else {
		fmt.Printf("   [OK] containerd found at %s\n", containerdPath)
	}

	fmt.Println("\n3. Checking nerdctl...")
	nerdctlPath, err := exec.LookPath("nerdctl")
	if err != nil {
		fmt.Println("   [WARN] nerdctl not found in PATH")
		fmt.Println("   Install from: https://github.com/containerd/nerdctl")
	} else {
		fmt.Printf("   [OK] nerdctl found at %s\n", nerdctlPath)
	}

	fmt.Println("\n4. Checking CNI plugins...")
	cniPlugins := []string{"bridge", "loopback", "host-local", "portmap"}
	for _, plugin := range cniPlugins {
		pluginPath := "/opt/cni/bin/" + plugin
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			fmt.Printf("   [WARN] %s not found\n", plugin)
		} else {
			fmt.Printf("   [OK] %s\n", plugin)
		}
	}

	fmt.Println("\n5. Configuring engine connection and authentication...")
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.Security.Password != "" && existingCfg.Agent.EngineHost != "" {
		fmt.Printf("   [OK] Config already exists at %s\n", configPath)
		fmt.Printf("   Engine: %s:%s (password set)\n", existingCfg.Agent.EngineHost, existingCfg.Agent.EnginePort)
	} else {
		fmt.Print("   Engine host (default: localhost): ")
		engineHost := types.ReadLine()
		if engineHost == "" {
			engineHost = "localhost"
		}
		fmt.Printf("   [OK] Engine host: %s\n", engineHost)

		fmt.Print("   Engine gRPC port (default: 50051): ")
		enginePort := types.ReadLine()
		if enginePort == "" {
			enginePort = "50051"
		}
		fmt.Printf("   [OK] Engine port: %s\n", enginePort)

		fmt.Println("\n6. Configuring authentication...")
		fmt.Print("   Enter cluster password (leave empty to skip): ")
		password := types.ReadLine()

		// Load existing config to preserve other sections (cli, engine)
		cfg := existingCfg
		cfg.Agent = types.AgentConfig{
			EngineHost: engineHost,
			EnginePort: enginePort,
		}
		if password != "" {
			cfg.Security = types.SecurityConfig{
				AuthType: "password",
				Password: password,
			}
		}

		if err := types.SaveConfig(configPath, &cfg); err != nil {
			fmt.Printf("   [WARN] Failed to save config: %v\n", err)
		} else {
			fmt.Printf("   [OK] Config saved to %s\n", configPath)
		}
	}

	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Println("\nNext step: banyan-agent start")
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

	// Get node name
	nodeName := agentNodeName
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

	// Get password from config
	password := types.GetConfigPassword(configPath)
	if password == "" {
		return fmt.Errorf("authentication required. Set password in %s", configPath)
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
		Password:       password,
		APIPort:        agentAPIPort,
		APIAddress:     agentAPIAddress,
		PidFile:        agentPidFile,
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
	password := types.GetConfigPassword(configPath)
	if password == "" {
		fmt.Println("NOT CONFIGURED (no password)")
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := agent.NewEngineClient(agentEngineEndpoint, password)
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
