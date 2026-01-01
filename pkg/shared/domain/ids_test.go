package domain

import (
	"strings"
	"testing"
)

func TestNewDeploymentID(t *testing.T) {
	id := NewDeploymentID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "dep-") {
		t.Errorf("expected ID to start with 'dep-', got %s", id)
	}
}

func TestParseDeploymentID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid ID", "dep-123456-abcd", false},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDeploymentID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDeploymentID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAgentID(t *testing.T) {
	id := NewAgentID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "agent-") {
		t.Errorf("expected ID to start with 'agent-', got %s", id)
	}

	// Test String method
	if id.String() == "" {
		t.Error("String() should return non-empty string")
	}
}

func TestTaskID(t *testing.T) {
	id := NewTaskID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "task-") {
		t.Errorf("expected ID to start with 'task-', got %s", id)
	}
}

func TestContainerID(t *testing.T) {
	id := NewContainerID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "ctr-") {
		t.Errorf("expected ID to start with 'ctr-', got %s", id)
	}
}

func TestServiceID(t *testing.T) {
	id := NewServiceID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "svc-") {
		t.Errorf("expected ID to start with 'svc-', got %s", id)
	}
}

func TestPluginID(t *testing.T) {
	id := NewPluginID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "plugin-") {
		t.Errorf("expected ID to start with 'plugin-', got %s", id)
	}
}

func TestVPCID(t *testing.T) {
	id := NewVPCID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "vpc-") {
		t.Errorf("expected ID to start with 'vpc-', got %s", id)
	}
}

func TestSubnetID(t *testing.T) {
	id := NewSubnetID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "subnet-") {
		t.Errorf("expected ID to start with 'subnet-', got %s", id)
	}
}

func TestSecurityGroupID(t *testing.T) {
	id := NewSecurityGroupID()

	if id.IsEmpty() {
		t.Error("expected non-empty ID")
	}

	if !strings.HasPrefix(string(id), "sg-") {
		t.Errorf("expected ID to start with 'sg-', got %s", id)
	}
}

func TestIDFromString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantErr  bool
	}{
		{"deployment ID", "dep-123456-abcd", "DeploymentID", false},
		{"agent ID", "agent-123456-abcd", "AgentID", false},
		{"task ID", "task-123456-abcd", "TaskID", false},
		{"container ID", "ctr-123456-abcd", "ContainerID", false},
		{"service ID", "svc-123456-abcd", "ServiceID", false},
		{"plugin ID", "plugin-123456-abcd", "PluginID", false},
		{"vpc ID", "vpc-123456-abcd", "VPCID", false},
		{"subnet ID", "subnet-123456-abcd", "SubnetID", false},
		{"security group ID", "sg-123456-abcd", "SecurityGroupID", false},
		{"empty string", "", "", true},
		{"invalid format", "invalid", "", true},
		{"unknown type", "unknown-123456-abcd", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := IDFromString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("IDFromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && id == nil {
				t.Error("expected non-nil ID")
			}
		})
	}
}

func TestIDUniqueness(t *testing.T) {
	// Generate multiple IDs and ensure they're unique
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewDeploymentID()
		if ids[string(id)] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[string(id)] = true
	}
}
