package agent

import (
	"context"
	"fmt"
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

const testBufSize = 1024 * 1024

// testToken is a fixed token used in tests.
const testToken = "test-auth-token-abc123"

// testTokenValidator validates tokens for testing.
type testTokenValidator struct {
	validHash string // sha256 of the valid token
}

func (v *testTokenValidator) ValidateToken(ctx context.Context, tokenHash string) error {
	if tokenHash != v.validHash {
		return status.Error(codes.Unauthenticated, "invalid auth token")
	}
	return nil
}

// setupEngineServer creates a bufconn-based engine gRPC server and an EngineClient connected to it.
func setupEngineServer(t *testing.T, token string) (*EngineClient, storage.StateStore, func()) {
	t.Helper()

	store := storage.NewMemoryStore()

	// Create a bcrypt hash for the password (used for ExchangeToken RPC)
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)

	validator := &testTokenValidator{validHash: banyanrpc.HashPassword(token)}

	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), validator)),
	)

	engineSrv := &testEngineServer{store: store, registryURL: "localhost:5000"}
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
		grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: token}),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}

	cleanup := func() {
		conn.Close()
		srv.Stop()
	}

	return client, store, cleanup
}

// testEngineServer is a minimal EngineService server for testing the EngineClient.
type testEngineServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store       storage.StateStore
	registryURL string
}

func (s *testEngineServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	node := &types.NodeRecord{
		Name:      req.AgentName,
		Status:    "ready",
		LastSeen:  time.Now(),
		CreatedAt: time.Now(),
	}
	s.store.Save(ctx, types.KeyNodes+req.AgentName, node)
	return &banyanpb.RegisterResponse{RegistryUrl: s.registryURL}, nil
}

func (s *testEngineServer) Heartbeat(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
	return &banyanpb.HeartbeatResponse{}, nil
}

func (s *testEngineServer) PollTasks(ctx context.Context, req *banyanpb.PollTasksRequest) (*banyanpb.PollTasksResponse, error) {
	taskPrefix := types.KeyTasks + req.AgentName + "/"
	keys, _ := s.store.List(ctx, taskPrefix)

	var tasks []*banyanpb.TaskRecord
	for _, key := range keys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Status == types.StatusPending {
			tasks = append(tasks, &banyanpb.TaskRecord{
				Id:            task.ID,
				DeploymentId:  task.DeploymentID,
				AgentId:       task.AgentID,
				Type:          task.Type,
				Status:        task.Status,
				Image:         task.Image,
				ContainerName: task.ContainerName,
			})
		}
	}
	return &banyanpb.PollTasksResponse{Tasks: tasks}, nil
}

func (s *testEngineServer) ReportTaskResult(ctx context.Context, req *banyanpb.ReportTaskResultRequest) (*banyanpb.ReportTaskResultResponse, error) {
	taskKey := types.KeyTasks + req.AgentId + "/" + req.TaskId
	var task types.TaskRecord
	if err := s.store.Get(ctx, taskKey, &task); err == nil {
		task.Status = req.Status
		s.store.Save(ctx, taskKey, &task)
	}
	return &banyanpb.ReportTaskResultResponse{}, nil
}

func (s *testEngineServer) ReportContainerHealth(ctx context.Context, req *banyanpb.ReportContainerHealthRequest) (*banyanpb.ReportContainerHealthResponse, error) {
	return &banyanpb.ReportContainerHealthResponse{}, nil
}

func (s *testEngineServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

// failingReportServer is a test server where ReportTaskResult always returns an error,
// but PollTasks works normally. Used to test the warning paths in processTasks.
type failingReportServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store       storage.StateStore
	registryURL string
}

func (s *failingReportServer) PollTasks(ctx context.Context, req *banyanpb.PollTasksRequest) (*banyanpb.PollTasksResponse, error) {
	taskPrefix := types.KeyTasks + req.AgentName + "/"
	keys, _ := s.store.List(ctx, taskPrefix)

	var tasks []*banyanpb.TaskRecord
	for _, key := range keys {
		var task types.TaskRecord
		if err := s.store.Get(ctx, key, &task); err != nil {
			continue
		}
		if task.Status == types.StatusPending {
			tasks = append(tasks, &banyanpb.TaskRecord{
				Id:            task.ID,
				DeploymentId:  task.DeploymentID,
				AgentId:       task.AgentID,
				Type:          task.Type,
				Status:        task.Status,
				Image:         task.Image,
				ContainerName: task.ContainerName,
			})
		}
	}
	return &banyanpb.PollTasksResponse{Tasks: tasks}, nil
}

