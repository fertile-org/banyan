// Package agent implements the Banyan Agent data plane.
// It connects to the Engine, executes container tasks, and reports health.
package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/proxy"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// PortProxy manages TCP port forwarding to container backends.
// The agent uses this interface so the proxy implementation can be swapped.
type PortProxy interface {
	AddBackend(hostPort, containerPort int, containerName, containerIP string) error
	RemoveBackend(containerName string) error
	io.Closer
}

// Options configures the Agent.
type Options struct {
	AgentName       string
	EngineEndpoint  string   // Primary endpoint (backward compat)
	EngineEndpoints []string // Multiple endpoints for HA failover
	PublicKey       string   // WireGuard public key (for pubkey auth)
	WGPrivateKey    string   // WireGuard private key (for overlay)
	WGPublicKey     string   // WireGuard public key (for overlay)
	APIPort         string
	APIAddress      string
	PidFile         string
	Tags            []string
}

// Agent is the Banyan data-plane worker.
type Agent struct {
	opts             Options
	client           *EngineClient
	containers       *containerTracker
	proxy            PortProxy
	vpcEnabled       bool
	overlayDriver    overlay.OverlayDriver
	remoteBackends   map[string]ServiceBackend // key: containerName
	dnsManager       *dns.Manager
	dnsServer        *dns.Server
	gatewayIP        string          // bridge gateway IP (e.g., "10.0.45.1")
	registeredDNS    map[string]bool // tracked DNS hostnames for stale cleanup
	connected        atomic.Bool     // true when registered and heartbeat is healthy
	allocatedSubnet  string          // VPC subnet allocated by engine (e.g., "10.0.45.0/24")
	metricsCollector *metrics.SystemCollector
	log              *logging.Logger
}

// Reconnection constants.
const (
	maxConsecutiveHeartbeatFails = 3
	reconnectBackoffInitial      = 2 * time.Second
	reconnectBackoffMax          = 60 * time.Second
)

// Package-level function variables for testing.
var (
	taskExecutor              = executeTask
	containerStatusFunc       = getContainerStatus
	containerHealthStatusFunc = getContainerHealthStatus
	commandRunner             = runCommand
	containerIDGetter         = getContainerID
	containerRemover          = removeContainer
	containerIPGetter         = getContainerIP
	containerMetricsCollector = collectContainerMetrics // mockable in tests
	vpcNetworkEnabled         bool                     // set by Agent.Run() after VPC init
	dnsGatewayIPAddr          string         // set by Agent.Run() after DNS init, used in buildNerdctlRunArgs
	bindMountDataDir          = "/var/lib/banyan/data" // base dir for relative bind mount paths
	heartbeatInterval         = 15 * time.Second // overridable in tests
	taskPollInterval          = 2 * time.Second  // overridable in tests
	healthCheckInterval       = 10 * time.Second // overridable in tests
)

// New creates a new Agent.
func New(opts *Options) (*Agent, error) {
	return &Agent{
		opts:           *opts,
		containers:     &containerTracker{},
		remoteBackends: make(map[string]ServiceBackend),
		registeredDNS:  make(map[string]bool),
		log:            logging.New("agent"),
	}, nil
}

// logger returns the agent's logger, initializing it if nil (for test convenience).
func (a *Agent) logger() *logging.Logger {
	if a.log == nil {
		a.log = logging.New("agent")
	}
	return a.log
}

