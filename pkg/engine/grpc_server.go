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

// rateLimiter provides per-IP request rate limiting.
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time // IP → timestamps of recent requests
	limit    int                    // max requests per window
	window   time.Duration          // sliding window duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// allow returns true if the request from the given IP is within rate limits.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Trim old entries
	timestamps := rl.requests[ip]
	start := 0
	for start < len(timestamps) && timestamps[start].Before(cutoff) {
		start++
	}
	timestamps = timestamps[start:]

	if len(timestamps) >= rl.limit {
		rl.requests[ip] = timestamps
		return false
	}

	rl.requests[ip] = append(timestamps, now)
	return true
}

// engineGRPCServer implements the EngineService gRPC server.
type engineGRPCServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store           storage.StateStore
	registryURL     string
	whitelistedKeys map[string]string                // publicKey → agentName
	tunnelIPToAgent map[string]string                // tunnelIP → agentName (reverse map for identity)
	allocator       overlay.SubnetAllocatorInterface // VPC subnet allocator (nil if VPC disabled)
	peerTracker     overlay.PeerTrackerInterface     // VPC peer tracker (nil if VPC disabled)
	vpcCIDR         string                           // VPC network CIDR (e.g., "10.0.0.0/16")
	scheduleCh      chan<- struct{}                   // triggers immediate scheduling (nil-safe)
	metricsRegistry *metrics.EngineMetricsRegistry
	events          EventLog
	secrets         *SecretsManager // nil if secrets.key not present
	limiter         *rateLimiter
	startedAt       time.Time
	log             *logging.Logger
}

// grpcServerOptions configures the engine gRPC server.
type grpcServerOptions struct {
	Store           storage.StateStore
	Port            string
	BindAddr        string // address to bind to (default: control tunnel IP or 127.0.0.1)
	RegistryURL     string
	Allocator       overlay.SubnetAllocatorInterface
	PeerTracker     overlay.PeerTrackerInterface
	VPCCIDR         string
	WhitelistedKeys map[string]string // publicKey → agentName
	AllowInsecure   bool              // allow running without authentication
	ScheduleCh      chan<- struct{}    // triggers immediate scheduling
	MetricsRegistry *metrics.EngineMetricsRegistry
	Events          EventLog
	Secrets         *SecretsManager // nil if secrets.key not present
	StartedAt       time.Time
}

// triggerSchedule signals the engine orchestration loop to process deployments immediately.
func (s *engineGRPCServer) triggerSchedule() {
	if s.scheduleCh == nil {
		return
	}
	select {
	case s.scheduleCh <- struct{}{}:
	default:
	}
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
	bindAddr := opts.BindAddr
	if bindAddr == "" {
		bindAddr = "127.0.0.1"
	}
	listenAddr := bindAddr + ":" + opts.Port
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC %s: %w", listenAddr, err)
	}

	// Build reverse map: tunnel IP → agent name (for caller identity from WireGuard)
	tunnelIPMap := make(map[string]string, len(opts.WhitelistedKeys))
	for pubKey, agentName := range opts.WhitelistedKeys {
		tunnelIP := types.TunnelIPFromPublicKey(pubKey)
		tunnelIPMap[tunnelIP.String()] = agentName
	}

	engineSrv := &engineGRPCServer{
		store:           opts.Store,
		registryURL:     opts.RegistryURL,
		whitelistedKeys: opts.WhitelistedKeys,
		tunnelIPToAgent: tunnelIPMap,
		allocator:       opts.Allocator,
		peerTracker:     opts.PeerTracker,
		vpcCIDR:         opts.VPCCIDR,
		scheduleCh:      opts.ScheduleCh,
		metricsRegistry: opts.MetricsRegistry,
		events:          opts.Events,
		secrets:         opts.Secrets,
		limiter:         newRateLimiter(100, time.Minute), // 100 requests/min per IP
		startedAt:       opts.StartedAt,
		log:             logging.New("engine"),
	}

	// WireGuard provides authentication at the network layer.
	// No application-layer auth interceptors needed — only peers with
	// whitelisted WireGuard keys can reach the tunnel IP.
	// Add audit logging, rate limiting, and panic recovery interceptors.
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			engineSrv.panicRecoveryInterceptor(),
			engineSrv.rateLimitInterceptor(),
			engineSrv.auditLogInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			engineSrv.panicRecoveryStreamInterceptor(),
			engineSrv.auditLogStreamInterceptor(),
		),
	)
	if opts.AllowInsecure {
		engineSrv.logger().Warn("gRPC server running WITHOUT authentication (--allow-insecure)")
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

// agentNameFromContext resolves the agent name from the gRPC peer's tunnel IP.
func (s *engineGRPCServer) agentNameFromContext(ctx context.Context) string {
	ip, err := banyanrpc.PeerIPFromContext(ctx)
	if err != nil {
		return ""
	}
	return s.tunnelIPToAgent[ip]
}

// panicRecoveryInterceptor returns a unary interceptor that recovers from panics in handlers.
func (s *engineGRPCServer) panicRecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				s.logger().Error("Panic in gRPC handler",
					"method", info.FullMethod, "panic", r)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// panicRecoveryStreamInterceptor returns a stream interceptor that recovers from panics.
func (s *engineGRPCServer) panicRecoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				s.logger().Error("Panic in stream gRPC handler",
					"method", info.FullMethod, "panic", r)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// rateLimitInterceptor returns a unary interceptor that enforces per-IP rate limiting.
func (s *engineGRPCServer) rateLimitInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		peerIP, _ := banyanrpc.PeerIPFromContext(ctx)
		if peerIP != "" && !s.limiter.allow(peerIP) {
			s.logger().Warn("Rate limited", "peer_ip", peerIP, "method", info.FullMethod)
			return nil, status.Errorf(codes.ResourceExhausted, "rate limit exceeded — try again later")
		}
		return handler(ctx, req)
	}
}

