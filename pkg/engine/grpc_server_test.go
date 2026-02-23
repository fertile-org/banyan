package engine

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	banyanrpc "github.com/fertile-org/banyan/pkg/rpc"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// testTokenValidator validates tokens against a set of known valid hashes.
type testTokenValidator struct {
	validTokens map[string]bool
}

func (v *testTokenValidator) ValidateToken(ctx context.Context, tokenHash string) error {
	if !v.validTokens[tokenHash] {
		return status.Error(codes.Unauthenticated, "invalid auth token")
	}
	return nil
}

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

func setupTestServer(t *testing.T, _ string) (banyanpb.EngineServiceClient, *engineGRPCServer, func()) {
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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

		// Verify session token was stored
		if tok := srv.GetSessionToken("worker-1"); tok != "token-abc" {
			t.Errorf("expected session token 'token-abc', got %q", tok)
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

	t.Run("missing session token", func(t *testing.T) {
		_, err := client.Register(ctx, &banyanpb.RegisterRequest{
			AgentName: "worker-2",
		})
		if err == nil {
			t.Fatal("expected error for missing session_token")
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", status.Code(err))
		}
	})
}

func TestHeartbeat(t *testing.T) {
	client, srv, cleanup := setupTestServer(t, "test-password")
	defer cleanup()

	ctx := context.Background()

	// Register first
	client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "worker-1",
		ApiAddress:   "worker-1:9090",
		SessionToken: "old-token",
	})

	t.Run("updates session token", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
			AgentName:    "worker-1",
			SessionToken: "new-token",
		})
		if err != nil {
			t.Fatalf("Heartbeat failed: %v", err)
		}
		if tok := srv.GetSessionToken("worker-1"); tok != "new-token" {
			t.Errorf("expected session token 'new-token', got %q", tok)
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
}

func TestReportTaskResult(t *testing.T) {
	client, srv, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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

func TestGetSessionToken(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}

	t.Run("not found returns empty", func(t *testing.T) {
		tok := srv.GetSessionToken("nonexistent")
		if tok != "" {
			t.Errorf("expected empty token, got %q", tok)
		}
	})

	t.Run("returns stored token", func(t *testing.T) {
		srv.sessions.Store("agent-1", "token-xyz")
		tok := srv.GetSessionToken("agent-1")
		if tok != "token-xyz" {
			t.Errorf("expected token-xyz, got %q", tok)
		}
	})
}

func TestHeartbeat_NewNode(t *testing.T) {
	client, srv, cleanup := setupTestServer(t, "test-password")
	defer cleanup()
	ctx := context.Background()

	// Heartbeat for a node that doesn't exist yet — should create it
	_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "new-agent",
		SessionToken: "token-new",
	})
	if err != nil {
		t.Fatalf("Heartbeat for new node failed: %v", err)
	}

	// Verify node was created
	var node types.NodeRecord
	if getErr := srv.store.Get(ctx, types.KeyNodes+"new-agent", &node); getErr != nil {
		t.Fatalf("expected node to be created: %v", getErr)
	}
	if node.Status != "ready" {
		t.Errorf("expected status ready, got %s", node.Status)
	}
}

func TestReportContainerHealth_MissingAgent(t *testing.T) {
	client, _, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
	client, _, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
		client2, srv2, cleanup2 := setupTestServer(t, "test-password")
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
		client3, srv3, cleanup3 := setupTestServer(t, "test-password")
		defer cleanup3()

		srv3.store.Save(ctx, types.KeyDeployments+"deploy-inv", &types.DeploymentRecord{
			ID: "deploy-inv", Name: "inv-app", Status: types.StatusRunning,
			Services: map[string]types.ServiceRecord{"web": {Image: "nginx"}},
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
		client4, srv4, cleanup4 := setupTestServer(t, "test-password")
		defer cleanup4()

		srv4.store.Save(ctx, types.KeyDeployments+"deploy-norun", &types.DeploymentRecord{
			ID: "deploy-norun", Name: "norun-app", Status: types.StatusPending,
			Services: map[string]types.ServiceRecord{"web": {Image: "nginx"}},
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
		client5, srv5, cleanup5 := setupTestServer(t, "test-password")
		defer cleanup5()

		// Deployment failed during deploy (1 task failed) but has completed tasks
		srv5.store.Save(ctx, types.KeyDeployments+"deploy-err", &types.DeploymentRecord{
			ID: "deploy-err", Name: "err-app", Status: types.StatusFailed,
			Error: "1/5 tasks failed: failed to start container: name-store error",
			Services: map[string]types.ServiceRecord{"web": {Image: "nginx", Replicas: 1}},
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
	client, _, cleanup := setupTestServer(t, "test-password")
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
	client, _, cleanup := setupTestServer(t, "test-password")
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
					DependsOn:   []string{"db"},
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
		if len(svc.DependsOn) != 1 || svc.DependsOn[0] != "db" {
			t.Errorf("unexpected depends_on: %v", svc.DependsOn)
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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

func TestTokenAuth(t *testing.T) {
	const testToken = "valid-test-token"
	tokenHash := banyanrpc.HashPassword(testToken)
	validator := &testTokenValidator{validTokens: map[string]bool{tokenHash: true}}
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), validator)),
	)
	engineSrv := &engineGRPCServer{
		store:       storage.NewMemoryStore(),
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)
	go srv.Serve(lis)
	defer srv.Stop()

	t.Run("valid token succeeds", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: testToken}),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)
		_, err := client.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "test"})
		if err != nil {
			t.Fatalf("expected success with valid token, got: %v", err)
		}
	})

	t.Run("wrong token rejected", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: "wrong-token"}),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)
		_, err := client.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "test"})
		if err == nil {
			t.Fatal("expected error with wrong token")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("missing token rejected", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)
		_, err := client.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "test"})
		if err == nil {
			t.Fatal("expected error with no token")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})
}