// Run connects to the engine, registers, and starts all loops.
// It blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	// Initialize system metrics collector
	a.metricsCollector = metrics.NewSystemCollector()
	// Seed the CPU sample so the first heartbeat gets a real reading
	a.metricsCollector.Collect()

	// Initialize iptables proxy for port management
	p, err := proxy.New()
	if err != nil {
		return fmt.Errorf("failed to initialize port proxy: %w", err)
	}
	a.proxy = p
	defer a.proxy.Close()

	// Connect to engine via gRPC
	if a.opts.PublicKey == "" && len(a.opts.EngineEndpoints) == 0 {
		return fmt.Errorf("no authentication configured (missing WireGuard public key)")
	}
	// Build endpoint list: prefer explicit list, fall back to single endpoint
	endpoints := a.opts.EngineEndpoints
	if len(endpoints) == 0 {
		endpoints = []string{a.opts.EngineEndpoint}
	}
	a.logger().Info("Connecting to engine", "endpoints", endpoints)
	client, err := NewEngineClientMulti(endpoints)
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}
	a.client = client
	defer client.Close()

	// Wait for engine to be ready
	a.logger().Info("Waiting for engine gRPC to be ready")
	if waitErr := a.waitForEngineGRPC(ctx, 30*time.Second); waitErr != nil {
		return fmt.Errorf("engine not ready: %w", waitErr)
	}
	a.logger().Info("Connected to engine")

	// Determine API address for this agent
	apiAddr := a.opts.APIAddress
	if apiAddr == "" {
		apiAddr = a.opts.AgentName + ":" + a.opts.APIPort
	}

	// Detect data-plane host IP for overlay peer endpoint
	detectedIP, _ := hostIPDetector()
	var hostIPStr string
	if detectedIP != nil {
		hostIPStr = detectedIP.String()
	}

	// Register node
	registryURL, vpcConfig, activeContainers, err := client.Register(ctx, RegisterRequest{
		Name:        a.opts.AgentName,
		APIAddr:     apiAddr,
		Tags:        a.opts.Tags,
		WGPublicKey: a.opts.WGPublicKey,
		HostIP:      hostIPStr,
	})
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}
	a.logger().Info("Agent registered", "agent", a.opts.AgentName, "registry", registryURL)
	a.connected.Store(true)

	// Initialize VPC overlay networking after registration
	if vpcConfig != nil && vpcConfig.AllocatedSubnet != "" {
		a.logger().Info("Initializing VPC overlay networking")
		if vpcErr := a.initializeVPCNetworking(ctx, vpcConfig); vpcErr != nil {
			return fmt.Errorf("VPC networking init failed: %w", vpcErr)
		}
		a.vpcEnabled = true
		a.allocatedSubnet = vpcConfig.AllocatedSubnet
		vpcNetworkEnabled = true
		a.logger().Info("VPC overlay networking ready")

		// Start DNS server on the bridge gateway IP
		if dnsErr := a.initializeDNS(ctx, vpcConfig.AllocatedSubnet); dnsErr != nil {
			return fmt.Errorf("DNS server init failed: %w", dnsErr)
		}
		dnsGatewayIPAddr = a.gatewayIP
	}

	// Restore proxy rules and tracking for containers that survived the restart
	a.restoreActiveContainers(ctx, activeContainers)

	// Do an immediate heartbeat to get VPC peers and service backends before
	// starting the task loop. This ensures cross-host routes are set up before
	// any containers are started, eliminating the 15s convergence delay.
	a.doOneHeartbeat(ctx)

	// Start the task execution loop
	go a.agentLoop(ctx)

	// Start heartbeat (periodic, every 15s)
	go a.agentHeartbeat(ctx)

	// Start container health monitoring
	go a.containerHealthLoop(ctx)

	// Start agent gRPC server for log streaming
	// Bind to agent's tunnel IP so only control tunnel peers can reach it
	agentTunnelIP := types.TunnelIPFromPublicKey(a.opts.PublicKey).String()
	go startAgentGRPC(ctx, &NerdctlLogProvider{}, agentTunnelIP, a.opts.APIPort)

	<-ctx.Done()

	// Stop DNS server
	if a.dnsServer != nil {
		if stopErr := a.dnsServer.Stop(); stopErr != nil {
			a.logger().Warn("DNS server stop failed", "error", stopErr)
		}
	}
	dnsGatewayIPAddr = ""

	// Clean up overlay networking
	if a.overlayDriver != nil {
		if cleanupErr := a.overlayDriver.Cleanup(context.Background()); cleanupErr != nil {
			a.logger().Warn("Overlay cleanup failed", "error", cleanupErr)
		}
	}

	a.logger().Info("Agent stopped")
	return nil
}

// agentLoop polls the engine for tasks assigned to this agent and executes them.
func (a *Agent) agentLoop(ctx context.Context) {
	ticker := time.NewTicker(taskPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.processTasks(ctx)
		}
	}
}

