// Package engine implements the Banyan Engine control plane.
// It manages deployments, schedules tasks to agents, and runs the gRPC server.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Options configures the Engine.
type Options struct {
	StoreBackend        string // "etcd" only
	StoreAddress        string // resolved address for the store backend
	VPCCIDR             string
	RegistryPort        string
	GRPCPort            string
	MetricsPort         string // Prometheus /metrics HTTP port (default "9090")
	APIPort             string // Connect HTTP API port for web dashboard (default "9091")
	DataDir             string
	EtcdUsername        string            // etcd RBAC username
	EtcdPassword        string            // etcd RBAC password
	EtcdCertFile        string            // client certificate for mTLS
	EtcdKeyFile         string            // client key for mTLS
	EtcdCAFile          string            // CA certificate for server verification
	WhitelistedKeys     map[string]string // publicKey → agentName
	AllowInsecure       bool              // allow running without authentication (development only)
	ControlTunnelActive bool              // WireGuard control tunnel is set up and active
	TunnelIP            string            // engine's WireGuard tunnel IP (derived from public key)
	ExternalRegistryURL string            // external registry URL (skips embedded registry when set)
	EngineID            string            // unique engine identifier (auto-generated if empty)
	MultiEngine         bool              // enable multi-engine coordination
	ConfigDir           string            // directory holding banyan.yaml + auth-bootstrap.json (default /etc/banyan)
}

// Engine is the Banyan control plane.
type Engine struct {
	store            storage.StateStore
	grpcServer       *engineGRPCServer
	secrets          *SecretsManager // nil if secrets.key not present
	opts             Options
	registryURL      string
	engineID         string
	multiEngine      bool
	scheduleCh       chan struct{} // triggers immediate scheduling (from Deploy/Register RPCs)
	metricsRegistry  *metrics.EngineMetricsRegistry
	metricsCollector *metrics.SystemCollector
	events           EventLog
	reconcilers      []Reconciler
	startedAt        time.Time
	log              *logging.Logger
}

// New creates a new Engine. It opens the store and sets up authentication.
func New(opts *Options) (*Engine, error) {
	store, err := storage.NewStoreWithOptions(&storage.EtcdOptions{
		Endpoints: []string{opts.StoreAddress},
		Prefix:    "/banyan",
		Username:  opts.EtcdUsername,
		Password:  opts.EtcdPassword,
		CertFile:  opts.EtcdCertFile,
		KeyFile:   opts.EtcdKeyFile,
		CAFile:    opts.EtcdCAFile,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", opts.StoreBackend, err)
	}

	// Initialize event store — WAL-backed when DataDir is set, in-memory fallback otherwise
	var eventLog EventLog
	if opts.DataDir != "" {
		walPath := filepath.Join(opts.DataDir, "events.wal")
		es, esErr := NewEventStore(walPath, DefaultMaxEvents)
		if esErr != nil {
			return nil, fmt.Errorf("failed to open event WAL: %w", esErr)
		}
		eventLog = es
	} else {
		eventLog = NewEventBuffer(100)
	}

	engineID := opts.EngineID
	if engineID == "" {
		engineID = GenerateEngineID()
	}

	e := &Engine{
		opts:             *opts,
		store:            store,
		engineID:         engineID,
		multiEngine:      opts.MultiEngine,
		scheduleCh:       make(chan struct{}, 1),
		metricsRegistry:  metrics.NewEngineMetricsRegistry(),
		metricsCollector: metrics.NewSystemCollector(),
		events:           eventLog,
		startedAt:        time.Now(),
		log:              logging.New("engine"),
	}
	// Seed the CPU sample so the first metrics read gets a real value
	e.metricsCollector.Collect()

	// Initialize reconcilers (Agent → Container → Deployment order)
	deps := &ReconcilerDeps{
		Store:   store,
		Events:  eventLog,
		Metrics: e.metricsRegistry,
		Log:     e.log,
	}
	e.reconcilers = []Reconciler{
		NewAgentReconciler(deps),
		NewContainerReconciler(deps),
		NewDeploymentReconciler(deps),
	}

	// Load secrets encryption key (optional — secrets features disabled if key missing)
	secretsKeyPath := filepath.Join(types.DefaultKeysDir, "secrets.key")
	if _, statErr := os.Stat(secretsKeyPath); statErr == nil {
		sm, smErr := NewSecretsManager(store, secretsKeyPath)
		if smErr != nil {
			return nil, fmt.Errorf("failed to initialize secrets manager: %w", smErr)
		}
		e.secrets = sm
	}

	return e, nil
}

// GenerateEngineID creates a unique engine ID from hostname + 8 random hex chars.
// Example: "prod-web-1-a3f2b1c4"
func GenerateEngineID() string {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "engine"
	}
	// Sanitize hostname: lowercase, replace dots with dashes
	hostname = strings.ToLower(hostname)
	hostname = strings.ReplaceAll(hostname, ".", "-")

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return hostname
	}
	return hostname + "-" + hex.EncodeToString(b)
}

