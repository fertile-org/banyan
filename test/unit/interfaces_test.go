package unit

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/interfaces"
)

func TestDeploymentStatus_States(t *testing.T) {
	validStates := []string{"running", "stopped", "failed", "pending"}

	status := interfaces.DeploymentStatus{
		ID:    "test-123",
		State: "running",
	}

	// Test state validation
	found := false
	for _, validState := range validStates {
		if status.State == validState {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("State '%s' is not in valid states", status.State)
	}
}

// Mock agent for testing
type mockAgent struct {
	id string
}

func (m *mockAgent) Execute(ctx context.Context, config interfaces.DeploymentConfig) error {
	if config.ID == "" {
		return interfaces.ErrMissingDeploymentID
	}
	return nil
}

func (m *mockAgent) HealthCheck(ctx context.Context) error {
	return nil
}

func TestMockAgent(t *testing.T) {
	agent := &mockAgent{id: "test-agent"}

	// Test execution with valid config
	err := agent.Execute(context.Background(), interfaces.DeploymentConfig{
		ID:   "test-123",
		Name: "test-deployment",
	})
	if err != nil {
		t.Errorf("Expected no execution error, got: %v", err)
	}

	// Test execution with invalid config
	err = agent.Execute(context.Background(), interfaces.DeploymentConfig{})
	if err == nil {
		t.Error("Expected execution error for empty config")
	}

	// Test health check
	err = agent.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("Expected no health check error, got: %v", err)
	}
}