package engine

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// --- Auto-scale tests ---

func TestEvaluateAutoscale_ScaleUp(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	// Create a running deployment with autoscale config
	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:    "myapp:latest",
				Replicas: 2,
				Autoscale: &types.ManifestAutoscale{
					Min:       1,
					Max:       5,
					TargetCPU: 70,
					Cooldown:  "1s", // short for testing
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	// Create 2 running tasks with HIGH CPU (> target 70%)
	for i := 0; i < 2; i++ {
		task := &types.TaskRecord{
			ID:              "dep-1-api-" + string(rune('0'+i)),
			DeploymentID:    "dep-1",
			ServiceName:     "api",
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusRunning,
			CPUPercent:      85.0, // above target (70%)
			AgentID:         "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}

	// Create an available agent
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
	})

	// Run autoscale evaluation
	e.evaluateAutoscale(ctx)

	// Should have scaled up: check that a new task was created
	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)

	if dep2.Services["api"].Replicas != 3 {
		t.Errorf("expected replicas=3 after scale up, got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_ScaleDown(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:    "myapp:latest",
				Replicas: 3,
				Autoscale: &types.ManifestAutoscale{
					Min:       1,
					Max:       5,
					TargetCPU: 70,
					Cooldown:  "1s",
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	// 3 running tasks with LOW CPU (< target/2 = 35%)
	for i := 0; i < 3; i++ {
		task := &types.TaskRecord{
			ID:              "dep-1-api-" + string(rune('0'+i)),
			DeploymentID:    "dep-1",
			ServiceName:     "api",
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusRunning,
			CPUPercent:      10.0, // well below target/2 (35%)
			AgentID:         "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
	})

	e.evaluateAutoscale(ctx)

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)

	if dep2.Services["api"].Replicas != 2 {
		t.Errorf("expected replicas=2 after scale down, got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_RespectsMin(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:    "myapp:latest",
				Replicas: 2,
				Autoscale: &types.ManifestAutoscale{
					Min:       2, // min = current
					Max:       5,
					TargetCPU: 70,
					Cooldown:  "1s",
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	// Low CPU but already at min
	for i := 0; i < 2; i++ {
		task := &types.TaskRecord{
			ID: "dep-1-api-" + string(rune('0'+i)), DeploymentID: "dep-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerStatus: types.StatusRunning,
			CPUPercent: 5.0, AgentID: "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
	})

	e.evaluateAutoscale(ctx)

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)
	if dep2.Services["api"].Replicas != 2 {
		t.Errorf("expected replicas=2 (min), got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_RespectsMax(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:    "myapp:latest",
				Replicas: 3,
				Autoscale: &types.ManifestAutoscale{
					Min:       1,
					Max:       3, // max = current
					TargetCPU: 70,
					Cooldown:  "1s",
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	for i := 0; i < 3; i++ {
		task := &types.TaskRecord{
			ID: "dep-1-api-" + string(rune('0'+i)), DeploymentID: "dep-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerStatus: types.StatusRunning,
			CPUPercent: 95.0, AgentID: "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
	})

	e.evaluateAutoscale(ctx)

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)
	if dep2.Services["api"].Replicas != 3 {
		t.Errorf("expected replicas=3 (max), got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_RespectsCooldown(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:       "myapp:latest",
				Replicas:    2,
				LastScaleAt: time.Now(), // just scaled
				Autoscale: &types.ManifestAutoscale{
					Min:       1,
					Max:       5,
					TargetCPU: 70,
					Cooldown:  "1h", // 1 hour cooldown
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	for i := 0; i < 2; i++ {
		task := &types.TaskRecord{
			ID: "dep-1-api-" + string(rune('0'+i)), DeploymentID: "dep-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerStatus: types.StatusRunning,
			CPUPercent: 95.0, AgentID: "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
	})

	e.evaluateAutoscale(ctx)

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)
	// Should NOT scale because cooldown hasn't expired
	if dep2.Services["api"].Replicas != 2 {
		t.Errorf("expected replicas=2 (cooldown), got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_NoMetrics(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {
				Image:    "myapp:latest",
				Replicas: 2,
				Autoscale: &types.ManifestAutoscale{
					Min: 1, Max: 5, TargetCPU: 70, Cooldown: "1s",
				},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	// Tasks with NO CPU metrics (CPUPercent = 0)
	for i := 0; i < 2; i++ {
		task := &types.TaskRecord{
			ID: "dep-1-api-" + string(rune('0'+i)), DeploymentID: "dep-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerStatus: types.StatusRunning,
			CPUPercent: 0, AgentID: "agent-1",
		}
		store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)
	}

	e.evaluateAutoscale(ctx)

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)
	if dep2.Services["api"].Replicas != 2 {
		t.Errorf("expected replicas=2 (no metrics), got %d", dep2.Services["api"].Replicas)
	}
}

func TestEvaluateAutoscale_SkipsNonRunning(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	dep := &types.DeploymentRecord{
		ID:     "dep-1",
		Name:   "myapp",
		Status: types.StatusDeploying, // not RUNNING
		Services: map[string]types.ServiceRecord{
			"api": {
				Image: "myapp:latest", Replicas: 2,
				Autoscale: &types.ManifestAutoscale{Min: 1, Max: 5, TargetCPU: 70},
			},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	e.evaluateAutoscale(ctx) // should do nothing

	var dep2 types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"dep-1", &dep2)
	if dep2.Services["api"].Replicas != 2 {
		t.Errorf("expected replicas=2 (deploying, should skip), got %d", dep2.Services["api"].Replicas)
	}
}

// --- Rebalance tests ---

func TestEvaluateRebalance_MigratesOverloaded(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	// Agent 1: overloaded (96% CPU)
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.96, MemoryTotalBytes: 8 * 1024 * 1024 * 1024,
		MemoryUsedBytes: 2 * 1024 * 1024 * 1024,
	})
	// Agent 2: underloaded (20% CPU)
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.20, MemoryTotalBytes: 8 * 1024 * 1024 * 1024,
		MemoryUsedBytes: 1 * 1024 * 1024 * 1024,
	})

	// Stateless container on agent-1
	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {Image: "myapp:latest", Replicas: 1},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	task := &types.TaskRecord{
		ID: "dep-1-api-0", DeploymentID: "dep-1", ServiceName: "api",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning, AgentID: "agent-1",
		CPUPercent: 15.0, ContainerName: "myapp-api-0",
	}
	store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)

	// Clear cooldown from previous tests
	for k := range rebalanceMigrationCooldown {
		delete(rebalanceMigrationCooldown, k)
	}

	e.evaluateRebalance(ctx)

	// Should have created a stop task on agent-1 and start task on agent-2
	stopKeys, _ := store.List(ctx, types.KeyTasks+"agent-1/")
	startKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")

	foundStop := false
	for _, k := range stopKeys {
		var t types.TaskRecord
		store.Get(ctx, k, &t)
		if t.Type == types.TaskTypeStopAndRemove {
			foundStop = true
		}
	}
	foundStart := false
	for _, k := range startKeys {
		var t types.TaskRecord
		store.Get(ctx, k, &t)
		if t.Type == types.TaskTypeCreateAndStart {
			foundStart = true
		}
	}

	if !foundStop {
		t.Error("expected stop task on overloaded agent")
	}
	if !foundStart {
		t.Error("expected start task on underloaded agent")
	}
}

func TestEvaluateRebalance_SkipsStateful(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.96,
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.20,
	})

	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"db": {Image: "postgres", Replicas: 1},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	// Container with volumes — should NOT be migrated
	task := &types.TaskRecord{
		ID: "dep-1-db-0", DeploymentID: "dep-1", ServiceName: "db",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning, AgentID: "agent-1",
		Volumes: []types.VolumeMount{{Type: "volume", Source: "data", Target: "/data"}},
	}
	store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)

	for k := range rebalanceMigrationCooldown {
		delete(rebalanceMigrationCooldown, k)
	}

	e.evaluateRebalance(ctx)

	// No tasks should be created on agent-2
	startKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(startKeys) > 0 {
		t.Error("stateful container should NOT be migrated")
	}
}

func TestEvaluateRebalance_SkipsPinned(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.96,
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.20,
	})

	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {Image: "myapp", Replicas: 1, Placement: "agent-1"}, // pinned
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	task := &types.TaskRecord{
		ID: "dep-1-api-0", DeploymentID: "dep-1", ServiceName: "api",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning, AgentID: "agent-1",
	}
	store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)

	for k := range rebalanceMigrationCooldown {
		delete(rebalanceMigrationCooldown, k)
	}

	e.evaluateRebalance(ctx)

	startKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(startKeys) > 0 {
		t.Error("pinned container should NOT be migrated")
	}
}