// processTasks polls and executes pending tasks for this agent.
func (a *Agent) processTasks(ctx context.Context) {
	if !a.connected.Load() {
		return
	}
	tasks, err := a.client.PollTasks(ctx, a.opts.AgentName)
	if err != nil {
		return
	}

	for _, pbTask := range tasks {
		// Report running (best-effort)
		if err := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusRunning, "", pbTask.ContainerName, nil); err != nil {
			a.logger().Warn("Failed to report running for task", "task_id", pbTask.Id, "error", err)
		}

		// Graceful drain before stopping a container:
		// 1. Remove from proxy (no new traffic)
		// 2. Remove from DNS
		// 3. Wait drain period for in-flight requests
		if pbTask.Type == types.TaskTypeStopAndRemove {
			if a.proxy != nil {
				if removeErr := a.proxy.RemoveBackend(pbTask.ContainerName); removeErr != nil {
					a.logger().Warn("Failed to remove proxy backend", "container", pbTask.ContainerName, "error", removeErr)
				}
			}
			if a.dnsManager != nil && pbTask.ServiceName != "" && pbTask.DeploymentName != "" {
				fqdn := pbTask.ServiceName + "." + pbTask.DeploymentName + ".internal"
				a.dnsManager.UnregisterHost(ctx, fqdn) //nolint:errcheck // best-effort
			}
			// Wait drain period (default 5s) for in-flight requests to complete
			drainPeriod := 5 * time.Second
			select {
			case <-time.After(drainPeriod):
			case <-ctx.Done():
			}
		}

		a.logger().Info("Executing task", "task_id", pbTask.Id, "type", pbTask.Type, "image", pbTask.Image)

		task := pbTaskToLocal(pbTask)
		result, err := taskExecutor(ctx, task)
		if err != nil {
			if reportErr := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusFailed, err.Error(), pbTask.ContainerName, nil); reportErr != nil {
				a.logger().Warn("Failed to report failure for task", "task_id", pbTask.Id, "error", reportErr)
			}
			a.logger().Error("Task failed", "task_id", pbTask.Id, "error", err)
			continue
		}

		var pbResult *banyanpb.TaskResult
		if result != nil {
			pbResult = &banyanpb.TaskResult{ContainerId: result.ContainerID}
		}
		if err := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusCompleted, "", pbTask.ContainerName, pbResult); err != nil {
			a.logger().Warn("Failed to report completion for task", "task_id", pbTask.Id, "error", err)
		}

		// Track containers and setup proxy after successful create_and_start
		if pbTask.Type == types.TaskTypeCreateAndStart {
			containerIP, proxyErr := a.setupProxyForContainer(ctx, task)
			if proxyErr != nil {
				a.logger().Warn("Proxy setup failed", "container", task.ContainerName, "error", proxyErr)
			}
			a.containers.Add(pbTask.ContainerName, pbTask.Id, containerIP)

			// Register local container in DNS immediately (no wait for heartbeat)
			if a.dnsManager != nil && task.ServiceName != "" && containerIP != "" {
				// Register deployment-scoped FQDN
				if task.DeploymentName != "" {
					fqdn := task.ServiceName + "." + task.DeploymentName + ".internal"
					a.dnsManager.RegisterHost(ctx, fqdn, net.ParseIP(containerIP)) //nolint:errcheck // best-effort
				}
				// Register short name (will be cleaned up if conflicting in reconcileDNS)
				hostname := task.ServiceName + ".internal"
				a.dnsManager.RegisterHost(ctx, hostname, net.ParseIP(containerIP)) //nolint:errcheck // best-effort
			}

			// Add container to network isolation rules immediately (no wait for heartbeat)
			if a.vpcEnabled && containerIP != "" && task.DeploymentName != "" {
				a.addContainerToIsolation(ctx, containerIP, task.DeploymentName)
			}
		}

		a.logger().Info("Task completed", "task_id", pbTask.Id, "container", pbTask.ContainerName)
	}
}

