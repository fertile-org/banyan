// grpc_handlers_agent.go contains gRPC handlers for agent-to-engine RPCs:
// Register, Heartbeat, PollTasks, ReportTaskResult, ReportContainerHealth,
// and related helper methods (reconcileDeploymentStatus, collectServiceBackends, etc.).
package engine

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

func (s *engineGRPCServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}
	if err := types.ValidateName(req.AgentName); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid agent name: %v", err)
	}

	// Enforce agent identity: claimed name must match WireGuard tunnel IP identity.
	if len(s.tunnelIPToAgent) > 0 {
		resolvedName := s.agentNameFromContext(ctx)
		if resolvedName != "" && resolvedName != req.AgentName {
			s.logger().Warn("Agent name mismatch",
				"claimed", req.AgentName, "resolved", resolvedName)
			return nil, status.Errorf(codes.PermissionDenied,
				"agent name %q does not match tunnel identity %q", req.AgentName, resolvedName)
		}
	}

	// Create node record in store
	node := &types.NodeRecord{
		Name:       req.AgentName,
		Status:     "ready",
		APIAddress: req.ApiAddress,
		Tags:       req.Tags,
		LastSeen:   time.Now(),
		CreatedAt:  time.Now(),
	}
	if err := s.store.Save(ctx, types.KeyNodes+req.AgentName, node); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to register node: %v", err)
	}

	s.logger().Info("Agent registered", "agent", req.AgentName, "api", req.ApiAddress)
	s.emitEvent("agent.registered", fmt.Sprintf("Agent %s registered (api: %s)", req.AgentName, req.ApiAddress), "info")

	resp := &banyanpb.RegisterResponse{
		RegistryUrl: s.registryURL,
	}

	// Allocate VPC subnet for this agent
	if s.allocator != nil && s.vpcCIDR != "" {
		subnet, allocErr := s.allocator.Allocate(ctx, req.AgentName)
		if allocErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to allocate subnet: %v", allocErr)
		}
		resp.VpcCidr = s.vpcCIDR
		resp.AllocatedSubnet = subnet.String()

		// Record peer info: prefer agent-reported host IP (avoids control tunnel IP),
		// fall back to gRPC connection IP.
		if s.peerTracker != nil {
			hostIP := agentHostIP(req.HostIp, ctx)
			if hostIP != nil {
				peer := overlay.Peer{
					Subnet:    *subnet,
					HostIP:    hostIP,
					VTEPIP:    overlay.VTEPIP(*subnet),
					PublicKey: req.WgPublicKey,
				}
				s.peerTracker.Update(ctx, req.AgentName, peer)
			}
		}
	}

	// Return active containers so agent can restore proxy rules after restart.
	// Only include containers from deployments that are still running.
	runningDeployments := make(map[string]bool)
	depKeys, _ := s.store.List(ctx, types.KeyDeployments)
	for _, key := range depKeys {
		var dep types.DeploymentRecord
		if err := s.store.Get(ctx, key, &dep); err != nil {
			continue
		}
		if dep.Status == types.StatusRunning {
			runningDeployments[dep.ID] = true
		}
	}

	taskPrefix := types.KeyTasks + req.AgentName + "/"
	taskKeys, _ := s.store.List(ctx, taskPrefix)
	for _, key := range taskKeys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Type != types.TaskTypeCreateAndStart || task.Status != types.StatusCompleted {
			continue
		}
		if task.ContainerStatus != types.StatusRunning {
			continue
		}
		if !runningDeployments[task.DeploymentID] {
			continue
		}
		resp.ActiveContainers = append(resp.ActiveContainers, &banyanpb.ActiveContainer{
			ContainerName: task.ContainerName,
			ContainerIp:   task.ContainerIP,
			Ports:         task.Ports,
			ServiceName:   task.ServiceName,
			DeploymentId:  task.DeploymentID,
			TaskId:        task.ID,
		})
	}

	// New agent available — trigger scheduling for any pending deployments
	s.triggerSchedule()

	return resp, nil
}

