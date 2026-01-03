//go:build ignore

// Health Monitoring Integration Test
//
// This test verifies health monitoring in Engine-Agent communication.
// Scenario: Container deployed with health check → Agent monitors →
// Health check fails → Agent reports → Engine detects drift → Engine triggers restart
//
// Usage:
//
//	go run ./test/integration/integration/run_health_monitoring_integration.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Agent Health components
	healthadapters "github.com/fertile-org/banyan/pkg/agent/health/adapters"
	healthdomain "github.com/fertile-org/banyan/pkg/agent/health/domain"
	healthoutbound "github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
	healthusecases "github.com/fertile-org/banyan/pkg/agent/health/usecases"

	// Engine Registry components
	registryadapters "github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	registrydomain "github.com/fertile-org/banyan/pkg/engine/registry/domain"
	registryusecases "github.com/fertile-org/banyan/pkg/engine/registry/usecases"

	// Engine State components
	stateadapters "github.com/fertile-org/banyan/pkg/engine/state/adapters"
	statedomain "github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateusecases "github.com/fertile-org/banyan/pkg/engine/state/usecases"

	"github.com/fertile-org/banyan/test/integration/helpers"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)
	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Health Monitoring Integration Test")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// ======================================================================
	// PART 1: Setup Engine Components
	// ======================================================================

	// Test 1: Setup Engine State Manager
	p.Step("1. Setting up Engine State Manager")
	stateRepo := stateadapters.NewMemoryStateRepository()
	stateUseCase := stateusecases.NewStateUseCase(stateRepo)
	if stateUseCase == nil {
		p.Error("Failed to create State UseCase")
		return 1
	}
	p.Success("Engine State Manager created")

	// Test 2: Setup Engine Agent Registry
	p.Step("2. Setting up Engine Agent Registry")
	registryRepo := registryadapters.NewMemoryAgentRepository()
	eventPublisher := registryadapters.NewMemoryEventPublisher()
	registryUseCase := registryusecases.NewRegistryUseCase(registryRepo, eventPublisher, logger)
	if registryUseCase == nil {
		p.Error("Failed to create Registry UseCase")
		return 1
	}
	p.Success("Engine Agent Registry created")

	// ======================================================================
	// PART 2: Setup Agent Health Monitor
	// ======================================================================

	// Test 3: Setup Agent Health Monitor
	p.Step("3. Setting up Agent Health Monitor")
	checkStore := healthadapters.NewMemoryCheckStore()
	systemMonitor := healthadapters.NewSystemMonitorAdapter()
	tcpChecker := healthadapters.NewTCPChecker()
	httpChecker := healthadapters.NewHTTPChecker()

	checkers := []healthoutbound.HealthChecker{tcpChecker, httpChecker}
	healthUseCase := healthusecases.NewHealthUseCase(checkers, checkStore, systemMonitor)
	if healthUseCase == nil {
		p.Error("Failed to create Health UseCase")
		return 1
	}
	p.Success("Agent Health Monitor created with HTTP and TCP checkers")

	// ======================================================================
	// PART 3: Agent Registration
	// ======================================================================

	// Test 4: Register agent with Engine
	p.Step("4. Registering agent with Engine")
	registerReq := &registrydomain.RegisterAgentRequest{
		Hostname: "health-test-host",
		Address:  "192.168.1.10:8080",
		Capabilities: []registrydomain.Capability{
			{Type: registrydomain.CapabilityContainerRuntime, Version: "1.7"},
		},
		Resources: registrydomain.Resources{
			CPUCores:       4,
			MemoryMB:       8192,
			CPUAvailable:   3.5,
			MemoryFreeMB:   6000,
			ContainerCount: 0,
			MaxContainers:  10,
		},
		Labels:  map[string]string{"role": "health-monitor"},
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
	// PART 4: Set Desired and Actual State
	// ======================================================================

	// Test 5: Set desired state with health check config
	p.Step("5. Setting desired state with health check configuration")
	deploymentID := "health-deployment-001"
	desiredState := &statedomain.DesiredState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceDesiredState{
			"web-service": {
				Name:     "web-service",
				Image:    "nginx:latest",
				Replicas: 2,
				Ports: []statedomain.PortMapping{
					{ContainerPort: 80, HostPort: 80, Protocol: "tcp"},
				},
				AgentIDs: []string{agentID},
			},
		},
		UpdatedAt: time.Now(),
	}

	if err := stateUseCase.SetDesiredState(ctx, desiredState); err != nil {
		p.Error(fmt.Sprintf("Failed to set desired state: %v", err))
		return 1
	}
	p.Success("Desired state set with web-service (2 replicas)")

	// Test 6: Simulate initial healthy deployment
	p.Step("6. Simulating initial healthy deployment")
	container1ID := healthdomain.ContainerID("container-health-001")
	container2ID := healthdomain.ContainerID("container-health-002")

	actualState := &statedomain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]statedomain.ServiceActualState{
			"web-service": {
				Name: "web-service",
				Instances: []statedomain.InstanceState{
					{
						ContainerID: string(container1ID),
						AgentID:     agentID,
						IP:          "10.0.0.10",
						Status:      statedomain.ContainerRunning,
						Health:      statedomain.HealthHealthy,
						StartedAt:   time.Now(),
					},
					{
						ContainerID: string(container2ID),
						AgentID:     agentID,
						IP:          "10.0.0.11",
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
	p.Success("Actual state set with 2 healthy containers")

	// ======================================================================
	// PART 5: Register Health Checks
	// ======================================================================

	// Test 7: Register health checks for containers
	p.Step("7. Registering health checks for containers")
	check1 := &healthdomain.HealthCheck{
		ID:          healthdomain.CheckID("http-check-001"),
		ContainerID: container1ID,
		Type:        healthdomain.CheckTypeHTTP,
		Target:      "http://10.0.0.10:80/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
		Retries:     3,
		StartPeriod: 10 * time.Second,
	}

	check2 := &healthdomain.HealthCheck{
		ID:          healthdomain.CheckID("http-check-002"),
		ContainerID: container2ID,
		Type:        healthdomain.CheckTypeHTTP,
		Target:      "http://10.0.0.11:80/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
		Retries:     3,
		StartPeriod: 10 * time.Second,
	}

	if err := healthUseCase.RegisterCheck(ctx, check1); err != nil {
		p.Error(fmt.Sprintf("Failed to register check 1: %v", err))
		return 1
	}
	if err := healthUseCase.RegisterCheck(ctx, check2); err != nil {
		p.Error(fmt.Sprintf("Failed to register check 2: %v", err))
		return 1
	}
	p.Success("Health checks registered for both containers")

	// ======================================================================
	// PART 6: Drift Detection (Initial - No Drift)
	// ======================================================================

	// Test 8: Verify no drift initially
	p.Step("8. Verifying no drift initially")
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
	// PART 7: Simulate Health Check Failure
	// ======================================================================

	// Test 9: Simulate health check failure on container 1
	p.Step("9. Simulating health check failure on container 1")
	failedResult := &healthdomain.CheckResult{
		CheckID:     check1.ID,
		ContainerID: container1ID,
		Status:      healthdomain.HealthStatusUnhealthy,
		Output:      "Connection refused: endpoint not responding",
		Timestamp:   time.Now(),
		Duration:    5 * time.Second,
		Attempt:     3,
	}
	if err := checkStore.SaveResult(ctx, failedResult); err != nil {
		p.Error(fmt.Sprintf("Failed to save failed result: %v", err))
		return 1
	}
	p.Success("Health check failure simulated for container 1")

	// Test 10: Get container health status (should show unhealthy)
	p.Step("10. Getting container health status")
	containerHealth, err := healthUseCase.GetContainerHealth(ctx, container1ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get container health: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("Container 1 health: Status=%s, Checks=%d", containerHealth.Status, len(containerHealth.Checks)))
	p.Success("Container health status retrieved")

	// ======================================================================
	// PART 8: Agent Reports Unhealthy to Engine
	// ======================================================================

	// Test 11: Agent reports unhealthy state to Engine
	p.Step("11. Agent reporting unhealthy state to Engine")
	// Update actual state with unhealthy container
	actualState.Services["web-service"] = statedomain.ServiceActualState{
		Name: "web-service",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: string(container1ID),
				AgentID:     agentID,
				IP:          "10.0.0.10",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthUnhealthy, // Updated to unhealthy
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: string(container2ID),
				AgentID:     agentID,
				IP:          "10.0.0.11",
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
	p.Success("Agent reported unhealthy container to Engine")

	// ======================================================================
	// PART 9: Engine Detects Health Drift
	// ======================================================================

	// Test 12: Engine detects health drift
	p.Step("12. Engine detecting health drift")
	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	var foundUnhealthyDrift bool
	for _, d := range drift.Drifts {
		if d.Type == statedomain.DriftUnhealthy {
			foundUnhealthyDrift = true
			p.Info(fmt.Sprintf("Drift detected: %s - %s", d.Type, d.Details))
		}
	}
	if !foundUnhealthyDrift {
		p.Error("Expected unhealthy drift not detected")
		return 1
	}
	p.Success(fmt.Sprintf("Health drift detected: %d total drifts", len(drift.Drifts)))

	// Test 13: Generate drift report
	p.Step("13. Generating drift report")
	report, err := stateUseCase.GetDriftReport(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get drift report: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("Drift report: Total=%d, WithDrift=%d, Critical=%d", report.TotalDeployments, report.DeploymentsWithDrift, report.CriticalDrifts))
	p.Success("Drift report generated")

	// ======================================================================
	// PART 10: Simulate Container Restart (Recovery)
	// ======================================================================

	// Test 14: Simulate container restart (update actual state to healthy)
	p.Step("14. Simulating container restart")
	actualState.Services["web-service"] = statedomain.ServiceActualState{
		Name: "web-service",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: string(container1ID),
				AgentID:     agentID,
				IP:          "10.0.0.10",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthHealthy, // Restored to healthy
				StartedAt:   time.Now(),                // New start time
			},
			{
				ContainerID: string(container2ID),
				AgentID:     agentID,
				IP:          "10.0.0.11",
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
	p.Success("Container restarted and health restored")

	// Test 15: Verify drift resolved
	p.Step("15. Verifying drift resolved after restart")
	drift, err = stateUseCase.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}
	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Unexpected drift still exists: %d drifts", len(drift.Drifts)))
		for _, d := range drift.Drifts {
			p.Info(fmt.Sprintf("  - %s: %s", d.Type, d.Details))
		}
		return 1
	}
	p.Success("Drift resolved - system back to desired state")

	// ======================================================================
	// PART 11: Agent Health Status
	// ======================================================================

	// Test 16: Get agent health status
	p.Step("16. Getting agent health status")
	agentHealth, err := healthUseCase.GetAgentHealth(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get agent health: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("Agent health - Status: %s, Uptime: %v", agentHealth.Status, agentHealth.Uptime))
	p.Success("Agent health status retrieved")

	// Test 17: Unregister health check
	p.Step("17. Unregistering health check")
	if err := healthUseCase.UnregisterCheck(ctx, container1ID, check1.ID); err != nil {
		p.Error(fmt.Sprintf("Failed to unregister check: %v", err))
		return 1
	}
	p.Success("Health check unregistered")

	// Test 18: List all container health statuses
	p.Step("18. Listing all container health statuses")
	healthList, err := healthUseCase.ListContainerHealths(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list container healths: %v", err))
		return 1
	}
	p.Info(fmt.Sprintf("Total containers being monitored: %d", len(healthList)))
	p.Success("Container health list retrieved")

	// ======================================================================
	// PART 12: Agent Heartbeat
	// ======================================================================

	// Test 19: Agent heartbeat with health info
	p.Step("19. Agent sending heartbeat with health info")
	heartbeat := registrydomain.HeartbeatStatus{
		Status: registrydomain.AgentStatusOnline,
		Resources: registrydomain.Resources{
			CPUCores:       4,
			MemoryMB:       8192,
			CPUAvailable:   2.6,
			MemoryFreeMB:   4500,
			ContainerCount: 2,
			MaxContainers:  10,
		},
		RunningTasks: []string{"container-health-001", "container-health-002"},
	}

	if err := registryUseCase.ProcessHeartbeat(ctx, registrydomain.AgentID(agentID), &heartbeat); err != nil {
		p.Error(fmt.Sprintf("Failed to process heartbeat: %v", err))
		return 1
	}
	p.Success("Agent heartbeat processed successfully")

	// ======================================================================
	// PART 13: Multiple Failure Scenario
	// ======================================================================

	// Test 20: Multiple failure scenario - both containers unhealthy
	p.Step("20. Testing multiple container failure scenario")
	actualState.Services["web-service"] = statedomain.ServiceActualState{
		Name: "web-service",
		Instances: []statedomain.InstanceState{
			{
				ContainerID: string(container1ID),
				AgentID:     agentID,
				IP:          "10.0.0.10",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthUnhealthy,
				StartedAt:   time.Now().Add(-5 * time.Minute),
			},
			{
				ContainerID: string(container2ID),
				AgentID:     agentID,
				IP:          "10.0.0.11",
				Status:      statedomain.ContainerRunning,
				Health:      statedomain.HealthUnhealthy,
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
	p.Info(fmt.Sprintf("Multiple failures: %d drifts detected", len(drift.Drifts)))
	p.Info(fmt.Sprintf("Drift severity: %s", drift.Severity))
	p.Success("Multiple failure scenario handled correctly")

	// ======================================================================
	// PART 14: Cleanup
	// ======================================================================

	// Test 21: Cleanup - deregister agent
	p.Step("21. Cleaning up - deregistering agent")
	if err := registryUseCase.DeregisterAgent(ctx, registrydomain.AgentID(agentID)); err != nil {
		p.Error(fmt.Sprintf("Failed to deregister agent: %v", err))
		return 1
	}
	p.Success("Agent deregistered")

	// Test 22: Cleanup - delete desired state
	p.Step("22. Cleaning up - deleting deployment state")
	if err := stateUseCase.DeleteDesiredState(ctx, deploymentID); err != nil {
		p.Error(fmt.Sprintf("Failed to delete desired state: %v", err))
		return 1
	}
	p.Success("Deployment state deleted")

	p.Result(true, "All health monitoring integration tests passed!")
	return 0
}
