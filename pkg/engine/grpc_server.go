package engine

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/metrics"
	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
)

// engineGRPCServer implements the EngineService gRPC server.
type engineGRPCServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store           storage.StateStore
	sessions        sync.Map // map[agentName]sessionToken
	registryURL     string
	whitelistedKeys map[string]string        // publicKey → agentName
	allocator       *overlay.SubnetAllocator // VPC subnet allocator (nil if VPC disabled)
	peerTracker     *overlay.PeerTracker     // VPC peer tracker (nil if VPC disabled)
	vpcCIDR         string                   // VPC network CIDR (e.g., "10.0.0.0/16")
	metricsRegistry *metrics.EngineMetricsRegistry
	events          EventLog
	startedAt       time.Time
	log             *logging.Logger
}

// grpcServerOptions configures the engine gRPC server.
type grpcServerOptions struct {
	Store           storage.StateStore
	Port            string
	RegistryURL     string
	Allocator       *overlay.SubnetAllocator
	PeerTracker     *overlay.PeerTracker
	VPCCIDR         string
	WhitelistedKeys map[string]string // publicKey → agentName
	MetricsRegistry *metrics.EngineMetricsRegistry
	Events          EventLog
	StartedAt       time.Time
}

// logger returns the gRPC server's logger, initializing it if nil (for test convenience).
func (s *engineGRPCServer) logger() *logging.Logger {
	if s.log == nil {
		s.log = logging.New("engine")
	}
	return s.log
}

// startEngineGRPC starts the gRPC server for agent communication.
func startEngineGRPC(ctx context.Context, opts *grpcServerOptions) (*engineGRPCServer, error) {
	lis, err := net.Listen("tcp", ":"+opts.Port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC port %s: %w", opts.Port, err)
	}

	engineSrv := &engineGRPCServer{
		store:           opts.Store,
		registryURL:     opts.RegistryURL,
		whitelistedKeys: opts.WhitelistedKeys,
		allocator:       opts.Allocator,
		peerTracker:     opts.PeerTracker,
		vpcCIDR:         opts.VPCCIDR,
		metricsRegistry: opts.MetricsRegistry,
		events:          opts.Events,
		startedAt:       opts.StartedAt,
		log:             logging.New("engine"),
	}

	var srv *grpc.Server
	if len(opts.WhitelistedKeys) > 0 {
		validator := &banyanrpc.PublicKeyValidator{AllowedKeys: opts.WhitelistedKeys}
		srv = grpc.NewServer(
			grpc.UnaryInterceptor(banyanrpc.NewPublicKeyAuthInterceptor(validator)),
			grpc.StreamInterceptor(banyanrpc.NewPublicKeyAuthStreamInterceptor(validator)),
		)
	} else {
		// No auth — warn at startup but allow (development/testing)
		srv = grpc.NewServer()
	}

	banyanpb.RegisterEngineServiceServer(srv, engineSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			engineSrv.logger().Error("gRPC server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		srv.GracefulStop()
	}()

	return engineSrv, nil
}

// GetSessionToken returns the session token for a given agent name.
func (s *engineGRPCServer) GetSessionToken(agentName string) string {
	if v, ok := s.sessions.Load(agentName); ok {
		return v.(string)
	}
	return ""
}

func (s *engineGRPCServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}
	if req.SessionToken == "" {
		return nil, status.Error(codes.InvalidArgument, "session_token is required")
	}

	// Store session token
	s.sessions.Store(req.AgentName, req.SessionToken)

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
		subnet, allocErr := s.allocator.Allocate(req.AgentName)
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
				s.peerTracker.Update(req.AgentName, peer)
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

	return resp, nil
}

