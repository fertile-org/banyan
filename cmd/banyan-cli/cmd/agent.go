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

	"github.com/fertile-org/banyan/pkg/vpc/cni"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Agent configuration
var (
	agentEngineEndpoint string
	agentDataDir        string
	agentNodeName       string
	agentPidFile        string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage the Banyan Agent (worker node)",
	Long: `Manage the Banyan Agent, which runs on worker nodes.

The Agent manages:
  - Container runtime (containerd)
  - CNI networking for pods
  - Local IPAM lease management
  - Security policy enforcement

Commands:
  init    Initialize Agent dependencies (containerd, CNI)
  start   Start the Agent (connects to Engine)
  stop    Stop the Agent
  status  Show Agent status`,
}

var agentInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Agent dependencies",
	Long: `Initialize the Banyan Agent environment.

This command:
  1. Creates required directories (/var/lib/banyan, /etc/cni/net.d)
  2. Checks if containerd is available
  3. Sets up CNI plugin configuration

Run this once before starting the Agent.

Next step: banyan-cli agent start --engine http://engine-host:2379`,
	RunE: runAgentInit,
}

var agentStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Banyan Agent",
	Long: `Start the Banyan Agent on this worker node.

This command:
  1. Connects to the Engine
  2. Registers this node with the Engine
  3. Initializes CNI networking
  4. Starts listening for container tasks`,
	RunE: runAgentStart,
}

var agentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Banyan Agent",
	Long:  `Stop the running Banyan Agent.`,
	RunE:  runAgentStop,
}

var agentStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Agent status",
	Long:  `Show the status of the Banyan Agent.`,
	RunE:  runAgentStatus,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentInitCmd)
	agentCmd.AddCommand(agentStartCmd)
	agentCmd.AddCommand(agentStopCmd)
	agentCmd.AddCommand(agentStatusCmd)

	// Common flags
	agentCmd.PersistentFlags().StringVar(&agentDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	// Start command flags
	agentStartCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "http://localhost:2379", "Engine endpoint")
	agentStartCmd.Flags().StringVar(&agentNodeName, "node-name", "", "Node name (defaults to hostname)")
	agentStartCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")

	// Status command flags
	agentStatusCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "http://localhost:2379", "Engine endpoint")
	agentStatusCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")
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
		if err := os.MkdirAll(dir, 0755); err != nil {
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

	fmt.Println("\n3. Checking CNI plugins...")
	cniPlugins := []string{"bridge", "loopback", "host-local", "portmap"}
	for _, plugin := range cniPlugins {
		pluginPath := filepath.Join("/opt/cni/bin", plugin)
		if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
			fmt.Printf("   [WARN] %s not found\n", plugin)
		} else {
			fmt.Printf("   [OK] %s\n", plugin)
		}
	}

	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Println("\nNext step: banyan-cli agent start --engine http://engine-host:2379")

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

	// Wait for engine connection
	fmt.Printf("Connecting to Engine at %s...\n", agentEngineEndpoint)
	if err := waitForAgentEtcd(ctx, agentEngineEndpoint, 30*time.Second); err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}
	fmt.Println("Connected to Engine")

	// Initialize storage
	store, err := storage.NewEtcdStore([]string{agentEngineEndpoint}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to etcd: %w", err)
	}
	fmt.Println("Storage initialized")

	// Initialize VPC components
	resolver := security.NewRuntimeServiceResolver(store)
	securityManager := security.NewManager(resolver, false)

	fmt.Println("VPC components initialized: Security")

	// Initialize CNI runtime
	cniRuntime := cni.NewRuntime(store, securityManager)
	_ = cniRuntime // Will be used when containers are created
	fmt.Println("CNI runtime initialized")

	// Register node with Engine
	if err := registerAgentNode(ctx, store, nodeName); err != nil {
		fmt.Printf("Warning: Node registration: %v\n", err)
	}

	// Write PID file
	pidDir := filepath.Dir(agentPidFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}
	if err := os.WriteFile(agentPidFile, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Println("========================================")
	fmt.Println("Agent is running. Ready to receive container tasks.")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	<-ctx.Done()

	// Cleanup
	os.Remove(agentPidFile)
	fmt.Println("Agent stopped")
	return nil
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

	fmt.Print("Engine connection: ")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{agentEngineEndpoint},
		DialTimeout: 5 * time.Second,
	})
	if err == nil {
		defer client.Close()
		_, err = client.Status(ctx, agentEngineEndpoint)
		if err == nil {
			fmt.Println("OK")
		} else {
			fmt.Printf("FAILED (%v)\n", err)
		}
	} else {
		fmt.Printf("FAILED (%v)\n", err)
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

func waitForAgentEtcd(ctx context.Context, endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		client, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{endpoint},
			DialTimeout: 2 * time.Second,
		})
		if err == nil {
			defer client.Close()
			ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err = client.Status(ctx2, endpoint)
			cancel()
			if err == nil {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for etcd")
}

// AgentNode represents a registered agent node
type AgentNode struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func registerAgentNode(ctx context.Context, store storage.StateStore, nodeName string) error {
	node := &AgentNode{
		ID:        nodeName,
		Name:      nodeName,
		Status:    "ready",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	return store.Save(ctx, "/nodes/"+nodeName, node)
}
