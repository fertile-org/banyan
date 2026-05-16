package engine

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb/banyanpbconnect"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
)

// setupConnectTestServer creates an httptest.Server serving the connect adapter over a
// real HTTP server so we can use the generated connect client.
func setupConnectTestServer(t *testing.T) (banyanpbconnect.EngineServiceClient, *engineGRPCServer, *httptest.Server) {
	t.Helper()

	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}

	adapter := &connectAdapter{srv: srv}
	mux := http.NewServeMux()
	path, handler := banyanpbconnect.NewEngineServiceHandler(adapter)
	mux.Handle(path, handler)

	ts := httptest.NewServer(mux)
	client := banyanpbconnect.NewEngineServiceClient(http.DefaultClient, ts.URL)
	return client, srv, ts
}

func TestConnectAdapter_Health(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	resp, err := client.Health(context.Background(), connect.NewRequest(&banyanpb.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
	if resp.Msg.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Msg.Status)
	}
}

func TestConnectAdapter_GetDashboardData(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Seed a node and deployment so the response is non-empty
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})
	_ = srv.store.Save(ctx, types.KeyDeployments+"myapp-1", &types.DeploymentRecord{
		ID:     "myapp-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		},
	})

	resp, err := client.GetDashboardData(ctx, connect.NewRequest(&banyanpb.GetDashboardDataRequest{}))
	if err != nil {
		t.Fatalf("GetDashboardData failed: %v", err)
	}
	if resp.Msg.Summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if resp.Msg.Summary.TotalAgents != 1 {
		t.Errorf("expected 1 agent, got %d", resp.Msg.Summary.TotalAgents)
	}
	if len(resp.Msg.Deployments) != 1 {
		t.Errorf("expected 1 deployment, got %d", len(resp.Msg.Deployments))
	}
}

func TestConnectAdapter_Scale(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Seed a running deployment with a service
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})
	_ = srv.store.Save(ctx, types.KeyDeployments+"myapp-1", &types.DeploymentRecord{
		ID:     "myapp-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1, Ports: []string{"80:80"}},
		},
	})
	// Seed a completed task for the existing replica
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID:            "task-1",
		DeploymentID:  "myapp-1",
		ServiceName:   "web",
		AgentID:       "agent-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusCompleted,
		Image:         "nginx",
		ContainerName: "myapp-web-1",
		Ports:         []string{"80:80"},
		ReplicaIndex:  0,
	})

	resp, err := client.Scale(ctx, connect.NewRequest(&banyanpb.ScaleRequest{
		Name:     "myapp",
		Replicas: map[string]int32{"web": 2},
	}))
	if err != nil {
		t.Fatalf("Scale failed: %v", err)
	}
	if resp.Msg.Current["web"] != 2 {
		t.Errorf("expected current web count 2, got %d", resp.Msg.Current["web"])
	}
}

func TestConnectAdapter_Down(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Seed a running deployment with a completed task
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})
	_ = srv.store.Save(ctx, types.KeyDeployments+"myapp-1", &types.DeploymentRecord{
		ID:     "myapp-1",
		Name:   "myapp",
		Status: types.StatusRunning,
		Services: map[string]types.ServiceRecord{
			"web": {Image: "nginx", Replicas: 1},
		},
	})
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID:            "task-1",
		DeploymentID:  "myapp-1",
		ServiceName:   "web",
		AgentID:       "agent-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusCompleted,
		ContainerName: "myapp-web-1",
	})

	resp, err := client.Down(ctx, connect.NewRequest(&banyanpb.DownRPCRequest{
		Name: "myapp",
	}))
	if err != nil {
		t.Fatalf("Down failed: %v", err)
	}
	if resp.Msg.TaskCount < 1 {
		t.Errorf("expected at least 1 stop task, got %d", resp.Msg.TaskCount)
	}
}

func TestConnectAdapter_GetStatus(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})

	resp, err := client.GetStatus(ctx, connect.NewRequest(&banyanpb.GetStatusRequest{}))
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if len(resp.Msg.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(resp.Msg.Agents))
	}
}

func TestConnectAdapter_GetInfo(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	resp, err := client.GetInfo(context.Background(), connect.NewRequest(&banyanpb.GetInfoRequest{}))
	if err != nil {
		t.Fatalf("GetInfo failed: %v", err)
	}
	if resp.Msg.RegistryUrl != "localhost:5000" {
		t.Errorf("expected registry URL 'localhost:5000', got %q", resp.Msg.RegistryUrl)
	}
}

