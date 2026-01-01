package usecases

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// mockContainerRuntime is a mock implementation of outbound.ContainerRuntime.
type mockContainerRuntime struct {
	createFunc          func(ctx context.Context, config *outbound.RuntimeContainerConfig) (string, error)
	startFunc           func(ctx context.Context, containerID string) error
	stopFunc            func(ctx context.Context, containerID string, timeout time.Duration) error
	removeFunc          func(ctx context.Context, containerID string, force bool) error
	inspectFunc         func(ctx context.Context, containerID string) (*outbound.RuntimeContainerInfo, error)
	listFunc            func(ctx context.Context, opts outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error)
	logsFunc            func(ctx context.Context, containerID string, opts outbound.RuntimeLogOptions) (io.ReadCloser, error)
	statsFunc           func(ctx context.Context, containerID string) (*outbound.RuntimeStats, error)
	execCreateFunc      func(ctx context.Context, containerID string, config *outbound.RuntimeExecConfig) (string, error)
	execStartFunc       func(ctx context.Context, execID string, opts outbound.ExecStartOptions) (io.ReadWriteCloser, error)
	execInspectFunc     func(ctx context.Context, execID string) (*outbound.ExecInspect, error)
	copyToContainerFunc func(ctx context.Context, containerID, path string, content io.Reader) error
	copyFromFunc        func(ctx context.Context, containerID, path string) (io.ReadCloser, error)
}

func (m *mockContainerRuntime) Create(ctx context.Context, config *outbound.RuntimeContainerConfig) (string, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, config)
	}
	return "container123456", nil
}

func (m *mockContainerRuntime) Start(ctx context.Context, containerID string) error {
	if m.startFunc != nil {
		return m.startFunc(ctx, containerID)
	}
	return nil
}

func (m *mockContainerRuntime) Stop(ctx context.Context, containerID string, timeout time.Duration) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx, containerID, timeout)
	}
	return nil
}

func (m *mockContainerRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, containerID, force)
	}
	return nil
}

func (m *mockContainerRuntime) Kill(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockContainerRuntime) Pause(_ context.Context, _ string) error {
	return nil
}

func (m *mockContainerRuntime) Unpause(_ context.Context, _ string) error {
	return nil
}

func (m *mockContainerRuntime) Inspect(ctx context.Context, containerID string) (*outbound.RuntimeContainerInfo, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, containerID)
	}
	return &outbound.RuntimeContainerInfo{
		ID:     containerID,
		Name:   "test-container",
		State:  "running",
		Status: "Up 1 hour",
		Image:  "nginx:latest",
	}, nil
}

func (m *mockContainerRuntime) List(ctx context.Context, opts outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, opts)
	}
	return nil, nil
}

func (m *mockContainerRuntime) Logs(ctx context.Context, containerID string, opts outbound.RuntimeLogOptions) (io.ReadCloser, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, containerID, opts)
	}
	return io.NopCloser(strings.NewReader("test logs")), nil
}

func (m *mockContainerRuntime) Stats(ctx context.Context, containerID string) (*outbound.RuntimeStats, error) {
	if m.statsFunc != nil {
		return m.statsFunc(ctx, containerID)
	}
	return &outbound.RuntimeStats{
		CPUPercent:  10.5,
		MemoryUsage: 1024 * 1024 * 100,
		MemoryLimit: 1024 * 1024 * 512,
	}, nil
}

func (m *mockContainerRuntime) Wait(_ context.Context, _ string) (<-chan outbound.WaitResult, error) {
	ch := make(chan outbound.WaitResult, 1)
	ch <- outbound.WaitResult{StatusCode: 0}
	return ch, nil
}

func (m *mockContainerRuntime) ExecCreate(ctx context.Context, containerID string, config *outbound.RuntimeExecConfig) (string, error) {
	if m.execCreateFunc != nil {
		return m.execCreateFunc(ctx, containerID, config)
	}
	return "exec123", nil
}

func (m *mockContainerRuntime) ExecStart(ctx context.Context, execID string, opts outbound.ExecStartOptions) (io.ReadWriteCloser, error) {
	if m.execStartFunc != nil {
		return m.execStartFunc(ctx, execID, opts)
	}
	return &mockReadWriteCloser{Reader: strings.NewReader("exec output")}, nil
}

func (m *mockContainerRuntime) ExecInspect(ctx context.Context, execID string) (*outbound.ExecInspect, error) {
	if m.execInspectFunc != nil {
		return m.execInspectFunc(ctx, execID)
	}
	return &outbound.ExecInspect{ExecID: execID, ExitCode: 0, Running: false}, nil
}

