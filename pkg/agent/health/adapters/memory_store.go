package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// MemoryCheckStore is an in-memory implementation of CheckStore.
type MemoryCheckStore struct {
	checks  map[domain.ContainerID]map[domain.CheckID]*domain.HealthCheck
	results map[domain.ContainerID]map[domain.CheckID]*domain.CheckResult
	mu      sync.RWMutex
}

// NewMemoryCheckStore creates a new in-memory check store.
func NewMemoryCheckStore() *MemoryCheckStore {
	return &MemoryCheckStore{
		checks:  make(map[domain.ContainerID]map[domain.CheckID]*domain.HealthCheck),
		results: make(map[domain.ContainerID]map[domain.CheckID]*domain.CheckResult),
	}
}

// SaveCheck saves a health check configuration.
func (s *MemoryCheckStore) SaveCheck(_ context.Context, check *domain.HealthCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.checks[check.ContainerID] == nil {
		s.checks[check.ContainerID] = make(map[domain.CheckID]*domain.HealthCheck)
	}
	s.checks[check.ContainerID][check.ID] = check
	return nil
}

// GetCheck retrieves a health check by ID.
func (s *MemoryCheckStore) GetCheck(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.HealthCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if container, ok := s.checks[containerID]; ok {
		if check, ok := container[checkID]; ok {
			return check, nil
		}
	}
	return nil, domain.ErrCheckNotFound
}

// DeleteCheck deletes a health check.
func (s *MemoryCheckStore) DeleteCheck(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if container, ok := s.checks[containerID]; ok {
		delete(container, checkID)
		if len(container) == 0 {
			delete(s.checks, containerID)
		}
	}

	if container, ok := s.results[containerID]; ok {
		delete(container, checkID)
		if len(container) == 0 {
			delete(s.results, containerID)
		}
	}

	return nil
}

// ListChecks lists all checks for a container.
func (s *MemoryCheckStore) ListChecks(_ context.Context, containerID domain.ContainerID) ([]*domain.HealthCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var checks []*domain.HealthCheck
	if container, ok := s.checks[containerID]; ok {
		for _, check := range container {
			checks = append(checks, check)
		}
	}
	return checks, nil
}

// SaveResult saves a check result.
func (s *MemoryCheckStore) SaveResult(_ context.Context, result *domain.CheckResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.results[result.ContainerID] == nil {
		s.results[result.ContainerID] = make(map[domain.CheckID]*domain.CheckResult)
	}
	s.results[result.ContainerID][result.CheckID] = result
	return nil
}

// GetLatestResult gets the most recent result for a check.
func (s *MemoryCheckStore) GetLatestResult(_ context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.CheckResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if container, ok := s.results[containerID]; ok {
		if result, ok := container[checkID]; ok {
			return result, nil
		}
	}
	return nil, domain.ErrCheckNotFound
}

// GetContainerHealth gets the aggregated health status for a container.
func (s *MemoryCheckStore) GetContainerHealth(_ context.Context, containerID domain.ContainerID) (*domain.ContainerHealth, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	checks, ok := s.checks[containerID]
	if !ok {
		return nil, domain.ErrContainerNotFound
	}

	health := &domain.ContainerHealth{
		ContainerID: containerID,
		Status:      domain.HealthStatusHealthy,
	}

	var checkList []domain.HealthCheck
	for _, check := range checks {
		checkList = append(checkList, *check)
	}
	health.Checks = checkList

	results, ok := s.results[containerID]
	if ok {
		var lastCheck time.Time
		for _, result := range results {
			if result.Timestamp.After(lastCheck) {
				lastCheck = result.Timestamp
			}
			if result.Status == domain.HealthStatusHealthy {
				health.SuccessCount++
				health.LastSuccess = result
			} else {
				health.FailCount++
				health.LastFailure = result
				health.Status = domain.HealthStatusUnhealthy
			}
		}
		health.LastCheck = lastCheck
	}

	return health, nil
}

// ListContainerHealths lists health for all containers.
func (s *MemoryCheckStore) ListContainerHealths(ctx context.Context) ([]*domain.ContainerHealth, error) {
	s.mu.RLock()
	containerIDs := make([]domain.ContainerID, 0, len(s.checks))
	for id := range s.checks {
		containerIDs = append(containerIDs, id)
	}
	s.mu.RUnlock()

	var healths []*domain.ContainerHealth
	for _, id := range containerIDs {
		health, err := s.GetContainerHealth(ctx, id)
		if err == nil {
			healths = append(healths, health)
		}
	}
	return healths, nil
}

// Ensure MemoryCheckStore implements CheckStore.
var _ outbound.CheckStore = (*MemoryCheckStore)(nil)
