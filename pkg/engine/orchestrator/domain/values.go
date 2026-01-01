package domain

import (
	"fmt"
	"time"
)

// DependencyGraph represents service dependencies.
type DependencyGraph struct {
	Nodes map[string]*DependencyNode
}

// DependencyNode represents a node in the dependency graph.
type DependencyNode struct {
	ServiceName string
	DependsOn   []string
	DependedBy  []string
}

// NewDependencyGraph builds a dependency graph from services.
func NewDependencyGraph(services []Service) *DependencyGraph {
	g := &DependencyGraph{
		Nodes: make(map[string]*DependencyNode),
	}

	// Create nodes
	for _, svc := range services {
		g.Nodes[svc.Name] = &DependencyNode{
			ServiceName: svc.Name,
			DependsOn:   svc.Dependencies,
		}
	}

	// Build reverse dependencies
	for _, node := range g.Nodes {
		for _, dep := range node.DependsOn {
			if depNode, exists := g.Nodes[dep]; exists {
				depNode.DependedBy = append(depNode.DependedBy, node.ServiceName)
			}
		}
	}

	return g
}

// HasCycle detects circular dependencies.
func (g *DependencyGraph) HasCycle() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var hasCycleFrom func(name string) bool
	hasCycleFrom = func(name string) bool {
		visited[name] = true
		recStack[name] = true

		node := g.Nodes[name]
		for _, dep := range node.DependsOn {
			if !visited[dep] {
				if hasCycleFrom(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[name] = false
		return false
	}

	for name := range g.Nodes {
		if !visited[name] {
			if hasCycleFrom(name) {
				return true
			}
		}
	}

	return false
}

// TopologicalSort returns services in dependency order.
func (g *DependencyGraph) TopologicalSort() ([][]string, error) {
	if g.HasCycle() {
		return nil, fmt.Errorf("circular dependency detected")
	}

	var phases [][]string
	remaining := make(map[string]bool)
	for name := range g.Nodes {
		remaining[name] = true
	}

	for len(remaining) > 0 {
		var phase []string

		for name := range remaining {
			node := g.Nodes[name]
			canDeploy := true

			for _, dep := range node.DependsOn {
				if remaining[dep] {
					canDeploy = false
					break
				}
			}

			if canDeploy {
				phase = append(phase, name)
			}
		}

		for _, name := range phase {
			delete(remaining, name)
		}

		phases = append(phases, phase)
	}

	return phases, nil
}

// PortMapping defines port exposure.
type PortMapping struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}

// ResourceSpec defines resource limits.
type ResourceSpec struct {
	CPUShares int64
	MemoryMB  int64
}

// HealthCheckSpec defines health check configuration.
type HealthCheckSpec struct {
	Type     string // http, tcp, exec
	Endpoint string
	Interval time.Duration
	Timeout  time.Duration
	Retries  int
}

// SidecarSpec defines a sidecar plugin.
type SidecarSpec struct {
	Name       string
	Image      string
	Parameters map[string]interface{}
}
