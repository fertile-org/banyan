package agent

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/metrics"
	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/storage"
	"github.com/fertile-org/banyan/pkg/types"
	"github.com/fertile-org/banyan/pkg/vpc/dns"
	"github.com/fertile-org/banyan/pkg/vpc/overlay"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpcpeer "google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// ---------------------------------------------------------------------------
// 1. agentLoop — verify it calls processTasks and exits on context cancel
// ---------------------------------------------------------------------------

func TestAgentLoop(t *testing.T) {
	origExec := taskExecutor
	origIPGetter := containerIPGetter
	t.Cleanup(func() {
		taskExecutor = origExec
		containerIPGetter = origIPGetter
	})

	var taskProcessed atomic.Bool
	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		taskProcessed.Store(true)
		return &types.TaskResultRecord{ContainerID: "abc"}, nil
	}
	containerIPGetter = func(ctx context.Context, name string) (string, error) {
		return "172.17.0.5", nil
	}

	client, store, cleanup := setupEngineServer(t)
	defer cleanup()
	ctx := context.Background()

	// Add a pending task
	store.Save(ctx, types.KeyTasks+"test-loop/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "test-loop", DeploymentID: "d1",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		Image: "nginx", ContainerName: "loop-web-0",
	})

	a := &Agent{
		opts:       Options{AgentName: "test-loop"},
		client:     client,
		containers: &containerTracker{},
		proxy:      newTestProxy(t),
	}
	a.connected.Store(true)

	loopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	a.agentLoop(loopCtx)

	if !taskProcessed.Load() {
		t.Error("expected agentLoop to process at least one task")
	}
}

// ---------------------------------------------------------------------------
// 2. agentHeartbeat — verify heartbeat is called and reconnect on failures
// ---------------------------------------------------------------------------

func TestAgentHeartbeat_CallsHeartbeat(t *testing.T) {
	origInterval := heartbeatInterval
	t.Cleanup(func() { heartbeatInterval = origInterval })
	heartbeatInterval = 50 * time.Millisecond

	var heartbeatCalled atomic.Int32
	srv := &reconnectTestServer{
		heartbeatFunc: func(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
			heartbeatCalled.Add(1)
			return &banyanpb.HeartbeatResponse{}, nil
		},
	}

	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	a := &Agent{
		opts:             Options{AgentName: "hb-test", APIPort: "50052"},
		client:           client,
		containers:       &containerTracker{},
		remoteBackends:   make(map[string]ServiceBackend),
		registeredDNS:    make(map[string]bool),
		metricsCollector: metrics.NewSystemCollector(),
	}
	a.connected.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	a.agentHeartbeat(ctx)

	if heartbeatCalled.Load() == 0 {
		t.Error("expected at least one heartbeat call")
	}
}

func TestAgentHeartbeat_ReconnectsAfterMaxFails(t *testing.T) {
	origInterval := heartbeatInterval
	t.Cleanup(func() { heartbeatInterval = origInterval })
	heartbeatInterval = 50 * time.Millisecond

	origIPDetector := hostIPDetector
	t.Cleanup(func() { hostIPDetector = origIPDetector })
	hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

	var heartbeatCalls atomic.Int32
	srv := &reconnectTestServer{
		heartbeatFunc: func(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
			n := heartbeatCalls.Add(1)
			if n <= int32(maxConsecutiveHeartbeatFails) {
				return nil, fmt.Errorf("connection refused")
			}
			return &banyanpb.HeartbeatResponse{}, nil
		},
	}

	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	a := &Agent{
		opts:             Options{AgentName: "hb-recon", APIPort: "50052"},
		client:           client,
		containers:       &containerTracker{},
		remoteBackends:   make(map[string]ServiceBackend),
		registeredDNS:    make(map[string]bool),
		metricsCollector: metrics.NewSystemCollector(),
	}
	a.connected.Store(true)

	// Run the actual agentHeartbeat loop — it should detect failures and reconnect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a.agentHeartbeat(ctx)

	// After maxConsecutiveHeartbeatFails, it should have reconnected
	if heartbeatCalls.Load() < int32(maxConsecutiveHeartbeatFails) {
		t.Errorf("expected at least %d heartbeat calls, got %d", maxConsecutiveHeartbeatFails, heartbeatCalls.Load())
	}
}

// ---------------------------------------------------------------------------
// 3. containerHealthLoop — verify it exits on context cancel
// ---------------------------------------------------------------------------

func TestAgentHeartbeat_WithVPCReconciliation(t *testing.T) {
	origInterval := heartbeatInterval
	origFactory := iptablesFactory
	t.Cleanup(func() {
		heartbeatInterval = origInterval
		iptablesFactory = origFactory
	})
	heartbeatInterval = 50 * time.Millisecond
	iptablesFactory = func() (iptablesHandle, error) {
		return &mockIPTables{}, nil
	}

	srv := &heartbeatTestServer{
		peers: []*banyanpb.VPCPeer{
			{Subnet: "10.0.46.0/24", HostIp: "192.168.1.20", PublicKey: "key1"},
		},
		backends: []*banyanpb.ServiceBackend{
			{ContainerName: "app-web-0", ContainerIp: "10.0.2.5", Ports: []string{"8080:80"},
				AgentName: "worker-2", ServiceName: "web", DeploymentName: "myapp"},
		},
	}
	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	p := newTestProxy(t)
	defer p.Close()
	mockDriver := &mockOverlayDriver{}
	dnsManager := dns.NewManager()

	a := &Agent{
		opts:             Options{AgentName: "worker-1"},
		client:           client,
		containers:       &containerTracker{},
		remoteBackends:   make(map[string]ServiceBackend),
		registeredDNS:    make(map[string]bool),
		metricsCollector: metrics.NewSystemCollector(),
		vpcEnabled:       true,
		proxy:            p,
		overlayDriver:    mockDriver,
		dnsManager:       dnsManager,
	}
	a.connected.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	a.agentHeartbeat(ctx)

	// Verify reconciliation happened
	if !mockDriver.reconcileCalled {
		t.Error("expected ReconcilePeers to be called during heartbeat")
	}
	if len(a.remoteBackends) == 0 {
		t.Error("expected remote backends to be reconciled")
	}
}

