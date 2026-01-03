package registry

import (
	"context"
	"net"
	"testing"

	"github.com/fertile-org/banyan/pkg/vpc/storage"
)

func TestServiceRegistry_PutService(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		containerID string
		ip          net.IP
		wantErr     bool
		errContains string
	}{
		{
			name:        "register valid service instance",
			serviceName: "web",
			containerID: "container-1",
			ip:          net.ParseIP("10.0.1.10"),
			wantErr:     false,
		},
		{
			name:        "register another service instance",
			serviceName: "api",
			containerID: "container-2",
			ip:          net.ParseIP("10.0.1.20"),
			wantErr:     false,
		},
		{
			name:        "register with IPv6",
			serviceName: "database",
			containerID: "container-3",
			ip:          net.ParseIP("::1"),
			wantErr:     false,
		},
		{
			name:        "register duplicate updates IP",
			serviceName: "web",
			containerID: "container-1",
			ip:          net.ParseIP("10.0.1.99"),
			wantErr:     false,
		},
		{
			name:        "register with empty service name",
			serviceName: "",
			containerID: "container-1",
			ip:          net.ParseIP("10.0.1.10"),
			wantErr:     true,
			errContains: "service name cannot be empty",
		},
		{
			name:        "register with empty container ID",
			serviceName: "web",
			containerID: "",
			ip:          net.ParseIP("10.0.1.10"),
			wantErr:     true,
			errContains: "container ID cannot be empty",
		},
		{
			name:        "register with nil IP",
			serviceName: "web",
			containerID: "container-1",
			ip:          nil,
			wantErr:     true,
			errContains: "IP cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemoryStore()
			reg := NewRegistry(store)

			err := reg.PutService(context.Background(), tt.serviceName, tt.containerID, tt.ip)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestServiceRegistry_DeleteService(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(reg *Registry)
		serviceName string
		containerID string
		wantErr     bool
		errContains string
	}{
		{
			name: "delete existing service instance",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
			},
			serviceName: "web",
			containerID: "container-1",
			wantErr:     false,
		},
		{
			name: "delete non-existent service instance",
			setup: func(reg *Registry) {
				// no setup
			},
			serviceName: "web",
			containerID: "container-999",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "delete with empty service name",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
			},
			serviceName: "",
			containerID: "container-1",
			wantErr:     true,
			errContains: "service name cannot be empty",
		},
		{
			name: "delete with empty container ID",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
			},
			serviceName: "web",
			containerID: "",
			wantErr:     true,
			errContains: "container ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemoryStore()
			reg := NewRegistry(store)

			if tt.setup != nil {
				tt.setup(reg)
			}

			err := reg.DeleteService(context.Background(), tt.serviceName, tt.containerID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestServiceRegistry_GetServiceInstances(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(reg *Registry)
		serviceName string
		wantCount   int
		wantErr     bool
		errContains string
	}{
		{
			name: "get single service instance",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
			},
			serviceName: "web",
			wantCount:   1,
			wantErr:     false,
		},
		{
			name: "get multiple service instances",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
				_ = reg.PutService(context.Background(), "web", "container-2", net.ParseIP("10.0.1.20"))
				_ = reg.PutService(context.Background(), "web", "container-3", net.ParseIP("10.0.1.30"))
			},
			serviceName: "web",
			wantCount:   3,
			wantErr:     false,
		},
		{
			name: "get non-existent service",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "api", "container-1", net.ParseIP("10.0.1.10"))
			},
			serviceName: "web",
			wantCount:   0,
			wantErr:     false,
		},
		{
			name:        "get with empty service name",
			setup:       func(reg *Registry) {},
			serviceName: "",
			wantErr:     true,
			errContains: "service name cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemoryStore()
			reg := NewRegistry(store)

			if tt.setup != nil {
				tt.setup(reg)
			}

			instances, err := reg.GetServiceInstances(context.Background(), tt.serviceName)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(instances) != tt.wantCount {
				t.Errorf("expected %d instances, got %d", tt.wantCount, len(instances))
			}
		})
	}
}

func TestServiceRegistry_GetContainerService(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(reg *Registry)
		containerID     string
		wantServiceName string
		wantErr         bool
		errContains     string
	}{
		{
			name: "get container service",
			setup: func(reg *Registry) {
				_ = reg.PutService(context.Background(), "web", "container-1", net.ParseIP("10.0.1.10"))
			},
			containerID:     "container-1",
			wantServiceName: "web",
			wantErr:         false,
		},
		{
			name:            "get non-existent container",
			setup:           func(reg *Registry) {},
			containerID:     "container-999",
			wantServiceName: "",
			wantErr:         true,
			errContains:     "not found",
		},
		{
			name:        "get with empty container ID",
			setup:       func(reg *Registry) {},
			containerID: "",
			wantErr:     true,
			errContains: "container ID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := storage.NewMemoryStore()
			reg := NewRegistry(store)

			if tt.setup != nil {
				tt.setup(reg)
			}

			serviceName, err := reg.GetContainerService(context.Background(), tt.containerID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if serviceName != tt.wantServiceName {
				t.Errorf("expected service name %q, got %q", tt.wantServiceName, serviceName)
			}
		})
	}
}

func TestServiceRegistry_Integration(t *testing.T) {
	t.Run("full lifecycle", func(t *testing.T) {
		store := storage.NewMemoryStore()
		reg := NewRegistry(store)
		ctx := context.Background()

		// Register multiple instances
		if err := reg.PutService(ctx, "web", "container-1", net.ParseIP("10.0.1.10")); err != nil {
			t.Fatalf("failed to put service: %v", err)
		}
		if err := reg.PutService(ctx, "web", "container-2", net.ParseIP("10.0.1.20")); err != nil {
			t.Fatalf("failed to put service: %v", err)
		}
		if err := reg.PutService(ctx, "api", "container-3", net.ParseIP("10.0.2.10")); err != nil {
			t.Fatalf("failed to put service: %v", err)
		}

		// Verify web service has 2 instances
		webInstances, err := reg.GetServiceInstances(ctx, "web")
		if err != nil {
			t.Fatalf("failed to get web instances: %v", err)
		}
		if len(webInstances) != 2 {
			t.Errorf("expected 2 web instances, got %d", len(webInstances))
		}

		// Verify api service has 1 instance
		apiInstances, err := reg.GetServiceInstances(ctx, "api")
		if err != nil {
			t.Fatalf("failed to get api instances: %v", err)
		}
		if len(apiInstances) != 1 {
			t.Errorf("expected 1 api instance, got %d", len(apiInstances))
		}

		// Delete one web instance
		if err := reg.DeleteService(ctx, "web", "container-1"); err != nil {
			t.Fatalf("failed to delete service: %v", err)
		}

		// Verify web service now has 1 instance
		webInstances, err = reg.GetServiceInstances(ctx, "web")
		if err != nil {
			t.Fatalf("failed to get web instances: %v", err)
		}
		if len(webInstances) != 1 {
			t.Errorf("expected 1 web instance, got %d", len(webInstances))
		}

		// Verify container-1 no longer associated
		_, err = reg.GetContainerService(ctx, "container-1")
		if err == nil {
			t.Error("expected error for deleted container")
		}
	})
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