// EngineID returns the engine's unique identifier.
func (e *Engine) EngineID() string {
	return e.engineID
}

// logger returns the engine's logger, initializing it if nil (for test convenience).
func (e *Engine) logger() *logging.Logger {
	if e.log == nil {
		e.log = logging.New("engine")
	}
	return e.log
}

// Run starts the engine: VPC init, registry, gRPC server, and orchestration loop.
// It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	if len(e.opts.WhitelistedKeys) > 0 {
		e.logger().Info("Public key authentication enabled", "whitelisted_keys", len(e.opts.WhitelistedKeys))
	} else if e.opts.AllowInsecure {
		e.logger().Warn("SECURITY WARNING: Running without authentication (--allow-insecure). Do NOT use in production.")
	} else {
		return fmt.Errorf("no whitelisted public keys configured, cannot start without authentication.\n" +
			"  Add client keys with: sudo banyan-engine add-client --name <name> --pubkey <key>\n" +
			"  Or use --allow-insecure for development only (NOT for production)")
	}

	// Initialize VPC overlay networking (subnet allocation + peer tracking)
	var allocator overlay.SubnetAllocatorInterface
	var peerTracker overlay.PeerTrackerInterface
	if e.opts.VPCCIDR != "" {
		if err := checkCIDRConflict(e.opts.VPCCIDR); err != nil {
			suggestion, _ := suggestFreeCIDR()
			if suggestion != "" {
				return fmt.Errorf("VPC CIDR conflict: %w\n"+
					"  → Suggested free range: %s\n"+
					"  → Fix: re-run `sudo banyan-engine init` to choose a range, "+
					"or start with --vpc-cidr %s",
					err, suggestion, suggestion)
			}
			return fmt.Errorf("VPC CIDR conflict: %w\n"+
				"  → No free candidate range found; specify one with --vpc-cidr", err)
		}

		if e.multiEngine {
			// Multi-engine: etcd-backed allocator and peer tracker for coordination
			lockStore, ok := e.store.(storage.LockStore)
			if !ok {
				return fmt.Errorf("multi-engine VPC requires etcd store with lock support")
			}
			var allocErr error
			allocator, allocErr = newEtcdSubnetAllocator(e.opts.VPCCIDR, e.store, lockStore)
			if allocErr != nil {
				return fmt.Errorf("failed to create etcd subnet allocator: %w", allocErr)
			}
			peerTracker = newEtcdPeerTracker(e.store)
			e.logger().Info("VPC overlay networking enabled (etcd-backed)", "cidr", e.opts.VPCCIDR)
		} else {
			// Single-engine: in-memory allocator and peer tracker
			var allocErr error
			allocator, allocErr = overlay.NewSubnetAllocator(e.opts.VPCCIDR)
			if allocErr != nil {
				return fmt.Errorf("failed to create subnet allocator: %w", allocErr)
			}
			peerTracker = overlay.NewPeerTracker()
			e.logger().Info("VPC overlay networking enabled", "cidr", e.opts.VPCCIDR)
		}
	}

	// Registry URL is set by the cmd layer (managed Distribution subprocess or external).
	if e.opts.ExternalRegistryURL == "" {
		return fmt.Errorf("no registry URL configured. Run 'banyan-engine init' to set up a managed or external registry")
	}
	e.registryURL = e.opts.ExternalRegistryURL
	if saveErr := e.store.Save(ctx, types.KeyRegistry, e.registryURL); saveErr != nil {
		return fmt.Errorf("failed to save registry URL: %w", saveErr)
	}

	// Set up application-layer authentication (JWT + RBAC).
	// Returns nil AuthDeps in --allow-insecure mode.
	configDir := e.opts.ConfigDir
	if configDir == "" {
		configDir = "/etc/banyan"
	}
	authDeps, authErr := setupEngineAuth(ctx, e.store, configDir, e.opts.AllowInsecure)
	if authErr != nil {
		return fmt.Errorf("failed to set up authentication: %w", authErr)
	}
	if authDeps != nil {
		e.logger().Info("Application-layer authentication enabled (JWT + RBAC)")
	} else {
		e.logger().Warn("Application-layer authentication DISABLED (--allow-insecure)")
	}

	// Start Engine gRPC server — bind to control tunnel IP when authenticated,
	// localhost in insecure mode. In multi-engine mode with insecure, bind to
	// all interfaces so agents on other hosts can connect.
	grpcBindAddr := "127.0.0.1"
	if e.opts.ControlTunnelActive {
		grpcBindAddr = e.opts.TunnelIP
	} else if e.opts.AllowInsecure && e.multiEngine {
		grpcBindAddr = "0.0.0.0"
	}
	grpcSrv, err := startEngineGRPC(ctx, &grpcServerOptions{
		Store:           e.store,
		Port:            e.opts.GRPCPort,
		BindAddr:        grpcBindAddr,
		RegistryURL:     e.registryURL,
		Allocator:       allocator,
		PeerTracker:     peerTracker,
		VPCCIDR:         e.opts.VPCCIDR,
		WhitelistedKeys: e.opts.WhitelistedKeys,
		AllowInsecure:   e.opts.AllowInsecure,
		ScheduleCh:      e.scheduleCh,
		MetricsRegistry: e.metricsRegistry,
		Events:          e.events,
		Secrets:         e.secrets,
		AuthDeps:        authDeps,
		StartedAt:       e.startedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	e.grpcServer = grpcSrv

	// Start Prometheus metrics HTTP server
	metricsPort := e.opts.MetricsPort
	if metricsPort == "" {
		metricsPort = "9090"
	}
	if startErr := e.startMetricsHTTP(ctx, metricsPort); startErr != nil {
		return fmt.Errorf("failed to start metrics server: %w", startErr)
	}

	// Start Connect HTTP API server for web dashboard
	apiPort := e.opts.APIPort
	if apiPort == "" {
		apiPort = "9091"
	}
	if startErr := startConnectAPI(ctx, grpcSrv, apiPort, nil); startErr != nil {
		return fmt.Errorf("failed to start Connect API server: %w", startErr)
	}

	// Register engine in etcd (for discovery by other engines / CLI)
	grpcAddr := grpcBindAddr + ":" + e.opts.GRPCPort
	e.registerEngine(ctx, grpcAddr)

	// NOTE: No startup reconciliation here. The 10s reconcile loop handles it.
	// Running reconciliation at startup is premature — agents haven't reconnected
	// yet, so all agents appear stale. The AgentReconciler would mark healthy
	// containers for cleanup, and when agents reconnect seconds later, they'd
	// kill their own running containers. The 10s loop gives agents time to
	// re-register and send heartbeats before reconciliation begins.

	// Start the orchestration loop
	go e.engineLoop(ctx)

	// Single summary line after all components are ready
	e.logger().Info("Engine ready",
		"grpc", grpcAddr,
		"api", ":"+apiPort,
		"registry", e.registryURL,
		"metrics", ":"+metricsPort,
		"clients", len(e.opts.WhitelistedKeys),
	)

	<-ctx.Done()
	return nil
}

// emitEvent adds an event to the buffer and increments the Prometheus counter.
func (e *Engine) emitEvent(eventType, message, severity string) {
	if e.events != nil {
		e.events.Add(Event{
			Timestamp: time.Now(),
			Type:      eventType,
			Message:   message,
			Severity:  severity,
		})
	}
	if e.metricsRegistry != nil {
		e.metricsRegistry.IncrementEvent(eventType)
	}
}

// registerEngine saves an EngineRecord to etcd with a keep-alive lease.
// The record auto-expires if this engine crashes (15s TTL, renewed every 10s).
func (e *Engine) registerEngine(ctx context.Context, grpcAddr string) {
	etcdStore, ok := e.store.(*storage.EtcdStore)
	if !ok {
		return // not etcd-backed (test/dev), skip registration
	}

	record := types.EngineRecord{
		ID:          e.engineID,
		Status:      "ready",
		GRPCAddr:    grpcAddr,
		RegistryURL: e.registryURL,
		StartedAt:   e.startedAt,
		LastSeen:    time.Now(),
	}
	if err := etcdStore.KeepAlive(ctx, types.KeyEngines+e.engineID, &record, 15*time.Second, 10*time.Second); err != nil {
		e.logger().Warn("Failed to register engine in etcd", "error", err)
	}
}

// Close releases engine resources.
func (e *Engine) Close() {
	if e.store != nil {
		e.store.Close()
	}
	if es, ok := e.events.(*EventStore); ok {
		es.Close()
	}
}

// startMetricsHTTP starts the Prometheus /metrics HTTP server.
func (e *Engine) startMetricsHTTP(ctx context.Context, port string) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", e.metricsRegistry.Handler())

	// Bind metrics to same address as gRPC (control tunnel or localhost)
	metricsBindAddr := "127.0.0.1"
	if e.opts.ControlTunnelActive {
		metricsBindAddr = e.opts.TunnelIP
	}
	lis, err := net.Listen("tcp", metricsBindAddr+":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen on metrics port %s: %w", port, err)
	}

	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(lis); err != nil && err != http.ErrServerClosed {
			e.logger().Error("Metrics server error", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return nil
}

// engineLoop is the main orchestration loop. All engines run this loop.
// In multi-engine mode, per-deployment distributed locks prevent duplicate work.
// The scheduleCh allows immediate scheduling when a Deploy RPC or agent
// registration occurs, instead of waiting for the next 3s tick.
func (e *Engine) engineLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	autoscaleTicker := time.NewTicker(30 * time.Second)
	defer autoscaleTicker.Stop()
	rebalanceTicker := time.NewTicker(60 * time.Second)
	defer rebalanceTicker.Stop()
	reconcileTicker := time.NewTicker(10 * time.Second)
	defer reconcileTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.processDeployments(ctx)
			e.updateMetrics(ctx)
		case <-e.scheduleCh:
			e.processDeployments(ctx)
		case <-autoscaleTicker.C:
			e.evaluateAutoscale(ctx)
		case <-rebalanceTicker.C:
			e.evaluateRebalance(ctx)
		case <-reconcileTicker.C:
			go func() {
				if !reconcileOnce.TryLock() {
					return
				}
				defer reconcileOnce.Unlock()
				RunReconciliation(ctx, e.reconcilers, e.log)
			}()
		}
	}
}

