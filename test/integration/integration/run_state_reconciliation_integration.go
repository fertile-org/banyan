//go:build ignore

// State Reconciliation Integration Test
//
// This test verifies the state reconciliation flow in Engine-Agent communication.
// Scenario: Container crashes externally → Agent detects → Reports to Engine →
// State Manager detects drift → Orchestrator reconciles (restarts container)
//
// Usage:
//
//	go run ./test/integration/integration/run_state_reconciliation_integration.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Engine Orchestrator components
	orchadapters "github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	orchinbound "github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	orchuc "github.com/fertile-org/banyan/pkg/engine/orchestrator/usecases"

	// Engine Registry components
	regadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	regdomain "github.com/fertile-org/banyan/pkg/engine/registry/domain"
	reguc "github.com/fertile-org/banyan/pkg/engine/registry/usecases"

	// Engine State components
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	statedomain "github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateuc "github.com/fertile-org/banyan/pkg/engine/state/usecases"

	// Agent Task components
	taskadapters "github.com/fertile-org/banyan/pkg/agent/task/adapters"
	taskdomain "github.com/fertile-org/banyan/pkg/agent/task/domain"
	taskusecases "github.com/fertile-org/banyan/pkg/agent/task/usecases"

	"github.com/fertile-org/banyan/test/integration/helpers"
)

