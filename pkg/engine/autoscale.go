package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/types"
)

// evaluateAutoscale checks all running deployments for autoscale config and
// adjusts replica counts based on per-container CPU metrics.
//
// Algorithm:
//   - Scale up:   avg CPU > target_cpu → add 1 replica (up to max)
//   - Scale down:  avg CPU < target_cpu * 0.5 → remove 1 replica (down to min)
//   - Cooldown:   minimum cooldown duration between consecutive scale events
func (e *Engine) evaluateAutoscale(ctx context.Context) {
	keys, err := e.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return
	}

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := e.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Status != types.StatusRunning {
			continue
		}

		allTasks := types.CollectDeploymentTasks(ctx, e.store, record.ID)

		for svcName, svc := range record.Services {
			if svc.Autoscale == nil || svc.Autoscale.Max <= 0 {
				continue
			}

			as := svc.Autoscale

			// Collect CPU metrics for running containers of this service
			var cpuSum float64
			var cpuCount int
			for i := range allTasks {
				if allTasks[i].ServiceName != svcName || allTasks[i].Type != types.TaskTypeCreateAndStart {
					continue
				}
				if allTasks[i].Status != types.StatusCompleted || allTasks[i].ContainerStatus != types.StatusRunning {
					continue
				}
				if allTasks[i].CPUPercent > 0 {
					cpuSum += allTasks[i].CPUPercent
					cpuCount++
				}
			}

			if cpuCount == 0 {
				continue // no metrics yet
			}

			avgCPU := cpuSum / float64(cpuCount)
			currentReplicas := svc.Replicas

			// Check cooldown
			cooldown := parseDuration(as.Cooldown, 60*time.Second)
			if !svc.LastScaleAt.IsZero() && time.Since(svc.LastScaleAt) < cooldown {
				continue
			}

			var targetReplicas int
			if as.TargetCPU > 0 && avgCPU > float64(as.TargetCPU) && currentReplicas < as.Max {
				// Scale up by 1
				targetReplicas = currentReplicas + 1
				if targetReplicas > as.Max {
					targetReplicas = as.Max
				}
			} else if as.TargetCPU > 0 && avgCPU < float64(as.TargetCPU)/2 && currentReplicas > as.Min {
				// Scale down by 1 (hysteresis: threshold = target/2)
				targetReplicas = currentReplicas - 1
				if targetReplicas < as.Min {
					targetReplicas = as.Min
				}
			} else {
				continue // no scaling needed
			}

			if targetReplicas == currentReplicas {
				continue
			}

			e.logger().Info("Autoscale: adjusting replicas",
				"deployment", record.Name, "service", svcName,
				"from", currentReplicas, "to", targetReplicas,
				"avg_cpu", fmt.Sprintf("%.1f%%", avgCPU),
				"target_cpu", as.TargetCPU)

			// Execute scale using the same logic as the Scale RPC
			e.scaleService(ctx, &record, key, svcName, targetReplicas)
		}
	}
}

// scaleService adds or removes replicas for a single service in a running deployment.
// Used by both the Scale RPC handler and the autoscale evaluation loop.
func (e *Engine) scaleService(ctx context.Context, deployment *types.DeploymentRecord, deploymentKey, svcName string, target int) {
	svc := deployment.Services[svcName]

	allTasks := types.CollectDeploymentTasks(ctx, e.store, deployment.ID)
	var runningTasks []types.TaskRecord
	for _, t := range allTasks {
		if t.ServiceName == svcName && t.Type == types.TaskTypeCreateAndStart && t.Status == types.StatusCompleted {
			runningTasks = append(runningTasks, t)
		}
	}

	current := len(runningTasks)

	if target > current {
		// Scale up
		agents, agentErr := ListAvailableAgents(ctx, e.store, deployment.Tags)
		if agentErr != nil || len(agents) == 0 {
			e.logger().Warn("Autoscale: no available agents", "service", svcName)
			return
		}
		for i := current; i < target; i++ {
			agent := agents[i%len(agents)]
			now := time.Now()
			task := &types.TaskRecord{
				ID:                fmt.Sprintf("%s-%s-%d", deployment.ID, svcName, i),
				DeploymentID:      deployment.ID,
				DeploymentName:    deployment.Name,
				ServiceName:       svcName,
				ReplicaIndex:      i,
				AgentID:           agent.Name,
				Type:              types.TaskTypeCreateAndStart,
				Status:            types.StatusPending,
				Image:             svc.Image,
				ContainerName:     fmt.Sprintf("%s-%s-%d", deployment.Name, svcName, i),
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
				CreatedAt:         now,
				UpdatedAt:         now,
			}
			taskKey := types.KeyTasks + agent.Name + "/" + task.ID
			_ = e.store.Save(ctx, taskKey, task)
		}
	} else if target < current {
		// Scale down (highest index first)
		for i := current - 1; i >= target; i-- {
			if i >= len(runningTasks) {
				continue
			}
			orig := runningTasks[i]
			stopTask := &types.TaskRecord{
				ID:            orig.ID + "-stop",
				DeploymentID:  orig.DeploymentID,
				ServiceName:   orig.ServiceName,
				ReplicaIndex:  orig.ReplicaIndex,
				AgentID:       orig.AgentID,
				Type:          types.TaskTypeStopAndRemove,
				Status:        types.StatusPending,
				ContainerName: orig.ContainerName,
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
			}
			taskKey := types.KeyTasks + orig.AgentID + "/" + stopTask.ID
			_ = e.store.Save(ctx, taskKey, stopTask)
		}
	}

	// Update deployment record
	svc.Replicas = target
	svc.LastScaleAt = time.Now()
	deployment.Services[svcName] = svc
	deployment.UpdatedAt = time.Now()
	_ = e.store.Save(ctx, deploymentKey, deployment)
}

