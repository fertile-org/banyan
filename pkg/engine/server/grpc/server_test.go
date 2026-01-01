package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/fertile-org/banyan/pkg/engine/server/config"
)

// mockOrchestratorService is a mock implementation of OrchestratorService.
type mockOrchestratorService struct {
	registered bool
}

func (m *mockOrchestratorService) Register(server *grpc.Server) {
	m.registered = true
}

// mockStateService is a mock implementation of StateService.
type mockStateService struct {
	registered bool
}

func (m *mockStateService) Register(server *grpc.Server) {
	m.registered = true
}

// mockVPCService is a mock implementation of VPCService.
type mockVPCService struct {
	registered bool
}

func (m *mockVPCService) Register(server *grpc.Server) {
	m.registered = true
}

// mockAgentService is a mock implementation of AgentService.
type mockAgentService struct {
	registered bool
}

func (m *mockAgentService) Register(server *grpc.Server) {
	m.registered = true
}

func TestNewServer_DefaultConfig(t *testing.T) {
	server, err := NewServer(nil, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to be created")
	}

	if server.server == nil {
		t.Error("Expected gRPC server to be created")
	}

	if server.healthSrv == nil {
		t.Error("Expected health server to be created")
	}
}

func TestNewServer_WithConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":9999"

	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if server.config.Address != ":9999" {
		t.Errorf("Expected address ':9999', got '%s'", server.config.Address)
	}
}

func TestNewServer_RegistersServices(t *testing.T) {
	orchestrator := &mockOrchestratorService{}
	state := &mockStateService{}
	vpc := &mockVPCService{}
	agent := &mockAgentService{}

	services := &Services{
		Orchestrator: orchestrator,
		State:        state,
		VPC:          vpc,
		Agent:        agent,
	}

	_, err := NewServer(nil, services)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	if !orchestrator.registered {
		t.Error("Expected orchestrator service to be registered")
	}
	if !state.registered {
		t.Error("Expected state service to be registered")
	}
	if !vpc.registered {
		t.Error("Expected VPC service to be registered")
	}
	if !agent.registered {
		t.Error("Expected agent service to be registered")
	}
}

func TestServer_StartStop(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":0" // Use random available port

	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	if !server.IsRunning() {
		t.Error("Expected server to be running")
	}

	// Check address is set
	addr := server.Address()
	if addr == "" {
		t.Error("Expected address to be set")
	}

	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if server.IsRunning() {
		t.Error("Expected server to be stopped")
	}
}

func TestServer_StartTwice(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":0"

	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Start server
	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	// Try to start again
	err = server.Start()
	if err == nil {
		t.Error("Expected error when starting twice")
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Stop(ctx)
}

func TestServer_StopWithoutStart(t *testing.T) {
	server, err := NewServer(nil, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx := context.Background()
	err = server.Stop(ctx)
	if err != nil {
		t.Errorf("Stop() should not error when not running: %v", err)
	}
}

func TestServer_GetGRPCServer(t *testing.T) {
	server, err := NewServer(nil, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	grpcServer := server.GetGRPCServer()
	if grpcServer == nil {
		t.Error("Expected gRPC server to be returned")
	}
}

func TestServer_GetServices(t *testing.T) {
	services := &Services{
		Orchestrator: &mockOrchestratorService{},
	}

	server, err := NewServer(nil, services)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	got := server.GetServices()
	if got != services {
		t.Error("Expected same services to be returned")
	}
}

func TestServer_StopWithTimeout(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":0"

	server, err := NewServer(cfg, nil)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Start server
	go func() {
		_ = server.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	// Stop with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// The stop should complete (either gracefully or by timeout)
	_ = server.Stop(ctx)

	// Server should not be running after stop
	if server.IsRunning() {
		t.Error("Expected server to be stopped")
	}
}
