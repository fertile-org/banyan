package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/server/config"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.ServerConfig
		services *Services
		wantErr  bool
	}{
		{
			name:     "nil config uses defaults",
			cfg:      nil,
			services: &Services{},
			wantErr:  false,
		},
		{
			name:     "custom config",
			cfg:      &config.ServerConfig{Address: ":50052"},
			services: &Services{},
			wantErr:  false,
		},
		{
			name:     "nil services",
			cfg:      nil,
			services: nil,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewServer(tt.cfg, tt.services)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("NewServer() returned nil server")
			}
		})
	}
}

func TestServer_StartStop(t *testing.T) {
	cfg := &config.ServerConfig{
		Address:         ":0", // Use any available port
		MaxRecvMsgSize:  4 * 1024 * 1024,
		MaxSendMsgSize:  4 * 1024 * 1024,
		KeepAliveTime:   30 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}

	server, err := NewServer(cfg, &Services{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Start server in background
	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	if !server.IsRunning() {
		t.Error("Server should be running after Start()")
	}

	addr := server.Address()
	if addr == "" {
		t.Error("Server address should not be empty after Start()")
	}

	// Stop server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if server.IsRunning() {
		t.Error("Server should not be running after Stop()")
	}
}

func TestServer_StartAlreadyRunning(t *testing.T) {
	cfg := &config.ServerConfig{
		Address:         ":0",
		MaxRecvMsgSize:  4 * 1024 * 1024,
		MaxSendMsgSize:  4 * 1024 * 1024,
		KeepAliveTime:   30 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}

	server, err := NewServer(cfg, &Services{})
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
		t.Error("Start() should return error when server is already running")
	}

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Stop(ctx)
}

func TestServer_StopNotRunning(t *testing.T) {
	server, err := NewServer(nil, &Services{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop without starting should not error
	if err := server.Stop(ctx); err != nil {
		t.Errorf("Stop() on non-running server error = %v", err)
	}
}

func TestServer_AddressBeforeStart(t *testing.T) {
	server, err := NewServer(nil, &Services{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	addr := server.Address()
	if addr != "" {
		t.Errorf("Address() before Start() should be empty, got %q", addr)
	}
}

func TestServer_GetGRPCServer(t *testing.T) {
	server, err := NewServer(nil, &Services{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	grpcServer := server.GetGRPCServer()
	if grpcServer == nil {
		t.Error("GetGRPCServer() should not return nil")
	}
}

func TestServer_GetServices(t *testing.T) {
	services := &Services{}
	server, err := NewServer(nil, services)
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	gotServices := server.GetServices()
	if gotServices != services {
		t.Error("GetServices() returned different services instance")
	}
}

func TestServer_StopWithTimeout(t *testing.T) {
	cfg := &config.ServerConfig{
		Address:         ":0",
		MaxRecvMsgSize:  4 * 1024 * 1024,
		MaxSendMsgSize:  4 * 1024 * 1024,
		KeepAliveTime:   30 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	}

	server, err := NewServer(cfg, &Services{})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	// Start server
	go func() {
		_ = server.Start()
	}()
	time.Sleep(100 * time.Millisecond)

	// Stop with very short timeout should still work for idle server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = server.Stop(ctx)
	// Context may or may not expire for idle server, both are acceptable
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("Stop() unexpected error = %v", err)
	}
}