func TestConnectAdapter_Register(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	resp, err := client.Register(context.Background(), connect.NewRequest(&banyanpb.RegisterRequest{
		AgentName:  "test-agent",
		ApiAddress: "127.0.0.1:8080",
	}))
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if resp.Msg.RegistryUrl != "localhost:5000" {
		t.Errorf("expected registry URL 'localhost:5000', got %q", resp.Msg.RegistryUrl)
	}
}

func TestConnectAdapter_Deploy(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Need at least one agent for scheduling
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})

	resp, err := client.Deploy(ctx, connect.NewRequest(&banyanpb.DeployRPCRequest{
		Manifest: &banyanpb.Manifest{
			Name: "testapp",
			Services: map[string]*banyanpb.ManifestService{
				"web": {Image: "nginx:latest"},
			},
		},
	}))
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if resp.Msg.DeploymentId == "" {
		t.Error("expected non-empty deployment ID")
	}
	if resp.Msg.Status != types.StatusPending {
		t.Errorf("expected status %q, got %q", types.StatusPending, resp.Msg.Status)
	}
}

func TestConnectAdapter_ListSecrets(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	// Secrets not enabled (no secrets manager), should return empty list
	resp, err := client.ListSecrets(context.Background(), connect.NewRequest(&banyanpb.ListSecretsRequest{}))
	if err != nil {
		t.Fatalf("ListSecrets failed: %v", err)
	}
	if len(resp.Msg.Secrets) != 0 {
		t.Errorf("expected empty secrets list, got %d", len(resp.Msg.Secrets))
	}
}

// connectErrorStore wraps a StateStore to return errors on all operations.
// Used to test error paths in adapter methods.
type connectErrorStore struct {
	storage.StateStore
}

func (e *connectErrorStore) List(_ context.Context, _ string) ([]string, error) {
	return nil, context.DeadlineExceeded
}

func (e *connectErrorStore) Get(_ context.Context, _ string, _ interface{}) error {
	return context.DeadlineExceeded
}

func setupConnectErrorServer(t *testing.T) (banyanpbconnect.EngineServiceClient, *httptest.Server) {
	t.Helper()

	srv := &engineGRPCServer{
		store:       &connectErrorStore{StateStore: storage.NewMemoryStore()},
		registryURL: "localhost:5000",
	}

	adapter := &connectAdapter{srv: srv}
	mux := http.NewServeMux()
	path, handler := banyanpbconnect.NewEngineServiceHandler(adapter)
	mux.Handle(path, handler)

	ts := httptest.NewServer(mux)
	client := banyanpbconnect.NewEngineServiceClient(http.DefaultClient, ts.URL)
	return client, ts
}

func TestConnectAdapter_ErrorPaths(t *testing.T) {
	client, ts := setupConnectErrorServer(t)
	defer ts.Close()

	ctx := context.Background()

	// GetStatus with erroring store
	_, err := client.GetStatus(ctx, connect.NewRequest(&banyanpb.GetStatusRequest{}))
	// GetStatus doesn't return store errors (ignores them), so this should succeed
	if err != nil {
		t.Logf("GetStatus with error store: %v (expected for some implementations)", err)
	}

	// GetInfo always succeeds (no store access)
	_, err = client.GetInfo(ctx, connect.NewRequest(&banyanpb.GetInfoRequest{}))
	if err != nil {
		t.Fatalf("GetInfo should always succeed: %v", err)
	}

	// Health always succeeds
	_, err = client.Health(ctx, connect.NewRequest(&banyanpb.HealthRequest{}))
	if err != nil {
		t.Fatalf("Health should always succeed: %v", err)
	}

	// GetDashboardData with erroring store (ignores store errors)
	_, err = client.GetDashboardData(ctx, connect.NewRequest(&banyanpb.GetDashboardDataRequest{}))
	if err != nil {
		t.Logf("GetDashboardData with error store: %v", err)
	}

	// ListSecrets with no secrets manager (returns empty)
	_, err = client.ListSecrets(ctx, connect.NewRequest(&banyanpb.ListSecretsRequest{}))
	if err != nil {
		t.Fatalf("ListSecrets should succeed when secrets disabled: %v", err)
	}
}

func TestCORSMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(inner, nil)

	t.Run("preflight request", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected CORS origin '*', got %q", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
			t.Error("expected Access-Control-Allow-Methods header")
		}
	})

	t.Run("normal request passes through", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("expected CORS origin '*', got %q", got)
		}
	})
}