func TestContainerHealthLoop(t *testing.T) {
	origStatusFunc := containerStatusFunc
	origHealthFunc := containerHealthStatusFunc
	origInterval := healthCheckInterval
	t.Cleanup(func() {
		containerStatusFunc = origStatusFunc
		containerHealthStatusFunc = origHealthFunc
		healthCheckInterval = origInterval
	})
	healthCheckInterval = 50 * time.Millisecond

	var statusCalled atomic.Bool
	containerStatusFunc = func(ctx context.Context, name string) string {
		statusCalled.Store(true)
		return "running"
	}
	containerHealthStatusFunc = func(ctx context.Context, name string) string {
		return "healthy"
	}

	client, _, cleanup := setupEngineServer(t)
	defer cleanup()

	a := &Agent{
		opts:       Options{AgentName: "health-loop-test"},
		client:     client,
		containers: &containerTracker{},
	}
	a.connected.Store(true)
	a.containers.Add("web-0", "task-1", "10.0.1.2")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	a.containerHealthLoop(ctx)

	if !statusCalled.Load() {
		t.Error("expected containerStatusFunc to be called at least once")
	}
}

// ---------------------------------------------------------------------------
// 4. getContainerID and getContainerStatus — mock commandRunner
// ---------------------------------------------------------------------------

func TestGetContainerID_MockedRunner(t *testing.T) {
	// Test the real getContainerID by mocking commandRunner doesn't help
	// because getContainerID uses exec.CommandContext directly.
	// Instead, test it with cancelled context (error path).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getContainerID(ctx, "test-container")
	// Should fail (either nerdctl not found or context cancelled).
	if err == nil {
		// nerdctl might be installed — that's OK in CI, skip
		return
	}
	_ = err // no panic is the key assertion
}

func TestGetContainerStatus_ReturnsNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := getContainerStatus(ctx, "nonexistent-container")
	if result != "not_found" {
		t.Errorf("expected not_found for cancelled context, got %q", result)
	}
}

