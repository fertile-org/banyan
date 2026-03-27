package engine

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// mockAgentService implements AgentServiceServer for testing the GetLogs proxy path.
type mockAgentService struct {
	banyanpb.UnimplementedAgentServiceServer
	logData [][]byte // chunks of log data to send
}

func (m *mockAgentService) StreamLogs(req *banyanpb.StreamLogsRequest, stream grpc.ServerStreamingServer[banyanpb.StreamLogsResponse]) error {
	for _, chunk := range m.logData {
		if err := stream.Send(&banyanpb.StreamLogsResponse{Data: chunk}); err != nil {
			return err
		}
	}
	return nil
}

// slowMockAgentService sends log chunks with a delay between each.
type slowMockAgentService struct {
	banyanpb.UnimplementedAgentServiceServer
	chunkCount int
	delay      time.Duration
}

func (m *slowMockAgentService) StreamLogs(req *banyanpb.StreamLogsRequest, stream grpc.ServerStreamingServer[banyanpb.StreamLogsResponse]) error {
	for i := 0; i < m.chunkCount; i++ {
		if err := stream.Send(&banyanpb.StreamLogsResponse{Data: []byte("log data\n")}); err != nil {
			return err
		}
		time.Sleep(m.delay)
	}
	return nil
}

// startSlowMockAgentServer starts a mock AgentService that sends chunks slowly.
func startSlowMockAgentServer(t *testing.T, chunkCount int, delay time.Duration) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	banyanpb.RegisterAgentServiceServer(srv, &slowMockAgentService{chunkCount: chunkCount, delay: delay})

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("slow mock agent server error: %v", err)
		}
	}()

	cleanup := func() {
		srv.Stop()
	}

	return lis.Addr().String(), cleanup
}

// startMockAgentServer starts a mock AgentService gRPC server on a random TCP port.
// It returns the listener address and a cleanup function.
func startMockAgentServer(t *testing.T, logData [][]byte) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	banyanpb.RegisterAgentServiceServer(srv, &mockAgentService{logData: logData})

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("mock agent server error: %v", err)
		}
	}()

	cleanup := func() {
		srv.Stop()
	}

	return lis.Addr().String(), cleanup
}

const bufSize = 1024 * 1024

func setupTestServer(t *testing.T) (banyanpb.EngineServiceClient, *engineGRPCServer, func()) {
	t.Helper()

	store := storage.NewMemoryStore()

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()

	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)

	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := banyanpb.NewEngineServiceClient(conn)
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}

	return client, engineSrv, cleanup
}

func TestRegister(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("successful registration", func(t *testing.T) {
		resp, err := client.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			ApiAddress:   "worker-1:9090",
			SessionToken: "token-abc",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		if resp.RegistryUrl != "localhost:5000" {
			t.Errorf("expected registry_url 'localhost:5000', got %q", resp.RegistryUrl)
		}
	})

	t.Run("missing agent name", func(t *testing.T) {
		_, err := client.Register(ctx, &banyanpb.RegisterRequest{
			SessionToken: "token",
		})
		if err == nil {
			t.Fatal("expected error for missing agent_name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", status.Code(err))
		}
	})

}

func TestHeartbeat(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Register first
	client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "worker-1",
		ApiAddress:   "worker-1:9090",
		SessionToken: "old-token",
	})

	t.Run("successful heartbeat", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName: "worker-1",
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
	})

	t.Run("missing agent name", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{})
		if err == nil {
			t.Fatal("expected error for missing agent_name")
		}
	})
}

func TestPollTasks(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("empty when no tasks", func(t *testing.T) {
		resp, err := client.PollTasks(ctx, &banyanpb.PollTasksRequest{
			AgentName: "worker-1",
		})
		if err != nil {
			t.Fatalf("PollTasks failed: %v", err)
		}
		if len(resp.Tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
		}
	})

	t.Run("returns only pending tasks", func(t *testing.T) {
		// Create pending and completed tasks
		srv.store.Save(ctx, types.KeyTasks+"worker-2/task-1", &types.TaskRecord{
			ID: "task-1", AgentID: "worker-2", DeploymentID: "deploy-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
			Image: "nginx:alpine", ContainerName: "myapp-web-0",
			ServiceName: "web", ReplicaIndex: 0,
			Ports: []string{"80:80"}, Environment: []string{"FOO=bar"},
			Command: []string{"nginx"},
		})
		srv.store.Save(ctx, types.KeyTasks+"worker-2/task-2", &types.TaskRecord{
			ID: "task-2", AgentID: "worker-2", DeploymentID: "deploy-1",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			Image: "nginx:alpine", ContainerName: "myapp-web-1",
		})

		resp, err := client.PollTasks(ctx, &banyanpb.PollTasksRequest{
			AgentName: "worker-2",
		})
		if err != nil {
			t.Fatalf("PollTasks failed: %v", err)
		}
		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 pending task, got %d", len(resp.Tasks))
		}
		task := resp.Tasks[0]
		if task.Id != "task-1" {
			t.Errorf("expected task-1, got %s", task.Id)
		}
		if task.Image != "nginx:alpine" {
			t.Errorf("expected nginx:alpine, got %s", task.Image)
		}
		if task.ContainerName != "myapp-web-0" {
			t.Errorf("expected myapp-web-0, got %s", task.ContainerName)
		}
		if len(task.Ports) != 1 || task.Ports[0] != "80:80" {
			t.Errorf("expected ports [80:80], got %v", task.Ports)
		}
	})

	t.Run("missing agent name", func(t *testing.T) {
		_, err := client.PollTasks(ctx, &banyanpb.PollTasksRequest{})
		if err == nil {
			t.Fatal("expected error for missing agent name")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("returns volumes in tasks", func(t *testing.T) {
		srv.store.Save(ctx, types.KeyTasks+"worker-vol/task-vol-1", &types.TaskRecord{
			ID: "task-vol-1", AgentID: "worker-vol", DeploymentID: "deploy-vol",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
			Image: "postgres:15", ContainerName: "app-db-0",
			ServiceName: "db",
			Volumes: []types.VolumeMount{
				{Type: "bind", Source: "/host/data", Target: "/data", ReadOnly: false},
				{Type: "tmpfs", Target: "/tmp", Tmpfs: &types.TmpfsOpt{Size: "128m"}},
				{Type: "nfs", Source: "10.0.0.1:/exports", Target: "/nfs", ReadOnly: true},
			},
		})

		resp, err := client.PollTasks(ctx, &banyanpb.PollTasksRequest{AgentName: "worker-vol"})
		if err != nil {
			t.Fatalf("PollTasks failed: %v", err)
		}
		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		task := resp.Tasks[0]
		if len(task.Volumes) != 3 {
			t.Fatalf("expected 3 volumes, got %d", len(task.Volumes))
		}
		// bind mount
		if task.Volumes[0].Type != "bind" || task.Volumes[0].Source != "/host/data" {
			t.Errorf("vol[0]: got type=%q source=%q", task.Volumes[0].Type, task.Volumes[0].Source)
		}
		// tmpfs with size
		if task.Volumes[1].Type != "tmpfs" || task.Volumes[1].Target != "/tmp" {
			t.Errorf("vol[1]: got type=%q target=%q", task.Volumes[1].Type, task.Volumes[1].Target)
		}
		if task.Volumes[1].Tmpfs == nil || task.Volumes[1].Tmpfs.Size != "128m" {
			t.Errorf("vol[1]: expected tmpfs size '128m', got %+v", task.Volumes[1].Tmpfs)
		}
		// nfs volume
		if task.Volumes[2].Type != "nfs" || !task.Volumes[2].ReadOnly {
			t.Errorf("vol[2]: got type=%q ro=%v", task.Volumes[2].Type, task.Volumes[2].ReadOnly)
		}
	})
}

func TestReportTaskResult(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a task in the store
	task := &types.TaskRecord{
		ID:            "task-1",
		DeploymentID:  "deploy-1",
		AgentID:       "worker-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusPending,
		Image:         "nginx:alpine",
		ContainerName: "test-web-0",
		CreatedAt:     time.Now(),
	}
	srv.store.Save(ctx, types.KeyTasks+"worker-1/task-1", task)

	t.Run("marks task completed", func(t *testing.T) {
		_, err := client.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{
			TaskId:        "task-1",
			AgentId:       "worker-1",
			Status:        types.StatusCompleted,
			ContainerName: "test-web-0",
			Result:        &banyanpb.TaskResult{ContainerId: "abc123"},
		})
		if err != nil {
			t.Fatalf("ReportTaskResult failed: %v", err)
		}

		// Verify task was updated
		var updated types.TaskRecord
		if err := srv.store.Get(ctx, types.KeyTasks+"worker-1/task-1", &updated); err != nil {
			t.Fatalf("failed to get updated task: %v", err)
		}
		if updated.Status != types.StatusCompleted {
			t.Errorf("expected status completed, got %s", updated.Status)
		}
		if updated.Result == nil || updated.Result.ContainerID != "abc123" {
			t.Error("expected result with container_id abc123")
		}
	})

	t.Run("missing required fields", func(t *testing.T) {
		_, err := client.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{})
		if err == nil {
			t.Fatal("expected error for missing fields")
		}
	})
}

func TestReportContainerHealth(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a completed task
	task := &types.TaskRecord{
		ID:            "task-1",
		AgentID:       "worker-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusCompleted,
		ContainerName: "test-web-0",
	}
	srv.store.Save(ctx, types.KeyTasks+"worker-1/task-1", task)

	t.Run("updates container status", func(t *testing.T) {
		_, err := client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
			AgentName: "worker-1",
			Containers: []*banyanpb.ContainerStatus{
				{ContainerName: "test-web-0", Status: "running"},
			},
		})
		if err != nil {
			t.Fatalf("ReportContainerHealth failed: %v", err)
		}

		// Verify task was updated
		var updated types.TaskRecord
		if err := srv.store.Get(ctx, types.KeyTasks+"worker-1/task-1", &updated); err != nil {
			t.Fatalf("failed to get updated task: %v", err)
		}
		if updated.ContainerStatus != "running" {
			t.Errorf("expected container_status 'running', got %q", updated.ContainerStatus)
		}
	})
}

func TestDeploy(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("successful deploy", func(t *testing.T) {
		resp, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "my-app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {
						Image: "nginx:alpine",
						Ports: []string{"80:80"},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}
		if resp.DeploymentId == "" {
			t.Error("expected non-empty deployment_id")
		}
		if resp.Status != types.StatusPending {
			t.Errorf("expected status pending, got %s", resp.Status)
		}

		// Verify deployment was stored
		keys, _ := srv.store.List(ctx, types.KeyDeployments)
		if len(keys) != 1 {
			t.Fatalf("expected 1 deployment, got %d", len(keys))
		}
	})

	t.Run("missing name", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Services: map[string]*banyanpb.ManifestService{
					"web": {Image: "nginx"},
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("no services", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{Name: "empty-app"},
		})
		if err == nil {
			t.Fatal("expected error for empty services")
		}
	})

	t.Run("nil manifest", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{})
		if err == nil {
			t.Fatal("expected error for nil manifest")
		}
	})

	t.Run("service without image or build", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "bad-app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {}, // no image, no build
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for service without image or build")
		}
	})
}

func TestHeartbeat_NewNode(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Pre-register the agent (heartbeat requires prior registration per MED-009)
	srv.store.Save(ctx, types.KeyNodes+"new-agent", &types.NodeRecord{
		Name:   "new-agent",
		Status: "pending",
	})

	// Heartbeat should update the registered node
	_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "new-agent",
		SessionToken: "token-new",
	})
	if err != nil {
		t.Fatalf("Heartbeat for registered node failed: %v", err)
	}

	// Verify node was updated to ready
	var node types.NodeRecord
	if getErr := srv.store.Get(ctx, types.KeyNodes+"new-agent", &node); getErr != nil {
		t.Fatalf("expected node to exist: %v", getErr)
	}
	if node.Status != "ready" {
		t.Errorf("expected status ready, got %s", node.Status)
	}
}

func TestHeartbeat_UnregisteredAgent(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Heartbeat for a node that hasn't registered — should be rejected
	_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "unknown-agent",
		SessionToken: "token-new",
	})
	if err == nil {
		t.Fatal("expected error for unregistered agent")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", status.Code(err))
	}
}

func TestReportContainerHealth_MissingAgent(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.ReportContainerHealth(context.Background(), &banyanpb.ReportContainerHealthRequest{})
	if err == nil {
		t.Fatal("expected error for missing agent name")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestReportContainerHealth_NonMatchingTask(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Create a task that's still pending (not completed), should be skipped
	srv.store.Save(ctx, types.KeyTasks+"worker-3/task-p", &types.TaskRecord{
		ID: "task-p", AgentID: "worker-3", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusPending, ContainerName: "pending-container",
	})
	// Create a stop task (not create_and_start), should be skipped
	srv.store.Save(ctx, types.KeyTasks+"worker-3/task-s", &types.TaskRecord{
		ID: "task-s", AgentID: "worker-3", Type: types.TaskTypeStopAndRemove,
		Status: types.StatusCompleted, ContainerName: "stop-container",
	})

	_, err := client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "worker-3",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "pending-container", Status: "running"},
			{ContainerName: "stop-container", Status: "running"},
		},
	})
	if err != nil {
		t.Fatalf("ReportContainerHealth failed: %v", err)
	}
}

func TestReportTaskResult_NotFound(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := client.ReportTaskResult(context.Background(), &banyanpb.ReportTaskResultRequest{
		TaskId:  "nonexistent",
		AgentId: "worker-1",
		Status:  types.StatusCompleted,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", status.Code(err))
	}
}

func TestDown(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a deployment and a completed task
	deployment := &types.DeploymentRecord{
		ID:     "deploy-1",
		Name:   "my-app",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		},
		CreatedAt: time.Now(),
	}
	srv.store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	// Register a node (needed for CollectDeploymentTasks)
	node := &types.NodeRecord{Name: "worker-1", Status: "ready"}
	srv.store.Save(ctx, types.KeyNodes+"worker-1", node)

	task := &types.TaskRecord{
		ID:            "deploy-1-web-0",
		DeploymentID:  "deploy-1",
		ServiceName:   "web",
		AgentID:       "worker-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusCompleted,
		ContainerName: "my-app-web-0",
	}
	srv.store.Save(ctx, types.KeyTasks+"worker-1/deploy-1-web-0", task)

	t.Run("creates stop tasks", func(t *testing.T) {
		resp, err := client.Down(ctx, &banyanpb.DownRPCRequest{Name: "my-app"})
		if err != nil {
			t.Fatalf("Down failed: %v", err)
		}
		if resp.TaskCount != 1 {
			t.Errorf("expected 1 stop task, got %d", resp.TaskCount)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		_, err := client.Down(ctx, &banyanpb.DownRPCRequest{})
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := client.Down(ctx, &banyanpb.DownRPCRequest{Name: "nonexistent"})
		if err == nil {
			t.Fatal("expected error for nonexistent deployment")
		}
	})

	t.Run("with service filter", func(t *testing.T) {
		client2, srv2, cleanup2 := setupTestServer(t)
		defer cleanup2()

		deployment2 := &types.DeploymentRecord{
			ID:     "deploy-svc",
			Name:   "filtered-app",
			Status: types.StatusRunning,
			Services: map[string]types.ServiceRecord{
				"web": {Image: "nginx", Replicas: 1},
				"api": {Image: "node", Replicas: 1},
			},
			CreatedAt: time.Now(),
		}
		srv2.store.Save(ctx, types.KeyDeployments+"deploy-svc", deployment2)
		srv2.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		srv2.store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "deploy-svc", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "filtered-app-web-0",
		})
		srv2.store.Save(ctx, types.KeyTasks+"agent-1/task-api", &types.TaskRecord{
			ID: "task-api", DeploymentID: "deploy-svc", ServiceName: "api",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "filtered-app-api-0",
		})

		// Only stop the web service
		resp, err := client2.Down(ctx, &banyanpb.DownRPCRequest{
			Name:     "filtered-app",
			Services: []string{"web"},
		})
		if err != nil {
			t.Fatalf("Down failed: %v", err)
		}
		if resp.TaskCount != 1 {
			t.Errorf("expected 1 stop task (only web), got %d", resp.TaskCount)
		}
	})

	t.Run("invalid service name", func(t *testing.T) {
		client3, srv3, cleanup3 := setupTestServer(t)
		defer cleanup3()

		srv3.store.Save(ctx, types.KeyDeployments+"deploy-inv", &types.DeploymentRecord{
			ID: "deploy-inv", Name: "inv-app", Status: types.StatusRunning,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
			CreatedAt: time.Now(),
		})

		_, err := client3.Down(ctx, &banyanpb.DownRPCRequest{
			Name:     "inv-app",
			Services: []string{"nonexistent-svc"},
		})
		if err == nil {
			t.Fatal("expected error for nonexistent service")
		}
	})

	t.Run("no completed tasks returns zero", func(t *testing.T) {
		client4, srv4, cleanup4 := setupTestServer(t)
		defer cleanup4()

		srv4.store.Save(ctx, types.KeyDeployments+"deploy-norun", &types.DeploymentRecord{
			ID: "deploy-norun", Name: "norun-app", Status: types.StatusPending,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
			CreatedAt: time.Now(),
		})

		resp, err := client4.Down(ctx, &banyanpb.DownRPCRequest{Name: "norun-app"})
		if err != nil {
			t.Fatalf("Down failed: %v", err)
		}
		if resp.TaskCount != 0 {
			t.Errorf("expected 0 stop tasks, got %d", resp.TaskCount)
		}
	})

	t.Run("clears stale deployment error", func(t *testing.T) {
		client5, srv5, cleanup5 := setupTestServer(t)
		defer cleanup5()

		// Deployment failed during deploy (1 task failed) but has completed tasks
		srv5.store.Save(ctx, types.KeyDeployments+"deploy-err", &types.DeploymentRecord{
			ID: "deploy-err", Name: "err-app", Status: types.StatusFailed,
			Error:     "1/5 tasks failed: failed to start container: name-store error",
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
			CreatedAt: time.Now(),
		})
		srv5.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		srv5.store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "deploy-err", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "err-app-web-0",
		})

		resp, err := client5.Down(ctx, &banyanpb.DownRPCRequest{Name: "err-app"})
		if err != nil {
			t.Fatalf("Down failed: %v", err)
		}
		if resp.TaskCount != 1 {
			t.Errorf("expected 1 stop task, got %d", resp.TaskCount)
		}

		// Verify deployment error was cleared when status set to stopping
		var updated types.DeploymentRecord
		srv5.store.Get(ctx, types.KeyDeployments+"deploy-err", &updated)
		if updated.Error != "" {
			t.Errorf("expected deployment error to be cleared, got %q", updated.Error)
		}
		if updated.Status != types.StatusStopping {
			t.Errorf("expected status stopping, got %s", updated.Status)
		}
	})
}

