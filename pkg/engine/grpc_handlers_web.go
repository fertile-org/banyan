// grpc_handlers_web.go implements per-page gRPC handlers for the web dashboard.
// Each handler returns a focused subset of cluster data, unlike GetDashboardData
// which returns everything in one response.
package engine

import (
	"context"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
)

// GetClusterOverview returns engine status, cluster summary, and recent events.
// Used by the web dashboard overview page for stat cards and event log.
func (s *engineGRPCServer) GetClusterOverview(ctx context.Context, _ *banyanpb.GetClusterOverviewRequest) (*banyanpb.GetClusterOverviewResponse, error) {
	resp := &banyanpb.GetClusterOverviewResponse{}

	// Engine status
	resp.Engine = s.collectEngineStatus()

	// Summary requires scanning agents, deployments, and tasks
	agents := s.collectAgentDetails(ctx)
	deployInfos, latestDeployIDs := s.collectDeployments(ctx, false)
	summary := s.buildClusterSummary(ctx, agents, deployInfos, latestDeployIDs)
	resp.Summary = summary

	// Recent events
	resp.RecentEvents = s.collectRecentEvents(50)

	return resp, nil
}

// ListAgents returns all agents with their system metrics.
func (s *engineGRPCServer) ListAgents(ctx context.Context, _ *banyanpb.ListAgentsRequest) (*banyanpb.ListAgentsResponse, error) {
	return &banyanpb.ListAgentsResponse{
		Agents: s.collectAgentDetails(ctx),
	}, nil
}

// ListDeployments returns all deployments without their full task lists.
func (s *engineGRPCServer) ListDeployments(ctx context.Context, _ *banyanpb.ListDeploymentsRequest) (*banyanpb.ListDeploymentsResponse, error) {
	deployments, _ := s.collectDeployments(ctx, false)
	return &banyanpb.ListDeploymentsResponse{
		Deployments: deployments,
	}, nil
}

// GetDeploymentDetail returns a single deployment with its full task list.
func (s *engineGRPCServer) GetDeploymentDetail(ctx context.Context, req *banyanpb.GetDeploymentDetailRequest) (*banyanpb.GetDeploymentDetailResponse, error) {
	deployments, _ := s.collectDeployments(ctx, true)
	for _, d := range deployments {
		if d.Id == req.Id {
			return &banyanpb.GetDeploymentDetailResponse{Deployment: d}, nil
		}
	}
	return &banyanpb.GetDeploymentDetailResponse{}, nil
}

// ListContainers returns a flat list of containers from the latest deployment per name.
func (s *engineGRPCServer) ListContainers(ctx context.Context, _ *banyanpb.ListContainersRequest) (*banyanpb.ListContainersResponse, error) {
	return &banyanpb.ListContainersResponse{
		Containers: s.collectContainers(ctx),
	}, nil
}

// ListEvents returns recent cluster events.
func (s *engineGRPCServer) ListEvents(_ context.Context, req *banyanpb.ListEventsRequest) (*banyanpb.ListEventsResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 50
	}
	return &banyanpb.ListEventsResponse{
		Events: s.collectRecentEvents(limit),
	}, nil
}