// parseDuration parses a duration string, returning defaultVal on error.
func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

// rebalanceMigrationCooldown tracks recently migrated containers to prevent ping-pong.
// Key: container name, value: time of last migration.
var rebalanceMigrationCooldown = make(map[string]time.Time)

// rebalanceCooldownDuration is the minimum time before a container can be migrated again.
const rebalanceCooldownDuration = 10 * time.Minute

// rebalanceOverloadThreshold is the CPU% above which an agent is considered overloaded.
// Set high (95%) because high CPU is often fine — only intervene when it's truly problematic.
const rebalanceOverloadThreshold = 95.0

// rebalanceTargetMaxCPU is the max CPU% the target agent should have AFTER migration.
// Prevents migrating a container to an agent that would also become overloaded.
const rebalanceTargetMaxCPU = 70.0

// rebalanceMinImbalance is the minimum CPU% difference between source and target
// required before a migration is considered worthwhile.
const rebalanceMinImbalance = 30.0

// evaluateRebalance checks for imbalanced agents and migrates stateless containers.
//
// Safeguards against infinite rebalancing:
//  1. Per-container cooldown (10 min) — prevents ping-pong
//  2. High threshold (95%) — only intervene when truly problematic
//  3. Target validation — won't overload destination (must stay < 70% after migration)
//  4. Minimum imbalance (30%) — source and target must differ significantly
//  5. Max 1 migration per agent per cycle
func (e *Engine) evaluateRebalance(ctx context.Context) {
	nodeKeys, err := e.store.List(ctx, types.KeyNodes)
	if err != nil {
		return
	}

	type agentInfo struct {
		node types.NodeRecord
		cpu  float64
	}

	var agents []agentInfo
	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := e.store.Get(ctx, key, &node); err != nil {
			continue
		}
		if node.Status != "ready" || time.Since(node.LastSeen) > 60*time.Second {
			continue
		}
		agents = append(agents, agentInfo{node: node, cpu: node.CPUUsageRatio * 100})
	}

	if len(agents) < 2 {
		return
	}

	// Find overloaded and underloaded agents
	var overloaded, underloaded []agentInfo
	for _, a := range agents {
		if a.cpu > rebalanceOverloadThreshold {
			overloaded = append(overloaded, a)
		} else if a.cpu < rebalanceTargetMaxCPU {
			underloaded = append(underloaded, a)
		}
	}

	if len(overloaded) == 0 || len(underloaded) == 0 {
		return
	}

	// Clean up expired cooldown entries
	for name, t := range rebalanceMigrationCooldown {
		if time.Since(t) > rebalanceCooldownDuration {
			delete(rebalanceMigrationCooldown, name)
		}
	}

	// For each overloaded agent, try to migrate one stateless container
	migrated := make(map[string]bool)
	for _, over := range overloaded {
		if migrated[over.node.Name] {
			continue // max 1 migration per agent per cycle
		}

		// Find migratable container (not recently migrated)
		candidate := e.findMigratableTask(ctx, over.node.Name)
		if candidate == nil {
			continue
		}

		// Check per-container cooldown — prevent ping-pong
		if lastMigrated, ok := rebalanceMigrationCooldown[candidate.ContainerName]; ok {
			if time.Since(lastMigrated) < rebalanceCooldownDuration {
				continue // recently migrated, skip
			}
		}

		// Pick the least loaded target agent
		var bestTarget *agentInfo
		for i := range underloaded {
			// Safeguard: require minimum imbalance between source and target
			if over.cpu-underloaded[i].cpu < rebalanceMinImbalance {
				continue
			}
			// Safeguard: estimate target CPU after migration — don't overload it
			// (rough estimate: add container's CPU% to target agent)
			estimatedTargetCPU := underloaded[i].cpu + candidate.CPUPercent
			if estimatedTargetCPU > rebalanceTargetMaxCPU {
				continue // would overload the target
			}
			if bestTarget == nil || underloaded[i].cpu < bestTarget.cpu {
				bestTarget = &underloaded[i]
			}
		}
		if bestTarget == nil {
			continue // no suitable target found
		}

		e.logger().Info("Rebalance: migrating container",
			"container", candidate.ContainerName,
			"from", over.node.Name, "to", bestTarget.node.Name,
			"from_cpu", fmt.Sprintf("%.1f%%", over.cpu),
			"to_cpu", fmt.Sprintf("%.1f%%", bestTarget.cpu),
			"container_cpu", fmt.Sprintf("%.1f%%", candidate.CPUPercent))

		e.migrateTask(ctx, candidate, over.node.Name, bestTarget.node.Name)
		migrated[over.node.Name] = true
		rebalanceMigrationCooldown[candidate.ContainerName] = time.Now()
	}
}