func TestExchangeToken(t *testing.T) {
	store := storage.NewMemoryStore()
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)

	engineSrv := &engineGRPCServer{
		store:        store,
		registryURL:  "localhost:5000",
		passwordHash: string(passwordHash),
	}

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), engineSrv)),
	)
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)
	go srv.Serve(lis)
	defer srv.Stop()

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	t.Run("valid password returns token", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: "test-password"}),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)
		resp, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "agent-1", Role: "agent",
		})
		if err != nil {
			t.Fatalf("ExchangeToken failed: %v", err)
		}
		if resp.Token == "" {
			t.Error("expected non-empty token")
		}

		// Verify the returned token works for other RPCs
		tokenConn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: resp.Token}),
		)
		defer tokenConn.Close()

		tokenClient := banyanpb.NewEngineServiceClient(tokenConn)
		_, err = tokenClient.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "agent-1"})
		if err != nil {
			t.Fatalf("Heartbeat with exchanged token failed: %v", err)
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: "wrong-password"}),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)
		_, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "agent-2", Role: "agent",
		})
		if err == nil {
			t.Fatal("expected error with wrong password")
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("re-issue replaces old token", func(t *testing.T) {
		conn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: "test-password"}),
		)
		defer conn.Close()

		client := banyanpb.NewEngineServiceClient(conn)

		resp1, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "agent-reissue", Role: "agent",
		})
		if err != nil {
			t.Fatalf("first ExchangeToken failed: %v", err)
		}

		resp2, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "agent-reissue", Role: "agent",
		})
		if err != nil {
			t.Fatalf("second ExchangeToken failed: %v", err)
		}

		if resp1.Token == resp2.Token {
			t.Error("re-issued token should be different from original")
		}

		// Old token should no longer work
		oldConn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: resp1.Token}),
		)
		defer oldConn.Close()

		oldClient := banyanpb.NewEngineServiceClient(oldConn)
		_, err = oldClient.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "agent-reissue"})
		if err == nil {
			t.Fatal("expected error with old token after re-issue")
		}

		// New token should work
		newConn, _ := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(dialer),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: resp2.Token}),
		)
		defer newConn.Close()

		newClient := banyanpb.NewEngineServiceClient(newConn)
		_, err = newClient.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{AgentName: "agent-reissue"})
		if err != nil {
			t.Fatalf("Heartbeat with new token failed: %v", err)
		}
	})
}

func TestExchangeToken_CLITokenHasExpiry(t *testing.T) {
	store := storage.NewMemoryStore()
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)

	engineSrv := &engineGRPCServer{
		store:        store,
		registryURL:  "localhost:5000",
		passwordHash: string(passwordHash),
	}

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), engineSrv)),
	)
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)
	go srv.Serve(lis)
	defer srv.Stop()

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: "test-password"}),
	)
	defer conn.Close()

	client := banyanpb.NewEngineServiceClient(conn)

	t.Run("CLI token has expiry set", func(t *testing.T) {
		resp, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "cli-laptop", Role: "cli",
		})
		if err != nil {
			t.Fatalf("ExchangeToken failed: %v", err)
		}

		// Verify the stored record has an ExpiresAt set
		tokenHash := banyanrpc.HashPassword(resp.Token)
		var record types.TokenRecord
		if err := store.Get(context.Background(), types.KeyTokens+tokenHash, &record); err != nil {
			t.Fatalf("failed to get token record: %v", err)
		}
		if record.Name != "cli-laptop" {
			t.Errorf("expected name 'cli-laptop', got %q", record.Name)
		}
		if record.Role != "cli" {
			t.Errorf("expected role 'cli', got %q", record.Role)
		}
		if record.ExpiresAt.IsZero() {
			t.Error("expected non-zero ExpiresAt for CLI token")
		}
		// Expiry should be ~30 days from now (allow 1 minute tolerance)
		expectedExpiry := time.Now().Add(types.DefaultCLITokenTTL)
		if record.ExpiresAt.Before(expectedExpiry.Add(-time.Minute)) || record.ExpiresAt.After(expectedExpiry.Add(time.Minute)) {
			t.Errorf("ExpiresAt %v not within 1 minute of expected %v", record.ExpiresAt, expectedExpiry)
		}
	})

	t.Run("agent token has no expiry", func(t *testing.T) {
		resp, err := client.ExchangeToken(context.Background(), &banyanpb.ExchangeTokenRequest{
			Name: "worker-1", Role: "agent",
		})
		if err != nil {
			t.Fatalf("ExchangeToken failed: %v", err)
		}

		tokenHash := banyanrpc.HashPassword(resp.Token)
		var record types.TokenRecord
		if err := store.Get(context.Background(), types.KeyTokens+tokenHash, &record); err != nil {
			t.Fatalf("failed to get token record: %v", err)
		}
		if record.Name != "worker-1" {
			t.Errorf("expected name 'worker-1', got %q", record.Name)
		}
		if record.Role != "agent" {
			t.Errorf("expected role 'agent', got %q", record.Role)
		}
		if !record.ExpiresAt.IsZero() {
			t.Errorf("expected zero ExpiresAt for agent token, got %v", record.ExpiresAt)
		}
	})
}