func TestGetStatus(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create agent and deployment
	node := &types.NodeRecord{Name: "worker-1", Status: "ready", LastSeen: time.Now(), CreatedAt: time.Now()}
	srv.store.Save(ctx, types.KeyNodes+"worker-1", node)

	deployment := &types.DeploymentRecord{
		ID:        "deploy-1",
		Name:      "my-app",
		Status:    types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(),
	}
	srv.store.Save(ctx, types.KeyDeployments+"deploy-1", deployment)

	task := &types.TaskRecord{
		ID:              "deploy-1-web-0",
		DeploymentID:    "deploy-1",
		ServiceName:     "web",
		AgentID:         "worker-1",
		Type:            types.TaskTypeCreateAndStart,
		Status:          types.StatusCompleted,
		ContainerName:   "my-app-web-0",
		ContainerStatus: types.StatusRunning,
	}
	srv.store.Save(ctx, types.KeyTasks+"worker-1/deploy-1-web-0", task)

	t.Run("returns agents and deployments", func(t *testing.T) {
		resp, err := client.GetStatus(ctx, &banyanpb.GetStatusRequest{})
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if len(resp.Agents) != 1 {
			t.Errorf("expected 1 agent, got %d", len(resp.Agents))
		}
		if resp.Agents[0].Name != "worker-1" {
			t.Errorf("expected agent name worker-1, got %s", resp.Agents[0].Name)
		}
		if len(resp.Deployments) != 1 {
			t.Errorf("expected 1 deployment, got %d", len(resp.Deployments))
		}
		if resp.Deployments[0].Healthy != 1 {
			t.Errorf("expected 1 healthy, got %d", resp.Deployments[0].Healthy)
		}
	})
}

func TestGetInfo(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("returns registry URL", func(t *testing.T) {
		resp, err := client.GetInfo(ctx, &banyanpb.GetInfoRequest{})
		if err != nil {
			t.Fatalf("GetInfo failed: %v", err)
		}
		if resp.RegistryUrl != "localhost:5000" {
			t.Errorf("expected registry_url 'localhost:5000', got %q", resp.RegistryUrl)
		}
	})
}

func TestHealth(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("returns ok", func(t *testing.T) {
		resp, err := client.Health(ctx, &banyanpb.HealthRequest{})
		if err != nil {
			t.Fatalf("Health failed: %v", err)
		}
		if resp.Status != "ok" {
			t.Errorf("expected status 'ok', got %q", resp.Status)
		}
	})
}

func TestProtoToManifest(t *testing.T) {
	t.Run("basic manifest", func(t *testing.T) {
		proto := &banyanpb.Manifest{
			Name:    "my-app",
			Version: "1.0",
			Services: map[string]*banyanpb.ManifestService{
				"web": {
					Image:       "nginx:alpine",
					Ports:       []string{"80:80"},
					Environment: []string{"FOO=bar"},
					Command:     []string{"nginx"},
					DependsOn:   map[string]*banyanpb.DependsOnCondition{"db": {Condition: "service_started"}},
				},
			},
		}
		result := protoToManifest(proto)
		if result.Name != "my-app" {
			t.Errorf("expected name 'my-app', got %q", result.Name)
		}
		if result.Version != "1.0" {
			t.Errorf("expected version '1.0', got %q", result.Version)
		}
		svc := result.Services["web"]
		if svc.Image != "nginx:alpine" {
			t.Errorf("expected image 'nginx:alpine', got %q", svc.Image)
		}
		if len(svc.Ports) != 1 || svc.Ports[0] != "80:80" {
			t.Errorf("unexpected ports: %v", svc.Ports)
		}
		if len(svc.DependsOn) != 1 {
			t.Errorf("expected 1 depends_on entry, got %d", len(svc.DependsOn))
		}
		if _, ok := svc.DependsOn["db"]; !ok {
			t.Errorf("expected depends_on to contain 'db', got %v", svc.DependsOn)
		}
	})

	t.Run("with build config", func(t *testing.T) {
		proto := &banyanpb.Manifest{
			Name: "my-app",
			Services: map[string]*banyanpb.ManifestService{
				"api": {
					Image: "my-api:latest",
					Build: &banyanpb.ManifestBuild{
						Context:    "./api",
						Dockerfile: "Dockerfile.prod",
					},
				},
			},
		}
		result := protoToManifest(proto)
		svc := result.Services["api"]
		if svc.Build == nil {
			t.Fatal("expected non-nil Build")
		}
		if svc.Build.Context != "./api" {
			t.Errorf("expected context './api', got %q", svc.Build.Context)
		}
		if svc.Build.Dockerfile != "Dockerfile.prod" {
			t.Errorf("expected dockerfile 'Dockerfile.prod', got %q", svc.Build.Dockerfile)
		}
	})

	t.Run("with deploy config", func(t *testing.T) {
		proto := &banyanpb.Manifest{
			Name: "my-app",
			Services: map[string]*banyanpb.ManifestService{
				"web": {
					Image:  "nginx",
					Deploy: &banyanpb.ManifestDeploy{Replicas: 3},
				},
			},
		}
		result := protoToManifest(proto)
		svc := result.Services["web"]
		if svc.Deploy == nil {
			t.Fatal("expected non-nil Deploy")
		}
		if svc.Deploy.Replicas != 3 {
			t.Errorf("expected 3 replicas, got %d", svc.Deploy.Replicas)
		}
	})

	t.Run("with volumes", func(t *testing.T) {
		proto := &banyanpb.Manifest{
			Name: "my-app",
			Services: map[string]*banyanpb.ManifestService{
				"db": {
					Image: "postgres:15",
					Volumes: []*banyanpb.VolumeMount{
						{
							Type:     "bind",
							Source:   "/host/data",
							Target:   "/var/lib/postgresql/data",
							ReadOnly: false,
						},
						{
							Type:     "volume",
							Source:   "cache",
							Target:   "/cache",
							ReadOnly: true,
						},
						{
							Type:   "tmpfs",
							Target: "/tmp",
							Tmpfs:  &banyanpb.TmpfsOpt{Size: "256m"},
						},
					},
				},
			},
			Volumes: map[string]*banyanpb.VolumeConfig{
				"cache": {
					Driver:     "local",
					DriverOpts: map[string]string{"type": "nfs"},
					External:   false,
					Name:       "my-cache",
				},
			},
		}
		result := protoToManifest(proto)
		svc := result.Services["db"]
		if len(svc.Volumes) != 3 {
			t.Fatalf("expected 3 volumes, got %d", len(svc.Volumes))
		}
		// bind mount
		if svc.Volumes[0].Type != "bind" || svc.Volumes[0].Source != "/host/data" {
			t.Errorf("vol[0]: expected bind /host/data, got %+v", svc.Volumes[0])
		}
		if svc.Volumes[0].ReadOnly {
			t.Error("vol[0] should not be read-only")
		}
		// named volume
		if svc.Volumes[1].Type != "volume" || !svc.Volumes[1].ReadOnly {
			t.Errorf("vol[1]: expected read-only volume, got %+v", svc.Volumes[1])
		}
		// tmpfs
		if svc.Volumes[2].Type != "tmpfs" || svc.Volumes[2].Target != "/tmp" {
			t.Errorf("vol[2]: expected tmpfs /tmp, got %+v", svc.Volumes[2])
		}
		if svc.Volumes[2].Tmpfs == nil || svc.Volumes[2].Tmpfs.Size != "256m" {
			t.Errorf("vol[2]: expected tmpfs size '256m', got %+v", svc.Volumes[2].Tmpfs)
		}
		// top-level volumes
		if len(result.Volumes) != 1 {
			t.Fatalf("expected 1 top-level volume, got %d", len(result.Volumes))
		}
		vc := result.Volumes["cache"]
		if vc.Driver != "local" {
			t.Errorf("expected driver 'local', got %q", vc.Driver)
		}
		if vc.Name != "my-cache" {
			t.Errorf("expected name 'my-cache', got %q", vc.Name)
		}
		if vc.DriverOpts["type"] != "nfs" {
			t.Errorf("expected driver_opts type 'nfs', got %q", vc.DriverOpts["type"])
		}
	})
}

func TestFindDeploymentByName(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID: "deploy-1", Name: "my-app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})

		deployment, key, err := srv.findDeploymentByName(ctx, "my-app", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deployment.Name != "my-app" {
			t.Errorf("expected name 'my-app', got %q", deployment.Name)
		}
		if key != types.KeyDeployments+"deploy-1" {
			t.Errorf("expected key %q, got %q", types.KeyDeployments+"deploy-1", key)
		}
	})

	t.Run("not found", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		_, _, err := srv.findDeploymentByName(ctx, "nonexistent", nil)
		if err == nil {
			t.Fatal("expected error for nonexistent deployment")
		}
	})

	t.Run("multiple versions returns most recent", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		older := time.Now().Add(-time.Hour)
		newer := time.Now()
		store.Save(ctx, types.KeyDeployments+"deploy-old", &types.DeploymentRecord{
			ID: "deploy-old", Name: "my-app", CreatedAt: older,
		})
		store.Save(ctx, types.KeyDeployments+"deploy-new", &types.DeploymentRecord{
			ID: "deploy-new", Name: "my-app", CreatedAt: newer,
		})

		deployment, _, err := srv.findDeploymentByName(ctx, "my-app", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deployment.ID != "deploy-new" {
			t.Errorf("expected most recent deployment 'deploy-new', got %q", deployment.ID)
		}
	})
}

func TestFindContainerAgent(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready", APIAddress: "agent-1:50052"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", AgentID: "agent-1", ContainerName: "myapp-web-0",
			Type: types.TaskTypeCreateAndStart,
		})

		task, node, err := srv.findContainerAgent(ctx, "myapp-web-0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ContainerName != "myapp-web-0" {
			t.Errorf("expected container name 'myapp-web-0', got %q", task.ContainerName)
		}
		if node.Name != "agent-1" {
			t.Errorf("expected node 'agent-1', got %q", node.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

		_, _, err := srv.findContainerAgent(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent container")
		}
	})
}

func TestGetLogs(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("empty container name", func(t *testing.T) {
		stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{ContainerName: ""})
		if err != nil {
			t.Fatalf("GetLogs call failed: %v", err)
		}
		_, recvErr := stream.Recv()
		if recvErr == nil {
			t.Fatal("expected error for empty container name")
		}
		if status.Code(recvErr) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", status.Code(recvErr))
		}
	})

	t.Run("container not found", func(t *testing.T) {
		// No nodes or tasks in store
		stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{ContainerName: "nonexistent"})
		if err != nil {
			t.Fatalf("GetLogs call failed: %v", err)
		}
		_, recvErr := stream.Recv()
		if recvErr == nil {
			t.Fatal("expected error for nonexistent container")
		}
		if status.Code(recvErr) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", status.Code(recvErr))
		}
	})

	t.Run("agent API address empty", func(t *testing.T) {
		// Create a node with empty API address and a task matching the container
		srv.store.Save(ctx, types.KeyNodes+"agent-no-api", &types.NodeRecord{
			Name: "agent-no-api", Status: "ready", APIAddress: "",
		})
		srv.store.Save(ctx, types.KeyTasks+"agent-no-api/task-logs", &types.TaskRecord{
			ID: "task-logs", AgentID: "agent-no-api", ContainerName: "myapp-web-0",
			Type: types.TaskTypeCreateAndStart,
		})

		stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{ContainerName: "myapp-web-0"})
		if err != nil {
			t.Fatalf("GetLogs call failed: %v", err)
		}
		_, recvErr := stream.Recv()
		if recvErr == nil {
			t.Fatal("expected error for empty API address")
		}
		if status.Code(recvErr) != codes.Unavailable {
			t.Errorf("expected Unavailable, got %v", status.Code(recvErr))
		}
	})
}

func TestGetLogs_SuccessProxy(t *testing.T) {
	// Start a mock agent gRPC server that streams log data
	logChunks := [][]byte{
		[]byte("line 1: hello from container\n"),
		[]byte("line 2: world\n"),
	}
	agentAddr, agentCleanup := startMockAgentServer(t, logChunks)
	defer agentCleanup()

	// Set up the engine test server with a stream interceptor
	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Seed the store with a node pointing to the mock agent and a matching task
	store.Save(ctx, types.KeyNodes+"agent-logs", &types.NodeRecord{
		Name: "agent-logs", Status: "ready", APIAddress: agentAddr,
	})
	store.Save(ctx, types.KeyTasks+"agent-logs/task-logs-1", &types.TaskRecord{
		ID: "task-logs-1", AgentID: "agent-logs", ContainerName: "test-container",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	// Create engine gRPC server with both unary and stream interceptors
	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(grpcSrv, engineSrv)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	t.Run("streams log data through proxy", func(t *testing.T) {
		stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{
			ContainerName: "test-container",
			Follow:        false,
			Tail:          100,
		})
		if err != nil {
			t.Fatalf("GetLogs call failed: %v", err)
		}

		var received []byte
		for {
			resp, recvErr := stream.Recv()
			if recvErr == io.EOF {
				break
			}
			if recvErr != nil {
				// The stream ends when the mock agent finishes sending
				break
			}
			received = append(received, resp.Data...)
		}

		expected := "line 1: hello from container\nline 2: world\n"
		if string(received) != expected {
			t.Errorf("expected log data %q, got %q", expected, string(received))
		}
	})
}

func TestGetLogs_AgentConnectionFailure(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Point to an address that nothing is listening on
	store.Save(ctx, types.KeyNodes+"agent-dead", &types.NodeRecord{
		Name: "agent-dead", Status: "ready", APIAddress: "127.0.0.1:1",
	})
	store.Save(ctx, types.KeyTasks+"agent-dead/task-dead", &types.TaskRecord{
		ID: "task-dead", AgentID: "agent-dead", ContainerName: "dead-container",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(grpcSrv, engineSrv)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{
		ContainerName: "dead-container",
	})
	if err != nil {
		t.Fatalf("GetLogs call failed: %v", err)
	}
	_, recvErr := stream.Recv()
	if recvErr == nil {
		t.Fatal("expected error when agent is unreachable")
	}
	if status.Code(recvErr) != codes.Unavailable {
		t.Errorf("expected Unavailable, got %v", status.Code(recvErr))
	}
}

func TestFindContainerAgent_MultipleAgents(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	// Two agents, each with different containers
	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready", APIAddress: "agent-1:50052"})
	store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{Name: "agent-2", Status: "ready", APIAddress: "agent-2:50052"})

	store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", ContainerName: "app-web-0",
		Type: types.TaskTypeCreateAndStart,
	})
	store.Save(ctx, types.KeyTasks+"agent-2/task-2", &types.TaskRecord{
		ID: "task-2", AgentID: "agent-2", ContainerName: "app-api-0",
		Type: types.TaskTypeCreateAndStart,
	})

	t.Run("finds container on second agent", func(t *testing.T) {
		task, node, err := srv.findContainerAgent(ctx, "app-api-0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ContainerName != "app-api-0" {
			t.Errorf("expected container name 'app-api-0', got %q", task.ContainerName)
		}
		if node.Name != "agent-2" {
			t.Errorf("expected node 'agent-2', got %q", node.Name)
		}
	})

	t.Run("finds container on first agent", func(t *testing.T) {
		task, node, err := srv.findContainerAgent(ctx, "app-web-0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ContainerName != "app-web-0" {
			t.Errorf("expected container name 'app-web-0', got %q", task.ContainerName)
		}
		if node.Name != "agent-1" {
			t.Errorf("expected node 'agent-1', got %q", node.Name)
		}
	})
}