func (s *engineGRPCServer) Heartbeat(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
	if req.AgentName == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_name is required")
	}

	// Update session token (engine restart resilience)
	if req.SessionToken != "" {
		s.sessions.Store(req.AgentName, req.SessionToken)
	}

	// Update node record in store
	var node types.NodeRecord
	if err := s.store.Get(ctx, types.KeyNodes+req.AgentName, &node); err != nil {
		// Node not found, create it
		node = types.NodeRecord{
			Name:      req.AgentName,
			CreatedAt: time.Now(),
		}
	}
	node.LastSeen = time.Now()
	node.Status = "ready"
	node.Tags = req.Tags
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
			if allocated, _ := s.allocator.Allocate(req.AgentName); allocated != nil {
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

		peers := s.peerTracker.GetPeersExcluding(req.AgentName)
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

		tasks = append(tasks, &banyanpb.TaskRecord{
			Id:            task.ID,
			DeploymentId:  task.DeploymentID,
			ServiceName:   task.ServiceName,
			ReplicaIndex:  int32(task.ReplicaIndex), //nolint:gosec // replica index is always small
			AgentId:       task.AgentID,
			Type:          task.Type,
			Status:        task.Status,
			Image:         task.Image,
			ContainerName: task.ContainerName,
			Ports:         task.Ports,
			Environment:   task.Environment,
			Command:       task.Command,
		})
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

	// Build maps of container statuses and IPs for quick lookup
	statusMap := make(map[string]string, len(req.Containers))
	ipMap := make(map[string]string, len(req.Containers))
	for _, c := range req.Containers {
		statusMap[c.ContainerName] = c.Status
		if c.Ip != "" {
			ipMap[c.ContainerName] = c.Ip
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

// --- CLI RPC handlers ---

func (s *engineGRPCServer) Deploy(ctx context.Context, req *banyanpb.DeployRPCRequest) (*banyanpb.DeployRPCResponse, error) {
	if req.Manifest == nil || req.Manifest.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "manifest must have a name")
	}
	if len(req.Manifest.Services) == 0 {
		return nil, status.Error(codes.InvalidArgument, "manifest must define at least one service")
	}

	for name, svc := range req.Manifest.Services {
		if svc.Image == "" && svc.Build == nil {
			return nil, status.Errorf(codes.InvalidArgument, "service %q must have either 'image' or 'build'", name)
		}
	}

	// Convert proto manifest to types
	manifest := protoToManifest(req.Manifest)
	allServices := types.BuildServiceRecords(manifest.Services)

	tags := types.SortTags(req.Tags)

	// Per-service deploy path
	if len(req.Services) > 0 {
		return s.deployServices(ctx, manifest.Name, allServices, req.Services, tags)
	}

	// Full deploy path (blue-green)
	replacesID := s.prepareForRedeploy(ctx, manifest.Name, tags)

	deploymentID := fmt.Sprintf("%s-%d", manifest.Name, time.Now().Unix())
	record := &types.DeploymentRecord{
		ID:        deploymentID,
		Name:      manifest.Name,
		Status:    types.StatusPending,
		Services:  allServices,
		Tags:      tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if replacesID != "" {
		record.UpdateStrategy = types.UpdateStrategyBlueGreen
		record.ReplacesID = replacesID
	}

	if err := s.store.Save(ctx, types.KeyDeployments+deploymentID, record); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create deployment: %v", err)
	}

	s.emitEvent("deployment.created", fmt.Sprintf("Deployment %s created (%d services)", manifest.Name, len(allServices)), "info")

	return &banyanpb.DeployRPCResponse{
		DeploymentId: deploymentID,
		Status:       types.StatusPending,
	}, nil
}

func (s *engineGRPCServer) Down(ctx context.Context, req *banyanpb.DownRPCRequest) (*banyanpb.DownRPCResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	deployment, deploymentKey, err := s.findDeploymentByName(ctx, req.Name, types.SortTags(req.Tags))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}

	// Validate requested services exist
	for _, name := range req.Services {
		if _, ok := deployment.Services[name]; !ok {
			return nil, status.Errorf(codes.InvalidArgument, "service %q not found in deployment %q", name, req.Name)
		}
	}

	// Stop all services → delegate to teardownDeployment
	if len(req.Services) == 0 {
		count, teardownErr := teardownDeployment(ctx, s.store, deployment, deploymentKey)
		if teardownErr != nil {
			return nil, status.Errorf(codes.Internal, "%v", teardownErr)
		}
		s.emitEvent("deployment.stopped", fmt.Sprintf("Deployment %s stopped (%d tasks)", req.Name, count), "info")
		return &banyanpb.DownRPCResponse{TaskCount: int32(count)}, nil //nolint:gosec // task count is always small
	}

	// Selective service stop — filter by service names
	allTasks := types.CollectDeploymentTasks(ctx, s.store, deployment.ID)

	var targetTasks []types.TaskRecord
	for i := range allTasks {
		if allTasks[i].Type == types.TaskTypeCreateAndStart && allTasks[i].Status == types.StatusCompleted {
			targetTasks = append(targetTasks, allTasks[i])
		}
	}

	serviceSet := make(map[string]bool, len(req.Services))
	for _, name := range req.Services {
		serviceSet[name] = true
	}
	var filtered []types.TaskRecord
	for i := range targetTasks {
		if serviceSet[targetTasks[i].ServiceName] {
			filtered = append(filtered, targetTasks[i])
		}
	}
	targetTasks = filtered

	if len(targetTasks) == 0 {
		return &banyanpb.DownRPCResponse{TaskCount: 0}, nil
	}

	// Create stop_and_remove tasks
	now := time.Now()
	for i := range targetTasks {
		stopTask := &types.TaskRecord{
			ID:            targetTasks[i].ID + "-stop",
			DeploymentID:  targetTasks[i].DeploymentID,
			ServiceName:   targetTasks[i].ServiceName,
			AgentID:       targetTasks[i].AgentID,
			Type:          types.TaskTypeStopAndRemove,
			Status:        types.StatusPending,
			ContainerName: targetTasks[i].ContainerName,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		taskKey := types.KeyTasks + targetTasks[i].AgentID + "/" + stopTask.ID
		if err := s.store.Save(ctx, taskKey, stopTask); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create stop task: %v", err)
		}
	}

	return &banyanpb.DownRPCResponse{TaskCount: int32(len(targetTasks))}, nil //nolint:gosec // task count is always small
}

