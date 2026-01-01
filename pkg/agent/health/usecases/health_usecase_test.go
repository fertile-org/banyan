package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// MockHealthChecker is a mock implementation of HealthChecker.
type MockHealthChecker struct {
	SupportedType domain.CheckType
	CheckFunc     func(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error)
}

func (m *MockHealthChecker) Check(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error) {
	if m.CheckFunc != nil {
		return m.CheckFunc(ctx, check)
	}
	return &domain.CheckResult{
		CheckID:     check.ID,
		ContainerID: check.ContainerID,
		Status:      domain.HealthStatusHealthy,
		Output:      "mock check passed",
		Timestamp:   time.Now(),
	}, nil
}

func (m *MockHealthChecker) SupportsType(checkType domain.CheckType) bool {
	return checkType == m.SupportedType
}

// MockCheckStore is a mock implementation of CheckStore.
type MockCheckStore struct {
	Checks  map[domain.ContainerID]map[domain.CheckID]*domain.HealthCheck
	Results map[domain.ContainerID]map[domain.CheckID]*domain.CheckResult
}

func NewMockCheckStore() *MockCheckStore {
	return &MockCheckStore{
		Checks:  make(map[domain.ContainerID]map[domain.CheckID]*domain.HealthCheck),
		Results: make(map[domain.ContainerID]map[domain.CheckID]*domain.CheckResult),
	}
}

func (m *MockCheckStore) SaveCheck(_ context.Context, check *domain.HealthCheck) error {
	if m.Checks[check.ContainerID] == nil {
		m.Checks[check.ContainerID] = make(map[domain.CheckID]*domain.HealthCheck)
	}
	m.Checks[check.ContainerID][check.ID] = check
	return nil
}

func (m *MockCheckStore) GetCheck(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.HealthCheck, error) {
	if container, ok := m.Checks[containerID]; ok {
		if check, ok := container[checkID]; ok {
			return check, nil
		}
	}
	return nil, domain.ErrCheckNotFound
}

func (m *MockCheckStore) DeleteCheck(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) error {
	if container, ok := m.Checks[containerID]; ok {
		delete(container, checkID)
	}
	return nil
}

func (m *MockCheckStore) ListChecks(_ context.Context, containerID domain.ContainerID) ([]*domain.HealthCheck, error) {
	var checks []*domain.HealthCheck
	if container, ok := m.Checks[containerID]; ok {
		for _, check := range container {
			checks = append(checks, check)
		}
	}
	return checks, nil
}

func (m *MockCheckStore) SaveResult(_ context.Context, result *domain.CheckResult) error {
	if m.Results[result.ContainerID] == nil {
		m.Results[result.ContainerID] = make(map[domain.CheckID]*domain.CheckResult)
	}
	m.Results[result.ContainerID][result.CheckID] = result
	return nil
}

func (m *MockCheckStore) GetLatestResult(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.CheckResult, error) {
	if container, ok := m.Results[containerID]; ok {
		if result, ok := container[checkID]; ok {
			return result, nil
		}
	}
	return nil, domain.ErrCheckNotFound
}

func (m *MockCheckStore) GetContainerHealth(_ context.Context, containerID domain.ContainerID) (*domain.ContainerHealth, error) {
	if _, ok := m.Checks[containerID]; !ok {
		return nil, domain.ErrContainerNotFound
	}
	return &domain.ContainerHealth{
		ContainerID: containerID,
		Status:      domain.HealthStatusHealthy,
	}, nil
}

func (m *MockCheckStore) ListContainerHealths(_ context.Context) ([]*domain.ContainerHealth, error) {
	var healths []*domain.ContainerHealth
	for id := range m.Checks {
		healths = append(healths, &domain.ContainerHealth{
			ContainerID: id,
			Status:      domain.HealthStatusHealthy,
		})
	}
	return healths, nil
}

// MockSystemMonitor is a mock implementation of SystemMonitor.
type MockSystemMonitor struct {
	CPU    float64
	Memory uint64
	Uptime int64
}

func (m *MockSystemMonitor) GetCPUUsage(_ context.Context) (float64, error) {
	return m.CPU, nil
}

func (m *MockSystemMonitor) GetMemoryUsage(_ context.Context) (uint64, error) {
	return m.Memory, nil
}

func (m *MockSystemMonitor) GetUptime(_ context.Context) (int64, error) {
	return m.Uptime, nil
}

// Ensure mocks implement interfaces.
var (
	_ outbound.HealthChecker = (*MockHealthChecker)(nil)
	_ outbound.CheckStore    = (*MockCheckStore)(nil)
	_ outbound.SystemMonitor = (*MockSystemMonitor)(nil)
)

