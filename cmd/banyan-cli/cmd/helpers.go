package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// buildServiceRecords converts manifest services to deployment service records.
// Services with 0 replicas default to 1.
func buildServiceRecords(manifest map[string]ManifestService) map[string]ServiceRecord {
	services := make(map[string]ServiceRecord, len(manifest))
	for name, svc := range manifest {
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

// buildTasksForDeployment creates task records for a deployment, distributing
// replicas round-robin across the given agents.
func buildTasksForDeployment(deployment *DeploymentRecord, agents []NodeRecord) []*TaskRecord {
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
				Type:          taskTypeCreateAndStart,
				Status:        statusPending,
				Image:         svc.Image,
				ContainerName: fmt.Sprintf("%s-%s-%d", deployment.Name, svcName, i),
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

// determineDeploymentStatus returns the new deployment status based on task counts.
// Returns empty string if no status change should occur (tasks not ready or still in progress).
func determineDeploymentStatus(totalTasks, completedTasks, failedTasks int, firstError string) (status string, errMsg string) {
	if totalTasks == 0 {
		return "", ""
	}
	if failedTasks > 0 {
		return statusFailed, fmt.Sprintf("%d/%d tasks failed: %s", failedTasks, totalTasks, firstError)
	}
	if completedTasks == totalTasks {
		return statusRunning, ""
	}
	return "", "" // still in progress
}

// buildNerdctlRunArgs builds the argument list for "nerdctl run" from a task.
func buildNerdctlRunArgs(task *TaskRecord) []string {
	args := []string{"run", "-d", "--name", task.ContainerName}

	for _, port := range task.Ports {
		args = append(args, "-p", port)
	}
	for _, env := range task.Environment {
		args = append(args, "-e", env)
	}

	args = append(args, task.Image)
	args = append(args, task.Command...)
	return args
}

// getContainerStatus runs nerdctl inspect to get the container's current status.
// Returns "running", "exited", "paused", etc., or "not_found" if the container doesn't exist.
func getContainerStatus(ctx context.Context, containerName string) string {
	cmd := exec.CommandContext(ctx, "nerdctl", "inspect", "--format", "{{.State.Status}}", containerName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "not_found"
	}
	status := strings.TrimSpace(stdout.String())
	if status == "" {
		return "not_found"
	}
	return status
}

// groupTasksByService groups tasks by their ServiceName and returns them
// with service names sorted alphabetically for stable output.
func groupTasksByService(tasks []TaskRecord) map[string][]TaskRecord {
	grouped := make(map[string][]TaskRecord)
	for _, task := range tasks {
		grouped[task.ServiceName] = append(grouped[task.ServiceName], task)
	}
	return grouped
}

// sortedServiceNames returns sorted service names from a grouped tasks map.
func sortedServiceNames(grouped map[string][]TaskRecord) []string {
	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectDeploymentTasks gathers all tasks for a given deployment across all agents.
func collectDeploymentTasks(ctx context.Context, store interface {
	List(ctx context.Context, prefix string) ([]string, error)
	Get(ctx context.Context, key string, dest interface{}) error
}, deploymentID string) []TaskRecord {
	nodeKeys, err := store.List(ctx, keyNodes)
	if err != nil {
		return nil
	}

	var tasks []TaskRecord
	for _, nodeKey := range nodeKeys {
		var node NodeRecord
		if err := store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskKeys, err := store.List(ctx, keyTasks+node.Name+"/")
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