// isControlTunnelIP returns true if the IP is on the WireGuard control tunnel network.
// Any IP on this network is an authenticated WG peer (the tunnel enforces this).
func isControlTunnelIP(ip string) bool {
	_, cidr, _ := net.ParseCIDR(types.ControlTunnelCIDR)
	return cidr != nil && cidr.Contains(net.ParseIP(ip))
}

// auditLogInterceptor returns a unary interceptor that logs all RPC calls with peer identity.
func (s *engineGRPCServer) auditLogInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		peerIP, _ := banyanrpc.PeerIPFromContext(ctx)
		agent := s.tunnelIPToAgent[peerIP]

		// Warn about unknown peers — but not for IPs on the control tunnel
		// (same-host agent/CLI traffic may arrive from the engine's own tunnel IP)
		if len(s.tunnelIPToAgent) > 0 && agent == "" && peerIP != "" && !isControlTunnelIP(peerIP) {
			s.logger().Warn("RPC from unknown peer",
				"method", info.FullMethod, "peer_ip", peerIP)
		}

		resp, err := handler(ctx, req)
		if err != nil {
			s.logger().Debug("RPC failed",
				"method", info.FullMethod, "peer_ip", peerIP, "agent", agent, "error", err)
		}
		return resp, err
	}
}

// auditLogStreamInterceptor returns a stream interceptor that logs stream RPC calls with peer identity.
func (s *engineGRPCServer) auditLogStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		peerIP, _ := banyanrpc.PeerIPFromContext(ss.Context())
		agent := s.tunnelIPToAgent[peerIP]

		if len(s.tunnelIPToAgent) > 0 && agent == "" && peerIP != "" && !isControlTunnelIP(peerIP) {
			s.logger().Warn("Stream RPC from unknown peer",
				"method", info.FullMethod, "peer_ip", peerIP)
		}

		err := handler(srv, ss)
		if err != nil {
			s.logger().Debug("Stream RPC failed",
				"method", info.FullMethod, "peer_ip", peerIP, "agent", agent, "error", err)
		}
		return err
	}
}

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
		cpuPercent   float64
		memUsed      uint64
		memLimit     uint64
		hasMetrics   bool
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

// --- CLI RPC handlers ---

