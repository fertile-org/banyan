package unit

import (
	"context"
	"testing"

	"github.com/fertile-org/banyan/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestDeploymentStatus_States(t *testing.T) {
	validStates := []string{"running", "stopped", "failed", "pending"}

	status := interfaces.DeploymentStatus{
		ID:    "test-123",
		State: "running",
	}

	assert.Contains(t, validStates, status.State, "State should be in valid states list")
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

	t.Run("Execute with valid config", func(t *testing.T) {
		err := agent.Execute(context.Background(), interfaces.DeploymentConfig{
			ID:   "test-123",
			Name: "test-deployment",
		})
		assert.NoError(t, err, "Execute should not return error for valid config")
	})

	t.Run("Execute with invalid config", func(t *testing.T) {
		err := agent.Execute(context.Background(), interfaces.DeploymentConfig{})
		assert.Error(t, err, "Execute should return error for empty config")
		assert.Equal(t, interfaces.ErrMissingDeploymentID, err, "Should return specific missing ID error")
	})

	t.Run("HealthCheck", func(t *testing.T) {
		err := agent.HealthCheck(context.Background())
		assert.NoError(t, err, "HealthCheck should not return error")
	})
}