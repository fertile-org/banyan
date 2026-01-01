package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
)

func TestMemoryTaskStore_SaveAndGet(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	task := &domain.Task{
		ID:        "task-1",
		Type:      domain.TaskTypeContainerCreate,
		State:     domain.TaskStatePending,
		CreatedAt: time.Now(),
	}

	err := store.Save(ctx, task)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	retrieved, err := store.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != task.ID {
		t.Errorf("expected ID %s, got %s", task.ID, retrieved.ID)
	}
}

func TestMemoryTaskStore_GetNotFound(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	if err != domain.ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestMemoryTaskStore_Update(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	task := &domain.Task{
		ID:    "task-1",
		Type:  domain.TaskTypeContainerCreate,
		State: domain.TaskStatePending,
	}

	err := store.Save(ctx, task)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	task.State = domain.TaskStateQueued
	err = store.Update(ctx, task)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	retrieved, _ := store.Get(ctx, task.ID)
	if retrieved.State != domain.TaskStateQueued {
		t.Errorf("expected state queued, got %s", retrieved.State)
	}
}

func TestMemoryTaskStore_UpdateNotFound(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	task := &domain.Task{ID: "nonexistent"}
	err := store.Update(ctx, task)
	if err != domain.ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestMemoryTaskStore_Delete(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	task := &domain.Task{ID: "task-1", Type: domain.TaskTypeContainerCreate}
	_ = store.Save(ctx, task)

	err := store.Delete(ctx, task.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.Get(ctx, task.ID)
	if err != domain.ErrTaskNotFound {
		t.Errorf("expected ErrTaskNotFound after delete, got %v", err)
	}
}

func TestMemoryTaskStore_List(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	tasks := []*domain.Task{
		{ID: "task-1", Type: domain.TaskTypeContainerCreate, State: domain.TaskStateCompleted, CreatedAt: time.Now()},
		{ID: "task-2", Type: domain.TaskTypeNetworkConnect, State: domain.TaskStatePending, CreatedAt: time.Now()},
		{ID: "task-3", Type: domain.TaskTypeContainerCreate, State: domain.TaskStateFailed, CreatedAt: time.Now()},
	}

	for _, task := range tasks {
		_ = store.Save(ctx, task)
	}

	t.Run("list all", func(t *testing.T) {
		result, err := store.List(ctx, &domain.TaskFilter{})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(result))
		}
	})

	t.Run("filter by state", func(t *testing.T) {
		result, err := store.List(ctx, &domain.TaskFilter{
			States: []domain.TaskState{domain.TaskStateCompleted},
		})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(result) != 1 {
			t.Errorf("expected 1 completed task, got %d", len(result))
		}
	})

	t.Run("filter by type", func(t *testing.T) {
		result, err := store.List(ctx, &domain.TaskFilter{
			Types: []domain.TaskType{domain.TaskTypeContainerCreate},
		})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 container.create tasks, got %d", len(result))
		}
	})

	t.Run("pagination", func(t *testing.T) {
		result, err := store.List(ctx, &domain.TaskFilter{Limit: 2})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(result) != 2 {
			t.Errorf("expected 2 tasks with limit, got %d", len(result))
		}
	})
}

func TestMemoryTaskStore_GetPending(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	tasks := []*domain.Task{
		{ID: "task-1", State: domain.TaskStatePending},
		{ID: "task-2", State: domain.TaskStateQueued},
		{ID: "task-3", State: domain.TaskStateRunning},
		{ID: "task-4", State: domain.TaskStateCompleted},
	}

	for _, task := range tasks {
		_ = store.Save(ctx, task)
	}

	result, err := store.GetPending(ctx)
	if err != nil {
		t.Fatalf("GetPending failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 pending tasks, got %d", len(result))
	}
}

func TestMemoryTaskStore_IsolatesCopies(t *testing.T) {
	store := NewMemoryTaskStoreAdapter()
	ctx := context.Background()

	task := &domain.Task{
		ID:    "task-1",
		Type:  domain.TaskTypeContainerCreate,
		State: domain.TaskStatePending,
	}

	_ = store.Save(ctx, task)

	// Modify the original
	task.State = domain.TaskStateRunning

	// Retrieved should still be pending
	retrieved, _ := store.Get(ctx, task.ID)
	if retrieved.State != domain.TaskStatePending {
		t.Error("store should return copies, not references")
	}
}