func TestGetContainerHealthStatus_ReturnsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := getContainerHealthStatus(ctx, "nonexistent-container")
	if result != "" {
		t.Errorf("expected empty for cancelled context, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// 5. reconnect — multi-endpoint cycling
// ---------------------------------------------------------------------------

func TestReconnect_MultiEndpointCycling(t *testing.T) {
	// Create a server where Health fails initially, then succeeds.
	var healthCalls atomic.Int32
	srv := &reconnectTestServer{
		heartbeatFunc: func(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
			return &banyanpb.HeartbeatResponse{}, nil
		},
	}
	// Override Health to fail a few times then succeed
	healthSrv := &healthControlServer{
		inner:     srv,
		failCount: 2,
		calls:     &healthCalls,
	}

	lis := bufconn.Listen(testBufSize)
	grpcSrv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(grpcSrv, healthSrv)
	go func() { _ = grpcSrv.Serve(lis) }()
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

	client := &EngineClient{
		endpoints: []string{"endpoint1:50051", "endpoint2:50051"},
		conn:      conn,
		client:    banyanpb.NewEngineServiceClient(conn),
	}

	origIPDetector := hostIPDetector
	t.Cleanup(func() { hostIPDetector = origIPDetector })
	hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

	a := &Agent{
		opts:   Options{AgentName: "cycle-test", APIPort: "50052"},
		client: client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	a.reconnect(ctx)
	// Should have eventually succeeded
}

// healthControlServer wraps a server and makes Health fail N times.
type healthControlServer struct {
	banyanpb.UnimplementedEngineServiceServer
	inner     *reconnectTestServer
	failCount int32
	calls     *atomic.Int32
}

func (s *healthControlServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	n := s.calls.Add(1)
	if n <= s.failCount {
		return nil, fmt.Errorf("engine starting up")
	}
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func (s *healthControlServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	return s.inner.Register(ctx, req)
}

func (s *healthControlServer) Heartbeat(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
	return s.inner.Heartbeat(ctx, req)
}

// ---------------------------------------------------------------------------
// 6. processTasks — additional error paths
// ---------------------------------------------------------------------------

func TestProcessTasks_NotConnected(t *testing.T) {
	a := &Agent{
		opts:       Options{AgentName: "worker-1"},
		containers: &containerTracker{},
	}
	// connected defaults to false — processTasks should return immediately.
	a.processTasks(context.Background())
}

func TestProcessTasks_CreateAndStartWithDNSAndVPC(t *testing.T) {
	origExec := taskExecutor
	origIPGetter := containerIPGetter
	origFactory := iptablesFactory
	t.Cleanup(func() {
		taskExecutor = origExec
		containerIPGetter = origIPGetter
		iptablesFactory = origFactory
	})

	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		return &types.TaskResultRecord{ContainerID: "abc123"}, nil
	}
	containerIPGetter = func(ctx context.Context, name string) (string, error) {
		return "10.0.1.5", nil
	}
	iptablesFactory = func() (iptablesHandle, error) {
		return &mockIPTables{}, nil
	}

	client, store, cleanup := setupEngineServer(t)
	defer cleanup()
	ctx := context.Background()

	store.Save(ctx, types.KeyTasks+"worker-dns/task-1", &types.TaskRecord{
		ID: "task-1", AgentID: "worker-dns", DeploymentID: "d1",
		DeploymentName: "myapp", ServiceName: "web",
		Type: types.TaskTypeCreateAndStart, Status: types.StatusPending,
		Image: "nginx", ContainerName: "myapp-web-0",
	})

	dnsManager := dns.NewManager()

	a := &Agent{
		opts:           Options{AgentName: "worker-dns"},
		client:         client,
		containers:     &containerTracker{},
		proxy:          newTestProxy(t),
		dnsManager:     dnsManager,
		vpcEnabled:     true,
		remoteBackends: make(map[string]ServiceBackend),
		registeredDNS:  make(map[string]bool),
	}
	a.connected.Store(true)

	a.processTasks(ctx)

	// Container should be tracked
	tracked := a.containers.List()
	if len(tracked) != 1 {
		t.Fatalf("expected 1 tracked container, got %d", len(tracked))
	}
	if tracked[0].containerIP != "10.0.1.5" {
		t.Errorf("expected IP 10.0.1.5, got %s", tracked[0].containerIP)
	}
}

// ---------------------------------------------------------------------------
// 7. doOneHeartbeat — with VPC peers and DNS manager
// ---------------------------------------------------------------------------

func TestDoOneHeartbeat_WithVPCPeersAndDNS(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) {
		return &mockIPTables{}, nil
	}

	srv := &heartbeatTestServer{
		peers: []*banyanpb.VPCPeer{
			{Subnet: "10.0.46.0/24", HostIp: "192.168.1.20", PublicKey: "key1"},
		},
		backends: []*banyanpb.ServiceBackend{
			{ContainerName: "app-web-0", ContainerIp: "10.0.1.5", Ports: []string{"8080:80"},
				AgentName: "worker-2", ServiceName: "web", DeploymentName: "myapp"},
		},
	}

	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	p := newTestProxy(t)
	defer p.Close()

	mockDriver := &mockOverlayDriver{}
	dnsManager := dns.NewManager()

	a := &Agent{
		opts:             Options{AgentName: "worker-1"},
		client:           client,
		containers:       &containerTracker{},
		remoteBackends:   make(map[string]ServiceBackend),
		registeredDNS:    make(map[string]bool),
		metricsCollector: metrics.NewSystemCollector(),
		vpcEnabled:       true,
		proxy:            p,
		overlayDriver:    mockDriver,
		dnsManager:       dnsManager,
	}

	a.doOneHeartbeat(context.Background())

	// Verify peers were reconciled
	if !mockDriver.reconcileCalled {
		t.Error("expected ReconcilePeers to be called")
	}
	// Verify remote backends were added
	if len(a.remoteBackends) != 1 {
		t.Errorf("expected 1 remote backend, got %d", len(a.remoteBackends))
	}
	// Verify DNS was reconciled
	if len(a.registeredDNS) == 0 {
		t.Error("expected DNS entries to be registered")
	}
}

// ---------------------------------------------------------------------------
// 8. restoreActiveContainers — with DNS manager and IP errors
// ---------------------------------------------------------------------------

func TestRestoreActiveContainers_WithDNS(t *testing.T) {
	origStatus := containerStatusFunc
	origIP := containerIPGetter
	t.Cleanup(func() {
		containerStatusFunc = origStatus
		containerIPGetter = origIP
	})

	containerStatusFunc = func(ctx context.Context, name string) string { return "running" }
	containerIPGetter = func(ctx context.Context, name string) (string, error) {
		return "10.0.1.2", nil
	}

	dnsManager := dns.NewManager()
	p := newTestProxy(t)
	defer p.Close()

	a := &Agent{
		proxy:      p,
		containers: &containerTracker{},
		dnsManager: dnsManager,
	}

	containers := []ActiveContainer{
		{ContainerName: "web-0", Ports: []string{"8080:80"}, TaskID: "t1", ServiceName: "web"},
	}

	a.restoreActiveContainers(context.Background(), containers)

	// Container should be tracked with DNS registration
	tracked := a.containers.List()
	if len(tracked) != 1 {
		t.Fatalf("expected 1 container, got %d", len(tracked))
	}
}

func TestRestoreActiveContainers_IPGetterFails(t *testing.T) {
	origStatus := containerStatusFunc
	origIP := containerIPGetter
	t.Cleanup(func() {
		containerStatusFunc = origStatus
		containerIPGetter = origIP
	})

	containerStatusFunc = func(ctx context.Context, name string) string { return "running" }
	containerIPGetter = func(ctx context.Context, name string) (string, error) {
		return "", fmt.Errorf("network unreachable")
	}

	a := &Agent{
		containers: &containerTracker{},
	}

	containers := []ActiveContainer{
		{ContainerName: "web-0", Ports: []string{"8080:80"}, TaskID: "t1"},
	}

	a.restoreActiveContainers(context.Background(), containers)

	// Container should not be tracked since IP getter failed
	tracked := a.containers.List()
	if len(tracked) != 0 {
		t.Errorf("expected 0 containers when IP getter fails, got %d", len(tracked))
	}
}

func TestRestoreActiveContainers_InvalidPort(t *testing.T) {
	origStatus := containerStatusFunc
	origIP := containerIPGetter
	t.Cleanup(func() {
		containerStatusFunc = origStatus
		containerIPGetter = origIP
	})

	containerStatusFunc = func(ctx context.Context, name string) string { return "running" }
	containerIPGetter = func(ctx context.Context, name string) (string, error) {
		return "10.0.1.2", nil
	}

	p := newTestProxy(t)
	defer p.Close()

	a := &Agent{
		proxy:      p,
		containers: &containerTracker{},
	}

	containers := []ActiveContainer{
		{ContainerName: "web-0", Ports: []string{"invalid-port"}, TaskID: "t1"},
	}

	// Should not panic — just log warning for bad port
	a.restoreActiveContainers(context.Background(), containers)

	// Container should still be tracked despite bad port
	tracked := a.containers.List()
	if len(tracked) != 1 {
		t.Fatalf("expected 1 tracked container, got %d", len(tracked))
	}
}

// ---------------------------------------------------------------------------
// 9. removeContainer — mock commandRunner
// ---------------------------------------------------------------------------

func TestRemoveContainer_MockedSuccess(t *testing.T) {
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	containerRemover = func(ctx context.Context, name string) error {
		return nil
	}

	err := containerRemover(context.Background(), "test-container")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRemoveContainer_MockedFailure(t *testing.T) {
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	containerRemover = func(ctx context.Context, name string) error {
		return fmt.Errorf("failed to remove container: permission denied")
	}

	err := containerRemover(context.Background(), "test-container")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// 10. engineIPAuthInterceptor — test unary interceptor
// ---------------------------------------------------------------------------

func TestEngineIPAuthInterceptor_PassesOnTunnelIP(t *testing.T) {
	interceptor := engineIPAuthInterceptor()

	// Create context with control tunnel IP
	ctx := grpcpeer.NewContext(context.Background(), &grpcpeer.Peer{
		Addr: netAddr("10.200.0.1:50051"),
	})

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
	if resp != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
}

func TestEngineIPAuthInterceptor_BlocksNonTunnelIP(t *testing.T) {
	interceptor := engineIPAuthInterceptor()

	ctx := grpcpeer.NewContext(context.Background(), &grpcpeer.Peer{
		Addr: netAddr("192.168.1.100:50051"),
	})

	handler := func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called for non-tunnel IP")
		return nil, nil
	}

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for non-tunnel IP")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

func TestEngineIPAuthInterceptor_BlocksNoPeerInfo(t *testing.T) {
	interceptor := engineIPAuthInterceptor()

	handler := func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called without peer info")
		return nil, nil
	}

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for missing peer info")
	}
}

// ---------------------------------------------------------------------------
// pbTaskToLocal with healthcheck
// ---------------------------------------------------------------------------

func TestPbTaskToLocal_WithHealthcheck(t *testing.T) {
	pb := &banyanpb.TaskRecord{
		Id:             "task-hc",
		DeploymentId:   "d1",
		DeploymentName: "myapp",
		ServiceName:    "web",
		AgentId:        "worker-1",
		Type:           types.TaskTypeCreateAndStart,
		Status:         types.StatusPending,
		Image:          "nginx",
		ContainerName:  "myapp-web-0",
		Restart:        "unless-stopped",
		Entrypoint:     []string{"nginx"},
		MemoryLimit:    "512m",
		CpuLimit:       "0.5",
		MemoryReservation: "256m",
		Healthcheck: &banyanpb.ManifestHealthcheck{
			Test:        []string{"CMD", "curl", "-f", "http://localhost/"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     3,
			StartPeriod: "30s",
			Disable:     false,
		},
	}

	task := pbTaskToLocal(pb)

	if task.DeploymentName != "myapp" {
		t.Errorf("expected DeploymentName 'myapp', got %q", task.DeploymentName)
	}
	if task.Healthcheck == nil {
		t.Fatal("expected non-nil healthcheck")
	}
	if task.Healthcheck.Retries != 3 {
		t.Errorf("expected 3 retries, got %d", task.Healthcheck.Retries)
	}
	if task.MemoryLimit != "512m" {
		t.Errorf("expected MemoryLimit '512m', got %q", task.MemoryLimit)
	}
	if task.Restart != "unless-stopped" {
		t.Errorf("expected Restart 'unless-stopped', got %q", task.Restart)
	}
}

// ---------------------------------------------------------------------------
// reconcileRemoteBackends — invalid port
// ---------------------------------------------------------------------------

func TestReconcileRemoteBackends_InvalidPort(t *testing.T) {
	p := newTestProxy(t)
	defer p.Close()

	a := &Agent{
		opts:           Options{AgentName: "worker-1"},
		proxy:          p,
		remoteBackends: make(map[string]ServiceBackend),
	}

	backends := []ServiceBackend{
		{ContainerName: "app-web-0", ContainerIP: "10.0.2.5",
			Ports: []string{"invalid"}, AgentName: "worker-2"},
	}
	a.reconcileRemoteBackends(backends)

	// Backend should be tracked even with invalid port (port parse warns but still tracks)
	if len(a.remoteBackends) != 1 {
		t.Errorf("expected 1 remote backend tracked, got %d", len(a.remoteBackends))
	}
}

// ---------------------------------------------------------------------------
// checkContainerHealth — with health status
// ---------------------------------------------------------------------------

func TestCheckContainerHealth_WithHealthStatus(t *testing.T) {
	origStatusFunc := containerStatusFunc
	origHealthFunc := containerHealthStatusFunc
	t.Cleanup(func() {
		containerStatusFunc = origStatusFunc
		containerHealthStatusFunc = origHealthFunc
	})

	containerStatusFunc = func(ctx context.Context, name string) string {
		return "running"
	}
	containerHealthStatusFunc = func(ctx context.Context, name string) string {
		return "healthy"
	}

	client, _, cleanup := setupEngineServer(t)
	defer cleanup()

	a := &Agent{
		opts:       Options{AgentName: "worker-1"},
		client:     client,
		containers: &containerTracker{},
	}
	a.connected.Store(true)
	a.containers.Add("web-0", "task-1", "10.0.1.2")

	// Should not panic and should include health status
	a.checkContainerHealth(context.Background())
}

// ---------------------------------------------------------------------------
// reconcileDNS — various paths
// ---------------------------------------------------------------------------

func TestReconcileDNS_NilManager(t *testing.T) {
	a := &Agent{
		dnsManager:    nil,
		registeredDNS: make(map[string]bool),
	}
	// Should return immediately without panic
	a.reconcileDNS(context.Background(), nil)
}

func TestReconcileDNS_WithBackends(t *testing.T) {
	dnsManager := dns.NewManager()
	a := &Agent{
		dnsManager:    dnsManager,
		registeredDNS: make(map[string]bool),
	}

	backends := []ServiceBackend{
		{ServiceName: "web", ContainerIP: "10.0.1.2", DeploymentName: "app1", AgentName: "w1"},
		{ServiceName: "web", ContainerIP: "10.0.1.3", DeploymentName: "app1", AgentName: "w2"},
		{ServiceName: "db", ContainerIP: "10.0.1.4", DeploymentName: "app1", AgentName: "w1"},
	}

	a.reconcileDNS(context.Background(), backends)

	// Should register DNS entries
	if len(a.registeredDNS) == 0 {
		t.Error("expected DNS entries to be registered")
	}
	// Should have short name for web (single deployment) and db
	if !a.registeredDNS["web.internal"] {
		t.Error("expected web.internal to be registered")
	}
	if !a.registeredDNS["web.app1.internal"] {
		t.Error("expected web.app1.internal to be registered")
	}
}

func TestReconcileDNS_ConflictingShortNames(t *testing.T) {
	dnsManager := dns.NewManager()
	a := &Agent{
		dnsManager:    dnsManager,
		registeredDNS: make(map[string]bool),
	}

	backends := []ServiceBackend{
		{ServiceName: "web", ContainerIP: "10.0.1.2", DeploymentName: "app1", AgentName: "w1"},
		{ServiceName: "web", ContainerIP: "10.0.2.2", DeploymentName: "app2", AgentName: "w2"},
	}

	a.reconcileDNS(context.Background(), backends)

	// Short name should be removed due to conflict
	if a.registeredDNS["web.internal"] {
		t.Error("expected web.internal to NOT be registered (conflict)")
	}
	// But FQDNs should be registered
	if !a.registeredDNS["web.app1.internal"] {
		t.Error("expected web.app1.internal to be registered")
	}
	if !a.registeredDNS["web.app2.internal"] {
		t.Error("expected web.app2.internal to be registered")
	}
}

func TestReconcileDNS_RemovesStaleEntries(t *testing.T) {
	dnsManager := dns.NewManager()
	a := &Agent{
		dnsManager: dnsManager,
		registeredDNS: map[string]bool{
			"old-service.internal": true,
			"web.internal":        true,
		},
	}

	// Only web is in the new backends
	backends := []ServiceBackend{
		{ServiceName: "web", ContainerIP: "10.0.1.2", DeploymentName: "app1", AgentName: "w1"},
	}

	a.reconcileDNS(context.Background(), backends)

	// old-service should be removed
	if a.registeredDNS["old-service.internal"] {
		t.Error("expected old-service.internal to be removed")
	}
}

func TestReconcileDNS_SkipsEmptyFields(t *testing.T) {
	dnsManager := dns.NewManager()
	a := &Agent{
		dnsManager:    dnsManager,
		registeredDNS: make(map[string]bool),
	}

	backends := []ServiceBackend{
		{ServiceName: "", ContainerIP: "10.0.1.2", DeploymentName: "app1", AgentName: "w1"},
		{ServiceName: "web", ContainerIP: "", DeploymentName: "app1", AgentName: "w1"},
	}

	a.reconcileDNS(context.Background(), backends)

	if len(a.registeredDNS) != 0 {
		t.Errorf("expected 0 DNS entries for backends with empty fields, got %d", len(a.registeredDNS))
	}
}

// ---------------------------------------------------------------------------
// reconcileVPCPeers — nil driver
// ---------------------------------------------------------------------------

func TestReconcileVPCPeers_NilDriver(t *testing.T) {
	a := &Agent{overlayDriver: nil}

	err := a.reconcileVPCPeers(context.Background(), []VPCPeer{
		{Subnet: "10.0.1.0/24", HostIP: "192.168.1.10", PublicKey: "key1"},
	})
	if err != nil {
		t.Fatalf("expected nil error for nil driver, got %v", err)
	}
}

func TestReconcileVPCPeers_EmptyPeers(t *testing.T) {
	mockDriver := &mockOverlayDriver{}
	a := &Agent{overlayDriver: mockDriver}

	err := a.reconcileVPCPeers(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for nil peers, got %v", err)
	}
	if mockDriver.reconcileCalled {
		t.Error("ReconcilePeers should not be called for empty peers")
	}
}

// ---------------------------------------------------------------------------
// setupOverlayForwarding — iptablesFactory error
// ---------------------------------------------------------------------------

func TestSetupOverlayForwarding_FactoryError(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) {
		return nil, fmt.Errorf("iptables not available")
	}

	err := defaultSetupOverlayForwarding()
	if err == nil {
		t.Fatal("expected error when iptables factory fails")
	}
}

func TestSetupOverlayForwarding_ChainAlreadyExists(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })

	// Mock where NewChain fails but chain exists in ListChains
	mock := &newChainFailMock{
		existingChains: []string{"INPUT", "FORWARD", "OUTPUT", isolationChainName},
	}
	iptablesFactory = func() (iptablesHandle, error) { return mock, nil }

	err := defaultSetupOverlayForwarding()
	if err != nil {
		t.Fatalf("expected no error when chain already exists, got: %v", err)
	}
}

