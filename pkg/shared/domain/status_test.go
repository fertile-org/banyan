package domain

import (
	"testing"
)

func TestDeploymentStatus(t *testing.T) {
	t.Run("IsTerminal", func(t *testing.T) {
		tests := []struct {
			status   DeploymentStatus
			terminal bool
		}{
			{DeploymentStatusPending, false},
			{DeploymentStatusRunning, false},
			{DeploymentStatusStopped, true},
			{DeploymentStatusFailed, true},
		}

		for _, tt := range tests {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("%s.IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
			}
		}
	})

	t.Run("IsActive", func(t *testing.T) {
		tests := []struct {
			status DeploymentStatus
			active bool
		}{
			{DeploymentStatusPending, false},
			{DeploymentStatusRunning, true},
			{DeploymentStatusRollingOut, true},
			{DeploymentStatusStopped, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsActive(); got != tt.active {
				t.Errorf("%s.IsActive() = %v, want %v", tt.status, got, tt.active)
			}
		}
	})

	t.Run("Validate", func(t *testing.T) {
		validStatuses := []DeploymentStatus{
			DeploymentStatusPending,
			DeploymentStatusValidating,
			DeploymentStatusScheduling,
			DeploymentStatusDeploying,
			DeploymentStatusRunning,
			DeploymentStatusStopping,
			DeploymentStatusStopped,
			DeploymentStatusFailed,
			DeploymentStatusRollingOut,
			DeploymentStatusRollback,
		}

		for _, s := range validStatuses {
			if err := s.Validate(); err != nil {
				t.Errorf("%s.Validate() returned error: %v", s, err)
			}
		}

		invalid := DeploymentStatus("invalid")
		if err := invalid.Validate(); err == nil {
			t.Error("expected error for invalid status")
		}
	})
}

func TestAgentStatus(t *testing.T) {
	t.Run("IsAvailable", func(t *testing.T) {
		tests := []struct {
			status    AgentStatus
			available bool
		}{
			{AgentStatusHealthy, true},
			{AgentStatusUnhealthy, false},
			{AgentStatusDraining, false},
			{AgentStatusDisconnected, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsAvailable(); got != tt.available {
				t.Errorf("%s.IsAvailable() = %v, want %v", tt.status, got, tt.available)
			}
		}
	})

	t.Run("IsConnected", func(t *testing.T) {
		tests := []struct {
			status    AgentStatus
			connected bool
		}{
			{AgentStatusHealthy, true},
			{AgentStatusUnhealthy, true},
			{AgentStatusDraining, true},
			{AgentStatusDisconnected, false},
			{AgentStatusDeregistered, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsConnected(); got != tt.connected {
				t.Errorf("%s.IsConnected() = %v, want %v", tt.status, got, tt.connected)
			}
		}
	})
}

func TestTaskStatus(t *testing.T) {
	t.Run("IsTerminal", func(t *testing.T) {
		tests := []struct {
			status   TaskStatus
			terminal bool
		}{
			{TaskStatusPending, false},
			{TaskStatusRunning, false},
			{TaskStatusSucceeded, true},
			{TaskStatusFailed, true},
			{TaskStatusCancelled, true},
			{TaskStatusSkipped, true},
			{TaskStatusTimedOut, true},
		}

		for _, tt := range tests {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("%s.IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
			}
		}
	})

	t.Run("IsActive", func(t *testing.T) {
		tests := []struct {
			status TaskStatus
			active bool
		}{
			{TaskStatusPending, false},
			{TaskStatusRunning, true},
			{TaskStatusRetrying, true},
			{TaskStatusSucceeded, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsActive(); got != tt.active {
				t.Errorf("%s.IsActive() = %v, want %v", tt.status, got, tt.active)
			}
		}
	})
}

func TestContainerStatus(t *testing.T) {
	t.Run("IsRunning", func(t *testing.T) {
		tests := []struct {
			status  ContainerStatus
			running bool
		}{
			{ContainerStatusCreating, false},
			{ContainerStatusRunning, true},
			{ContainerStatusStopped, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsRunning(); got != tt.running {
				t.Errorf("%s.IsRunning() = %v, want %v", tt.status, got, tt.running)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		tests := []struct {
			status   ContainerStatus
			terminal bool
		}{
			{ContainerStatusCreating, false},
			{ContainerStatusRunning, false},
			{ContainerStatusStopped, true},
			{ContainerStatusRemoved, true},
			{ContainerStatusFailed, true},
		}

		for _, tt := range tests {
			if got := tt.status.IsTerminal(); got != tt.terminal {
				t.Errorf("%s.IsTerminal() = %v, want %v", tt.status, got, tt.terminal)
			}
		}
	})
}

func TestHealthStatus(t *testing.T) {
	t.Run("IsHealthy", func(t *testing.T) {
		tests := []struct {
			status  HealthStatus
			healthy bool
		}{
			{HealthStatusHealthy, true},
			{HealthStatusUnhealthy, false},
			{HealthStatusDegraded, false},
			{HealthStatusUnknown, false},
		}

		for _, tt := range tests {
			if got := tt.status.IsHealthy(); got != tt.healthy {
				t.Errorf("%s.IsHealthy() = %v, want %v", tt.status, got, tt.healthy)
			}
		}
	})
}

func TestTaskType(t *testing.T) {
	validTypes := []TaskType{
		TaskTypeContainerCreate,
		TaskTypeContainerStart,
		TaskTypeContainerStop,
		TaskTypeContainerRemove,
		TaskTypeContainerRestart,
		TaskTypeNetworkSetup,
		TaskTypeNetworkTeardown,
		TaskTypeSecurityApply,
		TaskTypeSecurityRemove,
		TaskTypeHealthCheck,
	}

	for _, tt := range validTypes {
		if err := tt.Validate(); err != nil {
			t.Errorf("%s.Validate() returned error: %v", tt, err)
		}
	}

	invalid := TaskType("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("expected error for invalid task type")
	}
}

func TestHookPoint(t *testing.T) {
	validHooks := []HookPoint{
		HookPointPreValidate,
		HookPointPostValidate,
		HookPointPreDeploy,
		HookPointPostDeploy,
		HookPointPreStop,
		HookPointPostStop,
		HookPointOnFailure,
	}

	for _, h := range validHooks {
		if err := h.Validate(); err != nil {
			t.Errorf("%s.Validate() returned error: %v", h, err)
		}
	}

	invalid := HookPoint("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("expected error for invalid hook point")
	}
}

func TestPluginType(t *testing.T) {
	validTypes := []PluginType{
		PluginTypeWebhook,
		PluginTypeGRPC,
		PluginTypeBuiltin,
	}

	for _, pt := range validTypes {
		if err := pt.Validate(); err != nil {
			t.Errorf("%s.Validate() returned error: %v", pt, err)
		}
	}

	invalid := PluginType("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("expected error for invalid plugin type")
	}
}

func TestProbeType(t *testing.T) {
	validTypes := []ProbeType{
		ProbeTypeHTTP,
		ProbeTypeTCP,
		ProbeTypeExec,
	}

	for _, pt := range validTypes {
		if err := pt.Validate(); err != nil {
			t.Errorf("%s.Validate() returned error: %v", pt, err)
		}
	}

	invalid := ProbeType("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("expected error for invalid probe type")
	}
}

func TestSelectionStrategy(t *testing.T) {
	validStrategies := []SelectionStrategy{
		SelectionStrategyRoundRobin,
		SelectionStrategyLeastLoaded,
		SelectionStrategySpread,
		SelectionStrategyBinPack,
	}

	for _, s := range validStrategies {
		if err := s.Validate(); err != nil {
			t.Errorf("%s.Validate() returned error: %v", s, err)
		}
	}

	invalid := SelectionStrategy("invalid")
	if err := invalid.Validate(); err == nil {
		t.Error("expected error for invalid selection strategy")
	}
}
