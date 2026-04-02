package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

func TestAgentReconciler_AllHealthy_NoAction(t *testing.T) {
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

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks, got %d", n)
	}
}

func TestAgentReconciler_StaleAgent_MarksExitedAndReschedules(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always", Ports: []string{"8080:80"}, Environment: []string{"FOO=bar"}},
	})
	saveDeployment(t, ctx, store, dep)

	// Stale agent: last seen 3 minutes ago (exceeds 2min grace).
	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)

	// Healthy agent.
	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Original task should be marked exited with agent_stale reason.
	var updatedTask types.TaskRecord
	if err := store.Get(ctx, types.KeyTasks+"agent1/dep1-web-0", &updatedTask); err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.ContainerStatus != "exited" {
		t.Errorf("container_status: got %q, want %q", updatedTask.ContainerStatus, "exited")
	}
	if updatedTask.StopReason != "agent_stale" {
		t.Errorf("stop_reason: got %q, want %q", updatedTask.StopReason, "agent_stale")
	}

	// Should have cleanup task on stale agent.
	var cleanupFound bool
	staleKeys, _ := store.List(ctx, types.KeyTasks+"agent1/")
	for _, key := range staleKeys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.Type == types.TaskTypeStopAndRemove {
			cleanupFound = true
		}
	}
	if !cleanupFound {
		t.Error("expected cleanup task on stale agent")
	}

	// Should have rescheduled task on healthy agent-2.
	var rescheduleTask *types.TaskRecord
	healthyKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	for _, key := range healthyKeys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.Type == types.TaskTypeCreateAndStart {
			rescheduleTask = &tk
		}
	}
	if rescheduleTask == nil {
		t.Fatal("expected rescheduled task on agent-2")
	}
	if rescheduleTask.AgentID != "agent-2" {
		t.Errorf("agent: got %q, want %q", rescheduleTask.AgentID, "agent-2")
	}
	if rescheduleTask.RestartCount != 1 {
		t.Errorf("restart_count: got %d, want 1", rescheduleTask.RestartCount)
	}
	if rescheduleTask.Image != "nginx:latest" {
		t.Errorf("image: got %q, want %q", rescheduleTask.Image, "nginx:latest")
	}
	if len(rescheduleTask.Ports) != 1 || rescheduleTask.Ports[0] != "8080:80" {
		t.Errorf("ports: got %v, want [8080:80]", rescheduleTask.Ports)
	}
	if len(rescheduleTask.Environment) != 1 || rescheduleTask.Environment[0] != "FOO=bar" {
		t.Errorf("env: got %v, want [FOO=bar]", rescheduleTask.Environment)
	}
	if rescheduleTask.LastRestartAt.IsZero() {
		t.Error("expected LastRestartAt to be set")
	}
}

func TestAgentReconciler_WithinGracePeriod_NoAction(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	// Agent stale for 90s, within 2min grace.
	node := makeNode("agent1", time.Now().Add(-90*time.Second))
	saveNode(t, ctx, store, node)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (within grace), got %d", n)
	}
}

func TestAgentReconciler_LongRunning_ExtendedGrace(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	// Agent registered 1hr ago, stale for 3min → within 5min extended grace.
	node := &types.NodeRecord{
		Name:      "agent1",
		Status:    "ready",
		LastSeen:  time.Now().Add(-3 * time.Minute),
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	saveNode(t, ctx, store, node)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (extended grace), got %d", n)
	}
}

func TestAgentReconciler_LongRunning_PastExtendedGrace(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	// Agent registered 1hr ago, stale for 6min → past 5min extended grace.
	node := &types.NodeRecord{
		Name:      "agent1",
		Status:    "ready",
		LastSeen:  time.Now().Add(-6 * time.Minute),
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	saveNode(t, ctx, store, node)

	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created cleanup + reschedule tasks.
	if n := countNewTasks(t, ctx, store, origIDs); n < 2 {
		t.Errorf("expected at least 2 new tasks (cleanup + reschedule), got %d", n)
	}
}

func TestAgentReconciler_RestartNo_NoReschedule(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "no"},
	})
	saveDeployment(t, ctx, store, dep)

	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)
	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have cleanup but no reschedule on agent-2.
	healthyKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(healthyKeys) != 0 {
		t.Errorf("expected no tasks on agent-2, got %d", len(healthyKeys))
	}
}

