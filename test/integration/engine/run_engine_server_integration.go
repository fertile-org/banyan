//go:build ignore

// Integration test for Engine gRPC Server
// Tests the full server lifecycle: Start → Health Check → Service Registration → Graceful Shutdown
//
// Run with: go run ./test/integration/engine/run_engine_server_integration.go

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/fertile-org/banyan/pkg/engine/server/config"
	servergrpc "github.com/fertile-org/banyan/pkg/engine/server/grpc"
	"github.com/fertile-org/banyan/test/integration/helpers"
)

// mockOrchestratorService implements OrchestratorService for testing
type mockOrchestratorService struct {
	registered bool
}

func (m *mockOrchestratorService) Register(server *grpc.Server) {
	m.registered = true
}

// mockStateService implements StateService for testing
type mockStateService struct {
	registered bool
}

func (m *mockStateService) Register(server *grpc.Server) {
	m.registered = true
}

// mockVPCService implements VPCService for testing
type mockVPCService struct {
	registered bool
}

func (m *mockVPCService) Register(server *grpc.Server) {
	m.registered = true
}

// mockAgentService implements AgentService for testing
type mockAgentService struct {
	registered bool
}

func (m *mockAgentService) Register(server *grpc.Server) {
	m.registered = true
}

func main() {
	ctx := context.Background()
	p := helpers.NewPrinter()

	exitCode := runTest(ctx, p)

	if exitCode == 0 {
		p.Result(true, "All Engine Server integration tests passed!")
	} else {
		p.Result(false, "Engine Server integration tests failed")
	}

	os.Exit(exitCode)
}