func TestFindContainerAgent_SkipsNonCreateTasks(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	// A stop task with the same container name should be skipped
	store.Save(ctx, types.KeyTasks+"agent-1/task-stop", &types.TaskRecord{
		ID: "task-stop", AgentID: "agent-1", ContainerName: "myapp-web-0",
		Type: types.TaskTypeStopAndRemove,
	})

	_, _, err := srv.findContainerAgent(ctx, "myapp-web-0")
	if err == nil {
		t.Fatal("expected error because only stop tasks exist for this container")
	}
}

func TestFindContainerAgent_EmptyContainerName(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	// Task with empty container name
	store.Save(ctx, types.KeyTasks+"agent-1/task-empty", &types.TaskRecord{
		ID: "task-empty", AgentID: "agent-1", ContainerName: "",
		Type: types.TaskTypeCreateAndStart,
	})

	// Searching for a non-empty container should not match the empty one
	_, _, err := srv.findContainerAgent(ctx, "some-container")
	if err == nil {
		t.Fatal("expected error, task has empty container name")
	}
}

func TestFindContainerAgent_NoNodes(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findContainerAgent(ctx, "some-container")
	if err == nil {
		t.Fatal("expected error when no nodes exist")
	}
}

func TestGetStatus_EmptyStore(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	resp, err := client.GetStatus(context.Background(), &banyanpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(resp.Agents))
	}
	if len(resp.Deployments) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(resp.Deployments))
	}
}

func TestGetStatus_MultipleDeploymentsAndAgents(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Multiple agents
	srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready", APIAddress: "agent-1:50052",
		LastSeen: time.Now(), CreatedAt: time.Now(),
	})
	srv.store.Save(ctx, types.KeyNodes+"agent-2", &types.NodeRecord{
		Name: "agent-2", Status: "ready", APIAddress: "agent-2:50052",
		LastSeen: time.Now(), CreatedAt: time.Now(),
	})

	// Deployment with multiple services and tasks
	srv.store.Save(ctx, types.KeyDeployments+"deploy-multi", &types.DeploymentRecord{
		ID:     "deploy-multi",
		Name:   "multi-app",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1, Ports: []string{"80:80"}, Environment: []string{"ENV=prod"}, Command: []string{"nginx"}, DependsOn: types.DependsOnConfig{"db": {Condition: "service_started"}}},
			"db":  {Image: "postgres", Replicas: 1},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	// Task on agent-1: running container
	srv.store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
		ID: "task-web", DeploymentID: "deploy-multi", ServiceName: "web",
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, Image: "nginx", ContainerName: "multi-app-web-0",
		ContainerStatus: types.StatusRunning, ContainerCheckedAt: time.Now(),
		Ports: []string{"80:80"}, Environment: []string{"ENV=prod"}, Command: []string{"nginx"},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	// Task on agent-2: not yet running
	srv.store.Save(ctx, types.KeyTasks+"agent-2/task-db", &types.TaskRecord{
		ID: "task-db", DeploymentID: "deploy-multi", ServiceName: "db",
		AgentID: "agent-2", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, Image: "postgres", ContainerName: "multi-app-db-0",
		ContainerStatus: "stopped",
		CreatedAt:       time.Now(), UpdatedAt: time.Now(),
	})

	// A second deployment (stopped)
	srv.store.Save(ctx, types.KeyDeployments+"deploy-stopped", &types.DeploymentRecord{
		ID: "deploy-stopped", Name: "old-app", Status: types.StatusStopped,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Error: "previous deployment failed",
	})

	resp, err := client.GetStatus(ctx, &banyanpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if len(resp.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(resp.Agents))
	}
	if len(resp.Deployments) != 2 {
		t.Errorf("expected 2 deployments, got %d", len(resp.Deployments))
	}

	// Find the multi-app deployment and validate
	for _, d := range resp.Deployments {
		if d.Name == "multi-app" {
			if d.Healthy != 1 {
				t.Errorf("expected 1 healthy container, got %d", d.Healthy)
			}
			if d.Total != 2 {
				t.Errorf("expected 2 total create tasks, got %d", d.Total)
			}
			if len(d.Tasks) != 2 {
				t.Errorf("expected 2 tasks in response, got %d", len(d.Tasks))
			}
			if len(d.Services) != 2 {
				t.Errorf("expected 2 services, got %d", len(d.Services))
			}
			webSvc, ok := d.Services["web"]
			if !ok {
				t.Error("expected web service in response")
			} else {
				if webSvc.Image != "nginx" {
					t.Errorf("expected image 'nginx', got %q", webSvc.Image)
				}
				if len(webSvc.DependsOn) != 1 {
					t.Errorf("expected 1 depends_on entry, got %d", len(webSvc.DependsOn))
				}
				if _, ok := webSvc.DependsOn["db"]; !ok {
					t.Errorf("expected depends_on to contain 'db', got %v", webSvc.DependsOn)
				}
			}
		}
		if d.Name == "old-app" {
			if d.Error != "previous deployment failed" {
				t.Errorf("expected error message, got %q", d.Error)
			}
		}
	}
}

func TestReportTaskResult_NilResult(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	srv.store.Save(ctx, types.KeyTasks+"worker-1/task-noresult", &types.TaskRecord{
		ID: "task-noresult", AgentID: "worker-1", DeploymentID: "deploy-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		ContainerName: "test-web-1",
	})

	// Report result without the Result field
	_, err := client.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{
		TaskId:        "task-noresult",
		AgentId:       "worker-1",
		Status:        types.StatusFailed,
		Error:         "pull timeout",
		ContainerName: "test-web-1",
	})
	if err != nil {
		t.Fatalf("ReportTaskResult failed: %v", err)
	}

	var updated types.TaskRecord
	if err := srv.store.Get(ctx, types.KeyTasks+"worker-1/task-noresult", &updated); err != nil {
		t.Fatalf("failed to get updated task: %v", err)
	}
	if updated.Status != types.StatusFailed {
		t.Errorf("expected status failed, got %s", updated.Status)
	}
	if updated.Error != "pull timeout" {
		t.Errorf("expected error 'pull timeout', got %q", updated.Error)
	}
	if updated.Result != nil {
		t.Error("expected nil result when not provided")
	}
}

func TestDeploy_WithBuildAndDeploy(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: &banyanpb.Manifest{
			Name:    "build-app",
			Version: "2.0",
			Services: map[string]*banyanpb.ManifestService{
				"api": {
					Image: "my-api:latest",
					Build: &banyanpb.ManifestBuild{
						Context:    "./api",
						Dockerfile: "Dockerfile",
					},
					Deploy: &banyanpb.ManifestDeploy{Replicas: 3},
					Ports:  []string{"8080:8080"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if resp.DeploymentId == "" {
		t.Error("expected non-empty deployment_id")
	}

	// Verify the deployment was stored with correct service records
	keys, _ := srv.store.List(ctx, types.KeyDeployments)
	found := false
	for _, key := range keys {
		var record types.DeploymentRecord
		if err := srv.store.Get(ctx, key, &record); err != nil {
			continue
		}
		if record.Name == "build-app" {
			found = true
			if len(record.Services) != 1 {
				t.Errorf("expected 1 service, got %d", len(record.Services))
			}
		}
	}
	if !found {
		t.Error("deployment 'build-app' not found in store")
	}
}

func TestDown_ServiceFilterNoMatchingTasks(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Deployment with two services but tasks only for "web"
	srv.store.Save(ctx, types.KeyDeployments+"deploy-partial", &types.DeploymentRecord{
		ID: "deploy-partial", Name: "partial-app", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
			"api": {Image: "node", Replicas: 1},
		},
		CreatedAt: time.Now(),
	})
	srv.store.Save(ctx, types.KeyNodes+"agent-p", &types.NodeRecord{Name: "agent-p", Status: "ready"})
	srv.store.Save(ctx, types.KeyTasks+"agent-p/task-web-p", &types.TaskRecord{
		ID: "task-web-p", DeploymentID: "deploy-partial", ServiceName: "web",
		AgentID: "agent-p", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "partial-app-web-0",
	})

	// Filter to stop "api" service only, but no completed tasks for "api"
	resp, err := client.Down(ctx, &banyanpb.DownRPCRequest{
		Name:     "partial-app",
		Services: []string{"api"},
	})
	if err != nil {
		t.Fatalf("Down failed: %v", err)
	}
	if resp.TaskCount != 0 {
		t.Errorf("expected 0 stop tasks (no api tasks), got %d", resp.TaskCount)
	}
}

func TestGRPCLogReader_BufferedRead(t *testing.T) {
	// Test the grpcLogReader with a large chunk that exceeds the read buffer,
	// which exercises the buffered data path in Read().
	largeChunk := make([]byte, 8192)
	for i := range largeChunk {
		largeChunk[i] = byte('A' + (i % 26))
	}
	logChunks := [][]byte{largeChunk}

	agentAddr, agentCleanup := startMockAgentServer(t, logChunks)
	defer agentCleanup()

	store := storage.NewMemoryStore()
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-buf", &types.NodeRecord{
		Name: "agent-buf", Status: "ready", APIAddress: agentAddr,
	})
	store.Save(ctx, types.KeyTasks+"agent-buf/task-buf", &types.TaskRecord{
		ID: "task-buf", AgentID: "agent-buf", ContainerName: "buf-container",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(grpcSrv, engineSrv)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{
		ContainerName: "buf-container",
	})
	if err != nil {
		t.Fatalf("GetLogs call failed: %v", err)
	}

	// The engine's GetLogs reads in 4096 byte chunks. When the agent sends 8192 bytes,
	// the engine will read it in multiple chunks via the proxy. Verify all data arrives.
	var received []byte
	for {
		resp, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			break
		}
		received = append(received, resp.Data...)
	}

	if len(received) != len(largeChunk) {
		t.Errorf("expected %d bytes, got %d", len(largeChunk), len(received))
	}
	if string(received) != string(largeChunk) {
		t.Error("received data does not match sent data")
	}
}

// --- Direct server method tests for error branches ---
// These test uncovered error paths by using errorStore directly.

// errorStore wraps MemoryStore and can inject errors on specific operations.
// Defined in engine_test.go, but we reference it here because both files are
// in the same package.

func TestRegister_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, saveErr: true}
	srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

	ctx := context.Background()
	_, err := srv.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "agent-1",
		SessionToken: "token-1",
		ApiAddress:   "agent-1:9090",
	})
	if err == nil {
		t.Fatal("expected error when store.Save fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestRegister_VPCConfig(t *testing.T) {
	ctx := context.Background()

	t.Run("returns allocated subnet when allocator is set", func(t *testing.T) {
		store := storage.NewMemoryStore()
		allocator, err := overlay.NewSubnetAllocator("10.0.0.0/16")
		if err != nil {
			t.Fatalf("failed to create allocator: %v", err)
		}
		peerTracker := overlay.NewPeerTracker()
		srv := &engineGRPCServer{
			store:       store,
			registryURL: "localhost:5000",
			allocator:   allocator,
			peerTracker: peerTracker,
			vpcCIDR:     "10.0.0.0/16",
		}

		resp, err := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			SessionToken: "token-1",
			ApiAddress:   "worker-1:9090",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		if resp.AllocatedSubnet == "" {
			t.Error("expected non-empty allocated_subnet")
		}
		if resp.VpcCidr != "10.0.0.0/16" {
			t.Errorf("expected vpc_cidr '10.0.0.0/16', got %q", resp.VpcCidr)
		}
	})

	t.Run("idempotent allocation for same agent", func(t *testing.T) {
		store := storage.NewMemoryStore()
		allocator, _ := overlay.NewSubnetAllocator("10.0.0.0/16")
		peerTracker := overlay.NewPeerTracker()
		srv := &engineGRPCServer{
			store:       store,
			registryURL: "localhost:5000",
			allocator:   allocator,
			peerTracker: peerTracker,
			vpcCIDR:     "10.0.0.0/16",
		}

		resp1, _ := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			SessionToken: "token-1",
			ApiAddress:   "worker-1:9090",
		})
		resp2, _ := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			SessionToken: "token-1",
			ApiAddress:   "worker-1:9090",
		})

		if resp1.AllocatedSubnet != resp2.AllocatedSubnet {
			t.Errorf("expected same subnet for same agent, got %q and %q", resp1.AllocatedSubnet, resp2.AllocatedSubnet)
		}
	})

	t.Run("no VPC config when allocator is nil", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{
			store:       store,
			registryURL: "localhost:5000",
		}

		resp, err := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-3",
			SessionToken: "token-3",
			ApiAddress:   "worker-3:9090",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		if resp.AllocatedSubnet != "" {
			t.Errorf("expected empty allocated_subnet, got %q", resp.AllocatedSubnet)
		}
		if resp.VpcCidr != "" {
			t.Errorf("expected empty vpc_cidr, got %q", resp.VpcCidr)
		}
	})
}

func TestHeartbeat_VPCPeers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns VPC peers from peer tracker", func(t *testing.T) {
		store := storage.NewMemoryStore()
		allocator, _ := overlay.NewSubnetAllocator("10.0.0.0/16")
		peerTracker := overlay.NewPeerTracker()
		srv := &engineGRPCServer{
			store:       store,
			registryURL: "localhost:5000",
			allocator:   allocator,
			peerTracker: peerTracker,
			vpcCIDR:     "10.0.0.0/16",
		}

		// Register two agents to populate allocator and peer tracker
		srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName: "worker-1", SessionToken: "token-1", ApiAddress: "w1:9090",
		})
		srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName: "worker-2", SessionToken: "token-2", ApiAddress: "w2:9090",
		})

		// Manually update peer tracker with known IPs since test gRPC
		// context doesn't have real peer addresses
		subnet1, _ := allocator.Allocate(ctx, "worker-1")
		subnet2, _ := allocator.Allocate(ctx, "worker-2")
		peerTracker.Update(ctx, "worker-1", overlay.Peer{
			Subnet: *subnet1,
			HostIP: net.ParseIP("192.168.1.10"),
			VTEPIP: overlay.VTEPIP(*subnet1),
		})
		peerTracker.Update(ctx, "worker-2", overlay.Peer{
			Subnet: *subnet2,
			HostIP: net.ParseIP("192.168.1.20"),
			VTEPIP: overlay.VTEPIP(*subnet2),
		})

		// Heartbeat from worker-1 should see worker-2 as a peer
		store.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{Name: "worker-1", Status: "ready"})
		resp, err := srv.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-1",
			SessionToken: "token-1",
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		if len(resp.VpcPeers) != 1 {
			t.Fatalf("expected 1 VPC peer, got %d", len(resp.VpcPeers))
		}
		if resp.VpcPeers[0].HostIp != "192.168.1.20" {
			t.Errorf("expected peer host IP '192.168.1.20', got %q", resp.VpcPeers[0].HostIp)
		}
	})

	t.Run("no peers when tracker is nil", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{
			store:       store,
			registryURL: "localhost:5000",
		}

		store.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{Name: "worker-1", Status: "ready"})
		resp, err := srv.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-1",
			SessionToken: "token-1",
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		if len(resp.VpcPeers) != 0 {
			t.Errorf("expected 0 VPC peers when tracker is nil, got %d", len(resp.VpcPeers))
		}
	})
}

func TestExtractPeerIP(t *testing.T) {
	t.Run("returns nil for context without peer", func(t *testing.T) {
		ip := extractPeerIP(context.Background())
		if ip != nil {
			t.Errorf("expected nil IP for context without peer, got %v", ip)
		}
	})
}