// GetRecentLogs returns the last N lines of a container's logs as a unary response.
// Unlike GetLogs (streaming), this returns immediately with buffered output.
func (s *engineGRPCServer) GetRecentLogs(ctx context.Context, req *banyanpb.GetRecentLogsRequest) (*banyanpb.GetRecentLogsResponse, error) {
	tail := req.Tail
	if tail <= 0 {
		tail = 500
	}

	task, node, err := s.findContainerAgent(ctx, req.ContainerName, req.AgentId)
	if err != nil {
		return nil, err
	}
	if node.APIAddress == "" {
		return &banyanpb.GetRecentLogsResponse{ContainerName: req.ContainerName}, nil
	}

	reader, err := streamAgentLogs(ctx, node.APIAddress, task.ContainerName, false, tail)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil && len(data) == 0 {
		return nil, err
	}

	// Split into lines, remove trailing empty line from final newline
	raw := strings.Split(string(data), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return &banyanpb.GetRecentLogsResponse{
		Lines:         lines,
		ContainerName: req.ContainerName,
	}, nil
}

// --- Shared data collection helpers ---

// collectEngineStatus returns the current engine status with system metrics.
func (s *engineGRPCServer) collectEngineStatus() *banyanpb.EngineStatus {
	es := &banyanpb.EngineStatus{
		Status:        "running",
		StartedAtUnix: s.startedAt.Unix(),
	}
	if s.metricsRegistry != nil {
		if m, ok := s.metricsRegistry.GetEngineMetrics(); ok {
			es.SystemMetrics = &banyanpb.SystemMetrics{
				CpuUsageRatio:    m.CPUUsageRatio,
				MemoryUsedBytes:  m.MemoryUsedBytes,
				MemoryTotalBytes: m.MemoryTotalBytes,
				DiskUsedBytes:    m.DiskUsedBytes,
				DiskTotalBytes:   m.DiskTotalBytes,
				CpuCores:         m.CPUCores,
			}
		}
	}
	return es
}

// collectAgentDetails returns all agents with their system metrics and container counts.
func (s *engineGRPCServer) collectAgentDetails(ctx context.Context) []*banyanpb.AgentDetail {
	nodeKeys, _ := s.store.List(ctx, types.KeyNodes)
	var agents []*banyanpb.AgentDetail

	for _, key := range nodeKeys {
		var node types.NodeRecord
		if err := s.store.Get(ctx, key, &node); err != nil {
			continue
		}

		connected := node.Status == "ready" && time.Since(node.LastSeen) < agentStalenessThreshold
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

		if s.allocator != nil {
			if allocated, _ := s.allocator.Allocate(ctx, node.Name); allocated != nil {
				detail.VpcSubnet = allocated.String()
			}
		}

		// Count containers on this agent from latest deployments
		var containerCount int32
		taskKeys, _ := s.store.List(ctx, types.KeyTasks+node.Name+"/")
		latestIDs := s.latestDeploymentIDs(ctx)
		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := s.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.Type == types.TaskTypeCreateAndStart && task.Status == types.StatusCompleted && latestIDs[task.DeploymentID] {
				containerCount++
			}
		}
		detail.ContainerCount = containerCount

		agents = append(agents, detail)
	}
	return agents
}

// deployInfo bundles a deployment record with its storage key.
type deployInfo struct {
	record types.DeploymentRecord
	key    string
}

// collectDeployments returns all deployments as proto messages.
// If includeTasks is true, each deployment includes its full task list.
// Returns the deployments and a set of latest deployment IDs per name.
func (s *engineGRPCServer) collectDeployments(ctx context.Context, includeTasks bool) ([]*banyanpb.DeploymentInfo, map[string]bool) {
	deployKeys, _ := s.store.List(ctx, types.KeyDeployments)
	var allDeploys []deployInfo
	latestByName := make(map[string]int)

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

	latestDeployIDs := make(map[string]bool, len(latestByName))
	for _, idx := range latestByName {
		latestDeployIDs[allDeploys[idx].record.ID] = true
	}

	var result []*banyanpb.DeploymentInfo
	for i := range allDeploys {
		record := allDeploys[i].record

		allTasks := types.CollectDeploymentTasks(ctx, s.store, record.ID)
		// Filter to latest task per replica — old/failed restart tasks don't count.
		createTasks := latestTasksPerReplica(allTasks)

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
				Replicas:  int32(svc.Replicas), //nolint:gosec
				Ports:     svc.Ports,
				Command:   svc.Command,
				DependsOn: dependsOnToProto(svc.DependsOn),
			}
		}

		di := &banyanpb.DeploymentInfo{
			Id:            record.ID,
			Name:          record.Name,
			Status:        record.Status,
			HealthStatus:  record.HealthStatus,
			Services:      services,
			CreatedAtUnix: record.CreatedAt.Unix(),
			UpdatedAtUnix: record.UpdatedAt.Unix(),
			Error:         record.Error,
			Healthy:       int32(healthy),          //nolint:gosec
			Total:         int32(len(createTasks)), //nolint:gosec
			Tags:          record.Tags,
		}

		if includeTasks {
			for j := range createTasks {
				di.Tasks = append(di.Tasks, taskRecordToProto(&createTasks[j]))
			}
		}

		result = append(result, di)
	}

	return result, latestDeployIDs
}

