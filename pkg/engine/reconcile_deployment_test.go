package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

func TestDeploymentReconciler_AllHealthy(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 2},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	for i := 0; i < 2; i++ {
		task := makeTask("dep1-web-"+itoa(i), "dep1", "web", "agent1", i, types.StatusRunning)
		saveTask(t, ctx, store, task)
	}

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "healthy" {
		t.Errorf("health: got %q, want %q", updated.HealthStatus, "healthy")
	}
}

func TestDeploymentReconciler_Recovering(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 2},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// One healthy, one recently restarted.
	task0 := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task0)

	task1 := makeTask("dep1-web-1", "dep1", "web", "agent1", 1, types.StatusRunning)
	task1.RestartCount = 2
	task1.LastRestartAt = time.Now().Add(-2 * time.Minute)
	saveTask(t, ctx, store, task1)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "recovering" {
		t.Errorf("health: got %q, want %q", updated.HealthStatus, "recovering")
	}
}

func TestDeploymentReconciler_Degraded(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 2},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// One running, one exited.
	task0 := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task0)

	task1 := makeTask("dep1-web-1", "dep1", "web", "agent1", 1, "exited")
	saveTask(t, ctx, store, task1)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "degraded" {
		t.Errorf("health: got %q, want %q", updated.HealthStatus, "degraded")
	}
}

func TestDeploymentReconciler_AllDead_TransitionsToStopped(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 2},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	for i := 0; i < 2; i++ {
		task := makeTask("dep1-web-"+itoa(i), "dep1", "web", "agent1", i, "exited")
		saveTask(t, ctx, store, task)
	}

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "stopped" {
		t.Errorf("health: got %q, want %q", updated.HealthStatus, "stopped")
	}
	if updated.Status != types.StatusStopped {
		t.Errorf("status: got %q, want %q", updated.Status, types.StatusStopped)
	}
}

func TestDeploymentReconciler_PendingRestart_Recovering(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// Pending restart task (newer than the exited one).
	exitedTask := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	exitedTask.CreatedAt = time.Now().Add(-5 * time.Minute)
	saveTask(t, ctx, store, exitedTask)

	pendingTask := &types.TaskRecord{
		ID:              "dep1-web-0-r1",
		DeploymentID:    "dep1",
		DeploymentName:  "app",
		ServiceName:     "web",
		AgentID:         "agent1",
		ReplicaIndex:    0,
		Type:            types.TaskTypeCreateAndStart,
		Status:          types.StatusPending,
		Image:           "nginx:latest",
		ContainerName:   "app-web-0",
		ContainerStatus: "",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	saveTask(t, ctx, store, pendingTask)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "recovering" {
		t.Errorf("health: got %q, want %q", updated.HealthStatus, "recovering")
	}
}

func TestDeploymentReconciler_SupersededDeployment(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	// Older deployment.
	old := &types.DeploymentRecord{
		ID:        "dep-old",
		Name:      "app",
		Status:    types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx:1.24", Replicas: 1}},
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	saveDeployment(t, ctx, store, old)

	// Newer deployment with the same name.
	newer := &types.DeploymentRecord{
		ID:        "dep-new",
		Name:      "app",
		Status:    types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx:1.25", Replicas: 1}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	saveDeployment(t, ctx, store, newer)

	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// Tasks for newer deployment.
	task := makeTask("dep-new-web-0", "dep-new", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var oldDep types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep-old", &oldDep); err != nil {
		t.Fatalf("get old deployment: %v", err)
	}
	if oldDep.Status != types.StatusStopped {
		t.Errorf("old deployment status: got %q, want %q", oldDep.Status, types.StatusStopped)
	}
	if oldDep.HealthStatus != "stopped" {
		t.Errorf("old deployment health: got %q, want %q", oldDep.HealthStatus, "stopped")
	}

	// Newer should remain running.
	var newDep types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep-new", &newDep); err != nil {
		t.Fatalf("get new deployment: %v", err)
	}
	if newDep.Status != types.StatusRunning {
		t.Errorf("new deployment status: got %q, want %q", newDep.Status, types.StatusRunning)
	}
}

func TestDeploymentReconciler_NonRunning_Skipped(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := &types.DeploymentRecord{
		ID:       "dep1",
		Name:     "app",
		Status:   types.StatusStopped,
		Services: map[string]types.ServiceRecord{"web": {Image: "nginx:latest", Replicas: 1}},
	}
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Health should not be updated for non-running deployments.
	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if updated.HealthStatus != "" {
		t.Errorf("health: got %q, want empty (not updated)", updated.HealthStatus)
	}
}

func TestDeploymentReconciler_HealthUnchanged_NoWrite(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1},
	})
	dep.HealthStatus = "healthy"
	originalUpdatedAt := dep.UpdatedAt
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated types.DeploymentRecord
	if err := store.Get(ctx, types.KeyDeployments+"dep1", &updated); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	// UpdatedAt should not change since health was already "healthy".
	if !updated.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("expected no write when health is unchanged")
	}
}

func TestDeploymentReconciler_StoreError_GracefulReturn(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
	deps := testReconcilerDeps(store)

	r := NewDeploymentReconciler(deps)
	if err := r.Reconcile(ctx); err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestComputeHealthStatus(t *testing.T) {
	tests := []struct {
		name     string
		dep      *types.DeploymentRecord
		tasks    []types.TaskRecord
		expected string
	}{
		{
			name: "all running",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 2}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, CreatedAt: time.Now()},
				{ServiceName: "web", ReplicaIndex: 1, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, CreatedAt: time.Now()},
			},
			expected: "healthy",
		},
		{
			name: "one exited",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 2}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, CreatedAt: time.Now()},
				{ServiceName: "web", ReplicaIndex: 1, Type: types.TaskTypeCreateAndStart, ContainerStatus: "exited", CreatedAt: time.Now()},
			},
			expected: "degraded",
		},
		{
			name: "all exited",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 2}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: "exited", CreatedAt: time.Now()},
				{ServiceName: "web", ReplicaIndex: 1, Type: types.TaskTypeCreateAndStart, ContainerStatus: "exited", CreatedAt: time.Now()},
			},
			expected: "stopped",
		},
		{
			name: "pending restart",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 1}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: "exited", CreatedAt: time.Now().Add(-5 * time.Minute)},
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, Status: types.StatusPending, ContainerStatus: "", CreatedAt: time.Now()},
			},
			expected: "recovering",
		},
		{
			name: "recently restarted running",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 1}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, RestartCount: 2, LastRestartAt: time.Now().Add(-2 * time.Minute), CreatedAt: time.Now()},
			},
			expected: "recovering",
		},
		{
			name: "restarted long ago",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 1}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, RestartCount: 2, LastRestartAt: time.Now().Add(-15 * time.Minute), CreatedAt: time.Now()},
			},
			expected: "healthy",
		},
		{
			name: "no tasks",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 1}},
			},
			tasks:    nil,
			expected: "stopped",
		},
		{
			name: "stop_and_remove tasks ignored",
			dep: &types.DeploymentRecord{
				Services: map[string]types.ServiceRecord{"web": {Replicas: 1}},
			},
			tasks: []types.TaskRecord{
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeCreateAndStart, ContainerStatus: types.StatusRunning, CreatedAt: time.Now()},
				{ServiceName: "web", ReplicaIndex: 0, Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted, CreatedAt: time.Now().Add(time.Second)},
			},
			expected: "healthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeHealthStatus(tt.dep, tt.tasks)
			if got != tt.expected {
				t.Errorf("computeHealthStatus() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// itoa is a simple int to string conversion for test IDs.
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