func TestAgentHostIP(t *testing.T) {
	ctx := context.Background() // no peer info

	t.Run("prefers reported IP", func(t *testing.T) {
		ip := agentHostIP("192.168.1.10", ctx)
		if ip == nil || ip.String() != "192.168.1.10" {
			t.Errorf("expected 192.168.1.10, got %v", ip)
		}
	})

	t.Run("falls back to extractPeerIP when reported is empty", func(t *testing.T) {
		ip := agentHostIP("", ctx)
		if ip != nil {
			t.Errorf("expected nil (no peer in context), got %v", ip)
		}
	})

	t.Run("falls back when reported is invalid", func(t *testing.T) {
		ip := agentHostIP("not-an-ip", ctx)
		if ip != nil {
			t.Errorf("expected nil for invalid IP, got %v", ip)
		}
	})
}

func TestHeartbeat_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	// Pre-register the agent so Get succeeds (MED-009 check passes)
	memStore.Save(context.Background(), types.KeyNodes+"agent-1", &types.NodeRecord{
		Name: "agent-1", Status: "ready",
	})
	store := &errorStore{MemoryStore: memStore, saveErr: true}
	srv := &engineGRPCServer{store: store}

	ctx := context.Background()
	_, err := srv.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "agent-1",
		SessionToken: "token-1",
	})
	if err == nil {
		t.Fatal("expected error when store.Save fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestPollTasks_ListError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, listErr: true}
	srv := &engineGRPCServer{store: store}

	ctx := context.Background()
	_, err := srv.PollTasks(ctx, &banyanpb.PollTasksRequest{
		AgentName: "agent-1",
	})
	if err == nil {
		t.Fatal("expected error when store.List fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestPollTasks_GetError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	memStore.Save(context.Background(), types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", Status: types.StatusPending,
		Type: types.TaskTypeCreateAndStart, Image: "nginx",
	})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	srv := &engineGRPCServer{store: store}

	ctx := context.Background()
	resp, err := srv.PollTasks(ctx, &banyanpb.PollTasksRequest{
		AgentName: "agent-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Get error should cause task to be skipped
	if len(resp.Tasks) != 0 {
		t.Errorf("expected 0 tasks when Get fails, got %d", len(resp.Tasks))
	}
}

func TestReportContainerHealth_ListError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, listErr: true}
	srv := &engineGRPCServer{store: store}

	ctx := context.Background()
	_, err := srv.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "agent-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "test-web-0", Status: "running"},
		},
	})
	if err == nil {
		t.Fatal("expected error when store.List fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestReportContainerHealth_GetError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	memStore.Save(context.Background(), types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "test-web-0",
	})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	srv := &engineGRPCServer{store: store}

	ctx := context.Background()
	resp, err := srv.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "agent-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "test-web-0", Status: "running"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Get error should skip the task, still return OK
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestReportContainerHealth_ContainerNotInMap(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()

	// Task exists and is completed, but the reported containers don't match its name
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "test-web-0",
	})

	srv := &engineGRPCServer{store: memStore}

	resp, err := srv.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "agent-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "other-container", Status: "running"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}

	// Verify the task's container status was NOT updated
	var task types.TaskRecord
	memStore.Get(ctx, types.KeyTasks+"agent-1/task-1", &task)
	if task.ContainerStatus != "" {
		t.Errorf("expected empty container status, got %q", task.ContainerStatus)
	}
}

func TestReportContainerHealth_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()

	// Seed a completed create_and_start task
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "test-web-0",
	})

	// Use a store that only fails on Save (List and Get work normally)
	store := &saveOnlyErrorStore{MemoryStore: memStore}
	srv := &engineGRPCServer{store: store}

	// Save failure is only logged, not returned as error
	resp, err := srv.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "agent-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "test-web-0", Status: "running"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestDeploy_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, saveErr: true}
	srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

	ctx := context.Background()
	_, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: &banyanpb.Manifest{
			Name: "app",
			Services: map[string]*banyanpb.ManifestService{
				"web": {Image: "nginx"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error when store.Save fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestReportTaskResult_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", DeploymentID: "deploy-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
	})

	store := &errorStore{MemoryStore: memStore, saveErr: true}
	srv := &engineGRPCServer{store: store}

	_, err := srv.ReportTaskResult(ctx, &banyanpb.ReportTaskResultRequest{
		TaskId:  "task-1",
		AgentId: "agent-1",
		Status:  types.StatusCompleted,
	})
	if err == nil {
		t.Fatal("expected error when store.Save fails")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestDown_SaveStopTaskError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()

	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(),
	})
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", ServiceName: "web",
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "app-web-0",
	})

	store := &errorStore{MemoryStore: memStore, saveErr: true}
	srv := &engineGRPCServer{store: store}

	_, err := srv.Down(ctx, &banyanpb.DownRPCRequest{Name: "app"})
	if err == nil {
		t.Fatal("expected error when store.Save fails for stop task")
	}
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", status.Code(err))
	}
}

func TestDown_DeploymentStatusSaveWarning(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()

	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
		CreatedAt: time.Now(),
	})
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", ServiceName: "web",
		AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerName: "app-web-0",
	})

	// First Save (stop task) succeeds, second Save (deployment status) fails
	store := &countingSaveErrorStore{MemoryStore: memStore, failAfterN: 1}
	srv := &engineGRPCServer{store: store}

	// Down with no service filter (stopping all) triggers deployment status update
	resp, err := srv.Down(ctx, &banyanpb.DownRPCRequest{Name: "app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stop task was created successfully
	if resp.TaskCount != 1 {
		t.Errorf("expected 1 stop task, got %d", resp.TaskCount)
	}
}

func TestFindDeploymentByName_ListError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, listErr: true}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findDeploymentByName(context.Background(), "app", nil)
	if err == nil {
		t.Fatal("expected error when store.List fails")
	}
}

func TestFindContainerAgent_ListNodesError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	store := &errorStore{MemoryStore: memStore, listErr: true}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findContainerAgent(context.Background(), "container")
	if err == nil {
		t.Fatal("expected error when store.List fails")
	}
}

func TestFindContainerAgent_GetNodeError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findContainerAgent(ctx, "container")
	if err == nil {
		t.Fatal("expected error when no container found (nodes skipped due to Get error)")
	}
}

func TestGetStatus_GetNodeError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
		Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
		CreatedAt: time.Now(),
	})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	srv := &engineGRPCServer{store: store}

	resp, err := srv.GetStatus(ctx, &banyanpb.GetStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Nodes and deployments should be skipped due to Get errors
	if len(resp.Agents) != 0 {
		t.Errorf("expected 0 agents (skipped due to Get error), got %d", len(resp.Agents))
	}
	if len(resp.Deployments) != 0 {
		t.Errorf("expected 0 deployments (skipped due to Get error), got %d", len(resp.Deployments))
	}
}

func TestFindDeploymentByName_GetError(t *testing.T) {
	memStore := storage.NewMemoryStore()
	ctx := context.Background()
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
		CreatedAt: time.Now(),
	})

	store := &errorStore{MemoryStore: memStore, getErr: true}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findDeploymentByName(ctx, "app", nil)
	if err == nil {
		t.Fatal("expected error when all deployments fail to deserialize")
	}
}

func TestFindContainerAgent_ListTasksError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})

	// List succeeds for nodes (first call) but fails for tasks (second call)
	store := &countingListErrorStore{MemoryStore: memStore, failAfterN: 1}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findContainerAgent(ctx, "some-container")
	if err == nil {
		t.Fatal("expected error when container not found (task list failed)")
	}
}

func TestFindContainerAgent_GetTaskError(t *testing.T) {
	ctx := context.Background()
	memStore := storage.NewMemoryStore()
	memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", ContainerName: "target-container",
		Type: types.TaskTypeCreateAndStart,
	})

	// Get succeeds for node (first call) but fails for task (second call)
	store := &countingGetErrorStore{MemoryStore: memStore, failAfterN: 1}
	srv := &engineGRPCServer{store: store}

	_, _, err := srv.findContainerAgent(ctx, "target-container")
	if err == nil {
		t.Fatal("expected error when task Get fails")
	}
}

func TestStreamAgentLogs_StreamLogsError(t *testing.T) {
	// Start a mock agent server and immediately stop it so StreamLogs fails.
	agentAddr, cleanup := startMockAgentServer(t, nil)
	cleanup() // stop immediately — Stop() is synchronous

	ctx := context.Background()
	_, err := streamAgentLogs(ctx, agentAddr, "container", false, 10)
	if err == nil {
		t.Fatal("expected error when agent server is stopped")
	}
}

func TestGetLogs_ClientCancelDuringSend(t *testing.T) {
	// Create a mock agent that sends many chunks slowly, giving time for client cancel
	agentAddr, agentCleanup := startSlowMockAgentServer(t, 100, 50*time.Millisecond)
	defer agentCleanup()

	store := storage.NewMemoryStore()
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-cancel", &types.NodeRecord{
		Name: "agent-cancel", Status: "ready", APIAddress: agentAddr,
	})
	store.Save(ctx, types.KeyTasks+"agent-cancel/task-cancel", &types.TaskRecord{
		ID: "task-cancel", AgentID: "agent-cancel", ContainerName: "cancel-container",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(grpcSrv, engineSrv)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(gCtx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(gCtx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	// Cancel the context after receiving the first response,
	// which should cause a stream.Send error on the server side.
	cancelCtx, cancel := context.WithCancel(ctx)
	stream, err := client.GetLogs(cancelCtx, &banyanpb.GetLogsRequest{
		ContainerName: "cancel-container",
	})
	if err != nil {
		t.Fatalf("GetLogs call failed: %v", err)
	}

	// Read one response then immediately cancel
	_, recvErr := stream.Recv()
	if recvErr != nil {
		t.Fatalf("expected first recv to succeed, got: %v", recvErr)
	}
	cancel()

	// Drain remaining to ensure cleanup
	for {
		_, recvErr = stream.Recv()
		if recvErr != nil {
			break
		}
	}
}

func TestTeardownDeployment(t *testing.T) {
	ctx := context.Background()

	t.Run("creates stop tasks for completed containers", func(t *testing.T) {
		store := storage.NewMemoryStore()

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "deploy-1", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "app-web-0",
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-api", &types.TaskRecord{
			ID: "task-api", DeploymentID: "deploy-1", ServiceName: "api",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "app-api-0",
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-1", Name: "app", Status: types.StatusRunning,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}, "api": {Image: "node"}},
			CreatedAt: time.Now(),
		}
		deployKey := types.KeyDeployments + "deploy-1"
		store.Save(ctx, deployKey, deployment)

		count, err := teardownDeployment(ctx, store, deployment, deployKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 stop tasks, got %d", count)
		}

		// Verify deployment status changed to stopping
		var updated types.DeploymentRecord
		store.Get(ctx, deployKey, &updated)
		if updated.Status != types.StatusStopping {
			t.Errorf("expected stopping, got %s", updated.Status)
		}
		if updated.Error != "" {
			t.Errorf("expected error cleared, got %q", updated.Error)
		}
	})

	t.Run("no running containers marks stopped directly", func(t *testing.T) {
		store := storage.NewMemoryStore()

		// Deployment with no completed tasks (still pending)
		deployment := &types.DeploymentRecord{
			ID: "deploy-2", Name: "pending-app", Status: types.StatusPending,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
			CreatedAt: time.Now(),
		}
		deployKey := types.KeyDeployments + "deploy-2"
		store.Save(ctx, deployKey, deployment)

		count, err := teardownDeployment(ctx, store, deployment, deployKey)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 stop tasks, got %d", count)
		}

		var updated types.DeploymentRecord
		store.Get(ctx, deployKey, &updated)
		if updated.Status != types.StatusStopped {
			t.Errorf("expected stopped, got %s", updated.Status)
		}
	})

	t.Run("save error returns error", func(t *testing.T) {
		memStore := storage.NewMemoryStore()

		memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-3", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "app-web-0",
		})

		deployment := &types.DeploymentRecord{
			ID: "deploy-3", Name: "err-app", Status: types.StatusRunning,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
			CreatedAt: time.Now(),
		}
		deployKey := types.KeyDeployments + "deploy-3"
		memStore.Save(ctx, deployKey, deployment)

		store := &errorStore{MemoryStore: memStore, saveErr: true}

		_, err := teardownDeployment(ctx, store, deployment, deployKey)
		if err == nil {
			t.Fatal("expected error when save fails")
		}
	})
}

func TestPrepareForRedeploy(t *testing.T) {
	ctx := context.Background()

	t.Run("returns running deployment ID for blue-green", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID: "deploy-1", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "deploy-1" {
			t.Errorf("expected deploy-1, got %s", replacesID)
		}

		// Running deployment should NOT be torn down
		var d types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-1", &d)
		if d.Status != types.StatusRunning {
			t.Errorf("expected running (unchanged), got %s", d.Status)
		}
	})

	t.Run("tears down failed deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-f", &types.DeploymentRecord{
			ID: "deploy-f", Name: "app", Status: types.StatusFailed, CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty replacesID (no running), got %s", replacesID)
		}

		// Failed deployment should be torn down (stopped since no containers)
		var d types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-f", &d)
		if d.Status != types.StatusStopped {
			t.Errorf("expected stopped (torn down), got %s", d.Status)
		}
	})

	t.Run("tears down pending deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-p", &types.DeploymentRecord{
			ID: "deploy-p", Name: "app", Status: types.StatusPending, CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty replacesID, got %s", replacesID)
		}

		var d types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-p", &d)
		if d.Status != types.StatusStopped {
			t.Errorf("expected stopped, got %s", d.Status)
		}
	})

	t.Run("skips stopped and stopping deployments", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-stopped", &types.DeploymentRecord{
			ID: "deploy-stopped", Name: "app", Status: types.StatusStopped, CreatedAt: time.Now(),
		})
		store.Save(ctx, types.KeyDeployments+"deploy-stopping", &types.DeploymentRecord{
			ID: "deploy-stopping", Name: "app", Status: types.StatusStopping, CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty, got %s", replacesID)
		}
	})

	t.Run("returns most recent running deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		older := time.Now().Add(-time.Hour)
		newer := time.Now()
		store.Save(ctx, types.KeyDeployments+"deploy-old", &types.DeploymentRecord{
			ID: "deploy-old", Name: "app", Status: types.StatusRunning, CreatedAt: older,
		})
		store.Save(ctx, types.KeyDeployments+"deploy-new", &types.DeploymentRecord{
			ID: "deploy-new", Name: "app", Status: types.StatusRunning, CreatedAt: newer,
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "deploy-new" {
			t.Errorf("expected deploy-new, got %s", replacesID)
		}
	})

	t.Run("handles mixed: running + failed", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"deploy-running", &types.DeploymentRecord{
			ID: "deploy-running", Name: "app", Status: types.StatusRunning,
			CreatedAt: time.Now().Add(-time.Hour),
		})
		store.Save(ctx, types.KeyDeployments+"deploy-failed", &types.DeploymentRecord{
			ID: "deploy-failed", Name: "app", Status: types.StatusFailed,
			CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "deploy-running" {
			t.Errorf("expected deploy-running, got %s", replacesID)
		}

		// Failed should be torn down
		var failed types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-failed", &failed)
		if failed.Status != types.StatusStopped {
			t.Errorf("expected failed deployment to be stopped, got %s", failed.Status)
		}

		// Running should be unchanged
		var running types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"deploy-running", &running)
		if running.Status != types.StatusRunning {
			t.Errorf("expected running deployment unchanged, got %s", running.Status)
		}
	})

	t.Run("empty on list error", func(t *testing.T) {
		store := &errorStore{MemoryStore: storage.NewMemoryStore(), listErr: true}
		srv := &engineGRPCServer{store: store}

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty on error, got %s", replacesID)
		}
	})

	t.Run("skips different app name", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"other-app", &types.DeploymentRecord{
			ID: "other-app", Name: "other", Status: types.StatusRunning, CreatedAt: time.Now(),
		})

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty for different app, got %s", replacesID)
		}
	})

	t.Run("empty on no match", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		replacesID := srv.prepareForRedeploy(ctx, "nonexistent", nil)
		if replacesID != "" {
			t.Errorf("expected empty, got %s", replacesID)
		}
	})

	t.Run("handles teardown error for non-running deployment", func(t *testing.T) {
		memStore := storage.NewMemoryStore()

		// Failed deployment with a completed task (teardown will need to create stop tasks)
		memStore.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		memStore.Save(ctx, types.KeyDeployments+"deploy-f", &types.DeploymentRecord{
			ID: "deploy-f", Name: "app", Status: types.StatusFailed, CreatedAt: time.Now(),
		})
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-f", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "app-web-0",
		})

		// Save errors will cause teardown to fail
		store := &errorStore{MemoryStore: memStore, saveErr: true}
		srv := &engineGRPCServer{store: store}

		// Should not panic — just log the warning
		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty (no running deployment), got %s", replacesID)
		}
	})

	t.Run("handles get error skipping deployments", func(t *testing.T) {
		memStore := storage.NewMemoryStore()
		memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID: "deploy-1", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})

		store := &errorStore{MemoryStore: memStore, getErr: true}
		srv := &engineGRPCServer{store: store}

		replacesID := srv.prepareForRedeploy(ctx, "app", nil)
		if replacesID != "" {
			t.Errorf("expected empty when Get fails, got %s", replacesID)
		}
	})
}

