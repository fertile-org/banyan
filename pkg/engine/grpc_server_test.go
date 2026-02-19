package engine

import (
	"context"
	"net"
	"testing"
	"time"

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

const bufSize = 1024 * 1024

func setupTestServer(t *testing.T, password string) (banyanpb.EngineServiceClient, *engineGRPCServer, func()) {
	t.Helper()

	store := storage.NewMemoryStore()
	authProvider := &banyanrpc.PasswordAuthProvider{PasswordHash: banyanrpc.HashPassword(password)}

	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.AuthUnaryInterceptor(authProvider)),
	)

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
		grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: password}),
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
	client, _, cleanup := setupTestServer(t, "test-password")
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

func TestPasswordAuth(t *testing.T) {
	_, _, cleanup := setupTestServer(t, "correct-password")
	defer cleanup()

	// Create a client with wrong password
	lis := bufconn.Listen(bufSize)
	authProvider := &banyanrpc.PasswordAuthProvider{PasswordHash: banyanrpc.HashPassword("correct-password")}
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(banyanrpc.AuthUnaryInterceptor(authProvider)),
	)
	engineSrv := &engineGRPCServer{
		store:       storage.NewMemoryStore(),
		registryURL: "localhost:5000",
	}
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)
	go srv.Serve(lis)
	defer srv.Stop()

	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(&banyanrpc.PasswordCredentials{Password: "wrong-password"}),
	)
	defer conn.Close()

	wrongClient := banyanpb.NewEngineServiceClient(conn)

	_, err := wrongClient.Heartbeat(context.Background(), &banyanpb.HeartbeatRequest{
		AgentName: "test",
	})
	if err == nil {
		t.Fatal("expected error with wrong password")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("expected Unauthenticated, got %v", status.Code(err))
	}
}
