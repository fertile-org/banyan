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
	"os/exec"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/proxy"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
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
	AuthToken      string
	APIPort        string
	APIAddress     string
	PidFile        string
	Tags           []string
}

// Agent is the Banyan data-plane worker.
type Agent struct {
	opts          Options
	client        *EngineClient
	containers    *containerTracker
	sessionToken  string
	proxy         PortProxy
	vpcEnabled    bool
	overlayDriver overlay.OverlayDriver
}

// Package-level function variables for testing.
var (
	taskExecutor        = executeTask
	containerStatusFunc = getContainerStatus
	commandRunner       = runCommand
	containerIDGetter   = getContainerID
	containerRemover    = removeContainer
	containerIPGetter   = getContainerIP
	vpcNetworkEnabled   bool // set by Agent.Run() after VPC init
)

// New creates a new Agent with a random session token.
func New(opts *Options) (*Agent, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	return &Agent{
		opts:         *opts,
		containers:   &containerTracker{},
		sessionToken: hex.EncodeToString(tokenBytes),
	}, nil
}

// Run connects to the engine, registers, and starts all loops.
// It blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	// Initialize iptables proxy for port management
	p, err := proxy.New()
	if err != nil {
		return fmt.Errorf("failed to initialize port proxy: %w", err)
	}
	a.proxy = p
	defer a.proxy.Close()

	// Connect to engine via gRPC
	fmt.Printf("Connecting to Engine at %s...\n", a.opts.EngineEndpoint)
	client, err := NewEngineClient(a.opts.EngineEndpoint, a.opts.AuthToken)
	if err != nil {
		return fmt.Errorf("failed to connect to Engine: %w", err)
	}
	a.client = client
	defer client.Close()

	// Wait for engine to be ready
	fmt.Println("Waiting for Engine gRPC to be ready...")
	if waitErr := a.waitForEngineGRPC(ctx, 30*time.Second); waitErr != nil {
		return fmt.Errorf("engine not ready: %w", waitErr)
	}
	fmt.Println("Connected to Engine")

	// Determine API address for this agent
	apiAddr := a.opts.APIAddress
	if apiAddr == "" {
		apiAddr = a.opts.NodeName + ":" + a.opts.APIPort
	}

	// Register node
	registryURL, vpcConfig, err := client.Register(ctx, a.opts.NodeName, apiAddr, a.sessionToken, a.opts.Tags)
	if err != nil {
		return fmt.Errorf("failed to register node: %w", err)
	}
	fmt.Printf("Node registered: %s (registry: %s)\n", a.opts.NodeName, registryURL)

	// Initialize VPC overlay networking after registration
	if vpcConfig != nil && vpcConfig.AllocatedSubnet != "" {
		fmt.Println("Initializing VPC overlay networking...")
		if vpcErr := a.initializeVPCNetworking(ctx, vpcConfig); vpcErr != nil {
			return fmt.Errorf("VPC networking init failed: %w", vpcErr)
		}
		a.vpcEnabled = true
		vpcNetworkEnabled = true
		fmt.Println("VPC overlay networking ready")
	}

	// Start the task execution loop
	go a.agentLoop(ctx)

	// Start heartbeat
	go a.agentHeartbeat(ctx)

	// Start container health monitoring
	go a.containerHealthLoop(ctx)

	// Start agent gRPC server for log streaming
	go startAgentGRPC(ctx, &NerdctlLogProvider{}, a.opts.APIPort, a.sessionToken)

	<-ctx.Done()

	// Clean up overlay networking
	if a.overlayDriver != nil {
		if cleanupErr := a.overlayDriver.Cleanup(context.Background()); cleanupErr != nil {
			fmt.Printf("[Agent] WARNING: overlay cleanup failed: %v\n", cleanupErr)
		}
	}

	fmt.Println("Agent stopped")
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
	tasks, err := a.client.PollTasks(ctx, a.opts.NodeName)
	if err != nil {
		return
	}

	for _, pbTask := range tasks {
		// Report running (best-effort)
		if err := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusRunning, "", pbTask.ContainerName, nil); err != nil {
			fmt.Printf("[Agent] WARNING: failed to report running for task %s: %v\n", pbTask.Id, err)
		}

		// Remove proxy backends before stopping a container
		if pbTask.Type == types.TaskTypeStopAndRemove && a.proxy != nil {
			a.proxy.RemoveBackend(pbTask.ContainerName)
		}

		fmt.Printf("[Agent] Executing task %s: %s (image: %s)\n", pbTask.Id, pbTask.Type, pbTask.Image)

		task := pbTaskToLocal(pbTask)
		result, err := taskExecutor(ctx, task)
		if err != nil {
			if reportErr := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusFailed, err.Error(), pbTask.ContainerName, nil); reportErr != nil {
				fmt.Printf("[Agent] WARNING: failed to report failure for task %s: %v\n", pbTask.Id, reportErr)
			}
			fmt.Printf("[Agent] Task %s FAILED: %v\n", pbTask.Id, err)
			continue
		}

		var pbResult *banyanpb.TaskResult
		if result != nil {
			pbResult = &banyanpb.TaskResult{ContainerId: result.ContainerID}
		}
		if err := a.client.ReportTaskResult(ctx, pbTask.Id, pbTask.AgentId, types.StatusCompleted, "", pbTask.ContainerName, pbResult); err != nil {
			fmt.Printf("[Agent] WARNING: failed to report completion for task %s: %v\n", pbTask.Id, err)
		}

		// Track containers and setup proxy after successful create_and_start
		if pbTask.Type == types.TaskTypeCreateAndStart {
			if proxyErr := a.setupProxyForContainer(ctx, task); proxyErr != nil {
				fmt.Printf("[Agent] WARNING: proxy setup failed for %s: %v\n", task.ContainerName, proxyErr)
			}
			a.containers.Add(pbTask.ContainerName, pbTask.Id)
		}

		fmt.Printf("[Agent] Task %s COMPLETED (container: %s)\n", pbTask.Id, pbTask.ContainerName)
	}
}

