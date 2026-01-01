package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// HealthUseCase implements the HealthService interface.
type HealthUseCase struct {
	startTime     time.Time
	checkStore    outbound.CheckStore
	systemMonitor outbound.SystemMonitor
	checkers      []outbound.HealthChecker
}

// NewHealthUseCase creates a new HealthUseCase.
func NewHealthUseCase(
	checkers []outbound.HealthChecker,
	checkStore outbound.CheckStore,
	systemMonitor outbound.SystemMonitor,
) *HealthUseCase {
	return &HealthUseCase{
		startTime:     time.Now(),
		checkers:      checkers,
		checkStore:    checkStore,
		systemMonitor: systemMonitor,
	}
}

// RegisterCheck registers a health check for a container.
func (uc *HealthUseCase) RegisterCheck(ctx context.Context, check *domain.HealthCheck) error {
	if err := uc.validateCheck(check); err != nil {
		return err
	}

	// Check if already exists
	existing, err := uc.checkStore.GetCheck(ctx, check.ContainerID, check.ID)
	if err == nil && existing != nil {
		return domain.ErrCheckAlreadyExists
	}

	// Save check
	if err := uc.checkStore.SaveCheck(ctx, check); err != nil {
		return fmt.Errorf("failed to save check: %w", err)
	}

	return nil
}

// UnregisterCheck removes a health check.
func (uc *HealthUseCase) UnregisterCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) error {
	if containerID == "" {
		return domain.ErrInvalidContainerID
	}

	// Check if exists
	_, err := uc.checkStore.GetCheck(ctx, containerID, checkID)
	if err != nil {
		return domain.ErrCheckNotFound
	}

	if err := uc.checkStore.DeleteCheck(ctx, containerID, checkID); err != nil {
		return fmt.Errorf("failed to delete check: %w", err)
	}

	return nil
}

// GetContainerHealth gets the health status of a container.
func (uc *HealthUseCase) GetContainerHealth(ctx context.Context, containerID domain.ContainerID) (*domain.ContainerHealth, error) {
	if containerID == "" {
		return nil, domain.ErrInvalidContainerID
	}

	health, err := uc.checkStore.GetContainerHealth(ctx, containerID)
	if err != nil {
		return nil, domain.ErrContainerNotFound
	}

	return health, nil
}

// ListContainerHealths lists health status for all containers.
func (uc *HealthUseCase) ListContainerHealths(ctx context.Context) ([]*domain.ContainerHealth, error) {
	return uc.checkStore.ListContainerHealths(ctx)
}

// RunCheck manually runs a health check and returns the result.
func (uc *HealthUseCase) RunCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.CheckResult, error) {
	if containerID == "" {
		return nil, domain.ErrInvalidContainerID
	}

	check, err := uc.checkStore.GetCheck(ctx, containerID, checkID)
	if err != nil {
		return nil, domain.ErrCheckNotFound
	}

	// Find appropriate checker
	var checker outbound.HealthChecker
	for _, c := range uc.checkers {
		if c.SupportsType(check.Type) {
			checker = c
			break
		}
	}

	if checker == nil {
		return &domain.CheckResult{
			CheckID:     checkID,
			ContainerID: containerID,
			Status:      domain.HealthStatusUnknown,
			Output:      fmt.Sprintf("no checker available for type %s", check.Type),
			Timestamp:   time.Now(),
		}, nil
	}

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	// Run check
	result, err := checker.Check(checkCtx, check)
	if err != nil {
		result = &domain.CheckResult{
			CheckID:     checkID,
			ContainerID: containerID,
			Status:      domain.HealthStatusUnhealthy,
			Output:      err.Error(),
			Timestamp:   time.Now(),
		}
	}

	// Save result
	if saveErr := uc.checkStore.SaveResult(ctx, result); saveErr != nil {
		// Log but don't fail - the check itself succeeded/failed
		_ = saveErr
	}

	return result, nil
}

// GetAgentHealth gets the health status of the agent itself.
func (uc *HealthUseCase) GetAgentHealth(ctx context.Context) (*domain.AgentHealth, error) {
	health := &domain.AgentHealth{
		Healthy:       true,
		Status:        domain.HealthStatusHealthy,
		Uptime:        time.Since(uc.startTime),
		LastHeartbeat: time.Now(),
		Components:    make(map[string]ComponentHealth),
	}

	// Get system metrics if monitor is available
	if uc.systemMonitor != nil {
		cpu, err := uc.systemMonitor.GetCPUUsage(ctx)
		if err == nil {
			health.CPU = cpu
		}

		mem, err := uc.systemMonitor.GetMemoryUsage(ctx)
		if err == nil {
			health.Memory = mem
		}
	}

	// Count managed containers
	healths, err := uc.checkStore.ListContainerHealths(ctx)
	if err == nil {
		health.Containers = len(healths)
	}

	return health, nil
}

// validateCheck validates a health check configuration.
func (uc *HealthUseCase) validateCheck(check *domain.HealthCheck) error {
	if check == nil {
		return domain.ErrInvalidCheck
	}

	if check.ContainerID == "" {
		return domain.ErrInvalidContainerID
	}

	if check.ID == "" {
		return domain.ErrInvalidCheck
	}

	if check.Target == "" {
		return domain.ErrInvalidCheck
	}

	if check.Interval <= 0 {
		return domain.ErrInvalidCheck
	}

	if check.Timeout <= 0 {
		return domain.ErrInvalidCheck
	}

	return nil
}

// ComponentHealth is a type alias for domain.ComponentHealth used in AgentHealth.
type ComponentHealth = domain.ComponentHealth

// Ensure HealthUseCase implements inbound.HealthService
var _ inbound.HealthService = (*HealthUseCase)(nil)
