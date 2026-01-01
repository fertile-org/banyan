package domain

import (
	"testing"
)

func TestDependencyGraph_HasCycle(t *testing.T) {
	tests := []struct {
		name     string
		services []Service
		hasCycle bool
	}{
		{
			name: "no cycle",
			services: []Service{
				{Name: "api", Dependencies: []string{"db"}},
				{Name: "db", Dependencies: []string{}},
			},
			hasCycle: false,
		},
		{
			name: "simple cycle",
			services: []Service{
				{Name: "a", Dependencies: []string{"b"}},
				{Name: "b", Dependencies: []string{"a"}},
			},
			hasCycle: true,
		},
		{
			name: "complex cycle",
			services: []Service{
				{Name: "a", Dependencies: []string{"b"}},
				{Name: "b", Dependencies: []string{"c"}},
				{Name: "c", Dependencies: []string{"a"}},
			},
			hasCycle: true,
		},
		{
			name: "no dependencies",
			services: []Service{
				{Name: "a", Dependencies: []string{}},
				{Name: "b", Dependencies: []string{}},
			},
			hasCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewDependencyGraph(tt.services)
			if got := g.HasCycle(); got != tt.hasCycle {
				t.Errorf("HasCycle() = %v, want %v", got, tt.hasCycle)
			}
		})
	}
}

func TestDependencyGraph_TopologicalSort(t *testing.T) {
	tests := []struct {
		name        string
		services    []Service
		wantPhases  int
		shouldError bool
	}{
		{
			name: "simple dependency",
			services: []Service{
				{Name: "api", Dependencies: []string{"db"}},
				{Name: "db", Dependencies: []string{}},
			},
			wantPhases:  2,
			shouldError: false,
		},
		{
			name: "parallel services",
			services: []Service{
				{Name: "frontend", Dependencies: []string{"api"}},
				{Name: "worker", Dependencies: []string{"api"}},
				{Name: "api", Dependencies: []string{"db"}},
				{Name: "db", Dependencies: []string{}},
			},
			wantPhases:  3,
			shouldError: false,
		},
		{
			name: "circular dependency",
			services: []Service{
				{Name: "a", Dependencies: []string{"b"}},
				{Name: "b", Dependencies: []string{"a"}},
			},
			wantPhases:  0,
			shouldError: true,
		},
		{
			name: "no dependencies",
			services: []Service{
				{Name: "a", Dependencies: []string{}},
				{Name: "b", Dependencies: []string{}},
				{Name: "c", Dependencies: []string{}},
			},
			wantPhases:  1, // All in one phase
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewDependencyGraph(tt.services)
			phases, err := g.TopologicalSort()

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("TopologicalSort() error = %v", err)
			}

			if len(phases) != tt.wantPhases {
				t.Errorf("TopologicalSort() returned %d phases, want %d", len(phases), tt.wantPhases)
			}
		})
	}
}

func TestDependencyGraph_TopologicalSort_Order(t *testing.T) {
	services := []Service{
		{Name: "api", Dependencies: []string{"db", "cache"}},
		{Name: "db", Dependencies: []string{}},
		{Name: "cache", Dependencies: []string{}},
	}

	g := NewDependencyGraph(services)
	phases, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	// db and cache should be before api
	apiPhase := -1
	dbPhase := -1
	cachePhase := -1

	for i, phase := range phases {
		for _, svc := range phase {
			switch svc {
			case "api":
				apiPhase = i
			case "db":
				dbPhase = i
			case "cache":
				cachePhase = i
			}
		}
	}

	if dbPhase >= apiPhase {
		t.Error("db should be deployed before api")
	}
	if cachePhase >= apiPhase {
		t.Error("cache should be deployed before api")
	}
}

func TestNewDependencyGraph_ReverseDependencies(t *testing.T) {
	services := []Service{
		{Name: "api", Dependencies: []string{"db"}},
		{Name: "db", Dependencies: []string{}},
	}

	g := NewDependencyGraph(services)

	// db should be depended on by api
	dbNode := g.Nodes["db"]
	if len(dbNode.DependedBy) != 1 || dbNode.DependedBy[0] != "api" {
		t.Errorf("Expected db to be depended on by api, got %v", dbNode.DependedBy)
	}

	// api should have no dependents
	apiNode := g.Nodes["api"]
	if len(apiNode.DependedBy) != 0 {
		t.Errorf("Expected api to have no dependents, got %v", apiNode.DependedBy)
	}
}
