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
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// Options configures the Engine.
type Options struct {
	StoreBackend    string            // "etcd" only
	StoreAddress    string            // resolved address for the store backend
	VPCCIDR         string
	RegistryPort    string
	GRPCPort        string
	DataDir         string
	EtcdUsername    string            // etcd RBAC username
	EtcdPassword    string            // etcd RBAC password
	EtcdCertFile    string            // client certificate for mTLS
	EtcdKeyFile     string            // client key for mTLS
	EtcdCAFile      string            // CA certificate for server verification
	WhitelistedKeys map[string]string // publicKey → agentName
	OverlayType     string            // "wireguard" (default) or "vxlan"
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
	if len(e.opts.WhitelistedKeys) > 0 {
		fmt.Printf("Public key authentication enabled (%d whitelisted keys)\n", len(e.opts.WhitelistedKeys))
	} else {
		fmt.Println("WARNING: No authentication configured (no whitelisted keys)")
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
		fmt.Printf("VPC overlay networking enabled (CIDR: %s)\n", e.opts.VPCCIDR)
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
	grpcSrv, err := startEngineGRPC(ctx, &grpcServerOptions{
		Store:           e.store,
		Port:            e.opts.GRPCPort,
		RegistryURL:     e.registryURL,
		Allocator:       allocator,
		PeerTracker:     peerTracker,
		VPCCIDR:         e.opts.VPCCIDR,
		WhitelistedKeys: e.opts.WhitelistedKeys,
		OverlayType:     e.opts.OverlayType,
	})
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
			fmt.Printf("[Engine] Deployment '%s' FAILED: %s\n", deployment.Name, errMsg)
			if deployment.ReplacesID != "" {
				fmt.Printf("[Engine] Blue-green: keeping old deployment '%s' running (new deployment failed)\n", deployment.ReplacesID)
			}
		} else {
			fmt.Printf("[Engine] Deployment '%s' is RUNNING (%d containers)\n", deployment.Name, completedTasks)
			if deployment.UpdateStrategy != types.UpdateStrategyRecreate {
				e.blueGreenTeardownOld(ctx, deployment)
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
		fmt.Printf("[Engine] Blue-green: old deployment '%s' not found, skipping teardown\n", deployment.ReplacesID)
		return
	}

	// For selective service deploys: adopt services not being replaced
	// so they continue running under the new deployment.
	e.adoptUnreplacedServices(ctx, deployment, &oldDeployment)

	count, err := teardownDeployment(ctx, e.store, &oldDeployment, oldKey)
	if err != nil {
		fmt.Printf("[Engine] Blue-green: failed to teardown old deployment '%s': %v\n", oldDeployment.ID, err)
		return
	}
	fmt.Printf("[Engine] Blue-green: tearing down old deployment '%s' (%d stop tasks created)\n", oldDeployment.ID, count)
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
			fmt.Printf("[Engine] Blue-green: failed to adopt task %s: %v\n", oldTasks[i].ID, err)
		}
	}

	// Save updated new deployment with adopted services
	newDeployKey := types.KeyDeployments + newDeploy.ID
	if err := e.store.Save(ctx, newDeployKey, newDeploy); err != nil {
		fmt.Printf("[Engine] Blue-green: failed to save adopted services: %v\n", err)
	}

	fmt.Printf("[Engine] Blue-green: adopted %d service(s) from old deployment: %v\n", len(adoptNames), adoptNames)
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
	"banyan.1":   true, // VXLAN data plane
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
