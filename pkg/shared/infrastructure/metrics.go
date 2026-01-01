package infrastructure

import (
	"sync"
	"time"
)

// MetricType represents the type of metric.
type MetricType int

const (
	MetricTypeCounter MetricType = iota
	MetricTypeGauge
	MetricTypeHistogram
)

// MetricsCollector defines the interface for collecting metrics.
type MetricsCollector interface {
	// Counter increments a counter metric.
	Counter(name string, value int64, labels ...Label)

	// Gauge sets a gauge metric.
	Gauge(name string, value float64, labels ...Label)

	// Histogram records a histogram observation.
	Histogram(name string, value float64, labels ...Label)

	// Timer starts a timer and returns a function to stop it.
	Timer(name string, labels ...Label) func()
}

// Label represents a metric label key-value pair.
type Label struct {
	Key   string
	Value string
}

// L creates a new Label.
func L(key, value string) Label {
	return Label{Key: key, Value: value}
}

// Common label constructors.
func LComponent(component string) Label { return Label{Key: "component", Value: component} }
func LOperation(op string) Label        { return Label{Key: "operation", Value: op} }
func LStatus(status string) Label       { return Label{Key: "status", Value: status} }
func LAgentID(id string) Label          { return Label{Key: "agent_id", Value: id} }
func LDeploymentID(id string) Label     { return Label{Key: "deployment_id", Value: id} }

// inMemoryMetrics implements MetricsCollector with in-memory storage.
// Useful for testing and development.
type inMemoryMetrics struct {
	counters   map[string]int64
	gauges     map[string]float64
	histograms map[string][]float64
	mu         sync.RWMutex
}

// NewInMemoryMetrics creates a new in-memory metrics collector.
func NewInMemoryMetrics() *inMemoryMetrics {
	return &inMemoryMetrics{
		counters:   make(map[string]int64),
		gauges:     make(map[string]float64),
		histograms: make(map[string][]float64),
	}
}

func (m *inMemoryMetrics) Counter(name string, value int64, labels ...Label) {
	key := m.buildKey(name, labels)
	m.mu.Lock()
	m.counters[key] += value
	m.mu.Unlock()
}

func (m *inMemoryMetrics) Gauge(name string, value float64, labels ...Label) {
	key := m.buildKey(name, labels)
	m.mu.Lock()
	m.gauges[key] = value
	m.mu.Unlock()
}

func (m *inMemoryMetrics) Histogram(name string, value float64, labels ...Label) {
	key := m.buildKey(name, labels)
	m.mu.Lock()
	m.histograms[key] = append(m.histograms[key], value)
	m.mu.Unlock()
}

func (m *inMemoryMetrics) Timer(name string, labels ...Label) func() {
	start := time.Now()
	return func() {
		elapsed := time.Since(start).Seconds()
		m.Histogram(name, elapsed, labels...)
	}
}

func (m *inMemoryMetrics) buildKey(name string, labels []Label) string {
	key := name
	for _, l := range labels {
		key += "," + l.Key + "=" + l.Value
	}
	return key
}

// GetCounter returns the current value of a counter (for testing).
func (m *inMemoryMetrics) GetCounter(name string, labels ...Label) int64 {
	key := m.buildKey(name, labels)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.counters[key]
}

// GetGauge returns the current value of a gauge (for testing).
func (m *inMemoryMetrics) GetGauge(name string, labels ...Label) float64 {
	key := m.buildKey(name, labels)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gauges[key]
}

// GetHistogram returns all observations for a histogram (for testing).
func (m *inMemoryMetrics) GetHistogram(name string, labels ...Label) []float64 {
	key := m.buildKey(name, labels)
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]float64, len(m.histograms[key]))
	copy(result, m.histograms[key])
	return result
}

// Reset clears all metrics (for testing).
func (m *inMemoryMetrics) Reset() {
	m.mu.Lock()
	m.counters = make(map[string]int64)
	m.gauges = make(map[string]float64)
	m.histograms = make(map[string][]float64)
	m.mu.Unlock()
}

// NopMetrics is a no-operation metrics collector.
type NopMetrics struct{}

func (NopMetrics) Counter(name string, value int64, labels ...Label)     {}
func (NopMetrics) Gauge(name string, value float64, labels ...Label)     {}
func (NopMetrics) Histogram(name string, value float64, labels ...Label) {}
func (NopMetrics) Timer(name string, labels ...Label) func()             { return func() {} }

// Common metric names for consistency across components.
const (
	// Deployment metrics
	MetricDeploymentsTotal   = "banyan_deployments_total"
	MetricDeploymentDuration = "banyan_deployment_duration_seconds"
	MetricDeploymentsActive  = "banyan_deployments_active"

	// Task metrics
	MetricTasksTotal   = "banyan_tasks_total"
	MetricTaskDuration = "banyan_task_duration_seconds"
	MetricTasksQueued  = "banyan_tasks_queued"
	MetricTaskRetries  = "banyan_task_retries_total"

	// Agent metrics
	MetricAgentsTotal     = "banyan_agents_total"
	MetricAgentsHealthy   = "banyan_agents_healthy"
	MetricAgentHeartbeats = "banyan_agent_heartbeats_total"

	// Container metrics
	MetricContainersTotal   = "banyan_containers_total"
	MetricContainersRunning = "banyan_containers_running"
	MetricContainerRestarts = "banyan_container_restarts_total"

	// Network metrics
	MetricIPAllocations     = "banyan_ip_allocations_total"
	MetricNetworkOperations = "banyan_network_operations_total"

	// Plugin metrics
	MetricPluginExecutions = "banyan_plugin_executions_total"
	MetricPluginDuration   = "banyan_plugin_duration_seconds"

	// Health metrics
	MetricHealthChecks       = "banyan_health_checks_total"
	MetricHealthCheckLatency = "banyan_health_check_latency_seconds"

	// Error metrics
	MetricErrors = "banyan_errors_total"
)
