// Package proxy provides a lightweight TCP proxy with dynamic backends
// for load-balancing connections to containers.
package proxy

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePort parses Docker-style port strings: "80", "80:8080", "127.0.0.1:80:8080".
// Returns (hostPort, containerPort, error).
// Strips protocol suffixes like "/tcp" or "/udp" (only TCP is proxied).
func ParsePort(portStr string) (int, int, error) {
	if portStr == "" {
		return 0, 0, fmt.Errorf("empty port string")
	}

	// Strip protocol suffix (e.g., "/tcp", "/udp")
	if idx := strings.Index(portStr, "/"); idx != -1 {
		portStr = portStr[:idx]
	}

	parts := strings.Split(portStr, ":")
	switch len(parts) {
	case 1:
		// "80" → host 80, container 80
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port %q: %w", parts[0], err)
		}
		return port, port, nil
	case 2:
		// "80:8080" → host 80, container 8080
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid host port %q: %w", parts[0], err)
		}
		containerPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid container port %q: %w", parts[1], err)
		}
		return hostPort, containerPort, nil
	case 3:
		// "127.0.0.1:80:8080" → host 80, container 8080 (bind IP ignored for proxy)
		hostPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid host port %q: %w", parts[1], err)
		}
		containerPort, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid container port %q: %w", parts[2], err)
		}
		return hostPort, containerPort, nil
	default:
		return 0, 0, fmt.Errorf("invalid port format %q: expected 'port', 'hostPort:containerPort', or 'ip:hostPort:containerPort'", portStr)
	}
}