func TestRegisterCheck(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	// Verify check was saved
	saved, err := store.GetCheck(context.Background(), check.ContainerID, check.ID)
	if err != nil {
		t.Fatalf("GetCheck failed: %v", err)
	}
	if saved.Target != check.Target {
		t.Errorf("expected target %s, got %s", check.Target, saved.Target)
	}
}

func TestRegisterCheck_AlreadyExists(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	// Register once
	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("First RegisterCheck failed: %v", err)
	}

	// Try to register again
	err = uc.RegisterCheck(context.Background(), check)
	if !errors.Is(err, domain.ErrCheckAlreadyExists) {
		t.Errorf("expected ErrCheckAlreadyExists, got %v", err)
	}
}

func TestRegisterCheck_InvalidCheck(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	tests := []struct {
		name  string
		check *domain.HealthCheck
	}{
		{
			name:  "nil check",
			check: nil,
		},
		{
			name: "empty container ID",
			check: &domain.HealthCheck{
				ID:       "check-1",
				Target:   "http://localhost:8080/health",
				Interval: 30 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
		{
			name: "empty check ID",
			check: &domain.HealthCheck{
				ContainerID: "container-1",
				Target:      "http://localhost:8080/health",
				Interval:    30 * time.Second,
				Timeout:     5 * time.Second,
			},
		},
		{
			name: "empty target",
			check: &domain.HealthCheck{
				ID:          "check-1",
				ContainerID: "container-1",
				Interval:    30 * time.Second,
				Timeout:     5 * time.Second,
			},
		},
		{
			name: "zero interval",
			check: &domain.HealthCheck{
				ID:          "check-1",
				ContainerID: "container-1",
				Target:      "http://localhost:8080/health",
				Timeout:     5 * time.Second,
			},
		},
		{
			name: "zero timeout",
			check: &domain.HealthCheck{
				ID:          "check-1",
				ContainerID: "container-1",
				Target:      "http://localhost:8080/health",
				Interval:    30 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := uc.RegisterCheck(context.Background(), tt.check)
			if err == nil {
				t.Error("expected error for invalid check")
			}
		})
	}
}

func TestUnregisterCheck(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	// First register a check
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	// Unregister
	err = uc.UnregisterCheck(context.Background(), check.ContainerID, check.ID)
	if err != nil {
		t.Fatalf("UnregisterCheck failed: %v", err)
	}

	// Verify check was deleted
	_, err = store.GetCheck(context.Background(), check.ContainerID, check.ID)
	if !errors.Is(err, domain.ErrCheckNotFound) {
		t.Errorf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestUnregisterCheck_NotFound(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	err := uc.UnregisterCheck(context.Background(), "container-1", "check-1")
	if !errors.Is(err, domain.ErrCheckNotFound) {
		t.Errorf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestUnregisterCheck_InvalidContainerID(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	err := uc.UnregisterCheck(context.Background(), "", "check-1")
	if !errors.Is(err, domain.ErrInvalidContainerID) {
		t.Errorf("expected ErrInvalidContainerID, got %v", err)
	}
}

func TestGetContainerHealth(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	// Register a check first
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	health, err := uc.GetContainerHealth(context.Background(), check.ContainerID)
	if err != nil {
		t.Fatalf("GetContainerHealth failed: %v", err)
	}

	if health.ContainerID != check.ContainerID {
		t.Errorf("expected container ID %s, got %s", check.ContainerID, health.ContainerID)
	}
}

func TestGetContainerHealth_NotFound(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	_, err := uc.GetContainerHealth(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrContainerNotFound) {
		t.Errorf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestGetContainerHealth_InvalidContainerID(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	_, err := uc.GetContainerHealth(context.Background(), "")
	if !errors.Is(err, domain.ErrInvalidContainerID) {
		t.Errorf("expected ErrInvalidContainerID, got %v", err)
	}
}

func TestListContainerHealths(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	// Register checks for multiple containers
	for i := 1; i <= 3; i++ {
		check := &domain.HealthCheck{
			ID:          domain.CheckID("check-1"),
			ContainerID: domain.ContainerID("container-" + string(rune('0'+i))),
			Type:        domain.CheckTypeHTTP,
			Target:      "http://localhost:8080/health",
			Interval:    30 * time.Second,
			Timeout:     5 * time.Second,
		}
		if err := uc.RegisterCheck(context.Background(), check); err != nil {
			t.Fatalf("RegisterCheck failed: %v", err)
		}
	}

	healths, err := uc.ListContainerHealths(context.Background())
	if err != nil {
		t.Fatalf("ListContainerHealths failed: %v", err)
	}

	if len(healths) != 3 {
		t.Errorf("expected 3 container healths, got %d", len(healths))
	}
}

func TestRunCheck(t *testing.T) {
	store := NewMockCheckStore()
	checker := &MockHealthChecker{
		SupportedType: domain.CheckTypeHTTP,
	}
	uc := NewHealthUseCase([]outbound.HealthChecker{checker}, store, nil)

	// Register a check first
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	result, err := uc.RunCheck(context.Background(), check.ContainerID, check.ID)
	if err != nil {
		t.Fatalf("RunCheck failed: %v", err)
	}

	if result.Status != domain.HealthStatusHealthy {
		t.Errorf("expected status %s, got %s", domain.HealthStatusHealthy, result.Status)
	}
}

func TestRunCheck_CheckerError(t *testing.T) {
	store := NewMockCheckStore()
	checker := &MockHealthChecker{
		SupportedType: domain.CheckTypeHTTP,
		CheckFunc: func(_ context.Context, check *domain.HealthCheck) (*domain.CheckResult, error) {
			return nil, errors.New("connection refused")
		},
	}
	uc := NewHealthUseCase([]outbound.HealthChecker{checker}, store, nil)

	// Register a check first
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	result, err := uc.RunCheck(context.Background(), check.ContainerID, check.ID)
	if err != nil {
		t.Fatalf("RunCheck should not return error: %v", err)
	}

	if result.Status != domain.HealthStatusUnhealthy {
		t.Errorf("expected status %s, got %s", domain.HealthStatusUnhealthy, result.Status)
	}
}

func TestRunCheck_NoChecker(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil) // No checkers

	// Register a check first
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	result, err := uc.RunCheck(context.Background(), check.ContainerID, check.ID)
	if err != nil {
		t.Fatalf("RunCheck should not return error: %v", err)
	}

	if result.Status != domain.HealthStatusUnknown {
		t.Errorf("expected status %s, got %s", domain.HealthStatusUnknown, result.Status)
	}
}

func TestRunCheck_CheckNotFound(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	_, err := uc.RunCheck(context.Background(), "container-1", "check-1")
	if !errors.Is(err, domain.ErrCheckNotFound) {
		t.Errorf("expected ErrCheckNotFound, got %v", err)
	}
}

func TestRunCheck_InvalidContainerID(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil)

	_, err := uc.RunCheck(context.Background(), "", "check-1")
	if !errors.Is(err, domain.ErrInvalidContainerID) {
		t.Errorf("expected ErrInvalidContainerID, got %v", err)
	}
}

func TestGetAgentHealth(t *testing.T) {
	store := NewMockCheckStore()
	monitor := &MockSystemMonitor{
		CPU:    45.5,
		Memory: 1024 * 1024 * 512, // 512 MB
		Uptime: 3600,              // 1 hour
	}
	uc := NewHealthUseCase(nil, store, monitor)

	// Register a check to have some containers
	check := &domain.HealthCheck{
		ID:          "check-1",
		ContainerID: "container-1",
		Type:        domain.CheckTypeHTTP,
		Target:      "http://localhost:8080/health",
		Interval:    30 * time.Second,
		Timeout:     5 * time.Second,
	}

	err := uc.RegisterCheck(context.Background(), check)
	if err != nil {
		t.Fatalf("RegisterCheck failed: %v", err)
	}

	health, err := uc.GetAgentHealth(context.Background())
	if err != nil {
		t.Fatalf("GetAgentHealth failed: %v", err)
	}

	if !health.Healthy {
		t.Error("expected agent to be healthy")
	}

	if health.Status != domain.HealthStatusHealthy {
		t.Errorf("expected status %s, got %s", domain.HealthStatusHealthy, health.Status)
	}

	if health.CPU != 45.5 {
		t.Errorf("expected CPU 45.5, got %f", health.CPU)
	}

	if health.Memory != 1024*1024*512 {
		t.Errorf("expected memory 512MB, got %d", health.Memory)
	}

	if health.Containers != 1 {
		t.Errorf("expected 1 container, got %d", health.Containers)
	}
}

func TestGetAgentHealth_NoMonitor(t *testing.T) {
	store := NewMockCheckStore()
	uc := NewHealthUseCase(nil, store, nil) // No monitor

	health, err := uc.GetAgentHealth(context.Background())
	if err != nil {
		t.Fatalf("GetAgentHealth failed: %v", err)
	}

	if !health.Healthy {
		t.Error("expected agent to be healthy")
	}

	// CPU and Memory should be zero when no monitor
	if health.CPU != 0 {
		t.Errorf("expected CPU 0, got %f", health.CPU)
	}

	if health.Memory != 0 {
		t.Errorf("expected memory 0, got %d", health.Memory)
	}
}
