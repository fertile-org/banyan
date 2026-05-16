package engine

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// AgentReconciler detects stale agents and reschedules their work to healthy agents.
// It runs BEFORE ContainerReconciler in the reconciliation pipeline.
type AgentReconciler struct {
	deps *ReconcilerDeps
}

// NewAgentReconciler creates an AgentReconciler.
func NewAgentReconciler(deps *ReconcilerDeps) *AgentReconciler {
	return &AgentReconciler{deps: deps}
}

// Grace periods for stale agent detection.
const (
	agentGracePeriodStandard  = 2 * time.Minute
	agentGracePeriodExtended  = 5 * time.Minute
	agentLongRunningThreshold = 10 * time.Minute
	rescheduleCooldown        = 5 * time.Minute
)

// agentGracePeriod returns the grace period for a node based on how long it has been registered.
func agentGracePeriod(node types.NodeRecord) time.Duration {
	if time.Since(node.CreatedAt) > agentLongRunningThreshold {
		return agentGracePeriodExtended
	}
	return agentGracePeriodStandard
}

// Reconcile lists all nodes, identifies stale agents past their grace period,
// marks their tasks as exited, creates cleanup tasks, and reschedules work
// onto healthy agents.
func (a *AgentReconciler) Reconcile(ctx context.Context) error {
	nodeKeys, err := a.deps.Store.List(ctx, types.KeyNodes)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}

	// Load all nodes and partition into stale vs healthy.
	var staleNodes []types.NodeRecord
	var healthyNodes []types.NodeRecord
	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := a.deps.Store.Get(ctx, key, &node); err != nil {
			continue
		}
		if node.Status != "ready" {
			continue
		}
		staleDuration := time.Since(node.LastSeen)
		grace := agentGracePeriod(node)
		if staleDuration > grace {
			staleNodes = append(staleNodes, node)
		} else if staleDuration <= agentStalenessThreshold {
			healthyNodes = append(healthyNodes, node)
		}
	}

	if len(staleNodes) == 0 {
		return nil
	}

	// Load all deployments keyed by ID.
	depKeys, err := a.deps.Store.List(ctx, types.KeyDeployments)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	deployments := make(map[string]*types.DeploymentRecord, len(depKeys))
	for _, key := range depKeys {
		var dep types.DeploymentRecord
		if err := a.deps.Store.Get(ctx, key, &dep); err != nil {
			continue
		}
		deployments[dep.ID] = &dep
	}

	// Shared batch memory tracker across all rescheduling in this Reconcile() call.
	batchMemory := make(map[string]uint64, len(healthyNodes))

	for _, staleNode := range staleNodes {
		a.deps.EmitEvent("reconciler.agent.stale",
			fmt.Sprintf("Agent %s is stale (last seen %s ago)", staleNode.Name, time.Since(staleNode.LastSeen).Truncate(time.Second)),
			"warning")

		a.reconcileStaleAgent(ctx, staleNode, healthyNodes, deployments, batchMemory)
	}

	return nil
}

