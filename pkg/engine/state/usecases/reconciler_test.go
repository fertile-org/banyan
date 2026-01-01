package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/adapters"
	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

func TestReconcilerUseCase_StartStop(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)

	ctx := context.Background()

	err := reconciler.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Starting again should fail
	err = reconciler.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running reconciler")
	}

	err = reconciler.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stopping again should fail
	err = reconciler.Stop(ctx)
	if err == nil {
		t.Error("Expected error when stopping already stopped reconciler")
	}
}

func TestReconcilerUseCase_SetGetInterval(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)

	// Default interval
	if reconciler.GetReconcileInterval() != 30*time.Second {
		t.Errorf("Expected default interval 30s, got %v", reconciler.GetReconcileInterval())
	}

	// Set custom interval
	reconciler.SetReconcileInterval(60 * time.Second)
	if reconciler.GetReconcileInterval() != 60*time.Second {
		t.Errorf("Expected interval 60s, got %v", reconciler.GetReconcileInterval())
	}
}

func TestReconcilerUseCase_TriggerReconcile_NoDrift(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Set desired state
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Replicas: 1, AgentIDs: []string{"agent-1"}},
		},
	})

	// Set agent state matching desired
	querier.SetAgentState("agent-1", []outbound.ContainerInfo{
		{
			ID:     "c1",
			Name:   "api-1",
			Status: "running",
			Health: "healthy",
			Labels: map[string]string{
				"banyan.service":    "api",
				"banyan.deployment": "deploy-1",
			},
		},
	})

	err := reconciler.TriggerReconcile(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("TriggerReconcile failed: %v", err)
	}

	// No actions should be dispatched (no drift)
	actions := dispatcher.GetDispatchedActions()
	if len(actions) != 0 {
		t.Errorf("Expected no actions, got %d", len(actions))
	}
}

func TestReconcilerUseCase_TriggerReconcile_WithDrift(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Set desired state: 2 replicas
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Replicas: 2, AgentIDs: []string{"agent-1"}},
		},
	})

	// Set agent state: only 1 instance
	querier.SetAgentState("agent-1", []outbound.ContainerInfo{
		{
			ID:     "c1",
			Name:   "api-1",
			Status: "running",
			Health: "healthy",
			Labels: map[string]string{
				"banyan.service":    "api",
				"banyan.deployment": "deploy-1",
			},
		},
	})

	err := reconciler.TriggerReconcile(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("TriggerReconcile failed: %v", err)
	}

	// Scale action should be dispatched
	actions := dispatcher.GetDispatchedActions()
	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != domain.ActionScale {
		t.Errorf("Expected ActionScale, got %s", actions[0].Type)
	}
}

func TestReconcilerUseCase_TriggerReconcile_MissingService(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Set desired state with a service
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services: map[string]domain.ServiceDesiredState{
			"api": {Name: "api", Replicas: 1, AgentIDs: []string{"agent-1"}},
		},
	})

	// Set agent state with no containers
	querier.SetAgentState("agent-1", []outbound.ContainerInfo{})

	err := reconciler.TriggerReconcile(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("TriggerReconcile failed: %v", err)
	}

	// Deploy action should be dispatched
	actions := dispatcher.GetDispatchedActions()
	if len(actions) != 1 {
		t.Fatalf("Expected 1 action, got %d", len(actions))
	}
	if actions[0].Type != domain.ActionDeploy {
		t.Errorf("Expected ActionDeploy, got %s", actions[0].Type)
	}
}

func TestReconcilerUseCase_IsReconciling(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Set up state
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceDesiredState{},
	})

	// Initially not reconciling
	if reconciler.IsReconciling(ctx, "deploy-1") {
		t.Error("Expected not reconciling initially")
	}

	// After reconcile, should not be reconciling
	_ = reconciler.TriggerReconcile(ctx, "deploy-1")
	if reconciler.IsReconciling(ctx, "deploy-1") {
		t.Error("Expected not reconciling after completion")
	}
}

func TestReconcilerUseCase_GetLastReconcileTime(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Before reconcile, should error
	_, err := reconciler.GetLastReconcileTime(ctx, "deploy-1")
	if err == nil {
		t.Error("Expected error for unknown deployment")
	}

	// Set up state and reconcile
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceDesiredState{},
	})

	before := time.Now()
	_ = reconciler.TriggerReconcile(ctx, "deploy-1")
	after := time.Now()

	lastTime, err := reconciler.GetLastReconcileTime(ctx, "deploy-1")
	if err != nil {
		t.Fatalf("GetLastReconcileTime failed: %v", err)
	}

	if lastTime.Before(before) || lastTime.After(after) {
		t.Error("Last reconcile time out of expected range")
	}
}

func TestReconcilerUseCase_TriggerReconcileAll(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)
	querier := adapters.NewMemoryAgentQuerier()
	dispatcher := adapters.NewMemoryActionDispatcher()

	reconciler := NewReconcilerUseCase(stateUC, repo, querier, dispatcher)
	ctx := context.Background()

	// Set up multiple deployments
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-1",
		Services:     map[string]domain.ServiceDesiredState{},
	})
	_ = stateUC.SetDesiredState(ctx, &domain.DesiredState{
		DeploymentID: "deploy-2",
		Services:     map[string]domain.ServiceDesiredState{},
	})

	err := reconciler.TriggerReconcileAll(ctx)
	if err != nil {
		t.Fatalf("TriggerReconcileAll failed: %v", err)
	}

	// Give goroutines time to complete
	time.Sleep(100 * time.Millisecond)
}

func TestReconcilerUseCase_GenerateActions(t *testing.T) {
	repo := adapters.NewMemoryStateRepository()
	stateUC := NewStateUseCase(repo)

	reconciler := NewReconcilerUseCase(stateUC, repo, nil, nil)

	drift := &domain.StateDrift{
		DeploymentID: "deploy-1",
		Drifts: []domain.Drift{
			{Type: domain.DriftMissing, ServiceName: "svc1"},
			{Type: domain.DriftExtra, ServiceName: "svc2"},
			{Type: domain.DriftReplicas, ServiceName: "svc3"},
			{Type: domain.DriftUnhealthy, ServiceName: "svc4", AgentID: "agent-1"},
			{Type: domain.DriftWrongHost, ServiceName: "svc5", AgentID: "agent-2"},
		},
	}

	actions := reconciler.generateActions(drift)

	if len(actions) != 5 {
		t.Fatalf("Expected 5 actions, got %d", len(actions))
	}

	expectedTypes := []domain.ActionType{
		domain.ActionDeploy,
		domain.ActionStop,
		domain.ActionScale,
		domain.ActionRestart,
		domain.ActionMigrate,
	}

	for i, action := range actions {
		if action.Type != expectedTypes[i] {
			t.Errorf("Action %d: expected %s, got %s", i, expectedTypes[i], action.Type)
		}
	}
}