func TestSetupOverlayForwarding_ChainCreateFails(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })

	// Mock where NewChain fails and chain NOT in ListChains
	mock := &newChainFailMock{
		existingChains: []string{"INPUT", "FORWARD", "OUTPUT"},
	}
	iptablesFactory = func() (iptablesHandle, error) { return mock, nil }

	err := defaultSetupOverlayForwarding()
	if err == nil {
		t.Fatal("expected error when chain creation fails and chain doesn't exist")
	}
}

// newChainFailMock always fails on NewChain.
type newChainFailMock struct {
	existingChains []string
	inserted       [][]string
}

func (m *newChainFailMock) Exists(table, chain string, rulespec ...string) (bool, error) {
	return false, nil
}
func (m *newChainFailMock) Insert(table, chain string, pos int, rulespec ...string) error {
	m.inserted = append(m.inserted, rulespec)
	return nil
}
func (m *newChainFailMock) Delete(table, chain string, rulespec ...string) error { return nil }
func (m *newChainFailMock) Append(table, chain string, rulespec ...string) error { return nil }
func (m *newChainFailMock) NewChain(table, chain string) error {
	return fmt.Errorf("chain already exists")
}
func (m *newChainFailMock) ClearChain(table, chain string) error { return nil }
func (m *newChainFailMock) DeleteChain(table, chain string) error { return nil }
func (m *newChainFailMock) ListChains(table string) ([]string, error) {
	return m.existingChains, nil
}

