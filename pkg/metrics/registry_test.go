package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewEngineMetricsRegistry(t *testing.T) {
	r := NewEngineMetricsRegistry()
	if r == nil {
		t.Fatal("NewEngineMetricsRegistry returned nil")
	}
	if r.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestUpdateEngine(t *testing.T) {
	r := NewEngineMetricsRegistry()

	m := SystemMetrics{
		CPUUsageRatio:    0.42,
		MemoryUsedBytes:  4294967296,
		MemoryTotalBytes: 8589934592,
		DiskUsedBytes:    42949672960,
		DiskTotalBytes:   85899345920,
		CPUCores:         4,
	}
	r.UpdateEngine(m, 2*time.Hour)

	if got := testutil.ToFloat64(r.engineUptime); got != 7200 {
		t.Errorf("uptime = %f, want 7200", got)
	}
	if got := testutil.ToFloat64(r.engineCPU); got != 0.42 {
		t.Errorf("cpu = %f, want 0.42", got)
	}
	if got := testutil.ToFloat64(r.engineMemUsed); got != 4294967296 {
		t.Errorf("mem used = %f, want 4294967296", got)
	}
	if got := testutil.ToFloat64(r.engineMemTotal); got != 8589934592 {
		t.Errorf("mem total = %f, want 8589934592", got)
	}
	if got := testutil.ToFloat64(r.engineDiskUsed); got != 42949672960 {
		t.Errorf("disk used = %f, want 42949672960", got)
	}
	if got := testutil.ToFloat64(r.engineDiskTotal); got != 85899345920 {
		t.Errorf("disk total = %f, want 85899345920", got)
	}
	if got := testutil.ToFloat64(r.engineCPUCores); got != 4 {
		t.Errorf("cores = %f, want 4", got)
	}
}

func TestGetEngineMetrics(t *testing.T) {
	r := NewEngineMetricsRegistry()

	// Before any update, should return false
	_, ok := r.GetEngineMetrics()
	if ok {
		t.Error("GetEngineMetrics should return false before any update")
	}

	// After update, should return the metrics
	m := SystemMetrics{
		CPUUsageRatio:    0.55,
		MemoryUsedBytes:  1024,
		MemoryTotalBytes: 2048,
		DiskUsedBytes:    4096,
		DiskTotalBytes:   8192,
		CPUCores:         8,
	}
	r.UpdateEngine(m, time.Hour)

	got, ok := r.GetEngineMetrics()
	if !ok {
		t.Fatal("GetEngineMetrics should return true after update")
	}
	if got.CPUUsageRatio != 0.55 {
		t.Errorf("CPU = %f, want 0.55", got.CPUUsageRatio)
	}
	if got.MemoryUsedBytes != 1024 {
		t.Errorf("MemUsed = %d, want 1024", got.MemoryUsedBytes)
	}
	if got.CPUCores != 8 {
		t.Errorf("Cores = %d, want 8", got.CPUCores)
	}
}

func TestUpdateAgent(t *testing.T) {
	r := NewEngineMetricsRegistry()

	m := SystemMetrics{
		CPUUsageRatio:    0.65,
		MemoryUsedBytes:  2147483648,
		MemoryTotalBytes: 4294967296,
		CPUCores:         2,
	}
	r.UpdateAgent("worker-1", m, 5)

	if got := testutil.ToFloat64(r.agentCPU.WithLabelValues("worker-1")); got != 0.65 {
		t.Errorf("agent cpu = %f, want 0.65", got)
	}
	if got := testutil.ToFloat64(r.agentContainers.WithLabelValues("worker-1")); got != 5 {
		t.Errorf("agent containers = %f, want 5", got)
	}

	// Verify internal storage
	stored, ok := r.GetAgentMetrics("worker-1")
	if !ok {
		t.Fatal("GetAgentMetrics returned false")
	}
	if stored.CPUUsageRatio != 0.65 {
		t.Errorf("stored cpu = %f, want 0.65", stored.CPUUsageRatio)
	}

	// Non-existent agent
	_, ok = r.GetAgentMetrics("nonexistent")
	if ok {
		t.Error("expected false for non-existent agent")
	}
}

func TestUpdateCluster(t *testing.T) {
	r := NewEngineMetricsRegistry()

	stats := ClusterStats{
		TotalAgents:     3,
		ConnectedAgents: 2,
		DeploymentsByStatus: map[string]int32{
			"running": 2,
			"stopped": 1,
		},
		TotalContainers:   8,
		HealthyContainers: 7,
		TasksByStatus: map[string]int32{
			"completed": 12,
			"failed":    1,
		},
	}
	r.UpdateCluster(stats)

	if got := testutil.ToFloat64(r.clusterAgentsTotal); got != 3 {
		t.Errorf("agents total = %f, want 3", got)
	}
	if got := testutil.ToFloat64(r.clusterAgentsConnected); got != 2 {
		t.Errorf("agents connected = %f, want 2", got)
	}
	if got := testutil.ToFloat64(r.clusterDeployments.WithLabelValues("running")); got != 2 {
		t.Errorf("deployments running = %f, want 2", got)
	}
	if got := testutil.ToFloat64(r.clusterContainersTotal); got != 8 {
		t.Errorf("containers total = %f, want 8", got)
	}
	if got := testutil.ToFloat64(r.clusterContainersHealthy); got != 7 {
		t.Errorf("containers healthy = %f, want 7", got)
	}
	if got := testutil.ToFloat64(r.clusterTasks.WithLabelValues("completed")); got != 12 {
		t.Errorf("tasks completed = %f, want 12", got)
	}
	if got := testutil.ToFloat64(r.clusterTasks.WithLabelValues("failed")); got != 1 {
		t.Errorf("tasks failed = %f, want 1", got)
	}
}

func TestUpdateDeployments(t *testing.T) {
	r := NewEngineMetricsRegistry()

	deploys := []DeploymentMetrics{
		{
			Name:     "web-app",
			Status:   "running",
			Strategy: "blue-green",
			Services: map[string]ServiceReplicaMetrics{
				"web": {Desired: 3, Healthy: 3},
				"db":  {Desired: 1, Healthy: 1},
			},
		},
	}
	r.UpdateDeployments(deploys)

	if got := testutil.ToFloat64(r.deployInfo.WithLabelValues("web-app", "running", "blue-green")); got != 1 {
		t.Errorf("deploy info = %f, want 1", got)
	}
	if got := testutil.ToFloat64(r.deployReplicasDesired.WithLabelValues("web-app", "web")); got != 3 {
		t.Errorf("web desired = %f, want 3", got)
	}
	if got := testutil.ToFloat64(r.deployReplicasHealthy.WithLabelValues("web-app", "db")); got != 1 {
		t.Errorf("db healthy = %f, want 1", got)
	}

	// Update with different data — old entries should be cleared
	r.UpdateDeployments([]DeploymentMetrics{
		{Name: "api-svc", Status: "running", Strategy: "recreate"},
	})
	// web-app should be gone (reset)
	if got := testutil.ToFloat64(r.deployInfo.WithLabelValues("api-svc", "running", "recreate")); got != 1 {
		t.Errorf("api-svc info = %f, want 1", got)
	}
}

func TestIncrementEvent(t *testing.T) {
	r := NewEngineMetricsRegistry()

	r.IncrementEvent("agent.connected")
	r.IncrementEvent("agent.connected")
	r.IncrementEvent("deployment.started")

	if got := testutil.ToFloat64(r.eventsTotal.WithLabelValues("agent.connected")); got != 2 {
		t.Errorf("agent.connected = %f, want 2", got)
	}
	if got := testutil.ToFloat64(r.eventsTotal.WithLabelValues("deployment.started")); got != 1 {
		t.Errorf("deployment.started = %f, want 1", got)
	}
}

func TestUpdateAgentInfo(t *testing.T) {
	r := NewEngineMetricsRegistry()

	r.UpdateAgentInfo("worker-1", "ready", "10.0.45.0/24")
	if got := testutil.ToFloat64(r.agentInfo.WithLabelValues("worker-1", "ready", "10.0.45.0/24")); got != 1 {
		t.Errorf("agent info = %f, want 1", got)
	}

	// Update status — old label combo should be removed
	r.UpdateAgentInfo("worker-1", "disconnected", "10.0.45.0/24")
	if got := testutil.ToFloat64(r.agentInfo.WithLabelValues("worker-1", "disconnected", "10.0.45.0/24")); got != 1 {
		t.Errorf("updated agent info = %f, want 1", got)
	}
}
