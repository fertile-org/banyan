package cmd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"github.com/fertile-org/banyan/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestRunDown_NoConfig(t *testing.T) {
	origConfig := configPath
	origName := downName
	t.Cleanup(func() {
		configPath = origConfig
		downName = origName
	})

	configPath = "/tmp/nonexistent-cli-config.yaml"
	downName = "my-app"

	err := runDown(downCmd, nil)
	if err == nil {
		t.Fatal("expected error when no config")
	}
}

func TestRunDown_NoNameOrFile(t *testing.T) {
	origName := downName
	origFile := downFile
	t.Cleanup(func() {
		downName = origName
		downFile = origFile
	})

	downName = ""
	downFile = ""

	err := runDown(downCmd, nil)
	if err == nil {
		t.Fatal("expected error when no name or file")
	}
}

func TestRunDown_WithServer(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	origName := downName
	origNoWait := downNoWait
	t.Cleanup(func() {
		downName = origName
		downNoWait = origNoWait
	})

	downName = "my-app"
	downNoWait = true // Don't wait for completion (would need a status change loop)

	err := runDown(downCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// emptyDownServer returns TaskCount=0 to test the "no services" path.
type emptyDownServer struct {
	banyanpb.UnimplementedEngineServiceServer
}

func (s *emptyDownServer) Down(_ context.Context, _ *banyanpb.DownRPCRequest) (*banyanpb.DownRPCResponse, error) {
	return &banyanpb.DownRPCResponse{TaskCount: 0}, nil
}

func (s *emptyDownServer) Health(_ context.Context, _ *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func TestRunDown_NoServicesFound(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &emptyDownServer{})
	go srv.Serve(lis)
	t.Cleanup(func() { srv.Stop() })

	setupCLITestConfig(t, lis.Addr().String())

	origName := downName
	origNoWait := downNoWait
	t.Cleanup(func() {
		downName = origName
		downNoWait = origNoWait
	})

	downName = "no-such-app"
	downNoWait = false

	err = runDown(downCmd, nil)
	if err != nil {
		t.Errorf("expected nil error for no services, got %v", err)
	}
}

func TestFindLatestDeployment(t *testing.T) {
	t.Run("returns most recent by CreatedAtUnix", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "old", Name: "my-app", Status: "failed", CreatedAtUnix: 1000},
			{Id: "new", Name: "my-app", Status: "stopping", CreatedAtUnix: 2000},
		}
		d := findLatestDeployment(deployments, "my-app", nil)
		if d == nil {
			t.Fatal("expected non-nil deployment")
		}
		if d.Id != "new" {
			t.Errorf("expected newest deployment, got %s", d.Id)
		}
	})

	t.Run("filters by name", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "other", Name: "other-app", Status: "failed", CreatedAtUnix: 3000},
			{Id: "mine", Name: "my-app", Status: "stopping", CreatedAtUnix: 1000},
		}
		d := findLatestDeployment(deployments, "my-app", nil)
		if d == nil {
			t.Fatal("expected non-nil deployment")
		}
		if d.Id != "mine" {
			t.Errorf("expected my-app deployment, got %s", d.Id)
		}
	})

	t.Run("returns nil when no match", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "other", Name: "other-app", Status: "running", CreatedAtUnix: 1000},
		}
		d := findLatestDeployment(deployments, "my-app", nil)
		if d != nil {
			t.Errorf("expected nil, got %v", d)
		}
	})

	t.Run("returns nil for empty list", func(t *testing.T) {
		d := findLatestDeployment(nil, "my-app", nil)
		if d != nil {
			t.Errorf("expected nil, got %v", d)
		}
	})
}

func TestRunDown_FromManifestFile(t *testing.T) {
	addr, cleanup := setupCLITestTCPServer(t)
	defer cleanup()

	setupCLITestConfig(t, addr)

	// Create a manifest file
	tmpDir := t.TempDir()
	manifestPath := tmpDir + "/banyan.yaml"
	if err := os.WriteFile(manifestPath, []byte("name: manifest-app\nservices:\n  web:\n    image: nginx\n"), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	origName := downName
	origFile := downFile
	origNoWait := downNoWait
	t.Cleanup(func() {
		downName = origName
		downFile = origFile
		downNoWait = origNoWait
	})

	downName = ""
	downFile = manifestPath
	downNoWait = true

	err := runDown(downCmd, nil)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestFindLatestDeployment_WithTags(t *testing.T) {
	t.Run("matches deployment with matching tags", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "d1", Name: "my-app", Tags: []string{"staging"}, CreatedAtUnix: 1000},
		}
		d := findLatestDeployment(deployments, "my-app", []string{"staging"})
		if d == nil {
			t.Fatal("expected non-nil deployment")
		}
		if d.Id != "d1" {
			t.Errorf("expected d1, got %s", d.Id)
		}
	})

	t.Run("excludes deployment with different tags", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "d1", Name: "my-app", Tags: []string{"staging"}, CreatedAtUnix: 1000},
		}
		d := findLatestDeployment(deployments, "my-app", []string{"production"})
		if d != nil {
			t.Errorf("expected nil, got %v", d)
		}
	})

	t.Run("nil tags matches all", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "d1", Name: "my-app", Tags: []string{"staging"}, CreatedAtUnix: 1000},
			{Id: "d2", Name: "my-app", Tags: []string{"production"}, CreatedAtUnix: 2000},
		}
		d := findLatestDeployment(deployments, "my-app", nil)
		if d == nil {
			t.Fatal("expected non-nil deployment")
		}
		if d.Id != "d2" {
			t.Errorf("expected most recent deployment d2, got %s", d.Id)
		}
	})

	t.Run("same name different tags returns correct one", func(t *testing.T) {
		deployments := []*banyanpb.DeploymentInfo{
			{Id: "d1", Name: "my-app", Tags: []string{"staging"}, CreatedAtUnix: 1000},
			{Id: "d2", Name: "my-app", Tags: []string{"production"}, CreatedAtUnix: 2000},
		}
		d := findLatestDeployment(deployments, "my-app", []string{"staging"})
		if d == nil {
			t.Fatal("expected non-nil deployment")
		}
		if d.Id != "d1" {
			t.Errorf("expected staging deployment d1, got %s", d.Id)
		}
	})
}

