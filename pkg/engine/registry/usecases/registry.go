// Package usecases implements the business logic for the Agent Registry.
package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
	"github.com/fertile-org/banyan/pkg/engine/registry/ports/outbound"
)

// RegistryUseCase implements the agent registry operations.
type RegistryUseCase struct {
	repo       outbound.AgentRepository
	publisher  outbound.EventPublisher
	strategies map[string]domain.SelectionStrategy
	logger     *slog.Logger
}

// NewRegistryUseCase creates a new RegistryUseCase.
func NewRegistryUseCase(
	repo outbound.AgentRepository,
	publisher outbound.EventPublisher,
	logger *slog.Logger,
) *RegistryUseCase {
	if logger == nil {
		logger = slog.Default()
	}

	strategies := map[string]domain.SelectionStrategy{
		"round_robin":  domain.NewRoundRobinStrategy(),
		"least_loaded": domain.NewLeastLoadedStrategy(),
		"spread":       domain.NewSpreadStrategy(),
		"bin_pack":     domain.NewBinPackStrategy(),
	}

	return &RegistryUseCase{
		repo:       repo,
		publisher:  publisher,
		strategies: strategies,
		logger:     logger,
	}
}

// RegisterAgent registers a new agent.
func (uc *RegistryUseCase) RegisterAgent(
	ctx context.Context,
	req *domain.RegisterAgentRequest,
) (*domain.Agent, error) {
	// Validate request
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Create agent entity
	agent := &domain.Agent{
		ID:            domain.NewAgentID(),
		Hostname:      req.Hostname,
		Address:       req.Address,
		Status:        domain.AgentStatusOnline,
		Capabilities:  req.Capabilities,
		Resources:     req.Resources,
		Labels:        req.Labels,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		Version:       req.Version,
	}

	// Persist
	if err := uc.repo.Save(ctx, agent); err != nil {
		return nil, fmt.Errorf("failed to save agent: %w", err)
	}

	// Publish event
	if uc.publisher != nil {
		_ = uc.publisher.Publish(ctx, domain.AgentEvent{
			Type:      domain.AgentEventRegistered,
			AgentID:   agent.ID,
			Agent:     agent,
			Timestamp: time.Now(),
		})
	}

	uc.logger.Info("agent registered",
		"agent_id", agent.ID,
		"hostname", agent.Hostname,
		"address", agent.Address,
	)

	return agent, nil
}

// DeregisterAgent removes an agent from the registry.
func (uc *RegistryUseCase) DeregisterAgent(
	ctx context.Context,
	agentID domain.AgentID,
) error {
	agent, err := uc.repo.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	if err := uc.repo.Delete(ctx, agentID); err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	// Publish event
	if uc.publisher != nil {
		_ = uc.publisher.Publish(ctx, domain.AgentEvent{
			Type:      domain.AgentEventDeregistered,
			AgentID:   agentID,
			Agent:     agent,
			Timestamp: time.Now(),
		})
	}

	uc.logger.Info("agent deregistered",
		"agent_id", agentID,
	)

	return nil
}

// ProcessHeartbeat processes an agent heartbeat.
func (uc *RegistryUseCase) ProcessHeartbeat(
	ctx context.Context,
	agentID domain.AgentID,
	status *domain.HeartbeatStatus,
) error {
	agent, err := uc.repo.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	previousStatus := agent.Status

	// Update agent state
	agent.UpdateHeartbeat(status.Status, status.Resources)

	if err := uc.repo.Save(ctx, agent); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	// Publish status change event if needed
	if previousStatus != agent.Status && uc.publisher != nil {
		eventType := domain.AgentEventUpdated
		if agent.Status == domain.AgentStatusOnline {
			eventType = domain.AgentEventOnline
		} else if agent.Status == domain.AgentStatusOffline {
			eventType = domain.AgentEventOffline
		}

		_ = uc.publisher.Publish(ctx, domain.AgentEvent{
			Type:      eventType,
			AgentID:   agentID,
			Agent:     agent,
			Timestamp: time.Now(),
		})
	}

	return nil
}

// GetAgent returns an agent by ID.
func (uc *RegistryUseCase) GetAgent(
	ctx context.Context,
	agentID domain.AgentID,
) (*domain.Agent, error) {
	return uc.repo.FindByID(ctx, agentID)
}