func TestStartConnectAPI(t *testing.T) {
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use port 0 will fail (startConnectAPI uses a fixed addr format), so we test
	// with a real ephemeral port by checking it doesn't return an error.
	// We pick a high random port unlikely to conflict.
	err := startConnectAPI(ctx, srv, "0", nil)
	if err != nil {
		t.Fatalf("startConnectAPI failed: %v", err)
	}

	// Shutdown cleanly
	cancel()
}

func TestConnectAdapter_Heartbeat(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Register first, then heartbeat
	_, _ = client.Register(ctx, connect.NewRequest(&banyanpb.RegisterRequest{
		AgentName:  "agent-1",
		ApiAddress: "127.0.0.1:8080",
	}))

	resp, err := client.Heartbeat(ctx, connect.NewRequest(&banyanpb.HeartbeatRequest{
		AgentName: "agent-1",
	}))
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestConnectAdapter_PollTasks(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})

	resp, err := client.PollTasks(ctx, connect.NewRequest(&banyanpb.PollTasksRequest{
		AgentName: "agent-1",
	}))
	if err != nil {
		t.Fatalf("PollTasks failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestConnectAdapter_ReportTaskResult(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID:           "task-1",
		DeploymentID: "deploy-1",
		AgentID:      "agent-1",
		Type:         types.TaskTypeCreateAndStart,
		Status:       types.StatusPending,
	})

	resp, err := client.ReportTaskResult(ctx, connect.NewRequest(&banyanpb.ReportTaskResultRequest{
		AgentId: "agent-1",
		TaskId:  "task-1",
		Status:  types.StatusCompleted,
	}))
	if err != nil {
		t.Fatalf("ReportTaskResult failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestConnectAdapter_ReportContainerHealth(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:   "agent-1",
		Status: "ready", LastSeen: time.Now(),
	})

	resp, err := client.ReportContainerHealth(ctx, connect.NewRequest(&banyanpb.ReportContainerHealthRequest{
		AgentName: "agent-1",
	}))
	if err != nil {
		t.Fatalf("ReportContainerHealth failed: %v", err)
	}
	if resp.Msg == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestConnectAdapter_CreateSecret_NotEnabled(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	// Secrets not enabled, should fail with FailedPrecondition
	_, err := client.CreateSecret(context.Background(), connect.NewRequest(&banyanpb.CreateSecretRequest{
		Name:  "my-secret",
		Value: []byte("supersecret"),
	}))
	if err == nil {
		t.Fatal("expected error for secrets not enabled")
	}
}

func TestConnectAdapter_GetSecret_NotEnabled(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.GetSecret(context.Background(), connect.NewRequest(&banyanpb.GetSecretRequest{
		Name: "my-secret",
	}))
	if err == nil {
		t.Fatal("expected error for secrets not enabled")
	}
}

func TestConnectAdapter_DeleteSecret_NotEnabled(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.DeleteSecret(context.Background(), connect.NewRequest(&banyanpb.DeleteSecretRequest{
		Name: "my-secret",
	}))
	if err == nil {
		t.Fatal("expected error for secrets not enabled")
	}
}

func TestConnectAdapter_Deploy_InvalidRequest(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	// Deploy with nil manifest should fail
	_, err := client.Deploy(context.Background(), connect.NewRequest(&banyanpb.DeployRPCRequest{}))
	if err == nil {
		t.Fatal("expected error for nil manifest")
	}
}