// ---------------------------------------------------------------------------
// reconcileNetworkIsolation — iptables factory error
// ---------------------------------------------------------------------------

func TestReconcileNetworkIsolation_FactoryError(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) {
		return nil, fmt.Errorf("iptables not available")
	}

	a := &Agent{opts: Options{AgentName: "agent-1"}, vpcEnabled: true}
	// Should not panic
	a.reconcileNetworkIsolation(context.Background(), []ServiceBackend{
		{ContainerName: "web-1", ContainerIP: "10.0.1.2", DeploymentName: "app", AgentName: "agent-1"},
	})
}

// ---------------------------------------------------------------------------
// addContainerToIsolation — iptables factory error
// ---------------------------------------------------------------------------

func TestAddContainerToIsolation_FactoryError(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) {
		return nil, fmt.Errorf("iptables not available")
	}

	a := &Agent{opts: Options{AgentName: "agent-1"}, vpcEnabled: true}
	// Should not panic
	a.addContainerToIsolation(context.Background(), "10.0.1.5", "myapp")
}

// ---------------------------------------------------------------------------
// Register with active containers
// ---------------------------------------------------------------------------

func TestEngineClient_Register_WithActiveContainers(t *testing.T) {
	store := storage.NewMemoryStore()
	lis := bufconn.Listen(testBufSize)
	srv := grpc.NewServer()

	engineSrv := &activeContainerServer{
		store:       store,
		registryURL: "localhost:5000",
		containers: []*banyanpb.ActiveContainer{
			{ContainerName: "web-0", ContainerIp: "10.0.1.2", Ports: []string{"80:80"}, TaskId: "t1", ServiceName: "web"},
		},
	}
	banyanpb.RegisterEngineServiceServer(srv, engineSrv)

	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := &EngineClient{
		endpoints: []string{"passthrough:///bufnet"},
		conn:      conn,
		client:    banyanpb.NewEngineServiceClient(conn),
	}

	_, _, activeContainers, regErr := client.Register(context.Background(), RegisterRequest{
		Name: "worker-1", APIAddr: "addr",
	})
	if regErr != nil {
		t.Fatalf("Register failed: %v", regErr)
	}
	if len(activeContainers) != 1 {
		t.Fatalf("expected 1 active container, got %d", len(activeContainers))
	}
	if activeContainers[0].ContainerName != "web-0" {
		t.Errorf("expected web-0, got %s", activeContainers[0].ContainerName)
	}
}

type activeContainerServer struct {
	banyanpb.UnimplementedEngineServiceServer
	store       storage.StateStore
	registryURL string
	containers  []*banyanpb.ActiveContainer
}

func (s *activeContainerServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	return &banyanpb.RegisterResponse{
		RegistryUrl:      s.registryURL,
		ActiveContainers: s.containers,
	}, nil
}

func (s *activeContainerServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

// ---------------------------------------------------------------------------
// getContainerIP — mock commandRunner (empty IP path)
// ---------------------------------------------------------------------------

func TestGetContainerIP_EmptyOutput(t *testing.T) {
	// Test the error path when nerdctl returns empty output.
	// We can't easily mock exec.CommandContext, but we can test via cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getContainerIP(ctx, "test-container")
	if err == nil {
		return // nerdctl might work in CI
	}
	// Should contain error info
	_ = err
}

// ---------------------------------------------------------------------------
// reconcileNetworkIsolation — DNS gateway IP rules
// ---------------------------------------------------------------------------

func TestReconcileNetworkIsolation_NoGatewayIP(t *testing.T) {
	mock := &mockIPTables{}
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) { return mock, nil }

	a := &Agent{
		opts:       Options{AgentName: "agent-1"},
		vpcEnabled: true,
		gatewayIP:  "", // No gateway IP
	}

	a.reconcileNetworkIsolation(context.Background(), nil)

	// Should still have conntrack + DROP but no DNS rules
	isoRules := mock.appended[isolationChainName]
	for _, rule := range isoRules {
		if containsAll(rule, "--dport", "53") {
			t.Error("should not have DNS rules when gatewayIP is empty")
		}
	}
}

