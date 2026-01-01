package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/fertile-org/banyan/pkg/engine/server/config"
)

// Services holds all engine gRPC service implementations.
type Services struct {
	// Orchestrator handles deployment operations
	Orchestrator OrchestratorService

	// State handles state management operations
	State StateService

	// VPC handles VPC coordination
	VPC VPCService

	// Agent handles agent registration and communication
	Agent AgentService
}

// OrchestratorService defines the orchestrator gRPC service interface.
type OrchestratorService interface {
	// Register registers the service with the gRPC server
	Register(server *grpc.Server)
}

// StateService defines the state management gRPC service interface.
type StateService interface {
	// Register registers the service with the gRPC server
	Register(server *grpc.Server)
}

// VPCService defines the VPC coordination gRPC service interface.
type VPCService interface {
	// Register registers the service with the gRPC server
	Register(server *grpc.Server)
}

// AgentService defines the agent communication gRPC service interface.
type AgentService interface {
	// Register registers the service with the gRPC server
	Register(server *grpc.Server)
}

// Server is the engine gRPC server.
type Server struct {
	server    *grpc.Server
	listener  net.Listener
	config    *config.ServerConfig
	services  *Services
	healthSrv *health.Server
	mu        sync.Mutex
	running   bool
}

// NewServer creates a new engine gRPC server.
func NewServer(cfg *config.ServerConfig, services *Services) (*Server, error) {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time: cfg.KeepAliveTime,
		}),
	}

	// Add TLS if configured
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)

	// Register health service
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)

	// Enable reflection if configured
	if cfg.EnableReflection {
		reflection.Register(grpcServer)
	}

	// Register engine services if provided
	if services != nil {
		if services.Orchestrator != nil {
			services.Orchestrator.Register(grpcServer)
		}
		if services.State != nil {
			services.State.Register(grpcServer)
		}
		if services.VPC != nil {
			services.VPC.Register(grpcServer)
		}
		if services.Agent != nil {
			services.Agent.Register(grpcServer)
		}
	}

	return &Server{
		server:    grpcServer,
		config:    cfg,
		services:  services,
		healthSrv: healthSrv,
	}, nil
}

// Start starts the gRPC server.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}

	listener, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.listener = listener
	s.running = true
	s.mu.Unlock()

	// Set health status to serving
	s.healthSrv.SetServingStatus("engine", grpc_health_v1.HealthCheckResponse_SERVING)

	return s.server.Serve(listener)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Set health status to not serving
	s.healthSrv.SetServingStatus("engine", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		s.running = false
		return nil
	case <-ctx.Done():
		s.server.Stop()
		s.running = false
		return ctx.Err()
	}
}

// Address returns the server's listening address.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.config.Address
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// GetGRPCServer returns the underlying gRPC server.
func (s *Server) GetGRPCServer() *grpc.Server {
	return s.server
}

// GetServices returns the registered services.
func (s *Server) GetServices() *Services {
	return s.services
}