func TestValidateToken_Expired(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}
	ctx := context.Background()

	// Store a token that expired 1 hour ago
	expiredRecord := &types.TokenRecord{
		Name:      "cli-expired",
		Role:      "cli",
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	store.Save(ctx, types.KeyTokens+"expired-hash", expiredRecord)

	err := srv.ValidateToken(ctx, "expired-hash")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != "token expired, run 'auth' to re-authenticate" {
		t.Errorf("unexpected error message: %q", status.Convert(err).Message())
	}
}

func TestValidateToken_Valid(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}
	ctx := context.Background()

	// Store a valid non-expired CLI token
	validRecord := &types.TokenRecord{
		Name:      "cli-valid",
		Role:      "cli",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	store.Save(ctx, types.KeyTokens+"valid-hash", validRecord)

	if err := srv.ValidateToken(ctx, "valid-hash"); err != nil {
		t.Fatalf("expected no error for valid token, got: %v", err)
	}
}

func TestValidateToken_AgentNeverExpires(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}
	ctx := context.Background()

	// Store an agent token (no expiry)
	agentRecord := &types.TokenRecord{
		Name: "worker-1",
		Role: "agent",
	}
	store.Save(ctx, types.KeyTokens+"agent-hash", agentRecord)

	if err := srv.ValidateToken(ctx, "agent-hash"); err != nil {
		t.Fatalf("expected no error for agent token, got: %v", err)
	}
}

func TestValidateToken_NotFound(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{store: store}
	ctx := context.Background()

	err := srv.ValidateToken(ctx, "nonexistent-hash")
	if err == nil {
		t.Fatal("expected error for nonexistent token")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
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
	client, _, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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
		ID:   "deploy-multi",
		Name: "multi-app",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1, Ports: []string{"80:80"}, Environment: []string{"ENV=prod"}, Command: []string{"nginx"}, DependsOn: []string{"db"}},
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
				if len(webSvc.DependsOn) != 1 || webSvc.DependsOn[0] != "db" {
					t.Errorf("expected depends_on [db], got %v", webSvc.DependsOn)
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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

func TestHeartbeat_EmptySessionToken(t *testing.T) {
	client, srv, cleanup := setupTestServer(t, "test-password")
	defer cleanup()

	ctx := context.Background()

	// Register an agent first
	client.Register(ctx, &banyanpb.RegisterRequest{
		AgentName:    "agent-keep-token",
		ApiAddress:   "agent-keep-token:9090",
		SessionToken: "original-token",
	})

	// Heartbeat with empty session token should not overwrite the existing token
	_, err := client.Heartbeat(ctx, &banyanpb.HeartbeatRequest{
		AgentName:    "agent-keep-token",
		SessionToken: "",
	})
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Session token should remain unchanged
	if tok := srv.GetSessionToken("agent-keep-token"); tok != "original-token" {
		t.Errorf("expected session token 'original-token', got %q", tok)
	}
}

func TestDeploy_WithBuildAndDeploy(t *testing.T) {
	client, srv, cleanup := setupTestServer(t, "test-password")
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
	client, srv, cleanup := setupTestServer(t, "test-password")
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

func TestHeartbeat_SaveError(t *testing.T) {
	memStore := storage.NewMemoryStore()
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
		Services: map[string]types.ServiceRecord{"web": {Image: "nginx"}},
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
	cleanup() // stop immediately

	// Give the server time to fully shut down
	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	_, err := streamAgentLogs(ctx, agentAddr, "token", "container", false, 10)
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

	// Store a session token for the agent
	lis := bufconn.Listen(bufSize)
	grpcSrv := grpc.NewServer()
	engineSrv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}
	engineSrv.sessions.Store("agent-follow", "session-token-123")
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
			"web": {Image: "nginx:v2", Replicas: 1, DependsOn: []string{"api"}},
			"api": {Image: "myapi:v2", Replicas: 1, DependsOn: []string{"db"}},
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
			"api": {Image: "myapi", Replicas: 1, DependsOn: []string{"db"}},
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
