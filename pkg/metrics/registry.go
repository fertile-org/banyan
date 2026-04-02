package metrics

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EngineMetricsRegistry defines and manages all Prometheus metrics for the Banyan engine.
// It provides update methods called by the engine's gRPC handlers and orchestration loop.
type EngineMetricsRegistry struct {
	registry *prometheus.Registry

	// Engine system metrics
	engineUptime    prometheus.Gauge
	engineCPU       prometheus.Gauge
	engineMemUsed   prometheus.Gauge
	engineMemTotal  prometheus.Gauge
	engineDiskUsed  prometheus.Gauge
	engineDiskTotal prometheus.Gauge
	engineCPUCores  prometheus.Gauge

	// Cluster aggregate metrics
	clusterAgentsTotal       prometheus.Gauge
	clusterAgentsConnected   prometheus.Gauge
	clusterDeployments       *prometheus.GaugeVec
	clusterContainersTotal   prometheus.Gauge
	clusterContainersHealthy prometheus.Gauge
	clusterTasks             *prometheus.GaugeVec

	// Per-agent metrics (labeled by agent name)
	agentCPU        *prometheus.GaugeVec
	agentMemUsed    *prometheus.GaugeVec
	agentMemTotal   *prometheus.GaugeVec
	agentDiskUsed   *prometheus.GaugeVec
	agentDiskTotal  *prometheus.GaugeVec
	agentCPUCores   *prometheus.GaugeVec
	agentContainers *prometheus.GaugeVec
	agentInfo       *prometheus.GaugeVec

	// Per-deployment metrics
	deployReplicasDesired *prometheus.GaugeVec
	deployReplicasHealthy *prometheus.GaugeVec
	deployInfo            *prometheus.GaugeVec

	// Event counters
	eventsTotal *prometheus.CounterVec

	// Reconciliation counters
	reconcileRunsTotal    *prometheus.CounterVec
	reconcileActionsTotal *prometheus.CounterVec

	// Internal storage for dashboard data access (parallel to prometheus gauges)
	engineMetrics atomic.Pointer[SystemMetrics]
	agentMetrics  sync.Map // map[string]*SystemMetrics
}