// findMigratableTask finds a container on the given agent that can be migrated.
// Returns nil if no suitable container found.
func (e *Engine) findMigratableTask(ctx context.Context, agentName string) *types.TaskRecord {
	taskPrefix := types.KeyTasks + agentName + "/"
	keys, err := e.store.List(ctx, taskPrefix)
	if err != nil {
		return nil
	}

	for _, key := range keys {
		var task types.TaskRecord
		if err := e.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Type != types.TaskTypeCreateAndStart || task.Status != types.StatusCompleted {
			continue
		}
		if task.ContainerStatus != types.StatusRunning {
			continue
		}
		// Skip stateful containers (have volumes)
		if len(task.Volumes) > 0 {
			continue
		}
		// Look up service to check placement constraint
		var dep types.DeploymentRecord
		if err := e.store.Get(ctx, types.KeyDeployments+task.DeploymentID, &dep); err != nil {
			continue
		}
		if svc, ok := dep.Services[task.ServiceName]; ok {
			if svc.Placement != "" {
				continue // pinned service
			}
		}
		return &task
	}
	return nil
}

// migrateTask stops a container on the old agent and starts it on the new agent.
func (e *Engine) migrateTask(ctx context.Context, task *types.TaskRecord, fromAgent, toAgent string) {
	now := time.Now()

	// Create stop task on old agent
	stopTask := &types.TaskRecord{
		ID:            task.ID + "-migrate-stop",
		DeploymentID:  task.DeploymentID,
		ServiceName:   task.ServiceName,
		ReplicaIndex:  task.ReplicaIndex,
		AgentID:       fromAgent,
		Type:          types.TaskTypeStopAndRemove,
		Status:        types.StatusPending,
		ContainerName: task.ContainerName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	_ = e.store.Save(ctx, types.KeyTasks+fromAgent+"/"+stopTask.ID, stopTask)

	// Create start task on new agent
	startTask := &types.TaskRecord{
		ID:                task.ID + "-migrate",
		DeploymentID:      task.DeploymentID,
		DeploymentName:    task.DeploymentName,
		ServiceName:       task.ServiceName,
		ReplicaIndex:      task.ReplicaIndex,
		AgentID:           toAgent,
		Type:              types.TaskTypeCreateAndStart,
		Status:            types.StatusPending,
		Image:             task.Image,
		ContainerName:     task.ContainerName + "-m",
		Ports:             task.Ports,
		Environment:       task.Environment,
		Command:           task.Command,
		Entrypoint:        task.Entrypoint,
		Restart:           task.Restart,
		MemoryLimit:       task.MemoryLimit,
		CPULimit:          task.CPULimit,
		MemoryReservation: task.MemoryReservation,
		Healthcheck:       task.Healthcheck,
		Volumes:           task.Volumes,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	_ = e.store.Save(ctx, types.KeyTasks+toAgent+"/"+startTask.ID, startTask)

	e.emitEvent("rebalance.migrate",
		fmt.Sprintf("Migrating %s from %s to %s", task.ContainerName, fromAgent, toAgent), "info")
}

// ListAvailableAgentsForScale is exported for use by the Scale RPC handler.
var ListAvailableAgentsForScale = ListAvailableAgents
