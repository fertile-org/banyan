package adapters

import (
	"bytes"
	"context"
	"os/exec"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// ExecChecker implements command execution health checks.
type ExecChecker struct{}

// NewExecChecker creates a new exec health checker.
func NewExecChecker() *ExecChecker {
	return &ExecChecker{}
}

// Check executes a command-based health check.
func (c *ExecChecker) Check(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error) {
	start := time.Now()

	result := &domain.CheckResult{
		CheckID:     check.ID,
		ContainerID: check.ContainerID,
		Timestamp:   start,
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", check.Target) //nolint:gosec // Target is validated and controlled by the health check configuration
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(start)

	if err != nil {
		result.Status = domain.HealthStatusUnhealthy
		if stderr.Len() > 0 {
			result.Output = stderr.String()
		} else {
			result.Output = err.Error()
		}
		return result, nil
	}

	result.Status = domain.HealthStatusHealthy
	result.Output = stdout.String()
	if len(result.Output) > 1024 {
		result.Output = result.Output[:1024] + "..."
	}

	return result, nil
}

// SupportsType returns true if this checker supports exec checks.
func (c *ExecChecker) SupportsType(checkType domain.CheckType) bool {
	return checkType == domain.CheckTypeExec
}

// Ensure ExecChecker implements HealthChecker.
var _ outbound.HealthChecker = (*ExecChecker)(nil)