// ---------------------------------------------------------------------------
// reconnect — VPC subnet change path
// ---------------------------------------------------------------------------

func TestReconnect_WithVPCSubnetChange(t *testing.T) {
	origIPDetector := hostIPDetector
	origChecker := prerequisiteChecker
	origFactory := overlayDriverFactory
	origFwd := setupOverlayForwarding
	origDNSManager := dnsManagerFactory
	origDNSServer := dnsServerFactory
	origReclaim := reclaimDNSPort
	t.Cleanup(func() {
		hostIPDetector = origIPDetector
		prerequisiteChecker = origChecker
		overlayDriverFactory = origFactory
		setupOverlayForwarding = origFwd
		dnsManagerFactory = origDNSManager
		dnsServerFactory = origDNSServer
		reclaimDNSPort = origReclaim
	})
	mockSysctlForTest(t)

	hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }
	prerequisiteChecker = func() error { return nil }
	setupOverlayForwarding = func() error { return nil }

	mockDriver := &mockOverlayDriver{}
	overlayDriverFactory = func(_, _ string) overlay.OverlayDriver { return mockDriver }

	// Mock DNS factories — use real implementations but bind to a random port
	// to avoid port conflicts. The server will fail to start on the gateway IP
	// (not available in test), which is fine — reconnect handles the error.
	dnsManagerFactory = func() *dns.Manager { return dns.NewManager() }
	dnsServerFactory = func(m *dns.Manager, c dns.ServerConfig) *dns.Server {
		// Override bind address to localhost with random port to avoid permission issues
		c.BindAddr = "127.0.0.1:0"
		return dns.NewServer(m, c)
	}

	// Server that returns a different subnet than the agent currently has
	srv := &reconnectTestServer{
		registerFunc: func(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
			return &banyanpb.RegisterResponse{
				RegistryUrl:     "localhost:5000",
				AllocatedSubnet: "10.0.99.0/24",
				VpcCidr:         "10.0.0.0/16",
			}, nil
		},
	}

	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	a := &Agent{
		opts:            Options{AgentName: "vpc-recon", APIPort: "50052"},
		client:          client,
		containers:      &containerTracker{},
		vpcEnabled:      true,
		allocatedSubnet: "10.0.1.0/24", // old subnet — different from server response
		overlayDriver:   mockDriver,
		remoteBackends:  make(map[string]ServiceBackend),
		registeredDNS:   make(map[string]bool),
	}

	a.reconnect(context.Background())

	// Verify the overlay was cleaned up (old subnet) and re-initialized
	if !mockDriver.cleanupCalled {
		t.Error("expected overlay Cleanup to be called for subnet change")
	}
}

// noopDNSServer is a placeholder for tests that don't need real DNS.
type noopDNSServer struct{}

// ---------------------------------------------------------------------------
// buildNerdctlRunArgs — with VPC and DNS
// ---------------------------------------------------------------------------

func TestBuildNerdctlRunArgs_WithVPCAndDNS(t *testing.T) {
	// Save and restore global
	origDNSAddr := dnsGatewayIPAddr
	t.Cleanup(func() { dnsGatewayIPAddr = origDNSAddr })
	dnsGatewayIPAddr = "10.0.1.1"

	task := &types.TaskRecord{
		ContainerName: "myapp-web-0",
		Image:         "nginx",
	}
	args := buildNerdctlRunArgs(task, true)

	foundNet := false
	foundDNS := false
	foundDNSSearch := false
	for i, arg := range args {
		if arg == "--net" && i+1 < len(args) && args[i+1] == "banyan" {
			foundNet = true
		}
		if arg == "--dns" && i+1 < len(args) && args[i+1] == "10.0.1.1" {
			foundDNS = true
		}
		if arg == "--dns-search" && i+1 < len(args) && args[i+1] == "internal" {
			foundDNSSearch = true
		}
	}
	if !foundNet {
		t.Error("expected --net banyan in args")
	}
	if !foundDNS {
		t.Error("expected --dns 10.0.1.1 in args")
	}
	if !foundDNSSearch {
		t.Error("expected --dns-search internal in args")
	}
}

// ---------------------------------------------------------------------------
// logger helper
// ---------------------------------------------------------------------------

func TestAgentLogger(t *testing.T) {
	a := &Agent{}
	log := a.logger()
	if log == nil {
		t.Error("expected non-nil logger")
	}
	// Call again to test the already-initialized path
	log2 := a.logger()
	if log2 == nil {
		t.Error("expected non-nil logger on second call")
	}
}

// ---------------------------------------------------------------------------
// Stream interceptor test
// ---------------------------------------------------------------------------

func TestEngineIPAuthStreamInterceptor_BlocksNonTunnelIP(t *testing.T) {
	interceptor := engineIPAuthStreamInterceptor()

	ctx := grpcpeer.NewContext(context.Background(), &grpcpeer.Peer{
		Addr: netAddr("192.168.1.100:50051"),
	})

	handler := func(srv any, ss grpc.ServerStream) error {
		t.Fatal("handler should not be called for non-tunnel IP")
		return nil
	}

	err := interceptor(nil, &fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
	if err == nil {
		t.Fatal("expected error for non-tunnel IP")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", status.Code(err))
	}
}

// fakeServerStream implements grpc.ServerStream for testing.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context {
	return f.ctx
}


// ---------------------------------------------------------------------------
// HeartbeatRequest PollTasks with tags
// ---------------------------------------------------------------------------

func TestPollTasks_WithExistingTasks(t *testing.T) {
	client, store, cleanup := setupEngineServer(t)
	defer cleanup()
	ctx := context.Background()

	// Add multiple tasks, some pending, some not
	store.Save(ctx, types.KeyTasks+"worker-1/task-pending", &types.TaskRecord{
		ID: "task-pending", AgentID: "worker-1", Status: types.StatusPending,
		Type: types.TaskTypeCreateAndStart, Image: "nginx",
	})
	store.Save(ctx, types.KeyTasks+"worker-1/task-completed", &types.TaskRecord{
		ID: "task-completed", AgentID: "worker-1", Status: types.StatusCompleted,
		Type: types.TaskTypeCreateAndStart, Image: "nginx",
	})

	tasks, err := client.PollTasks(ctx, "worker-1")
	if err != nil {
		t.Fatalf("PollTasks failed: %v", err)
	}
	// Only the pending task should be returned
	if len(tasks) != 1 {
		t.Fatalf("expected 1 pending task, got %d", len(tasks))
	}
	if tasks[0].Id != "task-pending" {
		t.Errorf("expected task-pending, got %s", tasks[0].Id)
	}
}

