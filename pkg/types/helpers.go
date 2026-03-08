package types

import (
	"context"
	"fmt"
	"path"
	"sort"
	"time"
)

// TagsMatch returns true if an agent with agentTags can run a deployment with deploymentTags.
// Both empty = match. One empty = no match. Both non-empty = match if any intersection.
func TagsMatch(agentTags, deploymentTags []string) bool {
	if len(agentTags) == 0 && len(deploymentTags) == 0 {
		return true
	}
	if len(agentTags) == 0 || len(deploymentTags) == 0 {
		return false
	}
	for _, dt := range deploymentTags {
		for _, at := range agentTags {
			if dt == at {
				return true
			}
		}
	}
	return false
}

// TagsEqual returns true if two tag sets contain the same tags (order-independent).
func TagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa := SortTags(a)
	sb := SortTags(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// SortTags returns a sorted copy of the given tags slice.
func SortTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return sorted
}

// BuildServiceRecords converts manifest services to deployment service records.
// Services with 0 replicas default to 1.
func BuildServiceRecords(manifest map[string]ManifestService) map[string]ServiceRecord {
	services := make(map[string]ServiceRecord, len(manifest))
	for name, svc := range manifest { //nolint:gocritic // map iteration
		replicas := svc.GetReplicas()
		if replicas == 0 {
			replicas = 1
		}
		var placement string
		if svc.Deploy != nil && svc.Deploy.Placement != nil {
			placement = svc.Deploy.Placement.Node
		}
		var memLimit, cpuLimit, memReservation string
		if svc.Deploy != nil && svc.Deploy.Resources != nil {
			if svc.Deploy.Resources.Limits != nil {
				memLimit = svc.Deploy.Resources.Limits.Memory
				cpuLimit = svc.Deploy.Resources.Limits.CPUs
			}
			if svc.Deploy.Resources.Reservations != nil {
				memReservation = svc.Deploy.Resources.Reservations.Memory
			}
		}
		services[name] = ServiceRecord{
			Image:             svc.Image,
			Replicas:          replicas,
			Placement:         placement,
			Ports:             svc.Ports,
			Environment:       svc.Environment,
			Command:           svc.Command,
			Entrypoint:        svc.Entrypoint,
			DependsOn:         svc.DependsOn,
			Restart:           svc.Restart,
			MemoryLimit:       memLimit,
			CPULimit:          cpuLimit,
			MemoryReservation: memReservation,
			Healthcheck:       svc.Healthcheck,
		}
	}
	return services
}

// BuildTasksForDeployment creates task records for a deployment, distributing
// replicas round-robin across the given agents.
// When ReplacesID is set (blue-green deployment), container names use the deployment ID
// as prefix to avoid naming conflicts with the still-running old deployment.
// Services with deploy.placement.node are scheduled only on matching agents (glob pattern).
func BuildTasksForDeployment(deployment *DeploymentRecord, agents []NodeRecord) ([]*TaskRecord, error) {
	containerPrefix := deployment.Name
	if deployment.ReplacesID != "" {
		containerPrefix = deployment.ID
	}

	var tasks []*TaskRecord
	agentIdx := 0
	for svcName := range deployment.Services {
		svc := deployment.Services[svcName]
		eligible := agents
		if svc.Placement != "" {
			eligible = filterAgentsByPlacement(agents, svc.Placement)
			if len(eligible) == 0 {
				return nil, fmt.Errorf("service %q has placement %q but no agents match", svcName, svc.Placement)
			}
		}

		for i := 0; i < svc.Replicas; i++ {
			agent := eligible[agentIdx%len(eligible)]
			agentIdx++

			now := time.Now()
			tasks = append(tasks, &TaskRecord{
				ID:                fmt.Sprintf("%s-%s-%d", deployment.ID, svcName, i),
				DeploymentID:      deployment.ID,
				DeploymentName:    deployment.Name,
				ServiceName:       svcName,
				ReplicaIndex:      i,
				AgentID:           agent.Name,
				Type:              TaskTypeCreateAndStart,
				Status:            StatusPending,
				Image:             svc.Image,
				ContainerName:     fmt.Sprintf("%s-%s-%d", containerPrefix, svcName, i),
				Ports:             svc.Ports,
				Environment:       svc.Environment,
				Command:           svc.Command,
				Entrypoint:        svc.Entrypoint,
				Restart:           svc.Restart,
				MemoryLimit:       svc.MemoryLimit,
				CPULimit:          svc.CPULimit,
				MemoryReservation: svc.MemoryReservation,
				Healthcheck:       svc.Healthcheck,
				CreatedAt:         now,
				UpdatedAt:         now,
			})
		}
	}
	return tasks, nil
}

// filterAgentsByPlacement returns agents whose name matches the glob pattern.
func filterAgentsByPlacement(agents []NodeRecord, pattern string) []NodeRecord {
	var matched []NodeRecord
	for _, agent := range agents {
		if ok, _ := path.Match(pattern, agent.Name); ok {
			matched = append(matched, agent)
		}
	}
	return matched
}

// DetermineDeploymentStatus returns the new deployment status based on task counts.
// Returns empty string if no status change should occur.
func DetermineDeploymentStatus(totalTasks, completedTasks, failedTasks int, firstError string) (status, errMsg string) {
	if totalTasks == 0 {
		return "", ""
	}
	if failedTasks > 0 {
		return StatusFailed, fmt.Sprintf("%d/%d tasks failed: %s", failedTasks, totalTasks, firstError)
	}
	if completedTasks == totalTasks {
		return StatusRunning, ""
	}
	return "", ""
}

// GroupTasksByService groups tasks by their ServiceName.
func GroupTasksByService(tasks []TaskRecord) map[string][]TaskRecord {
	grouped := make(map[string][]TaskRecord)
	for i := range tasks {
		grouped[tasks[i].ServiceName] = append(grouped[tasks[i].ServiceName], tasks[i])
	}
	return grouped
}

// SortedServiceNames returns sorted service names from a grouped tasks map.
func SortedServiceNames(grouped map[string][]TaskRecord) []string {
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateServiceDependencies checks that every target service has its depends_on
// entries satisfied — either already running or being deployed in this batch.
func ValidateServiceDependencies(targetServices []string, allServices map[string]ServiceRecord, runningServiceNames []string) error {
	targetSet := make(map[string]bool, len(targetServices))
	for _, name := range targetServices {
		targetSet[name] = true
	}

	runningSet := make(map[string]bool, len(runningServiceNames))
	for _, name := range runningServiceNames {
		runningSet[name] = true
	}

	for _, svcName := range targetServices {
		svc, ok := allServices[svcName]
		if !ok {
			continue
		}
		for _, dep := range svc.DependsOn {
			if !targetSet[dep] && !runningSet[dep] {
				return fmt.Errorf("service %q depends on %q which is not running and not being deployed", svcName, dep)
			}
		}
	}
	return nil
}

// CollectDeploymentTasks gathers all tasks for a given deployment across all agents.
func CollectDeploymentTasks(ctx context.Context, store StateStore, deploymentID string) []TaskRecord {
	nodeKeys, err := store.List(ctx, KeyNodes)
	if err != nil {
		return nil
	}

	var tasks []TaskRecord
	for _, nodeKey := range nodeKeys {
		var node NodeRecord
		if err := store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskKeys, err := store.List(ctx, KeyTasks+node.Name+"/")
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task TaskRecord
			if err := store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.DeploymentID == deploymentID {
				tasks = append(tasks, task)
			}
		}
	}
	return tasks
}
