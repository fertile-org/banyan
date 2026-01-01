//go:build ignore

// Integration test for Engine Orchestrator
// Tests the full deployment workflow: Create → Plan → Execute → Verify → Rollback
//
// Run with: go run ./test/integration/engine/run_orchestrator_integration.go

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/adapters"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/usecases"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

const testBanyanYAML = `
services:
  frontend:
    image: nginx:alpine
    replicas: 2
    ports:
      - "80:80"
    depends_on:
      - api
  api:
    image: myapp/api:latest
    replicas: 3
    ports:
      - "8080:8080"
    environment:
      DB_HOST: db
      CACHE_HOST: cache
    depends_on:
      - db
      - cache
  db:
    image: postgres:15
    replicas: 1
    ports:
      - "5432:5432"
    healthcheck:
      test: "pg_isready"
      interval: "10s"
      retries: 3
  cache:
    image: redis:7
    replicas: 1
    ports:
      - "6379:6379"
`

const circularBanyanYAML = `
services:
  a:
    image: a:latest
    depends_on:
      - b
  b:
    image: b:latest
    depends_on:
      - c
  c:
    image: c:latest
    depends_on:
      - a
`

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All Orchestrator integration tests passed!")
	} else {
		p.Result(false, "Orchestrator integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Orchestrator Integration Test")

	// Create orchestrator with all adapters
	repo := adapters.NewMemoryDeploymentRepository()
	dispatcher := adapters.NewMemoryAgentDispatcher()
	scheduler := adapters.NewMemoryScheduler()
	plugins := adapters.NewMemoryPluginExecutor()
	parser := adapters.NewMemoryBanyanParser()

	uc := usecases.NewDeployUseCase(repo, dispatcher, scheduler, plugins, parser)

	// Test 1: Create deployment
	p.Step("Creating deployment from banyan.yml")

	req := inbound.CreateDeploymentRequest{
		Name:       "test-deployment",
		BanyanFile: testBanyanYAML,
	}

	deployment, err := uc.CreateDeployment(ctx, req)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create deployment: %v", err))
		return 1
	}

	if deployment.ID == "" {
		p.Error("Deployment ID is empty")
		return 1
	}

	p.Success(fmt.Sprintf("Deployment created: ID=%s, Name=%s", deployment.ID, deployment.Name))

	// Test 2: Verify services parsed
	p.Step("Verifying services were parsed correctly")

	if len(deployment.Services) != 4 {
		p.Error(fmt.Sprintf("Expected 4 services, got %d", len(deployment.Services)))
		return 1
	}

	serviceNames := make(map[string]bool)
	for _, svc := range deployment.Services {
		serviceNames[svc.Name] = true
		p.Info(fmt.Sprintf("  Service: %s (image: %s, replicas: %d)", svc.Name, svc.Image, svc.Replicas))
	}

	expectedServices := []string{"frontend", "api", "db", "cache"}
	for _, name := range expectedServices {
		if !serviceNames[name] {
			p.Error(fmt.Sprintf("Missing service: %s", name))
			return 1
		}
	}

	p.Success("All 4 services parsed correctly")

	// Test 3: Get deployment plan (dry-run)
	p.Step("Getting deployment plan (dry-run)")

	plan, err := uc.GetDeploymentPlan(ctx, req)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get deployment plan: %v", err))
		return 1
	}

	if plan == nil {
		p.Error("Plan is nil")
		return 1
	}

	p.Success(fmt.Sprintf("Plan generated: %d phases", len(plan.Phases)))

	for i, phase := range plan.Phases {
		p.Info(fmt.Sprintf("  Phase %d: %v", i+1, phase.Services))
	}

	// Test 4: Verify dependency ordering
	p.Step("Verifying dependency ordering in plan")

	// db and cache should be before api, api should be before frontend
	phaseMap := make(map[string]int)
	for i, phase := range plan.Phases {
		for _, svc := range phase.Services {
			phaseMap[svc] = i
		}
	}

	// db must be before api
	if phaseMap["db"] >= phaseMap["api"] {
		p.Error("db should be deployed before api")
		return 1
	}

	// cache must be before api
	if phaseMap["cache"] >= phaseMap["api"] {
		p.Error("cache should be deployed before api")
		return 1
	}

	// api must be before frontend
	if phaseMap["api"] >= phaseMap["frontend"] {
		p.Error("api should be deployed before frontend")
		return 1
	}

	p.Success("Dependency ordering is correct: db,cache → api → frontend")

	// Test 5: Execute deployment
	p.Step("Executing deployment")

	startTime := time.Now()
	err = uc.ExecuteDeployment(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to execute deployment: %v", err))
		return 1
	}
	duration := time.Since(startTime)

	p.Success(fmt.Sprintf("Deployment executed in %v", duration))

	// Test 6: Verify deployment state
	p.Step("Verifying deployment state after execution")

	updated, err := uc.GetDeployment(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to get deployment: %v", err))
		return 1
	}

	if updated.State != domain.StateActive {
		p.Error(fmt.Sprintf("Expected state Active, got %s", updated.State))
		return 1
	}

	p.Success(fmt.Sprintf("Deployment state: %s", updated.State))

	// Test 7: Verify hooks were executed
	p.Step("Verifying plugin hooks were executed")

	hooks := plugins.GetExecutedHooks()
	p.Info(fmt.Sprintf("  Executed hooks: %d", len(hooks)))

	for _, hook := range hooks {
		p.Info(fmt.Sprintf("    - %s", hook))
	}

	if len(hooks) < 4 {
		p.Error("Expected at least 4 hooks (pre-deploy, post-deploy for each service)")
		return 1
	}

	p.Success("Plugin hooks executed correctly")

	// Test 8: Verify tasks were dispatched
	p.Step("Verifying tasks were dispatched to agents")

	tasks := dispatcher.GetDispatchedTasks()
	p.Info(fmt.Sprintf("  Dispatched tasks: %d", len(tasks)))

	if len(tasks) < 1 {
		p.Error("Expected at least 1 task to be dispatched")
		return 1
	}

	p.Success("Tasks dispatched to agents")

	// Test 9: Test rollback
	p.Step("Testing rollback functionality")

	err = uc.RollbackDeployment(ctx, deployment.ID, inbound.RollbackImmediate)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to rollback deployment: %v", err))
		return 1
	}

	updated, _ = uc.GetDeployment(ctx, deployment.ID)
	if updated.State != domain.StateDestroyed {
		p.Error(fmt.Sprintf("Expected state Destroyed after rollback, got %s", updated.State))
		return 1
	}

	p.Success("Rollback completed successfully")

	// Test 10: List deployments
	p.Step("Testing list deployments")

	// Create another deployment
	req2 := inbound.CreateDeploymentRequest{
		Name:       "second-deployment",
		BanyanFile: testBanyanYAML,
	}
	_, _ = uc.CreateDeployment(ctx, req2)

	deployments, err := uc.ListDeployments(ctx, inbound.DeploymentFilter{})
	if err != nil {
		p.Error(fmt.Sprintf("Failed to list deployments: %v", err))
		return 1
	}

	if len(deployments) != 2 {
		p.Error(fmt.Sprintf("Expected 2 deployments, got %d", len(deployments)))
		return 1
	}

	p.Success(fmt.Sprintf("Listed %d deployments", len(deployments)))

	// Test 11: Circular dependency detection
	p.Step("Testing circular dependency detection")

	circularReq := inbound.CreateDeploymentRequest{
		Name:       "circular-deployment",
		BanyanFile: circularBanyanYAML,
	}

	circularDep, _ := uc.CreateDeployment(ctx, circularReq)
	err = uc.ExecuteDeployment(ctx, circularDep.ID)

	if err == nil {
		p.Error("Expected error for circular dependency, got none")
		return 1
	}

	p.Success(fmt.Sprintf("Circular dependency detected: %v", err))

	// Test 12: Delete deployment
	p.Step("Testing delete deployment")

	err = uc.DeleteDeployment(ctx, deployment.ID)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to delete deployment: %v", err))
		return 1
	}

	_, err = uc.GetDeployment(ctx, deployment.ID)
	if err == nil {
		p.Error("Expected error when getting deleted deployment")
		return 1
	}

	p.Success("Deployment deleted successfully")

	return 0
}
