package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	orchuc "github.com/fertile-org/banyan/pkg/engine/orchestrator/usecases"
	regadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	reguc "github.com/fertile-org/banyan/pkg/engine/registry/usecases"
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	stateuc "github.com/fertile-org/banyan/pkg/engine/state/usecases"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
	"github.com/spf13/cobra"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Engine configuration
var (
	engineEtcdEndpoints  string
	engineVPCCIDR        string
	engineDataDir        string
	engineEtcdDataDir    string
	engineEtcdPidFile    string
	engineEtcdLogFile    string
	engineEtcdClientURLs string
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Manage the Banyan Engine (control plane)",
	Long: `Manage the Banyan Engine, which is the control plane for Banyan.

The Engine manages:
  - Deployment orchestration
  - Agent registration and health
  - VPC networking (IPAM, DNS, Security)
  - State coordination via etcd

Commands:
  init    Initialize Engine dependencies (etcd, directories)
  start   Start the Engine (auto-starts etcd if needed)
  stop    Stop the Engine
  status  Show Engine status`,
}

var engineInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Engine dependencies",
	Long: `Initialize the Banyan Engine environment.

This command:
  1. Creates required directories (/var/lib/banyan)
  2. Checks if etcd binary is available
  3. Validates network configuration

Run this once before starting the Engine.`,
	RunE: runEngineInit,
}

var engineStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Banyan Engine",
	Long: `Start the Banyan Engine control plane.

This command:
  1. Starts etcd if not already running
  2. Initializes VPC networking (IPAM, DNS, Security)
  3. Starts the Engine components (Registry, State, Orchestrator)
  4. Waits for agents to register`,
	RunE: runEngineStart,
}

var engineStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Banyan Engine",
	Long:  `Stop the running Banyan Engine and optionally etcd.`,
	RunE:  runEngineStop,
}

var engineStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Engine status",
	Long:  `Show the status of the Banyan Engine and etcd.`,
	RunE:  runEngineStatus,
}

func init() {
	rootCmd.AddCommand(engineCmd)
	engineCmd.AddCommand(engineInitCmd)
	engineCmd.AddCommand(engineStartCmd)
	engineCmd.AddCommand(engineStopCmd)
	engineCmd.AddCommand(engineStatusCmd)

	// Common flags
	engineCmd.PersistentFlags().StringVar(&engineDataDir, "data-dir", "/var/lib/banyan", "Data directory")

	// Start command flags
	engineStartCmd.Flags().StringVar(&engineEtcdEndpoints, "etcd", "http://localhost:2379", "Etcd endpoints")
	engineStartCmd.Flags().StringVar(&engineVPCCIDR, "vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	engineStartCmd.Flags().StringVar(&engineEtcdDataDir, "etcd-data-dir", "/var/lib/banyan/etcd", "Etcd data directory")
	engineStartCmd.Flags().StringVar(&engineEtcdPidFile, "etcd-pid-file", "/var/run/banyan-etcd.pid", "Etcd PID file")
	engineStartCmd.Flags().StringVar(&engineEtcdLogFile, "etcd-log-file", "/var/log/banyan-etcd.log", "Etcd log file")
	engineStartCmd.Flags().StringVar(&engineEtcdClientURLs, "etcd-client-urls", "http://0.0.0.0:2379", "Etcd client URLs")

	// Status command flags
	engineStatusCmd.Flags().StringVar(&engineEtcdEndpoints, "etcd", "http://localhost:2379", "Etcd endpoints")
	engineStatusCmd.Flags().StringVar(&engineEtcdPidFile, "etcd-pid-file", "/var/run/banyan-etcd.pid", "Etcd PID file")
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

	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Println("\nNext step: banyan-cli engine start")

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

	// Initialize VPC network
	fmt.Printf("Initializing VPC network with CIDR %s...\n", engineVPCCIDR)
	if err := vpc.InitializeNetwork(ctx, []string{engineEtcdEndpoints}, engineVPCCIDR); err != nil {
		fmt.Printf("Warning: VPC initialization: %v\n", err)
	}

	// Initialize VPC components
	ipamManager, err := ipam.NewManager(store, engineVPCCIDR)
	if err != nil {
		return fmt.Errorf("failed to initialize IPAM: %w", err)
	}
	_ = dns.NewManagerWithStore(store)
	resolver := security.NewRuntimeServiceResolver(store)
	_ = security.NewManager(resolver, false)

	fmt.Println("VPC components initialized: IPAM, DNS, Security")

	// Initialize Engine components
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	agentRepo := regadapters.NewMemoryAgentRepository()
	eventPublisher := regadapters.NewMemoryEventPublisher()
	_ = reguc.NewRegistryUseCase(agentRepo, eventPublisher, logger)

	stateRepo := stateadapters.NewMemoryStateRepository()
	_ = stateuc.NewStateUseCase(stateRepo)

	orchRepo := adapters.NewMemoryDeploymentRepository()
	orchDispatcher := adapters.NewMemoryAgentDispatcher()
	orchScheduler := adapters.NewMemoryScheduler()
	orchPlugins := adapters.NewMemoryPluginExecutor()
	orchParser := adapters.NewMemoryBanyanParser()
	_ = orchuc.NewDeployUseCase(orchRepo, orchDispatcher, orchScheduler, orchPlugins, orchParser)

	fmt.Println("Engine components initialized: Registry, State, Orchestrator")
	fmt.Println("========================================")

	// Keep references to prevent unused variable warnings
	_ = ipamManager

	fmt.Println("Engine is running. Waiting for agents to register...")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  Deploy:      banyan-cli deploy --file banyan.yaml")
	fmt.Printf("  Agent start: banyan-cli agent start --engine %s\n", engineEtcdEndpoints)
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	<-ctx.Done()
	fmt.Println("Engine stopped")
	return nil
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

	fmt.Print("etcd: ")
	if isEngineEtcdRunning() {
		fmt.Println("RUNNING")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{engineEtcdEndpoints},
			DialTimeout: 5 * time.Second,
		})
		if err == nil {
			defer client.Close()
			_, err = client.Status(ctx, engineEtcdEndpoints)
			if err == nil {
				fmt.Println("  Connection: OK")
			} else {
				fmt.Printf("  Connection: FAILED (%v)\n", err)
			}
		}
	} else {
		fmt.Println("NOT RUNNING")
	}

	fmt.Println("========================================")
	return nil
}

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
