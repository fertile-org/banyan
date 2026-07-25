// grpc_handlers_cli.go contains gRPC handlers for CLI-to-engine RPCs:
// Deploy, Down, GetStatus, GetLogs, GetInfo, Health, Scale,
// and related helper methods (teardownDeployment, prepareForRedeploy, etc.).
package engine

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/fertile-org/banyan/pkg/logging"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

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

	// Validate autoscale bounds
	for svcName, svc := range allServices {
		if svc.Autoscale == nil {
			continue
		}
		as := svc.Autoscale
		if as.Min < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: autoscale.min must be >= 0, got %d", svcName, as.Min)
		}
		if as.Max < 1 {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: autoscale.max must be >= 1, got %d", svcName, as.Max)
		}
		if as.Min > as.Max {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: autoscale.min (%d) cannot exceed autoscale.max (%d)", svcName, as.Min, as.Max)
		}
		if as.Max > types.MaxReplicas {
			return nil, status.Errorf(codes.InvalidArgument, "service %q: autoscale.max (%d) exceeds maximum replicas (%d)", svcName, as.Max, types.MaxReplicas)
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
		targetTasks[i].StopReason = "user"
		targetTasks[i].UpdatedAt = now
		origKey := types.KeyTasks + targetTasks[i].AgentID + "/" + targetTasks[i].ID
		_ = s.store.Save(ctx, origKey, &targetTasks[i])

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
			Arch:          node.Arch,
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
	task, node, err := s.findContainerAgent(ctx, req.ContainerName, req.AgentId)
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
		if targetReplicas < 0 {
			return nil, status.Errorf(codes.InvalidArgument, "replica count for %q must be >= 0, got %d", svcName, targetReplicas)
		}
		if targetReplicas > int32(types.MaxReplicas) { //nolint:gosec // MaxReplicas is always small
			return nil, status.Errorf(codes.InvalidArgument, "replica count for %q exceeds maximum (%d)", svcName, types.MaxReplicas)
		}
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
		targetTasks[i].StopReason = "user"
		targetTasks[i].UpdatedAt = now
		origKey := types.KeyTasks + targetTasks[i].AgentID + "/" + targetTasks[i].ID
		_ = store.Save(ctx, origKey, &targetTasks[i])

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

// findContainerAgent finds which agent has the given container.
//
// If agentID is provided, does a direct O(1) lookup — no scanning.
// If agentID is empty (CLI fallback), scans only healthy agents.
func (s *engineGRPCServer) findContainerAgent(ctx context.Context, containerName, agentID string) (*types.TaskRecord, *types.NodeRecord, error) {
	// Direct lookup when agent_id is provided (web dashboard, connect API)
	if agentID != "" {
		var node types.NodeRecord
		if err := s.store.Get(ctx, types.KeyNodes+agentID, &node); err != nil {
			return nil, nil, fmt.Errorf("agent %q not found", agentID)
		}

		taskKeys, err := s.store.List(ctx, types.KeyTasks+agentID+"/")
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list tasks on agent %q: %w", agentID, err)
		}

		var best *types.TaskRecord
		for _, key := range taskKeys {
			var task types.TaskRecord
			if err := s.store.Get(ctx, key, &task); err != nil {
				continue
			}
			if task.ContainerName == containerName && task.Type == types.TaskTypeCreateAndStart {
				if best == nil || task.CreatedAt.After(best.CreatedAt) {
					t := task
					best = &t
				}
			}
		}
		if best == nil {
			return nil, nil, fmt.Errorf("container %q not found on agent %q", containerName, agentID)
		}
		return best, &node, nil
	}

	// Fallback: scan healthy agents only (CLI which doesn't have agent_id)
	nodeKeys, err := s.store.List(ctx, types.KeyNodes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var bestTask *types.TaskRecord
	var bestNode *types.NodeRecord

	for _, nodeKey := range nodeKeys {
		var node types.NodeRecord
		if err := s.store.Get(ctx, nodeKey, &node); err != nil {
			continue
		}
		if node.Status != "ready" || time.Since(node.LastSeen) > agentStalenessThreshold {
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
			if task.ContainerName != containerName || task.Type != types.TaskTypeCreateAndStart {
				continue
			}
			if bestTask == nil || task.CreatedAt.After(bestTask.CreatedAt) {
				t := task
				n := node
				bestTask = &t
				bestNode = &n
			}
		}
	}

	if bestTask == nil {
		return nil, nil, fmt.Errorf("container %q not found on any healthy agent", containerName)
	}
	return bestTask, bestNode, nil
}

// StopTask stops a single container by creating a stop_and_remove task for the given task.
func (s *engineGRPCServer) StopTask(ctx context.Context, req *banyanpb.StopTaskRequest) (*banyanpb.StopTaskResponse, error) {
	if req.TaskId == "" || req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id and agent_id are required")
	}

	// Look up the original task
	taskKey := types.KeyTasks + req.AgentId + "/" + req.TaskId
	var task types.TaskRecord
	if err := s.store.Get(ctx, taskKey, &task); err != nil {
		return nil, status.Errorf(codes.NotFound, "task %q not found on agent %q", req.TaskId, req.AgentId)
	}

	// Verify it's a running container
	if task.Type != types.TaskTypeCreateAndStart || task.Status != types.StatusCompleted || task.ContainerStatus != types.StatusRunning {
		return nil, status.Error(codes.FailedPrecondition, "container not running")
	}

	// Check if a stop task already exists
	stopTaskID := req.TaskId + "-stop"
	stopTaskKey := types.KeyTasks + req.AgentId + "/" + stopTaskID
	var existing types.TaskRecord
	if err := s.store.Get(ctx, stopTaskKey, &existing); err == nil {
		return nil, status.Error(codes.AlreadyExists, "already stopping")
	}

	// Mark original task with user stop reason
	task.StopReason = "user"
	task.UpdatedAt = time.Now()
	_ = s.store.Save(ctx, taskKey, &task)

	// Create the stop task
	now := time.Now()
	stopTask := &types.TaskRecord{
		ID:            stopTaskID,
		DeploymentID:  task.DeploymentID,
		ServiceName:   task.ServiceName,
		ReplicaIndex:  task.ReplicaIndex,
		AgentID:       task.AgentID,
		Type:          types.TaskTypeStopAndRemove,
		Status:        types.StatusPending,
		ContainerName: task.ContainerName,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.Save(ctx, stopTaskKey, stopTask); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create stop task: %v", err)
	}

	s.emitEvent("task.stopped", fmt.Sprintf("Stopping container %s on %s", task.ContainerName, req.AgentId), "info")

	return &banyanpb.StopTaskResponse{
		StopTaskId: stopTaskID,
		Status:     "stopping",
	}, nil
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
