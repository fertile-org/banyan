package types

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// BuildServiceRecords converts manifest services to deployment service records.
// Services with 0 replicas default to 1.
func BuildServiceRecords(manifest map[string]ManifestService) map[string]ServiceRecord {
	services := make(map[string]ServiceRecord, len(manifest))
	for name, svc := range manifest { //nolint:gocritic // map iteration
		replicas := svc.GetReplicas()
		if replicas == 0 {
			replicas = 1
		}
		services[name] = ServiceRecord{
			Image:       svc.Image,
			Replicas:    replicas,
			Ports:       svc.Ports,
			Environment: svc.Environment,
			Command:     svc.Command,
			DependsOn:   svc.DependsOn,
		}
	}
	return services
}

// BuildTasksForDeployment creates task records for a deployment, distributing
// replicas round-robin across the given agents.
// When ReplacesID is set (blue-green deployment), container names use the deployment ID
// as prefix to avoid naming conflicts with the still-running old deployment.
func BuildTasksForDeployment(deployment *DeploymentRecord, agents []NodeRecord) []*TaskRecord {
	containerPrefix := deployment.Name
	if deployment.ReplacesID != "" {
		containerPrefix = deployment.ID
	}

	var tasks []*TaskRecord
	agentIdx := 0
	for svcName, svc := range deployment.Services {
		for i := 0; i < svc.Replicas; i++ {
			agent := agents[agentIdx%len(agents)]
			agentIdx++

			now := time.Now()
			tasks = append(tasks, &TaskRecord{
				ID:            fmt.Sprintf("%s-%s-%d", deployment.ID, svcName, i),
				DeploymentID:  deployment.ID,
				ServiceName:   svcName,
				ReplicaIndex:  i,
				AgentID:       agent.Name,
				Type:          TaskTypeCreateAndStart,
				Status:        StatusPending,
				Image:         svc.Image,
				ContainerName: fmt.Sprintf("%s-%s-%d", containerPrefix, svcName, i),
				Ports:         svc.Ports,
				Environment:   svc.Environment,
				Command:       svc.Command,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
		}
	}
	return tasks
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