// updateMetrics refreshes engine system metrics and cluster aggregate metrics in the Prometheus registry.
func (e *Engine) updateMetrics(ctx context.Context) {
	// Engine's own system metrics
	sysMetrics := e.metricsCollector.Collect()
	uptime := time.Since(e.startedAt)
	e.metricsRegistry.UpdateEngine(sysMetrics, uptime)

	// Cluster aggregates
	e.metricsRegistry.UpdateCluster(e.collectClusterStats(ctx))
	e.metricsRegistry.UpdateDeployments(e.collectDeploymentMetrics(ctx))
}

// collectClusterStats computes aggregated cluster statistics from the store.
func (e *Engine) collectClusterStats(ctx context.Context) metrics.ClusterStats {
	stats := metrics.ClusterStats{
		DeploymentsByStatus: make(map[string]int32),
		TasksByStatus:       make(map[string]int32),
	}

	// Agents
	nodeKeys, _ := e.store.List(ctx, types.KeyNodes)
	stats.TotalAgents = int32(len(nodeKeys)) //nolint:gosec // count is always small
	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := e.store.Get(ctx, key, &node); err != nil {
			continue
		}
		if node.Status == "ready" && time.Since(node.LastSeen) < agentStalenessThreshold {
			stats.ConnectedAgents++
		}
	}

	// Deployments
	deployKeys, _ := e.store.List(ctx, types.KeyDeployments)
	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := e.store.Get(ctx, key, &record); err != nil {
			continue
		}
		stats.DeploymentsByStatus[record.Status]++
	}

	// Tasks and containers
	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := e.store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}
		taskKeys, _ := e.store.List(ctx, types.KeyTasks+node.Name+"/")
		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := e.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.Type == types.TaskTypeCreateAndStart {
				stats.TasksByStatus[task.Status]++
				if task.Status == types.StatusCompleted {
					stats.TotalContainers++
					if task.ContainerStatus == types.StatusRunning {
						stats.HealthyContainers++
					}
				}
			}
		}
	}

	return stats
}