func (s *engineGRPCServer) Deploy(ctx context.Context, req *banyanpb.DeployRPCRequest) (*banyanpb.DeployRPCResponse, error) {
	if req.Manifest == nil || req.Manifest.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "manifest must have a name")
	}
	if err := types.ValidateName(req.Manifest.Name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid app name: %v", err)
	}
	if len(req.Manifest.Services) == 0 {
		return nil, status.Error(codes.InvalidArgument, "manifest must define at least one service")
	}

	for name, svc := range req.Manifest.Services {
		if err := types.ValidateName(name); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service name %q: %v", name, err)
		}
		if svc.Image == "" && svc.Build == nil {
			return nil, status.Errorf(codes.InvalidArgument, "service %q must have either 'image' or 'build'", name)
		}
		for _, port := range svc.Ports {
			if err := types.ValidatePort(port); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "service %q: %v", name, err)
			}
		}
		if svc.Deploy != nil && svc.Deploy.Replicas > types.MaxReplicas {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: replicas %d exceeds maximum (%d)", name, svc.Deploy.Replicas, types.MaxReplicas)
		}
		if err := types.ValidateRestartPolicy(svc.Restart); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: %v", name, err)
		}
	}

	// Convert proto manifest to types
	manifest := protoToManifest(req.Manifest)
	allServices := types.BuildServiceRecords(manifest.Services)

	// Validate secret references
	for svcName, svc := range allServices {
		if len(svc.Secrets) == 0 {
			continue
		}
		if s.secrets == nil {
			return nil, status.Errorf(codes.FailedPrecondition,
				"service %q references secrets but secrets are not enabled on the engine (missing secrets.key)",
				svcName)
		}
		for _, secretName := range svc.Secrets {
			if _, err := s.secrets.Get(ctx, secretName); err != nil {
				return nil, status.Errorf(codes.InvalidArgument,
					"service %q references secret %q which does not exist. Create it with: banyan-cli secret create %s",
					svcName, secretName, secretName)
			}
		}
	}

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

	// Trigger immediate scheduling instead of waiting for next loop tick
	s.triggerSchedule()

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
			return nil, status.Errorf(codes.Internal, "failed to teardown deployment: %v", teardownErr)
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
				Image:     svc.Image,
				Replicas:  int32(svc.Replicas), //nolint:gosec // replica count is always small
				Ports:     svc.Ports,
				Command:   svc.Command,
				DependsOn: dependsOnToProto(svc.DependsOn),
				// Environment intentionally omitted — may contain secrets
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
				Command:                allTasks[i].Command,
				ContainerStatus:        allTasks[i].ContainerStatus,
				HealthStatus:           allTasks[i].HealthStatus,
				ContainerCheckedAtUnix: allTasks[i].ContainerCheckedAt.Unix(),
				CreatedAtUnix:          allTasks[i].CreatedAt.Unix(),
				UpdatedAtUnix:          allTasks[i].UpdatedAt.Unix(),
				Error:                  allTasks[i].Error,
				// Environment intentionally omitted — may contain secrets
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

	// Stream logs from agent via gRPC (no auth needed — agent verifies engine IP)
	reader, err := streamAgentLogs(ctx, node.APIAddress, req.ContainerName, req.Follow, req.Tail)
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