func (m *mockContainerRuntime) CopyToContainer(ctx context.Context, containerID, path string, content io.Reader) error {
	if m.copyToContainerFunc != nil {
		return m.copyToContainerFunc(ctx, containerID, path, content)
	}
	return nil
}

func (m *mockContainerRuntime) CopyFromContainer(ctx context.Context, containerID, path string) (io.ReadCloser, error) {
	if m.copyFromFunc != nil {
		return m.copyFromFunc(ctx, containerID, path)
	}
	return io.NopCloser(strings.NewReader("file content")), nil
}

// mockReadWriteCloser implements io.ReadWriteCloser for testing.
type mockReadWriteCloser struct {
	io.Reader
}

func (m *mockReadWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (m *mockReadWriteCloser) Close() error {
	return nil
}

// mockNetworkConnector is a mock implementation of outbound.NetworkConnector.
type mockNetworkConnector struct {
	connectFunc    func(ctx context.Context, containerID, networkID, ip string) error
	disconnectFunc func(ctx context.Context, containerID, networkID string) error
	getNetworkFunc func(ctx context.Context, containerID string) (*outbound.ContainerNetworkInfo, error)
}

func (m *mockNetworkConnector) ConnectContainer(ctx context.Context, containerID, networkID, ip string) error {
	if m.connectFunc != nil {
		return m.connectFunc(ctx, containerID, networkID, ip)
	}
	return nil
}

func (m *mockNetworkConnector) DisconnectContainer(ctx context.Context, containerID, networkID string) error {
	if m.disconnectFunc != nil {
		return m.disconnectFunc(ctx, containerID, networkID)
	}
	return nil
}

func (m *mockNetworkConnector) GetContainerNetwork(ctx context.Context, containerID string) (*outbound.ContainerNetworkInfo, error) {
	if m.getNetworkFunc != nil {
		return m.getNetworkFunc(ctx, containerID)
	}
	return &outbound.ContainerNetworkInfo{
		IPAddress: "10.0.0.5",
		NetworkID: "network123",
	}, nil
}

// mockEventEmitter is a mock implementation of outbound.EventEmitter.
type mockEventEmitter struct {
	events []interface{}
}

func (m *mockEventEmitter) Emit(event interface{}) {
	m.events = append(m.events, event)
}

func TestContainerUseCase_CreateContainer(t *testing.T) {
	tests := []struct {
		name         string
		spec         *inbound.ContainerSpec
		setupRuntime func(*mockContainerRuntime)
		setupImages  func(*mockImageRegistry)
		setupNetwork func(*mockNetworkConnector)
		wantErr      bool
		errContains  string
	}{
		{
			name: "successful creation with existing image",
			spec: &inbound.ContainerSpec{
				Name:      "test-container",
				ServiceID: "service-123",
				NetworkID: "network-123",
				IPAddress: "10.0.0.5",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{
						Registry: "docker.io",
						Name:     "library/nginx",
						Tag:      "latest",
					},
				},
			},
			setupImages: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return true, nil
				}
			},
			setupRuntime: func(m *mockContainerRuntime) {
				m.createFunc = func(_ context.Context, _ *outbound.RuntimeContainerConfig) (string, error) {
					return "container123456", nil
				}
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeContainerInfo, error) {
					return &outbound.RuntimeContainerInfo{
						ID:      "container123456",
						Name:    "test-container",
						State:   "created",
						Status:  "Created",
						Image:   "nginx:latest",
						Created: time.Now().Format(time.RFC3339),
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name: "successful creation with image pull",
			spec: &inbound.ContainerSpec{
				Name:      "test-container",
				ServiceID: "service-123",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{
						Registry: "docker.io",
						Name:     "library/nginx",
						Tag:      "latest",
					},
				},
			},
			setupImages: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return false, nil
				}
				m.pullFunc = func(_ context.Context, _ string, _ outbound.ImagePullOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:        "nil spec",
			spec:        nil,
			wantErr:     true,
			errContains: "container spec is required",
		},
		{
			name: "invalid config - empty image",
			spec: &inbound.ContainerSpec{
				Name: "test-container",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{},
				},
			},
			wantErr: true,
		},
		{
			name: "image pull failure",
			spec: &inbound.ContainerSpec{
				Name: "test-container",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{
						Registry: "docker.io",
						Name:     "library/nginx",
						Tag:      "latest",
					},
				},
			},
			setupImages: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return false, nil
				}
				m.pullFunc = func(_ context.Context, _ string, _ outbound.ImagePullOptions) error {
					return errors.New("pull failed")
				}
			},
			wantErr:     true,
			errContains: "failed to pull image",
		},
		{
			name: "runtime create failure",
			spec: &inbound.ContainerSpec{
				Name: "test-container",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{
						Registry: "docker.io",
						Name:     "library/nginx",
						Tag:      "latest",
					},
				},
			},
			setupImages: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return true, nil
				}
			},
			setupRuntime: func(m *mockContainerRuntime) {
				m.createFunc = func(_ context.Context, _ *outbound.RuntimeContainerConfig) (string, error) {
					return "", errors.New("create failed")
				}
			},
			wantErr:     true,
			errContains: "failed to create container",
		},
		{
			name: "network connect failure",
			spec: &inbound.ContainerSpec{
				Name:      "test-container",
				NetworkID: "network-123",
				IPAddress: "10.0.0.5",
				Config: domain.ContainerConfig{
					Image: domain.ImageRef{
						Registry: "docker.io",
						Name:     "library/nginx",
						Tag:      "latest",
					},
				},
			},
			setupImages: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return true, nil
				}
			},
			setupNetwork: func(m *mockNetworkConnector) {
				m.connectFunc = func(_ context.Context, _, _, _ string) error {
					return errors.New("network connect failed")
				}
			},
			wantErr:     true,
			errContains: "failed to connect to network",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			images := &mockImageRegistry{}
			network := &mockNetworkConnector{}
			events := &mockEventEmitter{}

			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}
			if tt.setupImages != nil {
				tt.setupImages(images)
			}
			if tt.setupNetwork != nil {
				tt.setupNetwork(network)
			}

			uc := NewContainerUseCase(runtime, network, events, images)
			result, err := uc.CreateContainer(context.Background(), tt.spec)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CreateContainer() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("CreateContainer() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("CreateContainer() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("CreateContainer() returned nil result")
			}
		})
	}
}

