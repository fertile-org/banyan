package usecases

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fertile-org/banyan/pkg/engine/registry/adapters"
	"github.com/fertile-org/banyan/pkg/engine/registry/domain"
)

func TestRegistryUseCase_RegisterAgent(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	publisher := adapters.NewMemoryEventPublisher()
	uc := NewRegistryUseCase(repo, publisher, nil)

	req := domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
		Capabilities: []domain.Capability{
			{Type: domain.CapabilityContainerRuntime},
		},
		Resources: domain.Resources{
			CPUCores:      4,
			MemoryMB:      8192,
			CPUAvailable:  4.0,
			MemoryFreeMB:  8192,
			MaxContainers: 100,
		},
		Labels: map[string]string{
			"zone": "us-east-1a",
		},
		Version: "1.0.0",
	}

	agent, err := uc.RegisterAgent(context.Background(), &req)

	require.NoError(t, err)
	assert.NotEmpty(t, agent.ID)
	assert.Equal(t, domain.AgentStatusOnline, agent.Status)
	assert.Equal(t, "worker-1", agent.Hostname)
	assert.Equal(t, "192.168.1.10:9090", agent.Address)
	assert.Len(t, agent.Capabilities, 1)
}

func TestRegistryUseCase_RegisterAgent_ValidationError(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	tests := []struct {
		name    string
		req     domain.RegisterAgentRequest
		wantErr error
	}{
		{
			name:    "empty hostname",
			req:     domain.RegisterAgentRequest{Address: "192.168.1.10:9090"},
			wantErr: domain.ErrInvalidHostname,
		},
		{
			name:    "empty address",
			req:     domain.RegisterAgentRequest{Hostname: "worker-1"},
			wantErr: domain.ErrInvalidAddress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := uc.RegisterAgent(context.Background(), &tt.req)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestRegistryUseCase_DeregisterAgent(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register an agent first
	req := domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
	}
	agent, err := uc.RegisterAgent(context.Background(), &req)
	require.NoError(t, err)

	// Deregister
	err = uc.DeregisterAgent(context.Background(), agent.ID)
	require.NoError(t, err)

	// Verify it's gone
	_, err = uc.GetAgent(context.Background(), agent.ID)
	assert.ErrorIs(t, err, domain.ErrAgentNotFound)
}

func TestRegistryUseCase_ProcessHeartbeat(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register an agent first
	req := domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
		Resources: domain.Resources{
			CPUCores:     4,
			MemoryMB:     8192,
			CPUAvailable: 4.0,
			MemoryFreeMB: 8192,
		},
	}
	agent, err := uc.RegisterAgent(context.Background(), &req)
	require.NoError(t, err)

	// Process heartbeat with updated resources
	err = uc.ProcessHeartbeat(context.Background(), agent.ID, &domain.HeartbeatStatus{
		Status: domain.AgentStatusOnline,
		Resources: domain.Resources{
			CPUCores:     4,
			MemoryMB:     8192,
			CPUAvailable: 2.5, // Changed
			MemoryFreeMB: 4096,
		},
	})
	require.NoError(t, err)

	// Verify resources updated
	updated, err := uc.GetAgent(context.Background(), agent.ID)
	require.NoError(t, err)
	assert.Equal(t, 2.5, updated.Resources.CPUAvailable)
	assert.Equal(t, int64(4096), updated.Resources.MemoryFreeMB)
}

func TestRegistryUseCase_SelectAgents_LeastLoaded(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register agents with different resources
	agents := []domain.RegisterAgentRequest{
		{
			Hostname: "worker-1",
			Address:  "192.168.1.10:9090",
			Resources: domain.Resources{
				CPUAvailable:  1.0,
				MemoryFreeMB:  1024,
				MaxContainers: 100,
			},
		},
		{
			Hostname: "worker-2",
			Address:  "192.168.1.11:9090",
			Resources: domain.Resources{
				CPUAvailable:  3.0, // More available
				MemoryFreeMB:  4096,
				MaxContainers: 100,
			},
		},
		{
			Hostname: "worker-3",
			Address:  "192.168.1.12:9090",
			Resources: domain.Resources{
				CPUAvailable:  2.0,
				MemoryFreeMB:  2048,
				MaxContainers: 100,
			},
		},
	}

	for i := range agents {
		_, err := uc.RegisterAgent(context.Background(), &agents[i])
		require.NoError(t, err)
	}

	// Select 2 agents using least_loaded strategy
	selected, err := uc.SelectAgents(context.Background(), domain.SelectionCriteria{
		Count: 2,
	}, "least_loaded")

	require.NoError(t, err)
	assert.Len(t, selected, 2)

	// Least loaded should return highest resources first
	assert.Equal(t, "worker-2", selected[0].Hostname)
	assert.Equal(t, "worker-3", selected[1].Hostname)
}

func TestRegistryUseCase_SelectAgents_WithCapabilities(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register agents with different capabilities
	_, err := uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
		Capabilities: []domain.Capability{
			{Type: domain.CapabilityContainerRuntime},
		},
		Resources: domain.Resources{CPUAvailable: 2.0, MemoryFreeMB: 2048, MaxContainers: 100},
	})
	require.NoError(t, err)

	_, err = uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname: "worker-2",
		Address:  "192.168.1.11:9090",
		Capabilities: []domain.Capability{
			{Type: domain.CapabilityContainerRuntime},
			{Type: domain.CapabilityNetworkNode},
		},
		Resources: domain.Resources{CPUAvailable: 2.0, MemoryFreeMB: 2048, MaxContainers: 100},
	})
	require.NoError(t, err)

	// Select agents requiring both container runtime and network node
	selected, err := uc.SelectAgents(context.Background(), domain.SelectionCriteria{
		RequiredCapabilities: []domain.CapabilityType{
			domain.CapabilityContainerRuntime,
			domain.CapabilityNetworkNode,
		},
		Count: 1,
	}, "least_loaded")

	require.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, "worker-2", selected[0].Hostname)
}

