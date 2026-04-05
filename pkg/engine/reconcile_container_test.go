package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// --- helpers ---

func testReconcilerDeps(store storage.StateStore) *ReconcilerDeps {
	return &ReconcilerDeps{
		Store:  store,
		Events: NewEventBuffer(100),
		Log:    logging.New("test"),
	}
}

func saveDeployment(t *testing.T, ctx context.Context, store storage.StateStore, dep *types.DeploymentRecord) {
	t.Helper()
	if err := store.Save(ctx, types.KeyDeployments+dep.ID, dep); err != nil {
		t.Fatalf("save deployment: %v", err)
	}
}

func saveNode(t *testing.T, ctx context.Context, store storage.StateStore, node *types.NodeRecord) {
	t.Helper()
	if err := store.Save(ctx, types.KeyNodes+node.Name, node); err != nil {
		t.Fatalf("save node: %v", err)
	}
}

func saveTask(t *testing.T, ctx context.Context, store storage.StateStore, task *types.TaskRecord) {
	t.Helper()
	key := types.KeyTasks + task.AgentID + "/" + task.ID
	if err := store.Save(ctx, key, task); err != nil {
		t.Fatalf("save task: %v", err)
	}
}

// countNewTasks counts tasks not in the original set.
func countNewTasks(t *testing.T, ctx context.Context, store storage.StateStore, originalIDs map[string]bool) int {
	t.Helper()
	count := 0
	keys, _ := store.List(ctx, types.KeyTasks)
	for _, key := range keys {
		var task types.TaskRecord
		if err := store.Get(ctx, key, &task); err != nil {
			continue
		}
		if !originalIDs[task.ID] {
			count++
		}
	}
	return count
}

func makeRunningDeployment(id, name string, services map[string]types.ServiceRecord) *types.DeploymentRecord {
	return &types.DeploymentRecord{
		ID:        id,
		Name:      name,
		Status:    types.StatusRunning,
		Services:  services,
		CreatedAt: time.Now().Add(-time.Hour),
		UpdatedAt: time.Now(),
	}
}

func makeNode(name string, lastSeen time.Time) *types.NodeRecord {
	return &types.NodeRecord{
		Name:      name,
		Status:    "ready",
		LastSeen:  lastSeen,
		CreatedAt: time.Now().Add(-5 * time.Minute), // recently registered → standard 2min grace
	}
}

func makeTask(id, depID, svcName, agentID string, replicaIndex int, containerStatus string) *types.TaskRecord {
	return &types.TaskRecord{
		ID:              id,
		DeploymentID:    depID,
		DeploymentName:  "app",
		ServiceName:     svcName,
		AgentID:         agentID,
		ReplicaIndex:    replicaIndex,
		Type:            types.TaskTypeCreateAndStart,
		Status:          types.StatusCompleted,
		Image:           "nginx:latest",
		ContainerName:   fmt.Sprintf("app-%s-%d", svcName, replicaIndex),
		ContainerStatus: containerStatus,
		CreatedAt:       time.Now().Add(-30 * time.Minute),
		UpdatedAt:       time.Now(),
	}
}

// --- Tests ---

func TestContainerReconciler_HealthyDeployment_NoAction(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks, got %d", n)
	}
}

func TestContainerReconciler_RestartAlways(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.ExitCode = 1
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 2 {
		t.Errorf("expected 2 new tasks (cleanup + restart), got %d", n)
	}
}

func TestContainerReconciler_RestartNo(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "no"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks, got %d", n)
	}
}

func TestContainerReconciler_OnFailure_ZeroExit(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "on-failure"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.ExitCode = 0
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks for exit code 0, got %d", n)
	}
}

func TestContainerReconciler_OnFailure_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "on-failure"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.ExitCode = 137
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 2 {
		t.Errorf("expected 2 new tasks, got %d", n)
	}
}

func TestContainerReconciler_OnFailureWithLimit(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "on-failure:3"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.ExitCode = 1
	task.RestartCount = 3 // already at limit
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (limit reached), got %d", n)
	}
}

func TestContainerReconciler_UnlessStopped_UserStop(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "unless-stopped"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.StopReason = "user"
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (user stop), got %d", n)
	}
}

func TestContainerReconciler_UnlessStopped_Crash(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "unless-stopped"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.StopReason = "" // crash, not user
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 2 {
		t.Errorf("expected 2 new tasks, got %d", n)
	}
}