func (s *engineGRPCServer) GetStatus(ctx context.Context, req *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	// Collect agents
	allAgentKeys, _ := s.store.List(ctx, types.KeyNodes)
	var agents []*banyanpb.AgentInfo
	for _, key := range allAgentKeys {
		var node types.NodeRecord
		if err := s.store.Get(ctx, key, &node); err != nil {
			continue
		}
		agents = append(agents, &banyanpb.AgentInfo{
			Name:          node.Name,
			Status:        node.Status,
			ApiAddress:    node.APIAddress,
			LastSeenUnix:  node.LastSeen.Unix(),
			CreatedAtUnix: node.CreatedAt.Unix(),
			Tags:          node.Tags,
		})
	}

	// Collect deployments
	deployKeys, _ := s.store.List(ctx, types.KeyDeployments)
	var deployments []*banyanpb.DeploymentInfo
	for _, key := range deployKeys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}

		allTasks := types.CollectDeploymentTasks(ctx, s.store, record.ID)
		var createTasks []types.TaskRecord
		for i := range allTasks {
			if allTasks[i].Type == types.TaskTypeCreateAndStart {
				createTasks = append(createTasks, allTasks[i])
			}
		}

		healthy := 0
		for i := range createTasks {
			if createTasks[i].ContainerStatus == types.StatusRunning {
				healthy++
			}
		}

		// Convert services
		services := make(map[string]*banyanpb.ServiceInfo, len(record.Services))
		for name, svc := range record.Services {
			services[name] = &banyanpb.ServiceInfo{
				Image:       svc.Image,
				Replicas:    int32(svc.Replicas), //nolint:gosec // replica count is always small
				Ports:       svc.Ports,
				Environment: svc.Environment,
				Command:     svc.Command,
				DependsOn:   svc.DependsOn,
			}
		}

		// Convert tasks
		var taskInfos []*banyanpb.TaskInfo
		for i := range allTasks {
			taskInfos = append(taskInfos, &banyanpb.TaskInfo{
				Id:                     allTasks[i].ID,
				DeploymentId:           allTasks[i].DeploymentID,
				ServiceName:            allTasks[i].ServiceName,
				ReplicaIndex:           int32(allTasks[i].ReplicaIndex), //nolint:gosec // replica index is always small
				AgentId:                allTasks[i].AgentID,
				Type:                   allTasks[i].Type,
				Status:                 allTasks[i].Status,
				Image:                  allTasks[i].Image,
				ContainerName:          allTasks[i].ContainerName,
				Ports:                  allTasks[i].Ports,
				Environment:            allTasks[i].Environment,
				Command:                allTasks[i].Command,
				ContainerStatus:        allTasks[i].ContainerStatus,
				ContainerCheckedAtUnix: allTasks[i].ContainerCheckedAt.Unix(),
				CreatedAtUnix:          allTasks[i].CreatedAt.Unix(),
				UpdatedAtUnix:          allTasks[i].UpdatedAt.Unix(),
				Error:                  allTasks[i].Error,
			})
		}

		deployments = append(deployments, &banyanpb.DeploymentInfo{
			Id:            record.ID,
			Name:          record.Name,
			Status:        record.Status,
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

	return &banyanpb.GetStatusResponse{
		Agents:      agents,
		Deployments: deployments,
	}, nil
}

func (s *engineGRPCServer) GetLogs(req *banyanpb.GetLogsRequest, stream grpc.ServerStreamingServer[banyanpb.GetLogsResponse]) error {
	if req.ContainerName == "" {
		return status.Error(codes.InvalidArgument, "container_name is required")
	}

	ctx := stream.Context()

	// Find which agent has this container
	task, node, err := s.findContainerAgent(ctx, req.ContainerName)
	if err != nil {
		return status.Errorf(codes.NotFound, "%v", err)
	}

	if node.APIAddress == "" {
		return status.Errorf(codes.Unavailable, "container %s is on node %s but agent API is not available", req.ContainerName, task.AgentID)
	}

	// Stream logs from agent via gRPC
	sessionToken := s.GetSessionToken(task.AgentID)
	reader, err := streamAgentLogs(ctx, node.APIAddress, sessionToken, req.ContainerName, req.Follow, req.Tail)
	if err != nil {
		return status.Errorf(codes.Unavailable, "failed to connect to agent: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if err := stream.Send(&banyanpb.GetLogsResponse{Data: buf[:n]}); err != nil {
				return err
			}
		}
		if readErr != nil {
			return nil // EOF or context cancel
		}
	}
}

