package cni_test

import (
	"context"
	"net"
	"testing"

	"github.com/fertile/banyan/pkg/vpc"
	"github.com/fertile/banyan/pkg/vpc/cni"
)

func TestCNIRuntime_AddToNetwork(t *testing.T) {
	ctx := context.Background()
	runtime := cni.NewRuntime()

	tests := []struct {
		name        string
		containerID string
		networkID   string
		ip          net.IP
		wantErr     bool
	}{
		{
			name:        "add container to network with specific IP",
			containerID: "container-001",
			networkID:   "network-001",
			ip:          net.ParseIP("10.0.1.5"),
			wantErr:     false,
		},
		{
			name:        "add container with empty container ID",
			containerID: "",
			networkID:   "network-001",
			ip:          net.ParseIP("10.0.1.6"),
			wantErr:     true,
		},
		{
			name:        "add container with empty network ID",
			containerID: "container-002",
			networkID:   "",
			ip:          net.ParseIP("10.0.1.7"),
			wantErr:     true,
		},
		{
			name:        "add container with nil IP (should auto-assign)",
			containerID: "container-003",
			networkID:   "network-001",
			ip:          nil,
			wantErr:     false,
		},
		{
			name:        "add same container twice (should fail)",
			containerID: "container-001", // Already added above
			networkID:   "network-001",
			ip:          net.ParseIP("10.0.1.8"),
			wantErr:     true,
		},
		{
			name:        "add container with IP outside network range",
			containerID: "container-004",
			networkID:   "network-001",
			ip:          net.ParseIP("192.168.1.1"), // Outside 10.0.0.0/16
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runtime.AddToNetwork(ctx, tt.containerID, tt.networkID, tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddToNetwork() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCNIRuntime_RemoveFromNetwork(t *testing.T) {
	ctx := context.Background()
	runtime := cni.NewRuntime()

	// First add a container to remove
	runtime.AddToNetwork(ctx, "remove-test-001", "network-001", net.ParseIP("10.0.1.10"))

	tests := []struct {
		name        string
		containerID string
		networkID   string
		wantErr     bool
	}{
		{
			name:        "remove existing container",
			containerID: "remove-test-001",
			networkID:   "network-001",
			wantErr:     false,
		},
		{
			name:        "remove non-existent container",
			containerID: "non-existent",
			networkID:   "network-001",
			wantErr:     true,
		},
		{
			name:        "remove with empty container ID",
			containerID: "",
			networkID:   "network-001",
			wantErr:     true,
		},
		{
			name:        "remove with empty network ID",
			containerID: "remove-test-001",
			networkID:   "",
			wantErr:     true,
		},
		{
			name:        "remove already removed container",
			containerID: "remove-test-001", // Already removed above
			networkID:   "network-001",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runtime.RemoveFromNetwork(ctx, tt.containerID, tt.networkID)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveFromNetwork() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCNIRuntime_SetupPlugin(t *testing.T) {
	ctx := context.Background()
	runtime := cni.NewRuntime()

	flannelConfig := []byte(`{
		"name": "flannel",
		"type": "flannel",
		"subnetFile": "/run/flannel/subnet.env",
		"dataDir": "/var/lib/cni/flannel",
		"delegate": {
			"hairpinMode": true,
			"isDefaultGateway": true
		}
	}`)

	calicoConfig := []byte(`{
		"name": "calico",
		"type": "calico",
		"etcd_endpoints": "http://localhost:2379",
		"log_level": "info"
	}`)

	invalidConfig := []byte(`invalid json`)

	tests := []struct {
		name    string
		plugin  string
		config  []byte
		wantErr bool
	}{
		{
			name:    "setup flannel plugin",
			plugin:  "flannel",
			config:  flannelConfig,
			wantErr: false,
		},
		{
			name:    "setup calico plugin",
			plugin:  "calico",
			config:  calicoConfig,
			wantErr: false,
		},
		{
			name:    "setup with invalid config",
			plugin:  "flannel",
			config:  invalidConfig,
			wantErr: true,
		},
		{
			name:    "setup unsupported plugin",
			plugin:  "unsupported",
			config:  []byte(`{}`),
			wantErr: true,
		},
		{
			name:    "setup with empty plugin name",
			plugin:  "",
			config:  flannelConfig,
			wantErr: true,
		},
		{
			name:    "setup with nil config",
			plugin:  "flannel",
			config:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runtime.SetupPlugin(ctx, tt.plugin, tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetupPlugin() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCNIRuntime_GetPluginStatus(t *testing.T) {
	ctx := context.Background()
	runtime := cni.NewRuntime()

	// Setup a plugin first
	runtime.SetupPlugin(ctx, "flannel", []byte(`{"name": "flannel", "type": "flannel"}`))

	tests := []struct {
		name    string
		plugin  string
		wantErr bool
		checks  func(t *testing.T, status *vpc.PluginStatus)
	}{
		{
			name:    "get status of configured plugin",
			plugin:  "flannel",
			wantErr: false,
			checks: func(t *testing.T, status *vpc.PluginStatus) {
				if status == nil {
					t.Fatal("expected status, got nil")
				}
				if status.Name != "flannel" {
					t.Errorf("expected name flannel, got %s", status.Name)
				}
				if status.Status != "active" {
					t.Errorf("expected status active, got %s", status.Status)
				}
			},
		},
		{
			name:    "get status of non-configured plugin",
			plugin:  "calico",
			wantErr: false,
			checks: func(t *testing.T, status *vpc.PluginStatus) {
				if status == nil {
					t.Fatal("expected status, got nil")
				}
				if status.Status != "inactive" {
					t.Errorf("expected status inactive, got %s", status.Status)
				}
			},
		},
		{
			name:    "get status with empty plugin name",
			plugin:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := runtime.GetPluginStatus(ctx, tt.plugin)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPluginStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.checks != nil {
				tt.checks(t, status)
			}
		})
	}
}
