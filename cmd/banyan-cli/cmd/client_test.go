package cmd

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const cliTestBufSize = 1024 * 1024

// cliTestServer is a minimal EngineService server for CLI client tests.
type cliTestServer struct {
	banyanpb.UnimplementedEngineServiceServer
}

func (s *cliTestServer) Deploy(_ context.Context, _ *banyanpb.DeployRPCRequest) (*banyanpb.DeployRPCResponse, error) {
	return &banyanpb.DeployRPCResponse{DeploymentId: "deploy-123", Status: types.StatusPending}, nil
}

func (s *cliTestServer) Down(_ context.Context, _ *banyanpb.DownRPCRequest) (*banyanpb.DownRPCResponse, error) {
	return &banyanpb.DownRPCResponse{TaskCount: 2}, nil
}

func (s *cliTestServer) GetStatus(_ context.Context, _ *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	return &banyanpb.GetStatusResponse{
		Agents: []*banyanpb.AgentInfo{{Name: "worker-1", Status: "ready", LastSeenUnix: time.Now().Unix()}},
		Deployments: []*banyanpb.DeploymentInfo{
			{
				Name: "my-app", Status: types.StatusRunning, Healthy: 1, Total: 1,
				Tasks: []*banyanpb.TaskInfo{
					{
						ServiceName:     "web",
						ContainerName:   "my-app-web-0",
						AgentId:         "worker-1",
						Type:            types.TaskTypeCreateAndStart,
						ContainerStatus: types.StatusRunning,
					},
				},
			},
		},
	}, nil
}

func (s *cliTestServer) GetInfo(_ context.Context, _ *banyanpb.GetInfoRequest) (*banyanpb.GetInfoResponse, error) {
	return &banyanpb.GetInfoResponse{RegistryUrl: "localhost:5000"}, nil
}

func (s *cliTestServer) Health(_ context.Context, _ *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func (s *cliTestServer) GetLogs(req *banyanpb.GetLogsRequest, stream grpc.ServerStreamingServer[banyanpb.GetLogsResponse]) error {
	_ = stream.Send(&banyanpb.GetLogsResponse{Data: []byte("log line 1\nlog line 2\n")})
	return nil
}

// setupCLITestTCPServer starts a gRPC server on a real TCP port for testing
// command runners (runStatus, runDown, etc.) that create their own EngineClient.
// It returns the server address and a cleanup function.
func setupCLITestTCPServer(t *testing.T) (addr string, cleanup func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &cliTestServer{})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	cleanup = func() {
		srv.Stop()
	}
	return lis.Addr().String(), cleanup
}

// setupCLITestConfig creates a config file pointing to the given engine address
// and sets the global configPath. Returns a cleanup function to restore the original.
func setupCLITestConfig(t *testing.T, engineAddr string) {
	t.Helper()

	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost: "127.0.0.1",
			EnginePort: "", // will be parsed from addr
		},
	}
	// Parse host:port from addr
	host, port, _ := net.SplitHostPort(engineAddr)
	cfg.CLI.EngineHost = host
	cfg.CLI.EnginePort = port
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath
}

func setupCLITestServer(t *testing.T) (*EngineClient, func()) {
	t.Helper()
	lis := bufconn.Listen(cliTestBufSize)
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &cliTestServer{})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
	return client, cleanup
}

func TestManifestToProto(t *testing.T) {
	t.Run("basic manifest", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name:    "my-app",
			Version: "1.0",
			Services: map[string]types.ManifestService{
				"web": {
					Image:       "nginx:alpine",
					Ports:       []string{"80:80"},
					Environment: []string{"FOO=bar"},
					Command:     []string{"nginx"},
					DependsOn:   []string{"db"},
				},
			},
		}

		proto := manifestToProto(manifest)
		if proto.Name != "my-app" {
			t.Errorf("expected name 'my-app', got %q", proto.Name)
		}
		if proto.Version != "1.0" {
			t.Errorf("expected version '1.0', got %q", proto.Version)
		}
		svc := proto.Services["web"]
		if svc == nil {
			t.Fatal("expected service 'web'")
		}
		if svc.Image != "nginx:alpine" {
			t.Errorf("expected image 'nginx:alpine', got %q", svc.Image)
		}
		if len(svc.Ports) != 1 || svc.Ports[0] != "80:80" {
			t.Errorf("unexpected ports: %v", svc.Ports)
		}
		if len(svc.Environment) != 1 || svc.Environment[0] != "FOO=bar" {
			t.Errorf("unexpected environment: %v", svc.Environment)
		}
		if len(svc.Command) != 1 || svc.Command[0] != "nginx" {
			t.Errorf("unexpected command: %v", svc.Command)
		}
		if len(svc.DependsOn) != 1 || svc.DependsOn[0] != "db" {
			t.Errorf("unexpected depends_on: %v", svc.DependsOn)
		}
	})

	t.Run("with build config", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"api": {
					Image: "my-api:latest",
					Build: &types.ManifestBuild{
						Context:    "./api",
						Dockerfile: "Dockerfile.prod",
					},
				},
			},
		}

		proto := manifestToProto(manifest)
		svc := proto.Services["api"]
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
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"web": {
					Image:  "nginx",
					Deploy: &types.ManifestDeploy{Replicas: 5},
				},
			},
		}

		proto := manifestToProto(manifest)
		svc := proto.Services["web"]
		if svc.Deploy == nil {
			t.Fatal("expected non-nil Deploy")
		}
		if svc.Deploy.Replicas != 5 {
			t.Errorf("expected 5 replicas, got %d", svc.Deploy.Replicas)
		}
	})

	t.Run("nil build and deploy", func(t *testing.T) {
		manifest := types.BanyanManifest{
			Name: "my-app",
			Services: map[string]types.ManifestService{
				"web": {Image: "nginx"},
			},
		}

		proto := manifestToProto(manifest)
		svc := proto.Services["web"]
		if svc.Build != nil {
			t.Error("expected nil Build")
		}
		if svc.Deploy != nil {
			t.Error("expected nil Deploy")
		}
	})
}

