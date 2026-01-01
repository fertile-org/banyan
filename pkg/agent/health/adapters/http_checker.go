package adapters

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/domain"
	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// HTTPChecker implements HTTP health checks.
type HTTPChecker struct {
	client *http.Client
}

// NewHTTPChecker creates a new HTTP health checker.
func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Check executes an HTTP health check.
func (c *HTTPChecker) Check(ctx context.Context, check *domain.HealthCheck) (*domain.CheckResult, error) {
	start := time.Now()

	result := &domain.CheckResult{
		CheckID:     check.ID,
		ContainerID: check.ContainerID,
		Timestamp:   start,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.Target, http.NoBody)
	if err != nil {
		result.Status = domain.HealthStatusUnhealthy
		result.Output = fmt.Sprintf("failed to create request: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	resp, err := c.client.Do(req)
	if err != nil {
		result.Status = domain.HealthStatusUnhealthy
		result.Output = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}
	defer resp.Body.Close()

	result.Duration = time.Since(start)

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		result.Status = domain.HealthStatusHealthy
		result.Output = fmt.Sprintf("HTTP %d", resp.StatusCode)
	} else {
		result.Status = domain.HealthStatusUnhealthy
		result.Output = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return result, nil
}

// SupportsType returns true if this checker supports HTTP checks.
func (c *HTTPChecker) SupportsType(checkType domain.CheckType) bool {
	return checkType == domain.CheckTypeHTTP
}

// Ensure HTTPChecker implements HealthChecker.
var _ outbound.HealthChecker = (*HTTPChecker)(nil)