// pbTaskToLocal converts a protobuf TaskRecord to a local types.TaskRecord for execution.
func pbTaskToLocal(pb *banyanpb.TaskRecord) *types.TaskRecord {
	task := &types.TaskRecord{
		ID:                pb.Id,
		DeploymentID:      pb.DeploymentId,
		DeploymentName:    pb.DeploymentName,
		ServiceName:       pb.ServiceName,
		ReplicaIndex:      int(pb.ReplicaIndex),
		AgentID:           pb.AgentId,
		Type:              pb.Type,
		Status:            pb.Status,
		Image:             pb.Image,
		ContainerName:     pb.ContainerName,
		Ports:             pb.Ports,
		Environment:       pb.Environment,
		Command:           pb.Command,
		Restart:           pb.Restart,
		Entrypoint:        pb.Entrypoint,
		MemoryLimit:       pb.MemoryLimit,
		CPULimit:          pb.CpuLimit,
		MemoryReservation: pb.MemoryReservation,
	}
	if pb.Healthcheck != nil {
		task.Healthcheck = &types.ManifestHealthcheck{
			Test:        pb.Healthcheck.Test,
			Interval:    pb.Healthcheck.Interval,
			Timeout:     pb.Healthcheck.Timeout,
			Retries:     int(pb.Healthcheck.Retries),
			StartPeriod: pb.Healthcheck.StartPeriod,
			Disable:     pb.Healthcheck.Disable,
		}
	}
	for _, vol := range pb.Volumes {
		vm := types.VolumeMount{
			Type: vol.Type, Source: vol.Source, Target: vol.Target, ReadOnly: vol.ReadOnly,
		}
		if vol.Tmpfs != nil {
			vm.Tmpfs = &types.TmpfsOpt{Size: vol.Tmpfs.Size}
		}
		task.Volumes = append(task.Volumes, vm)
	}
	return task
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
	// Resolve NFS volumes to local host mounts before building nerdctl args
	if len(task.Volumes) > 0 {
		resolved, nfsErr := ResolveNFSVolumes(ctx, task.Volumes)
		if nfsErr != nil {
			return nil, nfsErr
		}
		task.Volumes = resolved
	}

	logging.Info("Pulling image", "image", task.Image)
	if err := commandRunner(ctx, "nerdctl", "pull", "--insecure-registry", task.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %w", task.Image, err)
	}

	args := buildNerdctlRunArgs(task, vpcNetworkEnabled)

	logging.Info("Starting container", "container", task.ContainerName)
	if err := commandRunner(ctx, "nerdctl", args...); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	containerID, err := containerIDGetter(ctx, task.ContainerName)
	if err != nil {
		containerID = "unknown"
	}

	return &types.TaskResultRecord{
		ContainerID: containerID,
	}, nil
}

func executeStopAndRemove(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
	logging.Info("Removing container", "container", task.ContainerName)

	if err := containerRemover(ctx, task.ContainerName); err != nil {
		return nil, err
	}

	return &types.TaskResultRecord{}, nil
}

// removeContainer force-removes a container by name using nerdctl.
func removeContainer(ctx context.Context, containerName string) error {
	rmCmd := exec.CommandContext(ctx, "nerdctl", "rm", "-f", containerName) //nolint:gosec // container name comes from engine
	var stderr bytes.Buffer
	rmCmd.Stderr = &stderr
	if err := rmCmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "No such container") {
			logging.Info("Container already removed", "container", containerName)
			return nil
		}
		return fmt.Errorf("failed to remove container: %s", errMsg)
	}
	return nil
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

func getContainerIP(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{.NetworkSettings.IPAddress}}", containerName) //nolint:gosec // container name comes from engine
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		return "", fmt.Errorf("failed to get container IP for %s: %s", containerName, errMsg)
	}
	ip := strings.TrimSpace(stdout.String())
	if ip == "" {
		return "", fmt.Errorf("empty IP for container %s", containerName)
	}
	return ip, nil
}

