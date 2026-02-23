// Package engine implements the Banyan Engine control plane.
// It manages deployments, schedules tasks to agents, and runs the gRPC server.
package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/registry"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc"
)

// Options configures the Engine.
type Options struct {
	StoreBackend string // "etcd" only
	StoreAddress string // resolved address for the store backend
	VPCCIDR      string
	RegistryPort string
	GRPCPort     string
	PasswordHash string // bcrypt hash of cluster password
	DataDir      string
	EtcdUsername string // etcd RBAC username
	EtcdPassword string // etcd RBAC password
	EtcdCertFile string // client certificate for mTLS
	EtcdKeyFile  string // client key for mTLS
	EtcdCAFile   string // CA certificate for server verification
}

// Engine is the Banyan control plane.
type Engine struct {
	store       storage.StateStore
	grpcServer  *engineGRPCServer
	opts        Options
	registryURL string
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

	e := &Engine{
		opts:  *opts,
		store: store,
	}

	return e, nil
}

// Run starts the engine: VPC init, registry, gRPC server, and orchestration loop.
// It blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	if e.opts.PasswordHash != "" {
		fmt.Println("Token-based authentication enabled")
	} else {
		fmt.Println("WARNING: No authentication configured")
	}

	// Initialize VPC network (etcd only — Flannel requires etcd natively)
	if e.opts.StoreBackend == "etcd" {
		fmt.Printf("Initializing VPC network with CIDR %s...\n", e.opts.VPCCIDR)
		if vpcErr := vpc.InitializeNetwork(ctx, []string{e.opts.StoreAddress}, e.opts.VPCCIDR); vpcErr != nil {
			fmt.Printf("Warning: VPC initialization: %v\n", vpcErr)
		}
		fmt.Println("VPC initialized")
	} else {
		fmt.Println("VPC networking skipped (requires etcd backend for Flannel)")
	}

	// Start embedded OCI registry
	fmt.Printf("Starting OCI registry on port %s...\n", e.opts.RegistryPort)
	registryListener, err := startRegistry(ctx, e.opts.RegistryPort)
	if err != nil {
		return fmt.Errorf("failed to start registry: %w", err)
	}
	fmt.Printf("OCI registry listening on :%s\n", e.opts.RegistryPort)

	// Determine engine IP and store registry URL
	engineIP, err := DetermineEngineIP()
	if err != nil {
		return fmt.Errorf("failed to determine engine IP: %w", err)
	}
	e.registryURL = fmt.Sprintf("%s:%s", engineIP, e.opts.RegistryPort)
	if saveErr := e.store.Save(ctx, types.KeyRegistry, e.registryURL); saveErr != nil {
		return fmt.Errorf("failed to save registry URL: %w", saveErr)
	}
	fmt.Printf("Registry URL: %s (saved to store)\n", e.registryURL)
	_ = registryListener

	// Start Engine gRPC server
	fmt.Printf("Starting Engine gRPC server on port %s...\n", e.opts.GRPCPort)
	grpcSrv, err := startEngineGRPC(ctx, e.store, e.opts.GRPCPort, e.opts.PasswordHash, e.registryURL)
	if err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}
	e.grpcServer = grpcSrv
	fmt.Printf("Engine gRPC server listening on :%s\n", e.opts.GRPCPort)

	// Start the orchestration loop
	go e.engineLoop(ctx)

	<-ctx.Done()
	return nil
}

// Close releases engine resources.
func (e *Engine) Close() {
	if e.store != nil {
		e.store.Close()
	}
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
		}
	}
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

// hasConflictingDeployment checks if another deployment with the same name (different ID)
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

	agents, err := ListAvailableAgents(ctx, e.store)
	if err != nil || len(agents) == 0 {
		return
	}

	fmt.Printf("[Engine] Scheduling deployment '%s' (%d services, %d agents)\n",
		deployment.Name, len(deployment.Services), len(agents))

	tasks := types.BuildTasksForDeployment(deployment, agents)

	taskCount := 0
	for _, task := range tasks {
		taskKey := types.KeyTasks + task.AgentID + "/" + task.ID
		if err := e.store.Save(ctx, taskKey, task); err != nil {
			fmt.Printf("[Engine] Failed to create task %s: %v\n", task.ID, err)
			continue
		}

		fmt.Printf("[Engine]   Task %s → agent %s (container: %s)\n", task.ID, task.AgentID, task.ContainerName)
		taskCount++
	}

	deployment.Status = types.StatusDeploying
	deployment.UpdatedAt = time.Now()
	if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err != nil {
		fmt.Printf("[Engine] Failed to update deployment status: %v\n", err)
	}

	fmt.Printf("[Engine] Dispatched %d tasks for deployment '%s'\n", taskCount, deployment.Name)
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
			fmt.Printf("[Engine] Deployment '%s' FAILED: %s\n", deployment.Name, errMsg)
			if deployment.ReplacesID != "" {
				fmt.Printf("[Engine] Blue-green: keeping old deployment '%s' running (new deployment failed)\n", deployment.ReplacesID)
			}
		} else {
			fmt.Printf("[Engine] Deployment '%s' is RUNNING (%d containers)\n", deployment.Name, completedTasks)
			e.blueGreenTeardownOld(ctx, deployment)
		}
	}
}

// blueGreenTeardownOld tears down the old deployment referenced by ReplacesID
// after the new deployment has reached StatusRunning.
func (e *Engine) blueGreenTeardownOld(ctx context.Context, deployment *types.DeploymentRecord) {
	if deployment.ReplacesID == "" {
		return
	}

	oldKey := types.KeyDeployments + deployment.ReplacesID
	var oldDeployment types.DeploymentRecord
	if err := e.store.Get(ctx, oldKey, &oldDeployment); err != nil {
		fmt.Printf("[Engine] Blue-green: old deployment '%s' not found, skipping teardown\n", deployment.ReplacesID)
		return
	}

	count, err := teardownDeployment(ctx, e.store, &oldDeployment, oldKey)
	if err != nil {
		fmt.Printf("[Engine] Blue-green: failed to teardown old deployment '%s': %v\n", oldDeployment.ID, err)
		return
	}
	fmt.Printf("[Engine] Blue-green: tearing down old deployment '%s' (%d stop tasks created)\n", oldDeployment.ID, count)
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
			fmt.Printf("[Engine] Deployment '%s' stop FAILED: %s\n", deployment.Name, deployment.Error)
		}
		return
	}

	if completedTasks == totalTasks {
		deployment.Status = types.StatusStopped
		deployment.Error = ""
		deployment.UpdatedAt = time.Now()
		if err := e.store.Save(ctx, types.KeyDeployments+deployment.ID, deployment); err == nil {
			fmt.Printf("[Engine] Deployment '%s' is STOPPED (%d containers removed)\n", deployment.Name, completedTasks)
		}
	}
}

// ListAvailableAgents returns all registered agents with status "ready".
func ListAvailableAgents(ctx context.Context, store storage.StateStore) ([]types.NodeRecord, error) {
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

// --- Registry helpers ---

func startRegistry(ctx context.Context, port string) (net.Listener, error) {
	handler := registry.New()

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Registry server error: %v\n", err)
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