// collectDeploymentMetrics gathers per-deployment replica metrics for Prometheus.
func (e *Engine) collectDeploymentMetrics(ctx context.Context) []metrics.DeploymentMetrics {
	deployKeys, _ := e.store.List(ctx, types.KeyDeployments)
	var result []metrics.DeploymentMetrics

	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := e.store.Get(ctx, key, &record); err != nil {
			continue
		}

		dm := metrics.DeploymentMetrics{
			Name:     record.Name,
			Status:   record.Status,
			Strategy: record.UpdateStrategy,
			Services: make(map[string]metrics.ServiceReplicaMetrics),
		}

		// Initialize desired replicas from service definitions
		for svcName, svc := range record.Services {
			dm.Services[svcName] = metrics.ServiceReplicaMetrics{
				Desired: int32(svc.Replicas), //nolint:gosec // replica count is always small
			}
		}

		// Count healthy replicas from tasks
		allTasks := types.CollectDeploymentTasks(ctx, e.store, record.ID)
		for i := range allTasks {
			if allTasks[i].Type != types.TaskTypeCreateAndStart {
				continue
			}
			if allTasks[i].ContainerStatus == types.StatusRunning {
				if srm, ok := dm.Services[allTasks[i].ServiceName]; ok {
					srm.Healthy++
					dm.Services[allTasks[i].ServiceName] = srm
				}
			}
		}

		result = append(result, dm)
	}

	return result
}