func TestDeployBlueGreen(t *testing.T) {
	ctx := context.Background()

	t.Run("sets ReplacesID for running deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

		// Existing running deployment with a completed container task
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyDeployments+"my-app-old", &types.DeploymentRecord{
			ID: "my-app-old", Name: "my-app", Status: types.StatusRunning,
			Services:  map[string]types.ServiceRecord{"web": {Image: "nginx"}},
			CreatedAt: time.Now().Add(-time.Hour),
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-old", &types.TaskRecord{
			ID: "task-old", DeploymentID: "my-app-old", ServiceName: "web",
			AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, ContainerName: "my-app-web-0",
		})

		// Deploy same app name
		resp, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "my-app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {Image: "nginx:v2"},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}
		if resp.DeploymentId == "" {
			t.Error("expected non-empty deployment_id")
		}

		// Old deployment should STILL be running (blue-green)
		var old types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"my-app-old", &old)
		if old.Status != types.StatusRunning {
			t.Errorf("expected old deployment still running (blue-green), got %s", old.Status)
		}

		// New deployment should have ReplacesID and UpdateStrategy
		var newDeploy types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+resp.DeploymentId, &newDeploy)
		if newDeploy.ReplacesID != "my-app-old" {
			t.Errorf("expected ReplacesID my-app-old, got %s", newDeploy.ReplacesID)
		}
		if newDeploy.UpdateStrategy != types.UpdateStrategyBlueGreen {
			t.Errorf("expected blue-green strategy, got %s", newDeploy.UpdateStrategy)
		}
	})

	t.Run("no-op when no existing deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

		resp, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "new-app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {Image: "nginx"},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}
		if resp.Status != types.StatusPending {
			t.Errorf("expected pending, got %s", resp.Status)
		}

		// New deployment should have no ReplacesID
		var newDeploy types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+resp.DeploymentId, &newDeploy)
		if newDeploy.ReplacesID != "" {
			t.Errorf("expected empty ReplacesID, got %s", newDeploy.ReplacesID)
		}

		// Only one deployment should exist
		keys, _ := store.List(ctx, types.KeyDeployments)
		if len(keys) != 1 {
			t.Errorf("expected 1 deployment, got %d", len(keys))
		}
	})

	t.Run("skips stopped deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

		store.Save(ctx, types.KeyDeployments+"old-stopped", &types.DeploymentRecord{
			ID: "old-stopped", Name: "app", Status: types.StatusStopped,
			CreatedAt: time.Now().Add(-time.Hour),
		})

		resp, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {Image: "nginx"},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}

		// Old stopped deployment should remain stopped
		var old types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"old-stopped", &old)
		if old.Status != types.StatusStopped {
			t.Errorf("expected old deployment to remain stopped, got %s", old.Status)
		}

		// No ReplacesID
		var newDeploy types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+resp.DeploymentId, &newDeploy)
		if newDeploy.ReplacesID != "" {
			t.Errorf("expected empty ReplacesID, got %s", newDeploy.ReplacesID)
		}
	})

	t.Run("tears down failed deployment immediately", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store, registryURL: "localhost:5000"}

		store.Save(ctx, types.KeyDeployments+"failed-deploy", &types.DeploymentRecord{
			ID: "failed-deploy", Name: "app", Status: types.StatusFailed,
			Error:     "1/1 tasks failed",
			CreatedAt: time.Now().Add(-time.Hour),
		})

		resp, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "app",
				Services: map[string]*banyanpb.ManifestService{
					"web": {Image: "nginx"},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy failed: %v", err)
		}

		// Failed deployment with no running containers → marked stopped directly
		var old types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"failed-deploy", &old)
		if old.Status != types.StatusStopped {
			t.Errorf("expected old failed deployment to be stopped, got %s", old.Status)
		}

		// No ReplacesID since there's no running deployment
		var newDeploy types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+resp.DeploymentId, &newDeploy)
		if newDeploy.ReplacesID != "" {
			t.Errorf("expected empty ReplacesID, got %s", newDeploy.ReplacesID)
		}
	})
}

func TestGetLogs_SuccessProxyWithFollow(t *testing.T) {
	// Start a mock agent that sends log data
	logChunks := [][]byte{
		[]byte("log entry 1\n"),
		[]byte("log entry 2\n"),
		[]byte("log entry 3\n"),
	}
	agentAddr, agentCleanup := startMockAgentServer(t, logChunks)
	defer agentCleanup()

	store := storage.NewMemoryStore()
	ctx := context.Background()

	store.Save(ctx, types.KeyNodes+"agent-follow", &types.NodeRecord{
		Name: "agent-follow", Status: "ready", APIAddress: agentAddr,
	})
	store.Save(ctx, types.KeyTasks+"agent-follow/task-follow", &types.TaskRecord{
		ID: "task-follow", AgentID: "agent-follow", ContainerName: "follow-container",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})

	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(grpcSrv, engineSrv)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	defer grpcSrv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	stream, err := client.GetLogs(ctx, &banyanpb.GetLogsRequest{
		ContainerName: "follow-container",
		Follow:        true,
		Tail:          50,
	})
	if err != nil {
		t.Fatalf("GetLogs call failed: %v", err)
	}

	var received []byte
	for {
		resp, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			break
		}
		received = append(received, resp.Data...)
	}

	expected := "log entry 1\nlog entry 2\nlog entry 3\n"
	if string(received) != expected {
		t.Errorf("expected log data %q, got %q", expected, string(received))
	}
}

// --- Per-service deploy helper tests ---

func TestFindRunningDeploymentByName(t *testing.T) {
	ctx := context.Background()

	t.Run("returns most recent running deployment", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"old", &types.DeploymentRecord{
			ID: "old", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now().Add(-1 * time.Hour),
		})
		store.Save(ctx, types.KeyDeployments+"new", &types.DeploymentRecord{
			ID: "new", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})

		deploy, _, err := srv.findRunningDeploymentByName(ctx, "app", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deploy.ID != "new" {
			t.Errorf("expected 'new', got %q", deploy.ID)
		}
	})

	t.Run("skips non-running deployments", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"stopped", &types.DeploymentRecord{
			ID: "stopped", Name: "app", Status: types.StatusStopped, CreatedAt: time.Now(),
		})

		_, _, err := srv.findRunningDeploymentByName(ctx, "app", nil)
		if err == nil {
			t.Fatal("expected error when no running deployment exists")
		}
	})

	t.Run("returns error for unknown name", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		_, _, err := srv.findRunningDeploymentByName(ctx, "nonexistent", nil)
		if err == nil {
			t.Fatal("expected error for unknown name")
		}
	})
}

func TestGetRunningServiceNames(t *testing.T) {
	ctx := context.Background()

	t.Run("collects running service names", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-2", &types.TaskRecord{
			ID: "task-2", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		// Pending task should not be included
		store.Save(ctx, types.KeyTasks+"agent-1/task-3", &types.TaskRecord{
			ID: "task-3", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "db", Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		})

		names := srv.getRunningServiceNames(ctx, "deploy-1")
		nameSet := make(map[string]bool)
		for _, n := range names {
			nameSet[n] = true
		}
		if !nameSet["web"] || !nameSet["api"] {
			t.Errorf("expected web and api, got %v", names)
		}
		if nameSet["db"] {
			t.Errorf("db should not be included (pending), got %v", names)
		}
	})

	t.Run("returns empty for no tasks", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		names := srv.getRunningServiceNames(ctx, "deploy-1")
		if len(names) != 0 {
			t.Errorf("expected 0 names, got %d", len(names))
		}
	})
}

func TestTeardownDeploymentServices(t *testing.T) {
	ctx := context.Background()

	t.Run("creates stop tasks for target services only", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-web-0",
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-api", &types.TaskRecord{
			ID: "task-api", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-api-0",
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-db", &types.TaskRecord{
			ID: "task-db", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "db", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-db-0",
		})

		srv.teardownDeploymentServices(ctx, "deploy-1", []string{"web", "api"})

		// Stop tasks should exist for web and api only
		var webStop types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"agent-1/task-web-stop", &webStop); err != nil {
			t.Fatalf("expected web stop task: %v", err)
		}
		if webStop.Type != types.TaskTypeStopAndRemove {
			t.Errorf("expected stop_and_remove, got %s", webStop.Type)
		}

		var apiStop types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"agent-1/task-api-stop", &apiStop); err != nil {
			t.Fatalf("expected api stop task: %v", err)
		}

		// No stop task for db
		var dbStop types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"agent-1/task-db-stop", &dbStop); err == nil {
			t.Error("expected no stop task for db")
		}
	})

	t.Run("skips non-completed tasks", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "deploy-1", AgentID: "agent-1",
			ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
			ContainerName: "app-web-0",
		})

		srv.teardownDeploymentServices(ctx, "deploy-1", []string{"web"})

		// No stop task should be created for a pending task
		var stopTask types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"agent-1/task-web-stop", &stopTask); err == nil {
			t.Error("expected no stop task for pending create task")
		}
	})
}

func TestTeardownNonRunningDeployments(t *testing.T) {
	ctx := context.Background()

	t.Run("tears down pending and failed, leaves running", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"running", &types.DeploymentRecord{
			ID: "running", Name: "app", Status: types.StatusRunning,
		})
		store.Save(ctx, types.KeyDeployments+"pending", &types.DeploymentRecord{
			ID: "pending", Name: "app", Status: types.StatusPending,
		})
		store.Save(ctx, types.KeyDeployments+"failed", &types.DeploymentRecord{
			ID: "failed", Name: "app", Status: types.StatusFailed,
		})

		srv.teardownNonRunningDeployments(ctx, "app", nil)

		// Running should remain
		var running types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"running", &running)
		if running.Status != types.StatusRunning {
			t.Errorf("expected running unchanged, got %s", running.Status)
		}

		// Pending and failed should be stopped
		var pending types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"pending", &pending)
		if pending.Status != types.StatusStopped {
			t.Errorf("expected pending -> stopped, got %s", pending.Status)
		}

		var failed types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"failed", &failed)
		if failed.Status != types.StatusStopped {
			t.Errorf("expected failed -> stopped, got %s", failed.Status)
		}
	})

	t.Run("ignores other app names", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"other", &types.DeploymentRecord{
			ID: "other", Name: "other-app", Status: types.StatusPending,
		})

		srv.teardownNonRunningDeployments(ctx, "app", nil)

		var other types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+"other", &other)
		if other.Status != types.StatusPending {
			t.Errorf("expected pending unchanged, got %s", other.Status)
		}
	})
}

func TestDeployServices(t *testing.T) {
	ctx := context.Background()

	t.Run("creates blue-green deployment for target services", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		// Existing running deployment with web, api, db
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyDeployments+"old-deploy", &types.DeploymentRecord{
			ID: "old-deploy", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "old-deploy", AgentID: "agent-1",
			ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-web-0",
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-api", &types.TaskRecord{
			ID: "task-api", DeploymentID: "old-deploy", AgentID: "agent-1",
			ServiceName: "api", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-api-0",
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-db", &types.TaskRecord{
			ID: "task-db", DeploymentID: "old-deploy", AgentID: "agent-1",
			ServiceName: "db", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-db-0",
		})

		allServices := map[string]types.ServiceRecord{
			"web": {Image: "nginx:v2", Replicas: 1, DependsOn: types.DependsOnConfig{"api": {Condition: "service_started"}}},
			"api": {Image: "myapi:v2", Replicas: 1, DependsOn: types.DependsOnConfig{"db": {Condition: "service_started"}}},
			"db":  {Image: "postgres:15", Replicas: 1},
		}

		// Deploy only "web" — its dep "api" is already running
		resp, err := srv.deployServices(ctx, "app", allServices, []string{"web"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Status != types.StatusPending {
			t.Errorf("expected pending, got %s", resp.Status)
		}

		// Verify new deployment has blue-green strategy (not recreate)
		var newDeploy types.DeploymentRecord
		store.Get(ctx, types.KeyDeployments+resp.DeploymentId, &newDeploy)
		if newDeploy.UpdateStrategy != types.UpdateStrategyBlueGreen {
			t.Errorf("expected blue-green strategy, got %s", newDeploy.UpdateStrategy)
		}
		if newDeploy.ReplacesID != "old-deploy" {
			t.Errorf("expected replaces_id 'old-deploy', got %s", newDeploy.ReplacesID)
		}

		// Only "web" service in new deployment
		if len(newDeploy.Services) != 1 {
			t.Errorf("expected 1 service, got %d", len(newDeploy.Services))
		}
		if _, ok := newDeploy.Services["web"]; !ok {
			t.Error("expected 'web' service in new deployment")
		}

		// No stop task should exist upfront — blue-green tears down old after new is running
		var webStop types.TaskRecord
		if err := store.Get(ctx, types.KeyTasks+"agent-1/task-web-stop", &webStop); err == nil {
			t.Error("expected no upfront stop task for blue-green strategy")
		}
	})

	t.Run("rejects unknown service name", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		allServices := map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		}

		_, err := srv.deployServices(ctx, "app", allServices, []string{"nonexistent"}, nil)
		if err == nil {
			t.Fatal("expected error for unknown service")
		}
	})

	t.Run("rejects when no running deployment exists", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		allServices := map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		}

		_, err := srv.deployServices(ctx, "app", allServices, []string{"web"}, nil)
		if err == nil {
			t.Fatal("expected error when no running deployment exists")
		}
	})

	t.Run("rejects unsatisfied dependencies", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		// Running deployment with only web (no db)
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyDeployments+"old-deploy", &types.DeploymentRecord{
			ID: "old-deploy", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})
		store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
			ID: "task-web", DeploymentID: "old-deploy", AgentID: "agent-1",
			ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})

		allServices := map[string]types.ServiceRecord{
			"api": {Image: "myapi", Replicas: 1, DependsOn: types.DependsOnConfig{"db": {Condition: "service_started"}}},
			"web": {Image: "nginx", Replicas: 1},
			"db":  {Image: "postgres", Replicas: 1},
		}

		// Deploy "api" which depends on "db", but "db" is not running
		_, err := srv.deployServices(ctx, "app", allServices, []string{"api"}, nil)
		if err == nil {
			t.Fatal("expected error for unsatisfied dependency")
		}
	})
}

func TestReportContainerHealth_StoresIP(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create a completed task
	srv.store.Save(ctx, types.KeyTasks+"worker-1/task-ip1", &types.TaskRecord{
		ID: "task-ip1", AgentID: "worker-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0",
	})

	_, err := client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "worker-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "app-web-0", Status: "running", Ip: "10.0.1.5"},
		},
	})
	if err != nil {
		t.Fatalf("ReportContainerHealth failed: %v", err)
	}

	var updated types.TaskRecord
	if err := srv.store.Get(ctx, types.KeyTasks+"worker-1/task-ip1", &updated); err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.ContainerIP != "10.0.1.5" {
		t.Errorf("expected container IP '10.0.1.5', got %q", updated.ContainerIP)
	}
	if updated.ContainerStatus != "running" {
		t.Errorf("expected container status 'running', got %q", updated.ContainerStatus)
	}
}

func TestReportContainerHealth_StoresHealthStatus(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	srv.store.Save(ctx, types.KeyTasks+"worker-1/task-hc1", &types.TaskRecord{
		ID: "task-hc1", AgentID: "worker-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-db-0",
	})

	_, err := client.ReportContainerHealth(ctx, &banyanpb.ReportContainerHealthRequest{
		AgentName: "worker-1",
		Containers: []*banyanpb.ContainerStatus{
			{ContainerName: "app-db-0", Status: "running", Ip: "10.0.1.10", HealthStatus: "healthy"},
		},
	})
	if err != nil {
		t.Fatalf("ReportContainerHealth failed: %v", err)
	}

	var updated types.TaskRecord
	if err := srv.store.Get(ctx, types.KeyTasks+"worker-1/task-hc1", &updated); err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.HealthStatus != "healthy" {
		t.Errorf("expected health status 'healthy', got %q", updated.HealthStatus)
	}
	if updated.ContainerStatus != "running" {
		t.Errorf("expected container status 'running', got %q", updated.ContainerStatus)
	}
}

