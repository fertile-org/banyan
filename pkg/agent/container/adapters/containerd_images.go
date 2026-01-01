package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/remotes/docker"

	"github.com/fertile-org/banyan/pkg/agent/container/ports/outbound"
)

// ContainerdImageRegistry implements outbound.ImageRegistry using containerd.
type ContainerdImageRegistry struct {
	client    *containerd.Client
	namespace string
}

// NewContainerdImageRegistry creates a new containerd image registry adapter.
func NewContainerdImageRegistry(client *containerd.Client, namespace string) *ContainerdImageRegistry {
	if namespace == "" {
		namespace = defaultNamespace
	}
	return &ContainerdImageRegistry{
		client:    client,
		namespace: namespace,
	}
}

// Pull pulls an image from a registry.
func (r *ContainerdImageRegistry) Pull(ctx context.Context, ref string, opts outbound.ImagePullOptions) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	pullOpts := []containerd.RemoteOpt{
		containerd.WithPullUnpack,
	}

	// Add authentication if provided
	if opts.AuthConfig != nil && opts.AuthConfig.Username != "" {
		resolver := docker.NewResolver(docker.ResolverOptions{
			Authorizer: docker.NewDockerAuthorizer(
				docker.WithAuthCreds(func(_ string) (string, string, error) {
					return opts.AuthConfig.Username, opts.AuthConfig.Password, nil
				}),
			),
		})
		pullOpts = append(pullOpts, containerd.WithResolver(resolver))
	}

	// Add platform if specified
	if opts.Platform != "" {
		pullOpts = append(pullOpts, containerd.WithPlatform(opts.Platform))
	}

	_, err := r.client.Pull(ctx, ref, pullOpts...)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	return nil
}

// Push pushes an image to a registry.
func (r *ContainerdImageRegistry) Push(ctx context.Context, ref string, opts outbound.ImagePushOptions) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	image, err := r.client.GetImage(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to get image: %w", err)
	}

	pushOpts := []containerd.RemoteOpt{}

	// Add authentication if provided
	if opts.AuthConfig != nil && opts.AuthConfig.Username != "" {
		resolver := docker.NewResolver(docker.ResolverOptions{
			Authorizer: docker.NewDockerAuthorizer(
				docker.WithAuthCreds(func(_ string) (string, string, error) {
					return opts.AuthConfig.Username, opts.AuthConfig.Password, nil
				}),
			),
		})
		pushOpts = append(pushOpts, containerd.WithResolver(resolver))
	}

	err = r.client.Push(ctx, ref, image.Target(), pushOpts...)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	return nil
}

// List lists all images in the local store.
func (r *ContainerdImageRegistry) List(ctx context.Context) ([]*outbound.RuntimeImageInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	imageList, err := r.client.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	result := make([]*outbound.RuntimeImageInfo, 0, len(imageList))
	for _, img := range imageList {
		info, err := r.imageToRuntimeInfo(ctx, img)
		if err != nil {
			continue // Skip images that can't be inspected
		}
		result = append(result, info)
	}

	return result, nil
}

// Remove removes an image from the local store.
func (r *ContainerdImageRegistry) Remove(ctx context.Context, imageID string, _ bool) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	imageStore := r.client.ImageService()
	err := imageStore.Delete(ctx, imageID, images.SynchronousDelete())
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to remove image: %w", err)
	}

	return nil
}

// Inspect inspects an image.
func (r *ContainerdImageRegistry) Inspect(ctx context.Context, imageRef string) (*outbound.RuntimeImageInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	image, err := r.client.GetImage(ctx, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	return r.imageToRuntimeInfo(ctx, image)
}

// ImageExists checks if an image exists in the local store.
func (r *ContainerdImageRegistry) ImageExists(ctx context.Context, ref string) (bool, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	_, err := r.client.GetImage(ctx, ref)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check image: %w", err)
	}

	return true, nil
}

// imageToRuntimeInfo converts a containerd image to RuntimeImageInfo.
func (r *ContainerdImageRegistry) imageToRuntimeInfo(ctx context.Context, img containerd.Image) (*outbound.RuntimeImageInfo, error) {
	target := img.Target()
	metadata := img.Metadata()

	size, err := img.Size(ctx)
	if err != nil {
		size = 0 // Don't fail if size can't be determined
	}

	info := &outbound.RuntimeImageInfo{
		ID:          target.Digest.String(),
		Digest:      target.Digest.String(),
		RepoTags:    []string{img.Name()},
		RepoDigests: []string{img.Name() + "@" + target.Digest.String()},
		Size:        size,
		Created:     metadata.CreatedAt,
		Labels:      metadata.Labels,
	}

	return info, nil
}