// buildNerdctlRunArgs builds the argument list for "nerdctl run" from a task.
// Ports are handled by the agent's TCP proxy, not by nerdctl port mapping.
// When vpcEnabled is true, containers are connected to the "banyan" CNI network.
func buildNerdctlRunArgs(task *types.TaskRecord, vpcEnabled bool) []string {
	args := []string{"run", "-d", "--name", task.ContainerName}

	if vpcEnabled {
		args = append(args, "--net", "banyan")
		if dnsGatewayIPAddr != "" {
			args = append(args, "--dns", dnsGatewayIPAddr, "--dns-search", "internal")
		}
	}

	if task.Restart != "" {
		args = append(args, "--restart", task.Restart)
	}

	if task.MemoryLimit != "" {
		args = append(args, "--memory", task.MemoryLimit)
	}
	if task.CPULimit != "" {
		args = append(args, "--cpus", task.CPULimit)
	}
	if task.MemoryReservation != "" {
		args = append(args, "--memory-reservation", task.MemoryReservation)
	}

	if task.Healthcheck != nil && !task.Healthcheck.Disable {
		hc := task.Healthcheck
		if len(hc.Test) > 0 {
			switch hc.Test[0] {
			case "CMD":
				args = append(args, "--health-cmd", strings.Join(hc.Test[1:], " "))
			case "CMD-SHELL":
				if len(hc.Test) > 1 {
					args = append(args, "--health-cmd", hc.Test[1])
				}
			case "NONE":
				args = append(args, "--no-healthcheck")
			default:
				// String form from compose — treat as CMD-SHELL
				args = append(args, "--health-cmd", strings.Join(hc.Test, " "))
			}
		}
		if hc.Interval != "" {
			args = append(args, "--health-interval", hc.Interval)
		}
		if hc.Timeout != "" {
			args = append(args, "--health-timeout", hc.Timeout)
		}
		if hc.Retries > 0 {
			args = append(args, "--health-retries", strconv.Itoa(hc.Retries))
		}
		if hc.StartPeriod != "" {
			args = append(args, "--health-start-period", hc.StartPeriod)
		}
	} else if task.Healthcheck != nil && task.Healthcheck.Disable {
		args = append(args, "--no-healthcheck")
	}

	for _, env := range task.Environment {
		args = append(args, "-e", env)
	}

	// Volume mounts
	for _, vol := range task.Volumes {
		switch vol.Type {
		case "tmpfs":
			mount := "type=tmpfs,target=" + vol.Target
			if vol.Tmpfs != nil && vol.Tmpfs.Size != "" {
				mount += ",tmpfs-size=" + vol.Tmpfs.Size
			}
			args = append(args, "--mount", mount)
		default: // "volume", "bind", or empty
			source := vol.Source
			// Resolve relative bind mount paths on the agent (not the CLI)
			if vol.Type == "bind" && source != "" && !strings.HasPrefix(source, "/") {
				source = filepath.Join(bindMountDataDir, source)
			}
			flag := source + ":" + vol.Target
			if vol.ReadOnly {
				flag += ":ro"
			}
			args = append(args, "-v", flag)
		}
	}

	if len(task.Entrypoint) > 0 {
		args = append(args, "--entrypoint", task.Entrypoint[0])
	}

	args = append(args, task.Image)

	// Entrypoint args (after index 0) become command args
	if len(task.Entrypoint) > 1 {
		args = append(args, task.Entrypoint[1:]...)
	}
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

// getContainerHealthStatus runs nerdctl inspect to get the container's health status.
// Returns "" if no healthcheck is configured, or "starting", "healthy", "unhealthy".
func getContainerHealthStatus(ctx context.Context, containerName string) string {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{end}}", containerName) //nolint:gosec // container name comes from engine
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(stdout.String())
}

// doOneHeartbeat sends a single heartbeat and reconciles peers/backends/DNS.
// Called once at startup to get VPC peers before the task loop starts.
func (a *Agent) doOneHeartbeat(ctx context.Context) {
	sysMetrics := a.metricsCollector.Collect()
	peers, backends, err := a.client.Heartbeat(ctx, a.opts.AgentName, a.opts.Tags, sysMetrics)
	if err != nil {
		a.logger().Warn("Initial heartbeat failed (will retry in loop)", "error", err)
		return
	}
	if a.vpcEnabled && len(peers) > 0 {
		if reconcileErr := a.reconcileVPCPeers(ctx, peers); reconcileErr != nil {
			a.logger().Warn("Initial peer reconciliation failed", "error", reconcileErr)
		} else {
			a.logger().Info("VPC peers reconciled at startup", "peer_count", len(peers))
		}
	}
	if a.vpcEnabled && a.proxy != nil {
		a.reconcileRemoteBackends(backends)
	}
	if a.vpcEnabled {
		a.reconcileNetworkIsolation(ctx, backends)
	}
	if a.dnsManager != nil {
		a.reconcileDNS(ctx, backends)
	}
}