func (s *engineGRPCServer) Heartbeat(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}

	// Update node record in store — reject heartbeats from unregistered agents
	var node types.NodeRecord
	if err := s.store.Get(ctx, types.KeyNodes+req.AgentName, &node); err != nil {
		return nil, status.Errorf(codes.NotFound, "agent %q is not registered — call Register first", req.AgentName)
	}
	node.LastSeen = time.Now()
	node.Status = "ready"
	node.Tags = req.Tags
	if req.SystemMetrics != nil {
		// Bounds-check agent-reported metrics to prevent integer overflow in the
		// scheduler (pickAgentByResources casts uint64→int64) and filter bogus values.
		const maxMemory = 64 * 1024 * 1024 * 1024 * 1024 // 64 TB
		const maxCPUCores = 1024
		if req.SystemMetrics.MemoryTotalBytes > 0 && req.SystemMetrics.MemoryTotalBytes <= maxMemory {
			node.MemoryTotalBytes = req.SystemMetrics.MemoryTotalBytes
			node.MemoryUsedBytes = min(req.SystemMetrics.MemoryUsedBytes, req.SystemMetrics.MemoryTotalBytes)
		}
		if req.SystemMetrics.CpuCores > 0 && req.SystemMetrics.CpuCores <= maxCPUCores {
			node.CPUCores = req.SystemMetrics.CpuCores
		}
		if req.SystemMetrics.CpuUsageRatio >= 0 && req.SystemMetrics.CpuUsageRatio <= 1.0 {
			node.CPUUsageRatio = req.SystemMetrics.CpuUsageRatio
		}
	}
	if err := s.store.Save(ctx, types.KeyNodes+req.AgentName, &node); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update heartbeat: %v", err)
	}

	// Store agent system metrics in Prometheus registry
	if s.metricsRegistry != nil && req.SystemMetrics != nil {
		agentMetrics := metrics.SystemMetrics{
			CPUUsageRatio:    req.SystemMetrics.CpuUsageRatio,
			MemoryUsedBytes:  req.SystemMetrics.MemoryUsedBytes,
			MemoryTotalBytes: req.SystemMetrics.MemoryTotalBytes,
			DiskUsedBytes:    req.SystemMetrics.DiskUsedBytes,
			DiskTotalBytes:   req.SystemMetrics.DiskTotalBytes,
			CPUCores:         req.SystemMetrics.CpuCores,
		}
		containerCount := s.countAgentContainers(ctx, req.AgentName)
		s.metricsRegistry.UpdateAgent(req.AgentName, agentMetrics, containerCount)

		// Update agent info label (allocator.Allocate is idempotent)
		subnet := ""
		if s.allocator != nil {
			if allocated, _ := s.allocator.Allocate(ctx, req.AgentName); allocated != nil {
				subnet = allocated.String()
			}
		}
		s.metricsRegistry.UpdateAgentInfo(req.AgentName, "ready", subnet)
	}

	resp := &banyanpb.HeartbeatResponse{}

	// Return service backends for cross-host load balancing (VPC-gated)
	if s.peerTracker != nil {
		resp.ServiceBackends = s.collectServiceBackends(ctx)
	}

	// Return VPC peer list if overlay networking is enabled
	if s.peerTracker != nil {
		// Note: we do NOT update the host IP on heartbeat because the gRPC connection
		// may come through the WireGuard control tunnel (10.200.x.x), which is not
		// the data-plane address. The correct host IP was set during Register().

		peers := s.peerTracker.GetPeersExcluding(ctx, req.AgentName)
		for _, p := range peers {
			resp.VpcPeers = append(resp.VpcPeers, &banyanpb.VPCPeer{
				Subnet:    p.Subnet.String(),
				HostIp:    p.HostIP.String(),
				PublicKey: p.PublicKey,
			})
		}
	}

	return resp, nil
}

func (s *engineGRPCServer) PollTasks(ctx context.Context, req *banyanpb.PollTasksRequest) (*banyanpb.PollTasksResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}

	taskPrefix := types.KeyTasks + req.AgentName + "/"
	keys, err := s.store.List(ctx, taskPrefix)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	var tasks []*banyanpb.TaskRecord
	for _, key := range keys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Status != types.StatusPending {
			continue
		}

		pbTask := &banyanpb.TaskRecord{
			Id:                task.ID,
			DeploymentId:      task.DeploymentID,
			DeploymentName:    task.DeploymentName,
			ServiceName:       task.ServiceName,
			ReplicaIndex:      int32(task.ReplicaIndex), //nolint:gosec // replica index is always small
			AgentId:           task.AgentID,
			Type:              task.Type,
			Status:            task.Status,
			Image:             task.Image,
			ContainerName:     task.ContainerName,
			Ports:             task.Ports,
			Environment:       task.Environment,
			Command:           task.Command,
			Restart:           task.Restart,
			Entrypoint:        task.Entrypoint,
			MemoryLimit:       task.MemoryLimit,
			CpuLimit:          task.CPULimit,
			MemoryReservation: task.MemoryReservation,
		}
		if task.Healthcheck != nil {
			pbTask.Healthcheck = &banyanpb.ManifestHealthcheck{
				Test:        task.Healthcheck.Test,
				Interval:    task.Healthcheck.Interval,
				Timeout:     task.Healthcheck.Timeout,
				Retries:     int32(task.Healthcheck.Retries), //nolint:gosec // retries is always small
				StartPeriod: task.Healthcheck.StartPeriod,
				Disable:     task.Healthcheck.Disable,
			}
		}
		for _, vol := range task.Volumes {
			pbVol := &banyanpb.VolumeMount{
				Type: vol.Type, Source: vol.Source, Target: vol.Target, ReadOnly: vol.ReadOnly,
			}
			if vol.Tmpfs != nil {
				pbVol.Tmpfs = &banyanpb.TmpfsOpt{Size: vol.Tmpfs.Size}
			}
			pbTask.Volumes = append(pbTask.Volumes, pbVol)
		}
		// Resolve secret references just-in-time (values never stored in etcd)
		if len(task.SecretRefs) > 0 && s.secrets != nil {
			pbTask.SecretRefs = task.SecretRefs
			resolved, resolveErr := s.secrets.ResolveSecrets(ctx, task.SecretRefs)
			if resolveErr != nil {
				s.logger().Warn("Failed to resolve secrets for task", "task", task.ID, "error", resolveErr)
			} else {
				pbTask.ResolvedSecrets = resolved
			}
		}
		tasks = append(tasks, pbTask)
	}

	return &banyanpb.PollTasksResponse{Tasks: tasks}, nil
}

