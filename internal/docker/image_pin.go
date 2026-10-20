package docker

import (
	"context"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/image"
)

// ImagePinner adds and removes extra tags on local images, so content a
// caller still needs never becomes dangling and eligible for pruning.
type ImagePinner interface {
	TagImage(ctx context.Context, source, target string) error
	UntagImage(ctx context.Context, ref string) error
}

// TagImage points target at the image source names (a tag or image ID).
func (c *Client) TagImage(ctx context.Context, source, target string) error {
	if err := c.cli.ImageTag(ctx, source, target); err != nil {
		return fmt.Errorf("docker: tag %q as %q: %w", source, target, err)
	}
	return nil
}

// UntagImage removes ref without force. Docker deletes the image only when
// ref was its last reference and no container uses it. A missing ref is
// not an error.
func (c *Client) UntagImage(ctx context.Context, ref string) error {
	_, err := c.cli.ImageRemove(ctx, ref, image.RemoveOptions{Force: false, PruneChildren: false})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("docker: untag %q: %w", ref, err)
	}
	return nil
}