func (a *Agent) agentHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	var consecutiveFails int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sysMetrics := a.metricsCollector.Collect()
			peers, backends, err := a.client.Heartbeat(ctx, a.opts.AgentName, a.opts.Tags, sysMetrics)
			if err != nil {
				consecutiveFails++
				if consecutiveFails < maxConsecutiveHeartbeatFails {
					a.logger().Warn("Heartbeat failed", "attempt", consecutiveFails, "max", maxConsecutiveHeartbeatFails, "error", err)
					continue
				}

				a.logger().Warn("Heartbeat failed, reconnecting", "consecutive_fails", consecutiveFails)
				a.connected.Store(false)
				a.reconnect(ctx)
				if ctx.Err() != nil {
					return
				}
				a.connected.Store(true)
				consecutiveFails = 0
				a.logger().Info("Reconnected, resuming heartbeat")
				continue
			}
			consecutiveFails = 0

			if a.vpcEnabled && len(peers) > 0 {
				if reconcileErr := a.reconcileVPCPeers(ctx, peers); reconcileErr != nil {
					a.logger().Warn("Peer reconciliation failed", "error", reconcileErr)
				}
			}
			if a.vpcEnabled && a.proxy != nil {
				a.reconcileRemoteBackends(backends)
			}
			if a.vpcEnabled {
				a.reconcileNetworkIsolation(ctx, backends)
			}
			if a.dnsManager != nil {
				a.reconcileDNS(ctx, backends)
			}
		}
	}
}

func (a *Agent) containerHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkContainerHealth(ctx)
		}
	}
}

// ContainerMetrics holds per-container resource usage.
type ContainerMetrics struct {
	CPUPercent   float64
	MemoryUsed  uint64
	MemoryLimit uint64
}

// collectContainerMetrics runs "nerdctl stats --no-stream" and parses per-container metrics.
func collectContainerMetrics(ctx context.Context, names []string) map[string]ContainerMetrics {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"stats", "--no-stream", "--format",
		"{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"}, names...)
	cmd := exec.CommandContext(ctx, "nerdctl", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}

	result := make(map[string]ContainerMetrics)
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		cpuStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), "%")
		cpuPercent, _ := strconv.ParseFloat(cpuStr, 64)

		// Parse memory: "123.4MiB / 1.5GiB" or "123456 / 987654"
		memParts := strings.SplitN(parts[2], "/", 2)
		memUsed := parseMemoryValue(strings.TrimSpace(memParts[0]))
		var memLimit uint64
		if len(memParts) > 1 {
			memLimit = parseMemoryValue(strings.TrimSpace(memParts[1]))
		}

		result[name] = ContainerMetrics{
			CPUPercent:  cpuPercent,
			MemoryUsed:  memUsed,
			MemoryLimit: memLimit,
		}
	}
	return result
}

// parseMemoryValue parses memory strings like "123.4MiB", "1.5GiB", "123456".
func parseMemoryValue(s string) uint64 {
	s = strings.TrimSpace(s)
	multiplier := uint64(1)
	if strings.HasSuffix(s, "GiB") {
		s = strings.TrimSuffix(s, "GiB")
		multiplier = 1024 * 1024 * 1024
	} else if strings.HasSuffix(s, "MiB") {
		s = strings.TrimSuffix(s, "MiB")
		multiplier = 1024 * 1024
	} else if strings.HasSuffix(s, "KiB") {
		s = strings.TrimSuffix(s, "KiB")
		multiplier = 1024
	} else if strings.HasSuffix(s, "B") {
		s = strings.TrimSuffix(s, "B")
	}
	val, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return uint64(val * float64(multiplier)) //nolint:gosec // memory values are always positive
}

func (a *Agent) checkContainerHealth(ctx context.Context) {
	if !a.connected.Load() {
		return
	}
	tracked := a.containers.List()
	if len(tracked) == 0 {
		return
	}

	// Collect per-container resource metrics
	var names []string
	for _, c := range tracked {
		names = append(names, c.containerName)
	}
	metrics := containerMetricsCollector(ctx, names)

	var statuses []*banyanpb.ContainerStatus
	for _, c := range tracked {
		status := containerStatusFunc(ctx, c.containerName)
		cs := &banyanpb.ContainerStatus{
			ContainerName: c.containerName,
			Status:        status,
			Ip:            c.containerIP,
		}
		if hs := containerHealthStatusFunc(ctx, c.containerName); hs != "" {
			cs.HealthStatus = hs
		}
		// Add resource metrics if available
		if m, ok := metrics[c.containerName]; ok {
			cs.CpuPercent = m.CPUPercent
			cs.MemoryUsedBytes = m.MemoryUsed
			cs.MemoryLimitBytes = m.MemoryLimit
		}
		statuses = append(statuses, cs)
	}

	if err := a.client.ReportContainerHealth(ctx, a.opts.AgentName, statuses); err != nil {
		a.logger().Warn("Failed to report container health", "error", err)
	}
}

