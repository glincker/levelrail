package docker

import (
	"context"
	"fmt"

	cerrdefs "github.com/containerd/errdefs"
)

// VolumeExists reports whether a named volume exists, whatever instance label
// it carries. ListNamedVolumes hides volumes owned by another instance, so a
// guard that must not mistake those for missing asks here instead.
func (c *Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	if _, err := c.cli.VolumeInspect(ctx, name); err != nil {
		if cerrdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("docker: inspect volume %q: %w", name, err)
	}
	return true, nil
}