func (s *engineGRPCServer) ReportTaskResult(ctx context.Context, req *banyanpb.ReportTaskResultRequest) (*banyanpb.ReportTaskResultResponse, error) {
	if req.TaskId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id and agent_id are required")
	}

	taskKey := types.KeyTasks + req.AgentId + "/" + req.TaskId
	var task types.TaskRecord
	if err := s.store.Get(ctx, taskKey, &task); err != nil {
		return nil, status.Errorf(codes.NotFound, "task not found: %v", err)
	}

	task.Status = req.Status
	task.Error = req.Error
	task.ContainerName = req.ContainerName
	task.UpdatedAt = time.Now()

	if req.Result != nil {
		task.Result = &types.TaskResultRecord{
			ContainerID: req.Result.ContainerId,
		}
	}

	if err := s.store.Save(ctx, taskKey, &task); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update task: %v", err)
	}

	return &banyanpb.ReportTaskResultResponse{}, nil
}

func (s *engineGRPCServer) ReportContainerHealth(ctx context.Context, req *banyanpb.ReportContainerHealthRequest) (*banyanpb.ReportContainerHealthResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}

	// Build maps of container statuses, IPs, and health statuses for quick lookup
	statusMap := make(map[string]string, len(req.Containers))
	ipMap := make(map[string]string, len(req.Containers))
	healthMap := make(map[string]string, len(req.Containers))
	type containerMetrics struct {
		cpuPercent float64
		memUsed    uint64
		memLimit   uint64
		hasMetrics bool
	}
	metricsMap := make(map[string]containerMetrics)

	for _, c := range req.Containers {
		statusMap[c.ContainerName] = c.Status
		if c.Ip != "" {
			ipMap[c.ContainerName] = c.Ip
		}
		if c.HealthStatus != "" {
			healthMap[c.ContainerName] = c.HealthStatus
		}
		if c.CpuPercent > 0 || c.MemoryUsedBytes > 0 {
			metricsMap[c.ContainerName] = containerMetrics{
				cpuPercent: c.CpuPercent,
				memUsed:    c.MemoryUsedBytes,
				memLimit:   c.MemoryLimitBytes,
				hasMetrics: true,
			}
		}
	}

	// Update matching tasks in store
	taskPrefix := types.KeyTasks + req.AgentName + "/"
	keys, err := s.store.List(ctx, taskPrefix)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

	// Track which deployments have changed container status
	affectedDeployments := make(map[string]bool)

	for _, key := range keys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Type != types.TaskTypeCreateAndStart || task.Status != types.StatusCompleted {
			continue
		}
		if containerStatus, ok := statusMap[task.ContainerName]; ok {
			task.ContainerStatus = containerStatus
			task.ContainerCheckedAt = time.Now()
			if ip, hasIP := ipMap[task.ContainerName]; hasIP {
				task.ContainerIP = ip
			}
			if hs, hasHS := healthMap[task.ContainerName]; hasHS {
				task.HealthStatus = hs
			}
			if m, hasM := metricsMap[task.ContainerName]; hasM {
				task.CPUPercent = m.cpuPercent
				task.MemoryUsedBytes = m.memUsed
				task.MemoryLimitBytes = m.memLimit
			}
			if err := s.store.Save(ctx, key, &task); err != nil {
				s.logger().Warn("Failed to save container health", "container", task.ContainerName, "error", err)
			}
			if task.DeploymentID != "" {
				affectedDeployments[task.DeploymentID] = true
			}
		}
	}

	// Reconcile deployment status: if all containers are dead, mark deployment as stopped
	for deployID := range affectedDeployments {
		s.reconcileDeploymentStatus(ctx, deployID)
	}

	return &banyanpb.ReportContainerHealthResponse{}, nil
}