// setupProxyForContainer registers a container's ports with the TCP proxy.
// Returns the container IP and any error. If the task has no ports, proxy setup
// is skipped but the IP is still retrieved for cross-host load balancing.
func (a *Agent) setupProxyForContainer(ctx context.Context, task *types.TaskRecord) (string, error) {
	containerIP, err := containerIPGetter(ctx, task.ContainerName)
	if err != nil {
		return "", err
	}

	if len(task.Ports) == 0 || a.proxy == nil {
		return containerIP, nil
	}

	for _, portStr := range task.Ports {
		hostPort, containerPort, parseErr := proxy.ParsePort(portStr)
		if parseErr != nil {
			return containerIP, parseErr
		}
		if addErr := a.proxy.AddBackend(hostPort, containerPort, task.ContainerName, containerIP); addErr != nil {
			return containerIP, addErr
		}
	}
	return containerIP, nil
}

// restoreActiveContainers re-establishes proxy rules and container tracking for containers
// that were running before the agent restarted. It verifies each container is still running
// via nerdctl inspect before restoring.
func (a *Agent) restoreActiveContainers(ctx context.Context, containers []ActiveContainer) {
	if len(containers) == 0 {
		return
	}

	restored := 0
	skipped := 0
	for i := range containers {
		ac := &containers[i]

		// Verify the container is actually still running
		status := containerStatusFunc(ctx, ac.ContainerName)
		if status != "running" {
			skipped++
			continue
		}

		// Re-fetch container IP (may have changed if network was recreated)
		containerIP, err := containerIPGetter(ctx, ac.ContainerName)
		if err != nil {
			a.logger().Warn("Could not get IP for container", "container", ac.ContainerName, "error", err)
			continue
		}

		// Restore proxy DNAT rules
		if a.proxy != nil && len(ac.Ports) > 0 {
			for _, portStr := range ac.Ports {
				hostPort, containerPort, parseErr := proxy.ParsePort(portStr)
				if parseErr != nil {
					a.logger().Warn("Bad port for container", "port", portStr, "container", ac.ContainerName, "error", parseErr)
					continue
				}
				if addErr := a.proxy.AddBackend(hostPort, containerPort, ac.ContainerName, containerIP); addErr != nil {
					a.logger().Warn("Proxy restore failed", "container", ac.ContainerName, "port", portStr, "error", addErr)
				}
			}
		}

		// Restore container tracking (for health checks)
		a.containers.Add(ac.ContainerName, ac.TaskID, containerIP)

		// Restore DNS registration
		if a.dnsManager != nil && ac.ServiceName != "" && containerIP != "" {
			hostname := ac.ServiceName + ".internal"
			a.dnsManager.RegisterHost(ctx, hostname, net.ParseIP(containerIP)) //nolint:errcheck // best-effort
		}

		restored++
	}

	if restored > 0 || skipped > 0 {
		a.logger().Info("Container restore", "restored", restored, "skipped", skipped)
	}
}

