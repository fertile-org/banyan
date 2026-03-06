// Package engine implements the Banyan Engine control plane.
// It manages deployments, schedules tasks to agents, and runs the gRPC server.
package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Options configures the Engine.
type Options struct {
	StoreBackend    string // "etcd" only
	StoreAddress    string // resolved address for the store backend
	VPCCIDR         string
	RegistryPort    string
	GRPCPort        string
	MetricsPort     string // Prometheus /metrics HTTP port (default "9090")
	DataDir         string
	EtcdUsername    string            // etcd RBAC username
	EtcdPassword    string            // etcd RBAC password
	EtcdCertFile    string            // client certificate for mTLS
	EtcdKeyFile     string            // client key for mTLS
	EtcdCAFile      string            // CA certificate for server verification
	WhitelistedKeys map[string]string // publicKey → agentName
}

// Engine is the Banyan control plane.
type Engine struct {
	store            storage.StateStore
	grpcServer       *engineGRPCServer
	opts             Options
	registryURL      string
	metricsRegistry  *metrics.EngineMetricsRegistry
	metricsCollector *metrics.SystemCollector
	events           EventLog
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

	e := &Engine{
		opts:             *opts,
		store:            store,
		metricsRegistry:  metrics.NewEngineMetricsRegistry(),
		metricsCollector: metrics.NewSystemCollector(),
		events:           eventLog,
		startedAt:        time.Now(),
		log:              logging.New("engine"),
	}
	// Seed the CPU sample so the first metrics read gets a real value
	e.metricsCollector.Collect()

	return e, nil
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
	} else {
		e.logger().Warn("No authentication configured (no whitelisted keys)")
	}

	// Initialize VPC overlay networking (subnet allocation + peer tracking)
	var allocator *overlay.SubnetAllocator
	var peerTracker *overlay.PeerTracker
	if e.opts.VPCCIDR != "" {
		if err := checkCIDRConflict(e.opts.VPCCIDR); err != nil {
			return fmt.Errorf("VPC CIDR conflict: %w", err)
		}
		var allocErr error
		allocator, allocErr = overlay.NewSubnetAllocator(e.opts.VPCCIDR)
		if allocErr != nil {
			return fmt.Errorf("failed to create subnet allocator: %w", allocErr)
		}
		peerTracker = overlay.NewPeerTracker()
		e.logger().Info("VPC overlay networking enabled", "cidr", e.opts.VPCCIDR)
	}

	// Start embedded OCI registry
	e.logger().Info("Starting OCI registry", "port", e.opts.RegistryPort)
	registryListener, err := startRegistry(ctx, e.opts.RegistryPort)
	if err != nil {
		return fmt.Errorf("failed to start registry: %w", err)
	}
	e.logger().Info("OCI registry listening", "port", e.opts.RegistryPort)

	// Determine engine IP and store registry URL
	engineIP, err := DetermineEngineIP()
	if err != nil {
		return fmt.Errorf("failed to determine engine IP: %w", err)
	}
	e.registryURL = fmt.Sprintf("%s:%s", engineIP, e.opts.RegistryPort)
	if saveErr := e.store.Save(ctx, types.KeyRegistry, e.registryURL); saveErr != nil {
		return fmt.Errorf("failed to save registry URL: %w", saveErr)
	}
	e.logger().Info("Registry URL saved to store", "url", e.registryURL)
	_ = registryListener

	// Start Engine gRPC server
	e.logger().Info("Starting gRPC server", "port", e.opts.GRPCPort)
	grpcSrv, err := startEngineGRPC(ctx, &grpcServerOptions{
		Store:           e.store,
		Port:            e.opts.GRPCPort,
		RegistryURL:     e.registryURL,
		Allocator:       allocator,
		PeerTracker:     peerTracker,
		VPCCIDR:         e.opts.VPCCIDR,
		WhitelistedKeys: e.opts.WhitelistedKeys,
		MetricsRegistry: e.metricsRegistry,
		Events:          e.events,
		StartedAt:       e.startedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	e.grpcServer = grpcSrv
	e.logger().Info("gRPC server listening", "port", e.opts.GRPCPort)

	// Start Prometheus metrics HTTP server
	metricsPort := e.opts.MetricsPort
	if metricsPort == "" {
		metricsPort = "9090"
	}
	if startErr := e.startMetricsHTTP(ctx, metricsPort); startErr != nil {
		return fmt.Errorf("failed to start metrics server: %w", startErr)
	}
	e.logger().Info("Prometheus metrics available", "port", metricsPort)

	// Start the orchestration loop
	go e.engineLoop(ctx)

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

	lis, err := net.Listen("tcp", ":"+port)
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

// engineLoop is the main orchestration loop.
func (e *Engine) engineLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.processDeployments(ctx)
			e.updateMetrics(ctx)
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
		if node.Status == "ready" && time.Since(node.LastSeen) < 60*time.Second {
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
func (e *Engine) schedulePendingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
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
func (e *Engine) checkDeployingDeployment(ctx context.Context, deployment *types.DeploymentRecord) {
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

// --- Registry helpers ---

func startRegistry(ctx context.Context, port string) (net.Listener, error) {
	regLog := logging.New("engine.registry")
	handler := registry.New(registry.Logger(regLog.StdLogger()))

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logging.Error("Registry server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return listener, nil
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