func TestConnectAdapter_Down_MissingName(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.Down(context.Background(), connect.NewRequest(&banyanpb.DownRPCRequest{}))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestConnectAdapter_Scale_MissingName(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.Scale(context.Background(), connect.NewRequest(&banyanpb.ScaleRequest{}))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestConnectAdapter_ReportContainerHealth_MissingAgent(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.ReportContainerHealth(context.Background(), connect.NewRequest(&banyanpb.ReportContainerHealthRequest{}))
	if err == nil {
		t.Fatal("expected error for missing agent name")
	}
}

func TestConnectAdapter_Register_MissingName(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.Register(context.Background(), connect.NewRequest(&banyanpb.RegisterRequest{}))
	if err == nil {
		t.Fatal("expected error for missing agent name")
	}
}

func TestConnectAdapter_Heartbeat_UnknownAgent(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.Heartbeat(context.Background(), connect.NewRequest(&banyanpb.HeartbeatRequest{
		AgentName: "nonexistent",
	}))
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
}

func TestConnectAdapter_PollTasks_MissingAgent(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.PollTasks(context.Background(), connect.NewRequest(&banyanpb.PollTasksRequest{}))
	if err == nil {
		t.Fatal("expected error for missing agent name")
	}
}

func TestConnectAdapter_ReportTaskResult_MissingTaskID(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.ReportTaskResult(context.Background(), connect.NewRequest(&banyanpb.ReportTaskResultRequest{}))
	if err == nil {
		t.Fatal("expected error for missing task ID")
	}
}

func TestConnectAdapter_StopTask(t *testing.T) {
	client, srv, ts := setupConnectTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Seed a running task
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID:              "task-1",
		DeploymentID:    "deploy-1",
		ServiceName:     "web",
		AgentID:         "agent-1",
		Type:            types.TaskTypeCreateAndStart,
		Status:          types.StatusCompleted,
		ContainerStatus: types.StatusRunning,
		ContainerName:   "myapp-web-0",
	})

	resp, err := client.StopTask(ctx, connect.NewRequest(&banyanpb.StopTaskRequest{
		TaskId:  "task-1",
		AgentId: "agent-1",
	}))
	if err != nil {
		t.Fatalf("StopTask failed: %v", err)
	}
	if resp.Msg.StopTaskId != "task-1-stop" {
		t.Errorf("expected stop task ID 'task-1-stop', got %q", resp.Msg.StopTaskId)
	}
	if resp.Msg.Status != "stopping" {
		t.Errorf("expected status 'stopping', got %q", resp.Msg.Status)
	}
}

func TestConnectAdapter_StopTask_MissingFields(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	_, err := client.StopTask(context.Background(), connect.NewRequest(&banyanpb.StopTaskRequest{}))
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestConnectAdapter_GetLogs_MissingContainerName(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	// GetLogs with empty container name should fail
	stream, err := client.GetLogs(context.Background(), connect.NewRequest(&banyanpb.GetLogsRequest{}))
	if err != nil {
		// Some connect implementations return the error here
		return
	}
	// Otherwise the error comes when reading from the stream
	defer stream.Close()
	for stream.Receive() {
		// drain
	}
	if stream.Err() == nil {
		t.Error("expected error for missing container name")
	}
}

func TestConnectAdapter_GetLogs_ContainerNotFound(t *testing.T) {
	client, _, ts := setupConnectTestServer(t)
	defer ts.Close()

	stream, err := client.GetLogs(context.Background(), connect.NewRequest(&banyanpb.GetLogsRequest{
		ContainerName: "nonexistent",
	}))
	if err != nil {
		return // error returned upfront
	}
	defer stream.Close()
	for stream.Receive() {
		// drain
	}
	if stream.Err() == nil {
		t.Error("expected error for nonexistent container")
	}
}

func TestConnectAdapter_GetLogs_Success(t *testing.T) {
	// Start a mock agent gRPC server
	agentAddr, agentCleanup := startMockAgentServer(t, [][]byte{
		[]byte("line 1\n"),
		[]byte("line 2\n"),
	})
	defer agentCleanup()

	// Set up connect server with a task pointing to the mock agent
	store := storage.NewMemoryStore()
	srv := &engineGRPCServer{
		store:       store,
		registryURL: "localhost:5000",
	}

	ctx := context.Background()
	_ = srv.store.Save(ctx, types.KeyNodes+"agent-1", &types.NodeRecord{
		Name:       "agent-1",
		Status:     "ready",
		APIAddress: agentAddr,
		LastSeen:   time.Now(),
	})
	_ = srv.store.Save(ctx, types.KeyTasks+"agent-1/task-1", &types.TaskRecord{
		ID:            "task-1",
		DeploymentID:  "deploy-1",
		ServiceName:   "web",
		AgentID:       "agent-1",
		Type:          types.TaskTypeCreateAndStart,
		Status:        types.StatusCompleted,
		ContainerName: "myapp-web-1",
	})

	adapter := &connectAdapter{srv: srv}
	mux := http.NewServeMux()
	path, handler := banyanpbconnect.NewEngineServiceHandler(adapter)
	mux.Handle(path, handler)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	client := banyanpbconnect.NewEngineServiceClient(http.DefaultClient, ts.URL)
	stream, err := client.GetLogs(ctx, connect.NewRequest(&banyanpb.GetLogsRequest{
		ContainerName: "myapp-web-1",
	}))
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	defer stream.Close()

	var allData []byte
	for stream.Receive() {
		allData = append(allData, stream.Msg().Data...)
	}
	if err := stream.Err(); err != nil && err != io.EOF {
		t.Fatalf("stream error: %v", err)
	}
	if string(allData) != "line 1\nline 2\n" {
		t.Errorf("expected 'line 1\\nline 2\\n', got %q", string(allData))
	}
}
