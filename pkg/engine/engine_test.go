package engine

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// errorStore wraps MemoryStore and can inject errors on specific operations.
type errorStore struct {
	*storage.MemoryStore
	listErr bool // return error on List
	getErr  bool // return error on Get
	saveErr bool // return error on Save
}

func (e *errorStore) List(ctx context.Context, prefix string) ([]string, error) {
	if e.listErr {
		return nil, fmt.Errorf("injected list error")
	}
	return e.MemoryStore.List(ctx, prefix)
}

func (e *errorStore) Get(ctx context.Context, key string, value interface{}) error {
	if e.getErr {
		return fmt.Errorf("injected get error")
	}
	return e.MemoryStore.Get(ctx, key, value)
}

func (e *errorStore) Save(ctx context.Context, key string, value interface{}) error {
	if e.saveErr {
		return fmt.Errorf("injected save error")
	}
	return e.MemoryStore.Save(ctx, key, value)
}

// saveOnlyErrorStore wraps MemoryStore and only fails on Save.
// List and Get work normally, which is useful for testing error paths
// that require data to be read before a Save fails.
type saveOnlyErrorStore struct {
	*storage.MemoryStore
}

func (s *saveOnlyErrorStore) Save(ctx context.Context, key string, value interface{}) error {
	return fmt.Errorf("injected save error")
}

// countingListErrorStore wraps MemoryStore and fails List after N successful calls.
type countingListErrorStore struct {
	*storage.MemoryStore
	failAfterN int
	count      int
}

func (c *countingListErrorStore) List(ctx context.Context, prefix string) ([]string, error) {
	c.count++
	if c.count > c.failAfterN {
		return nil, fmt.Errorf("injected list error after %d calls", c.failAfterN)
	}
	return c.MemoryStore.List(ctx, prefix)
}

// countingGetErrorStore wraps MemoryStore and fails Get after N successful calls.
type countingGetErrorStore struct {
	*storage.MemoryStore
	failAfterN int
	count      int
}

func (c *countingGetErrorStore) Get(ctx context.Context, key string, value interface{}) error {
	c.count++
	if c.count > c.failAfterN {
		return fmt.Errorf("injected get error after %d calls", c.failAfterN)
	}
	return c.MemoryStore.Get(ctx, key, value)
}

// countingSaveErrorStore wraps MemoryStore and fails Save after N successful calls.
type countingSaveErrorStore struct {
	*storage.MemoryStore
	failAfterN int
	count      int
}

func (c *countingSaveErrorStore) Save(ctx context.Context, key string, value interface{}) error {
	c.count++
	if c.count > c.failAfterN {
		return fmt.Errorf("injected save error after %d calls", c.failAfterN)
	}
	return c.MemoryStore.Save(ctx, key, value)
}

func TestNewAndClose(t *testing.T) {
	t.Run("creates engine with badger", func(t *testing.T) {
		tmpDir := t.TempDir()
		eng, err := New(&Options{
			StoreBackend: "badger",
			StoreAddress: tmpDir,
		})
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}
		if eng == nil {
			t.Fatal("expected non-nil engine")
		}
		if eng.store == nil {
			t.Error("expected non-nil store")
		}
		eng.Close()
	})

	t.Run("fails with invalid backend", func(t *testing.T) {
		_, err := New(&Options{
			StoreBackend: "invalid",
			StoreAddress: "localhost:1234",
		})
		if err == nil {
			t.Fatal("expected error for invalid backend")
		}
	})

	t.Run("close with nil store", func(t *testing.T) {
		eng := &Engine{}
		eng.Close() // Should not panic
	})
}

func TestDetermineEngineIP(t *testing.T) {
	t.Run("auto-detects non-loopback IP", func(t *testing.T) {
		ip, err := DetermineEngineIP()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip == "" {
			t.Error("expected non-empty IP")
		}
		if ip == "127.0.0.1" || ip == "0.0.0.0" {
			t.Errorf("expected non-loopback IP, got %s", ip)
		}
	})
}