func TestContainerUseCase_StartContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name: "successful start",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.inspectFunc = func(_ context.Context, id string) (*outbound.RuntimeContainerInfo, error) {
					return &outbound.RuntimeContainerInfo{
						ID:    id,
						Name:  "test-container",
						State: "created", // Not running, so start should succeed
					}, nil
				}
				m.startFunc = func(_ context.Context, _ string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			wantErr: true,
		},
		{
			name: "start failure",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.startFunc = func(_ context.Context, _ string) error {
					return errors.New("container not found")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			err := uc.StartContainer(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Errorf("StartContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("StartContainer() unexpected error: %v", err)
			}
		})
	}
}

func TestContainerUseCase_StopContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		timeout      time.Duration
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name:    "successful stop",
			id:      domain.ContainerID("container123456"),
			timeout: 30 * time.Second,
			setupRuntime: func(m *mockContainerRuntime) {
				m.stopFunc = func(_ context.Context, _ string, _ time.Duration) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			timeout: 30 * time.Second,
			wantErr: true,
		},
		{
			name:    "stop failure",
			id:      domain.ContainerID("container123456"),
			timeout: 30 * time.Second,
			setupRuntime: func(m *mockContainerRuntime) {
				m.stopFunc = func(_ context.Context, _ string, _ time.Duration) error {
					return errors.New("stop failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			err := uc.StopContainer(context.Background(), tt.id, tt.timeout)

			if tt.wantErr {
				if err == nil {
					t.Errorf("StopContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("StopContainer() unexpected error: %v", err)
			}
		})
	}
}

func TestContainerUseCase_RemoveContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		force        bool
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name:  "successful remove",
			id:    domain.ContainerID("container123456"),
			force: false,
			setupRuntime: func(m *mockContainerRuntime) {
				m.removeFunc = func(_ context.Context, _ string, _ bool) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:  "force remove",
			id:    domain.ContainerID("container123456"),
			force: true,
			setupRuntime: func(m *mockContainerRuntime) {
				m.removeFunc = func(_ context.Context, _ string, force bool) error {
					if !force {
						return errors.New("expected force flag")
					}
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			force:   false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			err := uc.RemoveContainer(context.Background(), tt.id, tt.force)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemoveContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RemoveContainer() unexpected error: %v", err)
			}
		})
	}
}

func TestContainerUseCase_GetContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name: "successful get",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.inspectFunc = func(_ context.Context, id string) (*outbound.RuntimeContainerInfo, error) {
					return &outbound.RuntimeContainerInfo{
						ID:     id,
						Name:   "test-container",
						State:  "running",
						Status: "Up 1 hour",
						Image:  "nginx:latest",
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			wantErr: true,
		},
		{
			name: "container not found",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeContainerInfo, error) {
					return nil, errors.New("container not found")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			result, err := uc.GetContainer(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetContainer() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("GetContainer() returned nil result")
			}
		})
	}
}

