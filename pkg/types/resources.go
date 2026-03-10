package types

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultMemoryRequest is the memory assumed for scheduling when a service
	// doesn't specify deploy.resources. 512MB matches Docker Compose conventions.
	DefaultMemoryRequest uint64 = 512 * 1024 * 1024 // 512MB

	// DefaultCPURequest is the CPU assumed for scheduling when a service
	// doesn't specify deploy.resources.
	DefaultCPURequest float64 = 1.0
)

// ResourceRequest represents parsed resource requirements for scheduling.
type ResourceRequest struct {
	MemoryBytes uint64
	CPUCores    float64
}

// ServiceResourceRequest returns the parsed resource request for a service,
// falling back to defaults when not specified.
// It uses reservations if set (soft request), otherwise limits (hard cap).
func ServiceResourceRequest(svc ServiceRecord) ResourceRequest { //nolint:gocritic // value receiver avoids caller-side &
	req := ResourceRequest{
		MemoryBytes: DefaultMemoryRequest,
		CPUCores:    DefaultCPURequest,
	}

	// Prefer reservation (scheduling hint) over limit (hard cap)
	if svc.MemoryReservation != "" {
		if parsed, err := ParseMemoryBytes(svc.MemoryReservation); err == nil && parsed > 0 {
			req.MemoryBytes = parsed
		}
	} else if svc.MemoryLimit != "" {
		if parsed, err := ParseMemoryBytes(svc.MemoryLimit); err == nil && parsed > 0 {
			req.MemoryBytes = parsed
		}
	}

	if svc.CPULimit != "" {
		if parsed, err := ParseCPU(svc.CPULimit); err == nil && parsed > 0 {
			req.CPUCores = parsed
		}
	}

	return req
}

// ParseMemoryBytes parses Docker/nerdctl memory strings into bytes.
// Supports: "512m", "1g", "256k", "1024" (plain bytes), "0.5g".
func ParseMemoryBytes(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty memory string")
	}

	var multiplier uint64 = 1
	numStr := s

	switch {
	case strings.HasSuffix(s, "g"):
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "m"):
		multiplier = 1024 * 1024
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "k"):
		multiplier = 1024
		numStr = s[:len(s)-1]
	case strings.HasSuffix(s, "b"):
		numStr = s[:len(s)-1]
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative memory value %q", s)
	}

	return uint64(val * float64(multiplier)), nil
}

// ParseCPU parses Docker/nerdctl CPU strings into float64 cores.
// Supports: "0.5", "2", "1.0", "0.25".
func ParseCPU(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty CPU string")
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid CPU value %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("negative CPU value %q", s)
	}

	return val, nil
}