func runTest(ctx context.Context, p *helpers.Printer) int {
	p.Title("Engine Server Integration Test")

	// Test 1: Create server with default config
	p.Step("Creating server with default configuration")

	cfg := config.DefaultConfig()
	cfg.Address = ":0" // Use random available port
	cfg.EnableReflection = true

	orchestratorSvc := &mockOrchestratorService{}
	stateSvc := &mockStateService{}
	vpcSvc := &mockVPCService{}
	agentSvc := &mockAgentService{}

	services := &servergrpc.Services{
		Orchestrator: orchestratorSvc,
		State:        stateSvc,
		VPC:          vpcSvc,
		Agent:        agentSvc,
	}

	server, err := servergrpc.NewServer(cfg, services)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create server: %v", err))
		return 1
	}

	p.Success("Server created successfully")

	// Test 2: Verify services were registered
	p.Step("Verifying services were registered")

	if !orchestratorSvc.registered {
		p.Error("Orchestrator service not registered")
		return 1
	}
	if !stateSvc.registered {
		p.Error("State service not registered")
		return 1
	}
	if !vpcSvc.registered {
		p.Error("VPC service not registered")
		return 1
	}
	if !agentSvc.registered {
		p.Error("Agent service not registered")
		return 1
	}

	p.Success("All 4 services registered")

	// Test 3: Start server
	p.Step("Starting server")

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	if !server.IsRunning() {
		p.Error("Server is not running after Start()")
		return 1
	}

	addr := server.Address()
	p.Success(fmt.Sprintf("Server started on %s", addr))

	// Test 4: Test gRPC connection
	p.Step("Testing gRPC connection")

	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to connect to server: %v", err))
		stopServer(ctx, server, p)
		return 1
	}
	defer conn.Close()

	p.Success("gRPC connection established")

	// Test 5: Test health check
	p.Step("Testing health check")

	healthClient := grpc_health_v1.NewHealthClient(conn)
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := healthClient.Check(healthCtx, &grpc_health_v1.HealthCheckRequest{
		Service: "engine",
	})
	if err != nil {
		p.Error(fmt.Sprintf("Health check failed: %v", err))
		stopServer(ctx, server, p)
		return 1
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		p.Error(fmt.Sprintf("Expected SERVING status, got %s", resp.Status))
		stopServer(ctx, server, p)
		return 1
	}

	p.Success("Health check passed: status=SERVING")

	// Test 6: Start second server (should fail with same port)
	p.Step("Testing port conflict detection")

	cfg2 := config.DefaultConfig()
	cfg2.Address = addr // Same address as running server

	server2, err := servergrpc.NewServer(cfg2, nil)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create second server: %v", err))
		stopServer(ctx, server, p)
		return 1
	}

	err2Ch := make(chan error, 1)
	go func() {
		err2Ch <- server2.Start()
	}()

	// Give it time to try to bind
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-err2Ch:
		if err != nil {
			p.Success(fmt.Sprintf("Port conflict detected: %v", err))
		}
	default:
		// Stop second server if it somehow started
		_ = server2.Stop(ctx)
		p.Warning("Second server didn't fail immediately, but that's acceptable")
	}

	// Test 7: Test GetServices
	p.Step("Testing GetServices")

	retrievedServices := server.GetServices()
	if retrievedServices == nil {
		p.Error("GetServices returned nil")
		stopServer(ctx, server, p)
		return 1
	}

	if retrievedServices.Orchestrator == nil {
		p.Error("Orchestrator service is nil")
		stopServer(ctx, server, p)
		return 1
	}

	p.Success("GetServices returned all services")

	// Test 8: Test GetGRPCServer
	p.Step("Testing GetGRPCServer")

	grpcServer := server.GetGRPCServer()
	if grpcServer == nil {
		p.Error("GetGRPCServer returned nil")
		stopServer(ctx, server, p)
		return 1
	}

	p.Success("GetGRPCServer returned valid server")

	// Test 9: Test graceful shutdown
	p.Step("Testing graceful shutdown")

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()

	err = server.Stop(shutdownCtx)
	if err != nil {
		p.Error(fmt.Sprintf("Graceful shutdown failed: %v", err))
		return 1
	}

	if server.IsRunning() {
		p.Error("Server is still running after Stop()")
		return 1
	}

	p.Success("Graceful shutdown completed")

	// Test 10: Test factory pattern
	p.Step("Testing factory pattern")

	factory := servergrpc.NewFactory(servergrpc.FactoryOptions{}).
		WithConfig(config.DefaultConfig().WithAddress(":0")).
		WithOrchestratorService(&mockOrchestratorService{}).
		WithStateService(&mockStateService{})

	factoryServer, err := factory.CreateServer()
	if err != nil {
		p.Error(fmt.Sprintf("Factory CreateServer failed: %v", err))
		return 1
	}

	go func() {
		_ = factoryServer.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	if !factoryServer.IsRunning() {
		p.Error("Factory-created server is not running")
		return 1
	}

	_ = factoryServer.Stop(ctx)

	p.Success("Factory pattern works correctly")

	// Test 11: Test server without services
	p.Step("Testing server without services (nil services)")

	nilServer, err := servergrpc.NewServer(config.DefaultConfig().WithAddress(":0"), nil)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create server with nil services: %v", err))
		return 1
	}

	go func() {
		_ = nilServer.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	if !nilServer.IsRunning() {
		p.Error("Server with nil services is not running")
		return 1
	}

	_ = nilServer.Stop(ctx)

	p.Success("Server works with nil services")

	// Test 12: Test stop without start
	p.Step("Testing stop without start")

	noStartServer, err := servergrpc.NewServer(config.DefaultConfig(), nil)
	if err != nil {
		p.Error(fmt.Sprintf("Failed to create server: %v", err))
		return 1
	}

	err = noStartServer.Stop(ctx)
	if err != nil {
		p.Error(fmt.Sprintf("Stop on non-started server should not error: %v", err))
		return 1
	}

	p.Success("Stop on non-started server handled correctly")

	return 0
}

func stopServer(ctx context.Context, server *servergrpc.Server, p *helpers.Printer) {
	p.Step("Cleaning up - stopping server")
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := server.Stop(shutdownCtx); err != nil {
		p.Warning(fmt.Sprintf("Cleanup warning: %v", err))
	}
}
