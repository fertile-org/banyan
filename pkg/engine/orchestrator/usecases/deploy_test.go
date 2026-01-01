package usecases

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
)

const testBanyanYAML = `
services:
  api:
    image: myapp:latest
    replicas: 2
    ports:
      - "8080:80"
    environment:
      DB_HOST: db
    depends_on:
      - db
  db:
    image: postgres:15
    replicas: 1
`

func TestDeployUseCase_CreateDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}

	deployment, err := uc.CreateDeployment(ctx, req)
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}

	if deployment.ID == "" {
		t.Error("Expected deployment ID to be set")
	}
	if deployment.Name != "test-deployment" {
		t.Errorf("Expected name 'test-deployment', got '%s'", deployment.Name)
	}
	if len(deployment.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(deployment.Services))
	}
	if deployment.State != domain.StateCreated {
		t.Errorf("Expected state Created, got %s", deployment.State)
	}
}

func TestDeployUseCase_GetDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	// Create deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	created, _ := uc.CreateDeployment(ctx, req)

	// Get deployment
	deployment, err := uc.GetDeployment(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetDeployment failed: %v", err)
	}

	if deployment.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, deployment.ID)
	}
}

func TestDeployUseCase_ExecuteDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	dispatcher := adapters.NewMemoryAgentDispatcher()
	scheduler := adapters.NewMemoryScheduler()
	plugins := adapters.NewMemoryPluginExecutor()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, dispatcher, scheduler, plugins, parser)
	ctx := context.Background()

	// Create deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)

	// Execute deployment
	err := uc.ExecuteDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ExecuteDeployment failed: %v", err)
	}

	// Verify final state
	updated, _ := uc.GetDeployment(ctx, deployment.ID)
	if updated.State != domain.StateActive {
		t.Errorf("Expected state Active, got %s", updated.State)
	}
	if updated.Plan == nil {
		t.Error("Expected execution plan to be set")
	}

	// Verify hooks were executed
	hooks := plugins.GetExecutedHooks()
	if len(hooks) != 4 {
		t.Errorf("Expected 4 hooks, got %d", len(hooks))
	}

	// Verify tasks were dispatched
	tasks := dispatcher.GetDispatchedTasks()
	if len(tasks) < 1 {
		t.Error("Expected at least 1 task to be dispatched")
	}
}

func TestDeployUseCase_ExecuteDeployment_WithDependencies(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	// Create deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)

	// Execute deployment
	err := uc.ExecuteDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("ExecuteDeployment failed: %v", err)
	}

	// Verify plan has correct order (db before api)
	updated, _ := uc.GetDeployment(ctx, deployment.ID)
	if len(updated.Plan.Phases) < 2 {
		t.Fatalf("Expected at least 2 phases, got %d", len(updated.Plan.Phases))
	}

	// First phase should contain db
	foundDb := false
	for _, svc := range updated.Plan.Phases[0].Services {
		if svc == "db" {
			foundDb = true
			break
		}
	}
	if !foundDb {
		t.Error("Expected 'db' to be in first phase")
	}
}

func TestDeployUseCase_GetDeploymentPlan(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}

	plan, err := uc.GetDeploymentPlan(ctx, req)
	if err != nil {
		t.Fatalf("GetDeploymentPlan failed: %v", err)
	}

	if plan == nil {
		t.Fatal("Expected plan to be set")
	}
	if len(plan.Phases) < 2 {
		t.Errorf("Expected at least 2 phases, got %d", len(plan.Phases))
	}
}

func TestDeployUseCase_CancelDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	// Create deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)

	// Manually set state to deploying
	deployment.State = domain.StateDeploying
	_ = repo.Update(ctx, deployment)

	// Cancel deployment
	err := uc.CancelDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("CancelDeployment failed: %v", err)
	}

	// Verify state
	updated, _ := uc.GetDeployment(ctx, deployment.ID)
	if updated.State != domain.StateFailed {
		t.Errorf("Expected state Failed, got %s", updated.State)
	}
}

func TestDeployUseCase_DeleteDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	// Create deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)

	// Delete deployment
	err := uc.DeleteDeployment(ctx, deployment.ID)
	if err != nil {
		t.Fatalf("DeleteDeployment failed: %v", err)
	}

	// Verify deleted
	_, err = uc.GetDeployment(ctx, deployment.ID)
	if err == nil {
		t.Error("Expected error when getting deleted deployment")
	}
}

func TestDeployUseCase_ListDeployments(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	// Create multiple deployments
	req1 := inbound.CreateDeploymentRequest{Name: "deploy-1", BanyanFile: testBanyanYAML}
	req2 := inbound.CreateDeploymentRequest{Name: "deploy-2", BanyanFile: testBanyanYAML}
	_, _ = uc.CreateDeployment(ctx, req1)
	_, _ = uc.CreateDeployment(ctx, req2)

	// List deployments
	deployments, err := uc.ListDeployments(ctx, inbound.DeploymentFilter{})
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}

	if len(deployments) != 2 {
		t.Errorf("Expected 2 deployments, got %d", len(deployments))
	}
}

func TestDeployUseCase_RollbackDeployment(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	dispatcher := adapters.NewMemoryAgentDispatcher()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, dispatcher, scheduler, nil, parser)
	ctx := context.Background()

	// Create and execute deployment
	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)
	_ = uc.ExecuteDeployment(ctx, deployment.ID)

	// Rollback
	err := uc.RollbackDeployment(ctx, deployment.ID, inbound.RollbackImmediate)
	if err != nil {
		t.Fatalf("RollbackDeployment failed: %v", err)
	}

	// Verify state
	updated, _ := uc.GetDeployment(ctx, deployment.ID)
	if updated.State != domain.StateDestroyed {
		t.Errorf("Expected state Destroyed, got %s", updated.State)
	}
}

func TestDeployUseCase_CircularDependency(t *testing.T) {
	repo := adapters.NewMemoryDeploymentRepository()
	scheduler := adapters.NewMemoryScheduler()
	parser := adapters.NewMemoryBanyanParser()

	uc := NewDeployUseCase(repo, nil, scheduler, nil, parser)
	ctx := context.Background()

	circularYAML := `
services:
  a:
    image: a:latest
    depends_on:
      - b
  b:
    image: b:latest
    depends_on:
      - a
`

	req := inbound.CreateDeploymentRequest{
		Name:       "circular-deployment",
		BanyanFile: circularYAML,
	}
	deployment, _ := uc.CreateDeployment(ctx, req)

	// Execute should fail due to circular dependency
	err := uc.ExecuteDeployment(ctx, deployment.ID)
	if err == nil {
		t.Error("Expected error for circular dependency")
	}
}