func (s *engineGRPCServer) GetInfo(ctx context.Context, req *banyanpb.GetInfoRequest) (*banyanpb.GetInfoResponse, error) {
	return &banyanpb.GetInfoResponse{
		RegistryUrl: s.registryURL,
	}, nil
}

func (s *engineGRPCServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

// teardownDeployment creates stop_and_remove tasks for running containers and sets
// the deployment to StatusStopping. If there are no running containers (e.g. deployment
// is still pending), it marks the deployment StatusStopped directly.
// Returns the number of stop tasks created.
func teardownDeployment(ctx context.Context, store storage.StateStore, deployment *types.DeploymentRecord, deploymentKey string) (int, error) {
	allTasks := types.CollectDeploymentTasks(ctx, store, deployment.ID)

	var targetTasks []types.TaskRecord
	for i := range allTasks {
		if allTasks[i].Type == types.TaskTypeCreateAndStart && allTasks[i].Status == types.StatusCompleted {
			targetTasks = append(targetTasks, allTasks[i])
		}
	}

	if len(targetTasks) == 0 {
		deployment.Status = types.StatusStopped
		deployment.Error = ""
		deployment.UpdatedAt = time.Now()
		if err := store.Save(ctx, deploymentKey, deployment); err != nil {
			logging.Warn("Failed to update deployment status", "error", err)
		}
		return 0, nil
	}

	now := time.Now()
	for i := range targetTasks {
		stopTask := &types.TaskRecord{
			ID:            targetTasks[i].ID + "-stop",
			DeploymentID:  targetTasks[i].DeploymentID,
			ServiceName:   targetTasks[i].ServiceName,
			AgentID:       targetTasks[i].AgentID,
			Type:          types.TaskTypeStopAndRemove,
			Status:        types.StatusPending,
			ContainerName: targetTasks[i].ContainerName,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		taskKey := types.KeyTasks + targetTasks[i].AgentID + "/" + stopTask.ID
		if err := store.Save(ctx, taskKey, stopTask); err != nil {
			return 0, fmt.Errorf("failed to create stop task: %w", err)
		}
	}

	deployment.Status = types.StatusStopping
	deployment.Error = ""
	deployment.UpdatedAt = time.Now()
	if err := store.Save(ctx, deploymentKey, deployment); err != nil {
		logging.Warn("Failed to update deployment status", "error", err)
	}

	return len(targetTasks), nil
}

// prepareForRedeploy scans active deployments with the given name+tags and prepares for a new deploy.
// Non-running active deployments (pending/deploying/failed) are torn down immediately.
// Returns the ID of the most recent running deployment for blue-green replacement.
func (s *engineGRPCServer) prepareForRedeploy(ctx context.Context, name string, tags []string) string {
	keys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return ""
	}

	var replacesID string
	var latestRunningCreatedAt time.Time

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name || !types.TagsEqual(record.Tags, tags) {
			continue
		}
		if record.Status == types.StatusStopped || record.Status == types.StatusStopping {
			continue
		}

		if record.Status == types.StatusRunning {
			if replacesID == "" || record.CreatedAt.After(latestRunningCreatedAt) {
				replacesID = record.ID
				latestRunningCreatedAt = record.CreatedAt
			}
		} else {
			// Pending/deploying/failed: teardown immediately (recreate behavior)
			r := record
			if _, teardownErr := teardownDeployment(ctx, s.store, &r, key); teardownErr != nil {
				logging.Warn("Failed to teardown deployment", "deployment_id", record.ID, "error", teardownErr)
			}
		}
	}

	return replacesID
}