// Scale adjusts the replica count of services in a running deployment without
// blue-green redeployment. Adds new tasks (scale up) or creates stop tasks (scale down).
func (s *engineGRPCServer) Scale(ctx context.Context, req *banyanpb.ScaleRequest) (*banyanpb.ScaleResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if len(req.Replicas) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one service replica count is required")
	}

	tags := types.SortTags(req.Tags)
	deployment, deploymentKey, err := s.findRunningDeploymentByName(ctx, req.Name, tags)
	if err != nil {
		return nil, err
	}

	agents, agentErr := ListAvailableAgents(ctx, s.store, tags)
	if agentErr != nil || len(agents) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "no available agents")
	}

	previous := make(map[string]int32)
	current := make(map[string]int32)

	for svcName, targetReplicas := range req.Replicas {
		svc, ok := deployment.Services[svcName]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "service %q not found in deployment %q", svcName, req.Name)
		}

		// Count current running tasks for this service
		allTasks := types.CollectDeploymentTasks(ctx, s.store, deployment.ID)
		var runningTasks []types.TaskRecord
		for _, t := range allTasks {
			if t.ServiceName == svcName && t.Type == types.TaskTypeCreateAndStart && t.Status == types.StatusCompleted {
				runningTasks = append(runningTasks, t)
			}
		}

		currentCount := int32(len(runningTasks)) //nolint:gosec // count is small
		previous[svcName] = currentCount
		target := targetReplicas

		if target > currentCount {
			// Scale up: create new tasks
			for i := currentCount; i < target; i++ {
				agent := agents[int(i)%len(agents)]
				now := time.Now()
				task := &types.TaskRecord{
					ID:                fmt.Sprintf("%s-%s-%d", deployment.ID, svcName, i),
					DeploymentID:      deployment.ID,
					DeploymentName:    deployment.Name,
					ServiceName:       svcName,
					ReplicaIndex:      int(i),
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
				if saveErr := s.store.Save(ctx, taskKey, task); saveErr != nil {
					return nil, status.Errorf(codes.Internal, "failed to create scale-up task: %v", saveErr)
				}
			}
		} else if target < currentCount {
			// Scale down: create stop tasks for excess replicas (highest index first)
			for i := currentCount - 1; i >= target; i-- {
				if int(i) >= len(runningTasks) {
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
				if saveErr := s.store.Save(ctx, taskKey, stopTask); saveErr != nil {
					return nil, status.Errorf(codes.Internal, "failed to create scale-down task: %v", saveErr)
				}
			}
		}

		// Update replica count in deployment record
		svc.Replicas = int(target)
		deployment.Services[svcName] = svc
		current[svcName] = target
	}

	// Save updated deployment
	deployment.UpdatedAt = time.Now()
	if saveErr := s.store.Save(ctx, deploymentKey, deployment); saveErr != nil {
		return nil, status.Errorf(codes.Internal, "failed to update deployment: %v", saveErr)
	}

	s.triggerSchedule()

	return &banyanpb.ScaleResponse{
		DeploymentId: deployment.ID,
		Previous:     previous,
		Current:      current,
	}, nil
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

	s.triggerSchedule()

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

// dependsOnToProto converts types.DependsOnConfig to the proto map representation.
func dependsOnToProto(deps types.DependsOnConfig) map[string]*banyanpb.DependsOnCondition {
	if len(deps) == 0 {
		return nil
	}
	m := make(map[string]*banyanpb.DependsOnCondition, len(deps))
	for name, cond := range deps {
		m[name] = &banyanpb.DependsOnCondition{Condition: cond.Condition}
	}
	return m
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
			Restart:     svc.Restart,
			Entrypoint:  svc.Entrypoint,
		}
		if len(svc.DependsOn) > 0 {
			ms.DependsOn = make(types.DependsOnConfig, len(svc.DependsOn))
			for depName, dep := range svc.DependsOn {
				ms.DependsOn[depName] = types.DependsOnCondition{Condition: dep.Condition}
			}
		}
		if svc.Healthcheck != nil {
			ms.Healthcheck = &types.ManifestHealthcheck{
				Test:        svc.Healthcheck.Test,
				Interval:    svc.Healthcheck.Interval,
				Timeout:     svc.Healthcheck.Timeout,
				Retries:     int(svc.Healthcheck.Retries),
				StartPeriod: svc.Healthcheck.StartPeriod,
				Disable:     svc.Healthcheck.Disable,
			}
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
			if svc.Deploy.Resources != nil {
				mr := &types.ManifestResources{}
				if svc.Deploy.Resources.Limits != nil {
					mr.Limits = &types.ResourceSpec{
						Memory: svc.Deploy.Resources.Limits.Memory,
						CPUs:   svc.Deploy.Resources.Limits.Cpus,
					}
				}
				if svc.Deploy.Resources.Reservations != nil {
					mr.Reservations = &types.ResourceSpec{
						Memory: svc.Deploy.Resources.Reservations.Memory,
						CPUs:   svc.Deploy.Resources.Reservations.Cpus,
					}
				}
				md.Resources = mr
			}
			if svc.Deploy.Autoscale != nil {
				md.Autoscale = &types.ManifestAutoscale{
					Min:       int(svc.Deploy.Autoscale.Min),
					Max:       int(svc.Deploy.Autoscale.Max),
					TargetCPU: int(svc.Deploy.Autoscale.TargetCpu),
					Cooldown:  svc.Deploy.Autoscale.Cooldown,
				}
			}
			if svc.Deploy.StopGracePeriod != "" {
				md.StopGracePeriod = svc.Deploy.StopGracePeriod
			}
			ms.Deploy = md
		}
		// Volume mounts
		for _, vol := range svc.Volumes {
			vm := types.VolumeMount{
				Type:     vol.Type,
				Source:   vol.Source,
				Target:   vol.Target,
				ReadOnly: vol.ReadOnly,
			}
			if vol.Tmpfs != nil {
				vm.Tmpfs = &types.TmpfsOpt{Size: vol.Tmpfs.Size}
			}
			ms.Volumes = append(ms.Volumes, vm)
		}

		ms.Secrets = svc.Secrets

		services[name] = ms
	}

	// Top-level volume configs
	var volumes map[string]types.VolumeConfig
	if len(m.Volumes) > 0 {
		volumes = make(map[string]types.VolumeConfig, len(m.Volumes))
		for name, vc := range m.Volumes {
			volumes[name] = types.VolumeConfig{
				Driver:     vc.Driver,
				DriverOpts: vc.DriverOpts,
				External:   vc.External,
				Name:       vc.Name,
			}
		}
	}

	return types.BanyanManifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
		Volumes:  volumes,
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
		if err := s.store.Get(ctx, key, &d); err != nil {
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
				// Environment intentionally omitted — may contain secrets
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

// --- Secret RPCs ---

func (s *engineGRPCServer) CreateSecret(ctx context.Context, req *banyanpb.CreateSecretRequest) (*banyanpb.CreateSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}
	if err := ValidateSecretName(req.Name); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	// Try create first; if exists, update
	if err := s.secrets.Create(ctx, req.Name, req.Value); err != nil {
		// If already exists, update
		if updateErr := s.secrets.Update(ctx, req.Name, req.Value); updateErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to create/update secret: %v", updateErr)
		}
		s.emitEvent("secret.updated", fmt.Sprintf("Secret %q updated", req.Name), "info")
	} else {
		s.emitEvent("secret.created", fmt.Sprintf("Secret %q created", req.Name), "info")
	}
	return &banyanpb.CreateSecretResponse{}, nil
}