// downStatusServer returns a configurable deployment status for waitForDown tests.
type downStatusServer struct {
	banyanpb.UnimplementedEngineServiceServer
	status string
}

func (s *downStatusServer) GetStatus(_ context.Context, _ *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	return &banyanpb.GetStatusResponse{
		Deployments: []*banyanpb.DeploymentInfo{
			{Name: "my-app", Status: s.status, CreatedAtUnix: time.Now().Unix()},
		},
	}, nil
}

func setupDownBufconnClient(t *testing.T, status string) *EngineClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &downStatusServer{status: status})
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()
	t.Cleanup(func() { srv.Stop() })

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &EngineClient{
		conn:   conn,
		client: banyanpb.NewEngineServiceClient(conn),
	}
}

func TestWaitForDown_Stopped(t *testing.T) {
	origTags := downTags
	t.Cleanup(func() { downTags = origTags })
	downTags = nil

	client := setupDownBufconnClient(t, types.StatusStopped)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForDown(ctx, client, "my-app")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWaitForDown_Failed(t *testing.T) {
	origTags := downTags
	t.Cleanup(func() { downTags = origTags })
	downTags = nil

	client := setupDownBufconnClient(t, types.StatusFailed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForDown(ctx, client, "my-app")
	if err == nil {
		t.Fatal("expected error for failed down")
	}
	if !strings.Contains(err.Error(), "down failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWaitForDown_Timeout(t *testing.T) {
	origTags := downTags
	t.Cleanup(func() { downTags = origTags })
	downTags = nil

	// Use StatusRunning so it never reaches stopped
	client := setupDownBufconnClient(t, types.StatusRunning)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := waitForDown(ctx, client, "my-app")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("unexpected error: %v", err)
	}
}

// stoppedDownTCPServer returns StatusStopped so runDown's wait path completes.
type stoppedDownTCPServer struct {
	banyanpb.UnimplementedEngineServiceServer
}

func (s *stoppedDownTCPServer) Down(_ context.Context, _ *banyanpb.DownRPCRequest) (*banyanpb.DownRPCResponse, error) {
	return &banyanpb.DownRPCResponse{TaskCount: 2}, nil
}

func (s *stoppedDownTCPServer) GetStatus(_ context.Context, _ *banyanpb.GetStatusRequest) (*banyanpb.GetStatusResponse, error) {
	return &banyanpb.GetStatusResponse{
		Deployments: []*banyanpb.DeploymentInfo{
			{Name: "my-app", Status: types.StatusStopped, CreatedAtUnix: time.Now().Unix()},
		},
	}, nil
}

func (s *stoppedDownTCPServer) Health(_ context.Context, _ *banyanpb.HealthRequest) (*banyanpb.HealthResponse, error) {
	return &banyanpb.HealthResponse{Status: "ok"}, nil
}

func TestRunDown_WaitComplete(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	banyanpb.RegisterEngineServiceServer(srv, &stoppedDownTCPServer{})
	go srv.Serve(lis)
	t.Cleanup(func() { srv.Stop() })

	setupCLITestConfig(t, lis.Addr().String())

	origName := downName
	origNoWait := downNoWait
	origTags := downTags
	t.Cleanup(func() {
		downName = origName
		downNoWait = origNoWait
		downTags = origTags
	})

	downName = "my-app"
	downNoWait = false // exercise the wait path
	downTags = nil

	err = runDown(downCmd, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRunDown_ClientConnectFails(t *testing.T) {
	origConfig := configPath
	t.Cleanup(func() { configPath = origConfig })

	// Config with engine endpoint but no EngineWGPublicKey
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "banyan.yaml")
	cfg := types.BanyanConfig{
		CLI: types.CLIConfig{
			EngineHost:  "127.0.0.1",
			EnginePort:  "50051",
			WGPublicKey: "test-pubkey",
			// No EngineWGPublicKey → NewAutoEngineClient fails
		},
	}
	if err := types.SaveConfig(cfgPath, &cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	configPath = cfgPath

	origName := downName
	t.Cleanup(func() { downName = origName })
	downName = "my-app"

	err := runDown(downCmd, nil)
	if err == nil {
		t.Fatal("expected error when client connect fails")
	}
	if !strings.Contains(err.Error(), "failed to connect to engine") {
		t.Errorf("unexpected error: %v", err)
	}
}

