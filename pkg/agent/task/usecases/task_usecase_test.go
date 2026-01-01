package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/task/domain"
	"github.com/fertile-org/banyan/pkg/agent/task/ports/outbound"
)

// MockContainerService is a mock implementation of ContainerService.
type MockContainerService struct {
	CreateContainerFunc  func(ctx context.Context, spec *outbound.ContainerSpec) (*outbound.ContainerInfo, error)
	StartContainerFunc   func(ctx context.Context, containerID string) error
	StopContainerFunc    func(ctx context.Context, containerID string, timeout time.Duration) error
	RemoveContainerFunc  func(ctx context.Context, containerID string, force bool) error
	RestartContainerFunc func(ctx context.Context, containerID string, timeout time.Duration) error
	GetContainerFunc     func(ctx context.Context, containerID string) (*outbound.ContainerInfo, error)
}

func (m *MockContainerService) CreateContainer(ctx context.Context, spec *outbound.ContainerSpec) (*outbound.ContainerInfo, error) {
	if m.CreateContainerFunc != nil {
		return m.CreateContainerFunc(ctx, spec)
	}
	return &outbound.ContainerInfo{ID: "test-container-1", Name: spec.Name, State: "created"}, nil
}

func (m *MockContainerService) StartContainer(ctx context.Context, containerID string) error {
	if m.StartContainerFunc != nil {
		return m.StartContainerFunc(ctx, containerID)
	}
	return nil
}

func (m *MockContainerService) StopContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	if m.StopContainerFunc != nil {
		return m.StopContainerFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockContainerService) RemoveContainer(ctx context.Context, containerID string, force bool) error {
	if m.RemoveContainerFunc != nil {
		return m.RemoveContainerFunc(ctx, containerID, force)
	}
	return nil
}

