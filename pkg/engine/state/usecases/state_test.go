package usecases

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/engine/state/adapters"
	"github.com/fertile-org/banyan/pkg/engine/state/domain"
)

func TestStateUseCase_SetAndGetDesiredState(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	state := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {
				Name:     "api",
				Image:    "api:v1",
				Replicas: 3,
			},
		},
	}

	err := uc.SetDesiredState(ctx, state)
	if err != nil {
		t.Fatalf("SetDesiredState failed: %v", err)
	}

	got, err := uc.GetDesiredState(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("GetDesiredState failed: %v", err)
	}

	if got.DeploymentID != "deploy-1" {
		t.Errorf("Expected deployment ID 'deploy-1', got '%s'", got.DeploymentID)
	}
	if len(got.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(got.Services))
	}
	if got.Services["api"].Replicas != 3 {
		t.Errorf("Expected 3 replicas, got %d", got.Services["api"].Replicas)
	}
}

func TestStateUseCase_UpdateAndGetActualState(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	state := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "c1", AgentID: "agent-1", Status: domain.ContainerRunning},
					{ContainerID: "c2", AgentID: "agent-2", Status: domain.ContainerRunning},
				},
			},
		},
	}

	err := uc.UpdateActualState(ctx, state)
	if err != nil {
		t.Fatalf("UpdateActualState failed: %v", err)
	}

	got, err := uc.GetActualState(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("GetActualState failed: %v", err)
	}

	if got.DeploymentID != "deploy-1" {
		t.Errorf("Expected deployment ID 'deploy-1', got '%s'", got.DeploymentID)
	}
	if len(got.Services["api"].Instances) != 2 {
		t.Errorf("Expected 2 instances, got %d", len(got.Services["api"].Instances))
	}
}

func TestStateUseCase_DetectDrift_NoMismatch(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Set desired state: 2 replicas
	desired := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Image: "api:v1", Replicas: 2},
		},
	}
	_ = uc.SetDesiredState(ctx, desired)

	// Set actual state: 2 running instances
	actual := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "c1", Status: domain.ContainerRunning, Health: domain.HealthHealthy},
					{ContainerID: "c2", Status: domain.ContainerRunning, Health: domain.HealthHealthy},
				},
			},
		},
	}
	_ = uc.UpdateActualState(ctx, actual)

	drift, err := uc.DetectDrift(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}

	if len(drift.Drifts) != 0 {
		t.Errorf("Expected no drift, got %d drifts", len(drift.Drifts))
	}
}

func TestStateUseCase_DetectDrift_MissingService(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Set desired state: api service
	desired := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Image: "api:v1", Replicas: 1},
		},
	}
	_ = uc.SetDesiredState(ctx, desired)

	// Set actual state: no services
	actual := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceActualState{},
	}
	_ = uc.UpdateActualState(ctx, actual)

	drift, err := uc.DetectDrift(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}

	if len(drift.Drifts) != 1 {
		t.Fatalf("Expected 1 drift, got %d", len(drift.Drifts))
	}
	if drift.Drifts[0].Type != domain.DriftMissing {
		t.Errorf("Expected DriftMissing, got %s", drift.Drifts[0].Type)
	}
	if drift.Severity != domain.SeverityCritical {
		t.Errorf("Expected critical severity, got %s", drift.Severity)
	}
}

func TestStateUseCase_DetectDrift_ReplicaMismatch(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Set desired state: 3 replicas
	desired := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Image: "api:v1", Replicas: 3},
		},
	}
	_ = uc.SetDesiredState(ctx, desired)

	// Set actual state: 1 instance
	actual := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "c1", Status: domain.ContainerRunning, Health: domain.HealthHealthy},
				},
			},
		},
	}
	_ = uc.UpdateActualState(ctx, actual)

	drift, err := uc.DetectDrift(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}

	if len(drift.Drifts) != 1 {
		t.Fatalf("Expected 1 drift, got %d", len(drift.Drifts))
	}
	if drift.Drifts[0].Type != domain.DriftReplicas {
		t.Errorf("Expected DriftReplicas, got %s", drift.Drifts[0].Type)
	}
}