// --- Per-service deploy ---

// deployServices handles per-service redeployment using the blue-green strategy.
// With the TCP proxy on each agent, containers no longer bind host ports directly,
// so old and new containers can run simultaneously without port conflicts.
func (s *engineGRPCServer) deployServices(ctx context.Context, appName string, allServices map[string]types.ServiceRecord, serviceNames []string, tags []string) (*banyanpb.DeployRPCResponse, error) {
	// Validate service names exist
	for _, name := range serviceNames {
		if _, ok := allServices[name]; !ok {
			return nil, status.Errorf(codes.InvalidArgument, "service %q not found in manifest", name)
		}
	}

	// Find running deployment for this app
	runningDeploy, _, err := s.findRunningDeploymentByName(ctx, appName, tags)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "no running deployment found for %q: %v", appName, err)
	}

	// Get running service names from existing deployment
	runningNames := s.getRunningServiceNames(ctx, runningDeploy.ID)

	// Validate depends_on: target services' deps must be running or being deployed
	if depErr := types.ValidateServiceDependencies(serviceNames, allServices, runningNames); depErr != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "%v", depErr)
	}

	// Clean up non-running deployments (pending/deploying/failed)
	s.teardownNonRunningDeployments(ctx, appName, tags)

	// Filter services to only the target ones
	filteredServices := make(map[string]types.ServiceRecord, len(serviceNames))
	for _, name := range serviceNames {
		filteredServices[name] = allServices[name]
	}

	// Create new deployment with blue-green strategy.
	// Old containers stay running until the new deployment reaches StatusRunning,
	// then the engine's blueGreenTeardownOld() tears them down.
	deploymentID := fmt.Sprintf("%s-%d", appName, time.Now().Unix())
	record := &types.DeploymentRecord{
		ID:             deploymentID,
		Name:           appName,
		Status:         types.StatusPending,
		Services:       filteredServices,
		Tags:           tags,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		UpdateStrategy: types.UpdateStrategyBlueGreen,
		ReplacesID:     runningDeploy.ID,
	}

	if err := s.store.Save(ctx, types.KeyDeployments+deploymentID, record); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create deployment: %v", err)
	}

	return &banyanpb.DeployRPCResponse{
		DeploymentId: deploymentID,
		Status:       types.StatusPending,
	}, nil
}

// findRunningDeploymentByName returns the most recent Running deployment by name+tags.
func (s *engineGRPCServer) findRunningDeploymentByName(ctx context.Context, name string, tags []string) (*types.DeploymentRecord, string, error) {
	keys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list deployments: %w", err)
	}

	var best *types.DeploymentRecord
	var bestKey string
	for _, key := range keys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name || record.Status != types.StatusRunning {
			continue
		}
		if !types.TagsEqual(record.Tags, tags) {
			continue
		}
		if best == nil || record.CreatedAt.After(best.CreatedAt) {
			r := record
			best = &r
			bestKey = key
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no running deployment found with name %q", name)
	}
	return best, bestKey, nil
}