func (s *failingReportServer) ReportTaskResult(ctx context.Context, req *banyanpb.ReportTaskResultRequest) (*banyanpb.ReportTaskResultResponse, error) {
	return nil, fmt.Errorf("report failed intentionally")
}

func (s *failingReportServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

// setupFailingReportServer creates a bufconn-based server where ReportTaskResult always fails.
func setupFailingReportServer(t *testing.T, token string) (*EngineClient, storage.StateStore, func()) {
	t.Helper()

	store := storage.NewMemoryStore()

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	validator := &testTokenValidator{validHash: banyanrpc.HashPassword(token)}

	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), validator)),
	)

	engineSrv := &failingReportServer{store: store, registryURL: "localhost:5000"}
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
		grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: token}),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}

	cleanup := func() {
		conn.Close()
		srv.Stop()
	}

	return client, store, cleanup
}

func TestNewEngineClient(t *testing.T) {
	// NewEngineClient creates a lazy gRPC connection (no actual network needed)
	client, err := NewEngineClient("localhost:50051", "test-token")
	if err != nil {
		t.Fatalf("NewEngineClient failed: %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("expected non-nil conn")
	}
	if client.client == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewEngineClientWithPassword(t *testing.T) {
	client, err := NewEngineClientWithPassword("localhost:50051", "test-password")
	if err != nil {
		t.Fatalf("NewEngineClientWithPassword failed: %v", err)
	}
	defer client.Close()

	if client.conn == nil {
		t.Error("expected non-nil conn")
	}
	if client.client == nil {
		t.Error("expected non-nil client")
	}
}

func TestEngineClient_Register(t *testing.T) {
	client, _, cleanup := setupEngineServer(t, testToken)
	defer cleanup()

	registryURL, vpcConfig, err := client.Register(context.Background(), "worker-1", "worker-1:50052", "token-abc", nil)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if registryURL != "localhost:5000" {
		t.Errorf("expected registry URL 'localhost:5000', got %q", registryURL)
	}
	// testEngineServer doesn't set VPC fields, so vpcConfig should be nil
	if vpcConfig != nil {
		t.Errorf("expected nil VPC config from test server, got %+v", vpcConfig)
	}
}

func TestEngineClient_Register_WithVPCConfig(t *testing.T) {
	store := storage.NewMemoryStore()

	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	validator := &testTokenValidator{validHash: banyanrpc.HashPassword(testToken)}

	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.NewAuthInterceptor(string(passwordHash), validator)),
	)

	// Use a server that returns VPC config
	engineSrv := &vpcTestEngineServer{
		store:           store,
		registryURL:     "localhost:5000",
		allocatedSubnet: "10.0.0.0/24",
		vpcCIDR:         "10.0.0.0/16",
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
		grpc.WithPerRPCCredentials(&banyanrpc.TokenCredentials{Token: testToken}),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer func() {
		conn.Close()
		srv.Stop()
	}()

	client := &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}

	registryURL, vpcConfig, registerErr := client.Register(context.Background(), "worker-1", "worker-1:50052", "token-abc", nil)
	if registerErr != nil {
		t.Fatalf("Register failed: %v", registerErr)
	}
	if registryURL != "localhost:5000" {
		t.Errorf("expected registry URL 'localhost:5000', got %q", registryURL)
	}
	if vpcConfig == nil {
		t.Fatal("expected non-nil VPC config")
	}
	if vpcConfig.AllocatedSubnet != "10.0.0.0/24" {
		t.Errorf("expected allocated subnet '10.0.0.0/24', got %q", vpcConfig.AllocatedSubnet)
	}
	if vpcConfig.VPCCIDR != "10.0.0.0/16" {
		t.Errorf("expected VPC CIDR '10.0.0.0/16', got %q", vpcConfig.VPCCIDR)
	}
}

// vpcTestEngineServer returns VPC config in Register responses.
type vpcTestEngineServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store           storage.StateStore
	registryURL     string
	allocatedSubnet string
	vpcCIDR         string
}

func (s *vpcTestEngineServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	return &banyanpb.RegisterResponse{
		RegistryUrl:     s.registryURL,
		AllocatedSubnet: s.allocatedSubnet,
		VpcCidr:         s.vpcCIDR,
	}, nil
}

