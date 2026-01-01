// Package domain contains the core domain entities and value objects for the Agent Registry.
package domain

import (
	"sort"
	"sync"
)

// SelectionStrategy defines how agents are selected for tasks.
type SelectionStrategy interface {
	// Select selects agents from candidates based on criteria.
	Select(candidates []Agent, criteria *SelectionCriteria) []Agent
	// Name returns the strategy name.
	Name() string
}

// RoundRobinStrategy distributes load evenly across agents.
type RoundRobinStrategy struct {
	lastIndex int
	mu        sync.Mutex
}

// NewRoundRobinStrategy creates a new RoundRobinStrategy.
func NewRoundRobinStrategy() *RoundRobinStrategy {
	return &RoundRobinStrategy{}
}

// Name returns the strategy name.
func (s *RoundRobinStrategy) Name() string {
	return "round_robin"
}

// Select selects agents using round-robin algorithm.
func (s *RoundRobinStrategy) Select(candidates []Agent, criteria *SelectionCriteria) []Agent {
	if len(candidates) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	count := criteria.Count
	if count <= 0 {
		count = 1
	}
	if count > len(candidates) {
		count = len(candidates)
	}

	result := make([]Agent, 0, count)
	for i := 0; i < count; i++ {
		idx := (s.lastIndex + i) % len(candidates)
		result = append(result, candidates[idx])
	}
	s.lastIndex = (s.lastIndex + count) % len(candidates)

	return result
}

// LeastLoadedStrategy prefers agents with most available resources.
type LeastLoadedStrategy struct{}

// NewLeastLoadedStrategy creates a new LeastLoadedStrategy.
func NewLeastLoadedStrategy() *LeastLoadedStrategy {
	return &LeastLoadedStrategy{}
}

// Name returns the strategy name.
func (s *LeastLoadedStrategy) Name() string {
	return "least_loaded"
}

// Select selects agents with the most available resources.
func (s *LeastLoadedStrategy) Select(candidates []Agent, criteria *SelectionCriteria) []Agent {
	if len(candidates) == 0 {
		return nil
	}

	// Sort by available resources (most to least)
	sorted := make([]Agent, len(candidates))
	copy(sorted, candidates)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Resources.AvailableScore() > sorted[j].Resources.AvailableScore()
	})

	count := criteria.Count
	if count <= 0 {
		count = 1
	}
	if count > len(sorted) {
		count = len(sorted)
	}

	return sorted[:count]
}

// SpreadStrategy ensures containers spread across different agents.
type SpreadStrategy struct{}

// NewSpreadStrategy creates a new SpreadStrategy.
func NewSpreadStrategy() *SpreadStrategy {
	return &SpreadStrategy{}
}

// Name returns the strategy name.
func (s *SpreadStrategy) Name() string {
	return "spread"
}

// Select spreads selection across agents with fewest containers.
func (s *SpreadStrategy) Select(candidates []Agent, criteria *SelectionCriteria) []Agent {
	if len(candidates) == 0 {
		return nil
	}

	// Sort by container count (fewest first)
	sorted := make([]Agent, len(candidates))
	copy(sorted, candidates)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Resources.ContainerCount < sorted[j].Resources.ContainerCount
	})

	count := criteria.Count
	if count <= 0 {
		count = 1
	}
	if count > len(sorted) {
		count = len(sorted)
	}

	return sorted[:count]
}

// BinPackStrategy consolidates containers to fewer agents.
type BinPackStrategy struct{}

// NewBinPackStrategy creates a new BinPackStrategy.
func NewBinPackStrategy() *BinPackStrategy {
	return &BinPackStrategy{}
}

// Name returns the strategy name.
func (s *BinPackStrategy) Name() string {
	return "bin_pack"
}

// Select packs containers onto agents with most containers first.
func (s *BinPackStrategy) Select(candidates []Agent, criteria *SelectionCriteria) []Agent {
	if len(candidates) == 0 {
		return nil
	}

	// Sort by container count (most first) - fills existing agents before new ones
	sorted := make([]Agent, len(candidates))
	copy(sorted, candidates)

	sort.Slice(sorted, func(i, j int) bool {
		// Prefer agents with more containers (bin packing)
		// but still must have capacity
		return sorted[i].Resources.ContainerCount > sorted[j].Resources.ContainerCount
	})

	count := criteria.Count
	if count <= 0 {
		count = 1
	}
	if count > len(sorted) {
		count = len(sorted)
	}

	return sorted[:count]
}