func TestEvaluateRebalance_RespectsCooldown(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.96,
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.20,
	})

	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {Image: "myapp", Replicas: 1},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	task := &types.TaskRecord{
		ID: "dep-1-api-0", DeploymentID: "dep-1", ServiceName: "api",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning, AgentID: "agent-1",
		ContainerName: "myapp-api-0",
	}
	store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)

	// Set cooldown — recently migrated
	rebalanceMigrationCooldown["myapp-api-0"] = time.Now()

	e.evaluateRebalance(ctx)

	startKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(startKeys) > 0 {
		t.Error("recently migrated container should NOT be migrated again")
	}

	// Clean up
	delete(rebalanceMigrationCooldown, "myapp-api-0")
}

func TestEvaluateRebalance_NoImbalance(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	// Both agents at moderate load — no rebalancing needed
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.60,
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.50,
	})

	e.evaluateRebalance(ctx) // should do nothing (no overloaded agents)
}

func TestEvaluateRebalance_MemoryOverload(t *testing.T) {
	store := storage.NewMemoryStore()
	e := &Engine{store: store, log: nil}
	ctx := context.Background()

	// Agent 1: low CPU but 96% memory — overloaded by memory
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.30,
		MemoryTotalBytes: 4_000_000_000,
		MemoryUsedBytes:  3_900_000_000, // 97.5%
	})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", LastSeen: time.Now(),
		CPUUsageRatio: 0.20,
		MemoryTotalBytes: 8 * 1024 * 1024 * 1024,
		MemoryUsedBytes:  1 * 1024 * 1024 * 1024, // ~12%
	})

	dep := &types.DeploymentRecord{
		ID: "dep-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"api": {Image: "myapp", Replicas: 1},
		},
	}
	store.Save(ctx, types.KeyDeployments+"dep-1", dep)

	task := &types.TaskRecord{
		ID: "dep-1-api-0", DeploymentID: "dep-1", ServiceName: "api",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerStatus: types.StatusRunning, AgentID: "agent-1",
		CPUPercent: 5.0, ContainerName: "myapp-api-0",
	}
	store.Save(ctx, types.KeyTasks+"agent-1/"+task.ID, task)

	for k := range rebalanceMigrationCooldown {
		delete(rebalanceMigrationCooldown, k)
	}

	e.evaluateRebalance(ctx)

	// Should migrate because memory is overloaded
	startKeys, _ := store.List(ctx, types.KeyTasks+"agent-2/")
	if len(startKeys) == 0 {
		t.Error("expected migration due to memory overload")
	}
}

// --- Helper tests ---

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"60s", 60 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"", 30 * time.Second},           // default
		{"invalid", 30 * time.Second},     // default on error
	}
	for _, tt := range tests {
		result := parseDuration(tt.input, 30*time.Second)
		if result != tt.expected {
			t.Errorf("parseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