func TestContainerReconciler_StaleAgent_Skipped(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	// Agent last seen 2 minutes ago (stale).
	saveNode(t, ctx, store, makeNode("agent1", time.Now().Add(-2*time.Minute)))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (stale agent), got %d", n)
	}
}

func TestContainerReconciler_PendingRestartExists_Skipped(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// Exited task.
	exitedTask := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, exitedTask)

	// Already-pending restart task.
	pendingTask := &types.TaskRecord{
		ID:              "dep1-web-0-r1",
		DeploymentID:    "dep1",
		ServiceName:     "web",
		AgentID:         "agent1",
		ReplicaIndex:    0,
		Type:            types.TaskTypeCreateAndStart,
		Status:          types.StatusPending,
		ContainerName:   "app-web-0",
		ContainerStatus: "",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	saveTask(t, ctx, store, pendingTask)

	origIDs := map[string]bool{"dep1-web-0": true, "dep1-web-0-r1": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (pending exists), got %d", n)
	}
}

func TestContainerReconciler_Backoff_SecondCrash(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	r := NewContainerReconciler(deps)

	// First crash -> should restart.
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate: the restart task also crashed immediately.
	// Remove old tasks and add a new exited one.
	newTask := makeTask("dep1-web-0-r1", "dep1", "web", "agent1", 0, "exited")
	newTask.RestartCount = 1
	newTask.CreatedAt = time.Now() // newer than original
	saveTask(t, ctx, store, newTask)

	// Delete the pending tasks from first reconcile so hasPending is false.
	keys, _ := store.List(ctx, types.KeyTasks)
	for _, key := range keys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.Status == types.StatusPending {
			_ = store.Delete(ctx, key)
		}
	}

	origIDs := map[string]bool{}
	keys, _ = store.List(ctx, types.KeyTasks)
	for _, key := range keys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil {
			origIDs[tk.ID] = true
		}
	}

	// Second crash within 30s -> should be blocked by backoff.
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (backoff), got %d", n)
	}
}

func TestContainerReconciler_Backoff_Elapsed(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	r := NewContainerReconciler(deps)

	// Manually record a restart in the past so backoff has elapsed.
	backoffKey := "dep1-web-0"
	r.backoff.mu.Lock()
	r.backoff.entries[backoffKey] = &backoffEntry{
		consecutiveFailures: 1,
		nextRetryAt:         time.Now().Add(-1 * time.Second), // already elapsed
	}
	r.backoff.mu.Unlock()

	origIDs := map[string]bool{"dep1-web-0": true}

	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 2 {
		t.Errorf("expected 2 new tasks (backoff elapsed), got %d", n)
	}
}

func TestContainerReconciler_BackoffReset_HealthyLongEnough(t *testing.T) {
	b := newContainerBackoffTracker()
	key := "dep1-web-0"

	// Simulate a restart.
	b.recordRestart(key)
	if b.allows(key) {
		t.Fatal("should be blocked after restart")
	}

	// Simulate healthy for 10+ minutes.
	b.mu.Lock()
	b.entries[key].healthySince = time.Now().Add(-11 * time.Minute)
	b.mu.Unlock()

	b.markHealthy(key)

	if !b.allows(key) {
		t.Error("should be allowed after healthy reset")
	}
}

func TestContainerReconciler_DeploymentNotRunning_Skipped(t *testing.T) {
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

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (deployment stopped), got %d", n)
	}
}

func TestContainerReconciler_DefaultRestartPolicy(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	// Empty restart field -> defaults to "always".
	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: ""},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 2 {
		t.Errorf("expected 2 new tasks (default restart=always), got %d", n)
	}
}

func TestContainerReconciler_StoreError_GracefulReturn(t *testing.T) {
	ctx := context.Background()
	// Empty store -> List for deployments returns empty, no error.
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("expected no error on empty store, got: %v", err)
	}
}

