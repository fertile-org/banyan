package engine

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// ContainerReconciler restarts crashed containers on HEALTHY agents based on restart policy.
// It does NOT handle rescheduling to different agents (that is the AgentReconciler's job).
type ContainerReconciler struct {
	deps    *ReconcilerDeps
	backoff *containerBackoffTracker
}

// NewContainerReconciler creates a ContainerReconciler.
func NewContainerReconciler(deps *ReconcilerDeps) *ContainerReconciler {
	return &ContainerReconciler{
		deps:    deps,
		backoff: newContainerBackoffTracker(),
	}
}

// Reconcile lists running deployments, acquires a per-deployment lock, and
// reconciles each deployment's containers.
func (c *ContainerReconciler) Reconcile(ctx context.Context) error {
	keys, err := c.deps.Store.List(ctx, types.KeyDeployments)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	healthyAgents, err := c.loadHealthyAgents(ctx)
	if err != nil {
		return fmt.Errorf("load healthy agents: %w", err)
	}

	for _, key := range keys {
		var dep types.DeploymentRecord
		if err := c.deps.Store.Get(ctx, key, &dep); err != nil {
			continue
		}
		if dep.Status != types.StatusRunning {
			continue
		}

		c.reconcileWithLock(ctx, dep, healthyAgents)
	}

	c.backoff.cleanup()
	return nil
}

// reconcileWithLock acquires a per-deployment distributed lock (if available)
// then calls reconcileDeployment.
func (c *ContainerReconciler) reconcileWithLock(ctx context.Context, dep types.DeploymentRecord, healthyAgents map[string]bool) {
	lockStore, ok := c.deps.Store.(storage.LockStore)
	if ok {
		lockCtx, cancel := context.WithTimeout(ctx, reconcileLockTimeout)
		defer cancel()

		unlock, err := lockStore.Lock(lockCtx, "reconcile/container/"+dep.ID, reconcileLockTTL)
		if err != nil {
			c.deps.Log.Debug("Skipping deployment (lock unavailable)", "deployment", dep.Name, "error", err)
			return
		}
		defer unlock()
	}

	c.reconcileDeployment(ctx, dep, healthyAgents)
}

// reconcileDeployment checks each service replica for crashed containers and
// creates restart tasks when the restart policy allows it.
func (c *ContainerReconciler) reconcileDeployment(ctx context.Context, dep types.DeploymentRecord, healthyAgents map[string]bool) {
	allTasks := types.CollectDeploymentTasks(ctx, c.deps.Store, dep.ID)
	if len(allTasks) == 0 {
		return
	}

	grouped := types.GroupTasksByService(allTasks)

	for svcName, tasks := range grouped {
		svc, ok := dep.Services[svcName]
		if !ok {
			continue
		}

		// Find the latest task per replica (by CreatedAt).
		latestByReplica := make(map[int]*types.TaskRecord)
		for i := range tasks {
			t := &tasks[i]
			existing, found := latestByReplica[t.ReplicaIndex]
			if !found || t.CreatedAt.After(existing.CreatedAt) {
				latestByReplica[t.ReplicaIndex] = t
			}
		}

		// Check if there are any pending restart tasks already.
		hasPending := false
		for i := range tasks {
			if tasks[i].Status == types.StatusPending && tasks[i].Type == types.TaskTypeCreateAndStart {
				hasPending = true
				break
			}
		}
		if hasPending {
			continue
		}

		for _, task := range latestByReplica {
			// Only consider exited containers on healthy agents.
			if task.ContainerStatus != "exited" {
				// If running, mark healthy for backoff reset.
				if task.ContainerStatus == types.StatusRunning {
					backoffKey := fmt.Sprintf("%s-%s-%d", dep.ID, svcName, task.ReplicaIndex)
					c.backoff.markHealthy(backoffKey)
				}
				continue
			}
			if !healthyAgents[task.AgentID] {
				continue
			}
			if task.Type != types.TaskTypeCreateAndStart {
				continue
			}

			if !shouldRestart(task, svc) {
				continue
			}

			backoffKey := fmt.Sprintf("%s-%s-%d", dep.ID, svcName, task.ReplicaIndex)
			if !c.backoff.allows(backoffKey) {
				c.deps.Log.Debug("Backoff: skipping restart", "container", task.ContainerName, "key", backoffKey)
				continue
			}

			c.createRestartTask(ctx, task, &dep)
			c.backoff.recordRestart(backoffKey)
			c.deps.EmitEvent("container.restart",
				fmt.Sprintf("Restarting %s on %s (exit_code=%d, restart_count=%d)",
					task.ContainerName, task.AgentID, task.ExitCode, task.RestartCount),
				"info")
		}
	}
}

