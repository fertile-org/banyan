package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// DeploymentReconciler computes health status for running deployments and
// cleans up superseded deployments. It runs AFTER ContainerReconciler.
type DeploymentReconciler struct {
	deps *ReconcilerDeps
}

// NewDeploymentReconciler creates a DeploymentReconciler.
func NewDeploymentReconciler(deps *ReconcilerDeps) *DeploymentReconciler {
	return &DeploymentReconciler{deps: deps}
}

// Reconcile loads all deployments, computes health status, and marks superseded
// deployments as stopped.
func (d *DeploymentReconciler) Reconcile(ctx context.Context) error {
	keys, err := d.deps.Store.List(ctx, types.KeyDeployments)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	deployments := make([]types.DeploymentRecord, 0, len(keys))
	depKeyMap := make(map[string]string, len(keys))
	for _, key := range keys {
		var dep types.DeploymentRecord
		if err := d.deps.Store.Get(ctx, key, &dep); err != nil {
			continue
		}
		deployments = append(deployments, dep)
		depKeyMap[dep.ID] = key
	}

	d.reconcileHealth(ctx, deployments, depKeyMap)
	d.reconcileSuperseded(ctx, deployments, depKeyMap)

	return nil
}

// reconcileHealth computes the health status for each running deployment
// and persists changes.
func (d *DeploymentReconciler) reconcileHealth(ctx context.Context, deployments []types.DeploymentRecord, depKeyMap map[string]string) {
	for i := range deployments {
		dep := &deployments[i]
		if dep.Status != types.StatusRunning {
			continue
		}

		allTasks := types.CollectDeploymentTasks(ctx, d.deps.Store, dep.ID)
		newHealth := computeHealthStatus(dep, allTasks)
		if newHealth == dep.HealthStatus {
			continue
		}

		// Acquire per-deployment lock.
		lockStore, lockOK := d.deps.Store.(storage.LockStore)
		if lockOK {
			lockCtx, cancel := context.WithTimeout(ctx, reconcileLockTimeout)
			unlock, err := lockStore.Lock(lockCtx, "locks/reconcile/"+dep.ID, reconcileLockTTL)
			cancel()
			if err != nil {
				d.deps.Log.Debug("Skipping health update (lock unavailable)", "deployment", dep.Name, "error", err)
				continue
			}
			defer unlock()
		}

		oldHealth := dep.HealthStatus
		dep.HealthStatus = newHealth
		dep.UpdatedAt = time.Now()

		if newHealth == "stopped" {
			dep.Status = types.StatusStopped
		}

		key := depKeyMap[dep.ID]
		if err := d.deps.Store.Save(ctx, key, dep); err != nil {
			d.deps.Log.Error("Failed to save deployment health", "deployment", dep.Name, "error", err)
			continue
		}

		d.emitHealthTransition(dep, oldHealth, newHealth)
	}
}

// reconcileSuperseded finds the latest deployment per name and marks older
// "running" deployments as stopped.
func (d *DeploymentReconciler) reconcileSuperseded(ctx context.Context, deployments []types.DeploymentRecord, depKeyMap map[string]string) {
	// Find the latest deployment per name (by CreatedAt).
	latestByName := make(map[string]*types.DeploymentRecord)
	for i := range deployments {
		dep := &deployments[i]
		if dep.Status != types.StatusRunning && dep.Status != types.StatusStopped {
			continue
		}
		existing, found := latestByName[dep.Name]
		if !found || dep.CreatedAt.After(existing.CreatedAt) {
			latestByName[dep.Name] = dep
		}
	}

	// Mark older running deployments as stopped.
	for i := range deployments {
		dep := &deployments[i]
		if dep.Status != types.StatusRunning {
			continue
		}
		latest, found := latestByName[dep.Name]
		if !found || dep.ID == latest.ID {
			continue
		}

		dep.Status = types.StatusStopped
		dep.HealthStatus = "stopped"
		dep.UpdatedAt = time.Now()

		key := depKeyMap[dep.ID]
		if err := d.deps.Store.Save(ctx, key, dep); err != nil {
			d.deps.Log.Error("Failed to stop superseded deployment", "deployment", dep.Name, "id", dep.ID, "error", err)
			continue
		}

		d.deps.EmitEvent("reconciler.deployment.superseded",
			fmt.Sprintf("Deployment %s (%s) superseded by %s", dep.Name, dep.ID, latest.ID),
			"info")
	}
}

// emitHealthTransition emits an event when deployment health changes.
func (d *DeploymentReconciler) emitHealthTransition(dep *types.DeploymentRecord, oldHealth, newHealth string) {
	eventType := fmt.Sprintf("reconciler.deployment.%s", newHealth)
	message := fmt.Sprintf("Deployment %s health: %s → %s", dep.Name, oldHealth, newHealth)
	severity := "info"
	if newHealth == "degraded" || newHealth == "recovering" {
		severity = "warning"
	}
	if newHealth == "stopped" {
		severity = "error"
	}
	d.deps.EmitEvent(eventType, message, severity)
}

// computeHealthStatus determines the health of a deployment based on its tasks.
// Returns one of: "healthy", "degraded", "recovering", "stopped".
func computeHealthStatus(dep *types.DeploymentRecord, tasks []types.TaskRecord) string {
	if len(tasks) == 0 {
		return "stopped"
	}

	grouped := types.GroupTasksByService(tasks)

	var totalReplicas int
	var runningCount, exitedCount, pendingCount, recoveringCount int

	for svcName, svcTasks := range grouped {
		svc, ok := dep.Services[svcName]
		if !ok {
			continue
		}
		totalReplicas += svc.Replicas

		// Find the latest task per replica.
		latestByReplica := make(map[int]*types.TaskRecord)
		for i := range svcTasks {
			t := &svcTasks[i]
			// Skip stop_and_remove tasks for health computation.
			if t.Type == types.TaskTypeStopAndRemove {
				continue
			}
			existing, found := latestByReplica[t.ReplicaIndex]
			if !found || t.CreatedAt.After(existing.CreatedAt) {
				latestByReplica[t.ReplicaIndex] = t
			}
		}

		for _, task := range latestByReplica {
			switch {
			case task.ContainerStatus == types.StatusRunning:
				// Recently restarted containers that are now running = recovering.
				if task.RestartCount > 0 && !task.LastRestartAt.IsZero() &&
					time.Since(task.LastRestartAt) < backoffHealthyReset {
					recoveringCount++
				} else {
					runningCount++
				}
			case task.ContainerStatus == "exited":
				exitedCount++
			case task.Status == types.StatusPending:
				pendingCount++
			default:
				pendingCount++
			}
		}
	}

	// Priority-based health determination.
	total := runningCount + exitedCount + pendingCount + recoveringCount
	if total == 0 {
		return "stopped"
	}

	// All exited → stopped.
	if exitedCount == total {
		return "stopped"
	}

	// Any pending or recovering → recovering.
	if pendingCount > 0 || recoveringCount > 0 {
		return "recovering"
	}

	// Any exited (but not all) → degraded.
	if exitedCount > 0 {
		return "degraded"
	}

	return "healthy"
}