// pbTaskToLocal converts a protobuf TaskRecord to a local types.TaskRecord for execution.
func pbTaskToLocal(pb *banyanpb.TaskRecord) *types.TaskRecord {
	return &types.TaskRecord{
		ID:            pb.Id,
		DeploymentID:  pb.DeploymentId,
		ServiceName:   pb.ServiceName,
		ReplicaIndex:  int(pb.ReplicaIndex),
		AgentID:       pb.AgentId,
		Type:          pb.Type,
		Status:        pb.Status,
		Image:         pb.Image,
		ContainerName: pb.ContainerName,
		Ports:         pb.Ports,
		Environment:   pb.Environment,
		Command:       pb.Command,
	}
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
	fmt.Printf("[Agent]   Pulling image %s...\n", task.Image)
	if err := commandRunner(ctx, "nerdctl", "pull", "--insecure-registry", task.Image); err != nil {
		return nil, fmt.Errorf("failed to pull image %s: %v", task.Image, err)
	}

	args := buildNerdctlRunArgs(task, vpcNetworkEnabled)

	fmt.Printf("[Agent]   Starting container %s...\n", task.ContainerName)
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
	fmt.Printf("[Agent]   Removing container %s...\n", task.ContainerName)

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
			fmt.Printf("[Agent]   Container %s already removed\n", containerName)
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
	}

	for _, env := range task.Environment {
		args = append(args, "-e", env)
	}

	args = append(args, task.Image)
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

func (a *Agent) agentHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers, err := a.client.Heartbeat(ctx, a.opts.NodeName, a.sessionToken, a.opts.Tags)
			if err != nil {
				fmt.Printf("[Agent] WARNING: heartbeat failed: %v\n", err)
				continue
			}
			if a.vpcEnabled && len(peers) > 0 {
				if reconcileErr := a.reconcileVPCPeers(ctx, peers); reconcileErr != nil {
					fmt.Printf("[Agent] WARNING: peer reconciliation failed: %v\n", reconcileErr)
				}
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
	tracked := a.containers.List()
	if len(tracked) == 0 {
		return
	}

	var statuses []*banyanpb.ContainerStatus
	for _, c := range tracked {
		status := containerStatusFunc(ctx, c.containerName)
		statuses = append(statuses, &banyanpb.ContainerStatus{
			ContainerName: c.containerName,
			Status:        status,
		})
	}

	if err := a.client.ReportContainerHealth(ctx, a.opts.NodeName, statuses); err != nil {
		fmt.Printf("[Agent] WARNING: failed to report container health: %v\n", err)
	}
}

// setupProxyForContainer registers a container's ports with the TCP proxy.
// If the task has no ports, this is a no-op.
func (a *Agent) setupProxyForContainer(ctx context.Context, task *types.TaskRecord) error {
	if len(task.Ports) == 0 || a.proxy == nil {
		return nil
	}

	containerIP, err := containerIPGetter(ctx, task.ContainerName)
	if err != nil {
		return err
	}

	for _, portStr := range task.Ports {
		hostPort, containerPort, parseErr := proxy.ParsePort(portStr)
		if parseErr != nil {
			return parseErr
		}
		if addErr := a.proxy.AddBackend(hostPort, containerPort, task.ContainerName, containerIP); addErr != nil {
			return addErr
		}
	}
	return nil
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