// reconcileStaleAgent processes all tasks on a stale agent: marks them exited,
// creates cleanup tasks, and reschedules eligible work.
func (a *AgentReconciler) reconcileStaleAgent(
	ctx context.Context,
	staleNode types.NodeRecord,
	healthyNodes []types.NodeRecord,
	deployments map[string]*types.DeploymentRecord,
	batchMemory map[string]uint64,
) {
	taskKeys, err := a.deps.Store.List(ctx, types.KeyTasks+staleNode.Name+"/")
	if err != nil {
		return
	}

	// Pre-build pending replacement sets per deployment. This replaces the
	// old hasPendingReplacement() which scanned ALL agents per task — O(N*M).
	// Now we scan once per deployment — O(agents*tasks) total, not per-task.
	pendingByDep := make(map[string]map[string]bool) // depID → set of "svc-replica" keys

	for _, taskKey := range taskKeys {
		var task types.TaskRecord
		if err := a.deps.Store.Get(ctx, taskKey, &task); err != nil {
			continue
		}

		// Only process create_and_start tasks that are still "running".
		if task.Type != types.TaskTypeCreateAndStart {
			continue
		}
		if task.ContainerStatus == "exited" || task.ContainerStatus == "" {
			continue
		}

		dep, ok := deployments[task.DeploymentID]
		if !ok || dep.Status != types.StatusRunning {
			continue
		}

		// Acquire per-deployment lock.
		lockStore, lockOK := a.deps.Store.(storage.LockStore)
		if lockOK {
			lockCtx, cancel := context.WithTimeout(ctx, reconcileLockTimeout)
			unlock, err := lockStore.Lock(lockCtx, "locks/reconcile/"+dep.ID, reconcileLockTTL)
			cancel()
			if err != nil {
				a.deps.Log.Debug("Skipping deployment (lock unavailable)", "deployment", dep.Name, "error", err)
				continue
			}
			defer unlock()
		}

		svc, svcOK := dep.Services[task.ServiceName]
		if !svcOK {
			continue
		}

		// Mark the task as exited with agent_stale reason.
		task.ContainerStatus = "exited"
		task.StopReason = "agent_stale"
		task.UpdatedAt = time.Now()
		if err := a.deps.Store.Save(ctx, taskKey, &task); err != nil {
			a.deps.Log.Error("Failed to mark task exited", "task", task.ID, "error", err)
			continue
		}

		// Create idempotent cleanup task on the stale agent.
		a.createCleanupTask(ctx, &task, dep)

		// Check if we should reschedule.
		if !shouldRescheduleOnAgentDeath(svc) {
			continue
		}

		if isStatefullyPinned(&task, svc) {
			continue
		}

		// Anti-flapping: skip if recently restarted.
		if !task.LastRestartAt.IsZero() && time.Since(task.LastRestartAt) < rescheduleCooldown {
			continue
		}

		// Check for existing pending replacement (lazily build set per deployment).
		pending, built := pendingByDep[dep.ID]
		if !built {
			pending = buildPendingSet(ctx, a.deps.Store, dep.ID)
			pendingByDep[dep.ID] = pending
		}
		pendingKey := fmt.Sprintf("%s-%d", task.ServiceName, task.ReplicaIndex)
		if pending[pendingKey] {
			continue
		}

		// Find eligible healthy agents.
		eligible := a.filterEligibleAgents(healthyNodes, dep, svc)
		if len(eligible) == 0 {
			a.deps.Log.Warn("No healthy agents for reschedule", "task", task.ID)
			continue
		}

		// Pick the best agent using resource-aware scheduling.
		resReq := types.ServiceResourceRequest(svc)
		agent := types.PickAgentByResources(eligible, batchMemory, resReq)
		batchMemory[agent.Name] += resReq.MemoryBytes

		a.createRescheduleTask(ctx, &task, dep, svc, agent.Name)

		// Mark this replica as having a pending replacement so we don't
		// create duplicates for other tasks on the same replica.
		pending[pendingKey] = true
	}
}