// NewEngineMetricsRegistry creates a new registry with all Banyan metrics defined.
func NewEngineMetricsRegistry() *EngineMetricsRegistry {
	reg := prometheus.NewRegistry()

	r := &EngineMetricsRegistry{
		registry: reg,

		// Engine system
		engineUptime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_uptime_seconds",
			Help: "Seconds since engine started.",
		}),
		engineCPU: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_cpu_usage_ratio",
			Help: "Engine host CPU usage (0.0-1.0).",
		}),
		engineMemUsed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_memory_used_bytes",
			Help: "Engine host memory in use.",
		}),
		engineMemTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_memory_total_bytes",
			Help: "Engine host total memory.",
		}),
		engineDiskUsed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_disk_used_bytes",
			Help: "Engine host disk usage.",
		}),
		engineDiskTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_disk_total_bytes",
			Help: "Engine host total disk.",
		}),
		engineCPUCores: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_engine_cpu_cores",
			Help: "Engine host CPU core count.",
		}),

		// Cluster
		clusterAgentsTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_cluster_agents_total",
			Help: "Total registered agents.",
		}),
		clusterAgentsConnected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_cluster_agents_connected",
			Help: "Currently connected agents.",
		}),
		clusterDeployments: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_cluster_deployments_total",
			Help: "Total deployments by status.",
		}, []string{"status"}),
		clusterContainersTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_cluster_containers_total",
			Help: "Total containers across all agents.",
		}),
		clusterContainersHealthy: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "banyan_cluster_containers_healthy",
			Help: "Currently healthy containers.",
		}),
		clusterTasks: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_cluster_tasks_total",
			Help: "Total tasks by status.",
		}, []string{"status"}),

		// Per-agent
		agentCPU: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_cpu_usage_ratio",
			Help: "Agent CPU usage (0.0-1.0).",
		}, []string{"agent"}),
		agentMemUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_memory_used_bytes",
			Help: "Agent memory in use.",
		}, []string{"agent"}),
		agentMemTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_memory_total_bytes",
			Help: "Agent total memory.",
		}, []string{"agent"}),
		agentDiskUsed: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_disk_used_bytes",
			Help: "Agent disk usage.",
		}, []string{"agent"}),
		agentDiskTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_disk_total_bytes",
			Help: "Agent total disk.",
		}, []string{"agent"}),
		agentCPUCores: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_cpu_cores",
			Help: "Agent CPU core count.",
		}, []string{"agent"}),
		agentContainers: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_containers_total",
			Help: "Containers on this agent.",
		}, []string{"agent"}),
		agentInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_agent_info",
			Help: "Agent metadata (value is always 1).",
		}, []string{"agent", "status", "subnet"}),

		// Per-deployment
		deployReplicasDesired: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_deployment_replicas_desired",
			Help: "Desired replica count per service.",
		}, []string{"deployment", "service"}),
		deployReplicasHealthy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_deployment_replicas_healthy",
			Help: "Healthy replica count per service.",
		}, []string{"deployment", "service"}),
		deployInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "banyan_deployment_info",
			Help: "Deployment metadata (value is always 1).",
		}, []string{"deployment", "status", "strategy"}),

		// Events
		eventsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "banyan_events_total",
			Help: "Cumulative event count by type.",
		}, []string{"type"}),

		// Reconciliation
		reconcileRunsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "banyan_reconcile_runs_total",
			Help: "Number of reconciliation runs by reconciler type.",
		}, []string{"reconciler"}),
		reconcileActionsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "banyan_reconcile_actions_total",
			Help: "Number of reconciliation actions by type (restart, reschedule, mark_stale, etc).",
		}, []string{"action"}),
	}

	// Register all metrics
	reg.MustRegister(
		r.engineUptime, r.engineCPU, r.engineMemUsed, r.engineMemTotal,
		r.engineDiskUsed, r.engineDiskTotal, r.engineCPUCores,
		r.clusterAgentsTotal, r.clusterAgentsConnected,
		r.clusterDeployments, r.clusterContainersTotal, r.clusterContainersHealthy,
		r.clusterTasks,
		r.agentCPU, r.agentMemUsed, r.agentMemTotal,
		r.agentDiskUsed, r.agentDiskTotal, r.agentCPUCores,
		r.agentContainers, r.agentInfo,
		r.deployReplicasDesired, r.deployReplicasHealthy, r.deployInfo,
		r.eventsTotal,
		r.reconcileRunsTotal, r.reconcileActionsTotal,
	)

	return r
}

