package grpc

import (
	"testing"

	"github.com/fertile-org/banyan/pkg/agent/server/config"
)

func TestNewFactory(t *testing.T) {
	tests := []struct {
		name    string
		options *FactoryOptions
	}{
		{
			name:    "nil options uses defaults",
			options: nil,
		},
		{
			name: "custom config",
			options: &FactoryOptions{
				Config: &config.ServerConfig{Address: ":50052"},
			},
		},
		{
			name: "custom containerd socket",
			options: &FactoryOptions{
				ContainerdSocket: "/custom/containerd.sock",
			},
		},
		{
			name: "all options",
			options: &FactoryOptions{
				Config:           &config.ServerConfig{Address: ":50052"},
				ContainerdSocket: "/custom/containerd.sock",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := NewFactory(tt.options)
			if factory == nil {
				t.Error("NewFactory() returned nil")
				return
			}
			if factory.options == nil {
				t.Error("factory.options should not be nil")
			}
			if factory.options.Config == nil {
				t.Error("factory.options.Config should not be nil")
			}
			if factory.options.ContainerdSocket == "" {
				t.Error("factory.options.ContainerdSocket should not be empty")
			}
		})
	}
}

func TestNewFactory_DefaultValues(t *testing.T) {
	factory := NewFactory(nil)

	if factory.options.Config.Address != ":50051" {
		t.Errorf("expected default address :50051, got %s", factory.options.Config.Address)
	}

	if factory.options.ContainerdSocket != "/run/containerd/containerd.sock" {
		t.Errorf("expected default socket /run/containerd/containerd.sock, got %s", factory.options.ContainerdSocket)
	}
}

func TestFactory_CreateServer_FailsWithoutContainerd(t *testing.T) {
	// This test verifies that CreateServer fails when containerd is not available
	// In a test environment without containerd, this should return an error
	factory := NewFactory(&FactoryOptions{
		ContainerdSocket: "/nonexistent/containerd.sock",
	})

	_, err := factory.CreateServer()
	if err == nil {
		t.Error("CreateServer() should fail when containerd is not available")
	}
}
