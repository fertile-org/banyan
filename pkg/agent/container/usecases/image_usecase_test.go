package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// mockImageRegistry is a mock implementation of outbound.ImageRegistry for testing.
type mockImageRegistry struct {
	pullFunc        func(ctx context.Context, ref string, opts outbound.ImagePullOptions) error
	listFunc        func(ctx context.Context) ([]*outbound.RuntimeImageInfo, error)
	removeFunc      func(ctx context.Context, imageID string, force bool) error
	inspectFunc     func(ctx context.Context, imageRef string) (*outbound.RuntimeImageInfo, error)
	imageExistsFunc func(ctx context.Context, ref string) (bool, error)
}

func (m *mockImageRegistry) Pull(ctx context.Context, ref string, opts outbound.ImagePullOptions) error {
	if m.pullFunc != nil {
		return m.pullFunc(ctx, ref, opts)
	}
	return nil
}

func (m *mockImageRegistry) Push(_ context.Context, _ string, _ outbound.ImagePushOptions) error {
	return nil
}

func (m *mockImageRegistry) List(ctx context.Context) ([]*outbound.RuntimeImageInfo, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return nil, nil
}

func (m *mockImageRegistry) Remove(ctx context.Context, imageID string, force bool) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, imageID, force)
	}
	return nil
}

func (m *mockImageRegistry) Inspect(ctx context.Context, imageRef string) (*outbound.RuntimeImageInfo, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, imageRef)
	}
	return &outbound.RuntimeImageInfo{
		ID:       "sha256:abc123",
		RepoTags: []string{"nginx:latest"},
		Size:     1024,
		Created:  time.Now(),
	}, nil
}

func (m *mockImageRegistry) ImageExists(ctx context.Context, ref string) (bool, error) {
	if m.imageExistsFunc != nil {
		return m.imageExistsFunc(ctx, ref)
	}
	return true, nil
}