func (m *MockContainerService) RestartContainer(ctx context.Context, containerID string, timeout time.Duration) error {
	if m.RestartContainerFunc != nil {
		return m.RestartContainerFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *MockContainerService) GetContainer(ctx context.Context, containerID string) (*outbound.ContainerInfo, error) {
	if m.GetContainerFunc != nil {
		return m.GetContainerFunc(ctx, containerID)
	}
	return &outbound.ContainerInfo{ID: containerID, State: "running"}, nil
}

// MockNetworkService is a mock implementation of NetworkService.
type MockNetworkService struct {
	ConnectContainerFunc    func(ctx context.Context, containerID, networkID, ipAddress string) error
	DisconnectContainerFunc func(ctx context.Context, containerID, networkID string) error
	GetContainerNetworkFunc func(ctx context.Context, containerID string) (*outbound.ContainerNetworkInfo, error)
}

func (m *MockNetworkService) ConnectContainer(ctx context.Context, containerID, networkID, ipAddress string) error {
	if m.ConnectContainerFunc != nil {
		return m.ConnectContainerFunc(ctx, containerID, networkID, ipAddress)
	}
	return nil
}

func (m *MockNetworkService) DisconnectContainer(ctx context.Context, containerID, networkID string) error {
	if m.DisconnectContainerFunc != nil {
		return m.DisconnectContainerFunc(ctx, containerID, networkID)
	}
	return nil
}

func (m *MockNetworkService) GetContainerNetwork(ctx context.Context, containerID string) (*outbound.ContainerNetworkInfo, error) {
	if m.GetContainerNetworkFunc != nil {
		return m.GetContainerNetworkFunc(ctx, containerID)
	}
	return &outbound.ContainerNetworkInfo{ContainerID: containerID, IPAddress: "10.0.0.1"}, nil
}

// MockSecurityService is a mock implementation of SecurityService.
type MockSecurityService struct {
	ApplyPolicyFunc     func(ctx context.Context, policy *outbound.SecurityPolicy) error
	RemovePolicyFunc    func(ctx context.Context, policyID string) error
	GetPolicyStatusFunc func(ctx context.Context, policyID string) (*outbound.PolicyStatus, error)
}

func (m *MockSecurityService) ApplyPolicy(ctx context.Context, policy *outbound.SecurityPolicy) error {
	if m.ApplyPolicyFunc != nil {
		return m.ApplyPolicyFunc(ctx, policy)
	}
	return nil
}

func (m *MockSecurityService) RemovePolicy(ctx context.Context, policyID string) error {
	if m.RemovePolicyFunc != nil {
		return m.RemovePolicyFunc(ctx, policyID)
	}
	return nil
}

func (m *MockSecurityService) GetPolicyStatus(ctx context.Context, policyID string) (*outbound.PolicyStatus, error) {
	if m.GetPolicyStatusFunc != nil {
		return m.GetPolicyStatusFunc(ctx, policyID)
	}
	return &outbound.PolicyStatus{PolicyID: policyID, Applied: true}, nil
}

// MockHealthService is a mock implementation of HealthService.
type MockHealthService struct {
	RunHealthCheckFunc func(ctx context.Context, containerID, checkType, target string, timeout time.Duration) (*outbound.HealthCheckResult, error)
}

func (m *MockHealthService) RunHealthCheck(ctx context.Context, containerID, checkType, target string, timeout time.Duration) (*outbound.HealthCheckResult, error) {
	if m.RunHealthCheckFunc != nil {
		return m.RunHealthCheckFunc(ctx, containerID, checkType, target, timeout)
	}
	return &outbound.HealthCheckResult{Healthy: true, Message: "OK", Duration: 10 * time.Millisecond}, nil
}

// MockTaskStore is a mock implementation of TaskStore.
type MockTaskStore struct {
	Tasks map[domain.TaskID]*domain.Task
}

func NewMockTaskStore() *MockTaskStore {
	return &MockTaskStore{
		Tasks: make(map[domain.TaskID]*domain.Task),
	}
}

func (m *MockTaskStore) Save(_ context.Context, task *domain.Task) error {
	taskCopy := *task
	m.Tasks[task.ID] = &taskCopy
	return nil
}

func (m *MockTaskStore) Get(_ context.Context, taskID domain.TaskID) (*domain.Task, error) {
	task, ok := m.Tasks[taskID]
	if !ok {
		return nil, domain.ErrTaskNotFound
	}
	taskCopy := *task
	return &taskCopy, nil
}

func (m *MockTaskStore) Update(_ context.Context, task *domain.Task) error {
	if _, ok := m.Tasks[task.ID]; !ok {
		return domain.ErrTaskNotFound
	}
	taskCopy := *task
	m.Tasks[task.ID] = &taskCopy
	return nil
}

func (m *MockTaskStore) Delete(_ context.Context, taskID domain.TaskID) error {
	delete(m.Tasks, taskID)
	return nil
}

func (m *MockTaskStore) List(_ context.Context, filter *domain.TaskFilter) ([]*domain.Task, error) {
	var tasks []*domain.Task
	for _, task := range m.Tasks {
		if m.matchesFilter(task, filter) {
			taskCopy := *task
			tasks = append(tasks, &taskCopy)
		}
	}
	return tasks, nil
}

func (m *MockTaskStore) GetPending(_ context.Context) ([]*domain.Task, error) {
	var tasks []*domain.Task
	for _, task := range m.Tasks {
		if task.State == domain.TaskStatePending || task.State == domain.TaskStateQueued {
			taskCopy := *task
			tasks = append(tasks, &taskCopy)
		}
	}
	return tasks, nil
}

func (m *MockTaskStore) matchesFilter(task *domain.Task, filter *domain.TaskFilter) bool {
	if len(filter.States) > 0 {
		found := false
		for _, state := range filter.States {
			if task.State == state {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if task.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// MockEventEmitter is a mock implementation of EventEmitter.
type MockEventEmitter struct {
	Events []domain.TaskEvent
}

func NewMockEventEmitter() *MockEventEmitter {
	return &MockEventEmitter{
		Events: make([]domain.TaskEvent, 0),
	}
}

func (m *MockEventEmitter) Emit(event domain.TaskEvent) {
	m.Events = append(m.Events, event)
}

func (m *MockEventEmitter) Subscribe() <-chan domain.TaskEvent {
	ch := make(chan domain.TaskEvent, 100)
	return ch
}

func (m *MockEventEmitter) Unsubscribe(ch <-chan domain.TaskEvent) {
	// No-op for mock
}

// Ensure mocks implement interfaces
var (
	_ outbound.ContainerService = (*MockContainerService)(nil)
	_ outbound.NetworkService   = (*MockNetworkService)(nil)
	_ outbound.SecurityService  = (*MockSecurityService)(nil)
	_ outbound.HealthService    = (*MockHealthService)(nil)
	_ outbound.TaskStore        = (*MockTaskStore)(nil)
	_ outbound.EventEmitter     = (*MockEventEmitter)(nil)
)

func createTestUseCase() (*TaskUseCase, *MockTaskStore, *MockEventEmitter) {
	store := NewMockTaskStore()
	events := NewMockEventEmitter()
	container := &MockContainerService{}
	network := &MockNetworkService{}
	security := &MockSecurityService{}
	health := &MockHealthService{}

	uc := NewTaskUseCase(container, network, security, health, store, events)
	return uc, store, events
}

func TestSubmitTask_ContainerCreate(t *testing.T) {
	uc, store, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerCreate,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				Name:  "test-container",
				Image: "nginx:latest",
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}

	// Verify task was saved and completed
	saved, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if saved.State != domain.TaskStateCompleted {
		t.Errorf("expected state completed, got %s", saved.State)
	}
}

func TestSubmitTask_ContainerStart(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}
}

func TestSubmitTask_ContainerStop(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStop,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
				Timeout:     10 * time.Second,
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}
}

func TestSubmitTask_NetworkConnect(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeNetworkConnect,
		Payload: domain.TaskPayload{
			Network: &domain.NetworkTaskPayload{
				ContainerID: "container-1",
				NetworkID:   "network-1",
				IPAddress:   "10.0.0.5",
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}
}

func TestSubmitTask_SecurityApply(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeSecurityApply,
		Payload: domain.TaskPayload{
			Security: &domain.SecurityTaskPayload{
				PolicyID: "policy-1",
				Rules: []domain.SecurityRulePayload{
					{Direction: "ingress", Protocol: "tcp", Port: 80, Action: "allow"},
				},
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}
}

func TestSubmitTask_HealthCheck(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeHealthCheck,
		Payload: domain.TaskPayload{
			Health: &domain.HealthTaskPayload{
				ContainerID: "container-1",
				CheckType:   "http",
				Target:      "http://localhost:8080/health",
				Timeout:     5 * time.Second,
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure")
	}
}

func TestSubmitTask_InvalidPayload(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerCreate,
		// Missing Payload.Container
	}

	_, err := uc.SubmitTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestSubmitTask_UnknownTaskType(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: "unknown.task",
	}

	_, err := uc.SubmitTask(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for unknown task type")
	}
}

func TestSubmitTaskAsync(t *testing.T) {
	uc, store, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	taskID, err := uc.SubmitTaskAsync(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTaskAsync failed: %v", err)
	}

	if taskID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, taskID)
	}

	// Wait for async execution
	time.Sleep(100 * time.Millisecond)

	saved, err := store.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if saved.State != domain.TaskStateCompleted {
		t.Errorf("expected state completed, got %s", saved.State)
	}
}

func TestCancelTask(t *testing.T) {
	store := NewMockTaskStore()
	events := NewMockEventEmitter()
	container := &MockContainerService{
		StartContainerFunc: func(ctx context.Context, containerID string) error {
			// Simulate long-running task
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Second):
				return nil
			}
		},
	}

	uc := NewTaskUseCase(container, nil, nil, nil, store, events)

	task := &domain.Task{
		ID:      "task-1",
		Type:    domain.TaskTypeContainerStart,
		Timeout: 30 * time.Second,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	_, err := uc.SubmitTaskAsync(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTaskAsync failed: %v", err)
	}

	// Wait for task to start
	time.Sleep(50 * time.Millisecond)

	err = uc.CancelTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}

	// Verify state was updated
	cancelled, _ := store.Get(context.Background(), task.ID)
	if cancelled.State != domain.TaskStateCancelled {
		t.Errorf("expected state cancelled, got %s", cancelled.State)
	}
}

