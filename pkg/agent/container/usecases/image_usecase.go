package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/fertile-org/banyan/pkg/agent/container/domain"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/inbound"
	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// ImageUseCase implements the ImageService interface.
type ImageUseCase struct {
	registry outbound.ImageRegistry
}

// NewImageUseCase creates a new ImageUseCase.
func NewImageUseCase(registry outbound.ImageRegistry) *ImageUseCase {
	return &ImageUseCase{
		registry: registry,
	}
}

// PullImage pulls an image from a registry.
func (uc *ImageUseCase) PullImage(ctx context.Context, ref domain.ImageRef, opts inbound.PullOptions) (*domain.ImageInfo, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}

	pullOpts := outbound.ImagePullOptions{
		Platform: opts.Platform,
	}

	// Convert auth config
	if opts.AuthConfig != nil {
		pullOpts.AuthConfig = &outbound.RegistryAuthConfig{
			Username:      opts.AuthConfig.Username,
			Password:      opts.AuthConfig.Password,
			Auth:          opts.AuthConfig.Auth,
			Email:         opts.AuthConfig.Email,
			ServerAddress: opts.AuthConfig.ServerAddress,
			IdentityToken: opts.AuthConfig.IdentityToken,
			RegistryToken: opts.AuthConfig.RegistryToken,
		}
	}

	// Convert progress callback
	if opts.Progress != nil {
		pullOpts.Progress = func(status string, progress int, total int) {
			opts.Progress(status, progress, total)
		}
	}

	if err := uc.registry.Pull(ctx, ref.String(), pullOpts); err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Inspect the pulled image
	info, err := uc.registry.Inspect(ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("failed to inspect pulled image: %w", err)
	}

	return uc.toImageInfo(info), nil
}

// ListImages lists all local images.
func (uc *ImageUseCase) ListImages(ctx context.Context) ([]*domain.ImageInfo, error) {
	images, err := uc.registry.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	result := make([]*domain.ImageInfo, 0, len(images))
	for _, img := range images {
		result = append(result, uc.toImageInfo(img))
	}

	return result, nil
}

// RemoveImage removes an image.
func (uc *ImageUseCase) RemoveImage(ctx context.Context, id domain.ImageID, force bool) error {
	if err := id.Validate(); err != nil {
		return err
	}

	if err := uc.registry.Remove(ctx, string(id), force); err != nil {
		return fmt.Errorf("failed to remove image: %w", err)
	}

	return nil
}

// InspectImage returns information about an image.
func (uc *ImageUseCase) InspectImage(ctx context.Context, id domain.ImageID) (*domain.ImageInfo, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}

	info, err := uc.registry.Inspect(ctx, string(id))
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	return uc.toImageInfo(info), nil
}

// ImageExists checks if an image exists locally.
func (uc *ImageUseCase) ImageExists(ctx context.Context, ref domain.ImageRef) (bool, error) {
	if err := ref.Validate(); err != nil {
		return false, err
	}

	return uc.registry.ImageExists(ctx, ref.String())
}

// toImageInfo converts runtime image info to domain image info.
func (uc *ImageUseCase) toImageInfo(info *outbound.RuntimeImageInfo) *domain.ImageInfo {
	var created time.Time
	if !info.Created.IsZero() {
		created = info.Created
	}

	return &domain.ImageInfo{
		ID:          domain.ImageID(info.ID),
		Digest:      info.Digest,
		RepoTags:    info.RepoTags,
		RepoDigests: info.RepoDigests,
		Size:        info.Size,
		Created:     created,
		Labels:      info.Labels,
	}
}
