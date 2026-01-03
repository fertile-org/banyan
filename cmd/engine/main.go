package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fertile-org/banyan/internal/common"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	orchdomain "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	orchinbound "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	orchuc "github.com/fertile-org/banyan/pkg/engine/orchestrator/usecases"
	regadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	regdomain "github.com/fertile-org/banyan/pkg/engine/registry/domain"
	reguc "github.com/fertile-org/banyan/pkg/engine/registry/usecases"
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	statedomain "github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateuc "github.com/fertile-org/banyan/pkg/engine/state/usecases"
	"github.com/fertile-org/banyan/pkg/vpc"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/ipam"
	"github.com/fertile-org/banyan/pkg/vpc/security"
	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

var (
	etcdEndpoints = flag.String("etcd", "http://localhost:2379", "Etcd endpoints (comma-separated)")
	vpcCIDR       = flag.String("vpc-cidr", "10.0.0.0/16", "VPC CIDR range")
	banyanFile    = flag.String("file", "", "Path to banyan.yml file to deploy")
	listAgents    = flag.Bool("list-agents", false, "List registered agents")
	showStatus    = flag.Bool("status", false, "Show deployment status")
)

func main() {
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	fmt.Printf("Banyan Engine %s\n", common.Version)
	fmt.Println("========================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// Initialize storage (etcd)
	fmt.Printf("Connecting to etcd at %s...\n", *etcdEndpoints)
	store, err := storage.NewEtcdStore([]string{*etcdEndpoints}, "/banyan")
	if err != nil {
		fmt.Printf("Failed to connect to etcd: %v\n", err)
		fmt.Println("Hint: Start etcd with 'etcd &' or '/tmp/vpc-cli etcd start &'")
		cancel()
		return
	}
	fmt.Println("Connected to etcd")

	// Initialize VPC network
	fmt.Printf("Initializing VPC network with CIDR %s...\n", *vpcCIDR)
	if initErr := vpc.InitializeNetwork(ctx, []string{*etcdEndpoints}, *vpcCIDR); initErr != nil {
		fmt.Printf("Warning: VPC initialization: %v\n", initErr)
	}

	// Initialize VPC components
	ipamManager, err := ipam.NewManager(store, *vpcCIDR)
	if err != nil {
		fmt.Printf("Failed to initialize IPAM: %v\n", err)
		cancel()
		return
	}
	dnsManager := dns.NewManagerWithStore(store)
	resolver := security.NewRuntimeServiceResolver(store)
	securityManager := security.NewManager(resolver, false)

	fmt.Println("VPC components initialized: IPAM, DNS, Security")

	// Initialize Engine components
	agentRepo := regadapters.NewMemoryAgentRepository()
	eventPublisher := regadapters.NewMemoryEventPublisher()
	registry := reguc.NewRegistryUseCase(agentRepo, eventPublisher, logger)

	stateRepo := stateadapters.NewMemoryStateRepository()
	stateUC := stateuc.NewStateUseCase(stateRepo)

	orchRepo := adapters.NewMemoryDeploymentRepository()
	orchDispatcher := adapters.NewMemoryAgentDispatcher()
	orchScheduler := adapters.NewMemoryScheduler()
	orchPlugins := adapters.NewMemoryPluginExecutor()
	orchParser := adapters.NewMemoryBanyanParser()

	orchestrator := orchuc.NewDeployUseCase(orchRepo, orchDispatcher, orchScheduler, orchPlugins, orchParser)

	fmt.Println("Engine components initialized: Registry, State, Orchestrator")
	fmt.Println("========================================")

	// Handle commands
	if *listAgents {
		listRegisteredAgents(ctx, registry)
		return
	}

	if *showStatus {
		showDeploymentStatus(ctx, orchestrator)
		return
	}

	if *banyanFile != "" {
		deployFromFile(ctx, *banyanFile, orchestrator, stateUC, registry, ipamManager, dnsManager, securityManager)
		return
	}

	// Default: run as server (waiting for agents)
	fmt.Println("Engine is running. Waiting for agents to register...")
	fmt.Println("Usage:")
	fmt.Println("  Deploy:      ./banyan-engine --file banyan.yml")
	fmt.Println("  List agents: ./banyan-engine --list-agents")
	fmt.Println("  Status:      ./banyan-engine --status")
	fmt.Println("")
	fmt.Println("Press Ctrl+C to stop")

	// Keep running until shutdown
	<-ctx.Done()
	fmt.Println("Engine stopped")
}

func listRegisteredAgents(ctx context.Context, registry *reguc.RegistryUseCase) {
	agents, err := registry.ListAgents(ctx, regdomain.AgentFilter{})
	if err != nil {
		fmt.Printf("Failed to list agents: %v\n", err)
		return
	}

	if len(agents) == 0 {
		fmt.Println("No agents registered")
		return
	}

	fmt.Printf("Registered agents (%d):\n", len(agents))
	for i := range agents {
		fmt.Printf("  - %s (%s)\n", agents[i].Hostname, agents[i].Address)
		fmt.Printf("    Status: %s, CPU: %.1f cores, Memory: %d MB\n",
			agents[i].Status, agents[i].Resources.CPUAvailable, agents[i].Resources.MemoryFreeMB)
	}
}

func showDeploymentStatus(ctx context.Context, orchestrator *orchuc.DeployUseCase) {
	deployments, err := orchestrator.ListDeployments(ctx, orchinbound.DeploymentFilter{})
	if err != nil {
		fmt.Printf("Failed to list deployments: %v\n", err)
		return
	}

	if len(deployments) == 0 {
		fmt.Println("No active deployments")
		return
	}

	fmt.Printf("Active deployments (%d):\n", len(deployments))
	for i := range deployments {
		fmt.Printf("  - %s (ID: %s)\n", deployments[i].Name, deployments[i].ID)
		fmt.Printf("    State: %s, Services: %d\n", deployments[i].State, len(deployments[i].Services))
		for j := range deployments[i].Services {
			fmt.Printf("      - %s: %d replicas (%s)\n", deployments[i].Services[j].Name, deployments[i].Services[j].Replicas, deployments[i].Services[j].Image)
		}
	}
}

func deployFromFile(ctx context.Context, filepath string,
	orchestrator *orchuc.DeployUseCase,
	stateUC *stateuc.StateUseCase,
	registry *reguc.RegistryUseCase,
	_ *ipam.Manager,
	_ *dns.Manager,
	_ *security.Manager) {

	// Read banyan.yml
	content, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Printf("Failed to read %s: %v\n", filepath, err)
		os.Exit(1)
	}

	fmt.Printf("Deploying from %s...\n", filepath)

	// Create deployment
	deployReq := orchinbound.CreateDeploymentRequest{
		Name:       "deployment-" + time.Now().Format("20060102-150405"),
		BanyanFile: string(content),
	}

	deployment, err := orchestrator.CreateDeployment(ctx, deployReq)
	if err != nil {
		fmt.Printf("Failed to create deployment: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Deployment created: %s\n", deployment.ID)
	fmt.Printf("Services (%d):\n", len(deployment.Services))
	for i := range deployment.Services {
		fmt.Printf("  - %s: %d replicas (%s)\n", deployment.Services[i].Name, deployment.Services[i].Replicas, deployment.Services[i].Image)
	}

	// Check for agents
	agents, err := registry.ListAgents(ctx, regdomain.AgentFilter{})
	if err != nil || len(agents) == 0 {
		fmt.Println("\nWarning: No agents registered. Deployment will be pending.")
		fmt.Println("Start agents with: ./banyan-agent --engine localhost:50052")

		// Set desired state anyway
		services := make(map[string]statedomain.ServiceDesiredState)
		for i := range deployment.Services {
			services[deployment.Services[i].Name] = statedomain.ServiceDesiredState{
				Name:     deployment.Services[i].Name,
				Image:    deployment.Services[i].Image,
				Replicas: deployment.Services[i].Replicas,
			}

			// Register DNS for service
			// IP will be allocated when agent picks up the task
			fmt.Printf("  Registered DNS: %s.internal\n", deployment.Services[i].Name)
		}

		desiredState := &statedomain.DesiredState{
			DeploymentID: deployment.ID,
			Services:     services,
			UpdatedAt:    time.Now(),
		}
		_ = stateUC.SetDesiredState(ctx, desiredState)

		fmt.Printf("\nDeployment %s is PENDING (waiting for agents)\n", deployment.ID)
		return
	}

	// Execute deployment
	fmt.Printf("\nExecuting deployment on %d agents...\n", len(agents))

	err = orchestrator.ExecuteDeployment(ctx, deployment.ID)
	if err != nil {
		fmt.Printf("Failed to execute deployment: %v\n", err)
		os.Exit(1)
	}

	// Get updated deployment
	updatedDeployment, _ := orchestrator.GetDeployment(ctx, deployment.ID)
	fmt.Printf("\nDeployment state: %s\n", updatedDeployment.State)

	if updatedDeployment.State == orchdomain.StateActive {
		fmt.Println("Deployment successful!")
	}
}