// Handler returns an HTTP handler that serves Prometheus metrics.
func (r *EngineMetricsRegistry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// UpdateEngine updates engine system metrics and uptime.
func (r *EngineMetricsRegistry) UpdateEngine(m SystemMetrics, uptime time.Duration) {
	r.engineUptime.Set(uptime.Seconds())
	r.engineCPU.Set(m.CPUUsageRatio)
	r.engineMemUsed.Set(float64(m.MemoryUsedBytes))
	r.engineMemTotal.Set(float64(m.MemoryTotalBytes))
	r.engineDiskUsed.Set(float64(m.DiskUsedBytes))
	r.engineDiskTotal.Set(float64(m.DiskTotalBytes))
	r.engineCPUCores.Set(float64(m.CPUCores))

	stored := m
	r.engineMetrics.Store(&stored)
}

// GetEngineMetrics returns the latest engine system metrics.
func (r *EngineMetricsRegistry) GetEngineMetrics() (SystemMetrics, bool) {
	val := r.engineMetrics.Load()
	if val == nil {
		return SystemMetrics{}, false
	}
	return *val, true
}

// UpdateAgent updates per-agent system metrics in both Prometheus gauges
// and the internal map (for GetDashboardData).
func (r *EngineMetricsRegistry) UpdateAgent(name string, m SystemMetrics, containerCount int32) {
	r.agentCPU.WithLabelValues(name).Set(m.CPUUsageRatio)
	r.agentMemUsed.WithLabelValues(name).Set(float64(m.MemoryUsedBytes))
	r.agentMemTotal.WithLabelValues(name).Set(float64(m.MemoryTotalBytes))
	r.agentDiskUsed.WithLabelValues(name).Set(float64(m.DiskUsedBytes))
	r.agentDiskTotal.WithLabelValues(name).Set(float64(m.DiskTotalBytes))
	r.agentCPUCores.WithLabelValues(name).Set(float64(m.CPUCores))
	r.agentContainers.WithLabelValues(name).Set(float64(containerCount))

	// Store for dashboard data access
	stored := m
	r.agentMetrics.Store(name, &stored)
}

// UpdateAgentInfo updates the agent info metric with current status and subnet.
func (r *EngineMetricsRegistry) UpdateAgentInfo(name, status, subnet string) {
	// Reset previous info labels for this agent, then set new
	r.agentInfo.DeletePartialMatch(prometheus.Labels{"agent": name})
	r.agentInfo.WithLabelValues(name, status, subnet).Set(1)
}

// UpdateCluster updates cluster-level aggregate metrics.
func (r *EngineMetricsRegistry) UpdateCluster(stats ClusterStats) {
	r.clusterAgentsTotal.Set(float64(stats.TotalAgents))
	r.clusterAgentsConnected.Set(float64(stats.ConnectedAgents))

	// Reset deployment status gauges and set new values
	r.clusterDeployments.Reset()
	for status, count := range stats.DeploymentsByStatus {
		r.clusterDeployments.WithLabelValues(status).Set(float64(count))
	}

	r.clusterContainersTotal.Set(float64(stats.TotalContainers))
	r.clusterContainersHealthy.Set(float64(stats.HealthyContainers))

	// Reset task status gauges and set new values
	r.clusterTasks.Reset()
	for status, count := range stats.TasksByStatus {
		r.clusterTasks.WithLabelValues(status).Set(float64(count))
	}
}

// UpdateDeployments updates per-deployment replica metrics.
func (r *EngineMetricsRegistry) UpdateDeployments(deploys []DeploymentMetrics) {
	// Reset all deployment metrics to remove stale entries
	r.deployInfo.Reset()
	r.deployReplicasDesired.Reset()
	r.deployReplicasHealthy.Reset()

	for _, d := range deploys {
		r.deployInfo.WithLabelValues(d.Name, d.Status, d.Strategy).Set(1)
		for svc, rm := range d.Services {
			r.deployReplicasDesired.WithLabelValues(d.Name, svc).Set(float64(rm.Desired))
			r.deployReplicasHealthy.WithLabelValues(d.Name, svc).Set(float64(rm.Healthy))
		}
	}
}

// IncrementEvent increments the event counter for the given type.
func (r *EngineMetricsRegistry) IncrementEvent(eventType string) {
	r.eventsTotal.WithLabelValues(eventType).Inc()
}

// GetAgentMetrics returns the latest system metrics for an agent.
// Returns zero metrics and false if the agent hasn't reported yet.
func (r *EngineMetricsRegistry) GetAgentMetrics(name string) (SystemMetrics, bool) {
	val, ok := r.agentMetrics.Load(name)
	if !ok {
		return SystemMetrics{}, false
	}
	return *val.(*SystemMetrics), true
}

// IncrementReconcileRun increments the reconciliation run counter for the given reconciler type.
func (r *EngineMetricsRegistry) IncrementReconcileRun(reconciler string) {
	r.reconcileRunsTotal.WithLabelValues(reconciler).Inc()
}

// IncrementReconcileAction increments the reconciliation action counter for the given action type.
func (r *EngineMetricsRegistry) IncrementReconcileAction(action string) {
	r.reconcileActionsTotal.WithLabelValues(action).Inc()
}