func TestListAvailableAgents(t *testing.T) {
	ctx := context.Background()

	t.Run("returns ready agents", func(t *testing.T) {
		store := storage.NewMemoryStore()
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{Name: "agent-2", Status: "ready"})

		agents, err := ListAvailableAgents(ctx, store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(agents) != 2 {
			t.Errorf("expected 2 agents, got %d", len(agents))
		}
	})

	t.Run("filters non-ready agents", func(t *testing.T) {
		store := storage.NewMemoryStore()
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{Name: "agent-2", Status: "offline"})

		agents, err := ListAvailableAgents(ctx, store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(agents) != 1 {
			t.Errorf("expected 1 agent, got %d", len(agents))
		}
		if agents[0].Name != "agent-1" {
			t.Errorf("expected agent-1, got %s", agents[0].Name)
		}
	})

	t.Run("empty store returns nil", func(t *testing.T) {
		store := storage.NewMemoryStore()
		agents, err := ListAvailableAgents(ctx, store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(agents) != 0 {
			t.Errorf("expected 0 agents, got %d", len(agents))
		}
	})
}

func TestSchedulePendingDeployment(t *testing.T) {
	ctx := context.Background()

	t.Run("creates tasks and updates status", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		// Register an agent
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

		deployment := &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusPending,
			Services: map[string]types.ServiceRecord{
				"web": {Image: "nginx", Replicas: 2},
			},
			CreatedAt: time.Now(),
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.schedulePendingDeployment(ctx, deployment)

		// Verify deployment status changed to deploying
		var updated types.DeploymentRecord
		if err := store.Get(ctx, types.KeyDeployments+"deploy-1", &updated); err != nil {
			t.Fatalf("failed to get deployment: %v", err)
		}
		if updated.Status != types.StatusDeploying {
			t.Errorf("expected status deploying, got %s", updated.Status)
		}

		// Verify tasks were created
		taskKeys, _ := store.List(ctx, types.KeyTasks)
		if len(taskKeys) != 2 {
			t.Errorf("expected 2 tasks, got %d", len(taskKeys))
		}
	})

	t.Run("skips when no agents available", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		deployment := &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusPending,
			Services: map[string]types.ServiceRecord{
				"web": {Image: "nginx", Replicas: 1},
			},
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.schedulePendingDeployment(ctx, deployment)

		// Status should remain pending
		var updated types.DeploymentRecord
		if err := store.Get(ctx, types.KeyDeployments+"deploy-1", &updated); err != nil {
			t.Fatalf("failed to get deployment: %v", err)
		}
		if updated.Status != types.StatusPending {
			t.Errorf("expected status pending, got %s", updated.Status)
		}
	})
}

func TestCheckDeployingDeployment(t *testing.T) {
	ctx := context.Background()

	t.Run("all completed sets running", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-2", &types.TaskRecord{
			ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkDeployingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusRunning {
			t.Errorf("expected running, got %s", updated.Status)
		}
	})

	t.Run("any failed sets failed", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-2", &types.TaskRecord{
			ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusFailed, Error: "pull failed",
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkDeployingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusFailed {
			t.Errorf("expected failed, got %s", updated.Status)
		}
		if updated.Error == "" {
			t.Error("expected error message")
		}
	})

	t.Run("in-progress is noop", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-2", &types.TaskRecord{
			ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusRunning,
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkDeployingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusDeploying {
			t.Errorf("expected deploying (unchanged), got %s", updated.Status)
		}
	})
}

