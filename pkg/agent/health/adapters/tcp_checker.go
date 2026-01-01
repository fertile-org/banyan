package adapters

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// TCPChecker implements TCP health checks.
type TCPChecker struct {
	dialer *net.Dialer
}

// NewTCPChecker creates a new TCP health checker.
func NewTCPChecker() *TCPChecker {
	return &TCPChecker{
		dialer: &net.Dialer{
			Timeout: 30 * time.Second,
		},
	}
}

// Check executes a TCP health check.
func (c *TCPChecker) Check(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error) {
	start := time.Now()

	result := &domain.CheckResult{
		CheckID:     check.ID,
		ContainerID: check.ContainerID,
		Timestamp:   start,
	}

	conn, err := c.dialer.DialContext(ctx, "tcp", check.Target)
	if err != nil {
		result.Status = domain.HealthStatusUnhealthy
		result.Output = fmt.Sprintf("connection failed: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}
	defer conn.Close()

	result.Duration = time.Since(start)
	result.Status = domain.HealthStatusHealthy
	result.Output = fmt.Sprintf("connected to %s", check.Target)

	return result, nil
}

// SupportsType returns true if this checker supports TCP checks.
func (c *TCPChecker) SupportsType(checkType domain.CheckType) bool {
	return checkType == domain.CheckTypeTCP
}

// Ensure TCPChecker implements HealthChecker.
var _ outbound.HealthChecker = (*TCPChecker)(nil)
