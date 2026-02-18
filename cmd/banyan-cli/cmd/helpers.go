package cmd

import (
	"fmt"
	"time"
)

// buildServiceRecords converts manifest services to deployment service records.
// Services with 0 replicas default to 1.
func buildServiceRecords(manifest map[string]ManifestService) map[string]ServiceRecord {
	services := make(map[string]ServiceRecord, len(manifest))
	for name, svc := range manifest {
		replicas := svc.Replicas
		if replicas == 0 {
			replicas = 1
		}
		services[name] = ServiceRecord{
			Image:       svc.Image,
			Replicas:    replicas,
			Ports:       svc.Ports,
			Environment: svc.Env,
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