func TestCollectServiceBackends(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}
	ctx := context.Background()

	// Register agents and a running deployment
	store.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{Name: "worker-1", Status: "ready"})
	store.Save(ctx, types.KeyNodes+"worker-2", &types.NodeRecord{Name: "worker-2", Status: "ready"})
	store.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
	})
	// Old stopped deployment — backends should be excluded
	store.Save(ctx, types.KeyDeployments+"deploy-old", &types.DeploymentRecord{
		ID: "deploy-old", Name: "app", Status: types.StatusStopped,
	})

	// Task with IP, ports, running, completed — should be included
	store.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "worker-1", ServiceName: "web",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0", ContainerStatus: "running",
		ContainerIP: "10.0.1.5", Ports: []string{"8080:80"},
	})

	// Task without IP — should be excluded
	store.Save(ctx, types.KeyTasks+"worker-1/task-2", &types.TaskRecord{
		ID: "task-2", DeploymentID: "deploy-1", AgentID: "worker-1", ServiceName: "api",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-api-0", ContainerStatus: "running",
		Ports: []string{"9090:90"},
	})

	// Task with IP but no ports — should be INCLUDED (DNS needs portless containers)
	store.Save(ctx, types.KeyTasks+"worker-2/task-3", &types.TaskRecord{
		ID: "task-3", DeploymentID: "deploy-1", AgentID: "worker-2", ServiceName: "db",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-worker-0", ContainerStatus: "running",
		ContainerIP: "10.0.2.5",
	})

	// Task with IP and ports but not running — should be excluded
	store.Save(ctx, types.KeyTasks+"worker-2/task-4", &types.TaskRecord{
		ID: "task-4", DeploymentID: "deploy-1", AgentID: "worker-2", ServiceName: "db",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-db-0", ContainerStatus: "exited",
		ContainerIP: "10.0.2.6", Ports: []string{"5432:5432"},
	})

	// Stop task — should be excluded
	store.Save(ctx, types.KeyTasks+"worker-1/task-5", &types.TaskRecord{
		ID: "task-5", DeploymentID: "deploy-1", AgentID: "worker-1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
		ContainerName: "app-old-0",
	})

	// Full match on worker-2
	store.Save(ctx, types.KeyTasks+"worker-2/task-6", &types.TaskRecord{
		ID: "task-6", DeploymentID: "deploy-1", AgentID: "worker-2", ServiceName: "web",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-1", ContainerStatus: "running",
		ContainerIP: "10.0.2.7", Ports: []string{"8080:80"},
	})

	// Task from old stopped deployment — should be excluded
	store.Save(ctx, types.KeyTasks+"worker-1/task-7", &types.TaskRecord{
		ID: "task-7", DeploymentID: "deploy-old", AgentID: "worker-1", ServiceName: "web",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-old-web-0", ContainerStatus: "running",
		ContainerIP: "10.0.1.99", Ports: []string{"8080:80"},
	})

	backends := srv.collectServiceBackends(ctx)

	// 3 backends: app-web-0 (with ports), app-worker-0 (no ports but has IP), app-web-1 (with ports)
	if len(backends) != 3 {
		t.Fatalf("expected 3 backends, got %d", len(backends))
	}

	// Verify the included backends
	byName := make(map[string]*banyanpb.ServiceBackend)
	for _, b := range backends {
		byName[b.ContainerName] = b
		if b.ContainerIp == "" {
			t.Errorf("backend %s has empty IP", b.ContainerName)
		}
	}
	if _, ok := byName["app-web-0"]; !ok {
		t.Error("expected app-web-0 in backends")
	}
	if _, ok := byName["app-web-1"]; !ok {
		t.Error("expected app-web-1 in backends")
	}
	if _, ok := byName["app-worker-0"]; !ok {
		t.Error("expected app-worker-0 in backends (portless container with IP)")
	}

	// Verify ServiceName is populated
	if byName["app-web-0"].ServiceName != "web" {
		t.Errorf("expected ServiceName 'web' for app-web-0, got %q", byName["app-web-0"].ServiceName)
	}
	if byName["app-worker-0"].ServiceName != "db" {
		t.Errorf("expected ServiceName 'db' for app-worker-0, got %q", byName["app-worker-0"].ServiceName)
	}
}

func TestHeartbeat_ReturnsServiceBackends(t *testing.T) {
	memStore := storage.NewMemoryStore()
	peerTracker := overlay.NewPeerTracker()

	srv := &engineGRPCServer{
		store:       memStore,
		registryURL: "localhost:5000",
		peerTracker: peerTracker,
	}

	ctx := context.Background()

	// Register agent, deployment, and add a running container with IP
	memStore.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{Name: "worker-1", Status: "ready"})
	memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
		ID: "deploy-1", Name: "app", Status: types.StatusRunning,
	})
	memStore.Save(ctx, types.KeyTasks+"worker-1/task-hb1", &types.TaskRecord{
		ID: "task-hb1", DeploymentID: "deploy-1", AgentID: "worker-1", ServiceName: "web",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0", ContainerStatus: "running",
		ContainerIP: "10.0.1.5", Ports: []string{"8080:80"},
	})

	resp, err := srv.Heartbeat(ctx, &banyanpb.HeartbeatRequest{AgentName: "worker-1", SessionToken: "t1"})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if len(resp.ServiceBackends) != 1 {
		t.Fatalf("expected 1 service backend, got %d", len(resp.ServiceBackends))
	}
	b := resp.ServiceBackends[0]
	if b.ContainerName != "app-web-0" {
		t.Errorf("expected container name app-web-0, got %s", b.ContainerName)
	}
	if b.ContainerIp != "10.0.1.5" {
		t.Errorf("expected container IP 10.0.1.5, got %s", b.ContainerIp)
	}
	if b.AgentName != "worker-1" {
		t.Errorf("expected agent name worker-1, got %s", b.AgentName)
	}
	if b.ServiceName != "web" {
		t.Errorf("expected service name web, got %s", b.ServiceName)
	}
}

func TestHeartbeat_NoBackendsWithoutPeerTracker(t *testing.T) {
	memStore := storage.NewMemoryStore()

	srv := &engineGRPCServer{
		store:       memStore,
		registryURL: "localhost:5000",
		// peerTracker is nil — no VPC
	}

	ctx := context.Background()

	memStore.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{Name: "worker-1", Status: "ready"})
	memStore.Save(ctx, types.KeyTasks+"worker-1/task-hb2", &types.TaskRecord{
		ID: "task-hb2", AgentID: "worker-1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0", ContainerStatus: "running",
		ContainerIP: "10.0.1.5", Ports: []string{"8080:80"},
	})

	resp, err := srv.Heartbeat(ctx, &banyanpb.HeartbeatRequest{AgentName: "worker-1", SessionToken: "t1"})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Without peer tracker, no backends should be returned
	if len(resp.ServiceBackends) != 0 {
		t.Errorf("expected 0 service backends without VPC, got %d", len(resp.ServiceBackends))
	}
}

func TestCountAgentContainers(t *testing.T) {
	memStore := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: memStore}
	ctx := context.Background()

	t.Run("counts completed create_and_start tasks", func(t *testing.T) {
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-2", &types.TaskRecord{
			ID: "task-2", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		})
		// Pending task should not count
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-3", &types.TaskRecord{
			ID: "task-3", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		})
		// Stop task should not count
		memStore.Save(ctx, types.KeyTasks+"agent-1/task-4", &types.TaskRecord{
			ID: "task-4", AgentID: "agent-1", Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
		})

		count := srv.countAgentContainers(ctx, "agent-1")
		if count != 2 {
			t.Errorf("expected 2 containers, got %d", count)
		}
	})

	t.Run("returns 0 for unknown agent", func(t *testing.T) {
		count := srv.countAgentContainers(ctx, "unknown-agent")
		if count != 0 {
			t.Errorf("expected 0 containers, got %d", count)
		}
	})

	t.Run("returns 0 on store error", func(t *testing.T) {
		errSrv := &engineGRPCServer{store: &errorStore{MemoryStore: memStore, listErr: true}}
		count := errSrv.countAgentContainers(ctx, "agent-1")
		if count != 0 {
			t.Errorf("expected 0 on error, got %d", count)
		}
	})
}

func TestEmitEvent(t *testing.T) {
	memStore := storage.NewMemoryStore()
	events := NewEventBuffer(10)
	srv := &engineGRPCServer{
		store:  memStore,
		events: events,
	}

	t.Run("adds event to buffer", func(t *testing.T) {
		srv.emitEvent("test.event", "something happened", "info")
		recent := events.Recent(1)
		if len(recent) != 1 {
			t.Fatalf("expected 1 event, got %d", len(recent))
		}
		if recent[0].Type != "test.event" {
			t.Errorf("expected type 'test.event', got %q", recent[0].Type)
		}
		if recent[0].Message != "something happened" {
			t.Errorf("expected message 'something happened', got %q", recent[0].Message)
		}
		if recent[0].Severity != "info" {
			t.Errorf("expected severity 'info', got %q", recent[0].Severity)
		}
	})

	t.Run("safe with nil events and registry", func(t *testing.T) {
		nilSrv := &engineGRPCServer{store: memStore}
		nilSrv.emitEvent("test.event", "should not panic", "info") // should not panic
	})
}

func TestGetDashboardData(t *testing.T) {
	memStore := storage.NewMemoryStore()
	events := NewEventBuffer(10)

	srv := &engineGRPCServer{
		store:     memStore,
		events:    events,
		startedAt: time.Now().Add(-5 * time.Minute),
	}
	ctx := context.Background()

	// Set up test data
	memStore.Save(ctx, types.KeyNodes+"worker-1", &types.NodeRecord{
		Name: "worker-1", Status: "ready", APIAddress: "worker-1:9090",
		LastSeen: time.Now(), CreatedAt: time.Now().Add(-1 * time.Hour),
		Tags: []string{"zone:us-west"},
	})
	memStore.Save(ctx, types.KeyDeployments+"myapp-1", &types.DeploymentRecord{
		ID: "myapp-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx:alpine", Replicas: 2, Ports: []string{"80:80"}},
		},
		CreatedAt: time.Now().Add(-30 * time.Minute),
		UpdatedAt: time.Now(),
	})
	memStore.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "myapp-1", AgentID: "worker-1",
		ServiceName: "web", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusCompleted, ContainerStatus: types.StatusRunning,
		ContainerName: "myapp-web-0", Image: "nginx:alpine",
		CreatedAt: time.Now().Add(-25 * time.Minute),
		UpdatedAt: time.Now(),
	})

	// Add an event
	events.Add(Event{
		Timestamp: time.Now(),
		Type:      "deployment.running",
		Message:   "Deployment myapp is running",
		Severity:  "info",
	})

	t.Run("returns complete dashboard data", func(t *testing.T) {
		resp, err := srv.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
		if err != nil {
			t.Fatalf("GetDashboardData failed: %v", err)
		}

		// Engine status
		if resp.Engine == nil {
			t.Fatal("expected engine status")
		}
		if resp.Engine.Status != "running" {
			t.Errorf("expected engine status 'running', got %q", resp.Engine.Status)
		}
		if resp.Engine.StartedAtUnix == 0 {
			t.Error("expected non-zero started_at_unix")
		}

		// Agents
		if len(resp.Agents) != 1 {
			t.Fatalf("expected 1 agent, got %d", len(resp.Agents))
		}
		if resp.Agents[0].Name != "worker-1" {
			t.Errorf("expected agent name 'worker-1', got %q", resp.Agents[0].Name)
		}
		if resp.Agents[0].ContainerCount != 1 {
			t.Errorf("expected 1 container, got %d", resp.Agents[0].ContainerCount)
		}

		// Deployments
		if len(resp.Deployments) != 1 {
			t.Fatalf("expected 1 deployment, got %d", len(resp.Deployments))
		}
		if resp.Deployments[0].Name != "myapp" {
			t.Errorf("expected deployment name 'myapp', got %q", resp.Deployments[0].Name)
		}
		if resp.Deployments[0].Healthy != 1 {
			t.Errorf("expected 1 healthy, got %d", resp.Deployments[0].Healthy)
		}

		// Summary
		if resp.Summary == nil {
			t.Fatal("expected cluster summary")
		}
		if resp.Summary.TotalAgents != 1 {
			t.Errorf("expected 1 total agent, got %d", resp.Summary.TotalAgents)
		}
		if resp.Summary.ConnectedAgents != 1 {
			t.Errorf("expected 1 connected agent, got %d", resp.Summary.ConnectedAgents)
		}
		if resp.Summary.TotalDeployments != 1 {
			t.Errorf("expected 1 total deployment, got %d", resp.Summary.TotalDeployments)
		}
		if resp.Summary.RunningDeployments != 1 {
			t.Errorf("expected 1 running deployment, got %d", resp.Summary.RunningDeployments)
		}
		if resp.Summary.TotalContainers != 1 {
			t.Errorf("expected 1 total container, got %d", resp.Summary.TotalContainers)
		}
		if resp.Summary.HealthyContainers != 1 {
			t.Errorf("expected 1 healthy container, got %d", resp.Summary.HealthyContainers)
		}

		// Events
		if len(resp.RecentEvents) != 1 {
			t.Fatalf("expected 1 event, got %d", len(resp.RecentEvents))
		}
		if resp.RecentEvents[0].Type != "deployment.running" {
			t.Errorf("expected event type 'deployment.running', got %q", resp.RecentEvents[0].Type)
		}
	})

	t.Run("marks stale agents", func(t *testing.T) {
		memStore.Save(ctx, types.KeyNodes+"stale-worker", &types.NodeRecord{
			Name: "stale-worker", Status: "ready",
			LastSeen: time.Now().Add(-2 * time.Minute), // older than 60s
		})

		resp, err := srv.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
		if err != nil {
			t.Fatalf("GetDashboardData failed: %v", err)
		}

		for _, agent := range resp.Agents {
			if agent.Name == "stale-worker" {
				if agent.Status != "stale" {
					t.Errorf("expected stale-worker status 'stale', got %q", agent.Status)
				}
				return
			}
		}
		t.Error("stale-worker not found in response")
	})

	t.Run("empty store returns valid response", func(t *testing.T) {
		emptySrv := &engineGRPCServer{
			store:     storage.NewMemoryStore(),
			events:    NewEventBuffer(10),
			startedAt: time.Now(),
		}
		resp, err := emptySrv.GetDashboardData(ctx, &banyanpb.GetDashboardDataRequest{})
		if err != nil {
			t.Fatalf("GetDashboardData failed: %v", err)
		}
		if resp.Engine == nil {
			t.Fatal("expected engine status even on empty store")
		}
		if resp.Summary == nil {
			t.Fatal("expected summary even on empty store")
		}
		if resp.Summary.TotalAgents != 0 {
			t.Errorf("expected 0 agents, got %d", resp.Summary.TotalAgents)
		}
	})
}

func TestRegister_EmitsEvent(t *testing.T) {
	memStore := storage.NewMemoryStore()
	events := NewEventBuffer(10)
	srv := &engineGRPCServer{
		store:       memStore,
		registryURL: "localhost:5000",
		events:      events,
	}

	ctx := context.Background()
	_, err := srv.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "worker-1",
		ApiAddress:   "worker-1:9090",
		SessionToken: "token-abc",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	recent := events.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 event, got %d", len(recent))
	}
	if recent[0].Type != "agent.registered" {
		t.Errorf("expected event type 'agent.registered', got %q", recent[0].Type)
	}
}

func TestDeploy_EmitsEvent(t *testing.T) {
	memStore := storage.NewMemoryStore()
	events := NewEventBuffer(10)
	srv := &engineGRPCServer{
		store:       memStore,
		registryURL: "localhost:5000",
		events:      events,
	}

	ctx := context.Background()
	_, err := srv.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: &banyanpb.Manifest{
			Name:    "myapp",
			Version: "1",
			Services: map[string]*banyanpb.ManifestService{
				"web": {Image: "nginx:alpine"},
			},
		},
	})
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	recent := events.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 event, got %d", len(recent))
	}
	if recent[0].Type != "deployment.created" {
		t.Errorf("expected event type 'deployment.created', got %q", recent[0].Type)
	}
}

func TestDown_EmitsEvent(t *testing.T) {
	memStore := storage.NewMemoryStore()
	events := NewEventBuffer(10)
	srv := &engineGRPCServer{
		store:       memStore,
		registryURL: "localhost:5000",
		events:      events,
	}

	ctx := context.Background()

	// Create a deployment to stop
	memStore.Save(ctx, types.KeyDeployments+"myapp-1", &types.DeploymentRecord{
		ID: "myapp-1", Name: "myapp", Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx:alpine", Replicas: 1},
		},
		CreatedAt: time.Now(),
	})

	_, err := srv.Down(ctx, &banyanpb.DownRPCRequest{Name: "myapp"})
	if err != nil {
		t.Fatalf("Down failed: %v", err)
	}

	recent := events.Recent(1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 event, got %d", len(recent))
	}
	if recent[0].Type != "deployment.stopped" {
		t.Errorf("expected event type 'deployment.stopped', got %q", recent[0].Type)
	}
}

