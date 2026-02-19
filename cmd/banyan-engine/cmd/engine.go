package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/fertile-org/banyan/pkg/engine"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
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
  1. Opens the store backend (badger embedded, or connects to external redis/etcd)
  2. Initializes VPC networking (IPAM, DNS, Security) — etcd backend only
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

	// Store backend flags
	startCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (badger, redis, or etcd)")
	startCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Store address (badger: data dir; redis/etcd: server address)")

	// General flags
	startCmd.Flags().StringVar(&engineVPCCIDR, "vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	startCmd.Flags().StringVar(&engineRegistryPort, "registry-port", "5000", "Embedded OCI registry port")
	startCmd.Flags().StringVar(&engineGRPCPort, "grpc-port", "50051", "Engine gRPC port")

	// Status flags
	statusCmd.Flags().StringVar(&engineStoreBackend, "store-backend", "", "Store backend (badger, redis, or etcd)")
	statusCmd.Flags().StringVar(&engineStoreAddress, "store-address", "", "Store address")
}

func runEngineInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Banyan Engine - Initialization")
	fmt.Println("========================================")

	if os.Geteuid() != 0 {
		fmt.Println("Warning: Not running as root. Some operations may require sudo.")
	}

	dirs := []string{
		engineDataDir,
		filepath.Join(engineDataDir, "store"),
		filepath.Join(engineDataDir, "vpc"),
		"/var/log",
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

	fmt.Println("\n2. Store backend info...")
	existingCfg, _ := types.LoadConfig(configPath)

	fmt.Println("   [OK] badger (embedded, no external dependency)")
	fmt.Println("   [INFO] redis and etcd are also supported (user-managed)")

	fmt.Println("\n3. Configuring authentication...")
	if existingCfg.Security.Password != "" {
		fmt.Printf("   [OK] Config already exists at %s (password set)\n", configPath)
	} else {
		fmt.Print("   Enter cluster password (leave empty to skip): ")
		password := types.ReadLine()
		if password != "" {
			existingCfg.Security = types.SecurityConfig{
				AuthType: "password",
				Password: password,
			}
		} else {
			fmt.Println("   [SKIP] No password set")
		}
	}

	fmt.Println("\n4. Configuring store backend...")
	if existingCfg.Engine.StoreBackend != "" {
		storeBackend := existingCfg.Engine.GetStoreBackend()
		fmt.Printf("   [OK] Store backend already configured: %s\n", storeBackend)
	} else {
		fmt.Print("   Store backend (badger/redis/etcd) [badger]: ")
		backendInput := types.ReadLine()
		if backendInput == "" {
			backendInput = "badger"
		}
		if backendInput != "badger" && backendInput != "redis" && backendInput != "etcd" {
			fmt.Printf("   [WARN] Unknown backend %q, using badger\n", backendInput)
			backendInput = "badger"
		}
		existingCfg.Engine.StoreBackend = backendInput

		// Prompt for address for external backends
		if backendInput == "redis" || backendInput == "etcd" {
			defaultAddr := "localhost:6379"
			if backendInput == "etcd" {
				defaultAddr = "http://localhost:2379"
			}
			fmt.Printf("   Store address [%s]: ", defaultAddr)
			addrInput := types.ReadLine()
			if addrInput == "" {
				addrInput = defaultAddr
			}
			existingCfg.Engine.StoreAddress = addrInput
		}

		fmt.Printf("   [OK] Store backend: %s\n", backendInput)
	}

	// Save config with all collected settings
	if err := types.SaveConfig(configPath, &existingCfg); err != nil {
		fmt.Printf("   [WARN] Failed to save config: %v\n", err)
	} else {
		fmt.Printf("   [OK] Config saved to %s\n", configPath)
	}

	fmt.Println("\n========================================")
	fmt.Println("Initialization complete!")
	fmt.Println("\nNext step: banyan-engine start")
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

	// Resolve default addresses per backend
	switch storeBackend {
	case "badger":
		if storeAddress == "" {
			storeAddress = filepath.Join(engineDataDir, "store")
		}
		if mkdirErr := os.MkdirAll(storeAddress, 0o755); mkdirErr != nil {
			return fmt.Errorf("failed to create badger data directory: %w", mkdirErr)
		}
	case "redis":
		if storeAddress == "" {
			storeAddress = "localhost:6379"
		}
	case "etcd":
		if storeAddress == "" {
			storeAddress = "http://localhost:2379"
		}
	default:
		return fmt.Errorf("unsupported store backend: %s", storeBackend)
	}

	fmt.Printf("Connecting to %s at %s...\n", storeBackend, storeAddress)

	eng, err := engine.New(&engine.Options{
		StoreBackend: storeBackend,
		StoreAddress: storeAddress,
		VPCCIDR:      engineVPCCIDR,
		RegistryPort: engineRegistryPort,
		GRPCPort:     engineGRPCPort,
		Password:     cfg.Security.Password,
		DataDir:      engineDataDir,
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
	if storeAddress == "" {
		switch storeBackend {
		case "badger":
			storeAddress = filepath.Join(engineDataDir, "store")
		case "redis":
			storeAddress = "localhost:6379"
		case "etcd":
			storeAddress = "http://localhost:2379"
		}
	}

	store, err := storage.NewStore(storeBackend, storeAddress, "/banyan")
	if err != nil {
		fmt.Printf("Store (%s): NOT RUNNING\n", storeBackend)
		fmt.Printf("Connection: FAILED (%v)\n", err)
		fmt.Println("========================================")
		return nil
	}
	defer store.Close()

	fmt.Printf("Store (%s): RUNNING\n", storeBackend)
	fmt.Println("Connection: OK")

	if err := types.VerifyAuth(ctx, store, configPath); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	agents, _ := engine.ListAvailableAgents(ctx, store)
	fmt.Printf("\nAgents: %d\n", len(agents))
	for _, a := range agents {
		age := time.Since(a.LastSeen).Truncate(time.Second)
		fmt.Printf("  - %s (status: %s, last seen: %s ago)\n", a.Name, a.Status, age)
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