func TestContainerReconciler_RestartTaskHasCorrectFields(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {
			Image:             "nginx:1.25",
			Replicas:          1,
			Restart:           "always",
			Ports:             []string{"8080:80"},
			Environment:       []string{"FOO=bar"},
			Command:           []string{"/bin/sh"},
			Entrypoint:        []string{"/docker-entrypoint.sh"},
			MemoryLimit:       "512m",
			CPULimit:          "0.5",
			MemoryReservation: "256m",
		},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	task.RestartCount = 2
	saveTask(t, ctx, store, task)

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the new create_and_start task.
	keys, _ := store.List(ctx, types.KeyTasks)
	var restartTask *types.TaskRecord
	for _, key := range keys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.ID != "dep1-web-0" && tk.Type == types.TaskTypeCreateAndStart {
			restartTask = &tk
			break
		}
	}

	if restartTask == nil {
		t.Fatal("restart task not found")
	}

	// Verify fields copied from service config.
	if restartTask.Image != "nginx:1.25" {
		t.Errorf("image: got %q, want %q", restartTask.Image, "nginx:1.25")
	}
	if restartTask.RestartCount != 3 {
		t.Errorf("restart count: got %d, want 3", restartTask.RestartCount)
	}
	if restartTask.AgentID != "agent1" {
		t.Errorf("agent: got %q, want %q", restartTask.AgentID, "agent1")
	}
	if len(restartTask.Ports) != 1 || restartTask.Ports[0] != "8080:80" {
		t.Errorf("ports: got %v, want [8080:80]", restartTask.Ports)
	}
	if len(restartTask.Environment) != 1 || restartTask.Environment[0] != "FOO=bar" {
		t.Errorf("env: got %v, want [FOO=bar]", restartTask.Environment)
	}
	if restartTask.MemoryLimit != "512m" {
		t.Errorf("memory_limit: got %q, want %q", restartTask.MemoryLimit, "512m")
	}
	if restartTask.CPULimit != "0.5" {
		t.Errorf("cpu_limit: got %q, want %q", restartTask.CPULimit, "0.5")
	}
	if restartTask.Status != types.StatusPending {
		t.Errorf("status: got %q, want %q", restartTask.Status, types.StatusPending)
	}
	expectedID := "dep1-web-0-r3"
	if restartTask.ID != expectedID {
		t.Errorf("task ID: got %q, want %q", restartTask.ID, expectedID)
	}
}

// Regression: cleanup task must not shadow exited container in latestByReplica.
// A stop_and_remove cleanup task for the same replica has a newer CreatedAt and
// would be selected as "latest", hiding the exited create_and_start task.
// The reconciler must only track create_and_start tasks in latestByReplica.
func TestContainerReconciler_CleanupTaskDoesNotShadowExited(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"api": {Image: "api:latest", Replicas: 3, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// Two healthy replicas
	saveTask(t, ctx, store, makeTask("dep1-api-1", "dep1", "api", "agent1", 1, types.StatusRunning))
	saveTask(t, ctx, store, makeTask("dep1-api-2", "dep1", "api", "agent1", 2, types.StatusRunning))

	// Replica 0: original task is exited
	exitedTask := makeTask("dep1-api-0", "dep1", "api", "agent1", 0, "exited")
	exitedTask.CreatedAt = time.Now().Add(-time.Hour)
	saveTask(t, ctx, store, exitedTask)

	// Replica 0: cleanup task exists with NEWER CreatedAt (shadows the exited task if not filtered)
	cleanupTask := &types.TaskRecord{
		ID:            "dep1-api-0-agent-cleanup",
		DeploymentID:  "dep1",
		ServiceName:   "api",
		AgentID:       "agent1",
		ReplicaIndex:  0,
		Type:          types.TaskTypeStopAndRemove,
		Status:        types.StatusCompleted,
		ContainerName: "app-api-0",
		CreatedAt:     time.Now(), // newer than the exited task
	}
	saveTask(t, ctx, store, cleanupTask)

	origIDs := map[string]bool{
		"dep1-api-0": true, "dep1-api-1": true, "dep1-api-2": true,
		"dep1-api-0-agent-cleanup": true,
	}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A restart task should be created for replica 0 despite the cleanup task
	if n := countNewTasks(t, ctx, store, origIDs); n < 1 {
		t.Errorf("expected restart task for exited replica 0 (cleanup task must not shadow it), got %d new tasks", n)
	}
}

// Regression: container with status "not_found" should be restarted, same as "exited".
// Before the fix, "not_found" containers were stuck forever — the reconciler only
// checked for "exited" and ignored all other statuses.
func TestContainerReconciler_NotFound_RestartedLikeExited(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)
	saveNode(t, ctx, store, makeNode("agent1", time.Now()))

	// Container was removed from nerdctl (e.g., cleanup ran but create failed).
	// Agent reports "not_found" as the container status.
	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "not_found")
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewContainerReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n < 1 {
		t.Errorf("expected restart task for not_found container, got %d new tasks", n)
	}
}

