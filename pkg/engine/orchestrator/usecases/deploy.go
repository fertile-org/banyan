package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/engine/orchestrator/domain"
	orcerrors "github.com/fertile-org/banyan/pkg/engine/orchestrator/errors"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/inbound"
	"github.com/fertile-org/banyan/pkg/engine/orchestrator/ports/outbound"
	"github.com/google/uuid"
)

// DeployUseCase implements deployment workflow.
type DeployUseCase struct {
	repo       outbound.DeploymentRepository
	dispatcher outbound.AgentDispatcher
	scheduler  outbound.Scheduler
	plugins    outbound.PluginExecutor
	parser     outbound.BanyanParser
}

// NewDeployUseCase creates a new DeployUseCase.
func NewDeployUseCase(
	repo outbound.DeploymentRepository,
	dispatcher outbound.AgentDispatcher,
	scheduler outbound.Scheduler,
	plugins outbound.PluginExecutor,
	parser outbound.BanyanParser,
) *DeployUseCase {
	return &DeployUseCase{
		repo:       repo,
		dispatcher: dispatcher,
		scheduler:  scheduler,
		plugins:    plugins,
		parser:     parser,
	}
}

// CreateDeployment initializes a new deployment.
func (uc *DeployUseCase) CreateDeployment(ctx context.Context, req inbound.CreateDeploymentRequest) (*domain.Deployment, error) {
	// Parse banyan.yml file
	services, err := uc.parser.Parse(req.BanyanFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", orcerrors.ErrInvalidBanyanFile, err)
	}

	deployment := &domain.Deployment{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Services:  services,
		State:     domain.StateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := uc.repo.Save(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to save deployment: %w", err)
	}

	return deployment, nil
}

// ExecuteDeployment runs the full deployment pipeline.
func (uc *DeployUseCase) ExecuteDeployment(ctx context.Context, deploymentID string) error {
	deployment, err := uc.repo.Get(ctx, deploymentID)
	if err != nil {
		return fmt.Errorf("%w: %v", orcerrors.ErrDeploymentNotFound, err)
	}

	// Define pipeline stages
	stages := []struct {
		name   string
		state  domain.DeploymentState
		hook   outbound.LifecycleHook
		action func(context.Context, *domain.Deployment) error
	}{
		{"validate", domain.StateValidating, outbound.HookValidate, uc.validate},
		{"plan", domain.StatePlanning, outbound.HookPlan, uc.plan},
		{"deploy", domain.StateDeploying, outbound.HookDeploy, uc.deploy},
		{"verify", domain.StateVerifying, outbound.HookVerify, uc.verify},
	}

	for _, stage := range stages {
		// Update state
		deployment.State = stage.state
		deployment.UpdatedAt = time.Now()
		if err := uc.repo.Update(ctx, deployment); err != nil {
			return err
		}

		// Execute lifecycle hook (if plugins configured)
		if uc.plugins != nil {
			hookData := &outbound.HookData{
				DeploymentID: deployment.ID,
				Hook:         stage.hook,
				Services:     deployment.Services,
				Context:      make(map[string]interface{}),
			}

			result, err := uc.plugins.ExecuteHook(ctx, stage.hook, hookData)
			if err != nil {
				return uc.fail(ctx, deployment, stage.name, err)
			}
			if !result.Continue {
				return uc.fail(ctx, deployment, stage.name, fmt.Errorf("%w: %s", orcerrors.ErrHookRejected, result.Message))
			}
		}

		// Execute stage action
		if err := stage.action(ctx, deployment); err != nil {
			return uc.fail(ctx, deployment, stage.name, err)
		}
	}

	// Success
	deployment.State = domain.StateActive
	deployment.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, deployment)
}

// GetDeploymentPlan returns execution plan without executing.
func (uc *DeployUseCase) GetDeploymentPlan(ctx context.Context, req inbound.CreateDeploymentRequest) (*domain.ExecutionPlan, error) {
	services, err := uc.parser.Parse(req.BanyanFile)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", orcerrors.ErrInvalidBanyanFile, err)
	}

	deps := domain.NewDependencyGraph(services)
	return uc.scheduler.Schedule(ctx, services, deps)
}

// RollbackDeployment reverts to previous state.
func (uc *DeployUseCase) RollbackDeployment(ctx context.Context, deploymentID string, _ inbound.RollbackStrategy) error {
	deployment, err := uc.repo.Get(ctx, deploymentID)
	if err != nil {
		return orcerrors.ErrDeploymentNotFound
	}

	deployment.State = domain.StateRollingBack
	deployment.UpdatedAt = time.Now()
	if err := uc.repo.Update(ctx, deployment); err != nil {
		return err
	}

	// Stop all services (in reverse order)
	if deployment.Plan != nil {
		for i := len(deployment.Plan.Phases) - 1; i >= 0; i-- {
			phase := deployment.Plan.Phases[i]
			for _, serviceName := range phase.Services {
				if uc.dispatcher != nil {
					task := &outbound.Task{
						ID:           uuid.New().String(),
						DeploymentID: deployment.ID,
						Type:         outbound.TaskStopService,
						Payload: map[string]interface{}{
							"service": serviceName,
						},
					}
					_, _ = uc.dispatcher.DispatchTask(ctx, "agent-1", task) // Simplified
				}
			}
		}
	}

	deployment.State = domain.StateDestroyed
	deployment.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, deployment)
}

