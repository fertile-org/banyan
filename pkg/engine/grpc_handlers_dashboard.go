// grpc_handlers_dashboard.go contains the GetDashboardData gRPC handler,
// which returns a comprehensive cluster snapshot for the CLI dashboard.
package engine

import (
	"context"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// GetDashboardData returns a comprehensive snapshot of the cluster for the CLI dashboard.
func (s *engineGRPCServer) GetDashboardData(ctx context.Context, req *banyanpb.GetDashboardDataRequest) (*banyanpb.GetDashboardDataResponse, error) {
	resp := &banyanpb.GetDashboardDataResponse{}

	// Engine status
	resp.Engine = &banyanpb.EngineStatus{
		Status:        "running",
		StartedAtUnix: s.startedAt.Unix(),
	}
	if s.metricsRegistry != nil {
		if m, ok := s.metricsRegistry.GetEngineMetrics(); ok {
			resp.Engine.SystemMetrics = &banyanpb.SystemMetrics{
				CpuUsageRatio:    m.CPUUsageRatio,
				MemoryUsedBytes:  m.MemoryUsedBytes,
				MemoryTotalBytes: m.MemoryTotalBytes,
				DiskUsedBytes:    m.DiskUsedBytes,
				DiskTotalBytes:   m.DiskTotalBytes,
				CpuCores:         m.CPUCores,
			}
		}
	}

	// --- Load all deployments and identify the latest per name ---
	deployKeys, _ := s.store.List(ctx, types.KeyDeployments)
	type deployInfo struct {
		record types.DeploymentRecord
		key    string
	}
	var allDeploys []deployInfo
	latestByName := make(map[string]int) // name → index of latest in allDeploys

	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		idx := len(allDeploys)
		allDeploys = append(allDeploys, deployInfo{record: record, key: key})

		if prevIdx, exists := latestByName[record.Name]; exists {
			if record.CreatedAt.After(allDeploys[prevIdx].record.CreatedAt) {
				latestByName[record.Name] = idx
			}
		} else {
			latestByName[record.Name] = idx
		}
	}

	// Build set of latest deployment IDs — only these count for cluster summary
	latestDeployIDs := make(map[string]bool, len(latestByName))
	for _, idx := range latestByName {
		latestDeployIDs[allDeploys[idx].record.ID] = true
	}

	// NOTE: Superseded deployment reconciliation is now handled by DeploymentReconciler
	// (see reconcile_deployment.go). Dashboard is read-only — no state mutations.

	// --- Collect agents with system metrics ---
	nodeKeys, _ := s.store.List(ctx, types.KeyNodes)
	var totalAgents, connectedAgents, totalContainers, healthyContainers int32
	tasksByStatus := make(map[string]int32)

	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := s.store.Get(ctx, key, &node); err != nil {
			continue
		}
		totalAgents++
		connected := node.Status == "ready" && time.Since(node.LastSeen) < agentStalenessThreshold
		if connected {
			connectedAgents++
		}

		agentStatus := node.Status
		if !connected && node.Status == "ready" {
			agentStatus = "stale"
		}

		detail := &banyanpb.AgentDetail{
			Name:          node.Name,
			Status:        agentStatus,
			ApiAddress:    node.APIAddress,
			LastSeenUnix:  node.LastSeen.Unix(),
			CreatedAtUnix: node.CreatedAt.Unix(),
			Tags:          node.Tags,
		}

		// Attach system metrics from registry
		if s.metricsRegistry != nil {
			if m, ok := s.metricsRegistry.GetAgentMetrics(node.Name); ok {
				detail.SystemMetrics = &banyanpb.SystemMetrics{
					CpuUsageRatio:    m.CPUUsageRatio,
					MemoryUsedBytes:  m.MemoryUsedBytes,
					MemoryTotalBytes: m.MemoryTotalBytes,
					DiskUsedBytes:    m.DiskUsedBytes,
					DiskTotalBytes:   m.DiskTotalBytes,
					CpuCores:         m.CPUCores,
				}
			}
		}

		// Subnet info
		if s.allocator != nil {
			if allocated, _ := s.allocator.Allocate(ctx, node.Name); allocated != nil {
				detail.VpcSubnet = allocated.String()
			}
		}

		// Agent container count + cluster summary: only count tasks from latest deployments
		var agentContainerCount int32
		taskKeys, _ := s.store.List(ctx, types.KeyTasks+node.Name+"/")
		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := s.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.Type != types.TaskTypeCreateAndStart {
				continue
			}

			// Task-level status counts (all deployments)
			tasksByStatus[task.Status]++

			// Container counts: only from latest deployment per name
			if task.Status == types.StatusCompleted && latestDeployIDs[task.DeploymentID] {
				agentContainerCount++
				totalContainers++
				if task.ContainerStatus == types.StatusRunning {
					healthyContainers++
				}
			}
		}
		detail.ContainerCount = agentContainerCount

		resp.Agents = append(resp.Agents, detail)
	}

	// --- Build deployment response ---
	var totalDeployments, runningDeployments int32
	for i := range allDeploys {
		record := allDeploys[i].record
		totalDeployments++
		if record.Status == types.StatusRunning {
			runningDeployments++
		}

		allTasks := types.CollectDeploymentTasks(ctx, s.store, record.ID)
		var createTasks []types.TaskRecord
		for j := range allTasks {
			if allTasks[j].Type == types.TaskTypeCreateAndStart {
				createTasks = append(createTasks, allTasks[j])
			}
		}

		healthy := 0
		for j := range createTasks {
			if createTasks[j].ContainerStatus == types.StatusRunning {
				healthy++
			}
		}

		services := make(map[string]*banyanpb.ServiceInfo, len(record.Services))
		for name, svc := range record.Services {
			services[name] = &banyanpb.ServiceInfo{
				Image:     svc.Image,
				Replicas:  int32(svc.Replicas), //nolint:gosec // replica count is always small
				Ports:     svc.Ports,
				Command:   svc.Command,
				DependsOn: dependsOnToProto(svc.DependsOn),
				// Environment intentionally omitted — may contain secrets
			}
		}

		var taskInfos []*banyanpb.TaskInfo
		for j := range allTasks {
			taskInfos = append(taskInfos, &banyanpb.TaskInfo{
				Id:                     allTasks[j].ID,
				DeploymentId:           allTasks[j].DeploymentID,
				ServiceName:            allTasks[j].ServiceName,
				ReplicaIndex:           int32(allTasks[j].ReplicaIndex), //nolint:gosec // replica index is always small
				AgentId:                allTasks[j].AgentID,
				Type:                   allTasks[j].Type,
				Status:                 allTasks[j].Status,
				Image:                  allTasks[j].Image,
				ContainerName:          allTasks[j].ContainerName,
				Ports:                  allTasks[j].Ports,
				Command:                allTasks[j].Command,
				ContainerStatus:        allTasks[j].ContainerStatus,
				HealthStatus:           allTasks[j].HealthStatus,
				ContainerCheckedAtUnix: allTasks[j].ContainerCheckedAt.Unix(),
				CreatedAtUnix:          allTasks[j].CreatedAt.Unix(),
				UpdatedAtUnix:          allTasks[j].UpdatedAt.Unix(),
				Error:                  allTasks[j].Error,
				CpuPercent:             allTasks[j].CPUPercent,
				MemoryUsedBytes:        allTasks[j].MemoryUsedBytes,
				MemoryLimitBytes:       allTasks[j].MemoryLimitBytes,
				ExitCode:               int32(allTasks[j].ExitCode),     //nolint:gosec // exit code fits int32
				RestartCount:           int32(allTasks[j].RestartCount), //nolint:gosec // restart count is always small
				// Environment intentionally omitted — may contain secrets
			})
		}

		resp.Deployments = append(resp.Deployments, &banyanpb.DeploymentInfo{
			Id:            record.ID,
			Name:          record.Name,
			Status:        record.Status,
			HealthStatus:  record.HealthStatus,
			Services:      services,
			CreatedAtUnix: record.CreatedAt.Unix(),
			UpdatedAtUnix: record.UpdatedAt.Unix(),
			Error:         record.Error,
			Tasks:         taskInfos,
			Healthy:       int32(healthy),          //nolint:gosec // count is always small
			Total:         int32(len(createTasks)), //nolint:gosec // count is always small
			Tags:          record.Tags,
		})
	}

	// Cluster summary
	resp.Summary = &banyanpb.ClusterSummary{
		TotalAgents:        totalAgents,
		ConnectedAgents:    connectedAgents,
		TotalDeployments:   totalDeployments,
		RunningDeployments: runningDeployments,
		TotalContainers:    totalContainers,
		HealthyContainers:  healthyContainers,
		TasksByStatus:      tasksByStatus,
	}

	// Recent events
	if s.events != nil {
		recent := s.events.Recent(50)
		for _, ev := range recent {
			resp.RecentEvents = append(resp.RecentEvents, &banyanpb.ClusterEvent{
				TimestampUnix: ev.Timestamp.Unix(),
				Type:          ev.Type,
				Message:       ev.Message,
				Severity:      ev.Severity,
			})
		}
	}

	return resp, nil
}
