package engine

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// These tests verify the full Agent → Container → Deployment reconciliation
// chain. They catch bugs that unit tests on individual reconcilers cannot:
// ordering issues, duplicate task creation, and cross-reconciler state passing.

func TestReconciliation_FullChain_AgentDiesContainersRescheduled(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()

	deps := &ReconcilerDeps{
		Store:  store,
		Events: NewEventBuffer(100),
		Log:    logging.New("test"),
	}

	agentRec := NewAgentReconciler(deps)
	containerRec := NewContainerReconciler(deps)
	deploymentRec := NewDeploymentReconciler(deps)

	now := time.Now()

	// agent-1 is stale (3min, past 2min standard grace for fresh agent)
	stale := &types.NodeRecord{
		Name: "agent-1", Status: "ready",
		LastSeen: now.Add(-3 * time.Minute), CreatedAt: now.Add(-5 * time.Minute),
	}
	saveNode(t, ctx, store, stale)
	// agent-2 is healthy with resources
	healthy := &types.NodeRecord{
		Name: "agent-2", Status: "ready",
		LastSeen: now, CreatedAt: now.Add(-time.Hour),
		MemoryTotalBytes: 4 * 1024 * 1024 * 1024, MemoryUsedBytes: 1024 * 1024 * 1024,
	}
	saveNode(t, ctx, store, healthy)

	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "webapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx:latest", Replicas: 2, Restart: "always",
				Ports: []string{"8080:80"}, MemoryLimit: "256m"},
		},
		CreatedAt: now.Add(-time.Hour),
	}
	saveDeployment(t, ctx, store, dep)

	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-web-0", DeploymentID: "dep-1", DeploymentName: "webapp",
		ServiceName: "web", ReplicaIndex: 0,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "webapp-web-0", ContainerStatus: types.StatusRunning, CreatedAt: now,
	})
	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-web-1", DeploymentID: "dep-1", DeploymentName: "webapp",
		ServiceName: "web", ReplicaIndex: 1,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "webapp-web-1", ContainerStatus: types.StatusRunning, CreatedAt: now,
	})

	origIDs := map[string]bool{"dep-1-web-0": true, "dep-1-web-1": true}

	// --- Run the full chain: Agent → Container → Deployment ---

	if err := agentRec.Reconcile(ctx); err != nil {
		t.Fatalf("AgentReconciler: %v", err)
	}

	// Original tasks should be marked exited with agent_stale
	var task0 types.TaskRecord
	store.Get(ctx, types.KeyTasks+"agent-1/dep-1-web-0", &task0)
	if task0.ContainerStatus != "exited" || task0.StopReason != "agent_stale" {
		t.Errorf("task0: status=%q reason=%q, want exited/agent_stale", task0.ContainerStatus, task0.StopReason)
	}

	// Rescheduled tasks should exist (cleanup on agent-1, create on agent-2)
	newCount := countNewTasks(t, ctx, store, origIDs)
	if newCount < 2 {
		t.Errorf("expected at least 2 new tasks (rescheduled), got %d", newCount)
	}

	// ContainerReconciler should NOT create more tasks on stale agent-1
	if err := containerRec.Reconcile(ctx); err != nil {
		t.Fatalf("ContainerReconciler: %v", err)
	}

	// DeploymentReconciler computes health
	if err := deploymentRec.Reconcile(ctx); err != nil {
		t.Fatalf("DeploymentReconciler: %v", err)
	}

	var depResult types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &depResult)
	if depResult.HealthStatus != "recovering" {
		t.Errorf("HealthStatus = %q, want recovering", depResult.HealthStatus)
	}
	if depResult.Status != types.StatusRunning {
		t.Errorf("Status = %q, want running (has pending tasks)", depResult.Status)
	}
}

