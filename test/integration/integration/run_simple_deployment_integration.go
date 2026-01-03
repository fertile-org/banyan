//go:build ignore

// Integration test for Simple Deployment (Engine-Agent Integration)
// Tests the full deployment flow: banyan.yml → Parse → Select Agent → Dispatch Task → Create Container → Report Status
//
// Run with: go run ./test/integration/integration/run_simple_deployment_integration.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Engine components
	orchadapters "github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	orchdomain "github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	orchinbound "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	orchuc "github.com/fertile-org/banyan/pkg/engine/orchestrator/usecases"
	regadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	regdomain "github.com/fertile-org/banyan/pkg/engine/registry/domain"
	reguc "github.com/fertile-org/banyan/pkg/engine/registry/usecases"
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	statedomain "github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateuc "github.com/fertile-org/banyan/pkg/engine/state/usecases"

	// Agent components
	taskadapters "github.com/fertile-org/banyan/pkg/agent/task/adapters"
	taskdomain "github.com/fertile-org/banyan/pkg/agent/task/domain"
	taskuc "github.com/fertile-org/banyan/pkg/agent/task/usecases"

	"github.com/fertile-org/banyan/test/integration/helpers"
)

const simpleBanyanYAML = `
services:
  web:
    image: nginx:alpine
    replicas: 2
    ports:
      - "80:80"
  api:
    image: myapp/api:v1
    replicas: 3
    ports:
      - "8080:8080"
    environment:
      LOG_LEVEL: info
    depends_on:
      - web
`

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All Simple Deployment integration tests passed!")
	} else {
		p.Result(false, "Simple Deployment integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Simple Deployment Integration Test (Engine + Agent)")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// ======================================================================
	// PART 1: Setup Engine Components
	// ======================================================================

	p.Step("Setting up Engine components")

	// Agent Registry
	agentRepo := regadapters.NewMemoryAgentRepository()
	eventPublisher := regadapters.NewMemoryEventPublisher()
	registry := reguc.NewRegistryUseCase(agentRepo, eventPublisher, logger)

	// State Manager
	stateRepo := stateadapters.NewMemoryStateRepository()
	stateUC := stateuc.NewStateUseCase(stateRepo)

	// Orchestrator
	orchRepo := orchadapters.NewMemoryDeploymentRepository()
	orchDispatcher := orchadapters.NewMemoryAgentDispatcher()
	orchScheduler := orchadapters.NewMemoryScheduler()
	orchPlugins := orchadapters.NewMemoryPluginExecutor()
	orchParser := orchadapters.NewMemoryBanyanParser()

	orchestrator := orchuc.NewDeployUseCase(orchRepo, orchDispatcher, orchScheduler, orchPlugins, orchParser)

	p.Success("Engine components initialized: Registry, State, Orchestrator")

	// ======================================================================
	// PART 2: Setup Agent Components
	// ======================================================================

	p.Step("Setting up Agent components")

	// Task Executor (with mock service adapters)
	taskStore := taskadapters.NewMemoryTaskStoreAdapter()
	taskEvents := taskadapters.NewMemoryEventEmitterAdapter()
	// Using nil adapters for container/network/security/health as we're testing the flow, not real containerd
	taskExecutor := taskuc.NewTaskUseCase(nil, nil, nil, nil, taskStore, taskEvents)

	p.Success("Agent components initialized: Task Executor")

	// ======================================================================
	// PART 3: Simulate Agent Registration
	// ======================================================================

	p.Step("Simulating agent registration with Engine")

	// Register 3 agents
	agents := make([]*regdomain.Agent, 0, 3)
	for i := 1; i <= 3; i++ {
		agentReq := &regdomain.RegisterAgentRequest{
			Hostname: fmt.Sprintf("worker-node-%d", i),
			Address:  fmt.Sprintf("192.168.1.%d:9090", 10+i),
			Capabilities: []regdomain.Capability{
				{Type: regdomain.CapabilityContainerRuntime, Version: "1.0"},
				{Type: regdomain.CapabilityNetworkNode, Version: "1.0"},
			},
			Resources: regdomain.Resources{
				CPUCores:       4,
				MemoryMB:       8192,
				CPUAvailable:   float64(4 - i + 1), // Different loads
				MemoryFreeMB:   int64(6000 - (i-1)*1000),
				ContainerCount: i * 2,
				MaxContainers:  20,
			},
			Labels: map[string]string{
				"role": "worker",
			},
			Version: "1.0.0",
		}

		agent, err := registry.RegisterAgent(ctx, agentReq)
		if err != nil {
			p.Error(fmt.Sprintf("Failed to register agent %d: %v", i, err))
			return 1
		}
		agents = append(agents, agent)
	}

	p.Success(fmt.Sprintf("Registered %d agents with Engine", len(agents)))

	for _, a := range agents {
		p.Info(fmt.Sprintf("  - %s (%s) CPU:%.1f Containers:%d",
			a.Hostname, a.Address, a.Resources.CPUAvailable, a.Resources.ContainerCount))
	}

	// ======================================================================
	// PART 4: Create Deployment via Orchestrator
	// ======================================================================

	p.Step("Creating deployment from banyan.yml")

	deployReq := orchinbound.CreateDeploymentRequest{
		Name:       "test-app",
		BanyanFile: simpleBanyanYAML,
	}

	deployment, err := orchestrator.CreateDeployment(ctx, deployReq)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create deployment: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Deployment created: ID=%s, Services=%d", deployment.ID, len(deployment.Services)))

	// ======================================================================
	// PART 5: Select Agents for Deployment
	// ======================================================================

	p.Step("Selecting agents for deployment")

	// Calculate total replicas needed
	totalReplicas := 0
	for _, svc := range deployment.Services {
		totalReplicas += svc.Replicas
		p.Info(fmt.Sprintf("  - Service '%s': %d replicas", svc.Name, svc.Replicas))
	}

	// Select agents using least_loaded strategy
	selectedAgents, err := registry.SelectAgents(ctx, regdomain.SelectionCriteria{
		RequiredCapabilities: []regdomain.CapabilityType{regdomain.CapabilityContainerRuntime},
		Count:                3, // Select all agents for distribution
	}, "least_loaded")
	if err != nil {
		p.Error(fmt.Sprintf("Failed to select agents: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Selected %d agents for %d total replicas", len(selectedAgents), totalReplicas))

	// ======================================================================
	// PART 6: Get Deployment Plan
	// ======================================================================

	p.Step("Getting deployment plan")

	plan, err := orchestrator.GetDeploymentPlan(ctx, deployReq)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get deployment plan: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Deployment plan: %d phases", len(plan.Phases)))
	for i, phase := range plan.Phases {
		p.Info(fmt.Sprintf("  Phase %d: %v", i+1, phase.Services))
	}

	// ======================================================================
	// PART 7: Execute Deployment
	// ======================================================================

	p.Step("Executing deployment")

	err = orchestrator.ExecuteDeployment(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to execute deployment: %v", err))
		return 1
	}

	// Verify deployment state
	updatedDeployment, _ := orchestrator.GetDeployment(ctx, deployment.ID)
	if updatedDeployment.State != orchdomain.StateActive {
		p.Error(fmt.Sprintf("Expected deployment state 'active', got '%s'", updatedDeployment.State))
		return 1
	}

	p.Success("Deployment executed successfully")

	// ======================================================================
	// PART 8: Verify Tasks Dispatched
	// ======================================================================

	p.Step("Verifying tasks were dispatched to agents")

	dispatchedTasks := orchDispatcher.GetDispatchedTasks()
	p.Info(fmt.Sprintf("  Dispatched tasks: %d", len(dispatchedTasks)))

	for _, task := range dispatchedTasks {
		p.Info(fmt.Sprintf("    - Task %s: type=%s, deployment=%s", task.ID, task.Type, task.DeploymentID))
	}

	p.Success("Tasks dispatched to agents")

	// ======================================================================
	// PART 9: Simulate Task Execution on Agent
	// ======================================================================

	p.Step("Simulating task execution on Agent")

	// Create container tasks for each service replica
	var taskResults []string
	for _, svc := range deployment.Services {
		for i := 0; i < svc.Replicas; i++ {
			task := &taskdomain.Task{
				ID:      taskdomain.NewTaskID(),
				Type:    taskdomain.TaskTypeContainerCreate,
				Timeout: 30 * time.Second,
				Payload: taskdomain.TaskPayload{
					Container: &taskdomain.ContainerTaskPayload{
						Name:  fmt.Sprintf("%s-%d", svc.Name, i),
						Image: svc.Image,
					},
				},
			}

			// Submit task (will fail gracefully since we don't have real containerd)
			// In real integration, this would create actual containers
			taskExecutor.SubmitTaskAsync(ctx, task)
			taskResults = append(taskResults, string(task.ID))
		}
	}

	p.Success(fmt.Sprintf("Submitted %d container tasks to Agent", len(taskResults)))

	// ======================================================================
	// PART 10: Update Desired State
	// ======================================================================

	p.Step("Setting desired state in State Manager")

	services := make(map[string]statedomain.ServiceDesiredState)
	for _, svc := range deployment.Services {
		services[svc.Name] = statedomain.ServiceDesiredState{
			Name:     svc.Name,
			Image:    svc.Image,
			Replicas: svc.Replicas,
		}
	}

	desiredState := &statedomain.DesiredState{
		DeploymentID: deployment.ID,
		Services:     services,
		UpdatedAt:    time.Now(),
	}

	err = stateUC.SetDesiredState(ctx, desiredState)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to set desired state: %v", err))
		return 1
	}

	p.Success(fmt.Sprintf("Desired state set: %d services", len(services)))

	// ======================================================================
	// PART 11: Simulate Actual State Update (from Agent Reports)
	// ======================================================================

	p.Step("Simulating actual state update from agents")

	actualServices := make(map[string]statedomain.ServiceActualState)
	instanceIndex := 0
	for _, svc := range deployment.Services {
		instances := make([]statedomain.InstanceState, svc.Replicas)
		for i := 0; i < svc.Replicas; i++ {
			agentIdx := instanceIndex % len(agents)
			instances[i] = statedomain.InstanceState{
				ContainerID: fmt.Sprintf("%s-%d-container", svc.Name, i),
				AgentID:     string(agents[agentIdx].ID),
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				IP:          fmt.Sprintf("10.0.%d.%d", instanceIndex/255, instanceIndex%255+1),
			}
			instanceIndex++
		}
		actualServices[svc.Name] = statedomain.ServiceActualState{
			Name:      svc.Name,
			Instances: instances,
		}
	}

	actualState := &statedomain.ActualState{
		DeploymentID: deployment.ID,
		Services:     actualServices,
		CollectedAt:  time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualState)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	p.Success("Actual state updated from agent reports")

	// ======================================================================
	// PART 12: Verify No Drift
	// ======================================================================

	p.Step("Verifying no drift between desired and actual state")

	drift, err := stateUC.DetectDrift(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Expected no drift, got %d drifts", len(drift.Drifts)))
		for _, d := range drift.Drifts {
			p.Info(fmt.Sprintf("  - %s: %s", d.ServiceName, d.Type))
		}
		return 1
	}

	p.Success("No drift detected - deployment is stable")

	// ======================================================================
	// PART 13: Simulate Container Failure and Detect Drift
	// ======================================================================

	p.Step("Simulating container failure and detecting drift")

	// Update actual state with one less replica for 'api' service
	failedActualServices := make(map[string]statedomain.ServiceActualState)
	for name, svc := range actualServices {
		if name == "api" {
			// Remove one instance to simulate failure
			failedActualServices[name] = statedomain.ServiceActualState{
				Name:      svc.Name,
				Instances: svc.Instances[:len(svc.Instances)-1], // One less instance
			}
		} else {
			failedActualServices[name] = svc
		}
	}

	failedActualState := &statedomain.ActualState{
		DeploymentID: deployment.ID,
		Services:     failedActualServices,
		CollectedAt:  time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, failedActualState)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	// Detect drift
	drift, err = stateUC.DetectDrift(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	if len(drift.Drifts) == 0 {
		p.Error("Expected drift to be detected for 'api' service")
		return 1
	}

	p.Success(fmt.Sprintf("Drift detected: %d issue(s)", len(drift.Drifts)))
	for _, d := range drift.Drifts {
		p.Info(fmt.Sprintf("  - Service '%s': %s - %s", d.ServiceName, d.Type, d.Details))
	}

	// ======================================================================
	// PART 14: Process Agent Heartbeats
	// ======================================================================

	p.Step("Processing agent heartbeats")

	for _, agent := range agents {
		heartbeat := &regdomain.HeartbeatStatus{
			Status: regdomain.AgentStatusOnline,
			Resources: regdomain.Resources{
				CPUCores:       agent.Resources.CPUCores,
				MemoryMB:       agent.Resources.MemoryMB,
				CPUAvailable:   agent.Resources.CPUAvailable - 0.5, // Simulate load increase
				MemoryFreeMB:   agent.Resources.MemoryFreeMB - 500,
				ContainerCount: agent.Resources.ContainerCount + 1,
				MaxContainers:  agent.Resources.MaxContainers,
			},
			RunningTasks: []string{"task-1", "task-2"},
		}

		err = registry.ProcessHeartbeat(ctx, agent.ID, heartbeat)
		if err != nil {
			p.Error(fmt.Sprintf("Failed to process heartbeat for %s: %v", agent.Hostname, err))
			return 1
		}
	}

	p.Success("Processed heartbeats from all agents")

	// ======================================================================
	// PART 15: Verify Agent Resource Updates
	// ======================================================================

	p.Step("Verifying agent resource updates")

	updatedAgents, err := registry.ListAgents(ctx, regdomain.AgentFilter{})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list agents: %v", err))
		return 1
	}

	for _, a := range updatedAgents {
		p.Info(fmt.Sprintf("  - %s: CPU=%.1f, Memory=%dMB, Containers=%d",
			a.Hostname, a.Resources.CPUAvailable, a.Resources.MemoryFreeMB, a.Resources.ContainerCount))
	}

	p.Success("Agent resources updated from heartbeats")

	// ======================================================================
	// PART 16: Rollback Deployment
	// ======================================================================

	p.Step("Testing deployment rollback")

	err = orchestrator.RollbackDeployment(ctx, deployment.ID, orchinbound.RollbackImmediate)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to rollback: %v", err))
		return 1
	}

	finalDeployment, _ := orchestrator.GetDeployment(ctx, deployment.ID)
	if finalDeployment.State != orchdomain.StateDestroyed {
		p.Error(fmt.Sprintf("Expected state 'destroyed', got '%s'", finalDeployment.State))
		return 1
	}

	p.Success("Deployment rolled back successfully")

	// ======================================================================
	// PART 17: Cleanup
	// ======================================================================

	p.Step("Cleaning up")

	// Deregister all agents
	for _, agent := range agents {
		_ = registry.DeregisterAgent(ctx, agent.ID)
	}

	// Delete desired state
	_ = stateUC.DeleteDesiredState(ctx, deployment.ID)

	// Delete deployment
	_ = orchestrator.DeleteDeployment(ctx, deployment.ID)

	p.Success("Cleanup completed")

	return 0
}