func TestContainerUseCase_ListContainers(t *testing.T) {
	tests := []struct {
		name         string
		filter       *inbound.ContainerFilter
		setupRuntime func(*mockContainerRuntime)
		wantCount    int
		wantErr      bool
	}{
		{
			name:   "list all containers",
			filter: &inbound.ContainerFilter{All: true},
			setupRuntime: func(m *mockContainerRuntime) {
				m.listFunc = func(_ context.Context, _ outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
					return []*outbound.RuntimeContainerInfo{
						{ID: "container1", Name: "test1", State: "running"},
						{ID: "container2", Name: "test2", State: "exited"},
					}, nil
				}
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:   "empty list",
			filter: &inbound.ContainerFilter{},
			setupRuntime: func(m *mockContainerRuntime) {
				m.listFunc = func(_ context.Context, _ outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
					return []*outbound.RuntimeContainerInfo{}, nil
				}
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:   "nil filter",
			filter: nil,
			setupRuntime: func(m *mockContainerRuntime) {
				m.listFunc = func(_ context.Context, _ outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
					return []*outbound.RuntimeContainerInfo{
						{ID: "container1", Name: "test1", State: "running"},
					}, nil
				}
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:   "list error",
			filter: &inbound.ContainerFilter{},
			setupRuntime: func(m *mockContainerRuntime) {
				m.listFunc = func(_ context.Context, _ outbound.ListOptions) ([]*outbound.RuntimeContainerInfo, error) {
					return nil, errors.New("list failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			result, err := uc.ListContainers(context.Background(), tt.filter)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ListContainers() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ListContainers() unexpected error: %v", err)
				return
			}

			if len(result) != tt.wantCount {
				t.Errorf("ListContainers() count = %d, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestContainerUseCase_GetStats(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name: "successful stats",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.statsFunc = func(_ context.Context, _ string) (*outbound.RuntimeStats, error) {
					return &outbound.RuntimeStats{
						CPUPercent:  25.5,
						MemoryUsage: 1024 * 1024 * 100,
						MemoryLimit: 1024 * 1024 * 512,
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			wantErr: true,
		},
		{
			name: "stats error",
			id:   domain.ContainerID("container123456"),
			setupRuntime: func(m *mockContainerRuntime) {
				m.statsFunc = func(_ context.Context, _ string) (*outbound.RuntimeStats, error) {
					return nil, errors.New("stats failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			result, err := uc.GetStats(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetStats() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("GetStats() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("GetStats() returned nil result")
			}
		})
	}
}

func TestContainerUseCase_CopyToContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		path         string
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name: "successful copy",
			id:   domain.ContainerID("container123456"),
			path: "/app/config.json",
			setupRuntime: func(m *mockContainerRuntime) {
				m.copyToContainerFunc = func(_ context.Context, _, _ string, _ io.Reader) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			path:    "/app/config.json",
			wantErr: true,
		},
		{
			name: "copy error",
			id:   domain.ContainerID("container123456"),
			path: "/app/config.json",
			setupRuntime: func(m *mockContainerRuntime) {
				m.copyToContainerFunc = func(_ context.Context, _, _ string, _ io.Reader) error {
					return errors.New("copy failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			err := uc.CopyToContainer(context.Background(), tt.id, tt.path, strings.NewReader("test content"))

			if tt.wantErr {
				if err == nil {
					t.Errorf("CopyToContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("CopyToContainer() unexpected error: %v", err)
			}
		})
	}
}

func TestContainerUseCase_CopyFromContainer(t *testing.T) {
	tests := []struct {
		name         string
		id           domain.ContainerID
		path         string
		setupRuntime func(*mockContainerRuntime)
		wantErr      bool
	}{
		{
			name: "successful copy",
			id:   domain.ContainerID("container123456"),
			path: "/app/data.txt",
			setupRuntime: func(m *mockContainerRuntime) {
				m.copyFromFunc = func(_ context.Context, _, _ string) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("file content")), nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid container ID",
			id:      domain.ContainerID("abc"),
			path:    "/app/data.txt",
			wantErr: true,
		},
		{
			name: "copy error",
			id:   domain.ContainerID("container123456"),
			path: "/app/data.txt",
			setupRuntime: func(m *mockContainerRuntime) {
				m.copyFromFunc = func(_ context.Context, _, _ string) (io.ReadCloser, error) {
					return nil, errors.New("file not found")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockContainerRuntime{}
			if tt.setupRuntime != nil {
				tt.setupRuntime(runtime)
			}

			uc := NewContainerUseCase(runtime, &mockNetworkConnector{}, &mockEventEmitter{}, &mockImageRegistry{})
			result, err := uc.CopyFromContainer(context.Background(), tt.id, tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CopyFromContainer() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("CopyFromContainer() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("CopyFromContainer() returned nil result")
			}
			if result != nil {
				result.Close()
			}
		})
	}
}
