package grpc

import (
	"github.com/fertile-org/banyan/pkg/engine/server/config"
)

// FactoryOptions contains options for creating an engine server.
type FactoryOptions struct {
	// Config is the server configuration
	Config *config.ServerConfig

	// OrchestratorService is an optional pre-built orchestrator service
	OrchestratorService OrchestratorService

	// StateService is an optional pre-built state service
	StateService StateService

	// VPCService is an optional pre-built VPC service
	VPCService VPCService

	// AgentService is an optional pre-built agent service
	AgentService AgentService
}

// Factory creates engine gRPC servers.
type Factory struct {
	options FactoryOptions
}

// NewFactory creates a new engine server factory.
func NewFactory(options FactoryOptions) *Factory {
	return &Factory{
		options: options,
	}
}

// CreateServer creates a new engine gRPC server with all services.
func (f *Factory) CreateServer() (*Server, error) {
	services := f.createServices()
	return NewServer(f.options.Config, services)
}

// createServices creates all engine services.
func (f *Factory) createServices() *Services {
	services := &Services{}

	// Use provided services or create defaults
	if f.options.OrchestratorService != nil {
		services.Orchestrator = f.options.OrchestratorService
	}

	if f.options.StateService != nil {
		services.State = f.options.StateService
	}

	if f.options.VPCService != nil {
		services.VPC = f.options.VPCService
	}

	if f.options.AgentService != nil {
		services.Agent = f.options.AgentService
	}

	return services
}

// WithConfig sets the server configuration.
func (f *Factory) WithConfig(cfg *config.ServerConfig) *Factory {
	f.options.Config = cfg
	return f
}

// WithOrchestratorService sets the orchestrator service.
func (f *Factory) WithOrchestratorService(svc OrchestratorService) *Factory {
	f.options.OrchestratorService = svc
	return f
}

// WithStateService sets the state service.
func (f *Factory) WithStateService(svc StateService) *Factory {
	f.options.StateService = svc
	return f
}

// WithVPCService sets the VPC service.
func (f *Factory) WithVPCService(svc VPCService) *Factory {
	f.options.VPCService = svc
	return f
}

// WithAgentService sets the agent service.
func (f *Factory) WithAgentService(svc AgentService) *Factory {
	f.options.AgentService = svc
	return f
}