// collectContainers returns a flat list of containers from the latest deployment per name.
func (s *engineGRPCServer) collectContainers(ctx context.Context) []*banyanpb.ContainerInfo {
	deployKeys, _ := s.store.List(ctx, types.KeyDeployments)

	type deployMeta struct {
		record types.DeploymentRecord
	}
	var allDeploys []deployMeta
	latestByName := make(map[string]int)

	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		idx := len(allDeploys)
		allDeploys = append(allDeploys, deployMeta{record: record})
		if prevIdx, exists := latestByName[record.Name]; exists {
			if record.CreatedAt.After(allDeploys[prevIdx].record.CreatedAt) {
				latestByName[record.Name] = idx
			}
		} else {
			latestByName[record.Name] = idx
		}
	}

	latestIDs := make(map[string]bool, len(latestByName))
	for _, idx := range latestByName {
		latestIDs[allDeploys[idx].record.ID] = true
	}

	// Build deployment name lookup
	deployNameByID := make(map[string]string)
	deployCreatedByID := make(map[string]int64)
	for i := range allDeploys {
		deployNameByID[allDeploys[i].record.ID] = allDeploys[i].record.Name
		deployCreatedByID[allDeploys[i].record.ID] = allDeploys[i].record.CreatedAt.Unix()
	}

	var containers []*banyanpb.ContainerInfo
	for i := range allDeploys {
		if !latestIDs[allDeploys[i].record.ID] {
			continue
		}
		tasks := types.CollectDeploymentTasks(ctx, s.store, allDeploys[i].record.ID)
		for j := range tasks {
			t := &tasks[j]
			if t.Type != types.TaskTypeCreateAndStart || t.Status != types.StatusCompleted {
				continue
			}
			containers = append(containers, &banyanpb.ContainerInfo{
				TaskId:           t.ID,
				ContainerName:    t.ContainerName,
				ServiceName:      t.ServiceName,
				ReplicaIndex:     int32(t.ReplicaIndex), //nolint:gosec
				AgentId:          t.AgentID,
				DeploymentName:   deployNameByID[t.DeploymentID],
				DeploymentId:     t.DeploymentID,
				Image:            t.Image,
				ContainerStatus:  t.ContainerStatus,
				HealthStatus:     t.HealthStatus,
				Ports:            t.Ports,
				CpuPercent:       t.CPUPercent,
				MemoryUsedBytes:  t.MemoryUsedBytes,
				MemoryLimitBytes: t.MemoryLimitBytes,
				CreatedAtUnix:    t.CreatedAt.Unix(),
				UpdatedAtUnix:    t.UpdatedAt.Unix(),
				Error:            t.Error,
			})
		}
	}

	// Sort: deployment creation time (oldest first) → service → replica
	sort.Slice(containers, func(i, j int) bool {
		ci, cj := containers[i], containers[j]
		if ci.DeploymentId != cj.DeploymentId {
			ti, tj := deployCreatedByID[ci.DeploymentId], deployCreatedByID[cj.DeploymentId]
			if ti != tj {
				return ti < tj
			}
			return ci.DeploymentId < cj.DeploymentId
		}
		if ci.ServiceName != cj.ServiceName {
			return ci.ServiceName < cj.ServiceName
		}
		return ci.ReplicaIndex < cj.ReplicaIndex
	})

	return containers
}

// collectRecentEvents returns the most recent cluster events.
func (s *engineGRPCServer) collectRecentEvents(limit int) []*banyanpb.ClusterEvent {
	if s.events == nil {
		return nil
	}
	recent := s.events.Recent(limit)
	events := make([]*banyanpb.ClusterEvent, 0, len(recent))
	for _, ev := range recent {
		events = append(events, &banyanpb.ClusterEvent{
			TimestampUnix: ev.Timestamp.Unix(),
			Type:          ev.Type,
			Message:       ev.Message,
			Severity:      ev.Severity,
		})
	}
	return events
}