func TestAgentReconciler_StatefullyPinned_NoReschedule(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {
			Image:    "nginx:latest",
			Replicas: 1,
			Restart:  "always",
			Volumes:  []types.VolumeMount{{Type: "bind", Source: "/data", Target: "/app/data"}},
		},
	})
	saveDeployment(t, ctx, store, dep)

	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)
	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have cleanup but no reschedule on agent-2.
	healthyKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(healthyKeys) != 0 {
		t.Errorf("expected no tasks on agent-2 (statefully pinned), got %d", len(healthyKeys))
	}
}

func TestAgentReconciler_AntiFlapping_SkipsRecentRestart(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)
	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	task.LastRestartAt = time.Now().Add(-2 * time.Minute) // restarted 2min ago, within 5min cooldown
	saveTask(t, ctx, store, task)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have cleanup but no reschedule on agent-2 (anti-flapping).
	healthyKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(healthyKeys) != 0 {
		t.Errorf("expected no tasks on agent-2 (anti-flapping), got %d", len(healthyKeys))
	}
}

func TestAgentReconciler_NoHealthyAgents_SkipsReschedule(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	// Only one agent and it's stale.
	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cleanup should exist, but no reschedule (no healthy agents).
	var cleanupCount, rescheduleCount int
	keys, _ := store.List(ctx, types.KeyTasks)
	for _, key := range keys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.ID != "dep1-web-0" {
			if tk.Type == types.TaskTypeStopAndRemove {
				cleanupCount++
			} else if tk.Type == types.TaskTypeCreateAndStart {
				rescheduleCount++
			}
		}
	}
	if cleanupCount != 1 {
		t.Errorf("expected 1 cleanup task, got %d", cleanupCount)
	}
	if rescheduleCount != 0 {
		t.Errorf("expected 0 reschedule tasks, got %d", rescheduleCount)
	}
}

func TestAgentReconciler_AlreadyExited_NoDuplicateCleanup(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)

	// Task already marked exited — should be skipped entirely.
	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, "exited")
	saveTask(t, ctx, store, task)

	origIDs := map[string]bool{"dep1-web-0": true}

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n := countNewTasks(t, ctx, store, origIDs); n != 0 {
		t.Errorf("expected 0 new tasks (already exited), got %d", n)
	}
}

