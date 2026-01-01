package usecases

import (
	"context"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/state/domain"
	stateerrors "github.com/fertile-org/banyan/pkg/engine/state/errors"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/state/ports/outbound"
)

// ReconcilerUseCase implements reconciliation operations.
type ReconcilerUseCase struct {
	stateService inbound.StateService
	stateRepo    outbound.StateRepository
	agentQuerier outbound.AgentQuerier
	dispatcher   outbound.ActionDispatcher

	interval        time.Duration
	stopCh          chan struct{}
	wg              sync.WaitGroup
	reconciling     map[string]bool
	lastReconcile   map[string]time.Time
	mu              sync.RWMutex
	started         bool
}

// NewReconcilerUseCase creates a new ReconcilerUseCase.
func NewReconcilerUseCase(
	stateService inbound.StateService,
	stateRepo outbound.StateRepository,
	agentQuerier outbound.AgentQuerier,
	dispatcher outbound.ActionDispatcher,
) *ReconcilerUseCase {
	return &ReconcilerUseCase{
		stateService:  stateService,
		stateRepo:     stateRepo,
		agentQuerier:  agentQuerier,
		dispatcher:    dispatcher,
		interval:      30 * time.Second,
		reconciling:   make(map[string]bool),
		lastReconcile: make(map[string]time.Time),
	}
}

// Start begins the reconciliation loop.
func (uc *ReconcilerUseCase) Start(ctx context.Context) error {
	uc.mu.Lock()
	if uc.started {
		uc.mu.Unlock()
		return stateerrors.ErrReconcilerAlreadyStarted
	}
	uc.stopCh = make(chan struct{})
	uc.started = true
	uc.mu.Unlock()

	uc.wg.Add(1)
	go uc.reconcileLoop(ctx)
	return nil
}

// Stop stops the reconciliation loop.
func (uc *ReconcilerUseCase) Stop(_ context.Context) error {
	uc.mu.Lock()
	if !uc.started {
		uc.mu.Unlock()
		return stateerrors.ErrReconcilerNotStarted
	}
	close(uc.stopCh)
	uc.started = false
	uc.mu.Unlock()

	uc.wg.Wait()
	return nil
}