func TestImageUseCase_PullImage(t *testing.T) {
	tests := []struct {
		name        string
		ref         domain.ImageRef
		opts        inbound.PullOptions
		setupMock   func(*mockImageRegistry)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful pull",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			opts: inbound.PullOptions{},
			setupMock: func(m *mockImageRegistry) {
				m.pullFunc = func(_ context.Context, _ string, _ outbound.ImagePullOptions) error {
					return nil
				}
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeImageInfo, error) {
					return &outbound.RuntimeImageInfo{
						ID:       "sha256:abc123",
						RepoTags: []string{"nginx:latest"},
						Size:     1024,
						Created:  time.Now(),
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name: "pull with auth config",
			ref: domain.ImageRef{
				Registry: "private.registry.io",
				Name:     "myapp",
				Tag:      "v1",
			},
			opts: inbound.PullOptions{
				AuthConfig: &inbound.AuthConfig{
					Username: "user",
					Password: "pass",
				},
			},
			setupMock: func(m *mockImageRegistry) {
				m.pullFunc = func(_ context.Context, _ string, opts outbound.ImagePullOptions) error {
					if opts.AuthConfig == nil || opts.AuthConfig.Username != "user" {
						return errors.New("auth config not passed correctly")
					}
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "invalid image ref - empty name",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "",
				Tag:      "latest",
			},
			opts:        inbound.PullOptions{},
			wantErr:     true,
			errContains: "invalid image",
		},
		{
			name: "pull failure",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			opts: inbound.PullOptions{},
			setupMock: func(m *mockImageRegistry) {
				m.pullFunc = func(_ context.Context, _ string, _ outbound.ImagePullOptions) error {
					return errors.New("network error")
				}
			},
			wantErr:     true,
			errContains: "failed to pull image",
		},
		{
			name: "inspect failure after pull",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			opts: inbound.PullOptions{},
			setupMock: func(m *mockImageRegistry) {
				m.pullFunc = func(_ context.Context, _ string, _ outbound.ImagePullOptions) error {
					return nil
				}
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeImageInfo, error) {
					return nil, errors.New("inspect error")
				}
			},
			wantErr:     true,
			errContains: "failed to inspect pulled image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockImageRegistry{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			uc := NewImageUseCase(mock)
			result, err := uc.PullImage(context.Background(), tt.ref, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("PullImage() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("PullImage() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("PullImage() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("PullImage() returned nil result")
			}
		})
	}
}

func TestImageUseCase_ListImages(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mockImageRegistry)
		wantCount int
		wantErr   bool
	}{
		{
			name: "list images successfully",
			setupMock: func(m *mockImageRegistry) {
				m.listFunc = func(_ context.Context) ([]*outbound.RuntimeImageInfo, error) {
					return []*outbound.RuntimeImageInfo{
						{ID: "sha256:abc123", RepoTags: []string{"nginx:latest"}, Size: 1024},
						{ID: "sha256:def456", RepoTags: []string{"redis:alpine"}, Size: 2048},
					}, nil
				}
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "empty image list",
			setupMock: func(m *mockImageRegistry) {
				m.listFunc = func(_ context.Context) ([]*outbound.RuntimeImageInfo, error) {
					return []*outbound.RuntimeImageInfo{}, nil
				}
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "list error",
			setupMock: func(m *mockImageRegistry) {
				m.listFunc = func(_ context.Context) ([]*outbound.RuntimeImageInfo, error) {
					return nil, errors.New("list error")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockImageRegistry{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			uc := NewImageUseCase(mock)
			result, err := uc.ListImages(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Errorf("ListImages() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ListImages() unexpected error: %v", err)
				return
			}

			if len(result) != tt.wantCount {
				t.Errorf("ListImages() count = %d, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestImageUseCase_RemoveImage(t *testing.T) {
	tests := []struct {
		name      string
		id        domain.ImageID
		force     bool
		setupMock func(*mockImageRegistry)
		wantErr   bool
	}{
		{
			name:  "remove image successfully",
			id:    domain.ImageID("sha256:abc123"),
			force: false,
			setupMock: func(m *mockImageRegistry) {
				m.removeFunc = func(_ context.Context, _ string, _ bool) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:  "force remove image",
			id:    domain.ImageID("sha256:abc123"),
			force: true,
			setupMock: func(m *mockImageRegistry) {
				m.removeFunc = func(_ context.Context, _ string, force bool) error {
					if !force {
						return errors.New("force flag not passed")
					}
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid image ID",
			id:      domain.ImageID(""),
			force:   false,
			wantErr: true,
		},
		{
			name:  "remove error",
			id:    domain.ImageID("sha256:abc123"),
			force: false,
			setupMock: func(m *mockImageRegistry) {
				m.removeFunc = func(_ context.Context, _ string, _ bool) error {
					return errors.New("image in use")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockImageRegistry{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			uc := NewImageUseCase(mock)
			err := uc.RemoveImage(context.Background(), tt.id, tt.force)

			if tt.wantErr {
				if err == nil {
					t.Errorf("RemoveImage() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("RemoveImage() unexpected error: %v", err)
			}
		})
	}
}

func TestImageUseCase_InspectImage(t *testing.T) {
	tests := []struct {
		name      string
		id        domain.ImageID
		setupMock func(*mockImageRegistry)
		wantErr   bool
	}{
		{
			name: "inspect image successfully",
			id:   domain.ImageID("sha256:abc123"),
			setupMock: func(m *mockImageRegistry) {
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeImageInfo, error) {
					return &outbound.RuntimeImageInfo{
						ID:       "sha256:abc123",
						RepoTags: []string{"nginx:latest"},
						Size:     1024,
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:    "invalid image ID",
			id:      domain.ImageID(""),
			wantErr: true,
		},
		{
			name: "inspect error",
			id:   domain.ImageID("sha256:notfound"),
			setupMock: func(m *mockImageRegistry) {
				m.inspectFunc = func(_ context.Context, _ string) (*outbound.RuntimeImageInfo, error) {
					return nil, errors.New("image not found")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockImageRegistry{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			uc := NewImageUseCase(mock)
			result, err := uc.InspectImage(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Errorf("InspectImage() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("InspectImage() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Errorf("InspectImage() returned nil result")
			}
		})
	}
}

func TestImageUseCase_ImageExists(t *testing.T) {
	tests := []struct {
		name      string
		ref       domain.ImageRef
		setupMock func(*mockImageRegistry)
		want      bool
		wantErr   bool
	}{
		{
			name: "image exists",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "latest",
			},
			setupMock: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return true, nil
				}
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "image does not exist",
			ref: domain.ImageRef{
				Registry: "docker.io",
				Name:     "library/nginx",
				Tag:      "nonexistent",
			},
			setupMock: func(m *mockImageRegistry) {
				m.imageExistsFunc = func(_ context.Context, _ string) (bool, error) {
					return false, nil
				}
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "invalid ref",
			ref: domain.ImageRef{
				Name: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockImageRegistry{}
			if tt.setupMock != nil {
				tt.setupMock(mock)
			}

			uc := NewImageUseCase(mock)
			got, err := uc.ImageExists(context.Background(), tt.ref)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ImageExists() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("ImageExists() unexpected error: %v", err)
				return
			}

			if got != tt.want {
				t.Errorf("ImageExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

// containsString checks if s contains substr.
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
