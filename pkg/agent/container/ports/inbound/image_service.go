package inbound

import (
	"context"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
)

// ImageService defines the interface for image operations.
// This is an inbound port that driving adapters use to manage container images.
type ImageService interface {
	// Image operations
	PullImage(ctx context.Context, ref domain.ImageRef, opts PullOptions) (*domain.ImageInfo, error)
	ListImages(ctx context.Context) ([]*domain.ImageInfo, error)
	RemoveImage(ctx context.Context, id domain.ImageID, force bool) error
	InspectImage(ctx context.Context, id domain.ImageID) (*domain.ImageInfo, error)

	// Image existence
	ImageExists(ctx context.Context, ref domain.ImageRef) (bool, error)
}

// PullOptions contains options for image pull operation.
type PullOptions struct {
	AuthConfig *AuthConfig
	Progress   ProgressFunc
	Platform   string
}

// AuthConfig contains registry authentication credentials.
type AuthConfig struct {
	Username      string
	Password      string
	Auth          string
	Email         string
	ServerAddress string
	IdentityToken string
	RegistryToken string
}

// ProgressFunc is a callback for reporting progress.
type ProgressFunc func(status string, progress int, total int)