func TestAgentReconciler_Idempotent_NoDuplicateReschedule(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	deps := testReconcilerDeps(store)

	dep := makeRunningDeployment("dep1", "app", map[string]types.ServiceRecord{
		"web": {Image: "nginx:latest", Replicas: 1, Restart: "always"},
	})
	saveDeployment(t, ctx, store, dep)

	staleNode := makeNode("agent1", time.Now().Add(-3*time.Minute))
	saveNode(t, ctx, store, staleNode)
	healthyNode := makeNode("agent-2", time.Now())
	saveNode(t, ctx, store, healthyNode)

	task := makeTask("dep1-web-0", "dep1", "web", "agent1", 0, types.StatusRunning)
	saveTask(t, ctx, store, task)

	// Pre-existing pending replacement on agent-2.
	pendingTask := &types.TaskRecord{
		ID:            "dep1-web-0-rs1",
		DeploymentID:  "dep1",
		ServiceName:   "web",
		AgentID:       "agent-2",
		ReplicaIndex:  0,
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusPending,
		ContainerName: "app-web-0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	saveTask(t, ctx, store, pendingTask)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have cleanup on stale agent but no new reschedule (pending exists).
	var rescheduleCount int
	keys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	for _, key := range keys {
		var tk types.TaskRecord
		if store.Get(ctx, key, &tk) == nil && tk.Type == types.TaskTypeCreateAndStart {
			rescheduleCount++
		}
	}
	if rescheduleCount != 1 {
		t.Errorf("expected exactly 1 create_and_start on agent-2 (existing pending), got %d", rescheduleCount)
	}
}

func TestAgentReconciler_StoreError_GracefulReturn(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
	deps := testReconcilerDeps(store)

	r := NewAgentReconciler(deps)
	if err := r.Reconcile(ctx); err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestAgentGracePeriod(t *testing.T) {
	tests := []struct {
		name     string
		node     types.NodeRecord
		expected time.Duration
	}{
		{
			name:     "fresh agent (5min) -> standard grace",
			node:     types.NodeRecord{CreatedAt: time.Now().Add(-5 * time.Minute)},
			expected: agentGracePeriodStandard,
		},
		{
			name:     "long-running agent (1hr) -> extended grace",
			node:     types.NodeRecord{CreatedAt: time.Now().Add(-1 * time.Hour)},
			expected: agentGracePeriodExtended,
		},
		{
			name:     "borderline agent (9min) -> standard grace",
			node:     types.NodeRecord{CreatedAt: time.Now().Add(-9 * time.Minute)},
			expected: agentGracePeriodStandard,
		},
		{
			name:     "just past threshold (11min) -> extended grace",
			node:     types.NodeRecord{CreatedAt: time.Now().Add(-11 * time.Minute)},
			expected: agentGracePeriodExtended,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentGracePeriod(tt.node)
			if got != tt.expected {
				t.Errorf("agentGracePeriod() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestShouldRescheduleOnAgentDeath(t *testing.T) {
	tests := []struct {
		name     string
		svc      types.ServiceRecord
		expected bool
	}{
		{
			name:     "restart always",
			svc:      types.ServiceRecord{Restart: "always"},
			expected: true,
		},
		{
			name:     "restart no",
			svc:      types.ServiceRecord{Restart: "no"},
			expected: false,
		},
		{
			name:     "restart unless-stopped",
			svc:      types.ServiceRecord{Restart: "unless-stopped"},
			expected: true,
		},
		{
			name:     "restart on-failure",
			svc:      types.ServiceRecord{Restart: "on-failure"},
			expected: true,
		},
		{
			name:     "restart empty (default)",
			svc:      types.ServiceRecord{Restart: ""},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRescheduleOnAgentDeath(tt.svc)
			if got != tt.expected {
				t.Errorf("shouldRescheduleOnAgentDeath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsStatefullyPinned(t *testing.T) {
	tests := []struct {
		name     string
		task     *types.TaskRecord
		svc      types.ServiceRecord
		expected bool
	}{
		{
			name:     "no volumes, no placement",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{},
			expected: false,
		},
		{
			name:     "placement set",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Placement: "worker-*"},
			expected: true,
		},
		{
			name:     "bind volume",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Volumes: []types.VolumeMount{{Type: "bind", Source: "/data", Target: "/app"}}},
			expected: true,
		},
		{
			name:     "named volume",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Volumes: []types.VolumeMount{{Type: "volume", Source: "mydata", Target: "/data"}}},
			expected: true,
		},
		{
			name:     "tmpfs volume - not pinned",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Volumes: []types.VolumeMount{{Type: "tmpfs", Target: "/tmp"}}},
			expected: false,
		},
		{
			name:     "nfs volume - not pinned",
			task:     &types.TaskRecord{},
			svc:      types.ServiceRecord{Volumes: []types.VolumeMount{{Type: "nfs", Source: "server:/share", Target: "/mnt"}}},
			expected: false,
		},
		{
			name: "mixed: tmpfs + bind",
			task: &types.TaskRecord{},
			svc: types.ServiceRecord{Volumes: []types.VolumeMount{
				{Type: "tmpfs", Target: "/tmp"},
				{Type: "bind", Source: "/data", Target: "/app"},
			}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStatefullyPinned(tt.task, tt.svc)
			if got != tt.expected {
				t.Errorf("isStatefullyPinned() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatchPlacement(t *testing.T) {
	tests := []struct {
		agent   string
		pattern string
		want    bool
	}{
		{"worker-1", "worker-*", true},
		{"agent-1", "worker-*", false},
		{"gpu-node", "gpu-*", true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%s", tt.agent, tt.pattern), func(t *testing.T) {
			if got := matchPlacement(tt.agent, tt.pattern); got != tt.want {
				t.Errorf("matchPlacement(%q, %q) = %v, want %v", tt.agent, tt.pattern, got, tt.want)
			}
		})
	}
}
