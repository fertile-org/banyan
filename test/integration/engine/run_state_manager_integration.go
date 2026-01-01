//go:build ignore

// Integration test for Engine State Manager
// Tests the full state management workflow: Set Desired → Set Actual → Detect Drift → Reconcile
//
// Run with: go run ./test/integration/engine/run_state_manager_integration.go

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/adapters"
	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	"github.com/fertile-org/banyan/pkg/engine/state/usecases"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All State Manager integration tests passed!")
	} else {
		p.Result(false, "State Manager integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("State Manager Integration Test")

	// Create state manager with all adapters
	repo := adapters.NewMemoryStateRepository()
	agentQuerier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	stateUC := usecases.NewStateUseCase(repo)
	reconcilerUC := usecases.NewReconcilerUseCase(stateUC, repo, agentQuerier, dispatcher)

	deploymentID := "test-deployment-1"

	// Test 1: Set desired state
	p.Step("Setting desired state for deployment")

	desiredState := &domain.DesiredState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceDesiredState{
			"api": {
				Name:     "api",
				Image:    "myapp/api:v1.0",
				Replicas: 3,
				Ports: []domain.PortMapping{
					{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"},
				},
				Environment: map[string]string{
					"DB_HOST": "db.local",
				},
			},
			"db": {
				Name:     "db",
				Image:    "postgres:15",
				Replicas: 1,
				Ports: []domain.PortMapping{
					{HostPort: 5432, ContainerPort: 5432, Protocol: "tcp"},
				},
			},
		},
		UpdatedAt: time.Now(),
	}

	err := stateUC.SetDesiredState(ctx, desiredState)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to set desired state: %v", err))
		return 1
	}

	p.Success("Desired state set successfully")

	// Test 2: Get desired state
	p.Step("Retrieving desired state")

	retrieved, err := stateUC.GetDesiredState(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get desired state: %v", err))
		return 1
	}

	if len(retrieved.Services) != 2 {
		p.Error(fmt.Sprintf("Expected 2 services in desired state, got %d", len(retrieved.Services)))
		return 1
	}

	p.Success(fmt.Sprintf("Retrieved desired state with %d services", len(retrieved.Services)))

	// Test 3: Set actual state (matching desired - no drift)
	p.Step("Setting actual state matching desired (no drift)")

	actualState := &domain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "api-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.1"},
					{ContainerID: "api-2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.2"},
					{ContainerID: "api-3", AgentID: "agent-3", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.3"},
				},
			},
			"db": {
				Name: "db",
				Instances: []domain.InstanceState{
					{ContainerID: "db-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.2.1"},
				},
			},
		},
		CollectedAt: time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualState)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err := stateUC.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	if len(drift.Drifts) != 0 {
		p.Error(fmt.Sprintf("Expected no drift, got %d drifts", len(drift.Drifts)))
		return 1
	}

	p.Success("No drift detected when actual matches desired")

	// Test 4: Simulate drift - replica mismatch
	p.Step("Simulating replica mismatch drift")

	actualMismatch := &domain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "api-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.1"},
					{ContainerID: "api-2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.2"},
					// Missing third replica
				},
			},
			"db": {
				Name: "db",
				Instances: []domain.InstanceState{
					{ContainerID: "db-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.2.1"},
				},
			},
		},
		CollectedAt: time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualMismatch)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUC.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	if len(drift.Drifts) == 0 {
		p.Error("Expected drift to be detected for replica mismatch")
		return 1
	}

	p.Success(fmt.Sprintf("Drift detected: %d issues found", len(drift.Drifts)))
	for _, d := range drift.Drifts {
		p.Info(fmt.Sprintf("  - %s: type=%s, details=%s", d.ServiceName, d.Type, d.Details))
	}

	// Test 5: Simulate unhealthy instance drift
	p.Step("Simulating unhealthy instance drift")

	actualUnhealthy := &domain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "api-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.1"},
					{ContainerID: "api-2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthUnhealthy, IP: "10.0.1.2"}, // Unhealthy
					{ContainerID: "api-3", AgentID: "agent-3", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.3"},
				},
			},
			"db": {
				Name: "db",
				Instances: []domain.InstanceState{
					{ContainerID: "db-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.2.1"},
				},
			},
		},
		CollectedAt: time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualUnhealthy)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUC.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	hasUnhealthyDrift := false
	for _, d := range drift.Drifts {
		if d.Type == domain.DriftUnhealthy {
			hasUnhealthyDrift = true
			break
		}
	}

	if !hasUnhealthyDrift {
		p.Error("Expected unhealthy drift to be detected")
		return 1
	}

	p.Success("Unhealthy instance drift detected")

	// Test 6: Simulate missing service drift
	p.Step("Simulating missing service drift")

	actualMissing := &domain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "api-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.1"},
					{ContainerID: "api-2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.2"},
					{ContainerID: "api-3", AgentID: "agent-3", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.3"},
				},
			},
			// db service is missing
		},
		CollectedAt: time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualMissing)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUC.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	hasMissingDrift := false
	for _, d := range drift.Drifts {
		if d.Type == domain.DriftMissing && d.ServiceName == "db" {
			hasMissingDrift = true
			break
		}
	}

	if !hasMissingDrift {
		p.Error("Expected missing service drift to be detected")
		return 1
	}

	p.Success("Missing service drift detected")

	// Test 7: Trigger reconciliation
	p.Step("Triggering reconciliation")

	err = reconcilerUC.TriggerReconcile(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to trigger reconcile: %v", err))
		return 1
	}

	p.Success("Reconciliation triggered")

	// Test 8: Verify actions were dispatched
	p.Step("Verifying reconciliation actions were dispatched")

	actions := dispatcher.GetDispatchedActions()
	p.Info(fmt.Sprintf("  Dispatched actions: %d", len(actions)))

	for _, action := range actions {
		p.Info(fmt.Sprintf("    - %s: %s", action.Type, action.ServiceName))
	}

	if len(actions) == 0 {
		p.Error("Expected reconciliation actions to be dispatched")
		return 1
	}

	p.Success("Reconciliation actions dispatched")

	// Test 9: Get drift report
	p.Step("Getting drift report")

	report, err := stateUC.GetDriftReport(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get drift report: %v", err))
		return 1
	}

	p.Info(fmt.Sprintf("  Total deployments: %d", report.TotalDeployments))
	p.Info(fmt.Sprintf("  Deployments with drift: %d", report.DeploymentsWithDrift))
	p.Info(fmt.Sprintf("  Critical drifts: %d", report.CriticalDrifts))

	p.Success("Drift report generated")

	// Test 10: Test extra service detection
	p.Step("Simulating extra service drift")

	actualExtra := &domain.ActualState{
		DeploymentID: deploymentID,
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "api-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.1"},
					{ContainerID: "api-2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.2"},
					{ContainerID: "api-3", AgentID: "agent-3", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.1.3"},
				},
			},
			"db": {
				Name: "db",
				Instances: []domain.InstanceState{
					{ContainerID: "db-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.2.1"},
				},
			},
			"rogue-service": { // Extra service not in desired state
				Name: "rogue-service",
				Instances: []domain.InstanceState{
					{ContainerID: "rogue-1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy, IP: "10.0.3.1"},
				},
			},
		},
		CollectedAt: time.Now(),
	}

	err = stateUC.UpdateActualState(ctx, actualExtra)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to update actual state: %v", err))
		return 1
	}

	drift, err = stateUC.DetectDrift(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to detect drift: %v", err))
		return 1
	}

	hasExtraDrift := false
	for _, d := range drift.Drifts {
		if d.Type == domain.DriftExtra && d.ServiceName == "rogue-service" {
			hasExtraDrift = true
			break
		}
	}

	if !hasExtraDrift {
		p.Error("Expected extra service drift to be detected")
		return 1
	}

	p.Success("Extra service drift detected")

	// Test 11: Delete desired state
	p.Step("Deleting desired state")

	err = stateUC.DeleteDesiredState(ctx, deploymentID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to delete desired state: %v", err))
		return 1
	}

	_, err = stateUC.GetDesiredState(ctx, deploymentID)
	if err == nil {
		p.Error("Expected error when getting deleted state")
		return 1
	}

	p.Success("Desired state deleted successfully")

	// Test 12: Multiple deployments
	p.Step("Testing multiple deployments")

	for i := 1; i <= 3; i++ {
		depID := fmt.Sprintf("multi-deployment-%d", i)
		state := &domain.DesiredState{
			DeploymentID: depID,
			Services: map[string]domain.ServiceDesiredState{
				"svc": {
					Name:     "svc",
					Replicas: i,
					Image:    fmt.Sprintf("myapp:v%d", i),
				},
			},
			UpdatedAt: time.Now(),
		}
		if err := stateUC.SetDesiredState(ctx, state); err != nil {
			p.Error(fmt.Sprintf("Failed to set state for %s: %v", depID, err))
			return 1
		}
	}

	p.Success("Multiple deployments managed successfully")

	return 0
}