// ListAgents returns agents matching the filter.
func (uc *RegistryUseCase) ListAgents(
	ctx context.Context,
	filter domain.AgentFilter,
) ([]domain.Agent, error) {
	var agents []domain.Agent
	var err error

	// Get base set of agents
	if filter.Status != nil {
		agents, err = uc.repo.FindByStatus(ctx, *filter.Status)
	} else {
		agents, err = uc.repo.FindAll(ctx)
	}
	if err != nil {
		return nil, err
	}

	// Filter by capabilities
	if len(filter.Capabilities) > 0 {
		agents = filterByCapabilities(agents, filter.Capabilities)
	}

	// Filter by labels
	if len(filter.Labels) > 0 {
		agents = filterByLabels(agents, filter.Labels)
	}

	return agents, nil
}

// SelectAgents selects agents matching the criteria using the specified strategy.
func (uc *RegistryUseCase) SelectAgents(
	ctx context.Context,
	criteria domain.SelectionCriteria, //nolint:gocritic // hugeParam - used by value for filtering
	strategyName string,
) ([]domain.Agent, error) {
	// Get all online agents
	candidates, err := uc.repo.FindByStatus(ctx, domain.AgentStatusOnline)
	if err != nil {
		return nil, fmt.Errorf("failed to get agents: %w", err)
	}

	// Filter by capabilities
	candidates = filterByCapabilities(candidates, criteria.RequiredCapabilities)

	// Filter by resources
	candidates = filterByResources(candidates, &criteria)

	// Filter by labels
	candidates = filterByLabels(candidates, criteria.Labels)

	// Apply exclusions
	candidates = applyExclusions(candidates, criteria.ExcludedAgents)

	count := criteria.Count
	if count <= 0 {
		count = 1
	}

	if len(candidates) < count {
		return nil, fmt.Errorf("%w: need %d, found %d",
			domain.ErrInsufficientAgents, count, len(candidates))
	}

	// Apply selection strategy
	strategy := uc.strategies[strategyName]
	if strategy == nil {
		strategy = uc.strategies["least_loaded"] // Default
	}

	selected := strategy.Select(candidates, &criteria)
	if len(selected) > count {
		selected = selected[:count]
	}

	return selected, nil
}

// DrainAgent sets an agent to draining status.
func (uc *RegistryUseCase) DrainAgent(
	ctx context.Context,
	agentID domain.AgentID,
) error {
	agent, err := uc.repo.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	agent.MarkDraining()

	if err := uc.repo.Save(ctx, agent); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	uc.logger.Info("agent draining",
		"agent_id", agentID,
	)

	return nil
}

// ActivateAgent sets an agent to online status.
func (uc *RegistryUseCase) ActivateAgent(
	ctx context.Context,
	agentID domain.AgentID,
) error {
	agent, err := uc.repo.FindByID(ctx, agentID)
	if err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	agent.MarkOnline()

	if err := uc.repo.Save(ctx, agent); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	uc.logger.Info("agent activated",
		"agent_id", agentID,
	)

	return nil
}

// Helper functions

func filterByCapabilities(agents []domain.Agent, required []domain.CapabilityType) []domain.Agent {
	if len(required) == 0 {
		return agents
	}

	var filtered []domain.Agent
	for i := range agents {
		if agents[i].HasCapabilities(required) {
			filtered = append(filtered, agents[i])
		}
	}
	return filtered
}

func filterByResources(agents []domain.Agent, criteria *domain.SelectionCriteria) []domain.Agent {
	var filtered []domain.Agent
	for i := range agents {
		if agents[i].Resources.HasCapacity(criteria.MinCPU, criteria.MinMemoryMB) {
			filtered = append(filtered, agents[i])
		}
	}
	return filtered
}

func filterByLabels(agents []domain.Agent, labels map[string]string) []domain.Agent {
	if len(labels) == 0 {
		return agents
	}

	var filtered []domain.Agent
	for i := range agents {
		if matchesLabels(agents[i].Labels, labels) {
			filtered = append(filtered, agents[i])
		}
	}
	return filtered
}

func matchesLabels(agentLabels, required map[string]string) bool {
	for k, v := range required {
		if agentLabels[k] != v {
			return false
		}
	}
	return true
}

func applyExclusions(agents []domain.Agent, excluded []domain.AgentID) []domain.Agent {
	if len(excluded) == 0 {
		return agents
	}

	excludeSet := make(map[domain.AgentID]bool)
	for _, id := range excluded {
		excludeSet[id] = true
	}

	var filtered []domain.Agent
	for i := range agents {
		if !excludeSet[agents[i].ID] {
			filtered = append(filtered, agents[i])
		}
	}
	return filtered
}