func TestReconciliation_FullChain_ContainerCrashRestarted(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()

	deps := &ReconcilerDeps{
		Store:  store,
		Events: NewEventBuffer(100),
		Log:    logging.New("test"),
	}

	agentRec := NewAgentReconciler(deps)
	containerRec := NewContainerReconciler(deps)
	deploymentRec := NewDeploymentReconciler(deps)

	now := time.Now()
	saveNode(t, ctx, store, makeNode("agent-1", now))

	dep := makeRunningDeployment("dep-1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx", Replicas: 2, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-web-0", DeploymentID: "dep-1", ServiceName: "web", ReplicaIndex: 0,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0", ContainerStatus: types.StatusRunning, CreatedAt: now,
	})
	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-web-1", DeploymentID: "dep-1", ServiceName: "web", ReplicaIndex: 1,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-1", ContainerStatus: "exited", ExitCode: 137, CreatedAt: now,
	})

	origIDs := map[string]bool{"dep-1-web-0": true, "dep-1-web-1": true}

	agentRec.Reconcile(ctx)     // no stale agents, no-op
	containerRec.Reconcile(ctx) // finds crashed container, creates restart
	deploymentRec.Reconcile(ctx)

	newCount := countNewTasks(t, ctx, store, origIDs)
	if newCount < 1 {
		t.Errorf("expected at least 1 new task (restart), got %d", newCount)
	}

	var depResult types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &depResult)
	if depResult.HealthStatus != "recovering" {
		t.Errorf("HealthStatus = %q, want recovering", depResult.HealthStatus)
	}
}

func TestReconciliation_FullChain_RestartNo_AllDead_Stopped(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()

	deps := &ReconcilerDeps{
		Store:  store,
		Events: NewEventBuffer(100),
		Log:    logging.New("test"),
	}

	agentRec := NewAgentReconciler(deps)
	containerRec := NewContainerReconciler(deps)
	deploymentRec := NewDeploymentReconciler(deps)

	now := time.Now()
	saveNode(t, ctx, store, makeNode("agent-1", now))

	dep := makeRunningDeployment("dep-1", "batch", map[string]types.ServiceRecord{
		"worker": {Image: "worker:latest", Replicas: 2, Restart: "no"},
	})
	saveDeployment(t, ctx, store, dep)

	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-worker-0", DeploymentID: "dep-1", ServiceName: "worker", ReplicaIndex: 0,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "batch-worker-0", ContainerStatus: "exited", ExitCode: 0, CreatedAt: now,
	})
	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-worker-1", DeploymentID: "dep-1", ServiceName: "worker", ReplicaIndex: 1,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "batch-worker-1", ContainerStatus: "exited", ExitCode: 0, CreatedAt: now,
	})

	origIDs := map[string]bool{"dep-1-worker-0": true, "dep-1-worker-1": true}

	agentRec.Reconcile(ctx)
	containerRec.Reconcile(ctx) // restart=no → no restart
	deploymentRec.Reconcile(ctx)

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("restart=no should not create tasks, got %d", n)
	}

	var depResult types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &depResult)
	if depResult.Status != types.StatusStopped {
		t.Errorf("Status = %q, want stopped", depResult.Status)
	}
	if depResult.HealthStatus != "stopped" {
		t.Errorf("HealthStatus = %q, want stopped", depResult.HealthStatus)
	}
}

func TestReconciliation_FullChain_Idempotent(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()

	deps := &ReconcilerDeps{
		Store:  store,
		Events: NewEventBuffer(100),
		Log:    logging.New("test"),
	}

	reconcilers := []Reconciler{
		NewAgentReconciler(deps),
		NewContainerReconciler(deps),
		NewDeploymentReconciler(deps),
	}

	now := time.Now()
	saveNode(t, ctx, store, makeNode("agent-1", now))

	dep := makeRunningDeployment("dep-1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	saveTask(t, ctx, store, &types.TaskRecord{
		ID: "dep-1-web-0", DeploymentID: "dep-1", ServiceName: "web", ReplicaIndex: 0,
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0", ContainerStatus: "exited", ExitCode: 1, CreatedAt: now,
	})

	// First run
	RunReconciliation(ctx, reconcilers, logging.New("test"))
	count1 := countNewTasks(t, ctx, store, map[string]bool{"dep-1-web-0": true})

	// Second run — should be idempotent
	RunReconciliation(ctx, reconcilers, logging.New("test"))
	count2 := countNewTasks(t, ctx, store, map[string]bool{"dep-1-web-0": true})

	if count2 != count1 {
		t.Errorf("second run created extra tasks: first=%d second=%d", count1, count2)
	}
}