// CancelDeployment stops an in-progress deployment.
func (uc *DeployUseCase) CancelDeployment(ctx context.Context, deploymentID string) error {
	deployment, err := uc.repo.Get(ctx, deploymentID)
	if err != nil {
		return orcerrors.ErrDeploymentNotFound
	}

	// Can only cancel if in progress
	switch deployment.State {
	case domain.StateValidating, domain.StatePlanning, domain.StateDeploying, domain.StateVerifying:
		deployment.State = domain.StateFailed
		deployment.Error = "cancelled by user"
		deployment.UpdatedAt = time.Now()
		return uc.repo.Update(ctx, deployment)
	default:
		return fmt.Errorf("%w: cannot cancel deployment in state %s", orcerrors.ErrInvalidState, deployment.State)
	}
}

// GetDeployment retrieves deployment by ID.
func (uc *DeployUseCase) GetDeployment(ctx context.Context, deploymentID string) (*domain.Deployment, error) {
	return uc.repo.Get(ctx, deploymentID)
}

// ListDeployments lists all deployments.
func (uc *DeployUseCase) ListDeployments(ctx context.Context, filter inbound.DeploymentFilter) ([]*domain.Deployment, error) {
	return uc.repo.List(ctx, filter)
}

// DeleteDeployment removes a deployment.
func (uc *DeployUseCase) DeleteDeployment(ctx context.Context, deploymentID string) error {
	return uc.repo.Delete(ctx, deploymentID)
}

// validate validates the deployment.
func (uc *DeployUseCase) validate(_ context.Context, d *domain.Deployment) error {
	deps := domain.NewDependencyGraph(d.Services)
	if deps.HasCycle() {
		return orcerrors.ErrCircularDependency
	}
	return nil
}

// plan creates the execution plan.
func (uc *DeployUseCase) plan(ctx context.Context, d *domain.Deployment) error {
	deps := domain.NewDependencyGraph(d.Services)

	plan, err := uc.scheduler.Schedule(ctx, d.Services, deps)
	if err != nil {
		return err
	}

	d.Plan = plan
	return nil
}

// deploy executes the deployment.
func (uc *DeployUseCase) deploy(ctx context.Context, d *domain.Deployment) error {
	if d.Plan == nil {
		return orcerrors.ErrNoExecutionPlan
	}

	// Execute phases in order
	for _, phase := range d.Plan.Phases {
		if err := uc.executePhase(ctx, d, phase); err != nil {
			return err
		}
	}

	return nil
}

// executePhase executes a single phase.
func (uc *DeployUseCase) executePhase(ctx context.Context, d *domain.Deployment, phase domain.ExecutionPhase) error {
	if phase.Parallel {
		return uc.executeParallel(ctx, d, phase.Services)
	}
	return uc.executeSequential(ctx, d, phase.Services)
}

// executeParallel executes services in parallel.
func (uc *DeployUseCase) executeParallel(ctx context.Context, d *domain.Deployment, serviceNames []string) error {
	if uc.dispatcher == nil {
		return nil // Skip if no dispatcher (testing)
	}

	tasks := make(map[string]*outbound.Task)

	for _, serviceName := range serviceNames {
		service := uc.findService(d.Services, serviceName)
		if service == nil {
			continue
		}

		// TODO: Get agent assignment from scheduler
		agentID := "agent-1" // Placeholder

		tasks[agentID] = &outbound.Task{
			ID:           uuid.New().String(),
			DeploymentID: d.ID,
			Type:         outbound.TaskDeployService,
			Payload: map[string]interface{}{
				"service": service,
			},
		}
	}

	results, err := uc.dispatcher.DispatchBatch(ctx, tasks)
	if err != nil {
		return err
	}

	// Check results
	for agentID, result := range results {
		if result.Error != "" {
			return fmt.Errorf("agent %s failed: %s", agentID, result.Error)
		}
	}

	return nil
}

// executeSequential executes services sequentially.
func (uc *DeployUseCase) executeSequential(ctx context.Context, d *domain.Deployment, serviceNames []string) error {
	if uc.dispatcher == nil {
		return nil // Skip if no dispatcher (testing)
	}

	for _, serviceName := range serviceNames {
		service := uc.findService(d.Services, serviceName)
		if service == nil {
			continue
		}

		agentID := "agent-1" // Placeholder

		task := &outbound.Task{
			ID:           uuid.New().String(),
			DeploymentID: d.ID,
			Type:         outbound.TaskDeployService,
			Payload: map[string]interface{}{
				"service": service,
			},
		}

		result, err := uc.dispatcher.DispatchTask(ctx, agentID, task)
		if err != nil {
			return err
		}
		if result.Error != "" {
			return fmt.Errorf("deployment of %s failed: %s", serviceName, result.Error)
		}
	}

	return nil
}

// verify verifies the deployment.
func (uc *DeployUseCase) verify(_ context.Context, _ *domain.Deployment) error {
	// Check all services are healthy
	// TODO: Query agent for service health
	return nil
}

// fail marks the deployment as failed.
func (uc *DeployUseCase) fail(ctx context.Context, d *domain.Deployment, stage string, err error) error {
	d.State = domain.StateFailed
	d.Error = fmt.Sprintf("%s failed: %v", stage, err)
	d.UpdatedAt = time.Now()
	_ = uc.repo.Update(ctx, d)
	return err
}

// findService finds a service by name.
func (uc *DeployUseCase) findService(services []domain.Service, name string) *domain.Service {
	for i := range services {
		if services[i].Name == name {
			return &services[i]
		}
	}
	return nil
}

// Ensure DeployUseCase implements OrchestratorService.
var _ inbound.OrchestratorService = (*DeployUseCase)(nil)
