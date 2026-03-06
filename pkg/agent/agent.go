// Package agent implements the Banyan Agent data plane.
// It connects to the Engine, executes container tasks, and reports health.
package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os/exec"
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
	NodeName       string
	EngineEndpoint string
	PublicKey      string // WireGuard public key (for pubkey auth)
	WGPrivateKey   string // WireGuard private key (for overlay)
	WGPublicKey    string // WireGuard public key (for overlay)
	APIPort        string
	APIAddress     string
	PidFile        string
	Tags           []string
}

// Agent is the Banyan data-plane worker.
type Agent struct {
	opts             Options
	client           *EngineClient
	containers       *containerTracker
	sessionToken     string
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
	vpcNetworkEnabled         bool   // set by Agent.Run() after VPC init
	dnsGatewayIPAddr          string // set by Agent.Run() after DNS init, used in buildNerdctlRunArgs
)

// New creates a new Agent with a random session token.
func New(opts *Options) (*Agent, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	return &Agent{
		opts:           *opts,
		containers:     &containerTracker{},
		sessionToken:   hex.EncodeToString(tokenBytes),
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
	a.logger().Info("Connecting to engine", "endpoint", a.opts.EngineEndpoint)
	if a.opts.PublicKey == "" {
		return fmt.Errorf("no authentication configured (missing WireGuard public key)")
	}
	client, err := NewEngineClient(a.opts.EngineEndpoint, a.opts.PublicKey)
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
		apiAddr = a.opts.NodeName + ":" + a.opts.APIPort
	}

	// Detect data-plane host IP for overlay peer endpoint
	detectedIP, _ := hostIPDetector()
	var hostIPStr string
	if detectedIP != nil {
		hostIPStr = detectedIP.String()
	}

	// Register node
	registryURL, vpcConfig, activeContainers, err := client.Register(ctx, a.opts.NodeName, apiAddr, a.sessionToken, a.opts.Tags, a.opts.WGPublicKey, hostIPStr)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	a.logger().Info("Node registered", "node", a.opts.NodeName, "registry", registryURL)
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

	// Start the task execution loop
	go a.agentLoop(ctx)

	// Start heartbeat
	go a.agentHeartbeat(ctx)

	// Start container health monitoring
	go a.containerHealthLoop(ctx)

	// Start agent gRPC server for log streaming
	go startAgentGRPC(ctx, &NerdctlLogProvider{}, a.opts.APIPort, a.sessionToken)

	<-ctx.Done()

	// Stop DNS server
	if a.dnsServer != nil {
		a.dnsServer.Stop()
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
	ticker := time.NewTicker(2 * time.Second)
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
	tasks, err := a.client.PollTasks(ctx, a.opts.NodeName)
	if err != nil {
		return
	}

	for _, pbTask := range tasks {
		// Report running (best-effort)
		if err := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusRunning, "", pbTask.ContainerName, nil); err != nil {
			a.logger().Warn("Failed to report running for task", "task_id", pbTask.Id, "error", err)
		}

		// Remove proxy backends before stopping a container
		if pbTask.Type == types.TaskTypeStopAndRemove && a.proxy != nil {
			a.proxy.RemoveBackend(pbTask.ContainerName)
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
				hostname := task.ServiceName + ".internal"
				a.dnsManager.RegisterHost(ctx, hostname, net.ParseIP(containerIP)) //nolint:errcheck // best-effort
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
	logging.Info("Pulling image", "image", task.Image)
	if err := commandRunner(ctx, "nerdctl", "pull", "--insecure-registry", task.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %v", task.Image, err)
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

func (a *Agent) agentHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	var consecutiveFails int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sysMetrics := a.metricsCollector.Collect()
			peers, backends, err := a.client.Heartbeat(ctx, a.opts.NodeName, a.sessionToken, a.opts.Tags, sysMetrics)
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
			if a.dnsManager != nil {
				a.reconcileDNS(ctx, backends)
			}
		}
	}
}

func (a *Agent) containerHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
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

func (a *Agent) checkContainerHealth(ctx context.Context) {
	if !a.connected.Load() {
		return
	}
	tracked := a.containers.List()
	if len(tracked) == 0 {
		return
	}

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
		statuses = append(statuses, cs)
	}

	if err := a.client.ReportContainerHealth(ctx, a.opts.NodeName, statuses); err != nil {
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
// The heartbeat goroutine calls this — no extra goroutine is needed.
func (a *Agent) reconnect(ctx context.Context) {
	backoff := reconnectBackoffInitial

	for {
		a.logger().Info("Waiting for engine", "endpoint", a.opts.EngineEndpoint)
		if err := a.waitForEngineGRPC(ctx, 30*time.Second); err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger().Warn("Engine not ready, retrying", "error", err, "backoff", backoff)
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
			apiAddr = a.opts.NodeName + ":" + a.opts.APIPort
		}

		reIP, _ := hostIPDetector()
		var reHostIPStr string
		if reIP != nil {
			reHostIPStr = reIP.String()
		}

		_, vpcConfig, activeContainers, err := a.client.Register(ctx, a.opts.NodeName, apiAddr, a.sessionToken, a.opts.Tags, a.opts.WGPublicKey, reHostIPStr)
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
	ContainerName string
	ContainerIP   string
	Ports         []string
	AgentName     string
	ServiceName   string
}

// reconcileRemoteBackends adds/removes remote backends from the proxy
// based on the current set of backends from the engine.
func (a *Agent) reconcileRemoteBackends(backends []ServiceBackend) {
	// Build set of current remote backends (skip local)
	current := make(map[string]ServiceBackend)
	for _, b := range backends {
		if b.AgentName == a.opts.NodeName {
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