func TestGrpcLogStreamReader_Read(t *testing.T) {
	t.Run("reads data correctly", func(t *testing.T) {
		reader := &grpcLogStreamReader{
			buf: []byte("hello world"),
		}
		p := make([]byte, 5)
		n, err := reader.Read(p)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if n != 5 {
			t.Errorf("expected 5 bytes, got %d", n)
		}
		if string(p[:n]) != "hello" {
			t.Errorf("expected 'hello', got %q", string(p[:n]))
		}

		// Read remaining bytes (buffer is 5 bytes, so reads " worl")
		n, err = reader.Read(p)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if n != 5 {
			t.Errorf("expected 5 bytes, got %d", n)
		}
		if string(p[:n]) != " worl" {
			t.Errorf("expected ' worl', got %q", string(p[:n]))
		}

		// Read final byte "d"
		n, err = reader.Read(p)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		if n != 1 {
			t.Errorf("expected 1 byte, got %d", n)
		}
		if string(p[:n]) != "d" {
			t.Errorf("expected 'd', got %q", string(p[:n]))
		}
	})

	t.Run("Close returns nil", func(t *testing.T) {
		reader := &grpcLogStreamReader{}
		if err := reader.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	})
}

// Verify the interface is satisfied (compile-time check).
var _ banyanpb.EngineServiceClient = (banyanpb.EngineServiceClient)(nil)

func TestNewEngineClient(t *testing.T) {
	// NewEngineClient creates a lazy gRPC connection (no actual network needed)
	client, err := NewEngineClient("localhost:50051", "test-password")
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

func TestEngineClient_Deploy(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	manifest := types.BanyanManifest{
		Name: "my-app",
		Services: map[string]types.ManifestService{
			"web": {Image: "nginx:alpine"},
		},
	}

	resp, err := client.Deploy(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	if resp.DeploymentId != "deploy-123" {
		t.Errorf("expected deploy-123, got %s", resp.DeploymentId)
	}
	if resp.Status != types.StatusPending {
		t.Errorf("expected pending, got %s", resp.Status)
	}
}

func TestEngineClient_Down(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	resp, err := client.Down(context.Background(), "my-app", []string{"web"})
	if err != nil {
		t.Fatalf("Down failed: %v", err)
	}
	if resp.TaskCount != 2 {
		t.Errorf("expected 2 tasks, got %d", resp.TaskCount)
	}
}

func TestEngineClient_Status(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	resp, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(resp.Agents))
	}
}

func TestEngineClient_Info(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	resp, err := client.Info(context.Background())
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}
	if resp.RegistryUrl != "localhost:5000" {
		t.Errorf("expected localhost:5000, got %s", resp.RegistryUrl)
	}
}

func TestEngineClient_Health(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health failed: %v", err)
	}
}

func TestEngineClient_StreamLogs(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()

	t.Run("reads log data via stream", func(t *testing.T) {
		reader, err := client.StreamLogs(context.Background(), "myapp-web-0", false, 0)
		if err != nil {
			t.Fatalf("StreamLogs failed: %v", err)
		}
		defer reader.Close()

		// Use small buffer to exercise both stream Recv and buffer paths
		buf := make([]byte, 10)
		n, err := reader.Read(buf)
		if err != nil {
			t.Fatalf("first Read failed: %v", err)
		}
		if string(buf[:n]) != "log line 1" {
			t.Errorf("expected 'log line 1', got %q", string(buf[:n]))
		}

		// Second read should get data from internal buffer
		n, err = reader.Read(buf)
		if err != nil {
			t.Fatalf("second Read failed: %v", err)
		}
		if string(buf[:n]) != "\nlog line " {
			t.Errorf("expected '\\nlog line ', got %q", string(buf[:n]))
		}

		// Third read should get remaining buffer
		n, err = reader.Read(buf)
		if err != nil {
			t.Fatalf("third Read failed: %v", err)
		}
		if string(buf[:n]) != "2\n" {
			t.Errorf("expected '2\\n', got %q", string(buf[:n]))
		}

		// Fourth read should hit stream.Recv which returns EOF
		_, err = reader.Read(buf)
		if err != io.EOF {
			t.Errorf("expected io.EOF, got %v", err)
		}
	})
}

func TestEngineClient_Close(t *testing.T) {
	client, cleanup := setupCLITestServer(t)
	defer cleanup()
	client.Close() // Should not panic
}