func TestRegistryUseCase_SelectAgents_InsufficientAgents(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register only one agent
	_, err := uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname:  "worker-1",
		Address:   "192.168.1.10:9090",
		Resources: domain.Resources{CPUAvailable: 2.0, MemoryFreeMB: 2048, MaxContainers: 100},
	})
	require.NoError(t, err)

	// Request 5 agents
	_, err = uc.SelectAgents(context.Background(), domain.SelectionCriteria{
		Count: 5,
	}, "least_loaded")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInsufficientAgents)
}

func TestRegistryUseCase_DrainAndActivate(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register an agent
	agent, err := uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.AgentStatusOnline, agent.Status)

	// Drain the agent
	err = uc.DrainAgent(context.Background(), agent.ID)
	require.NoError(t, err)

	updated, err := uc.GetAgent(context.Background(), agent.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentStatusDraining, updated.Status)

	// Activate the agent
	err = uc.ActivateAgent(context.Background(), agent.ID)
	require.NoError(t, err)

	updated, err = uc.GetAgent(context.Background(), agent.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AgentStatusOnline, updated.Status)
}

func TestRegistryUseCase_ListAgents(t *testing.T) {
	repo := adapters.NewMemoryAgentRepository()
	uc := NewRegistryUseCase(repo, nil, nil)

	// Register multiple agents
	_, err := uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname: "worker-1",
		Address:  "192.168.1.10:9090",
		Labels:   map[string]string{"zone": "us-east-1a"},
	})
	require.NoError(t, err)

	_, err = uc.RegisterAgent(context.Background(), &domain.RegisterAgentRequest{
		Hostname: "worker-2",
		Address:  "192.168.1.11:9090",
		Labels:   map[string]string{"zone": "us-east-1b"},
	})
	require.NoError(t, err)

	// List all agents
	agents, err := uc.ListAgents(context.Background(), domain.AgentFilter{})
	require.NoError(t, err)
	assert.Len(t, agents, 2)

	// Filter by labels
	agents, err = uc.ListAgents(context.Background(), domain.AgentFilter{
		Labels: map[string]string{"zone": "us-east-1a"},
	})
	require.NoError(t, err)
	assert.Len(t, agents, 1)
	assert.Equal(t, "worker-1", agents[0].Hostname)
}
