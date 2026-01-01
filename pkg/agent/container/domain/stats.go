package domain

import "time"

// ContainerStats contains resource usage statistics for a container.
type ContainerStats struct {
	Timestamp   time.Time
	ContainerID ContainerID
	Network     NetworkStats
	CPU         CPUStats
	Memory      MemoryStats
	BlockIO     BlockIOStats
	PIDs        uint64
}

// CPUStats contains CPU usage statistics.
type CPUStats struct {
	UsagePercent    float64 // CPU usage percentage
	UserUsage       uint64  // User CPU time (nanoseconds)
	SystemUsage     uint64  // System CPU time (nanoseconds)
	TotalUsage      uint64  // Total CPU time (nanoseconds)
	ThrottledTime   uint64  // Throttled CPU time (nanoseconds)
	ThrottlePeriods uint64  // Number of throttle periods
}

// MemoryStats contains memory usage statistics.
type MemoryStats struct {
	Usage    uint64  // Current memory usage in bytes
	MaxUsage uint64  // Maximum memory usage in bytes
	Limit    uint64  // Memory limit in bytes
	Percent  float64 // Memory usage percentage
	Cache    uint64  // Cache memory in bytes
	RSS      uint64  // RSS memory in bytes
}

// NetworkStats contains network I/O statistics.
type NetworkStats struct {
	RxBytes   uint64 // Received bytes
	RxPackets uint64 // Received packets
	RxErrors  uint64 // Receive errors
	RxDropped uint64 // Received packets dropped
	TxBytes   uint64 // Transmitted bytes
	TxPackets uint64 // Transmitted packets
	TxErrors  uint64 // Transmit errors
	TxDropped uint64 // Transmitted packets dropped
}

// BlockIOStats contains block I/O statistics.
type BlockIOStats struct {
	ReadBytes  uint64 // Bytes read
	WriteBytes uint64 // Bytes written
	ReadOps    uint64 // Read operations
	WriteOps   uint64 // Write operations
}
