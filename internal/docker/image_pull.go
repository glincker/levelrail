package docker

import "context"

// ImagePuller pulls an image onto the node's own image store ahead of use.
type ImagePuller interface {
	PullImage(ctx context.Context, ref string, auth *RegistryAuth) error
}

// PullImage implements ImagePuller. A locally present image is left alone, so a
// repeat call is cheap and a mutable tag is never re-pulled behind a running release.
func (c *Client) PullImage(ctx context.Context, ref string, auth *RegistryAuth) error {
	return c.ensureImage(ctx, ref, auth, false, nil)
}