// buildClusterSummary computes aggregated cluster statistics.
func (s *engineGRPCServer) buildClusterSummary(ctx context.Context, agents []*banyanpb.AgentDetail, deployments []*banyanpb.DeploymentInfo, latestDeployIDs map[string]bool) *banyanpb.ClusterSummary {
	var connectedAgents int32
	for _, a := range agents {
		if a.Status == "ready" {
			connectedAgents++
		}
	}

	var totalDeployments, runningDeployments int32
	for _, d := range deployments {
		totalDeployments++
		if d.Status == types.StatusRunning {
			runningDeployments++
		}
	}

	var totalContainers, healthyContainers int32
	tasksByStatus := make(map[string]int32)

	for _, a := range agents {
		taskKeys, _ := s.store.List(ctx, types.KeyTasks+a.Name+"/")
		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := s.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			if task.Type != types.TaskTypeCreateAndStart {
				continue
			}
			tasksByStatus[task.Status]++
			if task.Status == types.StatusCompleted && latestDeployIDs[task.DeploymentID] {
				totalContainers++
				if task.ContainerStatus == types.StatusRunning {
					healthyContainers++
				}
			}
		}
	}

	return &banyanpb.ClusterSummary{
		TotalAgents:        int32(len(agents)), //nolint:gosec
		ConnectedAgents:    connectedAgents,
		TotalDeployments:   totalDeployments,
		RunningDeployments: runningDeployments,
		TotalContainers:    totalContainers,
		HealthyContainers:  healthyContainers,
		TasksByStatus:      tasksByStatus,
	}
}

// latestTasksPerReplica filters tasks to only the most recent create_and_start
// task per service+replica. Old tasks (superseded restarts, failed attempts)
// are excluded. This keeps container counts stable across restart cycles.
func latestTasksPerReplica(allTasks []types.TaskRecord) []types.TaskRecord {
	type key struct {
		svc     string
		replica int
	}
	latest := make(map[key]*types.TaskRecord)

	for i := range allTasks {
		t := &allTasks[i]
		if t.Type != types.TaskTypeCreateAndStart {
			continue
		}
		k := key{svc: t.ServiceName, replica: t.ReplicaIndex}
		if existing, ok := latest[k]; !ok || t.CreatedAt.After(existing.CreatedAt) {
			latest[k] = t
		}
	}

	result := make([]types.TaskRecord, 0, len(latest))
	for _, t := range latest {
		result = append(result, *t)
	}
	return result
}

// taskRecordToProto converts a types.TaskRecord to a banyanpb.TaskInfo.
func taskRecordToProto(t *types.TaskRecord) *banyanpb.TaskInfo {
	return &banyanpb.TaskInfo{
		Id:                     t.ID,
		DeploymentId:           t.DeploymentID,
		ServiceName:            t.ServiceName,
		ReplicaIndex:           int32(t.ReplicaIndex), //nolint:gosec
		AgentId:                t.AgentID,
		Type:                   t.Type,
		Status:                 t.Status,
		Image:                  t.Image,
		ContainerName:          t.ContainerName,
		Ports:                  t.Ports,
		Command:                t.Command,
		ContainerStatus:        t.ContainerStatus,
		HealthStatus:           t.HealthStatus,
		ContainerCheckedAtUnix: t.ContainerCheckedAt.Unix(),
		CreatedAtUnix:          t.CreatedAt.Unix(),
		UpdatedAtUnix:          t.UpdatedAt.Unix(),
		Error:                  t.Error,
		CpuPercent:             t.CPUPercent,
		MemoryUsedBytes:        t.MemoryUsedBytes,
		MemoryLimitBytes:       t.MemoryLimitBytes,
		ExitCode:               int32(t.ExitCode),     //nolint:gosec // exit code fits int32
		RestartCount:           int32(t.RestartCount), //nolint:gosec // restart count is always small
	}
}