// reconcileDeploymentStatus checks if all containers for a deployment are dead
// and updates the deployment status to "stopped" if so. This handles the case
// where containers are killed outside Banyan (e.g., nerdctl rm).
func (s *engineGRPCServer) reconcileDeploymentStatus(ctx context.Context, deploymentID string) {
	key := types.KeyDeployments + deploymentID
	var record types.DeploymentRecord
	if err := s.store.Get(ctx, key, &record); err != nil {
		return
	}

	// Only reconcile deployments currently marked as running
	if record.Status != types.StatusRunning {
		return
	}

	tasks := types.CollectDeploymentTasks(ctx, s.store, deploymentID)
	hasLiveContainer := false
	for i := range tasks {
		if tasks[i].Type != types.TaskTypeCreateAndStart {
			continue
		}
		cs := tasks[i].ContainerStatus
		if cs == types.StatusRunning || cs == "created" || cs == "paused" {
			hasLiveContainer = true
			break
		}
	}

	if !hasLiveContainer {
		record.Status = types.StatusStopped
		record.UpdatedAt = time.Now()
		if err := s.store.Save(ctx, key, &record); err != nil {
			s.logger().Warn("Failed to update deployment status to stopped", "deployment_id", deploymentID, "error", err)
		} else {
			s.emitEvent("deployment.stopped", fmt.Sprintf("Deployment %s stopped (all containers dead)", record.Name), "warn")
		}
	}
}

// collectServiceBackends gathers all running container backends across all agents.
// Used for cross-host load balancing via heartbeat responses.
func (s *engineGRPCServer) collectServiceBackends(ctx context.Context) []*banyanpb.ServiceBackend {
	// Build map of running deployment IDs → names (only include backends from active deployments)
	runningDeployments := map[string]string{} // deploymentID → deploymentName
	deployKeys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return nil
	}
	for _, key := range deployKeys {
		var d types.DeploymentRecord
		if getErr := s.store.Get(ctx, key, &d); getErr != nil {
			continue
		}
		if d.Status == types.StatusRunning {
			runningDeployments[d.ID] = d.Name
		}
	}

	nodeKeys, err := s.store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil
	}

	var backends []*banyanpb.ServiceBackend
	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := s.store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}

		taskKeys, err := s.store.List(ctx, types.KeyTasks+node.Name+"/")
		if err != nil {
			continue
		}

		for _, taskKey := range taskKeys {
			var task types.TaskRecord
			if err := s.store.Get(ctx, taskKey, &task); err != nil {
				continue
			}
			// Only include running containers from active deployments
			depName, isRunning := runningDeployments[task.DeploymentID]
			if task.Type != types.TaskTypeCreateAndStart ||
				task.Status != types.StatusCompleted ||
				task.ContainerStatus != types.StatusRunning ||
				task.ContainerIP == "" ||
				!isRunning {
				continue
			}
			backends = append(backends, &banyanpb.ServiceBackend{
				ContainerName:  task.ContainerName,
				ContainerIp:    task.ContainerIP,
				Ports:          task.Ports,
				AgentName:      task.AgentID,
				ServiceName:    task.ServiceName,
				DeploymentName: depName,
			})
		}
	}

	return backends
}

// countAgentContainers counts the number of running containers on a given agent.
func (s *engineGRPCServer) countAgentContainers(ctx context.Context, agentName string) int32 {
	taskKeys, err := s.store.List(ctx, types.KeyTasks+agentName+"/")
	if err != nil {
		return 0
	}
	var count int32
	for _, key := range taskKeys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Type == types.TaskTypeCreateAndStart && task.Status == types.StatusCompleted {
			count++
		}
	}
	return count
}

// agentHostIP returns the agent's data-plane host IP. It prefers the
// explicitly reported IP (which skips control tunnel interfaces on the agent)
// over the gRPC connection IP (which may be a control tunnel address).
func agentHostIP(reported string, ctx context.Context) net.IP {
	if reported != "" {
		if ip := net.ParseIP(reported); ip != nil {
			return ip
		}
	}
	return extractPeerIP(ctx)
}

// extractPeerIP extracts the remote IP address from the gRPC connection context.
func extractPeerIP(ctx context.Context) net.IP {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil
	}
	addr, ok := p.Addr.(*net.TCPAddr)
	if !ok {
		return nil
	}
	return addr.IP
}