func TestHeartbeat_UpdatesAgentMetrics(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Register agent first
	_, err := client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "worker-1",
		ApiAddress:   "worker-1:9090",
		SessionToken: "token-1",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Heartbeat without metrics (no metricsRegistry on test server) — should not panic
	_, err = client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "worker-1",
		SessionToken: "token-1",
		SystemMetrics: &banyanpb.SystemMetrics{
			CpuUsageRatio:    0.42,
			MemoryUsedBytes:  1000000,
			MemoryTotalBytes: 2000000,
			DiskUsedBytes:    5000000,
			DiskTotalBytes:   10000000,
			CpuCores:         4,
		},
	})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
}

func TestHeartbeat_MetricsBoundsChecking(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Register agent
	_, err := client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "worker-bounds",
		ApiAddress:   "worker-bounds:9090",
		SessionToken: "token-bounds",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	t.Run("valid metrics stored", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-bounds",
			SessionToken: "token-bounds",
			SystemMetrics: &banyanpb.SystemMetrics{
				MemoryTotalBytes: 8 * 1024 * 1024 * 1024, // 8 GB
				MemoryUsedBytes:  2 * 1024 * 1024 * 1024,  // 2 GB
				CpuCores:         4,
				CpuUsageRatio:    0.5,
			},
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		var node types.NodeRecord
		if err := srv.store.Get(ctx, types.KeyNodes+"worker-bounds", &node); err != nil {
			t.Fatalf("Get node failed: %v", err)
		}
		if node.MemoryTotalBytes != 8*1024*1024*1024 {
			t.Errorf("MemoryTotalBytes = %d, want %d", node.MemoryTotalBytes, 8*1024*1024*1024)
		}
		if node.MemoryUsedBytes != 2*1024*1024*1024 {
			t.Errorf("MemoryUsedBytes = %d, want %d", node.MemoryUsedBytes, 2*1024*1024*1024)
		}
		if node.CPUCores != 4 {
			t.Errorf("CPUCores = %d, want 4", node.CPUCores)
		}
		if node.CPUUsageRatio != 0.5 {
			t.Errorf("CPUUsageRatio = %f, want 0.5", node.CPUUsageRatio)
		}
	})

	t.Run("excessive memory rejected", func(t *testing.T) {
		// First set known good values
		_, _ = client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-bounds",
			SessionToken: "token-bounds",
			SystemMetrics: &banyanpb.SystemMetrics{
				MemoryTotalBytes: 8 * 1024 * 1024 * 1024,
				MemoryUsedBytes:  1 * 1024 * 1024 * 1024,
				CpuCores:         4,
				CpuUsageRatio:    0.25,
			},
		})

		// Send bogus metrics — should not update memory/cpu fields
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-bounds",
			SessionToken: "token-bounds",
			SystemMetrics: &banyanpb.SystemMetrics{
				MemoryTotalBytes: 1<<63 + 1, // exceeds 64 TB cap
				MemoryUsedBytes:  1 << 62,
				CpuCores:         9999,        // exceeds 1024 cap
				CpuUsageRatio:    5.0,         // exceeds 1.0 cap
			},
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		var node types.NodeRecord
		if err := srv.store.Get(ctx, types.KeyNodes+"worker-bounds", &node); err != nil {
			t.Fatalf("Get node failed: %v", err)
		}
		// Fields should retain previous good values
		if node.MemoryTotalBytes != 8*1024*1024*1024 {
			t.Errorf("MemoryTotalBytes = %d, want previous value %d (bogus value should be rejected)", node.MemoryTotalBytes, 8*1024*1024*1024)
		}
		if node.CPUCores != 4 {
			t.Errorf("CPUCores = %d, want previous value 4 (bogus value should be rejected)", node.CPUCores)
		}
		if node.CPUUsageRatio != 0.25 {
			t.Errorf("CPUUsageRatio = %f, want previous value 0.25 (bogus value should be rejected)", node.CPUUsageRatio)
		}
	})

	t.Run("used memory capped at total", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-bounds",
			SessionToken: "token-bounds",
			SystemMetrics: &banyanpb.SystemMetrics{
				MemoryTotalBytes: 4 * 1024 * 1024 * 1024, // 4 GB
				MemoryUsedBytes:  8 * 1024 * 1024 * 1024,  // 8 GB (> total, should be capped)
				CpuCores:         2,
				CpuUsageRatio:    0.9,
			},
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		var node types.NodeRecord
		if err := srv.store.Get(ctx, types.KeyNodes+"worker-bounds", &node); err != nil {
			t.Fatalf("Get node failed: %v", err)
		}
		if node.MemoryUsedBytes != 4*1024*1024*1024 {
			t.Errorf("MemoryUsedBytes = %d, want %d (should be capped at total)", node.MemoryUsedBytes, 4*1024*1024*1024)
		}
	})
}

func TestReconcileDeploymentStatus(t *testing.T) {
	t.Run("marks deployment stopped when all containers dead", func(t *testing.T) {
		store := storage.NewMemoryStore()
		ctx := context.Background()
		events := NewEventBuffer(10)

		srv := &engineGRPCServer{
			store:  store,
			events: events,
		}

		// Register the agent node so CollectDeploymentTasks can find tasks
		_ = store.Save(ctx, types.KeyNodes+"agent1", &types.NodeRecord{Name: "agent1"})

		// Create a running deployment
		depID := "dep-reconcile-1"
		_ = store.Save(ctx, types.KeyDeployments+depID, &types.DeploymentRecord{
			ID:     depID,
			Name:   "test-app",
			Status: types.StatusRunning,
		})

		// Create tasks with dead containers
		_ = store.Save(ctx, types.KeyTasks+"agent1/task-1", &types.TaskRecord{
			ID:              "task-1",
			DeploymentID:    depID,
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: "not_found",
			AgentID:         "agent1",
		})
		_ = store.Save(ctx, types.KeyTasks+"agent1/task-2", &types.TaskRecord{
			ID:              "task-2",
			DeploymentID:    depID,
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: "exited",
			AgentID:         "agent1",
		})

		srv.reconcileDeploymentStatus(ctx, depID)

		var record types.DeploymentRecord
		_ = store.Get(ctx, types.KeyDeployments+depID, &record)
		if record.Status != types.StatusStopped {
			t.Errorf("deployment status = %q, want stopped", record.Status)
		}

		// Check event was emitted
		recent := events.Recent(10)
		if len(recent) == 0 {
			t.Error("expected event to be emitted")
		}
	})

	t.Run("keeps deployment running when some containers alive", func(t *testing.T) {
		store := storage.NewMemoryStore()
		ctx := context.Background()

		srv := &engineGRPCServer{store: store}

		// Register the agent node so CollectDeploymentTasks can find tasks
		_ = store.Save(ctx, types.KeyNodes+"agent1", &types.NodeRecord{Name: "agent1"})

		depID := "dep-reconcile-2"
		_ = store.Save(ctx, types.KeyDeployments+depID, &types.DeploymentRecord{
			ID:     depID,
			Name:   "test-app",
			Status: types.StatusRunning,
		})

		_ = store.Save(ctx, types.KeyTasks+"agent1/task-1", &types.TaskRecord{
			ID:              "task-1",
			DeploymentID:    depID,
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusRunning,
			AgentID:         "agent1",
		})
		_ = store.Save(ctx, types.KeyTasks+"agent1/task-2", &types.TaskRecord{
			ID:              "task-2",
			DeploymentID:    depID,
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: "not_found",
			AgentID:         "agent1",
		})

		srv.reconcileDeploymentStatus(ctx, depID)

		var record types.DeploymentRecord
		_ = store.Get(ctx, types.KeyDeployments+depID, &record)
		if record.Status != types.StatusRunning {
			t.Errorf("deployment status = %q, want running (some containers still alive)", record.Status)
		}
	})

	t.Run("skips non-running deployments", func(t *testing.T) {
		store := storage.NewMemoryStore()
		ctx := context.Background()

		srv := &engineGRPCServer{store: store}

		depID := "dep-reconcile-3"
		_ = store.Save(ctx, types.KeyDeployments+depID, &types.DeploymentRecord{
			ID:     depID,
			Name:   "test-app",
			Status: types.StatusStopped,
		})

		// Even with dead containers, shouldn't change already-stopped deployment
		_ = store.Save(ctx, types.KeyTasks+"agent1/task-1", &types.TaskRecord{
			ID:              "task-1",
			DeploymentID:    depID,
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: "not_found",
			AgentID:         "agent1",
		})

		srv.reconcileDeploymentStatus(ctx, depID)

		var record types.DeploymentRecord
		_ = store.Get(ctx, types.KeyDeployments+depID, &record)
		if record.Status != types.StatusStopped {
			t.Errorf("deployment status = %q, want stopped (unchanged)", record.Status)
		}
	})
}

func TestRegister_ReturnsActiveContainers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns running containers from running deployments", func(t *testing.T) {
		memStore := storage.NewMemoryStore()
		srv := &engineGRPCServer{
			store:       memStore,
			registryURL: "localhost:5000",
			events:      NewEventBuffer(10),
		}

		// Running deployment
		memStore.Save(ctx, types.KeyDeployments+"deploy-1", &types.DeploymentRecord{
			ID:     "deploy-1",
			Name:   "myapp",
			Status: types.StatusRunning,
		})

		// Task from running deployment — should be returned
		memStore.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
			ID:              "task-1",
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusRunning,
			ContainerName:   "myapp-web-1",
			ContainerIP:     "10.0.1.2",
			Ports:           []string{"8080:80"},
			ServiceName:     "web",
			DeploymentID:    "deploy-1",
			AgentID:         "worker-1",
		})

		// Task with stopped container — should NOT be returned
		memStore.Save(ctx, types.KeyTasks+"worker-1/task-2", &types.TaskRecord{
			ID:              "task-2",
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusStopped,
			ContainerName:   "myapp-db-1",
			DeploymentID:    "deploy-1",
			AgentID:         "worker-1",
		})

		resp, err := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			ApiAddress:   "worker-1:9090",
			SessionToken: "token-abc",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		if len(resp.ActiveContainers) != 1 {
			t.Fatalf("expected 1 active container, got %d", len(resp.ActiveContainers))
		}

		ac := resp.ActiveContainers[0]
		if ac.ContainerName != "myapp-web-1" {
			t.Errorf("container name = %q, want myapp-web-1", ac.ContainerName)
		}
		if ac.ContainerIp != "10.0.1.2" {
			t.Errorf("container IP = %q, want 10.0.1.2", ac.ContainerIp)
		}
		if len(ac.Ports) != 1 || ac.Ports[0] != "8080:80" {
			t.Errorf("ports = %v, want [8080:80]", ac.Ports)
		}
		if ac.ServiceName != "web" {
			t.Errorf("service name = %q, want web", ac.ServiceName)
		}
		if ac.DeploymentId != "deploy-1" {
			t.Errorf("deployment ID = %q, want deploy-1", ac.DeploymentId)
		}
		if ac.TaskId != "task-1" {
			t.Errorf("task ID = %q, want task-1", ac.TaskId)
		}
	})

	t.Run("excludes containers from stopped deployments", func(t *testing.T) {
		memStore := storage.NewMemoryStore()
		srv := &engineGRPCServer{
			store:       memStore,
			registryURL: "localhost:5000",
			events:      NewEventBuffer(10),
		}

		// Stopped deployment
		memStore.Save(ctx, types.KeyDeployments+"deploy-old", &types.DeploymentRecord{
			ID:     "deploy-old",
			Name:   "myapp",
			Status: types.StatusStopped,
		})

		// Task from stopped deployment — should NOT be returned
		memStore.Save(ctx, types.KeyTasks+"worker-1/task-old", &types.TaskRecord{
			ID:              "task-old",
			Type:            types.TaskTypeCreateAndStart,
			Status:          types.StatusCompleted,
			ContainerStatus: types.StatusRunning,
			ContainerName:   "myapp-web-old",
			DeploymentID:    "deploy-old",
			AgentID:         "worker-1",
		})

		resp, err := srv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-1",
			ApiAddress:   "worker-1:9090",
			SessionToken: "token-abc",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		if len(resp.ActiveContainers) != 0 {
			t.Errorf("expected 0 active containers from stopped deployment, got %d", len(resp.ActiveContainers))
		}
	})

	t.Run("returns empty when no tasks exist", func(t *testing.T) {
		freshStore := storage.NewMemoryStore()
		freshSrv := &engineGRPCServer{
			store:       freshStore,
			registryURL: "localhost:5000",
			events:      NewEventBuffer(10),
		}

		resp, err := freshSrv.Register(ctx, &banyanpb.RegisterRequest{
			AgentName:    "worker-3",
			ApiAddress:   "worker-3:9090",
			SessionToken: "token-xyz",
		})
		if err != nil {
			t.Fatalf("Register failed: %v", err)
		}
		if len(resp.ActiveContainers) != 0 {
			t.Errorf("expected 0 active containers, got %d", len(resp.ActiveContainers))
		}
	})
}

func TestFindRunningDeploymentByName_TagFiltering(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	store.Save(ctx, types.KeyDeployments+"prod", &types.DeploymentRecord{
		ID: "prod", Name: "app", Status: types.StatusRunning, Tags: []string{"env:prod"},
		CreatedAt: time.Now(),
	})
	store.Save(ctx, types.KeyDeployments+"staging", &types.DeploymentRecord{
		ID: "staging", Name: "app", Status: types.StatusRunning, Tags: []string{"env:staging"},
		CreatedAt: time.Now(),
	})

	t.Run("matches correct tags", func(t *testing.T) {
		deploy, _, err := srv.findRunningDeploymentByName(ctx, "app", []string{"env:prod"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deploy.ID != "prod" {
			t.Errorf("expected 'prod', got %q", deploy.ID)
		}
	})

	t.Run("no match with wrong tags", func(t *testing.T) {
		_, _, err := srv.findRunningDeploymentByName(ctx, "app", []string{"env:dev"})
		if err == nil {
			t.Fatal("expected error when no deployment matches tags")
		}
	})

	t.Run("nil tags matches nil-tagged deployments only", func(t *testing.T) {
		store2 := storage.NewMemoryStore()
		srv2 := &engineGRPCServer{store: store2}
		store2.Save(ctx, types.KeyDeployments+"noTags", &types.DeploymentRecord{
			ID: "noTags", Name: "app", Status: types.StatusRunning, CreatedAt: time.Now(),
		})
		deploy, _, err := srv2.findRunningDeploymentByName(ctx, "app", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deploy.ID != "noTags" {
			t.Errorf("expected 'noTags', got %q", deploy.ID)
		}
	})
}

func TestGetRunningServiceNames_ExcludesStopTasks(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID: "task-1", DeploymentID: "deploy-1", AgentID: "agent-1",
		ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
	})
	store.Save(ctx, types.KeyTasks+"agent-1/task-1-stop", &types.TaskRecord{
		ID: "task-1-stop", DeploymentID: "deploy-1", AgentID: "agent-1",
		ServiceName: "web", Type: types.TaskTypeStopAndRemove, Status: types.StatusCompleted,
	})

	names := srv.getRunningServiceNames(ctx, "deploy-1")
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["web"] {
		t.Error("expected 'web' from create_and_start task")
	}
	if len(names) != 1 {
		t.Errorf("expected 1 name (stop task excluded), got %d: %v", len(names), names)
	}
}

func TestTeardownDeploymentServices_EmptyServiceList(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
	store.Save(ctx, types.KeyTasks+"agent-1/task-web", &types.TaskRecord{
		ID: "task-web", DeploymentID: "deploy-1", AgentID: "agent-1",
		ServiceName: "web", Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
		ContainerName: "app-web-0",
	})

	// Empty service list should create no stop tasks
	srv.teardownDeploymentServices(ctx, "deploy-1", []string{})

	var stopTask types.TaskRecord
	if err := store.Get(ctx, types.KeyTasks+"agent-1/task-web-stop", &stopTask); err == nil {
		t.Error("expected no stop task when service list is empty")
	}
}

func TestCollectServiceBackends_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("empty store returns nil", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		backends := srv.collectServiceBackends(ctx)
		if len(backends) != 0 {
			t.Errorf("expected 0 backends from empty store, got %d", len(backends))
		}
	})

	t.Run("no running deployments returns nil", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"d1", &types.DeploymentRecord{
			ID: "d1", Name: "app", Status: types.StatusStopped,
		})
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "d1", AgentID: "agent-1", ServiceName: "web",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-web-0", ContainerStatus: "running",
			ContainerIP: "10.0.1.5", Ports: []string{"8080:80"},
		})

		backends := srv.collectServiceBackends(ctx)
		if len(backends) != 0 {
			t.Errorf("expected 0 backends (deployment stopped), got %d", len(backends))
		}
	})

	t.Run("excludes containers without IP", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"d1", &types.DeploymentRecord{
			ID: "d1", Name: "app", Status: types.StatusRunning,
		})
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "d1", AgentID: "agent-1", ServiceName: "web",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-web-0", ContainerStatus: "running",
			ContainerIP: "", // no IP assigned yet
		})

		backends := srv.collectServiceBackends(ctx)
		if len(backends) != 0 {
			t.Errorf("expected 0 backends (no IP), got %d", len(backends))
		}
	})

	t.Run("excludes non-running containers", func(t *testing.T) {
		store := storage.NewMemoryStore()
		srv := &engineGRPCServer{store: store}

		store.Save(ctx, types.KeyDeployments+"d1", &types.DeploymentRecord{
			ID: "d1", Name: "app", Status: types.StatusRunning,
		})
		store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{Name: "agent-1", Status: "ready"})
		store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
			ID: "task-1", DeploymentID: "d1", AgentID: "agent-1", ServiceName: "web",
			Type: types.TaskTypeCreateAndStart, Status: types.StatusCompleted,
			ContainerName: "app-web-0", ContainerStatus: "exited",
			ContainerIP: "10.0.1.5",
		})

		backends := srv.collectServiceBackends(ctx)
		if len(backends) != 0 {
			t.Errorf("expected 0 backends (container exited), got %d", len(backends))
		}
	})
}