func TestCancelTask_NotFound(t *testing.T) {
	uc, _, _ := createTestUseCase()

	err := uc.CancelTask(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestGetTaskStatus(t *testing.T) {
	uc, store, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	_, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	status, err := uc.GetTaskStatus(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTaskStatus failed: %v", err)
	}

	if status.TaskID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, status.TaskID)
	}

	saved, _ := store.Get(context.Background(), task.ID)
	if status.State != saved.State {
		t.Errorf("expected state %s, got %s", saved.State, status.State)
	}
}

func TestGetTaskStatus_NotFound(t *testing.T) {
	uc, _, _ := createTestUseCase()

	_, err := uc.GetTaskStatus(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestGetTaskResult(t *testing.T) {
	uc, _, _ := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	retrievedResult, err := uc.GetTaskResult(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTaskResult failed: %v", err)
	}

	if retrievedResult.TaskID != result.TaskID {
		t.Errorf("expected task ID %s, got %s", result.TaskID, retrievedResult.TaskID)
	}
}

func TestGetTaskResult_NotCompleted(t *testing.T) {
	uc, store, _ := createTestUseCase()

	// Manually save a task that's still running
	task := &domain.Task{
		ID:    "task-1",
		Type:  domain.TaskTypeContainerStart,
		State: domain.TaskStateRunning,
	}
	store.Save(context.Background(), task)

	_, err := uc.GetTaskResult(context.Background(), task.ID)
	if err == nil {
		t.Fatal("expected error for non-completed task")
	}
}

func TestListTasks(t *testing.T) {
	uc, _, _ := createTestUseCase()

	// Submit multiple tasks
	for i := 1; i <= 3; i++ {
		task := &domain.Task{
			ID:   domain.TaskID("task-" + string(rune('0'+i))),
			Type: domain.TaskTypeContainerStart,
			Payload: domain.TaskPayload{
				Container: &domain.ContainerTaskPayload{
					ContainerID: "container-1",
				},
			},
		}
		if _, err := uc.SubmitTask(context.Background(), task); err != nil {
			t.Fatalf("SubmitTask failed: %v", err)
		}
	}

	tasks, err := uc.ListTasks(context.Background(), &domain.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestListTasks_WithFilter(t *testing.T) {
	uc, _, _ := createTestUseCase()

	// Submit tasks with different types
	tasks := []*domain.Task{
		{
			ID:   "task-1",
			Type: domain.TaskTypeContainerCreate,
			Payload: domain.TaskPayload{
				Container: &domain.ContainerTaskPayload{Name: "test", Image: "nginx"},
			},
		},
		{
			ID:   "task-2",
			Type: domain.TaskTypeNetworkConnect,
			Payload: domain.TaskPayload{
				Network: &domain.NetworkTaskPayload{ContainerID: "c1", NetworkID: "n1"},
			},
		},
	}

	for _, task := range tasks {
		if _, err := uc.SubmitTask(context.Background(), task); err != nil {
			t.Fatalf("SubmitTask failed: %v", err)
		}
	}

	filtered, err := uc.ListTasks(context.Background(), &domain.TaskFilter{
		Types: []domain.TaskType{domain.TaskTypeContainerCreate},
	})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("expected 1 container.create task, got %d", len(filtered))
	}
}

func TestTaskFailure(t *testing.T) {
	store := NewMockTaskStore()
	events := NewMockEventEmitter()
	container := &MockContainerService{
		StartContainerFunc: func(ctx context.Context, containerID string) error {
			return errors.New("container start failed")
		},
	}

	uc := NewTaskUseCase(container, nil, nil, nil, store, events)

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	result, err := uc.SubmitTask(context.Background(), task)

	// Should return an error
	if err == nil {
		t.Fatal("expected error for failed task")
	}

	// Result should indicate failure
	if result != nil && result.Success {
		t.Error("expected failure result")
	}

	// Task state should be failed
	saved, _ := store.Get(context.Background(), task.ID)
	if saved.State != domain.TaskStateFailed {
		t.Errorf("expected state failed, got %s", saved.State)
	}
}

func TestEventEmission(t *testing.T) {
	uc, _, events := createTestUseCase()

	task := &domain.Task{
		ID:   "task-1",
		Type: domain.TaskTypeContainerStart,
		Payload: domain.TaskPayload{
			Container: &domain.ContainerTaskPayload{
				ContainerID: "container-1",
			},
		},
	}

	_, err := uc.SubmitTask(context.Background(), task)
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	// Check events were emitted
	if len(events.Events) < 3 {
		t.Errorf("expected at least 3 events (queued, started, completed), got %d", len(events.Events))
	}

	// Verify event types
	eventTypes := make([]domain.TaskEventType, len(events.Events))
	for i, e := range events.Events {
		eventTypes[i] = e.EventType
	}

	expectedTypes := []domain.TaskEventType{
		domain.TaskEventQueued,
		domain.TaskEventStarted,
		domain.TaskEventCompleted,
	}

	for i, expected := range expectedTypes {
		if i >= len(eventTypes) {
			t.Errorf("missing expected event type %s", expected)
			continue
		}
		if eventTypes[i] != expected {
			t.Errorf("expected event type %s at position %d, got %s", expected, i, eventTypes[i])
		}
	}
}
