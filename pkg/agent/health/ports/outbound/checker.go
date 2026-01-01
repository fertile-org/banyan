package outbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
)

// HealthChecker executes health checks.
type HealthChecker interface {
	// Check executes a health check and returns the result.
	Check(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error)

	// SupportsType returns true if this checker supports the given check type.
	SupportsType(checkType domain.CheckType) bool
}

// CheckStore persists health check configurations and results.
type CheckStore interface {
	// SaveCheck saves a health check configuration.
	SaveCheck(ctx context.Context, check *domain.HealthCheck) error

	// GetCheck retrieves a health check by ID.
	GetCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.HealthCheck, error)

	// DeleteCheck deletes a health check.
	DeleteCheck(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) error

	// ListChecks lists all checks for a container.
	ListChecks(ctx context.Context, containerID domain.ContainerID) ([]*domain.HealthCheck, error)

	// SaveResult saves a check result.
	SaveResult(ctx context.Context, result *domain.CheckResult) error

	// GetLatestResult gets the most recent result for a check.
	GetLatestResult(ctx context.Context, containerID domain.ContainerID, checkID domain.CheckID) (*domain.CheckResult, error)

	// GetContainerHealth gets the aggregated health status for a container.
	GetContainerHealth(ctx context.Context, containerID domain.ContainerID) (*domain.ContainerHealth, error)

	// ListContainerHealths lists health for all containers.
	ListContainerHealths(ctx context.Context) ([]*domain.ContainerHealth, error)
}

// SystemMonitor provides system-level health metrics.
type SystemMonitor interface {
	// GetCPUUsage returns the current CPU usage percentage.
	GetCPUUsage(ctx context.Context) (float64, error)

	// GetMemoryUsage returns the current memory usage in bytes.
	GetMemoryUsage(ctx context.Context) (uint64, error)

	// GetUptime returns the agent uptime.
	GetUptime(ctx context.Context) (int64, error)
}