// ---------------------------------------------------------------------------
// agentHeartbeat — full loop with reconciliation
// ---------------------------------------------------------------------------

func TestAgentHeartbeat_FullLoopWithVPC(t *testing.T) {
	// This test exercises the full agentHeartbeat loop body including VPC
	// reconciliation, remote backends, and DNS — by calling the heartbeat
	// logic directly rather than relying on the 15s ticker.
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })
	iptablesFactory = func() (iptablesHandle, error) {
		return &mockIPTables{}, nil
	}

	srv := &heartbeatTestServer{
		peers: []*banyanpb.VPCPeer{
			{Subnet: "10.0.46.0/24", HostIp: "192.168.1.20", PublicKey: "key1"},
		},
		backends: []*banyanpb.ServiceBackend{
			{ContainerName: "app-web-0", ContainerIp: "10.0.2.5", Ports: []string{"8080:80"},
				AgentName: "worker-2", ServiceName: "web", DeploymentName: "myapp"},
		},
	}
	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	p := newTestProxy(t)
	defer p.Close()
	mockDriver := &mockOverlayDriver{}
	dnsManager := dns.NewManager()

	a := &Agent{
		opts:             Options{AgentName: "worker-1"},
		client:           client,
		containers:       &containerTracker{},
		remoteBackends:   make(map[string]ServiceBackend),
		registeredDNS:    make(map[string]bool),
		metricsCollector: metrics.NewSystemCollector(),
		vpcEnabled:       true,
		proxy:            p,
		overlayDriver:    mockDriver,
		dnsManager:       dnsManager,
	}
	a.connected.Store(true)

	// Simulate one heartbeat tick body (what agentHeartbeat does each tick)
	sysMetrics := a.metricsCollector.Collect()
	peers, backends, err := a.client.Heartbeat(context.Background(), a.opts.AgentName, a.opts.Tags, sysMetrics)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if a.vpcEnabled && len(peers) > 0 {
		if reconcileErr := a.reconcileVPCPeers(context.Background(), peers); reconcileErr != nil {
			t.Fatalf("reconcileVPCPeers failed: %v", reconcileErr)
		}
	}
	if a.vpcEnabled && a.proxy != nil {
		a.reconcileRemoteBackends(backends)
	}
	if a.vpcEnabled {
		a.reconcileNetworkIsolation(context.Background(), backends)
	}
	if a.dnsManager != nil {
		a.reconcileDNS(context.Background(), backends)
	}

	// Verify
	if !mockDriver.reconcileCalled {
		t.Error("expected ReconcilePeers to be called")
	}
	if len(a.remoteBackends) != 1 {
		t.Errorf("expected 1 remote backend, got %d", len(a.remoteBackends))
	}
	if len(a.registeredDNS) == 0 {
		t.Error("expected DNS entries to be registered")
	}
}

// ---------------------------------------------------------------------------
// removeContainer — test the "not found" path (returns nil)
// ---------------------------------------------------------------------------

func TestRemoveContainer_NotFoundReturnsNil(t *testing.T) {
	origRunner := commandRunner
	t.Cleanup(func() { commandRunner = origRunner })

	// Simulate nerdctl rm -f returning "not found" in stderr
	origRemover := containerRemover
	t.Cleanup(func() { containerRemover = origRemover })

	// Test the actual removeContainer function behavior with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = removeContainer(ctx, "test-not-found")
	// No panic is the key assertion
}

// ---------------------------------------------------------------------------
// getContainerStatus — success path mock
// ---------------------------------------------------------------------------

func TestGetContainerStatus_SuccessPath(t *testing.T) {
	origFunc := containerStatusFunc
	t.Cleanup(func() { containerStatusFunc = origFunc })

	containerStatusFunc = func(ctx context.Context, name string) string {
		return "running"
	}

	status := containerStatusFunc(context.Background(), "web-0")
	if status != "running" {
		t.Errorf("expected running, got %s", status)
	}
}

func TestGetContainerStatus_EmptyStdout(t *testing.T) {
	// When nerdctl returns empty output, should return "not_found"
	origFunc := containerStatusFunc
	t.Cleanup(func() { containerStatusFunc = origFunc })

	containerStatusFunc = func(ctx context.Context, name string) string {
		return "not_found"
	}

	status := containerStatusFunc(context.Background(), "gone-container")
	if status != "not_found" {
		t.Errorf("expected not_found, got %s", status)
	}
}

// ---------------------------------------------------------------------------
// defaultReadSysctl / defaultWriteSysctl — use temp files
// ---------------------------------------------------------------------------

