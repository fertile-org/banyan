package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// engineGRPCServer implements the EngineService gRPC server.
type engineGRPCServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store        storage.StateStore
	sessions     sync.Map // map[agentName]sessionToken
	registryURL  string
	passwordHash string
}

// startEngineGRPC starts the gRPC server for agent communication.
func startEngineGRPC(ctx context.Context, store storage.StateStore, port, passwordHash, registryURL string) (*engineGRPCServer, error) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on gRPC port %s: %w", port, err)
	}

	engineSrv := &engineGRPCServer{
		store:        store,
		registryURL:  registryURL,
		passwordHash: passwordHash,
	}

	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(passwordHash, engineSrv)),
		grpc.StreamInterceptor(banyanrpc.NewAuthStreamInterceptor(engineSrv)),
	)

	banyanpb.RegisterEngineServiceServer(srv, engineSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			fmt.Printf("Engine gRPC server error: %v\n", err)
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

	fmt.Printf("[Engine] Agent registered: %s (api: %s)\n", req.AgentName, req.ApiAddress)

	return &banyanpb.RegisterResponse{
		RegistryUrl: s.registryURL,
	}, nil
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

	return &banyanpb.HeartbeatResponse{}, nil
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

	// Build a map of container statuses for quick lookup
	statusMap := make(map[string]string, len(req.Containers))
	for _, c := range req.Containers {
		statusMap[c.ContainerName] = c.Status
	}

	// Update matching tasks in store
	taskPrefix := types.KeyTasks + req.AgentName + "/"
	keys, err := s.store.List(ctx, taskPrefix)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list tasks: %v", err)
	}

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
			if err := s.store.Save(ctx, key, &task); err != nil {
				log.Printf("WARNING: failed to save container health for %s: %v", task.ContainerName, err)
			}
		}
	}

	return &banyanpb.ReportContainerHealthResponse{}, nil
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

// ExchangeToken validates the password and returns a random auth token.
func (s *engineGRPCServer) ExchangeToken(ctx context.Context, req *banyanpb.ExchangeTokenRequest) (*banyanpb.ExchangeTokenResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Role == "" {
		return nil, status.Error(codes.InvalidArgument, "role is required")
	}

	// Delete existing token for this name (re-issue)
	var existingHash string
	indexKey := types.KeyTokenIndex + req.Name
	if err := s.store.Get(ctx, indexKey, &existingHash); err == nil {
		_ = s.store.Delete(ctx, types.KeyTokens+existingHash)
		_ = s.store.Delete(ctx, indexKey)
	}

	// Generate random token (32 bytes → 64-char hex)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token: %v", err)
	}
	token := hex.EncodeToString(tokenBytes)
	tokenHash := banyanrpc.HashPassword(token)

	// Build token record with TTL for CLI tokens
	record := types.TokenRecord{Name: req.Name, Role: req.Role}
	if req.Role == "cli" {
		record.ExpiresAt = time.Now().Add(types.DefaultCLITokenTTL)
	}

	// Store dual index: hash→record and name→hash
	if err := s.store.Save(ctx, types.KeyTokens+tokenHash, &record); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store token: %v", err)
	}
	if err := s.store.Save(ctx, indexKey, tokenHash); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to store token index: %v", err)
	}

	fmt.Printf("[Engine] Token issued for %s (role: %s)\n", req.Name, req.Role)

	return &banyanpb.ExchangeTokenResponse{Token: token}, nil
}

// ValidateToken implements rpc.TokenValidator by looking up the token hash in the store.
func (s *engineGRPCServer) ValidateToken(ctx context.Context, tokenHash string) error {
	var record types.TokenRecord
	if err := s.store.Get(ctx, types.KeyTokens+tokenHash, &record); err != nil {
		return status.Error(codes.Unauthenticated, "invalid auth token")
	}
	if record.IsExpired() {
		return status.Error(codes.Unauthenticated, "token expired, run 'auth' to re-authenticate")
	}
	return nil
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
			log.Printf("WARNING: failed to update deployment status: %v", err)
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
		log.Printf("WARNING: failed to update deployment status: %v", err)
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
				log.Printf("WARNING: failed to teardown deployment %s: %v", record.ID, teardownErr)
			}
		}
	}

	return replacesID
}

// --- Per-service deploy ---

// deployServices handles per-service redeployment using the recreate strategy.
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

	// Create stop tasks for target services in the running deployment
	s.teardownDeploymentServices(ctx, runningDeploy.ID, serviceNames)

	// Filter services to only the target ones
	filteredServices := make(map[string]types.ServiceRecord, len(serviceNames))
	for _, name := range serviceNames {
		filteredServices[name] = allServices[name]
	}

	// Create new deployment with recreate strategy
	deploymentID := fmt.Sprintf("%s-%d", appName, time.Now().Unix())
	record := &types.DeploymentRecord{
		ID:             deploymentID,
		Name:           appName,
		Status:         types.StatusPending,
		Services:       filteredServices,
		Tags:           tags,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		UpdateStrategy: types.UpdateStrategyRecreate,
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
			log.Printf("WARNING: failed to create stop task for service %s: %v", tasks[i].ServiceName, err)
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
			log.Printf("WARNING: failed to teardown deployment %s: %v", record.ID, teardownErr)
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
			ms.Deploy = &types.ManifestDeploy{
				Replicas: int(svc.Deploy.Replicas),
			}
		}
		services[name] = ms
	}
	return types.BanyanManifest{
		Name:     m.Name,
		Version:  m.Version,
		Services: services,
	}
}