func (s *vpcTestEngineServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func TestEngineClient_Heartbeat(t *testing.T) {
	client, _, cleanup := setupEngineServer(t, testToken)
	defer cleanup()

	peers, err := client.Heartbeat(context.Background(), "worker-1", "token-abc", nil)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	// testEngineServer returns empty response, so no peers expected
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestEngineClient_PollTasks(t *testing.T) {
	client, store, cleanup := setupEngineServer(t, testToken)
	defer cleanup()
	ctx := context.Background()

	t.Run("empty when no tasks", func(t *testing.T) {
		tasks, err := client.PollTasks(ctx, "worker-1")
		if err != nil {
			t.Fatalf("PollTasks failed: %v", err)
		}
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("returns pending tasks", func(t *testing.T) {
		store.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
			ID: "task-1", AgentID: "worker-1", Status: types.StatusPending,
			Type: types.TaskTypeCreateAndStart, Image: "nginx",
		})

		tasks, err := client.PollTasks(ctx, "worker-1")
		if err != nil {
			t.Fatalf("PollTasks failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(tasks))
		}
		if tasks[0].Id != "task-1" {
			t.Errorf("expected task-1, got %s", tasks[0].Id)
		}
	})
}

func TestEngineClient_ReportTaskResult(t *testing.T) {
	client, store, cleanup := setupEngineServer(t, testToken)
	defer cleanup()
	ctx := context.Background()

	store.Save(ctx, types.KeyTasks+"worker-1/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "worker-1", Status: types.StatusPending,
	})

	err := client.ReportTaskResult(ctx, "task-1", "worker-1", types.StatusCompleted, "", "myapp-web-0", nil)
	if err != nil {
		t.Fatalf("ReportTaskResult failed: %v", err)
	}

	var updated types.TaskRecord
	if err := store.Get(ctx, types.KeyTasks+"worker-1/task-1", &updated); err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if updated.Status != types.StatusCompleted {
		t.Errorf("expected completed, got %s", updated.Status)
	}
}

func TestEngineClient_ReportContainerHealth(t *testing.T) {
	client, _, cleanup := setupEngineServer(t, testToken)
	defer cleanup()

	err := client.ReportContainerHealth(context.Background(), "worker-1", []*banyanpb.ContainerStatus{
		{ContainerName: "myapp-web-0", Status: "running"},
	})
	if err != nil {
		t.Fatalf("ReportContainerHealth failed: %v", err)
	}
}

func TestEngineClient_Health(t *testing.T) {
	client, _, cleanup := setupEngineServer(t, testToken)
	defer cleanup()

	err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
}

func TestEngineClient_Close(t *testing.T) {
	client, _, cleanup := setupEngineServer(t, testToken)
	defer cleanup()

	// Close should not panic
	client.Close()

	// Close with nil conn should not panic
	nilClient := &EngineClient{}
	nilClient.Close()
}

func TestEngineClient_ErrorPaths(t *testing.T) {
	// Create a server and immediately stop it to trigger RPC errors.
	client, _, cleanup := setupEngineServer(t, testToken)
	cleanup() // Stop server immediately

	ctx := context.Background()

	t.Run("Register error", func(t *testing.T) {
		_, _, err := client.Register(ctx, "worker-1", "addr", "token", nil)
		if err == nil {
			t.Error("expected error from Register on stopped server")
		}
	})

	t.Run("Heartbeat error", func(t *testing.T) {
		_, err := client.Heartbeat(ctx, "worker-1", "token", nil)
		if err == nil {
			t.Error("expected error from Heartbeat on stopped server")
		}
	})

	t.Run("PollTasks error", func(t *testing.T) {
		_, err := client.PollTasks(ctx, "worker-1")
		if err == nil {
			t.Error("expected error from PollTasks on stopped server")
		}
	})

	t.Run("ReportTaskResult error", func(t *testing.T) {
		err := client.ReportTaskResult(ctx, "task-1", "worker-1", "completed", "", "container", nil)
		if err == nil {
			t.Error("expected error from ReportTaskResult on stopped server")
		}
	})

	t.Run("ReportContainerHealth error", func(t *testing.T) {
		err := client.ReportContainerHealth(ctx, "worker-1", []*banyanpb.ContainerStatus{
			{ContainerName: "web-0", Status: "running"},
		})
		if err == nil {
			t.Error("expected error from ReportContainerHealth on stopped server")
		}
	})

	t.Run("Health error", func(t *testing.T) {
		err := client.Health(ctx)
		if err == nil {
			t.Error("expected error from Health on stopped server")
		}
	})
}
