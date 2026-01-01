package outbound

import (
	"context"
	"time"
)

// ImageRegistry handles image pull/push operations.
// This is an outbound port that use cases use to manage container images.
type ImageRegistry interface {
	Pull(ctx context.Context, ref string, opts ImagePullOptions) error
	Push(ctx context.Context, ref string, opts ImagePushOptions) error
	List(ctx context.Context) ([]*RuntimeImageInfo, error)
	Remove(ctx context.Context, imageID string, force bool) error
	Inspect(ctx context.Context, imageRef string) (*RuntimeImageInfo, error)
	ImageExists(ctx context.Context, ref string) (bool, error)
}

// ImagePullOptions contains options for pulling an image.
type ImagePullOptions struct {
	AuthConfig *RegistryAuthConfig
	Progress   ImageProgressFunc
	Platform   string
}

// ImagePushOptions contains options for pushing an image.
type ImagePushOptions struct {
	AuthConfig *RegistryAuthConfig
	Tag        string
}

// RegistryAuthConfig contains registry authentication credentials.
type RegistryAuthConfig struct {
	Username      string
	Password      string
	Auth          string
	Email         string
	ServerAddress string
	IdentityToken string
	RegistryToken string
}

// ImageProgressFunc is a callback for reporting image operation progress.
type ImageProgressFunc func(status string, progress int, total int)

// RuntimeImageInfo contains information about an image.
type RuntimeImageInfo struct {
	Created     time.Time
	Labels      map[string]string
	ID          string
	Digest      string
	RepoTags    []string
	RepoDigests []string
	Size        int64
}
