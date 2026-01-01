package domain

import "time"

// Image represents a container image.
type Image struct {
	CreatedAt time.Time
	Labels    map[string]string
	Ref       ImageRef
	ID        ImageID
	Digest    string
	Size      int64
}

// ImageInfo contains information about an image.
type ImageInfo struct {
	Created     time.Time
	Labels      map[string]string
	Ref         ImageRef
	ID          ImageID
	Digest      string
	RepoTags    []string
	RepoDigests []string
	Size        int64
}