func TestStateUseCase_DetectDrift_UnhealthyInstance(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Set desired state: 2 replicas
	desired := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Image: "api:v1", Replicas: 2},
		},
	}
	_ = uc.SetDesiredState(ctx, desired)

	// Set actual state: 2 instances, 1 unhealthy
	actual := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"api": {
				Name: "api",
				Instances: []domain.InstanceState{
					{ContainerID: "c1", AgentID: "agent-1", Status: domain.ContainerRunning, Health: domain.HealthHealthy},
					{ContainerID: "c2", AgentID: "agent-2", Status: domain.ContainerRunning, Health: domain.HealthUnhealthy},
				},
			},
		},
	}
	_ = uc.UpdateActualState(ctx, actual)

	drift, err := uc.DetectDrift(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}

	if len(drift.Drifts) != 1 {
		t.Fatalf("Expected 1 drift, got %d", len(drift.Drifts))
	}
	if drift.Drifts[0].Type != domain.DriftUnhealthy {
		t.Errorf("Expected DriftUnhealthy, got %s", drift.Drifts[0].Type)
	}
	if drift.Severity != domain.SeverityHigh {
		t.Errorf("Expected high severity, got %s", drift.Severity)
	}
}

func TestStateUseCase_DetectDrift_ExtraService(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Set desired state: no services
	desired := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceDesiredState{},
	}
	_ = uc.SetDesiredState(ctx, desired)

	// Set actual state: unexpected service running
	actual := &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"unknown-svc": {
				Name: "unknown-svc",
				Instances: []domain.InstanceState{
					{ContainerID: "c1", Status: domain.ContainerRunning},
				},
			},
		},
	}
	_ = uc.UpdateActualState(ctx, actual)

	drift, err := uc.DetectDrift(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}

	if len(drift.Drifts) != 1 {
		t.Fatalf("Expected 1 drift, got %d", len(drift.Drifts))
	}
	if drift.Drifts[0].Type != domain.DriftExtra {
		t.Errorf("Expected DriftExtra, got %s", drift.Drifts[0].Type)
	}
}

func TestStateUseCase_GetDriftReport(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	// Create 2 deployments
	_ = uc.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Replicas: 1},
		},
	})
	_ = uc.UpdateActualState(ctx, &domain.ActualState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceActualState{
			"api": {Instances: []domain.InstanceState{{ContainerID: "c1", Health: domain.HealthHealthy}}},
		},
	})

	_ = uc.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-2",
		Services: map[string]domain.ServiceDesiredState{
			"web": {Name: "web", Replicas: 2},
		},
	})
	_ = uc.UpdateActualState(ctx, &domain.ActualState{
		DeploymentID: "deploy-2",
		Services: map[string]domain.ServiceActualState{
			"web": {Instances: []domain.InstanceState{{ContainerID: "c1", Health: domain.HealthHealthy}}},
		},
	})

	report, err := uc.GetDriftReport(ctx)
	if err != nil {
		t.Fatalf("GetDriftReport failed: %v", err)
	}

	if report.TotalDeployments != 2 {
		t.Errorf("Expected 2 total deployments, got %d", report.TotalDeployments)
	}
	if report.DeploymentsWithDrift != 1 {
		t.Errorf("Expected 1 deployment with drift, got %d", report.DeploymentsWithDrift)
	}
}

func TestStateUseCase_DeleteDesiredState(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	uc := NewStateUseCase(repo)
	ctx := context.Background()

	state := &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceDesiredState{},
	}
	_ = uc.SetDesiredState(ctx, state)

	err := uc.DeleteDesiredState(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("DeleteDesiredState failed: %v", err)
	}

	_, err = uc.GetDesiredState(ctx, "deploy-1")
	if err == nil {
		t.Error("Expected error when getting deleted state")
	}
}
