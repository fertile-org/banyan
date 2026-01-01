package grpc

import (
	"fmt"

	containeradapters "github.com/fertile-org/banyan/pkg/agent/container/adapters"
	containerinbound "github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	containeroutbound "github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
	containeruc "github.com/fertile-org/banyan/pkg/agent/container/usecases"
	healthadapters "github.com/fertile-org/banyan/pkg/agent/health/adapters"
	healthinbound "github.com/fertile-org/banyan/pkg/agent/health/ports/inbound"
	healthoutbound "github.com/fertile-org/banyan/pkg/agent/health/ports/outbound"
	healthuc "github.com/fertile-org/banyan/pkg/agent/health/usecases"
	networkadapters "github.com/fertile-org/banyan/pkg/agent/network/adapters"
	networkinbound "github.com/fertile-org/banyan/pkg/agent/network/ports/inbound"
	networkuc "github.com/fertile-org/banyan/pkg/agent/network/usecases"
	securityadapters "github.com/fertile-org/banyan/pkg/agent/security/adapters"
	securityinbound "github.com/fertile-org/banyan/pkg/agent/security/ports/inbound"
	securityuc "github.com/fertile-org/banyan/pkg/agent/security/usecases"
	taskadapters "github.com/fertile-org/banyan/pkg/agent/task/adapters"
	taskuc "github.com/fertile-org/banyan/pkg/agent/task/usecases"

	"github.com/fertile-org/banyan/pkg/agent/server/config"
)

// FactoryOptions contains options for creating the server.
type FactoryOptions struct {
	Config           *config.ServerConfig
	ContainerdSocket string
}

// Factory creates and configures the agent server with all dependencies.
type Factory struct {
	options *FactoryOptions
}

// NewFactory creates a new server factory.
func NewFactory(options *FactoryOptions) *Factory {
	if options == nil {
		options = &FactoryOptions{}
	}
	if options.Config == nil {
		options.Config = config.DefaultConfig()
	}
	if options.ContainerdSocket == "" {
		options.ContainerdSocket = "/run/containerd/containerd.sock"
	}
	return &Factory{options: options}
}

// CreateServer creates a fully configured agent server.
func (f *Factory) CreateServer() (*Server, error) {
	services, err := f.createServices()
	if err != nil {
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	server, err := NewServer(f.options.Config, services)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return server, nil
}

// createServices creates all agent services with their dependencies.
func (f *Factory) createServices() (*Services, error) {
	// Create container service
	containerRuntime, err := f.createContainerRuntime()
	if err != nil {
		return nil, fmt.Errorf("failed to create container runtime: %w", err)
	}

	containerService := containeruc.NewContainerUseCase(
		containerRuntime,
		nil, // NetworkConnector - to be injected
		nil, // EventEmitter - to be injected
		nil, // ImageRegistry - to be injected
	)

	// Create network service
	networkService := f.createNetworkService()

	// Create security service
	securityService := f.createSecurityService()

	// Create health service
	healthService := f.createHealthService()

	// Create task service with all dependencies wired
	taskService := f.createTaskService(containerService, networkService, securityService, healthService)

	return &Services{
		Container: containerService,
		Network:   networkService,
		Security:  securityService,
		Health:    healthService,
		Task:      taskService,
	}, nil
}

// createContainerRuntime creates the container runtime adapter.
func (f *Factory) createContainerRuntime() (containeroutbound.ContainerRuntime, error) {
	cfg := containeradapters.ContainerdRuntimeConfig{
		Address:   f.options.ContainerdSocket,
		Namespace: "banyan",
	}
	runtime, err := containeradapters.NewContainerdRuntime(cfg)
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

// createNetworkService creates the network service with dependencies.
func (f *Factory) createNetworkService() *networkuc.NetworkUseCase {
	endpointStore := networkadapters.NewMemoryEndpointStore()
	networkStore := networkadapters.NewMemoryNetworkStore()

	return networkuc.NewNetworkUseCase(
		nil, // IPAllocator - to be injected
		nil, // CNIProvider - to be injected
		endpointStore,
		networkStore,
	)
}

// createSecurityService creates the security service with dependencies.
func (f *Factory) createSecurityService() *securityuc.SecurityUseCase {
	ruleStore := securityadapters.NewMemoryRuleStore()

	return securityuc.NewSecurityUseCase(
		nil, // SecurityManager - to be injected
		ruleStore,
	)
}

// createHealthService creates the health service with dependencies.
func (f *Factory) createHealthService() *healthuc.HealthUseCase {
	checkStore := healthadapters.NewMemoryCheckStore()
	systemMonitor := healthadapters.NewSystemMonitorAdapter()

	checkers := []healthoutbound.HealthChecker{
		healthadapters.NewHTTPChecker(),
		healthadapters.NewTCPChecker(),
		healthadapters.NewExecChecker(),
	}

	return healthuc.NewHealthUseCase(checkers, checkStore, systemMonitor)
}

// createTaskService creates the task service with dependencies.
func (f *Factory) createTaskService(
	containerService containerinbound.ContainerService,
	networkService networkinbound.NetworkService,
	securityService securityinbound.SecurityService,
	healthService healthinbound.HealthService,
) *taskuc.TaskUseCase {
	// Create task store and event emitter
	taskStore := taskadapters.NewMemoryTaskStoreAdapter()
	eventEmitter := taskadapters.NewMemoryEventEmitterAdapter()

	// Create service adapters to bridge task executor's outbound ports to actual services
	containerAdapter := taskadapters.NewContainerServiceAdapter(containerService)
	networkAdapter := taskadapters.NewNetworkServiceAdapter(networkService)
	securityAdapter := taskadapters.NewSecurityServiceAdapter(securityService)
	healthAdapter := taskadapters.NewHealthServiceAdapter(healthService)

	return taskuc.NewTaskUseCase(
		containerAdapter,
		networkAdapter,
		securityAdapter,
		healthAdapter,
		taskStore,
		eventEmitter,
	)
}