// processDeployments checks for pending, deploying, and stopping deployments.
func (e *Engine) processDeployments(ctx context.Context) {
	keys, err := e.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return
	}

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := e.store.Get(ctx, key, &record); err != nil {
			continue
		}

		switch record.Status {
		case types.StatusPending:
			e.schedulePendingDeployment(ctx, &record)
		case types.StatusDeploying:
			e.checkDeployingDeployment(ctx, &record)
		case types.StatusStopping:
			e.checkStoppingDeployment(ctx, &record)
		}
	}
}

// hasConflictingDeployment checks if another deployment with the same name+tags (different ID)
// is currently stopping or deploying, which would cause port/container-name conflicts.
func (e *Engine) hasConflictingDeployment(ctx context.Context, deployment *types.DeploymentRecord) bool {
	keys, err := e.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return false
	}

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := e.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.ID == deployment.ID {
			continue
		}
		if record.Name != deployment.Name {
			continue
		}
		if !types.TagsEqual(record.Tags, deployment.Tags) {
			continue
		}
		if record.Status == types.StatusStopping || record.Status == types.StatusDeploying {
			return true
		}
	}

	return false
}

// schedulePendingDeployment assigns tasks to available agents.
// Uses a per-deployment distributed lock when etcd is available, so multiple
// engines can safely run this concurrently without creating duplicate tasks.
func (e *Engine) schedulePendingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
	// Acquire per-deployment lock to prevent duplicate scheduling across engines.
	// When the store doesn't support locks (e.g., MemoryStore in tests), skip locking.
	if lockStore, ok := e.store.(storage.LockStore); ok {
		unlock, lockErr := lockStore.Lock(ctx, "locks/deploy/"+deployment.ID, deploymentLockTimeout)
		if lockErr != nil {
			return // another engine is handling this, or etcd issue
		}
		defer unlock()

		// Re-read deployment after acquiring lock (may have been scheduled already)
		var fresh types.DeploymentRecord
		if err := e.store.Get(ctx, types.KeyDeployments+deployment.ID, &fresh); err != nil {
			return
		}
		if fresh.Status != types.StatusPending {
			return // already scheduled by another engine
		}
		deployment = &fresh
	}

	// Wait for conflicting deployments (same name, stopping/deploying) to finish
	if e.hasConflictingDeployment(ctx, deployment) {
		return
	}

	// Recreate strategy: wait for old service stop tasks to complete before scheduling
	if deployment.UpdateStrategy == types.UpdateStrategyRecreate && deployment.ReplacesID != "" {
		if !e.areReplacedServicesStopped(ctx, deployment) {
			return
		}
	}

	agents, err := ListAvailableAgents(ctx, e.store, deployment.Tags)
	if err != nil || len(agents) == 0 {
		return
	}

	// Validate cluster has enough total resources for this deployment
	if capErr := types.ValidateClusterCapacity(deployment, agents); capErr != nil {
		e.logger().Error("Cluster capacity exceeded", "name", deployment.Name, "error", capErr)
		deployment.Status = types.StatusFailed
		deployment.Error = capErr.Error()
		deployment.UpdatedAt = time.Now()
		_ = e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment)
		return
	}

	e.logger().Info("Scheduling deployment", "name", deployment.Name, "services", len(deployment.Services), "agents", len(agents))

	tasks, err := types.BuildTasksForDeployment(deployment, agents)
	if err != nil {
		e.logger().Error("Failed to schedule deployment", "name", deployment.Name, "error", err)
		deployment.Status = types.StatusFailed
		deployment.Error = err.Error()
		deployment.UpdatedAt = time.Now()
		_ = e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment)
		return
	}

	taskCount := 0
	for _, task := range tasks {
		taskKey := types.KeyTasks + task.AgentID + "/" + task.ID
		if err := e.store.Save(ctx, taskKey, task); err != nil {
			e.logger().Error("Failed to create task", "task_id", task.ID, "error", err)
			continue
		}

		e.logger().Info("Task dispatched", "task_id", task.ID, "agent", task.AgentID, "container", task.ContainerName)
		taskCount++
	}

	deployment.Status = types.StatusDeploying
	deployment.UpdatedAt = time.Now()
	if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err != nil {
		e.logger().Error("Failed to update deployment status", "error", err)
	}

	e.logger().Info("Dispatched tasks for deployment", "tasks", taskCount, "name", deployment.Name)
}

