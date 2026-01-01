package adapters

import (
	"context"
	"runtime"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
)

// SystemMonitorAdapter provides system-level health metrics.
type SystemMonitorAdapter struct {
	startTime time.Time
}

// NewSystemMonitorAdapter creates a new system monitor adapter.
func NewSystemMonitorAdapter() *SystemMonitorAdapter {
	return &SystemMonitorAdapter{
		startTime: time.Now(),
	}
}

// GetCPUUsage returns the current CPU usage percentage.
// Note: This is a simplified implementation that uses runtime stats.
// For production use, consider using a proper metrics library.
func (m *SystemMonitorAdapter) GetCPUUsage(_ context.Context) (float64, error) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	// Return a placeholder CPU usage based on goroutines
	// In production, this should use proper CPU profiling
	numGoroutines := runtime.NumGoroutine()
	numCPU := runtime.NumCPU()

	// Simplified estimation
	usage := float64(numGoroutines) / float64(numCPU*100) * 100
	if usage > 100 {
		usage = 100
	}
	return usage, nil
}

// GetMemoryUsage returns the current memory usage in bytes.
func (m *SystemMonitorAdapter) GetMemoryUsage(_ context.Context) (uint64, error) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.Alloc, nil
}

// GetUptime returns the agent uptime in seconds.
func (m *SystemMonitorAdapter) GetUptime(_ context.Context) (int64, error) {
	return int64(time.Since(m.startTime).Seconds()), nil
}

// Ensure SystemMonitorAdapter implements SystemMonitor.
var _ outbound.SystemMonitor = (*SystemMonitorAdapter)(nil)
