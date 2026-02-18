package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Engine configuration flags.
var (
	engineEtcdEndpoints  string
	engineVPCCIDR        string
	engineDataDir        string
	engineEtcdDataDir    string
	engineEtcdPidFile    string
	engineEtcdLogFile    string
	engineEtcdClientURLs string
	engineRegistryPort   string
	engineAPIPort        string
)

// configPath is the default path to the Banyan config file.
var configPath = types.DefaultConfigPath

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
  1. Starts etcd if not already running
  2. Initializes VPC networking (IPAM, DNS, Security)
  3. Starts the HTTP API server
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

	startCmd.Flags().StringVar(&engineEtcdEndpoints, "etcd", "http://localhost:2379", "Etcd endpoints")
	startCmd.Flags().StringVar(&engineVPCCIDR, "vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	startCmd.Flags().StringVar(&engineEtcdDataDir, "etcd-data-dir", "/var/lib/banyan/etcd", "Etcd data directory")
	startCmd.Flags().StringVar(&engineEtcdPidFile, "etcd-pid-file", "/var/run/banyan-etcd.pid", "Etcd PID file")
	startCmd.Flags().StringVar(&engineEtcdLogFile, "etcd-log-file", "/var/log/banyan-etcd.log", "Etcd log file")
	startCmd.Flags().StringVar(&engineEtcdClientURLs, "etcd-client-urls", "http://0.0.0.0:2379", "Etcd client URLs")
	startCmd.Flags().StringVar(&engineRegistryPort, "registry-port", "5000", "Embedded OCI registry port")
	startCmd.Flags().StringVar(&engineAPIPort, "api-port", "8443", "Engine HTTP API port")

	statusCmd.Flags().StringVar(&engineEtcdEndpoints, "etcd", "http://localhost:2379", "Etcd endpoints")
	statusCmd.Flags().StringVar(&engineEtcdPidFile, "etcd-pid-file", "/var/run/banyan-etcd.pid", "Etcd PID file")
}

func runEngineInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine - Initialization")
	fmt.Println("========================================")

	if os.Geteuid() != 0 {
		fmt.Println("Warning: Not running as root. Some operations may require sudo.")
	}

	dirs := []string{
		engineDataDir,
		filepath.Join(engineDataDir, "etcd"),
		filepath.Join(engineDataDir, "vpc"),
		"/var/log",
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

	fmt.Println("\n2. Checking etcd...")
	etcdPath, err := exec.LookPath("etcd")
	if err != nil {
		fmt.Println("   [WARN] etcd not found in PATH")
		fmt.Println("   Install with: apt install etcd-server")
	} else {
		fmt.Printf("   [OK] etcd found at %s\n", etcdPath)
	}

	fmt.Println("\n3. Configuring authentication...")
	existingCfg, _ := types.LoadConfig(configPath)
	if existingCfg.Security.Password != "" {
		fmt.Printf("   [OK] Config already exists at %s (password set)\n", configPath)
	} else {
		fmt.Print("   Enter cluster password (leave empty to skip): ")
		password := types.ReadLine()
		if password != "" {
			// Load existing config to preserve other sections (agent, cli)
			cfg := existingCfg
			cfg.Security = types.SecurityConfig{
				AuthType: "password",
				Password: password,
			}
			if err := types.SaveConfig(configPath, cfg); err != nil {
				fmt.Printf("   [WARN] Failed to save config: %v\n", err)
			} else {
				fmt.Printf("   [OK] Config saved to %s\n", configPath)
			}
		} else {
			fmt.Println("   [SKIP] No password set")
		}
	}

	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Println("\nNext step: banyan-engine start")
	return nil
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

	// Read API port from config if not overridden by flag
	if !cmd.Flags().Changed("api-port") {
		cfg, err := types.LoadConfig(configPath)
		if err == nil && cfg.Engine.APIPort != "" {
			engineAPIPort = cfg.Engine.APIPort
		}
	}

	// Start etcd if not running
	if err := ensureEngineEtcdRunning(); err != nil {
		return fmt.Errorf("failed to ensure etcd is running: %w", err)
	}

	// Wait for etcd
	fmt.Println("Waiting for etcd to be ready...")
	if err := waitForEngineEtcd(ctx, engineEtcdEndpoints, 30*time.Second); err != nil {
		return fmt.Errorf("etcd not ready: %w", err)
	}
	fmt.Println("etcd is ready")

	// Initialize storage
	fmt.Printf("Connecting to etcd at %s...\n", engineEtcdEndpoints)
	store, err := storage.NewEtcdStore([]string{engineEtcdEndpoints}, "/banyan")
	if err != nil {
		return fmt.Errorf("failed to connect to etcd: %w", err)
	}
	fmt.Println("Connected to etcd")

	// Store auth hash if password is configured
	cfg, err := types.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", err)
	} else if cfg.Security.Password != "" {
		hash := types.HashPassword(cfg.Security.Password)
		if err := store.Save(ctx, types.KeyAuthHash, hash); err != nil {
			return fmt.Errorf("failed to store auth hash: %w", err)
		}
		fmt.Println("Password authentication enabled")
	}

	// Initialize VPC network
	fmt.Printf("Initializing VPC network with CIDR %s...\n", engineVPCCIDR)
	if err := vpc.InitializeNetwork(ctx, []string{engineEtcdEndpoints}, engineVPCCIDR); err != nil {
		fmt.Printf("Warning: VPC initialization: %v\n", err)
	}
	fmt.Println("VPC initialized")

	// Start embedded OCI registry
	fmt.Printf("Starting OCI registry on port %s...\n", engineRegistryPort)
	registryListener, err := startRegistry(ctx, engineRegistryPort)
	if err != nil {
		return fmt.Errorf("failed to start registry: %w", err)
	}
	fmt.Printf("OCI registry listening on :%s\n", engineRegistryPort)

	// Determine engine IP and store registry URL in etcd
	engineIP, err := determineEngineIP()
	if err != nil {
		return fmt.Errorf("failed to determine engine IP: %w", err)
	}
	registryURL := fmt.Sprintf("%s:%s", engineIP, engineRegistryPort)
	if err := store.Save(ctx, types.KeyRegistry, registryURL); err != nil {
		return fmt.Errorf("failed to save registry URL: %w", err)
	}
	fmt.Printf("Registry URL: %s (saved to etcd)\n", registryURL)
	_ = registryListener

	// Start Engine HTTP API
	password := cfg.Security.Password
	fmt.Printf("Starting Engine API on port %s...\n", engineAPIPort)
	startEngineAPI(ctx, store, engineAPIPort, password, registryURL)
	fmt.Printf("Engine API listening on :%s\n", engineAPIPort)

	fmt.Println("========================================")
	fmt.Println("Engine is running. Watching for deployments...")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Printf("  Deploy:      banyan-cli deploy --file banyan.yaml --engine http://localhost:%s\n", engineAPIPort)
	fmt.Println("  Agent start: banyan-agent start --node-name <name>")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	// Start the orchestration loop
	go engineLoop(ctx, store)

	<-ctx.Done()

	// Stop etcd on shutdown
	if isEngineEtcdRunning() {
		fmt.Println("Stopping etcd...")
		if err := stopEngineEtcd(); err != nil {
			fmt.Printf("Warning: Failed to stop etcd: %v\n", err)
		} else {
			fmt.Println("etcd stopped")
		}
	}

	fmt.Println("Engine stopped")
	return nil
}

