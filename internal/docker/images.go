package docker

import (
	"context"
	"fmt"
	"sort"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
)

// ImageRecord is one local image as the Engine API reports it.
type ImageRecord struct {
	ID          string
	Tags        []string
	RepoDigests []string
	SizeBytes   int64
	SharedBytes int64
	Containers  int64
	CreatedAt   time.Time
	Labels      map[string]string
}

// ImageLayer is one history entry of an image, newest layer first.
type ImageLayer struct {
	ID        string
	CreatedBy string
	SizeBytes int64
	CreatedAt time.Time
}

// ImageInventory lists and manages every image on a node.
type ImageInventory interface {
	ListAllImages(ctx context.Context) ([]ImageRecord, error)
	ImageLayerHistory(ctx context.Context, ref string) ([]ImageLayer, error)
	RemoveImage(ctx context.Context, id string) error
}

// ListAllImages returns every local image with its size, digests and the
// number of containers using it, newest first.
func (c *Client) ListAllImages(ctx context.Context) ([]ImageRecord, error) {
	summaries, err := c.cli.ImageList(ctx, image.ListOptions{SharedSize: true})
	if err != nil {
		return nil, fmt.Errorf("docker: list all images: %w", err)
	}
	out := make([]ImageRecord, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, ImageRecord{
			ID: s.ID, Tags: s.RepoTags, RepoDigests: s.RepoDigests,
			SizeBytes: s.Size, SharedBytes: s.SharedSize, Containers: s.Containers,
			CreatedAt: time.Unix(s.Created, 0).UTC(), Labels: s.Labels,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ImageLayerHistory returns the layers of ref, newest first. A missing
// image returns nil.
func (c *Client) ImageLayerHistory(ctx context.Context, ref string) ([]ImageLayer, error) {
	items, err := c.cli.ImageHistory(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("docker: image history %q: %w", ref, err)
	}
	out := make([]ImageLayer, 0, len(items))
	for _, it := range items {
		out = append(out, ImageLayer{ID: it.ID, CreatedBy: it.CreatedBy, SizeBytes: it.Size, CreatedAt: time.Unix(it.Created, 0).UTC()})
	}
	return out, nil
}

// RemoveImage deletes an image by ID without force, so Docker refuses when a
// container still uses it. A missing image is not an error.
func (c *Client) RemoveImage(ctx context.Context, id string) error {
	_, err := c.cli.ImageRemove(ctx, id, image.RemoveOptions{Force: false, PruneChildren: false})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("docker: remove image %q: %w", id, err)
	}
	return nil
}