// checkDeployingDeployment checks if all tasks for a deployment have completed.
// Uses a per-deployment lock because blueGreenTeardownOld creates stop tasks (non-idempotent).
func (e *Engine) checkDeployingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
	if lockStore, ok := e.store.(storage.LockStore); ok {
		unlock, lockErr := lockStore.Lock(ctx, "locks/deploy/"+deployment.ID, deploymentLockTimeout)
		if lockErr != nil {
			return
		}
		defer unlock()

		var fresh types.DeploymentRecord
		if err := e.store.Get(ctx, types.KeyDeployments+deployment.ID, &fresh); err != nil {
			return
		}
		if fresh.Status != types.StatusDeploying {
			return
		}
		deployment = &fresh
	}

	nodeKeys, err := e.store.List(ctx, types.KeyNodes)
	if err != nil {
		return
	}

	totalTasks := 0
	completedTasks := 0
	failedTasks := 0
	var firstError string

	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := e.store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskPrefix := types.KeyTasks + node.Name + "/"
		taskKeys, err := e.store.List(ctx, taskPrefix)
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := e.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}

			if task.DeploymentID != deployment.ID {
				continue
			}
			// Only count create_and_start tasks — stop_and_remove tasks
			// (from Down or teardown) have the same DeploymentID and would
			// inflate totalTasks, preventing the deployment from reaching StatusRunning.
			if task.Type != types.TaskTypeCreateAndStart {
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

	// If all tasks completed successfully, wait for healthchecks before marking RUNNING.
	// This ensures the deployment is actually ready, not just started.
	// Services without healthchecks pass immediately (allHealthchecksHealthy returns true).
	if newStatus == types.StatusRunning && !e.allHealthchecksHealthy(ctx, deployment) {
		return // stay in DEPLOYING, check again on next loop tick
	}

	deployment.Status = newStatus
	deployment.Error = errMsg
	deployment.UpdatedAt = time.Now()
	if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
		if newStatus == types.StatusFailed {
			e.logger().Error("Deployment failed", "name", deployment.Name, "error", errMsg)
			e.emitEvent("deployment.failed", fmt.Sprintf("Deployment %s failed: %s", deployment.Name, errMsg), "error")
			if deployment.ReplacesID != "" {
				e.logger().Info("Blue-green: keeping old deployment running", "old_id", deployment.ReplacesID)
			}
		} else {
			e.logger().Info("Deployment is running", "name", deployment.Name, "containers", completedTasks)
			e.emitEvent("deployment.running", fmt.Sprintf("Deployment %s is running (%d containers)", deployment.Name, completedTasks), "info")
			if deployment.UpdateStrategy != types.UpdateStrategyRecreate {
				if e.allHealthchecksHealthy(ctx, deployment) {
					e.blueGreenTeardownOld(ctx, deployment)
				} else {
					e.logger().Info("Blue-green: waiting for healthchecks to pass", "name", deployment.Name)
				}
			}
		}
	}
}

// areReplacedServicesStopped checks whether all stop_and_remove tasks for the services
// in the new deployment have completed (or failed) in the old deployment.
// This is used by the recreate strategy to wait for old containers to stop before starting new ones.
func (e *Engine) areReplacedServicesStopped(ctx context.Context, deployment *types.DeploymentRecord) bool {
	oldTasks := types.CollectDeploymentTasks(ctx, e.store, deployment.ReplacesID)

	// Build set of service names in the new deployment
	newServiceNames := make(map[string]bool, len(deployment.Services))
	for name := range deployment.Services {
		newServiceNames[name] = true
	}

	for i := range oldTasks {
		if oldTasks[i].Type != types.TaskTypeStopAndRemove {
			continue
		}
		if !newServiceNames[oldTasks[i].ServiceName] {
			continue
		}
		// If any matching stop task is still pending or running, not ready yet
		if oldTasks[i].Status != types.StatusCompleted && oldTasks[i].Status != types.StatusFailed {
			return false
		}
	}

	return true
}