// engineLoop is the main orchestration loop.
func engineLoop(ctx context.Context, store *storage.EtcdStore) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processDeployments(ctx, store)
		}
	}
}

// processDeployments checks for pending and deploying deployments.
func processDeployments(ctx context.Context, store *storage.EtcdStore) {
	keys, err := store.List(ctx, types.KeyDeployments)
	if err != nil {
		return
	}

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := store.Get(ctx, key, &record); err != nil {
			continue
		}

		switch record.Status {
		case types.StatusPending:
			schedulePendingDeployment(ctx, store, &record)
		case types.StatusDeploying:
			checkDeployingDeployment(ctx, store, &record)
		case types.StatusStopping:
			checkStoppingDeployment(ctx, store, &record)
		}
	}
}

// schedulePendingDeployment assigns tasks to available agents.
func schedulePendingDeployment(ctx context.Context, store *storage.EtcdStore, deployment *types.DeploymentRecord) {
	agents, err := listAvailableAgents(ctx, store)
	if err != nil || len(agents) == 0 {
		return
	}

	fmt.Printf("[Engine] Scheduling deployment '%s' (%d services, %d agents)\n",
		deployment.Name, len(deployment.Services), len(agents))

	tasks := types.BuildTasksForDeployment(deployment, agents)

	taskCount := 0
	for _, task := range tasks {
		taskKey := types.KeyTasks + task.AgentID + "/" + task.ID
		if err := store.Save(ctx, taskKey, task); err != nil {
			fmt.Printf("[Engine] Failed to create task %s: %v\n", task.ID, err)
			continue
		}

		fmt.Printf("[Engine]   Task %s → agent %s (container: %s)\n", task.ID, task.AgentID, task.ContainerName)
		taskCount++
	}

	deployment.Status = types.StatusDeploying
	deployment.UpdatedAt = time.Now()
	if err := store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err != nil {
		fmt.Printf("[Engine] Failed to update deployment status: %v\n", err)
	}

	fmt.Printf("[Engine] Dispatched %d tasks for deployment '%s'\n", taskCount, deployment.Name)
}

