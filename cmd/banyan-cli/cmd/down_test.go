package cmd

import (
	"context"
	"net"
	"os"
	"testing"

	"github.com/fertile-org/banyan/pkg/rpc/banyanpb"
	"google.golang.org/grpc"
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