// allHealthchecksHealthy returns true if all containers with healthchecks in the
// deployment report healthy status. If no services have healthchecks, returns true.
func (e *Engine) allHealthchecksHealthy(ctx context.Context, deployment *types.DeploymentRecord) bool {
	// Check if any service has a healthcheck configured
	hasHealthcheck := false
	healthcheckServices := make(map[string]bool)
	for svcName, svc := range deployment.Services {
		if svc.Healthcheck != nil && !svc.Healthcheck.Disable {
			hasHealthcheck = true
			healthcheckServices[svcName] = true
		}
	}
	if !hasHealthcheck {
		return true
	}

	tasks := types.CollectDeploymentTasks(ctx, e.store, deployment.ID)
	for i := range tasks {
		task := &tasks[i]
		if task.Type != types.TaskTypeCreateAndStart {
			continue
		}
		if !healthcheckServices[task.ServiceName] {
			continue
		}
		if task.HealthStatus != "healthy" {
			return false
		}
	}
	return true
}

// blueGreenTeardownOld tears down the old deployment referenced by ReplacesID
// after the new deployment has reached StatusRunning.
// For selective service deploys (e.g. "up api"), services not being replaced
// are adopted into the new deployment so they keep running.
func (e *Engine) blueGreenTeardownOld(ctx context.Context, deployment *types.DeploymentRecord) {
	if deployment.ReplacesID == "" {
		return
	}

	oldKey := types.KeyDeployments + deployment.ReplacesID
	var oldDeployment types.DeploymentRecord
	if err := e.store.Get(ctx, oldKey, &oldDeployment); err != nil {
		e.logger().Warn("Blue-green: old deployment not found, skipping teardown", "old_id", deployment.ReplacesID)
		return
	}

	// For selective service deploys: adopt services not being replaced
	// so they continue running under the new deployment.
	e.adoptUnreplacedServices(ctx, deployment, &oldDeployment)

	count, err := teardownDeployment(ctx, e.store, &oldDeployment, oldKey)
	if err != nil {
		e.logger().Error("Blue-green: failed to teardown old deployment", "old_id", oldDeployment.ID, "error", err)
		return
	}
	e.logger().Info("Blue-green: tearing down old deployment", "old_id", oldDeployment.ID, "stop_tasks", count)
}

// adoptUnreplacedServices moves tasks and service definitions for services
// that exist in the old deployment but NOT in the new deployment.
// This prevents selective service deploys (e.g. "up api") from stopping
// unrelated services (e.g. "db").
func (e *Engine) adoptUnreplacedServices(ctx context.Context, newDeploy, oldDeploy *types.DeploymentRecord) {
	// Find services in old but not in new
	var adoptNames []string
	for svcName := range oldDeploy.Services {
		if _, inNew := newDeploy.Services[svcName]; !inNew {
			adoptNames = append(adoptNames, svcName)
		}
	}

	if len(adoptNames) == 0 {
		return // Full replacement, nothing to adopt
	}

	adoptSet := make(map[string]bool, len(adoptNames))
	for _, name := range adoptNames {
		adoptSet[name] = true
	}

	// Copy service definitions to new deployment
	for _, svcName := range adoptNames {
		newDeploy.Services[svcName] = oldDeploy.Services[svcName]
	}

	// Re-assign matching tasks from old deployment to new deployment
	oldTasks := types.CollectDeploymentTasks(ctx, e.store, oldDeploy.ID)
	for i := range oldTasks {
		if !adoptSet[oldTasks[i].ServiceName] || oldTasks[i].Type != types.TaskTypeCreateAndStart {
			continue
		}
		oldTasks[i].DeploymentID = newDeploy.ID
		taskKey := types.KeyTasks + oldTasks[i].AgentID + "/" + oldTasks[i].ID
		if err := e.store.Save(ctx, taskKey, &oldTasks[i]); err != nil {
			e.logger().Error("Blue-green: failed to adopt task", "task_id", oldTasks[i].ID, "error", err)
		}
	}

	// Save updated new deployment with adopted services
	newDeployKey := types.KeyDeployments + newDeploy.ID
	if err := e.store.Save(ctx, newDeployKey, newDeploy); err != nil {
		e.logger().Error("Blue-green: failed to save adopted services", "error", err)
	}

	e.logger().Info("Blue-green: adopted services from old deployment", "count", len(adoptNames), "services", adoptNames)
}