// --- mockReconciler for RunReconciliation tests ---

type mockReconciler struct {
	err      error
	panics   bool
	called   bool
}

func (m *mockReconciler) Reconcile(_ context.Context) error {
	m.called = true
	if m.panics {
		panic("test panic")
	}
	return m.err
}

func TestRunReconciliation_AllSucceed(t *testing.T) {
	r1 := &mockReconciler{}
	r2 := &mockReconciler{}
	r3 := &mockReconciler{}

	RunReconciliation(context.Background(), []Reconciler{r1, r2, r3}, logging.New("test"))

	if !r1.called || !r2.called || !r3.called {
		t.Error("all reconcilers should have been called")
	}
}

func TestRunReconciliation_OneErrors_OthersContinue(t *testing.T) {
	r1 := &mockReconciler{}
	r2 := &mockReconciler{err: fmt.Errorf("something broke")}
	r3 := &mockReconciler{}

	RunReconciliation(context.Background(), []Reconciler{r1, r2, r3}, logging.New("test"))

	if !r1.called || !r2.called || !r3.called {
		t.Error("all reconcilers should have been called even after error")
	}
}

func TestRunReconciliation_PanicRecovery(t *testing.T) {
	r1 := &mockReconciler{}
	r2 := &mockReconciler{panics: true}
	r3 := &mockReconciler{}

	RunReconciliation(context.Background(), []Reconciler{r1, r2, r3}, logging.New("test"))

	if !r1.called || !r2.called || !r3.called {
		t.Error("all reconcilers should have been called even after panic")
	}
}

// --- shouldRestart table-driven tests ---

func TestShouldRestart(t *testing.T) {
	tests := []struct {
		name     string
		task     *types.TaskRecord
		svc      types.ServiceRecord
		expected bool
	}{
		{
			name:     "empty policy defaults to always",
			task:     &types.TaskRecord{ExitCode: 1},
			svc:      types.ServiceRecord{Restart: ""},
			expected: true,
		},
		{
			name:     "no never restarts",
			task:     &types.TaskRecord{ExitCode: 1},
			svc:      types.ServiceRecord{Restart: "no"},
			expected: false,
		},
		{
			name:     "always restarts on exit 0",
			task:     &types.TaskRecord{ExitCode: 0},
			svc:      types.ServiceRecord{Restart: "always"},
			expected: true,
		},
		{
			name:     "always restarts on exit 1",
			task:     &types.TaskRecord{ExitCode: 1},
			svc:      types.ServiceRecord{Restart: "always"},
			expected: true,
		},
		{
			name:     "on-failure skips exit 0",
			task:     &types.TaskRecord{ExitCode: 0},
			svc:      types.ServiceRecord{Restart: "on-failure"},
			expected: false,
		},
		{
			name:     "on-failure restarts exit 1",
			task:     &types.TaskRecord{ExitCode: 1},
			svc:      types.ServiceRecord{Restart: "on-failure"},
			expected: true,
		},
		{
			name:     "on-failure:3 at limit",
			task:     &types.TaskRecord{ExitCode: 1, RestartCount: 3},
			svc:      types.ServiceRecord{Restart: "on-failure:3"},
			expected: false,
		},
		{
			name:     "on-failure:3 below limit",
			task:     &types.TaskRecord{ExitCode: 1, RestartCount: 2},
			svc:      types.ServiceRecord{Restart: "on-failure:3"},
			expected: true,
		},
		{
			name:     "unless-stopped with user stop",
			task:     &types.TaskRecord{StopReason: "user"},
			svc:      types.ServiceRecord{Restart: "unless-stopped"},
			expected: false,
		},
		{
			name:     "unless-stopped with crash",
			task:     &types.TaskRecord{StopReason: ""},
			svc:      types.ServiceRecord{Restart: "unless-stopped"},
			expected: true,
		},
		{
			name:     "unknown policy defaults to true",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Restart: "unknown-policy"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRestart(tt.task, tt.svc)
			if got != tt.expected {
				t.Errorf("shouldRestart() = %v, want %v", got, tt.expected)
			}
		})
	}
}