// createCleanupTask creates an idempotent stop_and_remove task on the stale agent.
func (a *AgentReconciler) createCleanupTask(ctx context.Context, task *types.TaskRecord, dep *types.DeploymentRecord) {
	now := time.Now()
	cleanupID := fmt.Sprintf("%s-agent-cleanup", task.ID)

	cleanup := &types.TaskRecord{
		ID:             cleanupID,
		DeploymentID:   dep.ID,
		DeploymentName: dep.Name,
		ServiceName:    task.ServiceName,
		ReplicaIndex:   task.ReplicaIndex,
		AgentID:        task.AgentID,
		Type:           types.TaskTypeStopAndRemove,
		Status:         types.StatusPending,
		ContainerName:  task.ContainerName,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	key := types.KeyTasks + task.AgentID + "/" + cleanupID
	if err := a.deps.Store.Save(ctx, key, cleanup); err != nil {
		a.deps.Log.Error("Failed to save cleanup task", "task", cleanupID, "error", err)
	}
}

// createRescheduleTask creates a new create_and_start task on a healthy agent.
func (a *AgentReconciler) createRescheduleTask(
	ctx context.Context,
	task *types.TaskRecord,
	dep *types.DeploymentRecord,
	svc types.ServiceRecord,
	targetAgent string,
) {
	now := time.Now()
	newRestartCount := task.RestartCount + 1
	rescheduleID := fmt.Sprintf("%s-%s-%d-rs%d", dep.ID, task.ServiceName, task.ReplicaIndex, newRestartCount)

	newTask := &types.TaskRecord{
		ID:                rescheduleID,
		DeploymentID:      dep.ID,
		DeploymentName:    dep.Name,
		ServiceName:       task.ServiceName,
		ReplicaIndex:      task.ReplicaIndex,
		AgentID:           targetAgent,
		Type:              types.TaskTypeCreateAndStart,
		Status:            types.StatusPending,
		Image:             svc.Image,
		ContainerName:     task.ContainerName,
		Ports:             svc.Ports,
		Environment:       svc.Environment,
		Command:           svc.Command,
		Entrypoint:        svc.Entrypoint,
		Restart:           svc.Restart,
		MemoryLimit:       svc.MemoryLimit,
		CPULimit:          svc.CPULimit,
		MemoryReservation: svc.MemoryReservation,
		Healthcheck:       svc.Healthcheck,
		Volumes:           svc.Volumes,
		SecretRefs:        svc.Secrets,
		RestartCount:      newRestartCount,
		LastRestartAt:     now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	key := types.KeyTasks + targetAgent + "/" + rescheduleID
	if err := a.deps.Store.Save(ctx, key, newTask); err != nil {
		a.deps.Log.Error("Failed to save reschedule task", "task", rescheduleID, "error", err)
		return
	}

	a.deps.EmitEvent("reconciler.container.rescheduled",
		fmt.Sprintf("Rescheduled %s from %s to %s (restart_count=%d)",
			task.ContainerName, task.AgentID, targetAgent, newRestartCount),
		"warning")
}

// filterEligibleAgents returns healthy agents that match deployment tags and service placement.
func (a *AgentReconciler) filterEligibleAgents(
	healthyNodes []types.NodeRecord,
	dep *types.DeploymentRecord,
	svc types.ServiceRecord,
) []types.NodeRecord {
	var eligible []types.NodeRecord
	for i := range healthyNodes {
		node := healthyNodes[i]
		if !types.TagsMatch(node.Tags, dep.Tags) {
			continue
		}
		if svc.Placement != "" && !matchPlacement(node.Name, svc.Placement) {
			continue
		}
		eligible = append(eligible, node)
	}
	return eligible
}

// buildPendingSet collects all pending create_and_start tasks for a deployment
// and returns a set keyed by "serviceName-replicaIndex". This is called once
// per deployment instead of once per task, reducing O(tasks*nodes*tasks) to
// O(nodes*tasks) via CollectDeploymentTasks.
func buildPendingSet(ctx context.Context, store storage.StateStore, depID string) map[string]bool {
	tasks := types.CollectDeploymentTasks(ctx, store, depID)
	set := make(map[string]bool)
	for i := range tasks {
		t := &tasks[i]
		if t.Type == types.TaskTypeCreateAndStart && t.Status == types.StatusPending {
			key := fmt.Sprintf("%s-%d", t.ServiceName, t.ReplicaIndex)
			set[key] = true
		}
	}
	return set
}

// shouldRescheduleOnAgentDeath returns true unless the service restart policy is "no".
func shouldRescheduleOnAgentDeath(svc types.ServiceRecord) bool {
	return svc.Restart != "no"
}

// isStatefullyPinned returns true if the task/service is pinned to a specific agent
// due to placement constraints or non-portable volumes.
func isStatefullyPinned(task *types.TaskRecord, svc types.ServiceRecord) bool {
	_ = task // task included for future extensions
	if svc.Placement != "" {
		return true
	}
	for _, vol := range svc.Volumes {
		if vol.Type != "tmpfs" && vol.Type != "nfs" && vol.Type != "" {
			return true
		}
	}
	return false
}

// matchPlacement checks if an agent name matches a glob placement pattern.
func matchPlacement(agentName, pattern string) bool {
	ok, _ := path.Match(pattern, agentName)
	return ok
}