// checkStoppingDeployment checks if all stop_and_remove tasks have completed.
func (e *Engine) checkStoppingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
	tasks := types.CollectDeploymentTasks(ctx, e.store, deployment.ID)

	totalTasks := 0
	completedTasks := 0
	failedTasks := 0
	var firstError string

	for i := range tasks {
		if tasks[i].Type != types.TaskTypeStopAndRemove {
			continue
		}
		totalTasks++
		switch tasks[i].Status {
		case types.StatusCompleted:
			completedTasks++
		case types.StatusFailed:
			failedTasks++
			if firstError == "" {
				firstError = tasks[i].Error
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
		if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
			e.logger().Error("Deployment stop failed", "name", deployment.Name, "error", deployment.Error)
		}
		return
	}

	if completedTasks == totalTasks {
		deployment.Status = types.StatusStopped
		deployment.Error = ""
		deployment.UpdatedAt = time.Now()
		if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
			e.logger().Info("Deployment stopped", "name", deployment.Name, "containers_removed", completedTasks)
		}
	}
}

// ListAvailableAgents returns all registered agents with status "ready" that match the given deployment tags.
func ListAvailableAgents(ctx context.Context, store storage.StateStore, deploymentTags []string) ([]types.NodeRecord, error) {
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
		if node.Status == "ready" && types.TagsMatch(node.Tags, deploymentTags) {
			agents = append(agents, node)
		}
	}
	return agents, nil
}

// DetermineEngineIP returns the engine's non-loopback IPv4 address.
func DetermineEngineIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}
	return findNonLoopbackIPv4(addrs)
}

// findNonLoopbackIPv4 returns the first non-loopback IPv4 address from the given list.
func findNonLoopbackIPv4(addrs []net.Addr) (string, error) {
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

// ifaceAddr pairs an interface name with its IP address.
type ifaceAddr struct {
	Name string
	Addr *net.IPNet
}

// listInterfaceAddrs returns all interface addresses with their names.
func listInterfaceAddrs() ([]ifaceAddr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var result []ifaceAddr
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				result = append(result, ifaceAddr{Name: iface.Name, Addr: ipNet})
			}
		}
	}
	return result, nil
}

// listInterfaceAddrsFunc is a variable for testing.
var listInterfaceAddrsFunc = listInterfaceAddrs

// banyanInterfaces are interface names managed by Banyan that should be
// excluded from CIDR conflict checks.
var banyanInterfaces = map[string]bool{
	"banyan0":    true, // VPC bridge
	"banyan-wg":  true, // WireGuard data plane
	"wg-ctl-eng": true, // WireGuard control tunnel (engine)
	"wg-ctl-agt": true, // WireGuard control tunnel (agent)
	"wg-ctl-cli": true, // WireGuard control tunnel (CLI)
}

// checkCIDRConflict verifies the VPC CIDR doesn't overlap with any host network interfaces.
// Banyan-managed interfaces (bridges, tunnels) are excluded from the check.
func checkCIDRConflict(vpcCIDR string) error {
	_, vpcNet, err := net.ParseCIDR(vpcCIDR)
	if err != nil {
		return fmt.Errorf("invalid VPC CIDR: %w", err)
	}

	addrs, err := listInterfaceAddrsFunc()
	if err != nil {
		return fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, entry := range addrs {
		if banyanInterfaces[entry.Name] {
			continue
		}
		if entry.Addr.IP.To4() == nil {
			continue
		}
		if vpcNet.Contains(entry.Addr.IP) || entry.Addr.Contains(vpcNet.IP) {
			return fmt.Errorf("VPC CIDR %s overlaps with host interface %s (%s)", vpcCIDR, entry.Name, entry.Addr.String())
		}
	}
	return nil
}

// candidateCIDRs are private ranges tried in order when suggesting a
// conflict-free VPC CIDR. 10.0.0.0/16 is excluded (it is the common cloud
// default and frequently collides with the host subnet), and 10.200.0.0/16 is
// excluded (reserved for the control tunnel, see pkg/types.ControlTunnelCIDR).
var candidateCIDRs = []string{
	"10.10.0.0/16", "10.20.0.0/16", "10.30.0.0/16",
	"10.50.0.0/16", "10.100.0.0/16",
	"172.20.0.0/16", "172.30.0.0/16",
}

// suggestFreeCIDR returns the first candidate range that does not overlap any
// host network interface, or "" if every candidate conflicts.
func suggestFreeCIDR() (string, error) {
	for _, candidate := range candidateCIDRs {
		if err := checkCIDRConflict(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

// SuggestFreeVPCCIDR returns the first candidate VPC CIDR that does not overlap
// any host interface, or "" if none is free. Exported for the engine CLI.
func SuggestFreeVPCCIDR() (string, error) {
	return suggestFreeCIDR()
}

// ValidateVPCCIDR returns an error if cidr is not a valid CIDR or overlaps a
// host network interface. Exported for the engine CLI.
func ValidateVPCCIDR(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("VPC CIDR cannot be empty")
	}
	return checkCIDRConflict(cidr)
}