// createRestartTask creates a stop_and_remove cleanup task (idempotent) plus a
// create_and_start task on the SAME agent.
func (c *ContainerReconciler) createRestartTask(ctx context.Context, task *types.TaskRecord, dep *types.DeploymentRecord) {
	now := time.Now()
	svc := dep.Services[task.ServiceName]

	newRestartCount := task.RestartCount + 1
	restartID := fmt.Sprintf("%s-%s-%d-r%d", dep.ID, task.ServiceName, task.ReplicaIndex, newRestartCount)

	// Stop-and-remove cleanup (idempotent).
	stopTask := &types.TaskRecord{
		ID:            restartID + "-cleanup",
		DeploymentID:  dep.ID,
		DeploymentName: dep.Name,
		ServiceName:   task.ServiceName,
		ReplicaIndex:  task.ReplicaIndex,
		AgentID:       task.AgentID,
		Type:          types.TaskTypeStopAndRemove,
		Status:        types.StatusPending,
		ContainerName: task.ContainerName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	stopKey := types.KeyTasks + task.AgentID + "/" + stopTask.ID
	if err := c.deps.Store.Save(ctx, stopKey, stopTask); err != nil {
		c.deps.Log.Error("Failed to save cleanup task", "task", stopTask.ID, "error", err)
		return
	}

	// Create-and-start on the same agent.
	startTask := &types.TaskRecord{
		ID:                restartID,
		DeploymentID:      dep.ID,
		DeploymentName:    dep.Name,
		ServiceName:       task.ServiceName,
		ReplicaIndex:      task.ReplicaIndex,
		AgentID:           task.AgentID,
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
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	startKey := types.KeyTasks + task.AgentID + "/" + startTask.ID
	if err := c.deps.Store.Save(ctx, startKey, startTask); err != nil {
		c.deps.Log.Error("Failed to save restart task", "task", startTask.ID, "error", err)
	}
}

// loadHealthyAgents returns a map of agent names that are ready and have a
// recent heartbeat (within agentStalenessThreshold).
func (c *ContainerReconciler) loadHealthyAgents(ctx context.Context) (map[string]bool, error) {
	keys, err := c.deps.Store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil, err
	}

	healthy := make(map[string]bool, len(keys))
	for _, key := range keys {
		var node types.NodeRecord
		if err := c.deps.Store.Get(ctx, key, &node); err != nil {
			continue
		}
		if node.Status == "ready" && time.Since(node.LastSeen) <= agentStalenessThreshold {
			healthy[node.Name] = true
		}
	}
	return healthy, nil
}

// shouldRestart determines whether a crashed container should be restarted
// based on the service restart policy and task state.
func shouldRestart(task *types.TaskRecord, svc types.ServiceRecord) bool {
	policy := svc.Restart
	if policy == "" {
		policy = "always"
	}

	switch {
	case policy == "no":
		return false
	case policy == "always":
		return true
	case policy == "unless-stopped":
		return task.StopReason != "user"
	case strings.HasPrefix(policy, "on-failure"):
		if task.ExitCode == 0 {
			return false
		}
		limit := parseOnFailureLimit(policy)
		if limit > 0 && task.RestartCount >= limit {
			return false
		}
		return true
	default:
		return true
	}
}

// parseOnFailureLimit parses "on-failure:3" -> 3, "on-failure" -> 0 (unlimited).
func parseOnFailureLimit(restart string) int {
	parts := strings.SplitN(restart, ":", 2)
	if len(parts) < 2 {
		return 0
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return n
}

// --- Backoff tracker ---

// backoffEntry tracks per-replica restart backoff state.
type backoffEntry struct {
	consecutiveFailures int
	nextRetryAt         time.Time
	healthySince        time.Time
}

// containerBackoffTracker prevents restart storms with exponential backoff.
type containerBackoffTracker struct {
	mu      sync.Mutex
	entries map[string]*backoffEntry
}

func newContainerBackoffTracker() *containerBackoffTracker {
	return &containerBackoffTracker{
		entries: make(map[string]*backoffEntry),
	}
}

// backoffDelays are the delays for consecutive failures: 0s, 30s, 60s, 5min cap.
var backoffDelays = []time.Duration{
	0,
	30 * time.Second,
	60 * time.Second,
	5 * time.Minute,
}

// backoffHealthyReset is how long a container must stay healthy before
// its backoff counter resets.
const backoffHealthyReset = 10 * time.Minute

// allows returns true if the backoff period has elapsed for the given key.
func (b *containerBackoffTracker) allows(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.entries[key]
	if !ok {
		return true
	}
	return time.Now().After(e.nextRetryAt)
}

// recordRestart increments the failure counter and sets the next retry time.
func (b *containerBackoffTracker) recordRestart(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.entries[key]
	if !ok {
		e = &backoffEntry{}
		b.entries[key] = e
	}

	e.consecutiveFailures++
	e.healthySince = time.Time{} // reset healthy timer

	idx := e.consecutiveFailures
	if idx >= len(backoffDelays) {
		idx = len(backoffDelays) - 1
	}
	e.nextRetryAt = time.Now().Add(backoffDelays[idx])
}

// markHealthy records that a replica is currently running. If it has been
// healthy for longer than backoffHealthyReset, the backoff counter resets.
func (b *containerBackoffTracker) markHealthy(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	e, ok := b.entries[key]
	if !ok {
		return
	}

	if e.healthySince.IsZero() {
		e.healthySince = time.Now()
		return
	}

	if time.Since(e.healthySince) >= backoffHealthyReset {
		delete(b.entries, key)
	}
}

// cleanup removes backoff entries that have been healthy long enough.
func (b *containerBackoffTracker) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, e := range b.entries {
		if !e.healthySince.IsZero() && time.Since(e.healthySince) >= backoffHealthyReset {
			delete(b.entries, key)
		}
	}
}