// getRunningServiceNames returns the names of services that have completed create_and_start tasks
// for the given deployment.
func (s *engineGRPCServer) getRunningServiceNames(ctx context.Context, deploymentID string) []string {
	tasks := types.CollectDeploymentTasks(ctx, s.store, deploymentID)

	nameSet := make(map[string]bool)
	for i := range tasks {
		if tasks[i].Type == types.TaskTypeCreateAndStart && tasks[i].Status == types.StatusCompleted {
			nameSet[tasks[i].ServiceName] = true
		}
	}

	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}
	return names
}

// teardownDeploymentServices creates stop_and_remove tasks for specific services
// within a deployment. It does NOT change the deployment status.
func (s *engineGRPCServer) teardownDeploymentServices(ctx context.Context, deploymentID string, serviceNames []string) {
	serviceSet := make(map[string]bool, len(serviceNames))
	for _, name := range serviceNames {
		serviceSet[name] = true
	}

	tasks := types.CollectDeploymentTasks(ctx, s.store, deploymentID)

	now := time.Now()
	for i := range tasks {
		if tasks[i].Type != types.TaskTypeCreateAndStart || tasks[i].Status != types.StatusCompleted {
			continue
		}
		if !serviceSet[tasks[i].ServiceName] {
			continue
		}

		stopTask := &types.TaskRecord{
			ID:            tasks[i].ID + "-stop",
			DeploymentID:  tasks[i].DeploymentID,
			ServiceName:   tasks[i].ServiceName,
			AgentID:       tasks[i].AgentID,
			Type:          types.TaskTypeStopAndRemove,
			Status:        types.StatusPending,
			ContainerName: tasks[i].ContainerName,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		taskKey := types.KeyTasks + tasks[i].AgentID + "/" + stopTask.ID
		if err := s.store.Save(ctx, taskKey, stopTask); err != nil {
			s.logger().Warn("Failed to create stop task", "service", tasks[i].ServiceName, "error", err)
		}
	}
}

// teardownNonRunningDeployments tears down all non-running active deployments (pending/deploying/failed)
// with the given name+tags.
func (s *engineGRPCServer) teardownNonRunningDeployments(ctx context.Context, name string, tags []string) {
	keys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return
	}

	for _, key := range keys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name || !types.TagsEqual(record.Tags, tags) {
			continue
		}
		if record.Status == types.StatusRunning || record.Status == types.StatusStopped || record.Status == types.StatusStopping {
			continue
		}
		// Pending/deploying/failed: teardown immediately
		r := record
		if _, teardownErr := teardownDeployment(ctx, s.store, &r, key); teardownErr != nil {
			logging.Warn("Failed to teardown deployment", "deployment_id", record.ID, "error", teardownErr)
		}
	}
}

// --- Helper functions ---

// findDeploymentByName scans all deployments and returns the most recent one matching the given name+tags.
func (s *engineGRPCServer) findDeploymentByName(ctx context.Context, name string, tags []string) (*types.DeploymentRecord, string, error) {
	keys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list deployments: %w", err)
	}

	var best *types.DeploymentRecord
	var bestKey string
	for _, key := range keys {
		var record types.DeploymentRecord
		if err := s.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name != name || !types.TagsEqual(record.Tags, tags) {
			continue
		}
		if best == nil || record.CreatedAt.After(best.CreatedAt) {
			r := record
			best = &r
			bestKey = key
		}
	}

	if best == nil {
		return nil, "", fmt.Errorf("no deployment found with name %q", name)
	}
	return best, bestKey, nil
}

