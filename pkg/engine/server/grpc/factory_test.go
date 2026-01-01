package grpc

import (
	"testing"

	"github.com/fertile-org/banyan/pkg/engine/server/config"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory(FactoryOptions{})
	if factory == nil {
		t.Fatal("Expected factory to be created")
	}
}

func TestFactory_CreateServer(t *testing.T) {
	factory := NewFactory(FactoryOptions{
		Config: config.DefaultConfig(),
	})

	server, err := factory.CreateServer()
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	if server == nil {
		t.Fatal("Expected server to be created")
	}
}

func TestFactory_WithConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":8080"

	factory := NewFactory(FactoryOptions{}).WithConfig(cfg)

	server, err := factory.CreateServer()
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	if server.config.Address != ":8080" {
		t.Errorf("Expected address ':8080', got '%s'", server.config.Address)
	}
}

func TestFactory_WithServices(t *testing.T) {
	orchestrator := &mockOrchestratorService{}
	state := &mockStateService{}
	vpc := &mockVPCService{}
	agent := &mockAgentService{}

	factory := NewFactory(FactoryOptions{}).
		WithOrchestratorService(orchestrator).
		WithStateService(state).
		WithVPCService(vpc).
		WithAgentService(agent)

	server, err := factory.CreateServer()
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	services := server.GetServices()
	if services.Orchestrator == nil {
		t.Error("Expected orchestrator service to be set")
	}
	if services.State == nil {
		t.Error("Expected state service to be set")
	}
	if services.VPC == nil {
		t.Error("Expected VPC service to be set")
	}
	if services.Agent == nil {
		t.Error("Expected agent service to be set")
	}
}

func TestFactory_ChainedMethods(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Address = ":9090"
	cfg.EnableReflection = true

	orchestrator := &mockOrchestratorService{}

	factory := NewFactory(FactoryOptions{}).
		WithConfig(cfg).
		WithOrchestratorService(orchestrator)

	server, err := factory.CreateServer()
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	if server.config.Address != ":9090" {
		t.Errorf("Expected address ':9090', got '%s'", server.config.Address)
	}
	if !server.config.EnableReflection {
		t.Error("Expected reflection to be enabled")
	}
}

func TestFactory_CreateServerWithNilConfig(t *testing.T) {
	factory := NewFactory(FactoryOptions{})

	server, err := factory.CreateServer()
	if err != nil {
		t.Fatalf("CreateServer() error = %v", err)
	}

	// Should use default config
	if server.config.Address != ":50052" {
		t.Errorf("Expected default address ':50052', got '%s'", server.config.Address)
	}
}