// waitForEngineGRPC retries a health check to verify the engine gRPC server is ready.
func (a *Agent) waitForEngineGRPC(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		hbCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := a.client.Health(hbCtx)
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

// reconnect re-registers with the engine after persistent heartbeat failures.
// It blocks until reconnection succeeds or ctx is cancelled.
// If multiple endpoints are configured, tries each before increasing backoff.
func (a *Agent) reconnect(ctx context.Context) {
	backoff := reconnectBackoffInitial

	for {
		// Try all endpoints before increasing backoff
		endpoints := len(a.client.endpoints)
		connected := false
		for range endpoints {
			endpoint := a.client.CurrentEndpoint()
			a.logger().Info("Waiting for engine", "endpoint", endpoint)
			if err := a.waitForEngineGRPC(ctx, 30*time.Second); err != nil {
				if ctx.Err() != nil {
					return
				}
				if endpoints > 1 {
					a.logger().Info("Engine not reachable, trying next endpoint", "endpoint", endpoint)
					if failErr := a.client.Failover(); failErr != nil {
						a.logger().Warn("Failover failed", "error", failErr)
					}
					continue
				}
			} else {
				connected = true
				break
			}
		}

		if !connected {
			a.logger().Warn("No reachable engine endpoint, retrying", "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > reconnectBackoffMax {
				backoff = reconnectBackoffMax
			}
			continue
		}

		apiAddr := a.opts.APIAddress
		if apiAddr == "" {
			apiAddr = a.opts.AgentName + ":" + a.opts.APIPort
		}

		reIP, _ := hostIPDetector()
		var reHostIPStr string
		if reIP != nil {
			reHostIPStr = reIP.String()
		}

		_, vpcConfig, activeContainers, err := a.client.Register(ctx, RegisterRequest{
			Name:        a.opts.AgentName,
			APIAddr:     apiAddr,
			Tags:        a.opts.Tags,
			WGPublicKey: a.opts.WGPublicKey,
			HostIP:      reHostIPStr,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger().Warn("Re-registration failed, retrying", "error", err, "backoff", backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > reconnectBackoffMax {
				backoff = reconnectBackoffMax
			}
			continue
		}

		// Handle VPC re-initialization if subnet changed
		if a.vpcEnabled && vpcConfig != nil && vpcConfig.AllocatedSubnet != "" {
			if a.allocatedSubnet != vpcConfig.AllocatedSubnet {
				a.logger().Info("VPC subnet changed, re-initializing overlay", "old", a.allocatedSubnet, "new", vpcConfig.AllocatedSubnet)
				if a.overlayDriver != nil {
					if cleanupErr := a.overlayDriver.Cleanup(ctx); cleanupErr != nil {
						a.logger().Warn("Overlay cleanup failed", "error", cleanupErr)
					}
				}
				if a.dnsServer != nil {
					a.dnsServer.Stop() //nolint:errcheck // best-effort cleanup before re-init
				}
				if vpcErr := a.initializeVPCNetworking(ctx, vpcConfig); vpcErr != nil {
					a.logger().Warn("VPC re-init failed", "error", vpcErr)
				}
				if dnsErr := a.initializeDNS(ctx, vpcConfig.AllocatedSubnet); dnsErr != nil {
					a.logger().Warn("DNS re-init failed", "error", dnsErr)
				}
				a.allocatedSubnet = vpcConfig.AllocatedSubnet
				dnsGatewayIPAddr = a.gatewayIP
			}
		}

		// Restore proxy rules for containers that survived the disconnect
		a.restoreActiveContainers(ctx, activeContainers)

		a.logger().Info("Reconnected and re-registered successfully")
		return
	}
}

// ServiceBackend represents a container backend for cross-host load balancing.
type ServiceBackend struct {
	ContainerName  string
	ContainerIP    string
	Ports          []string
	AgentName      string
	ServiceName    string
	DeploymentName string
}

// reconcileRemoteBackends adds/removes remote backends from the proxy
// based on the current set of backends from the engine.
func (a *Agent) reconcileRemoteBackends(backends []ServiceBackend) {
	// Build set of current remote backends (skip local)
	current := make(map[string]ServiceBackend)
	for _, b := range backends {
		if b.AgentName == a.opts.AgentName {
			continue // skip local backends
		}
		current[b.ContainerName] = b
	}

	// Remove stale or changed: in tracked but not in current, or IP changed
	for name, tracked := range a.remoteBackends {
		b, ok := current[name]
		if !ok || tracked.ContainerIP != b.ContainerIP {
			if err := a.proxy.RemoveBackend(name); err != nil {
				a.logger().Warn("Failed to remove remote backend", "backend", name, "error", err)
			}
			delete(a.remoteBackends, name)
		}
	}

	// Add new or re-add changed: in current but not in tracked
	for name, b := range current {
		if _, ok := a.remoteBackends[name]; ok {
			continue // already tracked with same IP
		}
		for _, portStr := range b.Ports {
			hostPort, containerPort, parseErr := proxy.ParsePort(portStr)
			if parseErr != nil {
				a.logger().Warn("Failed to parse port for remote backend", "port", portStr, "backend", name, "error", parseErr)
				continue
			}
			if addErr := a.proxy.AddBackend(hostPort, containerPort, b.ContainerName, b.ContainerIP); addErr != nil {
				a.logger().Warn("Failed to add remote backend", "backend", name, "error", addErr)
			}
		}
		a.remoteBackends[name] = b
	}
}