func TestCheckStoppingDeployment(t *testing.T) {
	ctx := context.Background()

	t.Run("all stop tasks completed sets stopped", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1-stop", &types.TaskRecord{
			ID: "task-1-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusStopping,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkStoppingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusStopped {
			t.Errorf("expected stopped, got %s", updated.Status)
		}
	})

	t.Run("any stop task failed sets failed", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1-stop", &types.TaskRecord{
			ID: "task-1-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeStopAndRemove, Status: types.StatusFailed, Error: "rm failed",
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusStopping,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkStoppingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusFailed {
			t.Errorf("expected failed, got %s", updated.Status)
		}
	})

	t.Run("no stop tasks is noop", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		// Only create_and_start tasks, no stop tasks
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "myapp", Status: types.StatusStopping,
		}
		store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

		eng.checkStoppingDeployment(ctx, deployment)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusStopping {
			t.Errorf("expected stopping (unchanged), got %s", updated.Status)
		}
	})
}

func TestCheckDeployingDeployment_MultipleAgents(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	eng := &Engine{store: store}

	// Two agents with tasks for the same deployment
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{Name: "agent-2", Status: "ready"})

	store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})
	store.Save(ctx, types.KeyTasks+"agent-2/task-2", &types.TaskRecord{
		ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-2",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})
	// Task for a different deployment (should be ignored)
	store.Save(ctx, types.KeyTasks+"agent-1/task-other", &types.TaskRecord{
		ID: "task-other", DeploymentID: "deploy-other", AgentID: "agent-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusFailed, Error: "other error",
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
	}
	store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	eng.checkDeployingDeployment(ctx, deployment)

	var updated types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
	if updated.Status != types.StatusRunning {
		t.Errorf("expected running, got %s", updated.Status)
	}
}

func TestCheckDeployingDeployment_NoNodes(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	eng := &Engine{store: store}

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
	}
	store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// No nodes in store — should be a noop
	eng.checkDeployingDeployment(ctx, deployment)

	var updated types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
	if updated.Status != types.StatusDeploying {
		t.Errorf("expected deploying (unchanged), got %s", updated.Status)
	}
}

func TestCheckStoppingDeployment_StillInProgress(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	eng := &Engine{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, types.KeyTasks+"agent-1/task-1-stop", &types.TaskRecord{
		ID: "task-1-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
	})
	store.Save(ctx, types.KeyTasks+"agent-1/task-2-stop", &types.TaskRecord{
		ID: "task-2-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusRunning,
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusStopping,
	}
	store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	eng.checkStoppingDeployment(ctx, deployment)

	var updated types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
	if updated.Status != types.StatusStopping {
		t.Errorf("expected stopping (in progress), got %s", updated.Status)
	}
}

func TestSchedulePendingDeployment_MultipleServices(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	eng := &Engine{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{Name: "agent-2", Status: "ready"})

	deployment := &types.DeploymentRecord{
		ID:     "deploy-multi",
		Name:   "multiapp",
		Status: types.StatusPending,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 2},
			"api": {Image: "node", Replicas: 1},
		},
		CreatedAt: time.Now(),
	}
	store.Save(ctx, types.KeyDeployments+"deploy-multi", deployment)

	eng.schedulePendingDeployment(ctx, deployment)

	var updated types.DeploymentRecord
	store.Get(ctx, types.KeyDeployments+"deploy-multi", &updated)
	if updated.Status != types.StatusDeploying {
		t.Errorf("expected deploying, got %s", updated.Status)
	}

	taskKeys, _ := store.List(ctx, types.KeyTasks)
	if len(taskKeys) != 3 {
		t.Errorf("expected 3 tasks (2 web replicas + 1 api), got %d", len(taskKeys))
	}
}

