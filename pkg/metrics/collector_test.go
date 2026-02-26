package metrics

import (
	"fmt"
	"testing"
)

func TestParseProcStatLine(t *testing.T) {
	// Real /proc/stat first line example
	line := "cpu  10132153 290696 3084719 46828483 16683 0 25195 0 0 0"
	sample, err := parseProcStatLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// idle = fields[4] + fields[5] = 46828483 + 16683
	expectedIdle := uint64(46828483 + 16683)
	if sample.idle != expectedIdle {
		t.Errorf("idle = %d, want %d", sample.idle, expectedIdle)
	}

	// total = sum of all fields[1:]
	expectedTotal := uint64(10132153 + 290696 + 3084719 + 46828483 + 16683 + 0 + 25195 + 0 + 0 + 0)
	if sample.total != expectedTotal {
		t.Errorf("total = %d, want %d", sample.total, expectedTotal)
	}
}

func TestParseProcStatLine_InvalidFormat(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"empty", ""},
		{"no cpu prefix", "mem  1234 5678"},
		{"too few fields", "cpu  100 200"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseProcStatLine(tt.line)
			if err == nil {
				t.Fatal("expected error for invalid format")
			}
		})
	}
}

func TestParseMeminfoKB(t *testing.T) {
	tests := []struct {
		line string
		want uint64
	}{
		{"MemTotal:       16384000 kB", 16384000},
		{"MemAvailable:    8192000 kB", 8192000},
		{"MemFree:          123456 kB", 123456},
		{"BadLine", 0},
		{"OnlyLabel:", 0},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := parseMeminfoKB(tt.line)
			if got != tt.want {
				t.Errorf("parseMeminfoKB(%q) = %d, want %d", tt.line, got, tt.want)
			}
		})
	}
}

func TestSystemCollector_Collect_WithMocks(t *testing.T) {
	// Save and restore original functions
	origStat := readProcStat
	origMem := readProcMeminfo
	origDisk := readDiskUsage
	origCores := getCPUCoreCount
	defer func() {
		readProcStat = origStat
		readProcMeminfo = origMem
		readDiskUsage = origDisk
		getCPUCoreCount = origCores
	}()

	callCount := 0
	readProcStat = func() (*cpuSample, error) {
		callCount++
		if callCount == 1 {
			return &cpuSample{idle: 100, total: 200}, nil
		}
		// Second call: 50% CPU usage (idle grew by 50, total grew by 100)
		return &cpuSample{idle: 150, total: 300}, nil
	}
	readProcMeminfo = func() (uint64, uint64, error) {
		return 4 * 1024 * 1024 * 1024, 8 * 1024 * 1024 * 1024, nil // 4GB used, 8GB total
	}
	readDiskUsage = func() (uint64, uint64, error) {
		return 40 * 1024 * 1024 * 1024, 80 * 1024 * 1024 * 1024, nil // 40GB used, 80GB total
	}
	getCPUCoreCount = func() int { return 4 }

	c := NewSystemCollector()

	// First collect: CPU should be 0 (no previous sample)
	m1 := c.Collect()
	if m1.CPUUsageRatio != 0 {
		t.Errorf("first CPU = %f, want 0", m1.CPUUsageRatio)
	}
	if m1.MemoryUsedBytes != 4*1024*1024*1024 {
		t.Errorf("memory used = %d, want %d", m1.MemoryUsedBytes, 4*1024*1024*1024)
	}
	if m1.MemoryTotalBytes != 8*1024*1024*1024 {
		t.Errorf("memory total = %d, want %d", m1.MemoryTotalBytes, 8*1024*1024*1024)
	}
	if m1.DiskUsedBytes != 40*1024*1024*1024 {
		t.Errorf("disk used = %d, want %d", m1.DiskUsedBytes, 40*1024*1024*1024)
	}
	if m1.DiskTotalBytes != 80*1024*1024*1024 {
		t.Errorf("disk total = %d, want %d", m1.DiskTotalBytes, 80*1024*1024*1024)
	}
	if m1.CPUCores != 4 {
		t.Errorf("cores = %d, want 4", m1.CPUCores)
	}

	// Second collect: CPU should be 0.5 (50% usage)
	m2 := c.Collect()
	if m2.CPUUsageRatio < 0.49 || m2.CPUUsageRatio > 0.51 {
		t.Errorf("second CPU = %f, want ~0.5", m2.CPUUsageRatio)
	}
}

func TestSystemCollector_Collect_ProcStatError(t *testing.T) {
	origStat := readProcStat
	origMem := readProcMeminfo
	origDisk := readDiskUsage
	origCores := getCPUCoreCount
	defer func() {
		readProcStat = origStat
		readProcMeminfo = origMem
		readDiskUsage = origDisk
		getCPUCoreCount = origCores
	}()

	readProcStat = func() (*cpuSample, error) {
		return nil, fmt.Errorf("no /proc/stat")
	}
	readProcMeminfo = func() (uint64, uint64, error) {
		return 0, 0, fmt.Errorf("no /proc/meminfo")
	}
	readDiskUsage = func() (uint64, uint64, error) {
		return 0, 0, fmt.Errorf("no disk")
	}
	getCPUCoreCount = func() int { return 2 }

	c := NewSystemCollector()
	m := c.Collect()

	// All values should be 0 on error, except cores
	if m.CPUUsageRatio != 0 {
		t.Errorf("CPU = %f, want 0", m.CPUUsageRatio)
	}
	if m.MemoryUsedBytes != 0 || m.MemoryTotalBytes != 0 {
		t.Errorf("memory = %d/%d, want 0/0", m.MemoryUsedBytes, m.MemoryTotalBytes)
	}
	if m.DiskUsedBytes != 0 || m.DiskTotalBytes != 0 {
		t.Errorf("disk = %d/%d, want 0/0", m.DiskUsedBytes, m.DiskTotalBytes)
	}
	if m.CPUCores != 2 {
		t.Errorf("cores = %d, want 2", m.CPUCores)
	}
}

func TestSystemCollector_CPUDeltaZero(t *testing.T) {
	origStat := readProcStat
	defer func() { readProcStat = origStat }()

	// Both samples identical — total delta is 0
	readProcStat = func() (*cpuSample, error) {
		return &cpuSample{idle: 100, total: 200}, nil
	}

	c := NewSystemCollector()
	c.Collect() // seed
	m := c.Collect()

	if m.CPUUsageRatio != 0 {
		t.Errorf("CPU = %f, want 0 (zero delta)", m.CPUUsageRatio)
	}
}
