package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
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

// trackedContainer holds info about containers created by this agent.
type trackedContainer struct {
	containerName string
	taskID        string
}

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

	// Generate session token for engine→agent auth
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("failed to generate session token: %w", err)
	}
	sessionToken := hex.EncodeToString(tokenBytes)

	// Connect to engine via gRPC
	fmt.Printf("Connecting to Engine at %s...\n", agentEngineEndpoint)
	client, err := newEngineClient(agentEngineEndpoint, password)
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}
	defer client.Close()

	// Wait for engine to be ready by retrying Health check
	fmt.Println("Waiting for Engine gRPC to be ready...")
	if waitErr := waitForEngineGRPC(ctx, client, 30*time.Second); waitErr != nil {
		return fmt.Errorf("engine not ready: %w", waitErr)
	}
	fmt.Println("Connected to Engine")

	// Check for nerdctl
	nerdctlPath, err := exec.LookPath("nerdctl")
	if err != nil {
		fmt.Println("Warning: nerdctl not found. Container operations will fail.")
		fmt.Println("Install from: https://github.com/containerd/nerdctl")
	} else {
		fmt.Printf("Container runtime: nerdctl (%s)\n", nerdctlPath)
	}

	// Determine API address for this agent
	apiAddr := agentAPIAddress
	if apiAddr == "" {
		apiAddr = nodeName + ":" + agentAPIPort
	}

	// Register node
	registryURL, err := client.Register(ctx, nodeName, apiAddr, sessionToken)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	fmt.Printf("Node registered: %s (registry: %s)\n", nodeName, registryURL)

	// Write PID file
	pidDir := filepath.Dir(agentPidFile)
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}
	if err := os.WriteFile(agentPidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Println("========================================")
	fmt.Println("Agent is running. Watching for tasks...")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	// Container tracker (filled by task execution)
	containers := &containerTracker{}

	// Start the task execution loop
	go agentLoop(ctx, client, nodeName, containers)

	// Start heartbeat
	go agentHeartbeat(ctx, client, nodeName, sessionToken)

	// Start container health monitoring
	go containerHealthLoop(ctx, client, nodeName, containers)

	// Start agent gRPC server for log streaming
	go startAgentGRPC(ctx, &NerdctlLogProvider{}, agentAPIPort, sessionToken)

	<-ctx.Done()

	// Cleanup
	os.Remove(agentPidFile)
	client.Close()

	fmt.Println("Agent stopped")
	return nil
}

// containerTracker tracks containers created by this agent.
type containerTracker struct {
	containers []trackedContainer
	mu         sync.Mutex
}

func (t *containerTracker) Add(containerName, taskID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.containers = append(t.containers, trackedContainer{
		containerName: containerName,
		taskID:        taskID,
	})
}

func (t *containerTracker) List() []trackedContainer {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]trackedContainer, len(t.containers))
	copy(result, t.containers)
	return result
}

// agentLoop polls the engine for tasks assigned to this agent and executes them.
func agentLoop(ctx context.Context, client *engineClient, nodeName string, containers *containerTracker) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processTasks(ctx, client, nodeName, containers)
		}
	}
}

// processTasks polls and executes pending tasks for this agent.
func processTasks(ctx context.Context, client *engineClient, nodeName string, containers *containerTracker) {
	tasks, err := client.PollTasks(ctx, nodeName)
	if err != nil {
		return
	}

	for _, pbTask := range tasks {
		// Report running (best-effort)
		if err := client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusRunning, "", pbTask.ContainerName, nil); err != nil {
			fmt.Printf("[Agent] WARNING: failed to report running for task %s: %v\n", pbTask.Id, err)
		}

		fmt.Printf("[Agent] Executing task %s: %s (image: %s)\n", pbTask.Id, pbTask.Type, pbTask.Image)

		task := pbTaskToLocal(pbTask)
		result, err := executeTask(ctx, task)
		if err != nil {
			if reportErr := client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusFailed, err.Error(), pbTask.ContainerName, nil); reportErr != nil {
				fmt.Printf("[Agent] WARNING: failed to report failure for task %s: %v\n", pbTask.Id, reportErr)
			}
			fmt.Printf("[Agent] Task %s FAILED: %v\n", pbTask.Id, err)
			continue
		}

		var pbResult *banyanpb.TaskResult
		if result != nil {
			pbResult = &banyanpb.TaskResult{ContainerId: result.ContainerID}
		}
		if err := client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusCompleted, "", pbTask.ContainerName, pbResult); err != nil {
			fmt.Printf("[Agent] WARNING: failed to report completion for task %s: %v\n", pbTask.Id, err)
		}

		// Track containers created by create_and_start tasks
		if pbTask.Type == types.TaskTypeCreateAndStart {
			containers.Add(pbTask.ContainerName, pbTask.Id)
		}

		fmt.Printf("[Agent] Task %s COMPLETED (container: %s)\n", pbTask.Id, pbTask.ContainerName)
	}
}