func TestProcessDeployments(t *testing.T) {
	ctx := context.Background()

	t.Run("routes pending to scheduling", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusPending,
			Services: map[string]types.ServiceRecord{
				"web": {Image: "nginx", Replicas: 1},
			},
		})

		eng.processDeployments(ctx)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusDeploying {
			t.Errorf("expected deploying, got %s", updated.Status)
		}
	})

	t.Run("routes deploying to check", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusDeploying,
		})

		eng.processDeployments(ctx)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusRunning {
			t.Errorf("expected running, got %s", updated.Status)
		}
	})

	t.Run("routes stopping to check", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1-stop", &types.TaskRecord{
			ID: "task-1-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
			Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusStopping,
		})

		eng.processDeployments(ctx)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusStopped {
			t.Errorf("expected stopped, got %s", updated.Status)
		}
	})

	t.Run("skips running deployments", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusRunning,
		})

		eng.processDeployments(ctx)

		var updated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &updated)
		if updated.Status != types.StatusRunning {
			t.Errorf("expected running (unchanged), got %s", updated.Status)
		}
	})

	t.Run("handles empty store", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}
		eng.processDeployments(ctx) // Should not panic
	})

	t.Run("processes multiple deployments in different states", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

		// Pending deployment
		store.Save(ctx, types.KeyDeployments+"deploy-pending", &types.DeploymentRecord{
			ID: "deploy-pending", Name: "pending-app", Status: types.StatusPending,
			Services: map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		})

		// Deploying deployment with all tasks completed
		store.Save(ctx, types.KeyDeployments+"deploy-deploying", &types.DeploymentRecord{
			ID: "deploy-deploying", Name: "deploying-app", Status: types.StatusDeploying,
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-dep", &types.TaskRecord{
			ID: "task-dep", DeploymentID: "deploy-deploying", AgentID: "agent-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})

		// Already running deployment (should be skipped)
		store.Save(ctx, types.KeyDeployments+"deploy-running", &types.DeploymentRecord{
			ID: "deploy-running", Name: "running-app", Status: types.StatusRunning,
		})

		eng.processDeployments(ctx)

		// Pending -> deploying
		var pendingUpdated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-pending", &pendingUpdated)
		if pendingUpdated.Status != types.StatusDeploying {
			t.Errorf("expected pending->deploying, got %s", pendingUpdated.Status)
		}

		// Deploying -> running
		var deployingUpdated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-deploying", &deployingUpdated)
		if deployingUpdated.Status != types.StatusRunning {
			t.Errorf("expected deploying->running, got %s", deployingUpdated.Status)
		}

		// Running -> still running
		var runningUpdated types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-running", &runningUpdated)
		if runningUpdated.Status != types.StatusRunning {
			t.Errorf("expected running (unchanged), got %s", runningUpdated.Status)
		}
	})

	t.Run("skips stopped and failed deployments", func(t *testing.T) {
		store := storage.NewMemoryStore()
		eng := &Engine{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-stopped", &types.DeploymentRecord{
			ID: "deploy-stopped", Name: "stopped-app", Status: types.StatusStopped,
		})
		store.Save(ctx, types.KeyDeployments+"deploy-failed", &types.DeploymentRecord{
			ID: "deploy-failed", Name: "failed-app", Status: types.StatusFailed,
		})

		eng.processDeployments(ctx)

		var stopped types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-stopped", &stopped)
		if stopped.Status != types.StatusStopped {
			t.Errorf("expected stopped (unchanged), got %s", stopped.Status)
		}

		var failed types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-failed", &failed)
		if failed.Status != types.StatusFailed {
			t.Errorf("expected failed (unchanged), got %s", failed.Status)
		}
	})
}

// --- Error path tests using errorStore ---

func TestProcessDeployments_ListError(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
	eng := &Engine{store: store}

	// Should not panic when List returns error
	eng.processDeployments(ctx)
}

func TestProcessDeployments_GetError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusPending,
	})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	eng := &Engine{store: store}

	// Should skip deployments that fail to deserialize
	eng.processDeployments(ctx)
}

func TestSchedulePendingDeployment_ListAgentsError(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
	eng := &Engine{store: store}

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusPending,
		Services: map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
	}

	// ListAvailableAgents returns error due to listErr, deployment should stay pending
	eng.schedulePendingDeployment(ctx, deployment)

	if deployment.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", deployment.Status)
	}
}

func TestSchedulePendingDeployment_SaveTaskError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusPending,
		Services: map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(),
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Enable save errors after seeding
	store := &errorStore{MemoryStore: memStore, saveErr: true}
	eng := &Engine{store: store}

	// Task save will fail, but the function should continue
	eng.schedulePendingDeployment(ctx, deployment)
}

