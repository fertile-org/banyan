// Package metrics provides system metrics collection and Prometheus exposition
// for the Banyan engine and agents.
package metrics

// SystemMetrics holds a snapshot of system resource usage.
type SystemMetrics struct {
	CPUUsageRatio    float64 // 0.0–1.0
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
	DiskUsedBytes    uint64
	DiskTotalBytes   uint64
	CPUCores         uint32
}

// ClusterStats holds aggregated cluster-level statistics.
type ClusterStats struct {
	TotalAgents         int32
	ConnectedAgents     int32
	DeploymentsByStatus map[string]int32
	TotalContainers     int32
	HealthyContainers   int32
	TasksByStatus       map[string]int32
}

// DeploymentMetrics holds per-deployment metrics for Prometheus exposition.
type DeploymentMetrics struct {
	Name     string
	Status   string
	Strategy string
	Services map[string]ServiceReplicaMetrics
}

// ServiceReplicaMetrics holds desired vs healthy replica counts.
type ServiceReplicaMetrics struct {
	Desired int32
	Healthy int32
}