// pbTaskToLocal converts a protobuf TaskRecord to a local types.TaskRecord for execution.
func pbTaskToLocal(pb *banyanpb.TaskRecord) *types.TaskRecord {
	return &types.TaskRecord{
		ID:            pb.Id,
		DeploymentID:  pb.DeploymentId,
		ServiceName:   pb.ServiceName,
		ReplicaIndex:  int(pb.ReplicaIndex),
		AgentID:       pb.AgentId,
		Type:          pb.Type,
		Status:        pb.Status,
		Image:         pb.Image,
		ContainerName: pb.ContainerName,
		Ports:         pb.Ports,
		Environment:   pb.Environment,
		Command:       pb.Command,
	}
}

func executeTask(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
	switch task.Type {
	case types.TaskTypeCreateAndStart:
		return executeCreateAndStart(ctx, task)
	case types.TaskTypeStopAndRemove:
		return executeStopAndRemove(ctx, task)
	default:
		return nil, fmt.Errorf("unknown task type: %s", task.Type)
	}
}

func executeCreateAndStart(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
	fmt.Printf("[Agent]   Pulling image %s...\n", task.Image)
	if err := runCommand(ctx, "nerdctl", "pull", "--insecure-registry", task.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %v", task.Image, err)
	}

	args := buildNerdctlRunArgs(task)

	fmt.Printf("[Agent]   Starting container %s...\n", task.ContainerName)
	if err := runCommand(ctx, "nerdctl", args...); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	containerID, err := getContainerID(ctx, task.ContainerName)
	if err != nil {
		containerID = "unknown"
	}

	return &types.TaskResultRecord{
		ContainerID: containerID,
	}, nil
}

func executeStopAndRemove(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
	fmt.Printf("[Agent]   Removing container %s...\n", task.ContainerName)

	rmCmd := exec.CommandContext(ctx, "nerdctl", "rm", "-f", task.ContainerName) //nolint:gosec // container name comes from engine
	var stderr bytes.Buffer
	rmCmd.Stderr = &stderr
	if err := rmCmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "No such container") {
			fmt.Printf("[Agent]   Container %s already removed\n", task.ContainerName)
			return &types.TaskResultRecord{}, nil
		}
		return nil, fmt.Errorf("failed to remove container: %s", errMsg)
	}

	return &types.TaskResultRecord{}, nil
}

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

func getContainerID(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{.Id}}", containerName)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// buildNerdctlRunArgs builds the argument list for "nerdctl run" from a task.
func buildNerdctlRunArgs(task *types.TaskRecord) []string {
	args := []string{"run", "-d", "--name", task.ContainerName}

	for _, port := range task.Ports {
		args = append(args, "-p", port)
	}
	for _, env := range task.Environment {
		args = append(args, "-e", env)
	}

	args = append(args, task.Image)
	args = append(args, task.Command...)
	return args
}

// getContainerStatus runs nerdctl inspect to get the container's current status.
func getContainerStatus(ctx context.Context, containerName string) string {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{.State.Status}}", containerName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "not_found"
	}
	status := strings.TrimSpace(stdout.String())
	if status == "" {
		return "not_found"
	}
	return status
}

func agentHeartbeat(ctx context.Context, client *engineClient, nodeName, sessionToken string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := client.Heartbeat(ctx, nodeName, sessionToken); err != nil {
				fmt.Printf("[Agent] WARNING: heartbeat failed: %v\n", err)
			}
		}
	}
}

func containerHealthLoop(ctx context.Context, client *engineClient, nodeName string, containers *containerTracker) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkContainerHealth(ctx, client, nodeName, containers)
		}
	}
}

func checkContainerHealth(ctx context.Context, client *engineClient, nodeName string, containers *containerTracker) {
	tracked := containers.List()
	if len(tracked) == 0 {
		return
	}

	var statuses []*banyanpb.ContainerStatus
	for _, c := range tracked {
		status := getContainerStatus(ctx, c.containerName)
		statuses = append(statuses, &banyanpb.ContainerStatus{
			ContainerName: c.containerName,
			Status:        status,
		})
	}

	if err := client.ReportContainerHealth(ctx, nodeName, statuses); err != nil {
		fmt.Printf("[Agent] WARNING: failed to report container health: %v\n", err)
	}
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
		client, err := newEngineClient(agentEngineEndpoint, password)
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

// waitForEngineGRPC retries a health check to verify the engine gRPC server is ready.
func waitForEngineGRPC(ctx context.Context, client *engineClient, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		hbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := client.Health(hbCtx)
		cancel()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout waiting for engine gRPC")
}
