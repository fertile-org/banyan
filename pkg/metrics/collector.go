package metrics

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// cpuSample holds a snapshot of /proc/stat aggregate CPU counters.
type cpuSample struct {
	idle  uint64
	total uint64
}

// SystemCollector reads system metrics from /proc and syscall.
// It is safe for concurrent use.
type SystemCollector struct {
	mu      sync.Mutex
	prevCPU *cpuSample
}

// Function variables for testing — override these in tests to inject mock data.
var (
	readProcStat    = defaultReadProcStat
	readProcMeminfo = defaultReadProcMeminfo
	readDiskUsage   = defaultReadDiskUsage
	getCPUCoreCount = defaultGetCPUCoreCount
)

// NewSystemCollector creates a new SystemCollector.
func NewSystemCollector() *SystemCollector {
	return &SystemCollector{}
}

// Collect returns a snapshot of current system metrics.
// CPU usage is computed as the delta since the last Collect call.
// The first call returns 0 for CPU (no previous sample to compare against).
func (c *SystemCollector) Collect() SystemMetrics {
	memUsed, memTotal := c.collectMemory()
	diskUsed, diskTotal := c.collectDisk()

	return SystemMetrics{
		CPUUsageRatio:    c.collectCPU(),
		MemoryUsedBytes:  memUsed,
		MemoryTotalBytes: memTotal,
		DiskUsedBytes:    diskUsed,
		DiskTotalBytes:   diskTotal,
		CPUCores:         uint32(getCPUCoreCount()), //nolint:gosec // core count is always small positive
	}
}

// collectCPU computes CPU usage ratio from /proc/stat delta.
func (c *SystemCollector) collectCPU() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	sample, err := readProcStat()
	if err != nil {
		return 0
	}

	if c.prevCPU == nil {
		c.prevCPU = sample
		return 0
	}

	totalDelta := sample.total - c.prevCPU.total
	idleDelta := sample.idle - c.prevCPU.idle
	c.prevCPU = sample

	if totalDelta == 0 {
		return 0
	}

	return 1.0 - float64(idleDelta)/float64(totalDelta)
}

func (c *SystemCollector) collectMemory() (used, total uint64) {
	used, total, err := readProcMeminfo()
	if err != nil {
		return 0, 0
	}
	return used, total
}

func (c *SystemCollector) collectDisk() (used, total uint64) {
	used, total, err := readDiskUsage()
	if err != nil {
		return 0, 0
	}
	return used, total
}

// --- Default reader implementations (Linux /proc) ---

func defaultReadProcStat() (*cpuSample, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty /proc/stat")
	}

	return parseProcStatLine(scanner.Text())
}

// parseProcStatLine parses the aggregate "cpu" line from /proc/stat.
// Format: "cpu  user nice system idle iowait irq softirq steal [guest guest_nice]"
func parseProcStatLine(line string) (*cpuSample, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return nil, fmt.Errorf("unexpected /proc/stat format: %q", line)
	}

	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
		// Fields 4 (idle) and 5 (iowait) count as idle time
		if i == 4 || i == 5 {
			idle += val
		}
	}

	return &cpuSample{idle: idle, total: total}, nil
}

func defaultReadProcMeminfo() (used, total uint64, err error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			memTotal = parseMeminfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvailable = parseMeminfoKB(line)
		}
	}

	// Values in /proc/meminfo are in kB
	return (memTotal - memAvailable) * 1024, memTotal * 1024, nil
}

// parseMeminfoKB extracts the numeric kB value from a /proc/meminfo line.
// Example: "MemTotal:       16384000 kB" → 16384000
func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		return val
	}
	return 0
}

func defaultReadDiskUsage() (used, total uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0, err
	}
	bsize := uint64(stat.Bsize) //nolint:gosec // Bsize is always positive
	total = stat.Blocks * bsize
	free := stat.Bfree * bsize
	return total - free, total, nil
}

func defaultGetCPUCoreCount() int {
	return runtime.NumCPU()
}