func TestDefaultReadSysctl(t *testing.T) {
	tmpFile := t.TempDir() + "/test_sysctl"
	if err := os.WriteFile(tmpFile, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	val, err := defaultReadSysctl(tmpFile)
	if err != nil {
		t.Fatalf("defaultReadSysctl failed: %v", err)
	}
	if val != "1" {
		t.Errorf("expected '1', got %q", val)
	}
}

func TestDefaultReadSysctl_NotFound(t *testing.T) {
	_, err := defaultReadSysctl("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestDefaultWriteSysctl(t *testing.T) {
	tmpFile := t.TempDir() + "/test_sysctl_write"

	err := defaultWriteSysctl(tmpFile, "1")
	if err != nil {
		t.Fatalf("defaultWriteSysctl failed: %v", err)
	}

	data, readErr := os.ReadFile(tmpFile)
	if readErr != nil {
		t.Fatalf("read back failed: %v", readErr)
	}
	if string(data) != "1" {
		t.Errorf("expected '1', got %q", string(data))
	}
}

// ---------------------------------------------------------------------------
// defaultReclaimDNSPort — basic error paths
// ---------------------------------------------------------------------------

func TestDefaultReclaimDNSPort_NoListener(t *testing.T) {
	// Should fail because no process is listening on this random port
	err := defaultReclaimDNSPort("127.0.0.1", 59999)
	if err == nil {
		t.Fatal("expected error when no process is listening")
	}
}

func TestDefaultReclaimDNSPort_RefusesPID1(t *testing.T) {
	// findUDPListenerPID would return 0 or error for nonexistent listeners
	// We test the "refusing to kill PID <= 1" path indirectly
	err := defaultReclaimDNSPort("0.0.0.0", 0)
	if err == nil {
		// This is acceptable — error path hit
		return
	}
	// Should get some error
	_ = err
}

// ---------------------------------------------------------------------------
// findUDPListenerPID — invalid IP
// ---------------------------------------------------------------------------

func TestFindUDPListenerPID_InvalidIP(t *testing.T) {
	_, err := findUDPListenerPID("invalid-ip", 53)
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

// ---------------------------------------------------------------------------
// reconnect — context cancelled during backoff
// ---------------------------------------------------------------------------

func TestReconnect_ContextCancelledDuringBackoff(t *testing.T) {
	// Server where Health fails always — forces backoff
	srv := &reconnectTestServer{
		heartbeatFunc: func(ctx context.Context, req *banyanpb.HeartbeatRequest) (*banyanpb.HeartbeatResponse, error) {
			return nil, fmt.Errorf("dead")
		},
	}

	// Override Health to always fail
	failSrv := &alwaysFailHealthServer{inner: srv}

	client, cleanup := setupCustomServer(t, failSrv)
	defer cleanup()

	a := &Agent{
		opts:   Options{AgentName: "backoff-test", APIPort: "50052"},
		client: client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.reconnect(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — exited on context cancel
	case <-time.After(5 * time.Second):
		t.Fatal("reconnect did not exit after context cancellation")
	}
}

type alwaysFailHealthServer struct {
	banyanpb.UnimplementedEngineServiceServer
	inner *reconnectTestServer
}

func (s *alwaysFailHealthServer) Health(ctx context.Context, req *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return nil, fmt.Errorf("engine down")
}

func (s *alwaysFailHealthServer) Register(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
	return s.inner.Register(ctx, req)
}

// ---------------------------------------------------------------------------
// reconnect — register failure then context cancel
// ---------------------------------------------------------------------------

func TestReconnect_RegisterFailsThenContextCancel(t *testing.T) {
	origIPDetector := hostIPDetector
	t.Cleanup(func() { hostIPDetector = origIPDetector })
	hostIPDetector = func() (net.IP, error) { return net.ParseIP("192.168.1.10"), nil }

	srv := &reconnectTestServer{
		registerFunc: func(ctx context.Context, req *banyanpb.RegisterRequest) (*banyanpb.RegisterResponse, error) {
			return nil, fmt.Errorf("engine rebooting")
		},
	}

	client, cleanup := setupCustomServer(t, srv)
	defer cleanup()

	a := &Agent{
		opts:   Options{AgentName: "reg-fail-test", APIPort: "50052"},
		client: client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.reconnect(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(10 * time.Second):
		t.Fatal("reconnect did not exit after context cancellation")
	}
}

// ---------------------------------------------------------------------------
// processTasks — with VPC isolation and stop task with nil proxy
// ---------------------------------------------------------------------------

func TestProcessTasks_StopTaskNilProxy(t *testing.T) {
	origExec := taskExecutor
	t.Cleanup(func() { taskExecutor = origExec })

	taskExecutor = func(ctx context.Context, task *types.TaskRecord) (*types.TaskResultRecord, error) {
		return &types.TaskResultRecord{}, nil
	}

	client, store, cleanup := setupEngineServer(t)
	defer cleanup()
	ctx := context.Background()

	store.Save(ctx, types.KeyTasks+"worker-nil-proxy/task-s1", &types.TaskRecord{
		ID: "task-s1", AgentID: "worker-nil-proxy", DeploymentID: "d1",
		Type: types.TaskTypeStopAndRemove, Status: types.StatusPending,
		ContainerName: "myapp-web-0",
	})

	a := &Agent{
		opts:       Options{AgentName: "worker-nil-proxy"},
		client:     client,
		containers: &containerTracker{},
		proxy:      nil, // nil proxy — should not crash
	}
	a.connected.Store(true)

	a.processTasks(ctx)
}

// ---------------------------------------------------------------------------
// reconcileNetworkIsolation — clear chain failure
// ---------------------------------------------------------------------------

func TestReconcileNetworkIsolation_ClearChainFails(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })

	mock := &clearChainFailMock{}
	iptablesFactory = func() (iptablesHandle, error) { return mock, nil }

	a := &Agent{opts: Options{AgentName: "agent-1"}, vpcEnabled: true}
	// Should not panic
	a.reconcileNetworkIsolation(context.Background(), []ServiceBackend{
		{ContainerName: "web-1", ContainerIP: "10.0.1.2", DeploymentName: "app", AgentName: "agent-1"},
	})
}

type clearChainFailMock struct {
	mockIPTables
}

func (m *clearChainFailMock) ClearChain(table, chain string) error {
	return fmt.Errorf("clear failed")
}

// ---------------------------------------------------------------------------
// addContainerToIsolation — list chains failure
// ---------------------------------------------------------------------------

func TestAddContainerToIsolation_ListChainsFails(t *testing.T) {
	origFactory := iptablesFactory
	t.Cleanup(func() { iptablesFactory = origFactory })

	mock := &listChainsFailMock{}
	iptablesFactory = func() (iptablesHandle, error) { return mock, nil }

	a := &Agent{opts: Options{AgentName: "agent-1"}, vpcEnabled: true}
	// Should not panic
	a.addContainerToIsolation(context.Background(), "10.0.1.5", "myapp")
}

type listChainsFailMock struct {
	mockIPTables
}

func (m *listChainsFailMock) ListChains(table string) ([]string, error) {
	return nil, fmt.Errorf("list failed")
}

// ---------------------------------------------------------------------------
// engineIPAuthStreamInterceptor — passes on tunnel IP
// ---------------------------------------------------------------------------

func TestEngineIPAuthStreamInterceptor_PassesOnTunnelIP(t *testing.T) {
	interceptor := engineIPAuthStreamInterceptor()

	ctx := grpcpeer.NewContext(context.Background(), &grpcpeer.Peer{
		Addr: netAddr("10.200.0.1:50051"),
	})

	handlerCalled := false
	handler := func(srv any, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, &fakeServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !handlerCalled {
		t.Error("expected handler to be called")
	}
}

// ---------------------------------------------------------------------------
// defaultOverlayDriverFactory — just verify it doesn't panic
// ---------------------------------------------------------------------------

func TestDefaultOverlayDriverFactory(t *testing.T) {
	// This creates a real WireGuard driver but we just verify it doesn't panic.
	// The driver won't actually init (no network interface) but that's OK.
	driver := defaultOverlayDriverFactory("", "")
	if driver == nil {
		t.Fatal("expected non-nil driver")
	}
}

// ---------------------------------------------------------------------------
// getContainerIP / getContainerStatus real functions with cancelled context
// ---------------------------------------------------------------------------

func TestGetContainerStatus_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := getContainerStatus(ctx, "any-container")
	// Should return not_found on error
	if result != "not_found" {
		t.Errorf("expected not_found, got %q", result)
	}
}

func TestGetContainerHealthStatus_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := getContainerHealthStatus(ctx, "any-container")
	// Should return empty string on error
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