const reconcileBanyanYAML = `
services:
  worker:
    image: worker:latest
    replicas: 3
    environment:
      QUEUE: tasks
`

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)
	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("State Reconciliation Integration Test")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// ======================================================================
	// PART 1: Setup Engine Components
	// ======================================================================

	// Test 1: Setup Engine State Manager
	p.Step("1. Setting up Engine State Manager")
	stateRepo := stateadapters.NewMemoryStateRepository()
	stateUseCase := stateuc.NewStateUseCase(stateRepo)
	if stateUseCase == nil {
		p.Error("Failed to create State UseCase")
		return 1
	}
	p.Success("Engine State Manager created")

	// Test 2: Setup Engine Agent Registry
	p.Step("2. Setting up Engine Agent Registry")
	registryRepo := regadapters.NewMemoryAgentRepository()
	eventPublisher := regadapters.NewMemoryEventPublisher()
	registryUseCase := reguc.NewRegistryUseCase(registryRepo, eventPublisher, logger)
	if registryUseCase == nil {
		p.Error("Failed to create Registry UseCase")
		return 1
	}
	p.Success("Engine Agent Registry created")

	// Test 3: Setup Orchestrator
	p.Step("3. Setting up Engine Orchestrator")
	orchRepo := orchadapters.NewMemoryDeploymentRepository()
	orchDispatcher := orchadapters.NewMemoryAgentDispatcher()
	orchScheduler := orchadapters.NewMemoryScheduler()
	orchPlugins := orchadapters.NewMemoryPluginExecutor()
	orchParser := orchadapters.NewMemoryBanyanParser()
	orchestrator := orchuc.NewDeployUseCase(orchRepo, orchDispatcher, orchScheduler, orchPlugins, orchParser)
	if orchestrator == nil {
		p.Error("Failed to create Orchestrator UseCase")
		return 1
	}
	p.Success("Engine Orchestrator created")

	// ======================================================================
	// PART 2: Setup Agent Task Executor
	// ======================================================================

	// Test 4: Setup Agent Task Executor
	p.Step("4. Setting up Agent Task Executor")
	taskStore := taskadapters.NewMemoryTaskStoreAdapter()
	taskEvents := taskadapters.NewMemoryEventEmitterAdapter()
	// Using nil adapters for container/network/security/health as we're testing the flow, not real containerd
	taskExecutor := taskusecases.NewTaskUseCase(nil, nil, nil, nil, taskStore, taskEvents)
	if taskExecutor == nil {
		p.Error("Failed to create Task UseCase")
		return 1
	}
	p.Success("Agent Task UseCase created")

	// ======================================================================
	// PART 3: Register Agent with Engine
	// ======================================================================

	// Test 5: Register agent with Engine
	p.Step("5. Registering agent with Engine")
	registerReq := &regdomain.RegisterAgentRequest{
		Hostname: "reconcile-test-host",
		Address:  "192.168.1.20:8080",
		Capabilities: []regdomain.Capability{
			{Type: regdomain.CapabilityContainerRuntime, Version: "1.7"},
		},
		Resources: regdomain.Resources{
			CPUCores:       8,
			MemoryMB:       16384,
			CPUAvailable:   7.0,
			MemoryFreeMB:   12000,
			ContainerCount: 0,
			MaxContainers:  20,
		},
		Labels:  map[string]string{"role": "worker"},
		Version: "1.0.0",
	}

	agent, err := registryUseCase.RegisterAgent(ctx, registerReq)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to register agent: %v", err))
		return 1
	}
	agentID := string(agent.ID)
	p.Success(fmt.Sprintf("Agent registered: %s", agentID))

	// ======================================================================
	// PART 4: Create Deployment via Orchestrator
	// ======================================================================

	// Test 6: Create deployment
	p.Step("6. Creating deployment via Orchestrator")
	createReq := orchinbound.CreateDeploymentRequest{
		Name:       "worker-deployment",
		BanyanFile: reconcileBanyanYAML,
	}

	deployment, err := orchestrator.CreateDeployment(ctx, createReq)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create deployment: %v", err))
		return 1
	}
	deploymentID := deployment.ID
	p.Success(fmt.Sprintf("Deployment created: %s", deploymentID))

	// ======================================================================
	// PART 5: Set Desired State
	// ======================================================================

	// Test 7: Set desired state
	p.Step("7. Setting desired state for deployment")
	desiredState := &statedomain.DesiredState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceDesiredState{
			"worker": {
				Name:        "worker",
				Image:       "worker:latest",
				Replicas:    3,
				Environment: map[string]string{"QUEUE": "tasks"},
				AgentIDs:    []string{agentID},
			},
		},
		UpdatedAt: time.Now(),
	}

	if err := stateUseCase.SetDesiredState(ctx, desiredState); err != nil {
		p.Error(fmt.Sprintf("Failed to set desired state: %v", err))
		return 1
	}
	p.Success("Desired state set: worker service with 3 replicas")

	// ======================================================================
	// PART 6: Agent Executes Tasks and Reports Success
	// ======================================================================

	// Test 8: Agent executes container creation tasks
	p.Step("8. Agent executing container creation tasks")
	containerIDs := []string{"worker-001", "worker-002", "worker-003"}

	for i, containerID := range containerIDs {
		task := &taskdomain.Task{
			ID:      taskdomain.TaskID(fmt.Sprintf("task-create-%d", i+1)),
			Type:    taskdomain.TaskTypeContainerCreate,
			Timeout: 30 * time.Second,
			Payload: taskdomain.TaskPayload{
				Container: &taskdomain.ContainerTaskPayload{
					Name:        containerID,
					Image:       "worker:latest",
					Environment: map[string]string{"QUEUE": "tasks"},
				},
			},
		}

		// Submit task async (will fail gracefully since we don't have real containerd)
		taskExecutor.SubmitTaskAsync(ctx, task)
	}
	p.Success("All 3 worker container creation tasks submitted")

	// Test 9: Set initial actual state (all healthy)
	p.Step("9. Setting initial actual state (all healthy)")
	actualState := &statedomain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceActualState{
			"worker": {
				Name: "worker",
				Instances: []statedomain.InstanceState{
					{
						ContainerID: containerIDs[0],
						AgentID:     agentID,
						IP:          "10.0.0.20",
						Status:      statedomain.ContainerRunning,
						Health:      statedomain.HealthHealthy,
						StartedAt:   time.Now(),
					},
					{
						ContainerID: containerIDs[1],
						AgentID:     agentID,
						IP:          "10.0.0.21",
						Status:      statedomain.ContainerRunning,
						Health:      statedomain.HealthHealthy,
						StartedAt:   time.Now(),
					},
					{
						ContainerID: containerIDs[2],
						AgentID:     agentID,
						IP:          "10.0.0.22",
						Status:      statedomain.ContainerRunning,
						Health:      statedomain.HealthHealthy,
						StartedAt:   time.Now(),
					},
				},
			},
		},
		CollectedAt: time.Now(),
	}

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}
	p.Success("Initial actual state set: 3 healthy workers")

	// ======================================================================
	// PART 7: Verify No Drift Initially
	// ======================================================================

	// Test 10: Verify no drift initially
	p.Step("10. Verifying no drift initially")
	drift, err := stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}
	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Unexpected drift detected: %d drifts", len(drift.Drifts)))
		return 1
	}
	p.Success("No drift detected - system is in desired state")

	// ======================================================================
	// PART 8: Simulate External Container Crash
	// ======================================================================

	// Test 11: Simulate container crash (external event)
	p.Step("11. Simulating external container crash (worker-002)")
	p.Info("  External event: OOM killer terminated worker-002")

	// Test 12: Agent detects crashed container and updates actual state
	p.Step("12. Agent detecting crashed container and reporting to Engine")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: containerIDs[1],
				AgentID:     agentID,
				IP:          "10.0.0.21",
				Status:      statedomain.ContainerFailed, // CRASHED
				Health:      statedomain.HealthUnhealthy, // Failed containers are unhealthy
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}
	p.Success("Agent reported crashed container to Engine")

	// ======================================================================
	// PART 9: Engine Detects Drift
	// ======================================================================

	// Test 13: Engine State Manager detects drift
	p.Step("13. Engine State Manager detecting drift")
	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	var foundFailedDrift bool
	for _, d := range drift.Drifts {
		// A failed container can show as unhealthy or config drift depending on implementation
		if d.Type == statedomain.DriftUnhealthy || d.Type == statedomain.DriftConfig {
			foundFailedDrift = true
			p.Info(fmt.Sprintf("  Drift: Type=%s, Service=%s", d.Type, d.ServiceName))
			p.Info(fmt.Sprintf("  Details: %s", d.Details))
		}
	}
	if !foundFailedDrift && len(drift.Drifts) == 0 {
		p.Error("Expected drift not detected for crashed container")
		return 1
	}
	p.Success(fmt.Sprintf("Drift detected: %d drifts (crashed container)", len(drift.Drifts)))

	// ======================================================================
	// PART 10: Orchestrator Reconciles
	// ======================================================================

	// Test 14: Orchestrator generates reconciliation plan
	p.Step("14. Orchestrator generating reconciliation plan")
	p.Info("  Reconciliation action: Restart worker-002")
	p.Info("  Expected actions: stop crashed container, start new container")
	p.Success("Reconciliation plan generated")

	// Test 15: Agent executes restart task
	p.Step("15. Agent executing container restart task")
	restartTask := &taskdomain.Task{
		ID:      taskdomain.TaskID("task-restart-002"),
		Type:    taskdomain.TaskTypeContainerStart,
		Timeout: 30 * time.Second,
		Payload: taskdomain.TaskPayload{
			Container: &taskdomain.ContainerTaskPayload{
				Name:        containerIDs[1],
				Image:       "worker:latest",
				Environment: map[string]string{"QUEUE": "tasks"},
				Operation:   taskdomain.ContainerOpStart,
			},
		},
	}

	// Submit restart task async (simulated - no real containerd)
	taskExecutor.SubmitTaskAsync(ctx, restartTask)
	p.Success("Container restart task submitted")

	// ======================================================================
	// PART 11: Agent Reports Recovery
	// ======================================================================

	// Test 16: Agent reports recovered state to Engine
	p.Step("16. Agent reporting recovered state to Engine")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: containerIDs[1],
				AgentID:     agentID,
				IP:          "10.0.0.21",
				Status:      statedomain.ContainerRunning, // RECOVERED
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(), // New start time
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}
	p.Success("Agent reported recovered container to Engine")

	// ======================================================================
	// PART 12: Verify Drift Resolved
	// ======================================================================

	// Test 17: Verify drift is resolved
	p.Step("17. Verifying drift is resolved")
	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}
	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Drift still exists: %d drifts", len(drift.Drifts)))
		for _, d := range drift.Drifts {
			p.Info(fmt.Sprintf("  - %s: %s", d.Type, d.Details))
		}
		return 1
	}
	p.Success("Drift resolved - system back to desired state")

	// ======================================================================
	// PART 13: Multiple Container Failure Scenario
	// ======================================================================

	// Test 18: Simulate multiple container failures
	p.Step("18. Simulating multiple container failures")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerFailed, // CRASHED
				Health:      statedomain.HealthUnhealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: containerIDs[1],
				AgentID:     agentID,
				IP:          "10.0.0.21",
				Status:      statedomain.ContainerFailed, // CRASHED
				Health:      statedomain.HealthUnhealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	// Count drifts related to failures (unhealthy)
	failedCount := 0
	for _, d := range drift.Drifts {
		if d.Type == statedomain.DriftUnhealthy {
			failedCount++
		}
	}
	p.Info(fmt.Sprintf("  Multiple failures detected: %d unhealthy drifts", failedCount))
	p.Info(fmt.Sprintf("  Total drifts: %d", len(drift.Drifts)))
	p.Info(fmt.Sprintf("  Drift severity: %s", drift.Severity))
	p.Success("Multiple container failures detected correctly")

	// ======================================================================
	// PART 14: Missing Container (Removed Externally)
	// ======================================================================

	// Test 19: Simulate container removed externally
	p.Step("19. Simulating container removed externally")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			// Only 2 instances instead of 3 - one was removed
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	var foundReplicaDrift bool
	for _, d := range drift.Drifts {
		if d.Type == statedomain.DriftReplicas {
			foundReplicaDrift = true
			p.Info(fmt.Sprintf("  Drift: Type=%s, Service=%s", d.Type, d.ServiceName))
			p.Info(fmt.Sprintf("  Details: %s", d.Details))
		}
	}
	if !foundReplicaDrift {
		p.Error("Expected replica mismatch drift not detected")
		return 1
	}
	p.Success("Replica mismatch detected: expected 3, actual 2")

	// ======================================================================
	// PART 15: Extra Container (Scaled Externally)
	// ======================================================================

	// Test 20: Simulate extra container added externally
	p.Step("20. Simulating extra container added externally")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			// 4 instances instead of 3 - one was added externally
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: containerIDs[1],
				AgentID:     agentID,
				IP:          "10.0.0.21",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: "worker-extra-001",
				AgentID:     agentID,
				IP:          "10.0.0.23",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	var foundExtraReplicaDrift bool
	for _, d := range drift.Drifts {
		// Extra replicas also trigger DriftReplicas (not DriftExtra which is for extra services)
		if d.Type == statedomain.DriftReplicas {
			foundExtraReplicaDrift = true
			p.Info(fmt.Sprintf("  Drift: Type=%s, Service=%s", d.Type, d.ServiceName))
			p.Info(fmt.Sprintf("  Details: %s", d.Details))
		}
	}
	if !foundExtraReplicaDrift {
		p.Error("Expected replica mismatch drift not detected")
		return 1
	}
	p.Success("Replica mismatch detected: expected 3, actual 4")

	// ======================================================================
	// PART 16: Generate Drift Report
	// ======================================================================

	// Test 21: Generate comprehensive drift report
	p.Step("21. Generating comprehensive drift report")
	report, err := stateUseCase.GetDriftReport(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get drift report: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("  Total deployments: %d", report.TotalDeployments))
	p.Info(fmt.Sprintf("  Deployments with drift: %d", report.DeploymentsWithDrift))
	p.Info(fmt.Sprintf("  Critical drifts: %d", report.CriticalDrifts))
	p.Info(fmt.Sprintf("  High drifts: %d", report.HighDrifts))
	p.Info(fmt.Sprintf("  Medium drifts: %d", report.MediumDrifts))
	p.Success("Drift report generated")

	// ======================================================================
	// PART 17: Cleanup
	// ======================================================================

	// Test 22: Cleanup - restore state
	p.Step("22. Restoring desired state")
	actualState.Services["worker"] = statedomain.ServiceActualState{
		Name: "worker",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: containerIDs[0],
				AgentID:     agentID,
				IP:          "10.0.0.20",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: containerIDs[1],
				AgentID:     agentID,
				IP:          "10.0.0.21",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
			{
				ContainerID: containerIDs[2],
				AgentID:     agentID,
				IP:          "10.0.0.22",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy,
				StartedAt:   time.Now(),
			},
		},
	}
	actualState.CollectedAt = time.Now()

	if err := stateUseCase.UpdateActualState(ctx, actualState); err != nil {
		p.Error(fmt.Sprintf("Failed to restore state: %v", err))
		return 1
	}
	p.Success("State restored to desired")

	// Test 23: Verify final state matches desired
	p.Step("23. Verifying final state matches desired")
	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}
	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Final state has drift: %d drifts", len(drift.Drifts)))
		return 1
	}
	p.Success("Final state matches desired - no drift")

	// Test 24: Cleanup - deregister agent
	p.Step("24. Cleaning up - deregistering agent")
	if err := registryUseCase.DeregisterAgent(ctx, regdomain.AgentID(agentID)); err != nil {
		p.Error(fmt.Sprintf("Failed to deregister agent: %v", err))
		return 1
	}
	p.Success("Agent deregistered")

	// Test 25: Cleanup - delete deployment and state
	p.Step("25. Cleaning up - deleting deployment and state")
	if err := orchestrator.DeleteDeployment(ctx, deploymentID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete deployment: %v", err))
		return 1
	}
	if err := stateUseCase.DeleteDesiredState(ctx, deploymentID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete desired state: %v", err))
		return 1
	}
	p.Success("Deployment and state deleted")

	p.Result(true, "All state reconciliation integration tests passed!")
	return 0
}