// reconcileLoop runs the periodic reconciliation.
func (uc *ReconcilerUseCase) reconcileLoop(ctx context.Context) {
	defer uc.wg.Done()

	ticker := time.NewTicker(uc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = uc.TriggerReconcileAll(ctx)
		case <-uc.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// TriggerReconcileAll triggers reconciliation for all deployments.
func (uc *ReconcilerUseCase) TriggerReconcileAll(ctx context.Context) error {
	states, err := uc.stateRepo.ListDesiredStates(ctx)
	if err != nil {
		return err
	}

	for _, state := range states {
		go func(deploymentID string) {
			_ = uc.TriggerReconcile(ctx, deploymentID)
		}(state.DeploymentID)
	}

	return nil
}

// TriggerReconcile triggers reconciliation for a specific deployment.
func (uc *ReconcilerUseCase) TriggerReconcile(ctx context.Context, deploymentID string) error {
	// Check if already reconciling
	uc.mu.Lock()
	if uc.reconciling[deploymentID] {
		uc.mu.Unlock()
		return stateerrors.ErrReconcileInProgress
	}
	uc.reconciling[deploymentID] = true
	uc.mu.Unlock()

	defer func() {
		uc.mu.Lock()
		delete(uc.reconciling, deploymentID)
		uc.lastReconcile[deploymentID] = time.Now()
		uc.mu.Unlock()
	}()

	// Collect actual state from agents
	if err := uc.collectActualState(ctx, deploymentID); err != nil {
		return err
	}

	// Detect drift
	drift, err := uc.stateService.DetectDrift(ctx, deploymentID)
	if err != nil {
		return err
	}

	if len(drift.Drifts) == 0 {
		return nil // No drift
	}

	// Generate and execute remediation actions
	actions := uc.generateActions(drift)
	if uc.dispatcher != nil {
		return uc.dispatcher.DispatchBatch(ctx, actions)
	}
	return nil
}

// collectActualState collects actual state from agents.
func (uc *ReconcilerUseCase) collectActualState(ctx context.Context, deploymentID string) error {
	desired, err := uc.stateRepo.GetDesiredState(ctx, deploymentID)
	if err != nil {
		return err
	}

	// Collect unique agent IDs
	agentSet := make(map[string]bool)
	for _, svc := range desired.Services {
		for _, agentID := range svc.AgentIDs {
			agentSet[agentID] = true
		}
	}

	// Query each agent
	actual := &domain.ActualState{
		DeploymentID: deploymentID,
		Services:     make(map[string]domain.ServiceActualState),
		CollectedAt:  time.Now(),
	}

	if uc.agentQuerier != nil {
		for agentID := range agentSet {
			agentState, err := uc.agentQuerier.GetAgentState(ctx, agentID)
			if err != nil {
				continue // Agent might be offline
			}

			// Map containers to services
			for _, container := range agentState.Containers {
				serviceName := container.Labels["banyan.service"]
				if serviceName == "" {
					continue
				}
				if container.Labels["banyan.deployment"] != deploymentID {
					continue
				}

				svcState := actual.Services[serviceName]
				svcState.Name = serviceName
				svcState.Instances = append(svcState.Instances, domain.InstanceState{
					ContainerID: container.ID,
					AgentID:     agentID,
					IP:          container.IP,
					Status:      domain.ContainerStatus(container.Status),
					Health:      domain.HealthStatus(container.Health),
				})
				actual.Services[serviceName] = svcState
			}
		}
	}

	return uc.stateService.UpdateActualState(ctx, actual)
}

// generateActions generates reconciliation actions from drift.
func (uc *ReconcilerUseCase) generateActions(drift *domain.StateDrift) []*domain.ReconcileAction {
	var actions []*domain.ReconcileAction

	for _, d := range drift.Drifts {
		switch d.Type {
		case domain.DriftMissing:
			actions = append(actions, &domain.ReconcileAction{
				Type:        domain.ActionDeploy,
				ServiceName: d.ServiceName,
			})

		case domain.DriftExtra:
			actions = append(actions, &domain.ReconcileAction{
				Type:        domain.ActionStop,
				ServiceName: d.ServiceName,
			})

		case domain.DriftReplicas:
			actions = append(actions, &domain.ReconcileAction{
				Type:        domain.ActionScale,
				ServiceName: d.ServiceName,
			})

		case domain.DriftUnhealthy:
			actions = append(actions, &domain.ReconcileAction{
				Type:        domain.ActionRestart,
				ServiceName: d.ServiceName,
				AgentID:     d.AgentID,
			})

		case domain.DriftWrongHost:
			actions = append(actions, &domain.ReconcileAction{
				Type:        domain.ActionMigrate,
				ServiceName: d.ServiceName,
				AgentID:     d.AgentID,
			})
		}
	}

	return actions
}

// SetReconcileInterval sets the reconciliation interval.
func (uc *ReconcilerUseCase) SetReconcileInterval(interval time.Duration) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.interval = interval
}

// GetReconcileInterval returns the reconciliation interval.
func (uc *ReconcilerUseCase) GetReconcileInterval() time.Duration {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.interval
}

// GetLastReconcileTime returns the last reconcile time for a deployment.
func (uc *ReconcilerUseCase) GetLastReconcileTime(_ context.Context, deploymentID string) (time.Time, error) {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	t, ok := uc.lastReconcile[deploymentID]
	if !ok {
		return time.Time{}, stateerrors.ErrDeploymentNotFound
	}
	return t, nil
}

// IsReconciling returns whether a deployment is currently being reconciled.
func (uc *ReconcilerUseCase) IsReconciling(_ context.Context, deploymentID string) bool {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	return uc.reconciling[deploymentID]
}

// Ensure ReconcilerUseCase implements ReconcilerService.
var _ inbound.ReconcilerService = (*ReconcilerUseCase)(nil)