func (s *engineGRPCServer) ListSecrets(ctx context.Context, _ *banyanpb.ListSecretsRequest) (*banyanpb.ListSecretsResponse, error) {
	if s.secrets == nil {
		return &banyanpb.ListSecretsResponse{}, nil
	}
	records, err := s.secrets.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list secrets: %v", err)
	}
	var infos []*banyanpb.SecretInfo
	for _, r := range records {
		infos = append(infos, &banyanpb.SecretInfo{
			Name:      r.Name,
			CreatedAt: r.CreatedAt.Format(time.RFC3339),
			UpdatedAt: r.UpdatedAt.Format(time.RFC3339),
		})
	}
	return &banyanpb.ListSecretsResponse{Secrets: infos}, nil
}

func (s *engineGRPCServer) GetSecret(ctx context.Context, req *banyanpb.GetSecretRequest) (*banyanpb.GetSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}
	meta, err := s.secrets.GetMetadata(ctx, req.Name)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	resp := &banyanpb.GetSecretResponse{
		Name:      meta.Name,
		CreatedAt: meta.CreatedAt.Format(time.RFC3339),
		UpdatedAt: meta.UpdatedAt.Format(time.RFC3339),
	}
	if req.Reveal {
		value, decErr := s.secrets.Get(ctx, req.Name)
		if decErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to decrypt secret: %v", decErr)
		}
		resp.Value = value
	}
	return resp, nil
}

func (s *engineGRPCServer) DeleteSecret(ctx context.Context, req *banyanpb.DeleteSecretRequest) (*banyanpb.DeleteSecretResponse, error) {
	if s.secrets == nil {
		return nil, status.Error(codes.FailedPrecondition, "secrets not enabled (missing secrets.key on engine)")
	}

	// Block deletion if any running deployment references this secret
	depKeys, _ := s.store.List(ctx, types.KeyDeployments)
	for _, key := range depKeys {
		var dep types.DeploymentRecord
		if getErr := s.store.Get(ctx, key, &dep); getErr != nil {
			continue
		}
		if dep.Status != types.StatusRunning {
			continue
		}
		for svcName, svc := range dep.Services {
			for _, ref := range svc.Secrets {
				if ref == req.Name {
					return nil, status.Errorf(codes.FailedPrecondition,
						"cannot delete secret %q: referenced by deployment %q (service: %s)",
						req.Name, dep.Name, svcName)
				}
			}
		}
	}

	if err := s.secrets.Delete(ctx, req.Name); err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	s.emitEvent("secret.deleted", fmt.Sprintf("Secret %q deleted", req.Name), "info")
	return &banyanpb.DeleteSecretResponse{}, nil
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