func TestRateLimiter(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := newRateLimiter(5, time.Minute)
		for i := 0; i < 5; i++ {
			if !rl.allow("10.0.0.1") {
				t.Fatalf("request %d should be allowed", i+1)
			}
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		rl := newRateLimiter(3, time.Minute)
		for i := 0; i < 3; i++ {
			rl.allow("10.0.0.1")
		}
		if rl.allow("10.0.0.1") {
			t.Error("4th request should be blocked")
		}
	})

	t.Run("separate limits per IP", func(t *testing.T) {
		rl := newRateLimiter(2, time.Minute)
		rl.allow("10.0.0.1")
		rl.allow("10.0.0.1")
		// IP 1 is at limit, IP 2 should still be allowed
		if !rl.allow("10.0.0.2") {
			t.Error("different IP should be allowed")
		}
		if rl.allow("10.0.0.1") {
			t.Error("first IP should be blocked")
		}
	})

	t.Run("expired entries are cleaned up", func(t *testing.T) {
		rl := newRateLimiter(2, 10*time.Millisecond)
		rl.allow("10.0.0.1")
		rl.allow("10.0.0.1")
		if rl.allow("10.0.0.1") {
			t.Error("should be blocked before window expires")
		}
		time.Sleep(15 * time.Millisecond)
		if !rl.allow("10.0.0.1") {
			t.Error("should be allowed after window expires")
		}
	})
}

func TestIsControlTunnelIP(t *testing.T) {
	t.Run("tunnel IP in range", func(t *testing.T) {
		if !isControlTunnelIP("10.200.0.1") {
			t.Error("expected 10.200.0.1 to be a control tunnel IP")
		}
	})

	t.Run("tunnel IP high range", func(t *testing.T) {
		if !isControlTunnelIP("10.200.255.254") {
			t.Error("expected 10.200.255.254 to be a control tunnel IP")
		}
	})

	t.Run("non-tunnel IP", func(t *testing.T) {
		if isControlTunnelIP("192.168.1.1") {
			t.Error("expected 192.168.1.1 to NOT be a control tunnel IP")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		if isControlTunnelIP("") {
			t.Error("expected empty string to NOT be a control tunnel IP")
		}
	})

	t.Run("invalid IP", func(t *testing.T) {
		if isControlTunnelIP("not-an-ip") {
			t.Error("expected invalid IP to NOT be a control tunnel IP")
		}
	})

	t.Run("adjacent subnet not in tunnel", func(t *testing.T) {
		if isControlTunnelIP("10.201.0.1") {
			t.Error("expected 10.201.0.1 to NOT be a control tunnel IP")
		}
	})
}

func TestTriggerSchedule(t *testing.T) {
	t.Run("nil channel is safe", func(t *testing.T) {
		s := &engineGRPCServer{scheduleCh: nil}
		// Should not panic
		s.triggerSchedule()
	})

	t.Run("sends on channel", func(t *testing.T) {
		ch := make(chan struct{}, 1)
		s := &engineGRPCServer{scheduleCh: ch}
		s.triggerSchedule()

		select {
		case <-ch:
			// success — signal was sent
		default:
			t.Error("expected triggerSchedule to send on scheduleCh")
		}
	})

	t.Run("non-blocking when channel is full", func(t *testing.T) {
		ch := make(chan struct{}, 1)
		ch <- struct{}{} // fill the channel
		s := &engineGRPCServer{scheduleCh: ch}

		// Should not block or panic
		s.triggerSchedule()

		// Channel should still have exactly one item
		select {
		case <-ch:
		default:
			t.Error("expected channel to still have one item")
		}
		// And now it should be empty
		select {
		case <-ch:
			t.Error("expected channel to be empty after draining one item")
		default:
		}
	})
}

// setupTestServerWithSecrets creates a test server with SecretsManager enabled.
func setupTestServerWithSecrets(t *testing.T) (banyanpb.EngineServiceClient, *engineGRPCServer, func()) {
	t.Helper()
	store := storage.NewMemoryStore()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	sm, err := NewSecretsManagerFromKey(store, key)
	if err != nil {
		t.Fatalf("NewSecretsManagerFromKey: %v", err)
	}

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		secrets:     sm,
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	client := banyanpb.NewEngineServiceClient(conn)
	cleanup := func() { conn.Close(); srv.Stop() }
	return client, engineSrv, cleanup
}

func TestSecretRPCs(t *testing.T) {
	client, srv, cleanup := setupTestServerWithSecrets(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("create secret", func(t *testing.T) {
		_, err := client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{
			Name:  "DB_PASSWORD",
			Value: []byte("secret123"),
		})
		if err != nil {
			t.Fatalf("CreateSecret: %v", err)
		}
	})

	t.Run("create duplicate updates", func(t *testing.T) {
		_, err := client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{
			Name:  "DB_PASSWORD",
			Value: []byte("updated"),
		})
		if err != nil {
			t.Fatalf("CreateSecret (update): %v", err)
		}
	})

	t.Run("create invalid name", func(t *testing.T) {
		_, err := client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{
			Name:  "bad-name",
			Value: []byte("value"),
		})
		if err == nil {
			t.Fatal("expected error for invalid name")
		}
	})

	t.Run("list secrets", func(t *testing.T) {
		resp, err := client.ListSecrets(ctx, &banyanpb.ListSecretsRequest{})
		if err != nil {
			t.Fatalf("ListSecrets: %v", err)
		}
		if len(resp.Secrets) != 1 {
			t.Errorf("expected 1 secret, got %d", len(resp.Secrets))
		}
		if resp.Secrets[0].Name != "DB_PASSWORD" {
			t.Errorf("expected DB_PASSWORD, got %s", resp.Secrets[0].Name)
		}
	})

	t.Run("get secret metadata", func(t *testing.T) {
		resp, err := client.GetSecret(ctx, &banyanpb.GetSecretRequest{Name: "DB_PASSWORD"})
		if err != nil {
			t.Fatalf("GetSecret: %v", err)
		}
		if resp.Name != "DB_PASSWORD" {
			t.Errorf("expected DB_PASSWORD, got %s", resp.Name)
		}
		if len(resp.Value) != 0 {
			t.Error("expected empty value without reveal")
		}
	})

	t.Run("get secret with reveal", func(t *testing.T) {
		resp, err := client.GetSecret(ctx, &banyanpb.GetSecretRequest{Name: "DB_PASSWORD", Reveal: true})
		if err != nil {
			t.Fatalf("GetSecret with reveal: %v", err)
		}
		if string(resp.Value) != "updated" {
			t.Errorf("expected 'updated', got %q", resp.Value)
		}
	})

	t.Run("get nonexistent secret", func(t *testing.T) {
		_, err := client.GetSecret(ctx, &banyanpb.GetSecretRequest{Name: "NOPE"})
		if err == nil {
			t.Fatal("expected error for nonexistent secret")
		}
	})

	t.Run("delete in-use secret blocked", func(t *testing.T) {
		// Create a running deployment that references the secret
		dep := &types.DeploymentRecord{
			ID:     "dep-1",
			Name:   "myapp",
			Status: types.StatusRunning,
			Services: map[string]types.ServiceRecord{
				"api": {Image: "nginx", Secrets: []string{"DB_PASSWORD"}},
			},
		}
		_ = srv.store.Save(ctx, types.KeyDeployments+"dep-1", dep)

		_, err := client.DeleteSecret(ctx, &banyanpb.DeleteSecretRequest{Name: "DB_PASSWORD"})
		if err == nil {
			t.Fatal("expected error for in-use secret")
		}

		// Clean up
		_ = srv.store.Delete(ctx, types.KeyDeployments+"dep-1")
	})

	t.Run("delete secret", func(t *testing.T) {
		_, err := client.DeleteSecret(ctx, &banyanpb.DeleteSecretRequest{Name: "DB_PASSWORD"})
		if err != nil {
			t.Fatalf("DeleteSecret: %v", err)
		}
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		_, err := client.DeleteSecret(ctx, &banyanpb.DeleteSecretRequest{Name: "GONE"})
		if err == nil {
			t.Fatal("expected error for nonexistent secret")
		}
	})
}

func TestSecretRPCs_NoSecretsManager(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("create without secrets enabled", func(t *testing.T) {
		_, err := client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{Name: "X", Value: []byte("v")})
		if err == nil {
			t.Fatal("expected FailedPrecondition error")
		}
	})

	t.Run("get without secrets enabled", func(t *testing.T) {
		_, err := client.GetSecret(ctx, &banyanpb.GetSecretRequest{Name: "X"})
		if err == nil {
			t.Fatal("expected FailedPrecondition error")
		}
	})

	t.Run("delete without secrets enabled", func(t *testing.T) {
		_, err := client.DeleteSecret(ctx, &banyanpb.DeleteSecretRequest{Name: "X"})
		if err == nil {
			t.Fatal("expected FailedPrecondition error")
		}
	})

	t.Run("list without secrets returns empty", func(t *testing.T) {
		resp, err := client.ListSecrets(ctx, &banyanpb.ListSecretsRequest{})
		if err != nil {
			t.Fatalf("ListSecrets: %v", err)
		}
		if len(resp.Secrets) != 0 {
			t.Errorf("expected 0 secrets, got %d", len(resp.Secrets))
		}
	})
}

func TestScale_RPC(t *testing.T) {
	client, srv, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	// Register an agent
	_, _ = client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:  "worker-1",
		ApiAddress: "worker-1:9090",
	})

	// Deploy an app
	_, _ = client.Deploy(ctx, &banyanpb.DeployRPCRequest{
		Manifest: &banyanpb.Manifest{
			Name: "myapp",
			Services: map[string]*banyanpb.ManifestService{
				"web": {Image: "nginx:alpine", Deploy: &banyanpb.ManifestDeploy{Replicas: 1}},
			},
		},
	})

	// Simulate deployment running with tasks
	keys, _ := srv.store.List(ctx, types.KeyDeployments)
	if len(keys) > 0 {
		var dep types.DeploymentRecord
		_ = srv.store.Get(ctx, keys[0], &dep)
		dep.Status = types.StatusRunning
		_ = srv.store.Save(ctx, keys[0], &dep)

		// Create a completed task so Scale knows current count
		task := &types.TaskRecord{
			ID: dep.ID + "-web-0", DeploymentID: dep.ID, ServiceName: "web",
			AgentID: "worker-1", Type: types.TaskTypeCreateAndStart,
			Status: types.StatusCompleted, Image: "nginx:alpine",
			ContainerName: "myapp-web-0",
		}
		_ = srv.store.Save(ctx, types.KeyTasks+"worker-1/"+task.ID, task)
	}

	t.Run("scale up", func(t *testing.T) {
		resp, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Name:     "myapp",
			Replicas: map[string]int32{"web": 3},
		})
		if err != nil {
			t.Fatalf("Scale: %v", err)
		}
		if resp.Previous["web"] != 1 {
			t.Errorf("expected previous=1, got %d", resp.Previous["web"])
		}
		if resp.Current["web"] != 3 {
			t.Errorf("expected current=3, got %d", resp.Current["web"])
		}
	})

	t.Run("negative replicas rejected", func(t *testing.T) {
		_, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Name:     "myapp",
			Replicas: map[string]int32{"web": -1},
		})
		if err == nil {
			t.Fatal("expected error for negative replicas")
		}
	})

	t.Run("exceeds max replicas", func(t *testing.T) {
		_, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Name:     "myapp",
			Replicas: map[string]int32{"web": int32(types.MaxReplicas + 1)}, //nolint:gosec // test value
		})
		if err == nil {
			t.Fatal("expected error for exceeding max replicas")
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		_, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Name:     "myapp",
			Replicas: map[string]int32{"db": 2},
		})
		if err == nil {
			t.Fatal("expected error for unknown service")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		_, err := client.Scale(ctx, &banyanpb.ScaleRequest{
			Replicas: map[string]int32{"web": 2},
		})
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})
}

func TestDeployValidatesAutoscaleBounds(t *testing.T) {
	client, _, cleanup := setupTestServer(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("min greater than max rejected", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "bad-autoscale",
				Services: map[string]*banyanpb.ManifestService{
					"api": {
						Image: "nginx",
						Deploy: &banyanpb.ManifestDeploy{
							Replicas:  1,
							Autoscale: &banyanpb.ManifestAutoscale{Min: 10, Max: 5},
						},
					},
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for min > max")
		}
	})

	t.Run("max exceeds MaxReplicas", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "bad-autoscale2",
				Services: map[string]*banyanpb.ManifestService{
					"api": {
						Image: "nginx",
						Deploy: &banyanpb.ManifestDeploy{
							Replicas:  1,
							Autoscale: &banyanpb.ManifestAutoscale{Min: 1, Max: int32(types.MaxReplicas + 1)}, //nolint:gosec // test value
						},
					},
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for max > MaxReplicas")
		}
	})
}

func TestDeployValidatesSecretRefs(t *testing.T) {
	client, _, cleanup := setupTestServerWithSecrets(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("missing secret rejected", func(t *testing.T) {
		_, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "needs-secret",
				Services: map[string]*banyanpb.ManifestService{
					"api": {Image: "nginx", Secrets: []string{"NONEXISTENT"}},
				},
			},
		})
		if err == nil {
			t.Fatal("expected error for missing secret reference")
		}
	})

	t.Run("existing secret accepted", func(t *testing.T) {
		// Create the secret first
		_, _ = client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{
			Name: "DB_PASS", Value: []byte("value"),
		})
		resp, err := client.Deploy(ctx, &banyanpb.DeployRPCRequest{
			Manifest: &banyanpb.Manifest{
				Name: "has-secret",
				Services: map[string]*banyanpb.ManifestService{
					"api": {Image: "nginx", Secrets: []string{"DB_PASS"}},
				},
			},
		})
		if err != nil {
			t.Fatalf("Deploy with valid secret: %v", err)
		}
		if resp.DeploymentId == "" {
			t.Error("expected deployment ID")
		}
	})
}

func TestPollTasksResolvesSecrets(t *testing.T) {
	client, srv, cleanup := setupTestServerWithSecrets(t)
	defer cleanup()
	ctx := context.Background()

	// Create secret
	_, _ = client.CreateSecret(ctx, &banyanpb.CreateSecretRequest{
		Name: "MY_SECRET", Value: []byte("secret-value"),
	})

	// Register agent
	_, _ = client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName: "agent-1", ApiAddress: "agent-1:9090",
	})

	// Create a pending task with secret refs
	task := &types.TaskRecord{
		ID: "task-1", AgentID: "agent-1", Type: types.TaskTypeCreateAndStart,
		Status: types.StatusPending, Image: "nginx", ContainerName: "app-0",
		SecretRefs: []string{"MY_SECRET"},
	}
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", task)

	// Poll tasks — should resolve secrets
	resp, err := client.PollTasks(ctx, &banyanpb.PollTasksRequest{AgentName: "agent-1"})
	if err != nil {
		t.Fatalf("PollTasks: %v", err)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
	}
	if resp.Tasks[0].ResolvedSecrets["MY_SECRET"] != "secret-value" {
		t.Errorf("expected resolved secret, got %v", resp.Tasks[0].ResolvedSecrets)
	}
}
