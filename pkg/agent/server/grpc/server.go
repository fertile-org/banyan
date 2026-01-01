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

	containerinbound "github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	healthinbound "github.com/fertile-org/banyan/pkg/agent/health/ports/inbound"
	networkinbound "github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"
	securityinbound "github.com/fertile-org/banyan/pkg/agent/security/ports/inbound"
	taskinbound "github.com/fertile-org/banyan/pkg/agent/task/ports/inbound"

	"github.com/fertile-org/banyan/pkg/agent/server/config"
)

// Services holds all agent services.
type Services struct {
	Container containerinbound.ContainerService
	Network   networkinbound.NetworkService
	Security  securityinbound.SecurityService
	Health    healthinbound.HealthService
	Task      taskinbound.TaskExecutorService
}

// Server represents the agent gRPC server.
type Server struct {
	server    *grpc.Server
	listener  net.Listener
	config    *config.ServerConfig
	services  *Services
	healthSrv *health.Server
	mu        sync.Mutex
	running   bool
}

// NewServer creates a new agent gRPC server.
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
	s.healthSrv.SetServingStatus("agent", grpc_health_v1.HealthCheckResponse_SERVING)

	return s.server.Serve(listener)
}

// Stop stops the gRPC server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	// Set health status to not serving
	s.healthSrv.SetServingStatus("agent", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

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

// Address returns the server's listen address.
func (s *Server) Address() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// IsRunning returns whether the server is currently running.
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// GetGRPCServer returns the underlying gRPC server for registering additional services.
func (s *Server) GetGRPCServer() *grpc.Server {
	return s.server
}

// GetServices returns the agent services.
func (s *Server) GetServices() *Services {
	return s.services
}