func TestCheckDeployingDeployment_ListNodesError(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
	eng := &Engine{store: store}

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusDeploying,
	}

	// List error should cause early return
	eng.checkDeployingDeployment(ctx, deployment)

	if deployment.Status != types.StatusDeploying {
		t.Errorf("expected status deploying (unchanged), got %s", deployment.Status)
	}
}

func TestCheckDeployingDeployment_GetNodeError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	eng := &Engine{store: store}

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusDeploying,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Get error on nodes should skip them — DetermineDeploymentStatus returns "" (no tasks)
	eng.checkDeployingDeployment(ctx, deployment)

	if deployment.Status != types.StatusDeploying {
		t.Errorf("expected status deploying (unchanged), got %s", deployment.Status)
	}
}

func TestListAvailableAgents_ListError(t *testing.T) {
	ctx := context.Background()
	store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}

	_, err := ListAvailableAgents(ctx, store)
	if err == nil {
		t.Fatal("expected error when store.List fails")
	}
}

func TestListAvailableAgents_GetError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	store := &errorStore{MemoryStore: memStore, getErr: true}

	agents, err := ListAvailableAgents(ctx, store)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Get fails, so no agents returned
	if len(agents) != 0 {
		t.Errorf("expected 0 agents when Get fails, got %d", len(agents))
	}
}

func TestCheckDeployingDeployment_SaveError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	// Seed a node and a completed task
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Enable save errors — the final Save to update deployment status will fail
	store := &errorStore{MemoryStore: memStore, saveErr: true}
	eng := &Engine{store: store}

	eng.checkDeployingDeployment(ctx, deployment)

	// The function sets the deployment status in memory but fails to persist it.
	// Verify the in-memory deployment was updated but the store still has the old value.
	var stored types.DeploymentRecord
	memStore.Get(ctx, types.KeyDeployments+"deploy-1", &stored)
	if stored.Status != types.StatusDeploying {
		t.Errorf("expected store to retain deploying (save failed), got %s", stored.Status)
	}
}

func TestCheckDeployingDeployment_TaskGetError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	// Seed node and tasks
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Get succeeds for node (first call) but fails for task (second call)
	store := &countingGetErrorStore{MemoryStore: memStore, failAfterN: 1}
	eng := &Engine{store: store}

	eng.checkDeployingDeployment(ctx, deployment)

	// Task get fails, so 0 tasks counted -> DetermineDeploymentStatus returns "" -> noop
	if deployment.Status != types.StatusDeploying {
		t.Errorf("expected status deploying (unchanged), got %s", deployment.Status)
	}
}

func TestCheckDeployingDeployment_TaskListError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	// Seed a node
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "myapp", Status: types.StatusDeploying,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// List succeeds for nodes (first call) but fails for tasks (second call)
	store := &countingListErrorStore{MemoryStore: memStore, failAfterN: 1}
	eng := &Engine{store: store}

	eng.checkDeployingDeployment(ctx, deployment)

	// Task list fails, so 0 tasks counted -> noop
	if deployment.Status != types.StatusDeploying {
		t.Errorf("expected status deploying (unchanged), got %s", deployment.Status)
	}
}

// --- checkStoppingDeployment error paths ---

func TestCheckStoppingDeployment_SaveError_Stopped(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-stop", &types.TaskRecord{
		ID: "task-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusStopping,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Save fails — the stopped print should not occur but status is set in memory
	store := &saveOnlyErrorStore{MemoryStore: memStore}
	eng := &Engine{store: store}

	eng.checkStoppingDeployment(ctx, deployment)

	// Status was set in memory but store Save failed
	if deployment.Status != types.StatusStopped {
		t.Errorf("expected in-memory status stopped, got %s", deployment.Status)
	}
}