// checkDeployingDeployment checks if all tasks for a deployment have completed.
func checkDeployingDeployment(ctx context.Context, store *storage.EtcdStore, deployment *types.DeploymentRecord) {
	nodeKeys, err := store.List(ctx, types.KeyNodes)
	if err != nil {
		return
	}

	totalTasks := 0
	completedTasks := 0
	failedTasks := 0
	var firstError string

	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskPrefix := types.KeyTasks + node.Name + "/"
		taskKeys, err := store.List(ctx, taskPrefix)
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := store.Get(ctx, taskKey, &task); err != nil {
				continue
			}

			if task.DeploymentID != deployment.ID {
				continue
			}

			totalTasks++
			switch task.Status {
			case types.StatusCompleted:
				completedTasks++
			case types.StatusFailed:
				failedTasks++
				if firstError == "" {
					firstError = task.Error
				}
			}
		}
	}

	newStatus, errMsg := types.DetermineDeploymentStatus(totalTasks, completedTasks, failedTasks, firstError)
	if newStatus == "" {
		return
	}

	deployment.Status = newStatus
	deployment.Error = errMsg
	deployment.UpdatedAt = time.Now()
	if err := store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
		if newStatus == types.StatusFailed {
			fmt.Printf("[Engine] Deployment '%s' FAILED: %s\n", deployment.Name, errMsg)
		} else {
			fmt.Printf("[Engine] Deployment '%s' is RUNNING (%d containers)\n", deployment.Name, completedTasks)
		}
	}
}

// checkStoppingDeployment checks if all stop_and_remove tasks have completed.
func checkStoppingDeployment(ctx context.Context, store *storage.EtcdStore, deployment *types.DeploymentRecord) {
	tasks := types.CollectDeploymentTasks(ctx, store, deployment.ID)

	totalTasks := 0
	completedTasks := 0
	failedTasks := 0
	var firstError string

	for _, task := range tasks {
		if task.Type != types.TaskTypeStopAndRemove {
			continue
		}
		totalTasks++
		switch task.Status {
		case types.StatusCompleted:
			completedTasks++
		case types.StatusFailed:
			failedTasks++
			if firstError == "" {
				firstError = task.Error
			}
		}
	}

	if totalTasks == 0 {
		return
	}

	if failedTasks > 0 {
		deployment.Status = types.StatusFailed
		deployment.Error = fmt.Sprintf("%d/%d stop tasks failed: %s", failedTasks, totalTasks, firstError)
		deployment.UpdatedAt = time.Now()
		if err := store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
			fmt.Printf("[Engine] Deployment '%s' stop FAILED: %s\n", deployment.Name, deployment.Error)
		}
		return
	}

	if completedTasks == totalTasks {
		deployment.Status = types.StatusStopped
		deployment.UpdatedAt = time.Now()
		if err := store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
			fmt.Printf("[Engine] Deployment '%s' is STOPPED (%d containers removed)\n", deployment.Name, completedTasks)
		}
	}
}

// listAvailableAgents returns all registered agents with status "ready".
func listAvailableAgents(ctx context.Context, store *storage.EtcdStore) ([]types.NodeRecord, error) {
	keys, err := store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil, err
	}

	var agents []types.NodeRecord
	for _, key := range keys {
		var node types.NodeRecord
		if err := store.Get(ctx, key, &node); err != nil {
			continue
		}
		if node.Status == "ready" {
			agents = append(agents, node)
		}
	}
	return agents, nil
}

// findDeploymentByName scans all deployments and returns the most recent one matching the given name.
func findDeploymentByName(ctx context.Context, store *storage.EtcdStore, name string) (*types.DeploymentRecord, string, error) {
	keys, err := store.List(ctx, types.KeyDeployments)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list deployments: %w", err)
	}

	var best *types.DeploymentRecord
	var bestKey string
	for _, key := range keys {
		var record types.DeploymentRecord
		if err := store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name {
			continue
		}
		if best == nil || record.CreatedAt.After(best.CreatedAt) {
			r := record
			best = &r
			bestKey = key
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no deployment found with name %q", name)
	}
	return best, bestKey, nil
}

func runEngineStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Stopping Banyan Engine...")

	if isEngineEtcdRunning() {
		fmt.Println("Stopping etcd...")
		if err := stopEngineEtcd(); err != nil {
			fmt.Printf("Warning: Failed to stop etcd: %v\n", err)
		} else {
			fmt.Println("etcd stopped")
		}
	}

	fmt.Println("Engine stopped")
	return nil
}

func runEngineStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine - Status")
	fmt.Println("========================================")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store, err := storage.NewEtcdStore([]string{engineEtcdEndpoints}, "/banyan")
	if err != nil {
		fmt.Println("etcd: NOT RUNNING")
		fmt.Printf("Connection: FAILED (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}

	fmt.Println("etcd: RUNNING")
	fmt.Println("Connection: OK")

	if err := types.VerifyAuth(ctx, store, configPath); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	agents, _ := listAvailableAgents(ctx, store)
	fmt.Printf("\nAgents: %d\n", len(agents))
	for _, agent := range agents {
		age := time.Since(agent.LastSeen).Truncate(time.Second)
		fmt.Printf("  - %s (status: %s, last seen: %s ago)\n", agent.Name, agent.Status, age)
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
		for _, t := range allTasks {
			if t.Type == types.TaskTypeCreateAndStart {
				tasks = append(tasks, t)
			}
		}

		healthy := 0
		for _, t := range tasks {
			if t.ContainerStatus == types.StatusRunning {
				healthy++
			}
		}

		fmt.Printf("  - %s (status: %s, containers: %d/%d healthy)\n",
			record.Name, record.Status, healthy, len(tasks))

		grouped := types.GroupTasksByService(tasks)
		for _, svcName := range types.SortedServiceNames(grouped) {
			fmt.Printf("    %s:\n", svcName)
			for _, t := range grouped[svcName] {
				containerStatus := t.ContainerStatus
				if containerStatus == "" {
					containerStatus = "pending"
				}
				checkedInfo := ""
				if !t.ContainerCheckedAt.IsZero() {
					ago := time.Since(t.ContainerCheckedAt).Truncate(time.Second)
					checkedInfo = fmt.Sprintf(" (checked %s ago)", ago)
				}
				fmt.Printf("      %s on %s: %s%s\n", t.ContainerName, t.AgentID, containerStatus, checkedInfo)
			}
		}
	}

	fmt.Println("\n========================================")
	return nil
}

// --- Registry helpers ---

func startRegistry(ctx context.Context, port string) (net.Listener, error) {
	handler := registry.New()

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Registry server error: %v\n", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		server.Shutdown(shutdownCtx)
	}()

	return listener, nil
}

func determineEngineIP() (string, error) {
	parsed, err := url.Parse(engineEtcdClientURLs)
	if err != nil {
		return "", fmt.Errorf("failed to parse etcd client URL %q: %w", engineEtcdClientURLs, err)
	}

	host := parsed.Hostname()
	if host != "0.0.0.0" && host != "" {
		return host, nil
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		return ip.String(), nil
	}
	return "", fmt.Errorf("no non-loopback IPv4 address found")
}

// --- etcd management helpers ---

func ensureEngineEtcdRunning() error {
	if isEngineEtcdRunning() {
		fmt.Println("etcd is already running")
		return nil
	}

	etcdBinary, err := exec.LookPath("etcd")
	if err != nil {
		return fmt.Errorf("etcd binary not found. Install with: apt install etcd-server")
	}

	if err := os.MkdirAll(engineEtcdDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create etcd data directory: %w", err)
	}

	logDir := filepath.Dir(engineEtcdLogFile)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	fmt.Println("Starting etcd...")

	etcdArgs := []string{
		"--name", "banyan-etcd",
		"--data-dir", engineEtcdDataDir,
		"--listen-client-urls", engineEtcdClientURLs,
		"--advertise-client-urls", "http://localhost:2379",
		"--listen-peer-urls", "http://127.0.0.1:2380",
		"--initial-advertise-peer-urls", "http://127.0.0.1:2380",
		"--initial-cluster", "banyan-etcd=http://127.0.0.1:2380",
		"--initial-cluster-token", "banyan-vpc-cluster",
		"--initial-cluster-state", "new",
	}

	logFile, err := os.OpenFile(engineEtcdLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	etcdProcess := exec.Command(etcdBinary, etcdArgs...)
	etcdProcess.Stdout = logFile
	etcdProcess.Stderr = logFile
	etcdProcess.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := etcdProcess.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start etcd: %w", err)
	}

	pidDir := filepath.Dir(engineEtcdPidFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return fmt.Errorf("failed to create PID directory: %w", err)
	}

	if err := os.WriteFile(engineEtcdPidFile, []byte(strconv.Itoa(etcdProcess.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	fmt.Printf("etcd started (PID: %d)\n", etcdProcess.Process.Pid)
	return nil
}

func isEngineEtcdRunning() bool {
	if _, err := os.Stat(engineEtcdPidFile); os.IsNotExist(err) {
		return false
	}
	pidBytes, err := os.ReadFile(engineEtcdPidFile)
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

func stopEngineEtcd() error {
	pidBytes, err := os.ReadFile(engineEtcdPidFile)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return err
	}
	os.Remove(engineEtcdPidFile)
	return nil
}

func waitForEngineEtcd(ctx context.Context, endpoint string, timeout time.Duration) error {
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