// findContainerAgent scans to find which agent has the given container.
func (s *engineGRPCServer) findContainerAgent(ctx context.Context, containerName string) (*types.TaskRecord, *types.NodeRecord, error) {
	nodeKeys, err := s.store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

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
			if task.ContainerName == containerName && task.Type == types.TaskTypeCreateAndStart {
				return &task, &node, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("container %q not found in cluster", containerName)
}

// protoToManifest converts a proto Manifest to types.BanyanManifest.
func protoToManifest(m *banyanpb.Manifest) types.BanyanManifest {
	services := make(map[string]types.ManifestService, len(m.Services))
	for name, svc := range m.Services {
		ms := types.ManifestService{
			Image:       svc.Image,
			Ports:       svc.Ports,
			Environment: svc.Environment,
			Command:     svc.Command,
			DependsOn:   svc.DependsOn,
		}
		if svc.Build != nil {
			ms.Build = &types.ManifestBuild{
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
			}
		}
		if svc.Deploy != nil {
			md := &types.ManifestDeploy{
				Replicas: int(svc.Deploy.Replicas),
			}
			if svc.Deploy.Placement != nil && svc.Deploy.Placement.Node != "" {
				md.Placement = &types.ManifestPlacement{
					Node: svc.Deploy.Placement.Node,
				}
			}
			ms.Deploy = md
		}
		services[name] = ms
	}
	return types.BanyanManifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
	}
}

// collectServiceBackends gathers all running container backends across all agents.
// Used for cross-host load balancing via heartbeat responses.
func (s *engineGRPCServer) collectServiceBackends(ctx context.Context) []*banyanpb.ServiceBackend {
	// Build set of running deployment IDs — only include backends from active deployments
	runningDeployments := map[string]bool{}
	deployKeys, err := s.store.List(ctx, types.KeyDeployments)
	if err != nil {
		return nil
	}
	for _, key := range deployKeys {
		var d types.DeploymentRecord
		if err := s.store.Get(ctx, key, &d); err != nil {
			continue
		}
		if d.Status == types.StatusRunning {
			runningDeployments[d.ID] = true
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
			if task.Type != types.TaskTypeCreateAndStart ||
				task.Status != types.StatusCompleted ||
				task.ContainerStatus != types.StatusRunning ||
				task.ContainerIP == "" ||
				!runningDeployments[task.DeploymentID] {
				continue
			}
			backends = append(backends, &banyanpb.ServiceBackend{
				ContainerName: task.ContainerName,
				ContainerIp:   task.ContainerIP,
				Ports:         task.Ports,
				AgentName:     task.AgentID,
				ServiceName:   task.ServiceName,
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

	// Auto-reconcile: mark superseded "running" deployments as stopped in the store
	for i := range allDeploys {
		d := &allDeploys[i]
		if d.record.Status == types.StatusRunning && !latestDeployIDs[d.record.ID] {
			d.record.Status = types.StatusStopped
			d.record.UpdatedAt = time.Now()
			if err := s.store.Save(ctx, d.key, &d.record); err != nil {
				s.logger().Warn("Failed to mark superseded deployment as stopped", "deployment_id", d.record.ID, "error", err)
			} else {
				s.emitEvent("deployment.stopped", fmt.Sprintf("Deployment %s superseded by newer version", d.record.Name), "info")
			}
		}
	}

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
		connected := node.Status == "ready" && time.Since(node.LastSeen) < 60*time.Second
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
			if allocated, _ := s.allocator.Allocate(node.Name); allocated != nil {
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
				Image:       svc.Image,
				Replicas:    int32(svc.Replicas), //nolint:gosec // replica count is always small
				Ports:       svc.Ports,
				Environment: svc.Environment,
				Command:     svc.Command,
				DependsOn:   svc.DependsOn,
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
				Environment:            allTasks[j].Environment,
				Command:                allTasks[j].Command,
				ContainerStatus:        allTasks[j].ContainerStatus,
				ContainerCheckedAtUnix: allTasks[j].ContainerCheckedAt.Unix(),
				CreatedAtUnix:          allTasks[j].CreatedAt.Unix(),
				UpdatedAtUnix:          allTasks[j].UpdatedAt.Unix(),
				Error:                  allTasks[j].Error,
			})
		}

		resp.Deployments = append(resp.Deployments, &banyanpb.DeploymentInfo{
			Id:            record.ID,
			Name:          record.Name,
			Status:        record.Status,
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

// emitEvent adds an event to the buffer and increments the Prometheus counter.
func (s *engineGRPCServer) emitEvent(eventType, message, severity string) {
	if s.events != nil {
		s.events.Add(Event{
			Timestamp: time.Now(),
			Type:      eventType,
			Message:   message,
			Severity:  severity,
		})
	}
	if s.metricsRegistry != nil {
		s.metricsRegistry.IncrementEvent(eventType)
	}
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
