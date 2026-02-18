package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
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
  1. Connects to the Engine (etcd)
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

	startCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "http://localhost:2379", "Engine endpoint")
	startCmd.Flags().StringVar(&agentNodeName, "node-name", "", "Node name (defaults to hostname)")
	startCmd.Flags().StringVar(&agentPidFile, "pid-file", "/var/run/banyan-agent.pid", "Agent PID file")
	startCmd.Flags().StringVar(&agentAPIPort, "api-port", "9090", "Agent API server port")
	startCmd.Flags().StringVar(&agentAPIAddress, "api-address", "", "Agent API address override (e.g. 192.168.1.10:9090)")

	statusCmd.Flags().StringVar(&agentEngineEndpoint, "engine", "http://localhost:2379", "Engine endpoint")
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

		fmt.Print("   Engine etcd port (default: 2379): ")
		enginePort := types.ReadLine()
		if enginePort == "" {
			enginePort = "2379"
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

		if err := types.SaveConfig(configPath, cfg); err != nil {
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

	// Verify authentication
	if err := types.VerifyAuth(ctx, store, configPath); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

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
	node := &types.NodeRecord{
		Name:       nodeName,
		Status:     "ready",
		APIAddress: apiAddr,
		LastSeen:   time.Now(),
		CreatedAt:  time.Now(),
	}
	if err := store.Save(ctx, types.KeyNodes+nodeName, node); err != nil {
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

	// Start container health monitoring
	go containerHealthLoop(ctx, store, nodeName)

	// Start agent HTTP server for log streaming
	go agentHTTPServer(ctx, &NerdctlLogProvider{}, agentAPIPort)

	<-ctx.Done()

	// Cleanup: update status then close etcd connection to stop goroutine warnings
	os.Remove(agentPidFile)
	node.Status = "offline"
	node.LastSeen = time.Now()
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 2*time.Second)
	store.Save(saveCtx, types.KeyNodes+nodeName, node)
	saveCancel()
	store.Close()

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
	taskPrefix := types.KeyTasks + nodeName + "/"
	keys, err := store.List(ctx, taskPrefix)
	if err != nil {
		return
	}

	for _, key := range keys {
		var task types.TaskRecord
		if err := store.Get(ctx, key, &task); err != nil {
			continue
		}

		if task.Status != types.StatusPending {
			continue
		}

		// Mark as running
		task.Status = types.StatusRunning
		task.UpdatedAt = time.Now()
		store.Save(ctx, key, &task)

		fmt.Printf("[Agent] Executing task %s: %s (image: %s)\n", task.ID, task.Type, task.Image)

		result, err := executeTask(ctx, &task)
		if err != nil {
			task.Status = types.StatusFailed
			task.Error = err.Error()
			task.UpdatedAt = time.Now()
			store.Save(ctx, key, &task)
			fmt.Printf("[Agent] Task %s FAILED: %v\n", task.ID, err)
			continue
		}

		task.Status = types.StatusCompleted
		task.Result = result
		task.UpdatedAt = time.Now()
		store.Save(ctx, key, &task)
		fmt.Printf("[Agent] Task %s COMPLETED (container: %s)\n", task.ID, task.ContainerName)
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

	rmCmd := exec.CommandContext(ctx, "nerdctl", "rm", "-f", task.ContainerName)
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

func agentHeartbeat(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var node types.NodeRecord
			if err := store.Get(ctx, types.KeyNodes+nodeName, &node); err != nil {
				continue
			}
			node.LastSeen = time.Now()
			node.Status = "ready"
			store.Save(ctx, types.KeyNodes+nodeName, &node)
		}
	}
}

func containerHealthLoop(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			checkContainerHealth(ctx, store, nodeName)
		}
	}
}

func checkContainerHealth(ctx context.Context, store *storage.EtcdStore, nodeName string) {
	taskPrefix := types.KeyTasks + nodeName + "/"
	keys, err := store.List(ctx, taskPrefix)
	if err != nil {
		return
	}

	for _, key := range keys {
		var task types.TaskRecord
		if err := store.Get(ctx, key, &task); err != nil {
			continue
		}

		if task.Type != types.TaskTypeCreateAndStart || task.Status != types.StatusCompleted {
			continue
		}

		status := getContainerStatus(ctx, task.ContainerName)
		task.ContainerStatus = status
		task.ContainerCheckedAt = time.Now()
		store.Save(ctx, key, &task)
	}
}

// agentHTTPServer starts an HTTP server for log streaming.
func agentHTTPServer(ctx context.Context, logProvider types.LogProvider, apiPort string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/logs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		containerName := strings.TrimPrefix(r.URL.Path, "/v1/logs/")
		containerName = strings.TrimSuffix(containerName, "/")
		if containerName == "" {
			http.Error(w, "container name required", http.StatusBadRequest)
			return
		}

		query := r.URL.Query()
		follow := query.Get("follow") == "true"
		tail := 0
		if tailStr := query.Get("tail"); tailStr != "" {
			if v, err := strconv.Atoi(tailStr); err == nil && v > 0 {
				tail = v
			}
		}

		opts := types.LogOptions{
			Follow: follow,
			Tail:   tail,
		}

		reqCtx := r.Context()
		reader, err := logProvider.StreamLogs(reqCtx, containerName, opts)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to stream logs: %v", err), http.StatusInternalServerError)
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)

		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				if readErr != io.EOF {
					fmt.Fprintf(w, "\n[error reading logs: %v]\n", readErr)
					if canFlush {
						flusher.Flush()
					}
				}
				return
			}
		}
	})

	var handler http.Handler = mux
	if password := types.GetConfigPassword(configPath); password != "" {
		handler = types.BasicAuthMiddleware(mux, password)
	}

	server := &http.Server{
		Addr:    net.JoinHostPort("", apiPort),
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("Agent API server listening on :%s\n", apiPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Agent API server error: %v\n", err)
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

			if password := types.GetConfigPassword(configPath); password != "" {
				store, storeErr := storage.NewEtcdStore([]string{agentEngineEndpoint}, "/banyan")
				if storeErr == nil {
					if authErr := types.VerifyAuth(ctx, store, configPath); authErr != nil {
						fmt.Printf("Authentication: FAILED (%v)\n", authErr)
					} else {
						fmt.Println("Authentication: OK")
					}
				}
			}
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
