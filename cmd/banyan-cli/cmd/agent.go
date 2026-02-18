package cmd

import (
	"bytes"
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
  - Container runtime (containerd via nerdctl)
  - Task execution from Engine
  - Health reporting

Commands:
  init    Initialize Agent dependencies (containerd, CNI)
  start   Start the Agent (connects to Engine)
  stop    Stop the Agent
  status  Show Agent status`,
}

var agentInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Agent dependencies",
	RunE:  runAgentInit,
}

var agentStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Banyan Agent",
	Long: `Start the Banyan Agent on this worker node.

This command:
  1. Connects to the Engine (etcd)
  2. Registers this node
  3. Watches for tasks and executes them (pull images, create containers)
  4. Reports task results and heartbeat`,
	RunE: runAgentStart,
}

var agentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Banyan Agent",
	RunE:  runAgentStop,
}

var agentStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Agent status",
	RunE:  runAgentStatus,
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentInitCmd)
	agentCmd.AddCommand(agentStartCmd)
	agentCmd.AddCommand(agentStopCmd)
	agentCmd.AddCommand(agentStatusCmd)

	agentCmd.PersistentFlags().StringVar(&agentDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	agentStartCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "http://localhost:2379", "Engine endpoint")
	agentStartCmd.Flags().StringVar(&agentNodeName, "node-name", "", "Node name (defaults to hostname)")
	agentStartCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")

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

	// Check for nerdctl
	nerdctlPath, err := exec.LookPath("nerdctl")
	if err != nil {
		fmt.Println("Warning: nerdctl not found. Container operations will fail.")
		fmt.Println("Install from: https://github.com/containerd/nerdctl")
	} else {
		fmt.Printf("Container runtime: nerdctl (%s)\n", nerdctlPath)
	}

	// Register node
	node := &NodeRecord{
		Name:      nodeName,
		Status:    "ready",
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
	}
	if err := store.Save(ctx, keyNodes+nodeName, node); err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	fmt.Printf("Node registered: %s\n", nodeName)

	// Write PID file
	pidDir := filepath.Dir(agentPidFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}
	if err := os.WriteFile(agentPidFile, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Println("========================================")
	fmt.Println("Agent is running. Watching for tasks...")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	// Start the task execution loop
	go agentLoop(ctx, store, nodeName)

	// Start heartbeat
	go agentHeartbeat(ctx, store, nodeName)

	<-ctx.Done()

	// Cleanup
	os.Remove(agentPidFile)
	// Mark node as offline
	node.Status = "offline"
	node.LastSeen = time.Now()
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer saveCancel()
	store.Save(saveCtx, keyNodes+nodeName, node)

	fmt.Println("Agent stopped")
	return nil
}

// agentLoop polls etcd for tasks assigned to this agent and executes them.
func agentLoop(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processTasks(ctx, store, nodeName)
		}
	}
}

// processTasks finds and executes pending tasks for this agent.
func processTasks(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	taskPrefix := keyTasks + nodeName + "/"
	keys, err := store.List(ctx, taskPrefix)
	if err != nil {
		return
	}

	for _, key := range keys {
		var task TaskRecord
		if err := store.Get(ctx, key, &task); err != nil {
			continue
		}

		if task.Status != statusPending {
			continue
		}

		// Mark as running
		task.Status = statusRunning
		task.UpdatedAt = time.Now()
		store.Save(ctx, key, &task)

		fmt.Printf("[Agent] Executing task %s: %s (image: %s)\n", task.ID, task.Type, task.Image)

		// Execute the task
		result, err := executeTask(ctx, &task)
		if err != nil {
			task.Status = statusFailed
			task.Error = err.Error()
			task.UpdatedAt = time.Now()
			store.Save(ctx, key, &task)
			fmt.Printf("[Agent] Task %s FAILED: %v\n", task.ID, err)
			continue
		}

		task.Status = statusCompleted
		task.Result = result
		task.UpdatedAt = time.Now()
		store.Save(ctx, key, &task)
		fmt.Printf("[Agent] Task %s COMPLETED (container: %s)\n", task.ID, task.ContainerName)
	}
}

// executeTask runs a container using nerdctl.
func executeTask(ctx context.Context, task *TaskRecord) (*TaskResultRecord, error) {
	switch task.Type {
	case taskTypeCreateAndStart:
		return executeCreateAndStart(ctx, task)
	default:
		return nil, fmt.Errorf("unknown task type: %s", task.Type)
	}
}

// executeCreateAndStart pulls the image and runs the container.
func executeCreateAndStart(ctx context.Context, task *TaskRecord) (*TaskResultRecord, error) {
	// Pull the image
	fmt.Printf("[Agent]   Pulling image %s...\n", task.Image)
	if err := runCommand(ctx, "nerdctl", "pull", task.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Build nerdctl run command
	args := buildNerdctlRunArgs(task)

	// Run the container
	fmt.Printf("[Agent]   Starting container %s...\n", task.ContainerName)
	if err := runCommand(ctx, "nerdctl", args...); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get container ID
	containerID, err := getContainerID(ctx, task.ContainerName)
	if err != nil {
		// Container started but we couldn't get the ID — not fatal
		containerID = "unknown"
	}

	return &TaskResultRecord{
		ContainerID: containerID,
	}, nil
}

// runCommand executes a command and returns an error if it fails.
func runCommand(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s: %s", err, errMsg)
		}
		return err
	}
	return nil
}

// getContainerID retrieves the container ID by name using nerdctl.
func getContainerID(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{.Id}}", containerName)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// agentHeartbeat periodically updates the node's last_seen timestamp in etcd.
func agentHeartbeat(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var node NodeRecord
			if err := store.Get(ctx, keyNodes+nodeName, &node); err != nil {
				continue
			}
			node.LastSeen = time.Now()
			node.Status = "ready"
			store.Save(ctx, keyNodes+nodeName, &node)
		}
	}
}

// --- Agent stop, status, helpers ---

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
			ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
			_, err = client.Status(ctx2, endpoint)
			cancel()
			client.Close()
			if err == nil {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for etcd")
}