func TestCheckStoppingDeployment_SaveError_Failed(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-stop", &types.TaskRecord{
		ID: "task-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusFailed, Error: "rm failed",
	})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusStopping,
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	store := &saveOnlyErrorStore{MemoryStore: memStore}
	eng := &Engine{store: store}

	eng.checkStoppingDeployment(ctx, deployment)

	if deployment.Status != types.StatusFailed {
		t.Errorf("expected in-memory status failed, got %s", deployment.Status)
	}
}

func TestSchedulePendingDeployment_SaveDeploymentError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()

	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	deployment := &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusPending,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(),
	}
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Fail after 1 save (the task save succeeds, but deployment status save fails)
	store := &countingSaveErrorStore{MemoryStore: memStore, failAfterN: 1}
	eng := &Engine{store: store}

	eng.schedulePendingDeployment(ctx, deployment)

	// Deployment status was set in memory to deploying
	if deployment.Status != types.StatusDeploying {
		t.Errorf("expected in-memory status deploying, got %s", deployment.Status)
	}
}

// --- Tests for startRegistry ---

func TestStartRegistry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Use port 0 to let OS assign a random port
	lis, err := startRegistry(ctx, "0")
	if err != nil {
		t.Fatalf("startRegistry failed: %v", err)
	}
	if lis == nil {
		t.Fatal("expected non-nil listener")
	}

	addr := lis.Addr().String()
	if addr == "" {
		t.Error("expected non-empty listener address")
	}

	cancel()
}

func TestStartRegistry_InvalidPort(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := startRegistry(ctx, "99999")
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

// --- Tests for findNonLoopbackIPv4 ---

// mockAddr implements net.Addr but is NOT a *net.IPNet.
type mockAddr struct {
	network string
	str     string
}

func (m *mockAddr) Network() string { return m.network }
func (m *mockAddr) String() string  { return m.str }

func TestFindNonLoopbackIPv4(t *testing.T) {
	t.Run("returns first non-loopback IPv4", func(t *testing.T) {
		addrs := []net.Addr{
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("192.168.1.100"), Mask: net.CIDRMask(24, 32)},
		}
		ip, err := findNonLoopbackIPv4(addrs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "192.168.1.100" {
			t.Errorf("expected 192.168.1.100, got %s", ip)
		}
	})

	t.Run("skips non-IPNet addresses", func(t *testing.T) {
		addrs := []net.Addr{
			&mockAddr{network: "tcp", str: "not-an-ipnet"},
			&net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(8, 32)},
		}
		ip, err := findNonLoopbackIPv4(addrs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "10.0.0.1" {
			t.Errorf("expected 10.0.0.1, got %s", ip)
		}
	})

	t.Run("skips IPv6 addresses", func(t *testing.T) {
		addrs := []net.Addr{
			&net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
			&net.IPNet{IP: net.ParseIP("fe80::1"), Mask: net.CIDRMask(64, 128)},
			&net.IPNet{IP: net.ParseIP("172.16.0.1"), Mask: net.CIDRMask(12, 32)},
		}
		ip, err := findNonLoopbackIPv4(addrs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ip != "172.16.0.1" {
			t.Errorf("expected 172.16.0.1, got %s", ip)
		}
	})

	t.Run("returns error when no non-loopback IPv4 found", func(t *testing.T) {
		addrs := []net.Addr{
			&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			&net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
		}
		_, err := findNonLoopbackIPv4(addrs)
		if err == nil {
			t.Fatal("expected error when no non-loopback IPv4 found")
		}
	})

	t.Run("returns error for empty list", func(t *testing.T) {
		_, err := findNonLoopbackIPv4(nil)
		if err == nil {
			t.Fatal("expected error for nil addr list")
		}
	})

	t.Run("only non-IPNet addresses returns error", func(t *testing.T) {
		addrs := []net.Addr{
			&mockAddr{network: "tcp", str: "addr1"},
			&mockAddr{network: "udp", str: "addr2"},
		}
		_, err := findNonLoopbackIPv4(addrs)
		if err == nil {
			t.Fatal("expected error when only non-IPNet addresses present")
		}
	})
}
